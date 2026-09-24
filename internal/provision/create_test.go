package provision

import (
	"strings"
	"testing"

	semibase "github.com/Semiteq/SemiBase"
)

// these pin the exact DDL text: a hostile database name comes out as one
// double-quoted identifier, inert as SQL. dropping Sanitize fails them
func TestGrantConnectStatement(t *testing.T) {
	hostile := `archive"; DROP DATABASE postgres; --`
	want := `GRANT CONNECT ON DATABASE "archive""; DROP DATABASE postgres; --" TO scada_writer`
	if got := grantConnectStatement(hostile, WriterRole); got != want {
		t.Errorf("grantConnectStatement(%q) = %q, want %q", hostile, got, want)
	}
}

func TestCreateDatabaseStatement(t *testing.T) {
	hostile := `archive"; DROP DATABASE postgres; --`
	want := `CREATE DATABASE "archive""; DROP DATABASE postgres; --"`
	if got := createDatabaseStatement(hostile); got != want {
		t.Errorf("createDatabaseStatement(%q) = %q, want %q", hostile, got, want)
	}
}

func TestRoleStatementsEscapeThePassword(t *testing.T) {
	password := `p'w\d`
	if got, want := createRoleStatement(WriterRole, password),
		`CREATE ROLE scada_writer LOGIN PASSWORD 'p''w\d'`; got != want {
		t.Errorf("createRoleStatement = %q, want %q", got, want)
	}
	if got, want := alterRolePasswordStatement(WriterRole, password),
		`ALTER ROLE scada_writer PASSWORD 'p''w\d'`; got != want {
		t.Errorf("alterRolePasswordStatement = %q, want %q", got, want)
	}
}

// the archive table is the vendor's shape, transcribed by hand from a dump, and the whole
// statement text is pinned rather than a few fragments of it: a consumer that reads id, l, t,
// v and q gets wrong charts rather than an error when a type, a default or a column drifts.
func TestEmbeddedTrendsSQL(t *testing.T) {
	want := "CREATE TABLE public.trends ( " +
		"id integer DEFAULT 0 NOT NULL, " +
		"l smallint DEFAULT 0 NOT NULL, " +
		"t timestamp(3) without time zone NOT NULL, " +
		"v double precision, " +
		"q integer NOT NULL " +
		") PARTITION BY RANGE (t); " +
		"ALTER TABLE ONLY public.trends ADD CONSTRAINT tpk PRIMARY KEY (id, l, t); " +
		"CREATE TABLE public.tpdefault PARTITION OF public.trends DEFAULT;"
	got := sqlStatementsOf(semibase.TrendsSQL)
	if got != want {
		t.Errorf("trends.sql statements =\n%q\nwant\n%q", got, want)
	}
	// unqualified as well as public.messages: the guard is about the object, not the spelling
	if strings.Contains(got, "messages") {
		t.Error("trends.sql creates messages; nothing we ship reads it")
	}
}
