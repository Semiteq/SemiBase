package provision

import (
	"fmt"
	"strings"

	semibase "github.com/Semiteq/SemiBase"
)

// applied in this order, after public.trends exists: the function body is checked against the pen
// table and trends when it is created, and a membership row references a pen
var semiplotSchemaFiles = []struct {
	name string
	sql  string
}{
	{"semiplot_tags.sql", semibase.SemiplotTagsSQL},
	{"semiplot_register.sql", semibase.SemiplotRegisterSQL},
	{"semiplot_groups.sql", semibase.SemiplotGroupsSQL},
	{"semiplot_meta.sql", semibase.SemiplotMetaSQL},
}

var plotEditableTagColumns = []string{
	"name",
	"unit",
	"format",
	"color",
	"line_style",
	"enabled_on_start",
	"scale_min",
	"scale_max",
}

const (
	plotReadGrants  = "SELECT"
	plotWriteGrants = "SELECT, INSERT, UPDATE, DELETE"
)

var plotTagsGrants = "SELECT, UPDATE (" + strings.Join(plotEditableTagColumns, ", ") + ")"

// docs/architecture/provisioning.md#the-semiplot-configuration-schema
var semiplotTables = []struct {
	name   string
	grants string
}{
	{"semiplot_tags", plotTagsGrants},
	{"semiplot_groups", plotWriteGrants},
	{"semiplot_pen_groups", plotWriteGrants},
	{"semiplot_meta", plotReadGrants},
}

func semiplotTableNames() []string {
	names := make([]string, 0, len(semiplotTables))
	for _, table := range semiplotTables {
		names = append(names, table.name)
	}
	return names
}

func plotGrantsOn(table string) string {
	for _, candidate := range semiplotTables {
		if candidate.name == table {
			return candidate.grants
		}
	}
	return ""
}

const registerNewPensFunction = "semiplot_register_new_pens()"

// a new function is executable by PUBLIC
func semiplotGrantStatements() []string {
	statements := make([]string, 0, len(semiplotTables)+1)
	for _, table := range semiplotTables {
		statements = append(statements, fmt.Sprintf("GRANT %s ON %s TO %s", table.grants, table.name, PlotRole))
	}
	return append(statements, fmt.Sprintf("REVOKE ALL ON FUNCTION %s FROM PUBLIC; GRANT EXECUTE ON FUNCTION %s TO %s",
		registerNewPensFunction, registerNewPensFunction, PlotRole))
}

var registrarInsertColumns = []string{"id", "name", "color", "enabled_on_start"}

// ON CONFLICT (id) needs SELECT on id to infer the primary key
func registrarStatements() []string {
	return []string{
		fmt.Sprintf("GRANT SELECT ON %s TO %s", archiveTable, RegistrarRole),
		fmt.Sprintf("GRANT SELECT (id), INSERT (%s) ON semiplot_tags TO %s",
			strings.Join(registrarInsertColumns, ", "), RegistrarRole),
		fmt.Sprintf("ALTER FUNCTION %s OWNER TO %s", registerNewPensFunction, RegistrarRole),
	}
}
