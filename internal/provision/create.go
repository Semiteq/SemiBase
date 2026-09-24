package provision

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	semibase "github.com/Semiteq/SemiBase"
)

// only the database name and the password reach DDL text here; role names are constants
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
		WriterRole, PlotRole)
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
		{PlotRole, o.PlotPassword},
	}
	for _, role := range roles {
		if err := ensureRole(ctx, super, role.name, role.password); err != nil {
			return err
		}
	}
	if err := ensureRegistrarRole(ctx, super); err != nil {
		return err
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

	plotSettings := []string{
		fmt.Sprintf("ALTER ROLE %s SET statement_timeout = '30s'", PlotRole),
		fmt.Sprintf("ALTER ROLE %s SET idle_in_transaction_session_timeout = '60s'", PlotRole),
	}
	for _, statement := range plotSettings {
		if _, err := super.Exec(ctx, statement); err != nil {
			return fmt.Errorf("%s: %w", statement, err)
		}
	}
	ok("%s timeouts set (statement 30s, idle-in-transaction 60s)", PlotRole)

	archive, err := o.connect(ctx, o.Database, o.SuperUser, o.SuperPassword)
	if err != nil {
		return err
	}
	defer archive.Close(ctx)

	if err := revokePublicCreate(ctx, archive); err != nil {
		return err
	}
	if _, err := archive.Exec(ctx, "GRANT CREATE ON SCHEMA public TO "+WriterRole); err != nil {
		return fmt.Errorf("granting CREATE on schema public: %w", err)
	}

	// the writer creates public.trends below and its partitions later, so SELECT on them
	// can only be granted ahead of time, through default privileges of their owner
	defaultPrivileges := defaultPrivilegesStatement()
	if _, err := archive.Exec(ctx, defaultPrivileges); err != nil {
		return fmt.Errorf("%s: %w", defaultPrivileges, err)
	}
	ok("default privileges set - tables %s creates are readable by %s", WriterRole, PlotRole)

	if err := ensureArchiveTable(ctx, archive); err != nil {
		return err
	}

	for _, file := range semiplotSchemaFiles {
		if _, err := archive.Exec(ctx, file.sql); err != nil {
			return fmt.Errorf("applying %s: %w", file.name, err)
		}
	}
	for _, statement := range append(registrarStatements(), semiplotGrantStatements()...) {
		if _, err := archive.Exec(ctx, statement); err != nil {
			return fmt.Errorf("%s: %w", statement, err)
		}
	}
	ok("SemiPlot configuration tables in place: %s", strings.Join(semiplotTableNames(), ", "))
	ok("%s in place, owned by %s, executable by %s and not by PUBLIC",
		registerNewPensFunction, RegistrarRole, PlotRole)
	return nil
}

// docs/architecture/provisioning.md#the-roles
func revokePublicCreate(ctx context.Context, archive *pgx.Conn) error {
	if _, err := archive.Exec(ctx, "REVOKE CREATE ON SCHEMA public FROM PUBLIC"); err != nil {
		return fmt.Errorf("revoking CREATE on schema public from PUBLIC: %w", err)
	}
	ok("PUBLIC holds no CREATE on schema public")
	return nil
}

func ensureArchiveTable(ctx context.Context, archive *pgx.Conn) error {
	exists, err := relationExists(ctx, archive, archiveTable)
	if err != nil {
		return err
	}
	if exists {
		return inspectArchiveTable(ctx, archive)
	}
	return createArchiveTable(ctx, archive)
}

// docs/architecture/provisioning.md#the-archive-table
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

func roleExists(ctx context.Context, conn *pgx.Conn, name string) (bool, error) {
	var exists bool
	if err := conn.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)", name).Scan(&exists); err != nil {
		return false, fmt.Errorf("checking role %s: %w", name, err)
	}
	return exists, nil
}

func ensureRole(ctx context.Context, conn *pgx.Conn, name, password string) error {
	exists, err := roleExists(ctx, conn, name)
	if err != nil {
		return err
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

// docs/architecture/provisioning.md#registering-new-pens
func ensureRegistrarRole(ctx context.Context, conn *pgx.Conn) error {
	exists, err := roleExists(ctx, conn, RegistrarRole)
	if err != nil {
		return err
	}
	if exists {
		ok("role %s exists", RegistrarRole)
		return nil
	}
	if _, err := conn.Exec(ctx, "CREATE ROLE "+RegistrarRole+" NOLOGIN"); err != nil {
		return fmt.Errorf("creating role %s: %w", RegistrarRole, err)
	}
	ok("role %s created, without a login", RegistrarRole)
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
