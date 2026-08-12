// Package provision creates and verifies the SemiBase PostgreSQL instance:
// server configuration, the archive database, the roles, the reader access
// chain, and semiplot_tags.
package provision

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	semibase "github.com/Semiteq/SemiBase"
)

// Roles of the archive database. The SCADA writes, the viewers read,
// commissioning owns the semiplot_* objects.
const (
	WriterRole = "scada_writer"
	ReaderRole = "semiplot_reader"
	AdminRole  = "semiplot_admin"
)

// versionFloor is PostgreSQL 14, the oldest release with date_bin, which the
// readers' bucketing query uses.
const versionFloor = 140000

// Options carries everything the phases need. Zero passwords mean "leave the
// existing credential untouched".
type Options struct {
	Host           string
	Port           int
	Database       string
	SuperUser      string
	SuperPassword  string
	WriterPassword string
	ReaderPassword string
	AdminPassword  string
	ExpectedMajor  int
	ServiceName    string
}

// databaseNamePattern is the identifier gate in front of every DDL statement
// that interpolates the database name.
var databaseNamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// Validate checks the invariants the phases rely on before any SQL runs.
func (o Options) Validate() error {
	if !databaseNamePattern.MatchString(o.Database) {
		return fmt.Errorf("database name %q must match %s", o.Database, databaseNamePattern)
	}
	return nil
}

// All runs config, create, and verify in order. A writer that has not run yet
// is reported as a state, not a failure.
func (o Options) All(ctx context.Context) error {
	if err := o.Config(ctx); err != nil {
		return err
	}
	if err := o.Create(ctx); err != nil {
		return err
	}
	return o.verify(ctx, true)
}

// Verify runs the post-writer checks and fails when the writer has not run.
func (o Options) Verify(ctx context.Context) error {
	return o.verify(ctx, false)
}

func (o Options) connect(ctx context.Context, database, user, password string) (*pgx.Conn, error) {
	target := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   net.JoinHostPort(o.Host, strconv.Itoa(o.Port)),
		Path:   "/" + database,
	}
	conn, err := pgx.Connect(ctx, target.String())
	if err != nil {
		return nil, fmt.Errorf("connecting to %s:%d/%s as %s: %w", o.Host, o.Port, database, user, err)
	}
	// escapeLiteral relies on this session setting: with conforming strings on,
	// a backslash is an ordinary character inside '...' literals, so doubling
	// single quotes is a complete escape. USERSET, so no privilege is needed.
	if _, err := conn.Exec(ctx, "SET standard_conforming_strings = on"); err != nil {
		_ = conn.Close(ctx)
		return nil, fmt.Errorf("setting standard_conforming_strings: %w", err)
	}
	return conn, nil
}

// escapeLiteral doubles single quotes. This is sufficient in every server mode
// only because connect forces standard_conforming_strings = on for the session;
// under that setting backslashes carry no escape meaning inside '...' literals.
func escapeLiteral(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

// The statement builders below are the only places a database name or a
// password reaches DDL text; tests pin their output for hostile input.

func grantConnectStatement(database, role string) string {
	return fmt.Sprintf("GRANT CONNECT ON DATABASE %s TO %s", pgx.Identifier{database}.Sanitize(), role)
}

func createDatabaseStatement(database string) string {
	return "CREATE DATABASE " + pgx.Identifier{database}.Sanitize()
}

func createRoleStatement(name, password string) string {
	return fmt.Sprintf("CREATE ROLE %s LOGIN PASSWORD '%s'", name, escapeLiteral(password))
}

func alterRolePasswordStatement(name, password string) string {
	return fmt.Sprintf("ALTER ROLE %s PASSWORD '%s'", name, escapeLiteral(password))
}

// Create provisions the archive database, the three roles, the grants, the
// default privileges, and semiplot_tags. Every step checks before it creates.
func (o Options) Create(ctx context.Context) error {
	if err := o.Validate(); err != nil {
		return err
	}
	step("Phase create: database %s, roles, access chain", o.Database)

	super, err := o.connect(ctx, "postgres", o.SuperUser, o.SuperPassword)
	if err != nil {
		return err
	}
	defer super.Close(ctx)

	if err := o.assertServerVersion(ctx, super); err != nil {
		return err
	}

	roles := []struct {
		name     string
		password string
	}{
		{WriterRole, o.WriterPassword},
		{ReaderRole, o.ReaderPassword},
		{AdminRole, o.AdminPassword},
	}
	for _, role := range roles {
		if err := ensureRole(ctx, super, role.name, role.password); err != nil {
			return err
		}
	}

	if err := o.ensureDatabase(ctx, super); err != nil {
		return err
	}
	for _, role := range roles {
		grant := grantConnectStatement(o.Database, role.name)
		if _, err := super.Exec(ctx, grant); err != nil {
			return fmt.Errorf("%s: %w", grant, err)
		}
	}

	readerSettings := []string{
		fmt.Sprintf("ALTER ROLE %s SET statement_timeout = '30s'", ReaderRole),
		fmt.Sprintf("ALTER ROLE %s SET idle_in_transaction_session_timeout = '60s'", ReaderRole),
	}
	for _, statement := range readerSettings {
		if _, err := super.Exec(ctx, statement); err != nil {
			return fmt.Errorf("%s: %w", statement, err)
		}
	}
	ok("%s timeouts set (statement 30s, idle-in-transaction 60s)", ReaderRole)

	archive, err := o.connect(ctx, o.Database, o.SuperUser, o.SuperPassword)
	if err != nil {
		return err
	}
	defer archive.Close(ctx)

	// PostgreSQL 15+ no longer grants CREATE on schema public to everyone; on 14 this is redundant.
	if _, err := archive.Exec(ctx, "GRANT CREATE ON SCHEMA public TO "+WriterRole); err != nil {
		return fmt.Errorf("granting CREATE on schema public: %w", err)
	}

	// The reader access chain: trends/messages do not exist until the SCADA runs, so SELECT on
	// them can only be granted ahead of time, through default privileges of their future owner.
	defaultPrivileges := fmt.Sprintf(
		"ALTER DEFAULT PRIVILEGES FOR ROLE %s IN SCHEMA public GRANT SELECT ON TABLES TO %s",
		WriterRole, ReaderRole)
	if _, err := archive.Exec(ctx, defaultPrivileges); err != nil {
		return fmt.Errorf("%s: %w", defaultPrivileges, err)
	}
	ok("default privileges set - tables %s creates are readable by %s", WriterRole, ReaderRole)

	if _, err := archive.Exec(ctx, semibase.SemiplotTagsSQL); err != nil {
		return fmt.Errorf("applying semiplot_tags.sql: %w", err)
	}
	if _, err := archive.Exec(ctx, "ALTER TABLE semiplot_tags OWNER TO "+AdminRole); err != nil {
		return fmt.Errorf("setting semiplot_tags owner: %w", err)
	}
	if _, err := archive.Exec(ctx, "GRANT SELECT ON semiplot_tags TO "+ReaderRole); err != nil {
		return fmt.Errorf("granting SELECT on semiplot_tags: %w", err)
	}
	ok("semiplot_tags in place, owned by %s, readable by %s", AdminRole, ReaderRole)
	return nil
}

func (o Options) assertServerVersion(ctx context.Context, conn *pgx.Conn) error {
	var versionNumber int
	if err := conn.QueryRow(ctx, "SELECT current_setting('server_version_num')::int").Scan(&versionNumber); err != nil {
		return fmt.Errorf("reading server version: %w", err)
	}
	major := versionNumber / 10000
	if o.ExpectedMajor > 0 && major != o.ExpectedMajor {
		return fmt.Errorf("server major version is %d, expected %d", major, o.ExpectedMajor)
	}
	if versionNumber < versionFloor {
		return fmt.Errorf("server version %d is below the floor %d (PostgreSQL 14, date_bin)", versionNumber, versionFloor)
	}
	ok("server version %d (major %d)", versionNumber, major)
	return nil
}

func ensureRole(ctx context.Context, conn *pgx.Conn, name, password string) error {
	var exists bool
	if err := conn.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)", name).Scan(&exists); err != nil {
		return fmt.Errorf("checking role %s: %w", name, err)
	}
	switch {
	case !exists && password == "":
		return fmt.Errorf("role %s does not exist and no password was given to create it", name)
	case !exists:
		if _, err := conn.Exec(ctx, createRoleStatement(name, password)); err != nil {
			return fmt.Errorf("creating role %s: %w", name, err)
		}
		ok("role %s created", name)
	case password != "":
		if _, err := conn.Exec(ctx, alterRolePasswordStatement(name, password)); err != nil {
			return fmt.Errorf("updating role %s password: %w", name, err)
		}
		ok("role %s exists, password updated", name)
	default:
		ok("role %s exists, credentials untouched", name)
	}
	return nil
}

func (o Options) ensureDatabase(ctx context.Context, conn *pgx.Conn) error {
	var exists bool
	if err := conn.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", o.Database).Scan(&exists); err != nil {
		return fmt.Errorf("checking database %s: %w", o.Database, err)
	}
	if exists {
		ok("database %s exists", o.Database)
		return nil
	}
	if _, err := conn.Exec(ctx, createDatabaseStatement(o.Database)); err != nil {
		return fmt.Errorf("creating database %s: %w", o.Database, err)
	}
	ok("database %s created", o.Database)
	return nil
}

func (o Options) verify(ctx context.Context, asPartOfAll bool) error {
	if err := o.Validate(); err != nil {
		return err
	}
	step("Phase verify: post-writer checks")

	archive, err := o.connect(ctx, o.Database, o.SuperUser, o.SuperPassword)
	if err != nil {
		return err
	}
	defer archive.Close(ctx)

	var trendsExists bool
	if err := archive.QueryRow(ctx, "SELECT to_regclass('public.trends') IS NOT NULL").Scan(&trendsExists); err != nil {
		return fmt.Errorf("checking public.trends: %w", err)
	}
	if !trendsExists {
		message := "writer has not run yet: public.trends does not exist. " +
			"Start the Simple-Scada project against this database once, then run verify."
		if asPartOfAll {
			note("%s", message)
			return nil
		}
		return fmt.Errorf("%s", message)
	}
	ok("public.trends exists")

	var messagesExists bool
	if err := archive.QueryRow(ctx, "SELECT to_regclass('public.messages') IS NOT NULL").Scan(&messagesExists); err != nil {
		return fmt.Errorf("checking public.messages: %w", err)
	}
	if messagesExists {
		ok("public.messages exists")
	} else {
		warn("public.messages does not exist")
	}

	var canSelect bool
	if err := archive.QueryRow(ctx,
		"SELECT has_table_privilege($1, 'public.trends', 'SELECT')", ReaderRole).Scan(&canSelect); err != nil {
		return fmt.Errorf("checking reader SELECT privilege: %w", err)
	}
	if !canSelect {
		return fmt.Errorf("%s cannot SELECT on public.trends: the writer ran before default privileges "+
			"were set; repair with GRANT SELECT ON ALL TABLES IN SCHEMA public TO %s, then re-run create",
			ReaderRole, ReaderRole)
	}
	ok("%s holds SELECT on public.trends", ReaderRole)

	var canInsert bool
	if err := archive.QueryRow(ctx,
		"SELECT has_table_privilege($1, 'public.trends', 'INSERT')", ReaderRole).Scan(&canInsert); err != nil {
		return fmt.Errorf("checking reader INSERT privilege: %w", err)
	}
	if canInsert {
		return fmt.Errorf("%s holds INSERT on public.trends - the reader must be read-only", ReaderRole)
	}
	ok("%s does not hold INSERT", ReaderRole)

	if o.ReaderPassword != "" {
		reader, err := o.connect(ctx, o.Database, ReaderRole, o.ReaderPassword)
		if err != nil {
			return fmt.Errorf("live reader connection: %w", err)
		}
		defer reader.Close(ctx)
		var probed int
		if err := reader.QueryRow(ctx,
			"SELECT count(*) FROM (SELECT 1 FROM public.trends LIMIT 1) probe").Scan(&probed); err != nil {
			return fmt.Errorf("live reader probe: %w", err)
		}
		ok("live connection as %s reads public.trends", ReaderRole)
	} else {
		note("reader password not supplied - live connection as %s not tested.", ReaderRole)
	}

	var defaultPartitionExists bool
	if err := archive.QueryRow(ctx, "SELECT to_regclass('public.tpdefault') IS NOT NULL").Scan(&defaultPartitionExists); err != nil {
		return fmt.Errorf("checking public.tpdefault: %w", err)
	}
	if defaultPartitionExists {
		var strayRows int64
		if err := archive.QueryRow(ctx, "SELECT count(*) FROM public.tpdefault").Scan(&strayRows); err != nil {
			return fmt.Errorf("counting tpdefault rows: %w", err)
		}
		if strayRows > 0 {
			warn("default partition tpdefault holds %d rows - a day partition was missing at write time.", strayRows)
		} else {
			ok("default partition tpdefault is empty")
		}
	}
	return nil
}
