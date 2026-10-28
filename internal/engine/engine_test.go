package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestClassify(t *testing.T) {
	good17 := Service{Name: "postgresql-x64-17", Data: `C:\pg\17\data`, Major: 17, Running: true,
		Port: 5432, Listening: 5432}
	stopped17 := Service{Name: "postgresql-x64-17", Data: `C:\pg\17\data`, Major: 17, Port: 5432}
	service14 := Service{Name: "postgresql-14", Data: `C:\pg\14\data`, Major: 14, Running: true,
		Port: 15432, Listening: 15432}
	broken13 := Service{Name: "postgresql-13", Major: 13, Problem: "service postgresql-13: x"}
	unknown := Service{Name: "postgresql-ci-17", Data: `C:\ci\data`,
		Problem: `service postgresql-ci-17: no file C:\ci\data\PG_VERSION`}
	moved17 := Service{Name: "postgresql-x64-17", Data: `C:\pg\17\data`, Major: 17, Running: true,
		Port: 15433, PortFile: `C:\pg\17\data\postgresql.conf`, Listening: 5432}
	movedByCommandLine := Service{Name: "postgresql-x64-17", Data: `C:\pg\17\data`, Major: 17,
		Running: true, Port: 15433, PortOnCommandLine: true, Listening: 5432}
	movedByDefault := Service{Name: "postgresql-x64-17", Data: `C:\pg\17\data`, Major: 17,
		Running: true, Port: 5432, Listening: 15433}
	unreadable17 := Service{Name: "postgresql-x64-17", Data: `C:\pg\17\data`, Major: 17,
		Unreadable: `C:\pg\17\data\postgresql.conf line 2: syntax error after the value of port`}
	unqueried := Service{Name: "PostgreSQL-other", Unreadable: "opening the service: Access is denied"}
	second17 := Service{Name: "postgresql-ci-17", Data: `C:\ci\data`, Major: 17, Port: 5432}
	service96 := Service{Name: "postgresql-9.6", Data: `C:\pg\9.6\data`, Major: 9, Port: 5433}
	tests := []struct {
		name         string
		services     []Service
		wantState    State
		wantService  string
		wantServices []string
		wantDetail   []string
	}{
		{"ambiguous: two 17 services", []Service{good17, second17}, StateAmbiguous, "",
			[]string{"postgresql-x64-17", "postgresql-ci-17"},
			[]string{"postgresql-x64-17, postgresql-ci-17"}},
		{"ambiguous: an unknown major beside a good 17", []Service{good17, unknown}, StateAmbiguous,
			"", []string{"postgresql-x64-17", "postgresql-ci-17"}, nil},
		{"ambiguous: an unqueried postgresql service beside a good 17", []Service{good17, unqueried},
			StateAmbiguous, "", []string{"postgresql-x64-17", "PostgreSQL-other"}, nil},
		{"incomplete: the one candidate has a problem", []Service{unknown}, StateIncomplete,
			"postgresql-ci-17", nil, []string{`no file C:\ci\data\PG_VERSION`}},
		{"incomplete wins over an unreadable setting", []Service{{Name: "postgresql-ci-17",
			Problem: "service postgresql-ci-17 runs pg_ctl.exe with no -D", Unreadable: "x"}},
			StateIncomplete, "postgresql-ci-17", nil, []string{"with no -D"}},
		{"unreadable: a 17 whose postgresql.conf does not parse", []Service{unreadable17},
			StateUnreadable, "postgresql-x64-17", nil,
			[]string{`service postgresql-x64-17: C:\pg\17\data\postgresql.conf line 2: syntax error`}},
		{"unreadable: an unqueried postgresql service alone", []Service{unqueried}, StateUnreadable,
			"PostgreSQL-other", nil,
			[]string{"service PostgreSQL-other: opening the service: Access is denied"}},
		{"port-mismatch: running on another port", []Service{moved17}, StatePortMismatch,
			"postgresql-x64-17", nil, []string{"5432", "15433", `C:\pg\17\data\postgresql.conf`}},
		{"port-mismatch: the command line sets the port", []Service{movedByCommandLine},
			StatePortMismatch, "postgresql-x64-17", nil,
			[]string{"listens on port 5432, and the service command line sets port 15433"}},
		{"port-mismatch: no setting, so the default", []Service{movedByDefault}, StatePortMismatch,
			"postgresql-x64-17", nil, []string{"listens on port 15433, and the default sets port 5432"}},
		{"found: running", []Service{good17}, StateFound, "postgresql-x64-17", nil,
			[]string{"postgresql-x64-17", "5432"}},
		{"found: stopped", []Service{stopped17}, StateFound, "postgresql-x64-17", nil, nil},
		{"found: 14 beside 17", []Service{service14, good17}, StateFound, "postgresql-x64-17", nil,
			nil},
		{"found: a broken other major beside 17", []Service{broken13, good17}, StateFound,
			"postgresql-x64-17", nil, nil},
		{"found: a 9.6 beside 17", []Service{service96, good17}, StateFound, "postgresql-x64-17",
			nil, nil},
		{"other-major: only 14", []Service{service14}, StateOtherMajor, "", nil,
			[]string{"postgresql-14 runs PostgreSQL 14"}},
		{"other-major: only 9.6", []Service{service96}, StateOtherMajor, "", nil,
			[]string{"postgresql-9.6 runs PostgreSQL 9"}},
		{"fresh: no service", nil, StateFresh, "", nil,
			[]string{"no service runs pg_ctl.exe runservice"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.services, 17)
			if got.State != tt.wantState {
				t.Fatalf("State = %q, want %q (detail %q)", got.State, tt.wantState, got.Detail)
			}
			if got.Service.Name != tt.wantService {
				t.Errorf("Service = %q, want %q", got.Service.Name, tt.wantService)
			}
			if !slices.Equal(got.Services, tt.wantServices) {
				t.Errorf("Services = %v, want %v", got.Services, tt.wantServices)
			}
			for _, want := range tt.wantDetail {
				if !strings.Contains(got.Detail, want) {
					t.Errorf("Detail = %q, want it to contain %q", got.Detail, want)
				}
			}
			if tt.wantState == StateFound && strings.Contains(got.Detail, "postgresql-14") {
				t.Errorf("Detail = %q names another service", got.Detail)
			}
		})
	}
}

func TestDescribe(t *testing.T) {
	const pid = "4242\nC:/pg/data\n1700000000\n5432\nlocalhost\n"
	tests := []struct {
		name            string
		files           map[string]string
		noData          bool
		commandLinePort int
		commandLineErr  error
		running         bool
		wantState       State
		wantPort        int
		wantListening   int
		wantUninstaller bool
		wantDetail      string
	}{
		{"a running 17", map[string]string{
			"data/PG_VERSION": "17\n", "data/postgresql.conf": "port = 5432\n",
			"data/postmaster.pid": pid, "uninstall-postgresql.exe": "",
		}, false, 0, nil, true, StateFound, 5432, 5432, true, ""},
		{"no -D", map[string]string{"uninstall-postgresql.exe": ""}, true, 0, nil, false,
			StateIncomplete, 0, 0, true, "service postgresql-ci-17 runs pg_ctl.exe with no -D"},
		{"no PG_VERSION", map[string]string{"data/postgresql.conf": ""}, false, 0, nil, false,
			StateIncomplete, 0, 0, false, "no file " + filepath.Join("<base>", "data", "PG_VERSION")},
		{"a pre-10 PG_VERSION is another major", map[string]string{
			"data/PG_VERSION": "9.6\n", "data/postgresql.conf": "",
		}, false, 0, nil, false, StateOtherMajor, 5432, 0, false, "postgresql-ci-17 runs PostgreSQL 9"},
		{"no postgresql.conf", map[string]string{"data/PG_VERSION": "17"}, false, 0, nil, false,
			StateUnreadable, 0, 0, false,
			"service postgresql-ci-17: no file " + filepath.Join("<base>", "data", "postgresql.conf")},
		{"a postgresql.conf that does not parse", map[string]string{
			"data/PG_VERSION": "17", "data/postgresql.conf": "port = '5432\n",
		}, false, 0, nil, false, StateUnreadable, 0, 0, false,
			filepath.Join("<base>", "data", "postgresql.conf") + " line 1: syntax error"},
		{"running without a pid file", map[string]string{
			"data/PG_VERSION": "17", "data/postgresql.conf": "",
		}, false, 0, nil, true, StateUnreadable, 5432, 0, false,
			"no listening port while the service runs: no file"},
		{"running with a short pid file", map[string]string{
			"data/PG_VERSION": "17", "data/postgresql.conf": "", "data/postmaster.pid": "4242\n",
		}, false, 0, nil, true, StateUnreadable, 5432, 0, false, "has no line 4"},
		{"the command line port wins over the conf", map[string]string{
			"data/PG_VERSION": "17", "data/postgresql.conf": "port = 15433",
			"data/postmaster.pid": strings.Replace(pid, "5432", "5433", 1),
		}, false, 5433, nil, true, StateFound, 5433, 5433, false, ""},
		{"a command line port that is not a number", map[string]string{
			"data/PG_VERSION": "17", "data/postgresql.conf": "port = 5432",
		}, false, 0, errors.New(`server options "-p five" set port "five", not a port number`), false,
			StateUnreadable, 0, 0, false, `service postgresql-ci-17: server options "-p five"`},
		{"stopped on a configured port", map[string]string{
			"data/PG_VERSION": "17", "data/postgresql.conf": "port = 15433",
		}, false, 0, nil, false, StateFound, 15433, 0, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := t.TempDir()
			writeFiles(t, base, tt.files)
			command := pgCtlCommand{base: base, data: filepath.Join(base, "data"),
				port: tt.commandLinePort, portErr: tt.commandLineErr}
			if tt.noData {
				command.data = ""
			}
			service := describe("postgresql-ci-17", command, tt.running)
			got := Classify([]Service{service}, 17)
			if got.State != tt.wantState {
				t.Fatalf("State = %q, want %q (detail %q)", got.State, tt.wantState, got.Detail)
			}
			if service.Port != tt.wantPort || service.Listening != tt.wantListening {
				t.Errorf("Port, Listening = %d, %d, want %d, %d",
					service.Port, service.Listening, tt.wantPort, tt.wantListening)
			}
			if service.Uninstaller != tt.wantUninstaller {
				t.Errorf("Uninstaller = %v, want %v", service.Uninstaller, tt.wantUninstaller)
			}
			wantDetail := strings.ReplaceAll(tt.wantDetail, "<base>", base)
			if !strings.Contains(got.Detail, wantDetail) {
				t.Errorf("Detail = %q, want it to contain %q", got.Detail, wantDetail)
			}
		})
	}
}

func TestDescribeNamesThePortFile(t *testing.T) {
	data := t.TempDir()
	writeFiles(t, data, map[string]string{"PG_VERSION": "17", "postgresql.conf": "port = 15433\n"})
	fromFile := describe("pg", pgCtlCommand{data: data}, false)
	if fromFile.PortFile != filepath.Join(data, "postgresql.conf") || fromFile.PortOnCommandLine {
		t.Errorf("from the conf: PortFile, PortOnCommandLine = %q, %v", fromFile.PortFile,
			fromFile.PortOnCommandLine)
	}
	fromCommandLine := describe("pg", pgCtlCommand{data: data, port: 5433}, false)
	if fromCommandLine.PortFile != "" || !fromCommandLine.PortOnCommandLine {
		t.Errorf("from the command line: PortFile, PortOnCommandLine = %q, %v",
			fromCommandLine.PortFile, fromCommandLine.PortOnCommandLine)
	}
}

func TestCollectServices(t *testing.T) {
	pgCtl := Service{Name: "postgresql-x64-17", Major: 17}
	// the trailing period of a Windows error text is trimmed
	denied := fmt.Errorf("opening the service: %s", "Access is denied.")
	read := func(name string) (Service, bool, error) {
		switch name {
		case "postgresql-x64-17":
			return pgCtl, true, nil
		case "Spooler":
			return Service{}, false, nil
		default:
			return Service{}, false, denied
		}
	}
	tests := []struct {
		name  string
		names []string
		want  []Service
	}{
		{"a pg_ctl service", []string{"postgresql-x64-17"}, []Service{pgCtl}},
		{"a service that runs no pg_ctl.exe", []string{"Spooler"}, nil},
		{"an unqueried service of another name", []string{"OtherService"}, nil},
		{"an unqueried postgresql service, any letter case", []string{"PostgreSQL-Other"},
			[]Service{{Name: "PostgreSQL-Other", Unreadable: "opening the service: Access is denied"}}},
		{"both, in list order", []string{"postgresql-old", "Spooler", "postgresql-x64-17"},
			[]Service{{Name: "postgresql-old", Unreadable: "opening the service: Access is denied"},
				pgCtl}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collectServices(tt.names, read)
			if !slices.Equal(got, tt.want) {
				t.Errorf("collectServices(%v) = %+v, want %+v", tt.names, got, tt.want)
			}
		})
	}
}

func TestReadMajor(t *testing.T) {
	tests := []struct {
		name      string
		content   *string
		wantMajor int
		wantError string
	}{
		{"a major with a newline", new("17\n"), 17, ""},
		{"a major without a newline", new("14"), 14, ""},
		{"a major with CRLF", new("17\r\n"), 17, ""},
		{"a pre-10 major.minor", new("9.6\n"), 9, ""},
		{"missing", nil, 0, "no file "},
		{"not a number", new("seventeen\n"), 0, "not a major version"},
		{"garbage after the dot", new("9.x\n"), 0, "not a major version"},
		{"three parts", new("9.6.1\n"), 0, "not a major version"},
		{"a dot with no minor", new("9.\n"), 0, "not a major version"},
		{"a sign", new("+17\n"), 0, "not a major version"},
		{"zero", new("0\n"), 0, "not a major version"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := t.TempDir()
			if tt.content != nil {
				writeFiles(t, data, map[string]string{"PG_VERSION": *tt.content})
			}
			major, err := readMajor(data)
			if major != tt.wantMajor {
				t.Errorf("major = %d, want %d", major, tt.wantMajor)
			}
			assertError(t, err, tt.wantError)
		})
	}
}

func TestReadMajorTellsAReadErrorFromAMissingFile(t *testing.T) {
	data := t.TempDir()
	if err := os.Mkdir(filepath.Join(data, "PG_VERSION"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := readMajor(data)
	if err == nil || strings.HasPrefix(err.Error(), "no file") {
		t.Fatalf("readMajor over a folder named PG_VERSION = %v, want a read error", err)
	}
}

func TestListeningPort(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantPort  int
		wantError string
	}{
		{"line 4", "4242\nC:/pg/data\n1700000000\n15432\nlocalhost\n", 15432, ""},
		{"CRLF line endings", "4242\r\nC:/pg/data\r\n1700000000\r\n5432\r\n", 5432, ""},
		{"three lines", "4242\nC:/pg/data\n1700000000", 0, "has no line 4"},
		{"no number on line 4", "4242\nC:/pg/data\n1700000000\n\n", 0, "not a port"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := t.TempDir()
			writeFiles(t, data, map[string]string{"postmaster.pid": tt.content})
			port, err := listeningPort(data)
			if port != tt.wantPort {
				t.Errorf("port = %d, want %d", port, tt.wantPort)
			}
			assertError(t, err, tt.wantError)
		})
	}
}

func TestParsePort(t *testing.T) {
	tests := []struct {
		value  string
		want   int
		wantOK bool
	}{
		{"5432", 5432, true},
		{" 5432 ", 5432, true},
		{"0x1538", 5432, true},
		{"0X1538", 5432, true},
		{"012470", 5432, true},
		{"+5432", 5432, true},
		{"65535", 65535, true},
		{"0", 0, false},
		{"65536", 0, false},
		{"-5432", 0, false},
		{"five", 0, false},
		{"", 0, false},
		{"5_432", 0, false},
		{"0o12470", 0, false},
		{"0b1010100111000", 0, false},
		{"5432abc", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got, ok := parsePort(tt.value)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("parsePort(%q) = %d, %v, want %d, %v", tt.value, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestParseConfLine(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantName  string
		wantValue string
		wantOK    bool
		wantError string
	}{
		{"name = value", "port = 5432", "port", "5432", true, ""},
		{"no equals sign", "port 5432", "port", "5432", true, ""},
		{"no spaces", "port=5432", "port", "5432", true, ""},
		{"upper-case name", "PORT = 5432", "PORT", "5432", true, ""},
		{"quoted value", "port = '5432'", "port", "5432", true, ""},
		{"quoted value with a doubled quote", `include = 'it''s.conf'`, "include", "it's.conf", true, ""},
		{"quoted value with escapes", `include = 'a\\b\'c\d'`, "include", `a\b'cd`, true, ""},
		{"quoted value with a hash", "include = 'a#b.conf' # note", "include", "a#b.conf", true, ""},
		{"trailing comment", "port = 5432  # the default", "port", "5432", true, ""},
		{"trailing comment without a space", "port = 5432#x", "port", "5432", true, ""},
		{"leading whitespace", "\t  port = 5432\r", "port", "5432", true, ""},
		{"commented-out line", "#port = 5432", "", "", false, ""},
		{"indented comment", "   # port = 5432", "", "", false, ""},
		{"blank line", "  \r", "", "", false, ""},
		{"no value", "port =", "", "", false, "has no value"},
		{"no value before a comment", "port = # x", "", "", false, "has no value"},
		{"a name alone", "port", "", "", false, "no value"},
		{"unterminated quote", "port = '5432", "", "", false, "unterminated"},
		{"garbage after the value", "port = 5432 5433", "", "", false, "after the value"},
		{"no name", "= 5432", "", "", false, "no setting name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, value, ok, err := parseConfLine(tt.line)
			if name != tt.wantName || value != tt.wantValue || ok != tt.wantOK {
				t.Errorf("parseConfLine(%q) = %q, %q, %v, want %q, %q, %v",
					tt.line, name, value, ok, tt.wantName, tt.wantValue, tt.wantOK)
			}
			assertError(t, err, tt.wantError)
		})
	}
}

func TestConfiguredPort(t *testing.T) {
	tests := []struct {
		name      string
		files     map[string]string
		wantPort  int
		wantFile  string
		wantError string
	}{
		{"no setting", map[string]string{"postgresql.conf": "max_connections = 100\n"},
			defaultPort, "", ""},
		{"commented-out setting", map[string]string{"postgresql.conf": "#port = 15432\n"},
			defaultPort, "", ""},
		{"the last setting wins", map[string]string{"postgresql.conf": "port = 15432\nport = 15433\n"},
			15433, "postgresql.conf", ""},
		{"upper-case name", map[string]string{"postgresql.conf": "PORT 15432\n"},
			15432, "postgresql.conf", ""},
		{"a hexadecimal port", map[string]string{"postgresql.conf": "port = 0x1538\n"},
			5432, "postgresql.conf", ""},
		{"a leading-zero octal port", map[string]string{"postgresql.conf": "port = '012470'\n"},
			5432, "postgresql.conf", ""},
		{"postgresql.auto.conf over postgresql.conf", map[string]string{
			"postgresql.conf":      "port = 15432\n",
			"postgresql.auto.conf": "# Do not edit this file manually!\nport = '15433'\n",
		}, 15433, "postgresql.auto.conf", ""},
		{"postgresql.auto.conf without a port", map[string]string{
			"postgresql.conf":      "port = 15432\n",
			"postgresql.auto.conf": "listen_addresses = 'localhost'\n",
		}, 15432, "postgresql.conf", ""},
		{"a relative include", map[string]string{
			"postgresql.conf":  "port = 15432\ninclude 'conf/extra.conf'\n",
			"conf/extra.conf":  "include = 'nested.conf'\n",
			"conf/nested.conf": "port = 15434\n",
		}, 15434, filepath.Join("conf", "nested.conf"), ""},
		{"a setting after an include", map[string]string{
			"postgresql.conf": "include 'extra.conf'\nport = 15432\n",
			"extra.conf":      "port = 15434\n",
		}, 15432, "postgresql.conf", ""},
		{"include_dir in name order", map[string]string{
			"postgresql.conf":     "include_dir 'conf.d'\n",
			"conf.d/10-a.conf":    "port = 15441\n",
			"conf.d/20-b.conf":    "port = 15442\n",
			"conf.d/.hidden.conf": "port = 15443\n",
			"conf.d/30-c.txt":     "port = 15444\n",
		}, 15442, filepath.Join("conf.d", "20-b.conf"), ""},
		{"a missing include_if_exists file", map[string]string{
			"postgresql.conf": "port = 15432\ninclude_if_exists 'absent.conf'\n",
		}, 15432, "postgresql.conf", ""},
		{"a missing include file", map[string]string{
			"postgresql.conf": "include 'absent.conf'\n",
		}, 0, "", "no file "},
		{"a missing include_dir", map[string]string{
			"postgresql.conf": "include_dir 'absent'\n",
		}, 0, "", "no folder "},
		{"a self include", map[string]string{"postgresql.conf": "include 'postgresql.conf'\n"},
			0, "", "nest deeper"},
		{"a port that is not a number", map[string]string{"postgresql.conf": "port = five\n"},
			0, "", "line 1: port \"five\""},
		{"a syntax error", map[string]string{"postgresql.conf": "\nport = '5432\n"},
			0, "", "line 2: syntax error"},
		{"no postgresql.conf", map[string]string{}, 0, "", "no file "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := t.TempDir()
			writeFiles(t, data, tt.files)
			port, file, err := configuredPort(data)
			wantFile := tt.wantFile
			if wantFile != "" {
				wantFile = filepath.Join(data, wantFile)
			}
			if port != tt.wantPort || file != wantFile {
				t.Errorf("configuredPort = %d, %q, want %d, %q", port, file, tt.wantPort, wantFile)
			}
			assertError(t, err, tt.wantError)
		})
	}
}

func assertError(t *testing.T, err error, want string) {
	t.Helper()
	switch {
	case want == "" && err != nil:
		t.Errorf("error = %v, want none", err)
	case want != "" && (err == nil || !strings.Contains(err.Error(), want)):
		t.Errorf("error = %v, want one containing %q", err, want)
	}
}
