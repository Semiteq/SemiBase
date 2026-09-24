package provision

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// a probe against semiplot_meta or public.trends would write what the run then asserts the role
// cannot write
func TestPlotWriteProbesCoverTheWritableTables(t *testing.T) {
	want := map[string][]string{
		"semiplot_tags":       {"UPDATE"},
		"semiplot_groups":     {"INSERT", "UPDATE", "DELETE"},
		"semiplot_pen_groups": {"INSERT", "UPDATE", "DELETE"},
	}
	exercised := map[string][]string{}
	for _, probe := range plotWriteProbes() {
		exercised[probe.table] = append(exercised[probe.table], probe.operation)
	}
	if len(exercised) != len(want) {
		t.Fatalf("the probes touch %d tables, want %d", len(exercised), len(want))
	}
	for table, operations := range want {
		t.Run(table, func(t *testing.T) {
			got := slices.Sorted(slices.Values(exercised[table]))
			if !slices.Equal(got, slices.Sorted(slices.Values(operations))) {
				t.Errorf("the probes issue %v against %s, want %v", got, table, operations)
			}
		})
	}
}

func TestPlotTagsUpdateProbeWritesEverySettingsColumn(t *testing.T) {
	want := "UPDATE semiplot_tags SET name = name, unit = unit, format = format, color = color, " +
		"line_style = line_style, enabled_on_start = enabled_on_start, scale_min = scale_min, " +
		"scale_max = scale_max WHERE id = -1"
	probes := plotWriteProbes()
	if got := probes[probePosition(t, probes, "semiplot_tags", "UPDATE")].statement; got != want {
		t.Errorf("the semiplot_tags probe is %q, want %q", got, want)
	}
}

func probePosition(t *testing.T, probes []plotWriteProbe, table, operation string) int {
	t.Helper()
	for i, probe := range probes {
		if probe.table == table && probe.operation == operation {
			return i
		}
	}
	t.Fatalf("no %s probe against %s", operation, table)
	return -1
}

func TestPlotForbiddenTagWritesCoverTheWithheldWrites(t *testing.T) {
	want := []string{"INSERT INTO semiplot_tags ", "DELETE FROM semiplot_tags ", "UPDATE semiplot_tags SET id = "}
	probes := plotForbiddenTagWrites()
	if len(probes) != len(want) {
		t.Fatalf("%d refused-write probes, want %d", len(probes), len(want))
	}
	for i, prefix := range want {
		if !strings.HasPrefix(probes[i].statement, prefix) {
			t.Errorf("refused-write probe %d is %q, want it to start with %q", i, probes[i].statement, prefix)
		}
	}
}

// a membership row names a group, so the group is inserted before it and deleted after it
func TestPlotWriteProbeOrderSatisfiesTheForeignKeys(t *testing.T) {
	probes := plotWriteProbes()
	position := func(table, operation string) int {
		return probePosition(t, probes, table, operation)
	}
	if position("semiplot_groups", "INSERT") > position("semiplot_pen_groups", "INSERT") {
		t.Error("a membership row is written before the group it names")
	}
	if position("semiplot_pen_groups", "DELETE") > position("semiplot_groups", "DELETE") {
		t.Error("the group is removed before the membership row that names it")
	}
}

func TestWriteProbeFailure(t *testing.T) {
	probe := plotWriteProbe{"semiplot_groups", "UPDATE", "UPDATE semiplot_groups SET name = name"}

	want := "semiplot cannot UPDATE semiplot_groups: permission denied. " +
		"Repair with GRANT SELECT, INSERT, UPDATE, DELETE ON semiplot_groups TO semiplot, then re-run this command"
	if got := writeProbeFailure(probe, errors.New("permission denied")); got.Error() != want {
		t.Errorf("writeProbeFailure() = %q, want %q", got, want)
	}

	tags := writeProbeFailure(plotWriteProbe{"semiplot_tags", "UPDATE", ""}, errors.New("permission denied"))
	wantTags := "GRANT SELECT, UPDATE (name, unit, format, color, line_style, enabled_on_start, scale_min, " +
		"scale_max) ON semiplot_tags TO semiplot"
	if !strings.Contains(tags.Error(), wantTags) {
		t.Errorf("the semiplot_tags repair does not prescribe the column grant: %q", tags)
	}
}

func TestTooWideTagGrantFailure(t *testing.T) {
	cases := []struct {
		operation string
		named     []string
	}{
		{"INSERT", []string{"semiplot may INSERT semiplot_tags", "REVOKE INSERT ON semiplot_tags FROM semiplot"}},
		{"UPDATE (id)", []string{"semiplot may UPDATE (id) semiplot_tags", "REVOKE UPDATE ON semiplot_tags FROM"}},
	}
	for _, c := range cases {
		t.Run(c.operation, func(t *testing.T) {
			message := tooWideTagGrantFailure(plotWriteProbe{"semiplot_tags", c.operation, ""}).Error()
			for _, named := range append(c.named, "too wide", "PUBLIC", "membership") {
				if !strings.Contains(message, named) {
					t.Errorf("the message does not name %q: %q", named, message)
				}
			}
		})
	}
}

func TestRefusalVerdict(t *testing.T) {
	probe := plotWriteProbe{"semiplot_tags", "INSERT", "INSERT INTO semiplot_tags (id, name) VALUES (-1, 'x')"}
	tooWide := tooWideTagGrantFailure(probe).Error()
	cases := []struct {
		name        string
		probeErr    error
		wantPass    bool
		wantTooWide bool
	}{
		{"accepted", nil, false, true},
		{"refused", &pgconn.PgError{Code: "42501", Message: "permission denied"}, true, false},
		{"unique violation", &pgconn.PgError{Code: "23505", Message: "duplicate key"}, false, true},
		{"check violation", &pgconn.PgError{Code: "23514", Message: "violates check"}, false, true},
		{"statement timeout", &pgconn.PgError{Code: "57014", Message: "canceling statement"}, false, false},
		{"connection lost", errors.New("unexpected EOF"), false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			verdict := refusalVerdict(probe, c.probeErr)
			if c.wantPass {
				if verdict != nil {
					t.Fatalf("refusalVerdict() = %q, want nil", verdict)
				}
				return
			}
			if verdict == nil {
				t.Fatal("refusalVerdict() = nil, want a failure")
			}
			if got := strings.Contains(verdict.Error(), tooWide); got != c.wantTooWide {
				t.Errorf("names the grant too wide = %t, want %t: %q", got, c.wantTooWide, verdict)
			}
			if c.probeErr != nil && !errors.Is(verdict, c.probeErr) {
				t.Errorf("the verdict does not wrap the probe error: %q", verdict)
			}
		})
	}
}

func TestPublicRegistersPensFailure(t *testing.T) {
	message := publicRegistersPensFailure().Error()
	for _, named := range []string{"PUBLIC holds EXECUTE on semiplot_register_new_pens()",
		"REVOKE ALL ON FUNCTION semiplot_register_new_pens() FROM PUBLIC"} {
		if !strings.Contains(message, named) {
			t.Errorf("the message does not name %q: %q", named, message)
		}
	}
}

// has_table_privilege and has_schema_privilege both answer through role membership, so a repair
// line prescribing only the direct REVOKE sends the operator into a no-op they can repeat forever
func TestExcessPrivilegeFailure(t *testing.T) {
	want := "semiplot holds INSERT on public.trends and must not write it. " +
		"Repair with REVOKE INSERT ON public.trends FROM semiplot, or, if the privilege comes through " +
		"PUBLIC or a role semiplot is a member of, revoke it there or drop the membership; " +
		"then re-run this command"
	if got := excessPrivilegeFailure("public.trends", "INSERT"); got.Error() != want {
		t.Errorf("excessPrivilegeFailure() = %q, want %q", got, want)
	}
}

// check runs only after create, which has just revoked CREATE from PUBLIC, so the one repair this
// message must not prescribe is that same REVOKE
func TestCreateInPublicFailure(t *testing.T) {
	message := createInPublicFailure().Error()
	if strings.Contains(message, "FROM PUBLIC") {
		t.Errorf("the repair prescribes the revoke create has already issued: %q", message)
	}
	for _, named := range []string{"REVOKE CREATE ON SCHEMA public FROM semiplot", "membership", WriterRole} {
		if !strings.Contains(message, named) {
			t.Errorf("the message does not name %q: %q", named, message)
		}
	}
}

// the relations the tail check asserts the SemiPlot role cannot write, which is also the set no
// grant this package issues to that role may name
var archiveRelations = []string{archiveTable, scadaMessages}

// a relation in both lists would be granted by one half of the run and refused by the other
func TestArchiveRelationsAreDisjointFromTheGrantedTables(t *testing.T) {
	for _, relation := range archiveRelations {
		if slices.Contains(semiplotTableNames(), relation) {
			t.Errorf("%s is both asserted unwritable and granted by this package", relation)
		}
	}
}
