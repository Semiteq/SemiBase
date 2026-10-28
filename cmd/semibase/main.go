package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/Semiteq/SemiBase/internal/engine"
	"github.com/Semiteq/SemiBase/internal/provision"
)

const usage = `semibase - brings a PostgreSQL instance to its provisioned state and exits.

Usage:
  semibase <command> [flags]

Commands:
  site     an installation machine: apply the server settings (ALTER SYSTEM + reload),
           then create the archive database, the roles, the access chain, public.trends,
           the five semiplot_* tables and semiplot_register_new_pens()
  bench    a throwaway container: the same, without the tuning
  engine   find the Windows service that runs PostgreSQL of --expected-major and print its
           state and facts as key=value lines; it reads the service list and the data
           folder, opens no connection and needs no elevation
  version  print the build revision

site and bench end by checking the semiplot role in a transaction that is rolled back:
it reads public.trends and semiplot_meta, registers new pens through
semiplot_register_new_pens(), updates pen settings, groups and markers, and is
refused an insert, a delete or a key change on semiplot_tags; it writes neither the
archive nor semiplot_meta and holds no CREATE on schema public. A failed check is a
non-zero exit.

The tuning includes settings that take effect only after a server restart. site
reports them and leaves the restart to the operator, unless --restart-service names
the Windows service to restart; the restart needs a loopback --host, because the
server then listens on the loopback interface only.

Every step checks before it creates; re-running either command is safe.

Passwords come from a flag, then an environment variable, then the convention that a
role's password is its name:
  --super-password    SEMIBASE_SUPER_PASSWORD    superuser      postgres
  --writer-password   SEMIBASE_WRITER_PASSWORD   scada_writer   scada_writer
  --plot-password     SEMIBASE_PLOT_PASSWORD     semiplot       semiplot

semibase never changes the superuser's password: on a server whose superuser password
is not postgres, pass the real one. Every run sets the semiplot password, creating or
resetting the role with it, and tests the semiplot login over TCP unless --host is a
socket directory. scada_writer gets its password only when the role is created: the SCADA
project holds the same password, so no run changes it. semiplot_registrar owns the
registration function and never logs in, so it has no password.

Run 'semibase <command> --help' for the command's flags.
`

// set by a release with -ldflags "-X main.revision=..."
var revision string

const phaseTimeout = 5 * time.Minute

// variables so tests can capture the streams
var (
	output      io.Writer = os.Stdout
	errorOutput io.Writer = os.Stderr
)

// only tests write to this map, to insert stubs
var commands = map[string]func(context.Context, provision.Options) error{
	"site":  func(ctx context.Context, options provision.Options) error { return options.Site(ctx) },
	"bench": func(ctx context.Context, options provision.Options) error { return options.Bench(ctx) },
}

var detectEngine = engine.Detect

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	exitCode := run(ctx, os.Args[1:])
	stop()
	os.Exit(exitCode)
}

func run(ctx context.Context, arguments []string) int {
	if len(arguments) == 0 {
		fmt.Fprint(errorOutput, usage)
		return 2
	}
	command := arguments[0]
	switch command {
	case "help", "-h", "--help":
		fmt.Fprint(output, usage)
		return 0
	case "version", "--version":
		fmt.Fprintln(output, resolveRevision(revision))
		return 0
	case "engine":
		return runEngine(arguments[1:])
	}

	execute, known := commands[command]
	if !known {
		fmt.Fprintf(errorOutput, "unknown command %q\n\n%s", command, usage)
		return 2
	}

	options, err := parseOptions(command, arguments[1:])
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		// parse and validation failures are already reported at the flag set's output
		return 2
	}

	commandContext, cancel := context.WithTimeout(ctx, phaseTimeout)
	defer cancel()
	if err := execute(commandContext, options); err != nil {
		fmt.Fprintln(errorOutput, "error:", err)
		if errors.Is(commandContext.Err(), context.DeadlineExceeded) {
			fmt.Fprintf(errorOutput, "%s ran out of its %s limit; every step checks before it creates, "+
				"so re-running it continues from where it stopped\n", command, phaseTimeout)
		}
		return 1
	}
	fmt.Fprintf(output, "\nDone: %s completed against %s.\n",
		command, options.Endpoint())
	return 0
}

func runEngine(arguments []string) int {
	flags := flag.NewFlagSet("engine", flag.ContinueOnError)
	flags.SetOutput(errorOutput)
	expectedMajor := flags.Int("expected-major", 0, "the PostgreSQL major version to find, such as 17")
	if err := flags.Parse(arguments); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(errorOutput, "engine takes no arguments, got %q\n", flags.Args())
		return 2
	}
	if *expectedMajor <= 0 {
		fmt.Fprintln(errorOutput, "engine needs --expected-major, such as --expected-major 17")
		return 2
	}

	result, err := detectEngine(*expectedMajor)
	if err != nil {
		fmt.Fprintln(errorOutput, "error:", err)
		return 1
	}
	writeEngineResult(output, result)
	return 0
}

// writeEngineResult prints the lines Setup reads; a number that is not known prints empty.
func writeEngineResult(w io.Writer, result engine.Result) {
	fmt.Fprintf(w, "state=%s\n", result.State)
	if len(result.Services) > 0 {
		fmt.Fprintf(w, "services=%s\n", strings.Join(result.Services, ", "))
	}
	if service := result.Service; service.Name != "" {
		uninstaller := "no"
		if service.Uninstaller {
			uninstaller = "yes"
		}
		fmt.Fprintf(w, "service=%s\n", service.Name)
		fmt.Fprintf(w, "base=%s\n", service.Base)
		fmt.Fprintf(w, "data=%s\n", service.Data)
		fmt.Fprintf(w, "port=%s\n", knownNumber(service.Port))
		fmt.Fprintf(w, "port-file=%s\n", service.PortFile)
		fmt.Fprintf(w, "listening=%s\n", knownNumber(service.Listening))
		fmt.Fprintf(w, "uninstaller=%s\n", uninstaller)
		fmt.Fprintf(w, "unreadable=%s\n", service.Unreadable)
	}
	fmt.Fprintf(w, "detail=%s\n", result.Detail)
}

func knownNumber(number int) string {
	if number == 0 {
		return ""
	}
	return strconv.Itoa(number)
}

func resolveRevision(embedded string) string {
	if embedded != "" {
		return embedded
	}
	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	return revisionFromSettings(buildInfo.Settings)
}

func revisionFromSettings(settings []debug.BuildSetting) string {
	vcsRevision := ""
	dirty := false
	for _, setting := range settings {
		switch setting.Key {
		case "vcs.revision":
			vcsRevision = setting.Value
		case "vcs.modified":
			dirty = setting.Value == "true"
		}
	}
	if vcsRevision == "" {
		return "unknown"
	}
	if dirty {
		return vcsRevision + "-dirty"
	}
	return vcsRevision
}

// Password flags default to empty so the usage output can only ever show the variable names,
// never their values.
func newFlagSet(command string, options *provision.Options) *flag.FlagSet {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(errorOutput)
	flags.StringVar(&options.Host, "host", "localhost",
		"server host, or a unix socket directory such as /var/run/postgresql")
	flags.IntVar(&options.Port, "port", 5432, "server port")
	flags.StringVar(&options.Database, "database", "scada_archive", "archive database name")
	flags.StringVar(&options.SuperUser, "superuser", "postgres", "superuser role name")
	flags.StringVar(&options.SuperPassword, "super-password", "",
		"superuser password (env SEMIBASE_SUPER_PASSWORD, else postgres)")
	flags.StringVar(&options.WriterPassword, "writer-password", "",
		"scada_writer password for a new role (env SEMIBASE_WRITER_PASSWORD, else scada_writer); "+
			"an existing role keeps its own")
	flags.StringVar(&options.PlotPassword, "plot-password", "",
		"semiplot password (env SEMIBASE_PLOT_PASSWORD, else semiplot)")
	flags.IntVar(&options.ExpectedMajor, "expected-major", 0,
		"required server major version; 0 accepts any major >= 14")
	if command == "site" {
		flags.StringVar(&options.RestartService, "restart-service", "",
			"Windows service to restart when the server settings wait for a restart; "+
				"needs a loopback --host")
	}
	return flags
}

func parseOptions(command string, arguments []string) (provision.Options, error) {
	options := provision.Options{}
	flags := newFlagSet(command, &options)
	if err := flags.Parse(arguments); err != nil {
		return provision.Options{}, err
	}
	resolvePasswords(&options)
	if err := options.Validate(); err != nil {
		fmt.Fprintln(flags.Output(), err)
		return provision.Options{}, err
	}
	return options, nil
}

const conventionalSuperPassword = "postgres"

// an empty WriterPassword stays empty: it creates a missing role with the convention and keeps
// an existing role's password, which the SCADA holds
func resolvePasswords(options *provision.Options) {
	options.SuperPassword = cmp.Or(options.SuperPassword,
		os.Getenv("SEMIBASE_SUPER_PASSWORD"), conventionalSuperPassword)
	options.WriterPassword = cmp.Or(options.WriterPassword, os.Getenv("SEMIBASE_WRITER_PASSWORD"))
	options.PlotPassword = cmp.Or(options.PlotPassword,
		os.Getenv("SEMIBASE_PLOT_PASSWORD"), provision.PlotRole)
}
