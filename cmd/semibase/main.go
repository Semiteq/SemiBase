// Command semibase provisions the SemiBase PostgreSQL instance.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/Semiteq/SemiBase/internal/provision"
)

const usage = `semibase - provisions the SemiBase PostgreSQL instance.

Usage:
  semibase <command> [flags]

Commands:
  config   apply the server configuration deltas (ALTER SYSTEM); service restart required
  create   create the archive database, the roles, the access chain, and semiplot_tags
  verify   post-writer checks: archive tables exist, the reader reads and cannot write
  all      config + create + verify

Every step checks before it creates; re-running any command is safe. Passwords of
existing roles change only when the corresponding flag or variable is set.

Passwords come from flags, environment variables, or a .env file in the working
directory (flag wins over environment, environment wins over .env):
  --super-password    SEMIBASE_SUPER_PASSWORD    superuser
  --writer-password   SEMIBASE_WRITER_PASSWORD   scada_writer
  --reader-password   SEMIBASE_READER_PASSWORD   semiplot_reader
  --admin-password    SEMIBASE_ADMIN_PASSWORD    semiplot_admin

Run 'semibase <command> --help' for the command's flags.
`

var databaseNamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

func main() {
	provision.EnableColors()
	loadDotEnv()
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(2)
	}
	command := os.Args[1]
	switch command {
	case "config", "create", "verify", "all":
	case "-h", "--help", "help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", command, usage)
		os.Exit(2)
	}

	options, err := parseOptions(command, os.Args[2:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if err := run(command, options); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Printf("\nDone: %s completed against %s:%d/%s.\n", command, options.Host, options.Port, options.Database)
}

func parseOptions(command string, arguments []string) (provision.Options, error) {
	flags := flag.NewFlagSet(command, flag.ExitOnError)
	options := provision.Options{}

	flags.StringVar(&options.Host, "host", "localhost", "server host")
	flags.IntVar(&options.Port, "port", 5432, "server port")
	flags.StringVar(&options.Database, "database", "scada_archive", "archive database name")
	flags.StringVar(&options.SuperUser, "superuser", "postgres", "superuser role name")
	flags.StringVar(&options.SuperPassword, "super-password",
		os.Getenv("SEMIBASE_SUPER_PASSWORD"), "superuser password (env SEMIBASE_SUPER_PASSWORD)")
	flags.StringVar(&options.WriterPassword, "writer-password",
		os.Getenv("SEMIBASE_WRITER_PASSWORD"), "scada_writer password (env SEMIBASE_WRITER_PASSWORD)")
	flags.StringVar(&options.ReaderPassword, "reader-password",
		os.Getenv("SEMIBASE_READER_PASSWORD"), "semiplot_reader password (env SEMIBASE_READER_PASSWORD)")
	flags.StringVar(&options.AdminPassword, "admin-password",
		os.Getenv("SEMIBASE_ADMIN_PASSWORD"), "semiplot_admin password (env SEMIBASE_ADMIN_PASSWORD)")
	flags.IntVar(&options.ExpectedMajor, "expected-major", 0,
		"required server major version; 0 accepts any major >= 14")
	flags.StringVar(&options.ServiceName, "service", "",
		"Windows service to restart after config, e.g. postgresql-x64-17")

	if err := flags.Parse(arguments); err != nil {
		return provision.Options{}, err
	}
	if !databaseNamePattern.MatchString(options.Database) {
		return provision.Options{}, fmt.Errorf("database name %q must match %s", options.Database, databaseNamePattern)
	}
	return options, nil
}

// loadDotEnv applies variables from a .env file in the working directory.
// Variables already present in the process environment win; flags win over both.
func loadDotEnv() {
	file, err := os.Open(".env")
	if err != nil {
		return
	}
	defer file.Close()
	applyEnv(file)
}

func applyEnv(reader io.Reader) {
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
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if name == "" {
			continue
		}
		if _, exists := os.LookupEnv(name); !exists {
			os.Setenv(name, value)
		}
	}
}

func run(command string, options provision.Options) error {
	ctx := context.Background()
	switch command {
	case "config":
		return options.Config(ctx)
	case "create":
		return options.Create(ctx)
	case "verify":
		return options.Verify(ctx)
	default:
		return options.All(ctx)
	}
}
