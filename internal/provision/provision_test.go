package provision

import (
	"testing"

	"github.com/jackc/pgx/v5"
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

// the string above is only worth anything if the driver reads the socket path back out of it
func TestConnectionStringIsParsedByTheDriver(t *testing.T) {
	tests := []struct {
		name    string
		options Options
		want    string
	}{
		{"tcp host", Options{Host: "localhost", Port: 5432}, "localhost"},
		{"socket directory", Options{Host: "/var/run/postgresql", Port: 15432}, "/var/run/postgresql"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := pgx.ParseConfig(tt.options.connectionString("scada_archive", "postgres", "p@ss word"))
			if err != nil {
				t.Fatalf("ParseConfig: %v", err)
			}
			if config.Host != tt.want {
				t.Errorf("parsed host = %q, want %q", config.Host, tt.want)
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
		})
	}
}

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
			"/var/run/postgresql/.s.PGSQL.5432/scada_archive",
		},
		{
			"trailing slash does not double",
			Options{Host: "/var/run/postgresql/", Port: 15432},
			"/var/run/postgresql/.s.PGSQL.15432/scada_archive",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.options.Endpoint("scada_archive"); got != tt.want {
				t.Errorf("Endpoint() = %q, want %q", got, tt.want)
			}
		})
	}
}
