package provision

import (
	"slices"
	"strings"
	"testing"

	semibase "github.com/Semiteq/SemiBase"
)

func sqlStatementsOf(sql string) string {
	kept := make([]string, 0, strings.Count(sql, "\n"))
	for line := range strings.SplitSeq(sql, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(strings.Fields(strings.Join(kept, " ")), " ")
}

// columnNamesOf returns the column names of a CREATE TABLE body in declaration order,
// leaving out the table constraints.
func columnNamesOf(sql string) []string {
	body := sqlStatementsOf(sql)
	open := strings.Index(body, "(")
	if open < 0 {
		return nil
	}
	body = body[open+1:]
	names := make([]string, 0, strings.Count(body, ",")+1)
	depth, start := 0, 0
	for i, character := range body {
		switch character {
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return appendColumnName(names, body[start:i])
			}
			depth--
		case ',':
			if depth == 0 {
				names = appendColumnName(names, body[start:i])
				start = i + 1
			}
		}
	}
	return names
}

func appendColumnName(names []string, definition string) []string {
	fields := strings.Fields(definition)
	if len(fields) == 0 || strings.EqualFold(fields[0], "CONSTRAINT") {
		return names
	}
	return append(names, fields[0])
}

// the viewer names these columns in its own SELECT, so the name set is the contract; the order is
// pinned too, to make an inserted or dropped column fail here rather than pass unnoticed. neither
// this package nor a test in SemiPlot sees both halves, so the want list is spelled out here
func TestSemiplotTagsColumns(t *testing.T) {
	want := []string{
		"id",
		"name",
		"unit",
		"format",
		"color",
		"line_style",
		"enabled_on_start",
		"scale_min",
		"scale_max",
	}
	got := columnNamesOf(semibase.SemiplotTagsSQL)
	if len(got) != len(want) {
		t.Fatalf("semiplot_tags.sql declares %d columns %v, want %d %v", len(got), got, len(want), want)
	}
	for i, name := range want {
		t.Run(name, func(t *testing.T) {
			if got[i] != name {
				t.Errorf("column %d is %q, want %q", i, got[i], name)
			}
		})
	}
}

// serial would make the group id a sequence the semiplot role needs USAGE on to insert a group,
// which no grant this package issues carries
func TestSemiplotGroupsUseAnIdentityColumn(t *testing.T) {
	if strings.Contains(sqlStatementsOf(semibase.SemiplotGroupsSQL), "serial") {
		t.Error("semiplot_groups.sql uses serial; the identity column needs no sequence grant")
	}
}

func TestMembershipForeignKeysCascade(t *testing.T) {
	statements := sqlStatementsOf(semibase.SemiplotGroupsSQL)
	for _, reference := range []string{
		"REFERENCES semiplot_tags (id) ON DELETE CASCADE",
		"REFERENCES semiplot_groups (id) ON DELETE CASCADE",
	} {
		t.Run(reference, func(t *testing.T) {
			if !strings.Contains(statements, reference) {
				t.Errorf("semiplot_groups.sql does not declare %s", reference)
			}
		})
	}
	if keys, cascades := strings.Count(statements, "REFERENCES "),
		strings.Count(statements, "ON DELETE CASCADE"); keys != cascades {
		t.Errorf("%d foreign keys against %d ON DELETE CASCADE clauses", keys, cascades)
	}
}

func TestSemiplotSchemaFileOrder(t *testing.T) {
	want := []string{"semiplot_tags.sql", "semiplot_register.sql", "semiplot_groups.sql", "semiplot_meta.sql"}
	if len(semiplotSchemaFiles) != len(want) {
		t.Fatalf("semiplotSchemaFiles holds %d files, want %d", len(semiplotSchemaFiles), len(want))
	}
	for i, file := range semiplotSchemaFiles {
		if file.name != want[i] {
			t.Errorf("semiplotSchemaFiles[%d] = %q, want %q", i, file.name, want[i])
		}
	}
}

// semiplotTables is what the run reports and what the grants will name, so it has to
// stay the set the embedded files actually create
func TestSemiplotTablesMatchTheEmbeddedFiles(t *testing.T) {
	var builder strings.Builder
	for _, file := range semiplotSchemaFiles {
		builder.WriteString(sqlStatementsOf(file.sql))
		builder.WriteString(" ")
	}
	created := builder.String()
	for _, table := range semiplotTables {
		t.Run(table.name, func(t *testing.T) {
			if !strings.Contains(created, "CREATE TABLE IF NOT EXISTS "+table.name+" (") {
				t.Errorf("no embedded file creates %s", table.name)
			}
		})
	}
	if got, want := strings.Count(created, "CREATE TABLE"), len(semiplotTables); got != want {
		t.Errorf("the embedded files create %d tables, semiplotTables names %d", got, want)
	}
}

func TestSemiplotGrantStatements(t *testing.T) {
	want := []struct {
		object    string
		statement string
	}{
		{"semiplot_tags", "GRANT SELECT, UPDATE (name, unit, format, color, line_style, enabled_on_start, " +
			"scale_min, scale_max) ON semiplot_tags TO semiplot"},
		{"semiplot_groups", "GRANT SELECT, INSERT, UPDATE, DELETE ON semiplot_groups TO semiplot"},
		{"semiplot_pen_groups", "GRANT SELECT, INSERT, UPDATE, DELETE ON semiplot_pen_groups TO semiplot"},
		{"semiplot_meta", "GRANT SELECT ON semiplot_meta TO semiplot"},
		{"semiplot_register_new_pens", "REVOKE ALL ON FUNCTION semiplot_register_new_pens() FROM PUBLIC; " +
			"GRANT EXECUTE ON FUNCTION semiplot_register_new_pens() TO semiplot"},
	}
	got := semiplotGrantStatements()
	if len(got) != len(want) {
		t.Fatalf("semiplotGrantStatements() returned %d statements, want %d", len(got), len(want))
	}
	for i, expected := range want {
		t.Run(expected.object, func(t *testing.T) {
			if got[i] != expected.statement {
				t.Errorf("semiplotGrantStatements()[%d] = %q, want %q", i, got[i], expected.statement)
			}
		})
	}
}

// docs/architecture/provisioning.md#registering-new-pens
func TestRegisterFunctionIgnoresTheCallersSchemas(t *testing.T) {
	statements := sqlStatementsOf(semibase.SemiplotRegisterSQL)
	for _, clause := range []string{
		"CREATE OR REPLACE FUNCTION " + registerNewPensFunction + " RETURNS integer",
		"SECURITY DEFINER SET search_path = pg_catalog, pg_temp AS",
	} {
		if !strings.Contains(statements, clause) {
			t.Errorf("semiplot_register.sql does not declare %q", clause)
		}
	}
	for _, relation := range []string{"trends", "semiplot_tags"} {
		named, qualified := strings.Count(statements, relation), strings.Count(statements, "public."+relation)
		if named == 0 || named != qualified {
			t.Errorf("semiplot_register.sql names %s %d times, %d of them as public.%s", relation, named,
				qualified, relation)
		}
	}
}

func TestRegistrarStatements(t *testing.T) {
	want := []string{
		"GRANT SELECT ON public.trends TO semiplot_registrar",
		"GRANT SELECT (id), INSERT (id, name, color, enabled_on_start) ON semiplot_tags TO semiplot_registrar",
		"ALTER FUNCTION semiplot_register_new_pens() OWNER TO semiplot_registrar",
	}
	if got := registrarStatements(); !slices.Equal(got, want) {
		t.Errorf("registrarStatements() = %q, want %q", got, want)
	}
}

func TestRegistrarInsertGrantMatchesTheFunctionBody(t *testing.T) {
	const insert = "INSERT INTO public.semiplot_tags ("
	_, columns, found := strings.Cut(sqlStatementsOf(semibase.SemiplotRegisterSQL), insert)
	if !found {
		t.Fatalf("semiplot_register.sql has no %q", insert)
	}
	columns, _, _ = strings.Cut(columns, ")")
	if got := strings.Split(columns, ", "); !slices.Equal(got, registrarInsertColumns) {
		t.Errorf("the function body inserts %v, the registrar's INSERT grant covers %v", got,
			registrarInsertColumns)
	}
}

// a column added to semiplot_tags later fails here instead of arriving silently uneditable
func TestTagsUpdateGrantCoversEverySettingsColumn(t *testing.T) {
	var settings []string
	for _, column := range columnNamesOf(semibase.SemiplotTagsSQL) {
		if column != "id" {
			settings = append(settings, column)
		}
	}
	if !slices.Equal(plotEditableTagColumns, settings) {
		t.Errorf("the semiplot_tags UPDATE grant covers %v, the table's settings columns are %v",
			plotEditableTagColumns, settings)
	}
}

// the invariant the one-role decision does not weaken: nothing this package grants reaches the
// archive, and no semiplot_* table reaches the writer role.
func TestSemiplotGrantsLeaveTheArchiveAlone(t *testing.T) {
	forbidden := append([]string{WriterRole, "CREATE"}, archiveRelations...)
	for _, relation := range archiveRelations {
		_, bare, _ := strings.Cut(relation, ".")
		forbidden = append(forbidden, bare)
	}
	for _, statement := range semiplotGrantStatements() {
		for _, word := range forbidden {
			if strings.Contains(statement, word) {
				t.Errorf("%q names %s; the SemiPlot grants cover semiplot_* tables only", statement, word)
			}
		}
	}
}
