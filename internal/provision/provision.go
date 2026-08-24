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

const (
	WriterRole = "scada_writer"
	ReaderRole = "semiplot_reader"
)

const versionFloor = 140000

type Options struct {
	Host           string
	Port           int
	Database       string
	SuperUser      string
	SuperPassword  string
	WriterPassword string
	ReaderPassword string
	ExpectedMajor  int
}

var databaseNamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

func (o Options) Validate() error {
	if !databaseNamePattern.MatchString(o.Database) {
		return fmt.Errorf("database name %q must match %s", o.Database, databaseNamePattern)
	}
	return nil
}

// the tuning phase is the only thing that can leave a setting waiting for a service
// restart, so only the site run reads pending_restart back at the end
const (
	afterTuning   = true
	withoutTuning = false
)

// Site brings an installation machine to its commissioned state: memory tuning, then
// everything the archive needs.
func (o Options) Site(ctx context.Context) error {
	if err := o.config(ctx); err != nil {
		return err
	}
	if err := o.create(ctx); err != nil {
		return err
	}
	return o.check(ctx, afterTuning)
}

// Bench brings a throwaway container to the same state minus the tuning: its memory
// constants are sized for an installation machine and mean nothing to a container.
func (o Options) Bench(ctx context.Context) error {
	if err := o.create(ctx); err != nil {
		return err
	}
	return o.check(ctx, withoutTuning)
}

// pgx dials a unix socket exactly when pgconn.isAbsolutePath accepts the host. Widening this
// past pgx would route a host into the query parameter that the driver then resolves as a TCP
// name: libpq's extra "@" form for Linux's abstract namespace is that case, so it stays out.
func isSocketHost(host string) bool {
	return strings.HasPrefix(host, "/") || isWindowsDrivePath(host)
}

// pgconn.isAbsolutePath's drive-letter clause, character for character
func isWindowsDrivePath(host string) bool {
	return len(host) >= 3 && host[0] >= 'A' && host[0] <= 'Z' && host[1] == ':' && host[2] == '\\'
}

// a socket directory cannot ride in the URL authority: its slashes would end the authority
// and the rest of the path would be read as the URL path. libpq and pgx take the socket
// directory from the host query parameter instead, percent-encoded like any query value
func (o Options) connectionString(database, user, password string) string {
	target := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Path:   "/" + database,
	}
	if isSocketHost(o.Host) {
		target.RawQuery = url.Values{
			"host": {o.Host},
			"port": {strconv.Itoa(o.Port)},
		}.Encode()
		return target.String()
	}
	target.Host = net.JoinHostPort(o.Host, strconv.Itoa(o.Port))
	return target.String()
}

// Endpoint names what a connection to the archive database targets, for messages.
func (o Options) Endpoint() string {
	return o.endpoint(o.Database)
}

// the socket file is <directory>/.s.PGSQL.<port>, so the database cannot follow it behind a
// slash: that would name a path under the socket file and send a reader of the message
// looking for a directory that can never exist
func (o Options) endpoint(database string) string {
	if isSocketHost(o.Host) {
		return fmt.Sprintf("%s/.s.PGSQL.%d (%s)", strings.TrimRight(o.Host, `/\`), o.Port, database)
	}
	return fmt.Sprintf("%s:%d/%s", o.Host, o.Port, database)
}

func (o Options) connect(ctx context.Context, database, user, password string) (*pgx.Conn, error) {
	conn, err := pgx.Connect(ctx, o.connectionString(database, user, password))
	if err != nil {
		return nil, fmt.Errorf("connecting to %s as %s: %w", o.endpoint(database), user, err)
	}
	// escapeLiteral relies on this; USERSET, so no privilege is needed
	if _, err := conn.Exec(ctx, "SET standard_conforming_strings = on"); err != nil {
		_ = conn.Close(ctx)
		return nil, fmt.Errorf("setting standard_conforming_strings: %w", err)
	}
	return conn, nil
}

// quote doubling alone is safe only because connect sets standard_conforming_strings = on,
// which strips backslashes of any escape meaning inside '...' literals
func escapeLiteral(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

// only the database name and the password reach DDL text here; role names are
// package constants, never user input
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

func (o Options) create(ctx context.Context) error {
	if err := o.Validate(); err != nil {
		return err
	}
	step("Phase create: database %s, roles, access chain, archive table", o.Database)

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

	// PostgreSQL 15+ no longer grants CREATE on schema public to everyone; on 14 this is redundant
	if _, err := archive.Exec(ctx, "GRANT CREATE ON SCHEMA public TO "+WriterRole); err != nil {
		return fmt.Errorf("granting CREATE on schema public: %w", err)
	}

	// the writer creates public.trends below and its partitions later, so SELECT on them
	// can only be granted ahead of time, through default privileges of their owner
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
	if _, err := archive.Exec(ctx, "GRANT SELECT ON semiplot_tags TO "+ReaderRole); err != nil {
		return fmt.Errorf("granting SELECT on semiplot_tags: %w", err)
	}
	ok("semiplot_tags in place, readable by %s", ReaderRole)

	return o.ensureArchiveTable(ctx, archive)
}

// public.trends is created over a scada_writer login of its own, never with SET ROLE: the
// reader's SELECT has to arrive through the default privileges set for that role above, which
// is how a site gets it. A superuser-owned table would hand the reader access for a different
// reason, and a bench would then test something production does not do.
func (o Options) ensureArchiveTable(ctx context.Context, archive *pgx.Conn) error {
	var exists bool
	if err := archive.QueryRow(ctx,
		"SELECT to_regclass('public.trends') IS NOT NULL").Scan(&exists); err != nil {
		return fmt.Errorf("checking public.trends: %w", err)
	}
	if exists {
		ok("public.trends exists, left untouched")
		return nil
	}
	if o.WriterPassword == "" {
		return fmt.Errorf("public.trends does not exist and no %s password was given to create it "+
			"as that role; supply --writer-password or SEMIBASE_WRITER_PASSWORD", WriterRole)
	}

	writer, err := o.connect(ctx, o.Database, WriterRole, o.WriterPassword)
	if err != nil {
		return err
	}
	defer writer.Close(ctx)

	if _, err := writer.Exec(ctx, semibase.TrendsSQL); err != nil {
		return fmt.Errorf("applying trends.sql as %s: %w", WriterRole, err)
	}
	ok("public.trends created by %s, partitioned by t, with the tpdefault partition", WriterRole)
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

// the tail of every run. The tool created public.trends itself a moment ago, so the reader's
// access to it is knowable at exit instead of after the SCADA's first start.
func (o Options) check(ctx context.Context, reportPendingRestart bool) error {
	if err := o.Validate(); err != nil {
		return err
	}
	step("Phase check: the reader access chain")

	archive, err := o.connect(ctx, o.Database, o.SuperUser, o.SuperPassword)
	if err != nil {
		return err
	}
	defer archive.Close(ctx)

	if reportPendingRestart {
		pending, err := pendingRestartSettings(ctx, archive)
		if err != nil {
			return err
		}
		if len(pending) > 0 {
			warn("settings waiting for a service restart: %s - restart the PostgreSQL service or reboot the machine.",
				strings.Join(pending, ", "))
		}
	}

	var canSelect bool
	if err := archive.QueryRow(ctx,
		"SELECT has_table_privilege($1, 'public.trends', 'SELECT')", ReaderRole).Scan(&canSelect); err != nil {
		return fmt.Errorf("checking reader SELECT privilege: %w", err)
	}
	if !canSelect {
		return fmt.Errorf("%s cannot SELECT on public.trends: the table was created before the default "+
			"privileges were set; repair with GRANT SELECT ON ALL TABLES IN SCHEMA public TO %s",
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
	return nil
}
