package provision

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type plotWriteProbe struct {
	table     string
	operation string
	statement string
}

const writeProbeName = "semibase write probe"

// SCADA ids are non-negative, so no pen and no trends key is -1
const absentKey = -1

// a membership row pairs any existing pen with the probe group, so an empty pen catalogue inserts
// nothing and still needs the privilege
func plotWriteProbes() []plotWriteProbe {
	probeGroup := fmt.Sprintf("SELECT id FROM semiplot_groups WHERE name = '%s'", writeProbeName)
	return []plotWriteProbe{
		{"semiplot_tags", "UPDATE", fmt.Sprintf("UPDATE semiplot_tags SET %s WHERE id = %d",
			selfAssignments(plotEditableTagColumns), absentKey)},
		{"semiplot_groups", "INSERT", fmt.Sprintf(
			"INSERT INTO semiplot_groups (name) VALUES ('%s')", writeProbeName)},
		{"semiplot_groups", "UPDATE", fmt.Sprintf(
			"UPDATE semiplot_groups SET name = '%s' WHERE name = '%s'", writeProbeName, writeProbeName)},
		{"semiplot_pen_groups", "INSERT", fmt.Sprintf(
			"INSERT INTO semiplot_pen_groups (pen_id, group_id) SELECT tag.id, grp.id "+
				"FROM semiplot_tags tag, semiplot_groups grp WHERE grp.name = '%s' LIMIT 1", writeProbeName)},
		{"semiplot_pen_groups", "UPDATE", fmt.Sprintf(
			"UPDATE semiplot_pen_groups SET group_id = group_id WHERE group_id IN (%s)", probeGroup)},
		{"semiplot_pen_groups", "DELETE", fmt.Sprintf(
			"DELETE FROM semiplot_pen_groups WHERE group_id IN (%s)", probeGroup)},
		{"semiplot_groups", "DELETE", fmt.Sprintf(
			"DELETE FROM semiplot_groups WHERE name = '%s'", writeProbeName)},
	}
}

func plotForbiddenTagWrites() []plotWriteProbe {
	return []plotWriteProbe{
		{"semiplot_tags", "INSERT", fmt.Sprintf(
			"INSERT INTO semiplot_tags (id, name) VALUES (%d, '%s')", absentKey, writeProbeName)},
		{"semiplot_tags", "DELETE", fmt.Sprintf("DELETE FROM semiplot_tags WHERE id = %d", absentKey)},
		{"semiplot_tags", "UPDATE (id)", "UPDATE semiplot_tags SET id = id WHERE false"},
	}
}

func selfAssignments(columns []string) string {
	assignments := make([]string, 0, len(columns))
	for _, column := range columns {
		assignments = append(assignments, column+" = "+column)
	}
	return strings.Join(assignments, ", ")
}

var forbiddenWriteOperations = []string{"INSERT", "UPDATE", "DELETE"}

const scadaMessages = "public.messages" // the SCADA's alone, so it is asserted only where it already exists

const (
	insufficientPrivilege             = "42501"
	integrityConstraintViolationClass = "23"
)

func writeProbeFailure(probe plotWriteProbe, cause error) error {
	return fmt.Errorf("%s cannot %s %s: %w. Repair with GRANT %s ON %s TO %s, then re-run this command",
		PlotRole, probe.operation, probe.table, cause, plotGrantsOn(probe.table), probe.table, PlotRole)
}

// REVOKE UPDATE on the table clears the column grants too, and the next run grants them back
func tooWideTagGrantFailure(probe plotWriteProbe) error {
	privilege, _, _ := strings.Cut(probe.operation, " ")
	return fmt.Errorf("%s may %s %s and must not: it edits a pen's settings and never adds, deletes or "+
		"re-keys a pen. The grant is too wide. %s",
		PlotRole, probe.operation, probe.table, revokeRepair(privilege, probe.table))
}

func revokeRepair(privilege, relation string) string {
	return fmt.Sprintf("Repair with REVOKE %s ON %s FROM %s, or, if the privilege comes through PUBLIC or a "+
		"role %s is a member of, revoke it there or drop the membership; then re-run this command",
		privilege, relation, PlotRole, PlotRole)
}

// a class 23 error is raised past the privilege check, so the statement was allowed to run
func refusalVerdict(probe plotWriteProbe, probeErr error) error {
	var pgErr *pgconn.PgError
	switch {
	case probeErr == nil:
		return tooWideTagGrantFailure(probe)
	case hasSQLState(probeErr, insufficientPrivilege):
		return nil
	case errors.As(probeErr, &pgErr) && strings.HasPrefix(pgErr.Code, integrityConstraintViolationClass):
		return fmt.Errorf("the %s %s probe passed the privilege check and failed on a constraint: %w. %w",
			probe.table, probe.operation, probeErr, tooWideTagGrantFailure(probe))
	default:
		return fmt.Errorf("running the %s %s probe: %w", probe.table, probe.operation, probeErr)
	}
}

func hasSQLState(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

func excessPrivilegeFailure(relation, operation string) error {
	return fmt.Errorf("%s holds %s on %s and must not write it. %s",
		PlotRole, operation, relation, revokeRepair(operation, relation))
}

func createInPublicFailure() error {
	return fmt.Errorf("%s holds CREATE on schema public and must not: it would add objects beside the "+
		"tables granted to it. This run already revoked CREATE from PUBLIC, so the privilege is held by "+
		"name or through a role %s is a member of, %s among them. Repair with REVOKE CREATE ON SCHEMA "+
		"public FROM %s, or drop the membership that carries it; then re-run this command",
		PlotRole, PlotRole, WriterRole, PlotRole)
}

// the cheapest statement that still needs SELECT on public.trends and USAGE on the schema it
// lives in, whatever the table's size
const plotProbe = "SELECT count(*) FROM (SELECT 1 FROM " + archiveTable + " LIMIT 1) probe"

// docs/architecture/provisioning.md#what-the-tail-checks-prove
func (o Options) check(ctx context.Context) error {
	if err := o.Validate(); err != nil {
		return err
	}
	step("Phase check: the %s access chain", PlotRole)

	archive, err := o.connect(ctx, o.Database, o.SuperUser, o.SuperPassword)
	if err != nil {
		return err
	}
	defer archive.Close(ctx)

	if err := assertPlotReads(ctx, archive); err != nil {
		return err
	}
	if err := assertPlotReadsTheSchemaVersion(ctx, archive); err != nil {
		return err
	}
	if err := assertPlotWriteAccess(ctx, archive); err != nil {
		return err
	}
	if err := assertPublicCannotRegisterPens(ctx, archive); err != nil {
		return err
	}
	if err := assertPlotCannotWriteTheArchive(ctx, archive); err != nil {
		return err
	}
	if err := assertPlotCannotWriteTheSchemaVersion(ctx, archive); err != nil {
		return err
	}
	if err := assertPlotCannotCreateInPublic(ctx, archive); err != nil {
		return err
	}
	if err := o.checkPlotLogin(ctx); err != nil {
		return err
	}
	// last, so a pending shared_buffers is the last thing the run prints
	return reportPendingRestart(ctx, archive)
}

// the read itself, never has_table_privilege: the catalog bit says yes for a role that
// REVOKE USAGE ON SCHEMA public has locked out of the schema the table lives in (measured)
func assertPlotReads(ctx context.Context, archive *pgx.Conn) error {
	var probeErr error
	if err := runAs(ctx, archive, PlotRole, func() error {
		var probed int
		probeErr = archive.QueryRow(ctx, plotProbe).Scan(&probed)
		return nil
	}); err != nil {
		return err
	}
	if probeErr != nil {
		return diagnoseFailedRead(ctx, archive, probeErr)
	}
	ok("%s reads public.trends", PlotRole)
	return nil
}

func assertPlotReadsTheSchemaVersion(ctx context.Context, archive *pgx.Conn) error {
	var version int
	var probeErr error
	if err := runAs(ctx, archive, PlotRole, func() error {
		probeErr = archive.QueryRow(ctx, "SELECT schema_version FROM semiplot_meta").Scan(&version)
		return nil
	}); err != nil {
		return err
	}
	if probeErr != nil {
		return fmt.Errorf("%s cannot read the schema version from semiplot_meta: %w. Repair with "+
			"GRANT %s ON semiplot_meta TO %s, then re-run this command",
			PlotRole, probeErr, plotReadGrants, PlotRole)
	}
	ok("%s reads semiplot_meta.schema_version = %d", PlotRole, version)
	return nil
}

func diagnoseFailedRead(ctx context.Context, archive *pgx.Conn, probeErr error) error {
	var granted bool
	if err := archive.QueryRow(ctx,
		"SELECT has_table_privilege($1, $2, 'SELECT')", PlotRole, archiveTable).Scan(&granted); err != nil {
		return fmt.Errorf("%s cannot read public.trends (%w) and its SELECT privilege could not be read: %w",
			PlotRole, probeErr, err)
	}
	if !granted {
		return fmt.Errorf("%s cannot read public.trends: it holds no SELECT on the table, so the table was "+
			"created before the default privileges were set (%w). Repair with GRANT SELECT ON ALL TABLES IN "+
			"SCHEMA public TO %s for the tables that exist, then %s for the partitions still to come, then "+
			"re-run this command", PlotRole, probeErr, PlotRole, defaultPrivilegesStatement())
	}
	return fmt.Errorf("%s holds SELECT on public.trends and still cannot read it: %w. The usual cause is a "+
		"revoked schema privilege; repair with GRANT USAGE ON SCHEMA public TO %s, then re-run this command",
		PlotRole, probeErr, PlotRole)
}

func assertPlotWriteAccess(ctx context.Context, archive *pgx.Conn) error {
	return runAs(ctx, archive, PlotRole, func() error {
		transaction, err := archive.Begin(ctx)
		if err != nil {
			return fmt.Errorf("opening the %s write probe transaction: %w", PlotRole, err)
		}
		defer func() { _ = transaction.Rollback(ctx) }()

		if err := assertPlotRegistersNewPens(ctx, transaction); err != nil {
			return err
		}
		if err := assertTemporaryTrendsAreIgnored(ctx, transaction); err != nil {
			return err
		}
		if err := assertPlotWritesItsTables(ctx, transaction); err != nil {
			return err
		}
		return assertPlotCannotEditPenKeys(ctx, transaction)
	})
}

const registerNewPensProbe = "SELECT " + registerNewPensFunction

func assertPlotRegistersNewPens(ctx context.Context, transaction pgx.Tx) error {
	var unregistered int
	if err := transaction.QueryRow(ctx, registerNewPensProbe).Scan(&unregistered); err != nil {
		return fmt.Errorf("calling %s as %s: %w", registerNewPensFunction, PlotRole, err)
	}
	ok("%s runs %s: %d keys in %s have no pen yet", PlotRole, registerNewPensFunction, unregistered, archiveTable)
	return nil
}

// docs/architecture/provisioning.md#registering-new-pens
func assertTemporaryTrendsAreIgnored(ctx context.Context, transaction pgx.Tx) error {
	shadow := []string{
		"CREATE TEMPORARY TABLE trends (id integer) ON COMMIT DROP",
		"GRANT SELECT ON pg_temp.trends TO " + RegistrarRole,
		fmt.Sprintf("INSERT INTO trends (id) VALUES (%d)", absentKey),
		registerNewPensProbe,
	}
	for _, statement := range shadow {
		if _, err := transaction.Exec(ctx, statement); err != nil {
			return fmt.Errorf("replaying the temporary-trends attack as %s (%s): %w", PlotRole, statement, err)
		}
	}
	var registered bool
	if err := transaction.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM public.semiplot_tags WHERE id = $1)",
		absentKey).Scan(&registered); err != nil {
		return fmt.Errorf("reading back the shadow key: %w", err)
	}
	if registered {
		return fmt.Errorf("%s registered pen %d from a temporary trends %s created: a caller can add any "+
			"pen it names. This is a build defect in sql/semiplot_register.sql, whose body must name %s "+
			"by schema; no grant repairs it", registerNewPensFunction, absentKey, PlotRole, archiveTable)
	}
	ok("%s reads %s, not a temporary trends of its caller", registerNewPensFunction, archiveTable)
	return nil
}

func assertPlotWritesItsTables(ctx context.Context, transaction pgx.Tx) error {
	for _, probe := range plotWriteProbes() {
		if _, err := transaction.Exec(ctx, probe.statement); err != nil {
			return writeProbeFailure(probe, err)
		}
	}
	ok("%s inserts, updates and deletes semiplot_groups and semiplot_pen_groups, and updates the "+
		"settings columns of semiplot_tags", PlotRole)
	return nil
}

func assertPlotCannotEditPenKeys(ctx context.Context, transaction pgx.Tx) error {
	for _, probe := range plotForbiddenTagWrites() {
		if err := assertRefused(ctx, transaction, probe); err != nil {
			return err
		}
	}
	ok("%s cannot insert, delete or re-key a pen in semiplot_tags", PlotRole)
	return nil
}

// semiplot's own call succeeds either way, so this one is a catalog question
func assertPublicCannotRegisterPens(ctx context.Context, archive *pgx.Conn) error {
	var granted bool
	if err := archive.QueryRow(ctx, "SELECT has_function_privilege('public', $1, 'EXECUTE')",
		registerNewPensFunction).Scan(&granted); err != nil {
		return fmt.Errorf("checking the PUBLIC EXECUTE privilege on %s: %w", registerNewPensFunction, err)
	}
	if granted {
		return publicRegistersPensFailure()
	}
	ok("PUBLIC holds no EXECUTE on %s", registerNewPensFunction)
	return nil
}

func publicRegistersPensFailure() error {
	return fmt.Errorf("PUBLIC holds EXECUTE on %s and must not: every role that connects could add pens. "+
		"Repair with REVOKE ALL ON FUNCTION %s FROM PUBLIC, then re-run this command",
		registerNewPensFunction, registerNewPensFunction)
}

// a savepoint per statement, because a refused statement aborts the transaction around it
func assertRefused(ctx context.Context, transaction pgx.Tx, probe plotWriteProbe) error {
	savepoint, err := transaction.Begin(ctx)
	if err != nil {
		return fmt.Errorf("opening a savepoint for the %s %s probe: %w", probe.table, probe.operation, err)
	}
	defer func() { _ = savepoint.Rollback(ctx) }()

	_, probeErr := savepoint.Exec(ctx, probe.statement)
	return refusalVerdict(probe, probeErr)
}

func assertPlotCannotWriteTheArchive(ctx context.Context, archive *pgx.Conn) error {
	if err := assertNoWritePrivilege(ctx, archive, archiveTable); err != nil {
		return err
	}
	asserted := []string{archiveTable}

	messages, err := relationExists(ctx, archive, scadaMessages)
	if err != nil {
		return err
	}
	if messages {
		if err := assertNoWritePrivilege(ctx, archive, scadaMessages); err != nil {
			return err
		}
		asserted = append(asserted, scadaMessages)
	}
	ok("%s holds no INSERT, UPDATE or DELETE on %s", PlotRole, strings.Join(asserted, ", "))
	return nil
}

func assertPlotCannotCreateInPublic(ctx context.Context, archive *pgx.Conn) error {
	var granted bool
	if err := archive.QueryRow(ctx,
		"SELECT has_schema_privilege($1, 'public', 'CREATE')", PlotRole).Scan(&granted); err != nil {
		return fmt.Errorf("checking the %s CREATE privilege on schema public: %w", PlotRole, err)
	}
	if granted {
		return createInPublicFailure()
	}
	ok("%s holds no CREATE on schema public", PlotRole)
	return nil
}

func assertPlotCannotWriteTheSchemaVersion(ctx context.Context, archive *pgx.Conn) error {
	if err := assertNoWritePrivilege(ctx, archive, "semiplot_meta"); err != nil {
		return err
	}
	ok("%s holds no INSERT, UPDATE or DELETE on semiplot_meta", PlotRole)
	return nil
}

func assertNoWritePrivilege(ctx context.Context, archive *pgx.Conn, relation string) error {
	for _, operation := range forbiddenWriteOperations {
		var granted bool
		if err := archive.QueryRow(ctx, "SELECT has_table_privilege($1, $2, $3)",
			PlotRole, relation, operation).Scan(&granted); err != nil {
			return fmt.Errorf("checking the %s %s privilege on %s: %w", PlotRole, operation, relation, err)
		}
		if granted {
			return excessPrivilegeFailure(relation, operation)
		}
	}
	return nil
}

func (o Options) checkPlotLogin(ctx context.Context) error {
	switch {
	case o.PlotPassword == "":
		note("no %s password given - the login was not tested, only the privileges behind it.", PlotRole)
		return nil
	case isSocketHost(o.Host):
		note("socket host - the %s login was not tested; a consumer connects over TCP.", PlotRole)
		return nil
	}

	plot, err := o.connect(ctx, o.Database, PlotRole, o.PlotPassword)
	if err != nil {
		return err
	}
	defer plot.Close(ctx)

	var probed int
	if err := plot.QueryRow(ctx, plotProbe).Scan(&probed); err != nil {
		return fmt.Errorf("%s reads public.trends over its own login: %w", PlotRole, err)
	}
	ok("%s logs in and reads public.trends", PlotRole)
	return nil
}

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
