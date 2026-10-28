package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/Semiteq/SemiBase/internal/engine"
	"github.com/Semiteq/SemiBase/internal/provision"
)

var passwordEnvironmentNames = []string{
	"SEMIBASE_SUPER_PASSWORD",
	"SEMIBASE_WRITER_PASSWORD",
	"SEMIBASE_PLOT_PASSWORD",
}

// callers must not use t.Parallel: the streams are package-level
func captureOutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	buffer := &bytes.Buffer{}
	previous := output
	output = buffer
	t.Cleanup(func() { output = previous })
	return buffer
}

func captureErrorOutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	buffer := &bytes.Buffer{}
	previous := errorOutput
	errorOutput = buffer
	t.Cleanup(func() { errorOutput = previous })
	return buffer
}

// t.Setenv registers restoration of the original value, the Unsetenv right after
// clears the variable from the process environment
func unsetEnvForTest(t *testing.T, names ...string) {
	t.Helper()
	for _, name := range names {
		t.Setenv(name, "")
		os.Unsetenv(name)
	}
}

func clearPasswordEnvironment(t *testing.T) {
	t.Helper()
	unsetEnvForTest(t, passwordEnvironmentNames...)
}

func TestFlagUsageHidesPasswordValues(t *testing.T) {
	secrets := map[string]string{
		"SEMIBASE_SUPER_PASSWORD":  "hunter2-super",
		"SEMIBASE_WRITER_PASSWORD": "hunter2-writer",
		"SEMIBASE_PLOT_PASSWORD":   "hunter2-plot",
	}
	for name, value := range secrets {
		t.Setenv(name, value)
	}

	options := provision.Options{}
	flags := newFlagSet("bench", &options)
	buffer := &bytes.Buffer{}
	flags.SetOutput(buffer)
	flags.PrintDefaults()

	rendered := buffer.String()
	for name, value := range secrets {
		if strings.Contains(rendered, value) {
			t.Errorf("usage output leaks the value of %s", name)
		}
		if !strings.Contains(rendered, name) {
			t.Errorf("usage output does not name %s", name)
		}
	}
}

func TestParseOptions(t *testing.T) {
	tests := []struct {
		name        string
		command     string
		arguments   []string
		environment map[string]string
		wantErr     error
		wantAnyErr  bool
		wantErrText string
		check       func(t *testing.T, options provision.Options)
	}{
		{
			name:    "site without --restart-service restarts nothing",
			command: "site",
			check: func(t *testing.T, options provision.Options) {
				if options.RestartService != "" {
					t.Errorf("RestartService = %q, want empty", options.RestartService)
				}
			},
		},
		{
			name:        "bench has no --restart-service",
			command:     "bench",
			arguments:   []string{"--restart-service", "x"},
			wantErrText: "flag provided but not defined: -restart-service",
		},
		{
			name:        "site rejects --restart-service with a non-loopback host",
			command:     "site",
			arguments:   []string{"--restart-service", "x", "--host", "db.internal"},
			wantErrText: "loopback",
		},
		{
			name:        "site rejects --restart-service with a socket host",
			command:     "site",
			arguments:   []string{"--restart-service", "x", "--host", "/var/run/postgresql"},
			wantErrText: "loopback",
		},
		{
			name:      "site accepts --restart-service with the default host",
			command:   "site",
			arguments: []string{"--restart-service", "postgresql-x64-17"},
			check: func(t *testing.T, options provision.Options) {
				if options.RestartService != "postgresql-x64-17" {
					t.Errorf("RestartService = %q, want %q", options.RestartService, "postgresql-x64-17")
				}
			},
		},
		{
			name:      "site accepts --restart-service with 127.0.0.1",
			command:   "site",
			arguments: []string{"--restart-service", "x", "--host", "127.0.0.1"},
		},
		{
			name:      "site accepts --restart-service with ::1",
			command:   "site",
			arguments: []string{"--restart-service", "x", "--host", "::1"},
		},
		{
			name:       "invalid database name rejected via Validate",
			arguments:  []string{"--database", "Bad-Name"},
			wantAnyErr: true,
		},
		{
			name:       "unknown flag returns an error",
			arguments:  []string{"--no-such-flag"},
			wantAnyErr: true,
		},
		{
			name:      "help returns flag.ErrHelp",
			arguments: []string{"--help"},
			wantErr:   flag.ErrHelp,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			captureErrorOutput(t)
			clearPasswordEnvironment(t)
			for name, value := range tt.environment {
				t.Setenv(name, value)
			}

			command := tt.command
			if command == "" {
				command = "bench"
			}
			options, err := parseOptions(command, tt.arguments)

			switch {
			case tt.wantErrText != "":
				if err == nil || !strings.Contains(err.Error(), tt.wantErrText) {
					t.Fatalf("parseOptions error = %v, want one containing %q", err, tt.wantErrText)
				}
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("parseOptions error = %v, want %v", err, tt.wantErr)
				}
			case tt.wantAnyErr:
				if err == nil {
					t.Fatal("parseOptions returned nil error, want an error")
				}
			default:
				if err != nil {
					t.Fatalf("parseOptions returned %v, want nil", err)
				}
			}
			if tt.check != nil {
				tt.check(t, options)
			}
		})
	}
}

func TestPasswordResolutionOrder(t *testing.T) {
	roles := []struct {
		role       string
		flag       string
		variable   string
		convention string
		password   func(provision.Options) string
	}{
		{"superuser", "--super-password", "SEMIBASE_SUPER_PASSWORD", "postgres",
			func(o provision.Options) string { return o.SuperPassword }},
		{"scada_writer", "--writer-password", "SEMIBASE_WRITER_PASSWORD", "",
			func(o provision.Options) string { return o.WriterPassword }},
		{"semiplot", "--plot-password", "SEMIBASE_PLOT_PASSWORD", "semiplot",
			func(o provision.Options) string { return o.PlotPassword }},
	}
	sources := []struct {
		name        string
		useFlag     bool
		useVariable bool
	}{
		{"flag beats the variable", true, true},
		{"variable fills an empty flag", false, true},
		{"convention fills both", false, false},
	}
	for _, role := range roles {
		for _, source := range sources {
			t.Run(role.role+"/"+source.name, func(t *testing.T) {
				captureErrorOutput(t)
				clearPasswordEnvironment(t)
				var arguments []string
				want := role.convention
				if source.useVariable {
					t.Setenv(role.variable, "from-variable")
					want = "from-variable"
				}
				if source.useFlag {
					arguments = []string{role.flag, "from-flag"}
					want = "from-flag"
				}

				options, err := parseOptions("bench", arguments)
				if err != nil {
					t.Fatalf("parseOptions returned %v, want nil", err)
				}
				if got := role.password(options); got != want {
					t.Errorf("%s password = %q, want %q", role.role, got, want)
				}
			})
		}
	}
}

func TestResolveRevisionReturnsLdflagsValueVerbatim(t *testing.T) {
	if got := resolveRevision("v1.2.3-abc1234"); got != "v1.2.3-abc1234" {
		t.Errorf("resolveRevision = %q, want %q", got, "v1.2.3-abc1234")
	}
}

func TestRevisionFromSettings(t *testing.T) {
	tests := []struct {
		name     string
		settings []debug.BuildSetting
		want     string
	}{
		{"no settings", nil, "unknown"},
		{
			"revision recorded",
			[]debug.BuildSetting{{Key: "vcs.revision", Value: "abc1234"}},
			"abc1234",
		},
		{
			"clean tree",
			[]debug.BuildSetting{
				{Key: "vcs.revision", Value: "abc1234"},
				{Key: "vcs.modified", Value: "false"},
			},
			"abc1234",
		},
		{
			"modified tree gets the dirty suffix",
			[]debug.BuildSetting{
				{Key: "vcs.revision", Value: "abc1234"},
				{Key: "vcs.modified", Value: "true"},
			},
			"abc1234-dirty",
		},
		{
			"modified without a revision stays unknown",
			[]debug.BuildSetting{{Key: "vcs.modified", Value: "true"}},
			"unknown",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := revisionFromSettings(tt.settings); got != tt.want {
				t.Errorf("revisionFromSettings = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunVersionCommandExitsZero(t *testing.T) {
	captureErrorOutput(t)
	buffer := captureOutput(t)

	for _, command := range []string{"version", "--version"} {
		buffer.Reset()
		if got := run(context.Background(), []string{command}); got != 0 {
			t.Errorf("run(%s) = %d, want exit code 0", command, got)
		}
		if buffer.Len() == 0 {
			t.Errorf("run(%s) printed no revision", command)
		}
	}
}

// the surface is two commands and nothing else: config, create, verify and all were
// removed with no alias and no tombstone
func TestCommandSurface(t *testing.T) {
	want := map[string]bool{"site": true, "bench": true}
	for name := range commands {
		if !want[name] {
			t.Errorf("unexpected command %q in the dispatch map", name)
		}
	}
	for name := range want {
		if _, known := commands[name]; !known {
			t.Errorf("command %q is missing from the dispatch map", name)
		}
	}
}

func TestRunUnmappedCommandIsAnError(t *testing.T) {
	buffer := captureErrorOutput(t)

	if got := run(context.Background(), []string{"frobnicate"}); got != 2 {
		t.Fatalf("run(frobnicate) = %d, want exit code 2", got)
	}
	if !strings.Contains(buffer.String(), `unknown command "frobnicate"`) {
		t.Errorf("error output does not report the unknown command: %q", buffer.String())
	}
}

func TestRunBareInvocationPrintsUsageToStderr(t *testing.T) {
	buffer := captureErrorOutput(t)

	if got := run(context.Background(), nil); got != 2 {
		t.Fatalf("run() = %d, want exit code 2", got)
	}
	if !strings.Contains(buffer.String(), "Usage:") {
		t.Errorf("error output does not contain the usage text: %q", buffer.String())
	}
}

func TestRunHelpFlagExitsZero(t *testing.T) {
	buffer := captureErrorOutput(t)
	clearPasswordEnvironment(t)

	if got := run(context.Background(), []string{"bench", "--help"}); got != 0 {
		t.Fatalf("run(bench --help) = %d, want exit code 0", got)
	}
	if !strings.Contains(buffer.String(), "-super-password") {
		t.Errorf("help output does not list the command flags: %q", buffer.String())
	}
}

func TestRunGivesCommandsADeadline(t *testing.T) {
	captureErrorOutput(t)
	captureOutput(t)
	clearPasswordEnvironment(t)

	// mutates the shared commands map, so no t.Parallel here
	var deadline time.Time
	var hasDeadline bool
	commands["stub-deadline"] = func(ctx context.Context, _ provision.Options) error {
		deadline, hasDeadline = ctx.Deadline()
		return nil
	}
	t.Cleanup(func() { delete(commands, "stub-deadline") })

	if got := run(context.Background(), []string{"stub-deadline"}); got != 0 {
		t.Fatalf("run(stub-deadline) = %d, want exit code 0", got)
	}
	if !hasDeadline {
		t.Fatal("command context carries no deadline")
	}
	// the lower bound catches a dispatch bug wrapping commands in a near-zero
	// timeout, which "at most phaseTimeout" alone would pass
	remaining := time.Until(deadline)
	if remaining > phaseTimeout || remaining < phaseTimeout/2 {
		t.Errorf("deadline is %v away, want close to %v", remaining, phaseTimeout)
	}
}

// the Done line is the only place the endpoint reaches an operator on success, and the
// socket spelling of it is what the init-script path prints
func TestRunReportsTheEndpointItRanAgainst(t *testing.T) {
	captureErrorOutput(t)
	buffer := captureOutput(t)
	clearPasswordEnvironment(t)

	// mutates the shared commands map, so no t.Parallel here
	commands["stub-done"] = func(context.Context, provision.Options) error { return nil }
	t.Cleanup(func() { delete(commands, "stub-done") })

	arguments := []string{"stub-done", "--host", "/var/run/postgresql", "--database", "semiplot_dev"}
	if got := run(context.Background(), arguments); got != 0 {
		t.Fatalf("run(stub-done) = %d, want exit code 0", got)
	}
	want := "Done: stub-done completed against /var/run/postgresql/.s.PGSQL.5432 (semiplot_dev)."
	if !strings.Contains(buffer.String(), want) {
		t.Errorf("output %q does not contain %q", buffer.String(), want)
	}
}

func TestRunNamesTheUsedUpTimeLimit(t *testing.T) {
	buffer := captureErrorOutput(t)
	clearPasswordEnvironment(t)

	// mutates the shared commands map, so no t.Parallel here
	commands["stub-deadline-exceeded"] = func(ctx context.Context, _ provision.Options) error {
		<-ctx.Done()
		return ctx.Err()
	}
	t.Cleanup(func() { delete(commands, "stub-deadline-exceeded") })

	// an expired parent deadline stands in for a command that used up phaseTimeout
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if got := run(ctx, []string{"stub-deadline-exceeded"}); got != 1 {
		t.Fatalf("run(stub-deadline-exceeded) = %d, want exit code 1", got)
	}
	if want := "ran out of its " + phaseTimeout.String() + " limit"; !strings.Contains(buffer.String(), want) {
		t.Errorf("error output %q does not contain %q", buffer.String(), want)
	}
}

func TestRunReportsCommandFailure(t *testing.T) {
	buffer := captureErrorOutput(t)
	clearPasswordEnvironment(t)

	// mutates the shared commands map, so no t.Parallel here
	commands["stub-failure"] = func(context.Context, provision.Options) error {
		return errors.New("stub-failure sentinel")
	}
	t.Cleanup(func() { delete(commands, "stub-failure") })

	if got := run(context.Background(), []string{"stub-failure"}); got != 1 {
		t.Fatalf("run(stub-failure) = %d, want exit code 1", got)
	}
	if !strings.Contains(buffer.String(), "error: stub-failure sentinel") {
		t.Errorf("error output does not report the failure: %q", buffer.String())
	}
	if strings.Contains(buffer.String(), "limit") {
		t.Errorf("error output blames the time limit for an ordinary failure: %q", buffer.String())
	}
}

func stubDetectEngine(t *testing.T, result engine.Result, err error) *int {
	t.Helper()
	requested := new(int)
	previous := detectEngine
	detectEngine = func(expectedMajor int) (engine.Result, error) {
		*requested = expectedMajor
		return result, err
	}
	t.Cleanup(func() { detectEngine = previous })
	return requested
}

func TestRunEnginePrintsEachState(t *testing.T) {
	found := engine.Service{
		Name:        "postgresql-x64-17",
		Base:        `C:\Program Files\PostgreSQL\17`,
		Data:        `C:\Program Files\PostgreSQL\17\data`,
		Major:       17,
		Running:     true,
		Port:        5432,
		PortFile:    `C:\Program Files\PostgreSQL\17\data\postgresql.conf`,
		Listening:   5432,
		Uninstaller: true,
	}
	tests := []struct {
		name   string
		result engine.Result
		want   string
	}{
		{
			name: "found",
			result: engine.Result{State: engine.StateFound, Service: found,
				Detail: `service postgresql-x64-17 runs PostgreSQL 17 from C:\Program Files\PostgreSQL\17\data on port 5432`},
			want: `state=found
service=postgresql-x64-17
base=C:\Program Files\PostgreSQL\17
data=C:\Program Files\PostgreSQL\17\data
port=5432
port-file=C:\Program Files\PostgreSQL\17\data\postgresql.conf
listening=5432
uninstaller=yes
unreadable=
detail=service postgresql-x64-17 runs PostgreSQL 17 from C:\Program Files\PostgreSQL\17\data on port 5432
`,
		},
		{
			name: "port-mismatch",
			result: engine.Result{State: engine.StatePortMismatch,
				Service: engine.Service{Name: "postgresql-x64-17", Base: `C:\PG`, Data: `C:\PG\data`,
					Major: 17, Running: true, Port: 15433, PortFile: `C:\PG\data\postgresql.conf`,
					Listening: 5432, Uninstaller: true},
				Detail: `service postgresql-x64-17 listens on port 5432, and C:\PG\data\postgresql.conf sets port 15433`},
			want: `state=port-mismatch
service=postgresql-x64-17
base=C:\PG
data=C:\PG\data
port=15433
port-file=C:\PG\data\postgresql.conf
listening=5432
uninstaller=yes
unreadable=
detail=service postgresql-x64-17 listens on port 5432, and C:\PG\data\postgresql.conf sets port 15433
`,
		},
		{
			name: "incomplete leaves the unknown numbers empty",
			result: engine.Result{State: engine.StateIncomplete,
				Service: engine.Service{Name: "postgresql-ci-17", Base: `C:\ci`, Data: `C:\ci\data`,
					Problem: `service postgresql-ci-17: no file C:\ci\data\PG_VERSION`},
				Detail: `service postgresql-ci-17: no file C:\ci\data\PG_VERSION`},
			want: `state=incomplete
service=postgresql-ci-17
base=C:\ci
data=C:\ci\data
port=
port-file=
listening=
uninstaller=no
unreadable=
detail=service postgresql-ci-17: no file C:\ci\data\PG_VERSION
`,
		},
		{
			name: "unreadable names what could not be read",
			result: engine.Result{State: engine.StateUnreadable,
				Service: engine.Service{Name: "postgresql-x64-17", Base: `C:\PG`, Data: `C:\PG\data`,
					Major: 17, Uninstaller: true,
					Unreadable: `C:\PG\data\postgresql.conf line 2: syntax error after the value of port`},
				Detail: `service postgresql-x64-17: C:\PG\data\postgresql.conf line 2: syntax error after the value of port`},
			want: `state=unreadable
service=postgresql-x64-17
base=C:\PG
data=C:\PG\data
port=
port-file=
listening=
uninstaller=yes
unreadable=C:\PG\data\postgresql.conf line 2: syntax error after the value of port
detail=service postgresql-x64-17: C:\PG\data\postgresql.conf line 2: syntax error after the value of port
`,
		},
		{
			name: "ambiguous names the services",
			result: engine.Result{State: engine.StateAmbiguous,
				Services: []string{"postgresql-ci-17", "postgresql-x64-17"},
				Detail:   "services postgresql-ci-17, postgresql-x64-17 run PostgreSQL 17 or a server of unknown major"},
			want: `state=ambiguous
services=postgresql-ci-17, postgresql-x64-17
detail=services postgresql-ci-17, postgresql-x64-17 run PostgreSQL 17 or a server of unknown major
`,
		},
		{
			name: "other-major",
			result: engine.Result{State: engine.StateOtherMajor,
				Detail: "no service runs PostgreSQL 17; postgresql-ci-14 runs PostgreSQL 14"},
			want: `state=other-major
detail=no service runs PostgreSQL 17; postgresql-ci-14 runs PostgreSQL 14
`,
		},
		{
			name:   "fresh",
			result: engine.Result{State: engine.StateFresh, Detail: "no service runs pg_ctl.exe runservice"},
			want: `state=fresh
detail=no service runs pg_ctl.exe runservice
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errorBuffer := captureErrorOutput(t)
			buffer := captureOutput(t)
			requested := stubDetectEngine(t, tt.result, nil)

			if got := run(context.Background(), []string{"engine", "--expected-major", "17"}); got != 0 {
				t.Fatalf("run(engine) = %d, want exit code 0; stderr %q", got, errorBuffer.String())
			}
			if *requested != 17 {
				t.Errorf("detection asked for major %d, want 17", *requested)
			}
			if buffer.String() != tt.want {
				t.Errorf("output\n%s\nwant\n%s", buffer.String(), tt.want)
			}
		})
	}
}

func TestRunEngineRefusals(t *testing.T) {
	tests := []struct {
		name       string
		arguments  []string
		detectErr  error
		wantCode   int
		wantStderr string
	}{
		{"no --expected-major", nil, nil, 2, "engine needs --expected-major"},
		{"zero --expected-major", []string{"--expected-major", "0"}, nil, 2, "engine needs --expected-major"},
		{"non-numeric --expected-major", []string{"--expected-major", "x"}, nil, 2, "invalid value"},
		{"connection flags are not engine flags", []string{"--expected-major", "17", "--port", "5432"},
			nil, 2, "flag provided but not defined: -port"},
		{"password flags are not engine flags", []string{"--expected-major", "17", "--super-password", "x"},
			nil, 2, "flag provided but not defined: -super-password"},
		{"a positional argument", []string{"--expected-major", "17", "extra"}, nil, 2, "engine takes no arguments"},
		{"help", []string{"--help"}, nil, 0, "-expected-major"},
		{"the service list cannot be read", []string{"--expected-major", "17"},
			errors.New("engine detection needs the Windows service list"), 1,
			"error: engine detection needs the Windows service list"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errorBuffer := captureErrorOutput(t)
			buffer := captureOutput(t)
			stubDetectEngine(t, engine.Result{State: engine.StateFresh}, tt.detectErr)

			if got := run(context.Background(), append([]string{"engine"}, tt.arguments...)); got != tt.wantCode {
				t.Fatalf("run(engine %v) = %d, want exit code %d", tt.arguments, got, tt.wantCode)
			}
			if !strings.Contains(errorBuffer.String(), tt.wantStderr) {
				t.Errorf("error output %q does not contain %q", errorBuffer.String(), tt.wantStderr)
			}
			if buffer.Len() != 0 {
				t.Errorf("standard output %q, want nothing", buffer.String())
			}
		})
	}
}

func TestCancelledContextReturnsPromptly(t *testing.T) {
	previous := provision.SetConsoleOutput(io.Discard)
	t.Cleanup(func() { provision.SetConsoleOutput(previous) })

	options := provision.Options{
		Host:      "localhost",
		Port:      5432,
		Database:  "scada_archive",
		SuperUser: "postgres",
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	started := time.Now()
	err := commands["bench"](ctx, options)
	elapsed := time.Since(started)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("bench with cancelled context returned %v, want context.Canceled in the chain", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("bench with cancelled context took %v, want a prompt return", elapsed)
	}
}
