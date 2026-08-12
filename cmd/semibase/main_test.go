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

	"github.com/Semiteq/SemiBase/internal/provision"
)

var passwordEnvironmentNames = []string{
	"SEMIBASE_SUPER_PASSWORD",
	"SEMIBASE_WRITER_PASSWORD",
	"SEMIBASE_READER_PASSWORD",
	"SEMIBASE_ADMIN_PASSWORD",
}

// captureOutput redirects the package success stream into a buffer for the
// duration of the test. Tests mutating it must not run in parallel.
func captureOutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	buffer := &bytes.Buffer{}
	previous := output
	output = buffer
	t.Cleanup(func() { output = previous })
	return buffer
}

// captureErrorOutput redirects the package error stream into a buffer for the
// duration of the test. Tests mutating it must not run in parallel.
func captureErrorOutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	buffer := &bytes.Buffer{}
	previous := errorOutput
	errorOutput = buffer
	t.Cleanup(func() { errorOutput = previous })
	return buffer
}

// unsetEnvForTest removes the variables for the duration of the test:
// t.Setenv registers restoration of the original value, and the Unsetenv
// right after actually clears the variable from the process environment.
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
		"SEMIBASE_READER_PASSWORD": "hunter2-reader",
		"SEMIBASE_ADMIN_PASSWORD":  "hunter2-admin",
	}
	for name, value := range secrets {
		t.Setenv(name, value)
	}

	options := provision.Options{}
	flags := newFlagSet("create", &options)
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
		arguments   []string
		environment map[string]string
		wantErr     error
		wantAnyErr  bool
		check       func(t *testing.T, options provision.Options)
	}{
		{
			name:        "flag beats environment",
			arguments:   []string{"--super-password", "from-flag"},
			environment: map[string]string{"SEMIBASE_SUPER_PASSWORD": "from-env"},
			check: func(t *testing.T, options provision.Options) {
				if options.SuperPassword != "from-flag" {
					t.Errorf("SuperPassword = %q, want %q", options.SuperPassword, "from-flag")
				}
			},
		},
		{
			name:        "environment fills empty flag",
			environment: map[string]string{"SEMIBASE_WRITER_PASSWORD": "from-env"},
			check: func(t *testing.T, options provision.Options) {
				if options.WriterPassword != "from-env" {
					t.Errorf("WriterPassword = %q, want %q", options.WriterPassword, "from-env")
				}
			},
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

			options, err := parseOptions("create", tt.arguments)

			switch {
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

	if got := run(context.Background(), []string{"create", "--help"}); got != 0 {
		t.Fatalf("run(create --help) = %d, want exit code 0", got)
	}
	if !strings.Contains(buffer.String(), "-super-password") {
		t.Errorf("help output does not list the command flags: %q", buffer.String())
	}
}

func TestRunGivesCommandsADeadline(t *testing.T) {
	captureErrorOutput(t)
	captureOutput(t)
	clearPasswordEnvironment(t)

	// Mutates the shared commands map, so this test must not use t.Parallel.
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
	// The lower bound catches a dispatch bug wrapping commands in a
	// near-zero timeout, which "at most phaseTimeout" alone would pass.
	remaining := time.Until(deadline)
	if remaining > phaseTimeout || remaining < phaseTimeout/2 {
		t.Errorf("deadline is %v away, want close to %v", remaining, phaseTimeout)
	}
}

func TestRunReportsCommandFailure(t *testing.T) {
	buffer := captureErrorOutput(t)
	clearPasswordEnvironment(t)

	// Mutates the shared commands map, so this test must not use t.Parallel.
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
}

func TestCancelledContextReturnsPromptly(t *testing.T) {
	// Create prints its step banner through the provision console; discard it
	// so the test output stays clean.
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
	err := commands["create"](ctx, options)
	elapsed := time.Since(started)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("create with cancelled context returned %v, want context.Canceled in the chain", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("create with cancelled context took %v, want a prompt return", elapsed)
	}
}
