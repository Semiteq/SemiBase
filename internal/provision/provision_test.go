package provision

import (
	"testing"
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

// The statement-builder tests pin the exact DDL text Create and ensureDatabase
// execute: a hostile database name comes out as one double-quoted identifier,
// inert as SQL. Dropping the Sanitize call at either site fails these.
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

// The backslash cases pin escapeLiteral's standard_conforming_strings
// contract.
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
