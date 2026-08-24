package provision

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	semibase "github.com/Semiteq/SemiBase"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name     string
		database string
		wantErr  bool
	}{
		{"plain", "scada_archive", false},
		{"leading underscore", "_archive", false},
		{"digits after letter", "archive2", false},
		{"single letter", "a", false},
		{"uppercase", "Scada_Archive", true},
		{"hyphen", "scada-archive", true},
		{"leading digit", "2archive", true},
		{"double quote", `archive"x`, true},
		{"single quote", "archive'x", true},
		{"semicolon", "archive;drop database postgres", true},
		{"empty", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Options{Database: tt.database}.Validate()
			if gotErr := err != nil; gotErr != tt.wantErr {
				t.Errorf("Validate() with database %q: error = %v, wantErr %v", tt.database, err, tt.wantErr)
			}
		})
	}
}

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

// the backslash cases pin the standard_conforming_strings contract
func TestEscapeLiteral(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain", "secret", "secret"},
		{"single quote", "o'brien", "o''brien"},
		{"injection attempt", "'; DROP ROLE postgres; --", "''; DROP ROLE postgres; --"},
		{"backslash kept literal", `pa\ss`, `pa\ss`},
		{"backslash before quote", `\'`, `\''`},
		{"trailing backslash", `pass\`, `pass\`},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapeLiteral(tt.input); got != tt.want {
				t.Errorf("escapeLiteral(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// the socket form is what makes the binary usable as a postgres image init script: the
// entrypoint's temporary server sets listen_addresses to empty, so nothing answers on TCP
func TestConnectionString(t *testing.T) {
	tests := []struct {
		name    string
		options Options
		want    string
	}{
		{
			"tcp host",
			Options{Host: "localhost", Port: 5432},
			"postgres://postgres:secret@localhost:5432/scada_archive",
		},
		{
			"tcp host on a custom port",
			Options{Host: "db.internal", Port: 15432},
			"postgres://postgres:secret@db.internal:15432/scada_archive",
		},
		{
			"ipv6 literal keeps its brackets",
			Options{Host: "::1", Port: 5432},
			"postgres://postgres:secret@[::1]:5432/scada_archive",
		},
		{
			"socket directory moves to the host parameter",
			Options{Host: "/var/run/postgresql", Port: 5432},
			"postgres://postgres:secret@/scada_archive?host=%2Fvar%2Frun%2Fpostgresql&port=5432",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.options.connectionString("scada_archive", "postgres", "secret")
			if got != tt.want {
				t.Errorf("connectionString() = %q, want %q", got, tt.want)
			}
		})
	}
}

// the golden strings above prove nothing on their own: ?host=/var/run/postgresql and
// ?host=%2Fvar%2Frun%2Fpostgresql parse to the same config. What has to hold is that the
// driver then dials the socket, which is the network pgconn derives from the parsed host
func TestConnectionStringIsDialedAsTheRightNetwork(t *testing.T) {
	tests := []struct {
		name        string
		options     Options
		wantHost    string
		wantNetwork string
	}{
		{"tcp host", Options{Host: "localhost", Port: 5432}, "localhost", "tcp"},
		{
			"socket directory",
			Options{Host: "/var/run/postgresql", Port: 15432},
			"/var/run/postgresql",
			"unix",
		},
		{
			"windows drive path",
			Options{Host: `C:\pgsock`, Port: 5432},
			`C:\pgsock`,
			"unix",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := pgx.ParseConfig(tt.options.connectionString("scada_archive", "postgres", "p@ss word"))
			if err != nil {
				t.Fatalf("ParseConfig: %v", err)
			}
			if config.Host != tt.wantHost {
				t.Errorf("parsed host = %q, want %q", config.Host, tt.wantHost)
			}
			if config.Port != uint16(tt.options.Port) {
				t.Errorf("parsed port = %d, want %d", config.Port, tt.options.Port)
			}
			if config.Database != "scada_archive" {
				t.Errorf("parsed database = %q, want %q", config.Database, "scada_archive")
			}
			if config.Password != "p@ss word" {
				t.Errorf("parsed password = %q, want %q", config.Password, "p@ss word")
			}
			// the address is filepath.Join'd, so it differs by GOOS; only the network is portable
			network, address := pgconn.NetworkAddress(config.Host, config.Port)
			if network != tt.wantNetwork {
				t.Errorf("dialled network = %q (address %q), want %q", network, address, tt.wantNetwork)
			}
		})
	}
}

// the boundary of the socket branch is pgconn.isAbsolutePath and nothing wider: a host this
// says yes to and pgx says no to would be dialled as a TCP name found in the host parameter
func TestIsSocketHost(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"/var/run/postgresql", true},
		{"/", true},
		{`C:\pgsock`, true},
		{`C:\`, true},
		{`c:\pgsock`, false},
		{"C:/pgsock", false},
		{"C:pgsock", false},
		{"@abstract", false},
		{"localhost", false},
		{"::1", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := isSocketHost(tt.host); got != tt.want {
				t.Errorf("isSocketHost(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

// the socket form must stay one quotable path: a database appended behind a slash would
// name a directory under the socket file, which cannot exist
func TestEndpoint(t *testing.T) {
	tests := []struct {
		name    string
		options Options
		want    string
	}{
		{"tcp host", Options{Host: "localhost", Port: 5432}, "localhost:5432/scada_archive"},
		{
			"socket directory names the socket file",
			Options{Host: "/var/run/postgresql", Port: 5432},
			"/var/run/postgresql/.s.PGSQL.5432 (scada_archive)",
		},
		{
			"trailing slash does not double",
			Options{Host: "/var/run/postgresql/", Port: 15432},
			"/var/run/postgresql/.s.PGSQL.15432 (scada_archive)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.options.endpoint("scada_archive"); got != tt.want {
				t.Errorf("endpoint() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Endpoint is the exported one, and it names the archive database rather than taking one
func TestEndpointUsesTheConfiguredDatabase(t *testing.T) {
	options := Options{Host: "/var/run/postgresql", Port: 5432, Database: "semiplot_dev"}
	if got, want := options.Endpoint(), "/var/run/postgresql/.s.PGSQL.5432 (semiplot_dev)"; got != want {
		t.Errorf("Endpoint() = %q, want %q", got, want)
	}
}

// the file's header prose names the objects the DDL deliberately leaves alone, so a claim
// about what the DDL creates has to be made against the statements only
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

// the archive table is the vendor's shape, transcribed by hand from a dump, and the whole
// statement text is pinned rather than a few fragments of it: a consumer that reads id, l, t,
// v and q gets wrong charts rather than an error when a type, a default or a column drifts.
// Reformatting the file is meant to fail this - re-read sql/semiplot_dev.sql in the consumer
// repository before changing the golden text.
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
