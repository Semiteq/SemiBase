package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"time"

	"github.com/Semiteq/SemiBase/internal/provision"
)

const usage = `semibase - brings a PostgreSQL instance to its provisioned state and exits.

Usage:
  semibase <command> [flags]

Commands:
  site     an installation machine: apply the memory tuning (ALTER SYSTEM + reload),
           then create the archive database, the roles, the access chain, public.trends,
           the four semiplot_* tables and semiplot_register_new_pens()
  bench    a throwaway container: the same, without the tuning
  version  print the build revision

Both commands end by checking the semiplot role in a transaction that is rolled back:
it reads public.trends and semiplot_meta, registers new pens through
semiplot_register_new_pens(), updates pen settings and groups, and is refused an insert,
a delete or a key change on semiplot_tags; it writes neither the archive nor
semiplot_meta and holds no CREATE on schema public. A failed check is a non-zero exit.

Every step checks before it creates; re-running either command is safe. Passwords of
existing roles change only when the corresponding flag or variable is set.

Passwords come from flags, environment variables, or a .env file in the working
directory (flag wins over environment, environment wins over .env):
  --super-password    SEMIBASE_SUPER_PASSWORD    superuser
  --writer-password   SEMIBASE_WRITER_PASSWORD   scada_writer
  --plot-password     SEMIBASE_PLOT_PASSWORD     semiplot

A first run creates scada_writer and semiplot and fails without their passwords; it also
creates semiplot_registrar, which owns the registration function and never logs in, so
it has no password. Once the roles exist,
the superuser password alone carries a run; the semiplot password then only decides
whether the run also tests the semiplot login, which it does over TCP.

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

func main() {
	loadDotEnv()
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
		return 1
	}
	fmt.Fprintf(output, "\nDone: %s completed against %s.\n",
		command, options.Endpoint())
	return 0
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

// One flag set serves both commands: site and bench differ only in the tuning phase,
// which reads no flag of its own, so every flag below is used by either command.
// Password flags default to empty so the usage output can only ever show the variable
// names, never their values.
func newFlagSet(command string, options *provision.Options) *flag.FlagSet {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(errorOutput)
	flags.StringVar(&options.Host, "host", "localhost",
		"server host, or a unix socket directory such as /var/run/postgresql")
	flags.IntVar(&options.Port, "port", 5432, "server port")
	flags.StringVar(&options.Database, "database", "scada_archive", "archive database name")
	flags.StringVar(&options.SuperUser, "superuser", "postgres", "superuser role name")
	flags.StringVar(&options.SuperPassword, "super-password", "",
		"superuser password (env SEMIBASE_SUPER_PASSWORD)")
	flags.StringVar(&options.WriterPassword, "writer-password", "",
		"scada_writer password (env SEMIBASE_WRITER_PASSWORD)")
	flags.StringVar(&options.PlotPassword, "plot-password", "",
		"semiplot password (env SEMIBASE_PLOT_PASSWORD)")
	flags.IntVar(&options.ExpectedMajor, "expected-major", 0,
		"required server major version; 0 accepts any major >= 14")
	return flags
}

func parseOptions(command string, arguments []string) (provision.Options, error) {
	options := provision.Options{}
	flags := newFlagSet(command, &options)
	if err := flags.Parse(arguments); err != nil {
		return provision.Options{}, err
	}
	resolvePasswordsFromEnvironment(&options)
	if err := options.Validate(); err != nil {
		fmt.Fprintln(flags.Output(), err)
		return provision.Options{}, err
	}
	return options, nil
}

func resolvePasswordsFromEnvironment(options *provision.Options) {
	if options.SuperPassword == "" {
		options.SuperPassword = os.Getenv("SEMIBASE_SUPER_PASSWORD")
	}
	if options.WriterPassword == "" {
		options.WriterPassword = os.Getenv("SEMIBASE_WRITER_PASSWORD")
	}
	if options.PlotPassword == "" {
		options.PlotPassword = os.Getenv("SEMIBASE_PLOT_PASSWORD")
	}
}

const dotEnvName = ".env"

// an unreadable file is reported so it does not degrade into a misleading
// missing-password error later
func loadDotEnv() {
	file, err := os.Open(dotEnvName)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(errorOutput, "warning: cannot open %s: %v\n", dotEnvName, err)
		}
		return
	}
	defer file.Close()
	if err := applyEnv(file); err != nil {
		fmt.Fprintf(errorOutput, "warning: cannot apply %s: %v\n", dotEnvName, err)
	}
}

func applyEnv(reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		name = strings.TrimSpace(name)
		value = trimMatchingQuotes(strings.TrimSpace(value))
		if name == "" {
			continue
		}
		if _, exists := os.LookupEnv(name); !exists {
			if err := os.Setenv(name, value); err != nil {
				return fmt.Errorf("setting %s: %w", name, err)
			}
		}
	}
	return scanner.Err()
}

// only a matching pair is removed, so a password that legitimately starts or
// ends with a quote passes intact
func trimMatchingQuotes(value string) string {
	if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
		return value[1 : len(value)-1]
	}
	return value
}
