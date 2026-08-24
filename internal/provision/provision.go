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

// Site brings an installation machine to its commissioned state: memory tuning, then
// everything the archive needs.
func (o Options) Site(ctx context.Context) error {
	if err := o.config(ctx); err != nil {
		return err
	}
	if err := o.create(ctx); err != nil {
		return err
	}
	return o.check(ctx)
}

// Bench brings a throwaway container to the same state minus the tuning: its memory
// constants are sized for an installation machine and mean nothing to a container.
func (o Options) Bench(ctx context.Context) error {
	if err := o.create(ctx); err != nil {
		return err
	}
	return o.check(ctx)
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

func defaultPrivilegesStatement() string {
	return fmt.Sprintf("ALTER DEFAULT PRIVILEGES FOR ROLE %s IN SCHEMA public GRANT SELECT ON TABLES TO %s",
		WriterRole, ReaderRole)
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
	defaultPrivileges := defaultPrivilegesStatement()
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

	return ensureArchiveTable(ctx, archive)
}

func relationExists(ctx context.Context, conn *pgx.Conn, name string) (bool, error) {
	var exists bool
	if err := conn.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", name).Scan(&exists); err != nil {
		return false, fmt.Errorf("checking %s: %w", name, err)
	}
	return exists, nil
}

// runAs assumes role for the statements fn issues on conn, and drops back afterwards. The
// superuser connection is short-lived, but a leaked role would silently change what every
// later statement on it is allowed to do.
func runAs(ctx context.Context, conn *pgx.Conn, role string, fn func() error) error {
	if _, err := conn.Exec(ctx, "SET ROLE "+role); err != nil {
		return fmt.Errorf("assuming the %s role: %w", role, err)
	}
	err := fn()
	if _, resetErr := conn.Exec(ctx, "RESET ROLE"); resetErr != nil && err == nil {
		return fmt.Errorf("dropping the %s role: %w", role, resetErr)
	}
	return err
}

func ensureArchiveTable(ctx context.Context, archive *pgx.Conn) error {
	exists, err := relationExists(ctx, archive, "public.trends")
	if err != nil {
		return err
	}
	if exists {
		return inspectArchiveTable(ctx, archive)
	}
	return createArchiveTable(ctx, archive)
}

// public.trends is created as scada_writer, so the table's owner is the role the SCADA writes
// with and the reader's SELECT arrives through the default privileges set for that role a
// moment earlier. A superuser-owned table would hand the reader access for a different reason,
// and a bench would then test something production does not do.
//
// The role is assumed with SET ROLE on the superuser connection rather than through a
// scada_writer login. Both produce the same owner and the same relacl - measured, not assumed -
// but a login also has to be admitted by pg_hba.conf, and "local all all peer", the default on
// Debian, Ubuntu and RHEL, refuses it on a unix socket.
func createArchiveTable(ctx context.Context, archive *pgx.Conn) error {
	err := runAs(ctx, archive, WriterRole, func() error {
		if _, err := archive.Exec(ctx, semibase.TrendsSQL); err != nil {
			return fmt.Errorf("applying trends.sql as %s: %w", WriterRole, err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	ok("public.trends created as %s, partitioned by t, with the tpdefault partition", WriterRole)
	return nil
}

// an existing table is never altered - it may be one the SCADA has since changed - but the run
// still promises the state this tool describes, so what it promises is read back.
func inspectArchiveTable(ctx context.Context, archive *pgx.Conn) error {
	ok("public.trends exists, left untouched")

	defaultPartition, err := relationExists(ctx, archive, "public.tpdefault")
	if err != nil {
		return err
	}
	if !defaultPartition {
		return fmt.Errorf("public.trends exists without its default partition public.tpdefault: a row "+
			"whose timestamp no day partition covers is rejected at write time. Repair with "+
			"CREATE TABLE public.tpdefault PARTITION OF public.trends DEFAULT, issued as %s", WriterRole)
	}

	// on the create path this is always zero; here the table can be months old, and a non-empty
	// tpdefault is the only signal that a day partition was missing at write time
	var strayRows int64
	if err := archive.QueryRow(ctx, "SELECT count(*) FROM public.tpdefault").Scan(&strayRows); err != nil {
		return fmt.Errorf("counting public.tpdefault rows: %w", err)
	}
	if strayRows > 0 {
		warn("default partition tpdefault holds %d rows - a day partition was missing at write time.", strayRows)
		return nil
	}
	ok("default partition tpdefault is present and empty")
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

// the cheapest statement that still needs SELECT on public.trends and USAGE on the schema it
// lives in, whatever the table's size
const readerProbe = "SELECT count(*) FROM (SELECT 1 FROM public.trends LIMIT 1) probe"

// the tail of every run. The tool created public.trends itself a moment ago, so the reader's
// access to it is knowable at exit instead of after the SCADA's first start.
func (o Options) check(ctx context.Context) error {
	if err := o.Validate(); err != nil {
		return err
	}
	step("Phase check: the reader access chain")

	archive, err := o.connect(ctx, o.Database, o.SuperUser, o.SuperPassword)
	if err != nil {
		return err
	}
	defer archive.Close(ctx)

	if err := assertReaderReads(ctx, archive); err != nil {
		return err
	}
	if err := assertReaderCannotWrite(ctx, archive); err != nil {
		return err
	}
	if err := o.checkReaderLogin(ctx); err != nil {
		return err
	}
	// last, so a pending shared_buffers is the last thing the run prints
	return reportPendingRestart(ctx, archive)
}

// the load-bearing check: the read itself, issued as the reader. has_table_privilege answers
// from one catalog bit and stops there - it says yes for a reader that REVOKE USAGE ON SCHEMA
// public has locked out of the schema the table lives in, a state this tool has to fail on.
// SET ROLE rather than a login, for the pg_hba reason createArchiveTable states; the login is
// checked separately, where it can be.
func assertReaderReads(ctx context.Context, archive *pgx.Conn) error {
	var probeErr error
	if err := runAs(ctx, archive, ReaderRole, func() error {
		var probed int
		probeErr = archive.QueryRow(ctx, readerProbe).Scan(&probed)
		return nil
	}); err != nil {
		return err
	}
	if probeErr != nil {
		return diagnoseFailedRead(ctx, archive, probeErr)
	}
	ok("%s reads public.trends", ReaderRole)
	return nil
}

// the catalog bit earns its place here and only here: it separates a missing table grant, whose
// repair is the default-privileges chain, from a schema the reader cannot enter
func diagnoseFailedRead(ctx context.Context, archive *pgx.Conn, probeErr error) error {
	var granted bool
	if err := archive.QueryRow(ctx,
		"SELECT has_table_privilege($1, 'public.trends', 'SELECT')", ReaderRole).Scan(&granted); err != nil {
		return fmt.Errorf("%s cannot read public.trends (%w) and its SELECT privilege could not be read: %w",
			ReaderRole, probeErr, err)
	}
	if !granted {
		return fmt.Errorf("%s cannot read public.trends: it holds no SELECT on the table, so the table was "+
			"created before the default privileges were set (%w). Repair with GRANT SELECT ON ALL TABLES IN "+
			"SCHEMA public TO %s for the tables that exist, then %s for the partitions still to come, then "+
			"re-run this command", ReaderRole, probeErr, ReaderRole, defaultPrivilegesStatement())
	}
	return fmt.Errorf("%s holds SELECT on public.trends and still cannot read it: %w. The usual cause is a "+
		"revoked schema privilege; repair with GRANT USAGE ON SCHEMA public TO %s, then re-run this command",
		ReaderRole, probeErr, ReaderRole)
}

// the reader is read-only. No probe can prove the absence of a privilege the way a read proves
// its presence, so this one stays a catalog question.
func assertReaderCannotWrite(ctx context.Context, archive *pgx.Conn) error {
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

// what SET ROLE cannot reach: pg_hba admitting the reader, and the password a consumer will
// carry. It needs that password, and it is skipped on a socket host - "local all all peer" is
// the platform default there, no consumer reads over the socket, and the socket path exists so
// this tool can run as an init script inside a postgres image.
func (o Options) checkReaderLogin(ctx context.Context) error {
	switch {
	case o.ReaderPassword == "":
		note("no %s password given - the login was not tested, only the privileges behind it.", ReaderRole)
		return nil
	case isSocketHost(o.Host):
		note("socket host - the %s login was not tested; a consumer connects over TCP.", ReaderRole)
		return nil
	}

	reader, err := o.connect(ctx, o.Database, ReaderRole, o.ReaderPassword)
	if err != nil {
		return err
	}
	defer reader.Close(ctx)

	var probed int
	if err := reader.QueryRow(ctx, readerProbe).Scan(&probed); err != nil {
		return fmt.Errorf("%s reads public.trends over its own login: %w", ReaderRole, err)
	}
	ok("%s logs in and reads public.trends", ReaderRole)
	return nil
}

// both commands report it: bench tunes nothing, but it can run against a server a previous
// site run tuned and nobody restarted
func reportPendingRestart(ctx context.Context, conn *pgx.Conn) error {
	pending, err := pendingRestartSettings(ctx, conn)
	if err != nil {
		return err
	}
	if len(pending) > 0 {
		warn("settings waiting for a service restart: %s - restart the PostgreSQL service or reboot the machine.",
			strings.Join(pending, ", "))
	}
	return nil
}
