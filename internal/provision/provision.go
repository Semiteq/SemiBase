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
)

const (
	WriterRole = "scada_writer"
	PlotRole   = "semiplot"
	// docs/architecture/provisioning.md#registering-new-pens
	RegistrarRole = "semiplot_registrar"
)

const versionFloor = 140000

const archiveTable = "public.trends"

type Options struct {
	Host           string
	Port           int
	Database       string
	SuperUser      string
	SuperPassword  string
	WriterPassword string
	PlotPassword   string
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

// docs/architecture/provisioning.md#the-connection-target
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

// docs/architecture/provisioning.md#the-connection-target
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
	// both are USERSET, so no privilege is needed. escapeLiteral relies on the first, and the
	// second pins where an unqualified semiplot_* name resolves.
	for _, setting := range []string{"SET standard_conforming_strings = on", "SET search_path = public"} {
		if _, err := conn.Exec(ctx, setting); err != nil {
			_ = conn.Close(ctx)
			return nil, fmt.Errorf("%s: %w", setting, err)
		}
	}
	return conn, nil
}

// quote doubling alone is safe only because connect sets standard_conforming_strings = on,
// which strips backslashes of any escape meaning inside '...' literals
func escapeLiteral(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

func relationExists(ctx context.Context, conn *pgx.Conn, name string) (bool, error) {
	var exists bool
	if err := conn.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", name).Scan(&exists); err != nil {
		return false, fmt.Errorf("checking %s: %w", name, err)
	}
	return exists, nil
}

// a leaked role would silently change what every later statement on the connection is allowed to do
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
