//go:build windows

package engine

import (
	"os"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestDataDirectoryFlag(t *testing.T) {
	tests := []struct {
		name      string
		pathName  string
		want      string
		wantFound bool
	}{
		{
			name:     "pg_ctl register, quoted with spaces",
			pathName: `"C:\Program Files\PostgreSQL\14\bin\pg_ctl.exe" runservice -N "postgresql-14" -D "C:\Program Files\PostgreSQL\14\data" -w`,
			want:     `C:\Program Files\PostgreSQL\14\data`, wantFound: true,
		},
		{
			name:     "unquoted",
			pathName: `C:\pg\bin\pg_ctl.exe runservice -D C:\pg\data -w`,
			want:     `C:\pg\data`, wantFound: true,
		},
		{
			name:     "value attached to the flag",
			pathName: `C:\pg\bin\pg_ctl.exe runservice -DC:\pg\data`,
			want:     `C:\pg\data`, wantFound: true,
		},
		{name: "no -D", pathName: `"C:\pg\bin\pg_ctl.exe" runservice -N "pg" -w`},
		{name: "-D with no value", pathName: `C:\pg\bin\pg_ctl.exe runservice -D`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := DataDirectoryFlag(tt.pathName)
			if got != tt.want || found != tt.wantFound {
				t.Errorf("DataDirectoryFlag() = %q, %v, want %q, %v", got, found, tt.want, tt.wantFound)
			}
		})
	}
}

func TestParsePgCtlCommand(t *testing.T) {
	tests := []struct {
		name        string
		pathName    string
		wantPgCtl   bool
		want        pgCtlCommand
		wantPortErr string
	}{
		{
			name:      "EDB 17, quoted path with spaces",
			pathName:  `"C:\Program Files\PostgreSQL\17\bin\pg_ctl.exe" runservice -N "postgresql-x64-17" -D "C:\Program Files\PostgreSQL\17\data" -w`,
			wantPgCtl: true,
			want:      pgCtlCommand{base: `C:\Program Files\PostgreSQL\17`, data: `C:\Program Files\PostgreSQL\17\data`},
		},
		{
			name:      "unquoted path, -D separate",
			pathName:  `C:\pg\bin\pg_ctl.exe runservice -N pg -D C:\pg\data -w`,
			wantPgCtl: true,
			want:      pgCtlCommand{base: `C:\pg`, data: `C:\pg\data`},
		},
		{
			name:      "-D joined",
			pathName:  `C:\pg\bin\pg_ctl.exe runservice -DC:\pg\data`,
			wantPgCtl: true,
			want:      pgCtlCommand{base: `C:\pg`, data: `C:\pg\data`},
		},
		{
			name:      "executable name in upper case",
			pathName:  `C:\PG\BIN\PG_CTL.EXE runservice -D C:\pg\data`,
			wantPgCtl: true,
			want:      pgCtlCommand{base: `C:\PG`, data: `C:\pg\data`},
		},
		{
			name:      "no -D",
			pathName:  `"C:\pg\bin\pg_ctl.exe" runservice -N "pg" -w`,
			wantPgCtl: true,
			want:      pgCtlCommand{base: `C:\pg`},
		},
		{
			name:      "the bin folder in upper case",
			pathName:  `C:\pg\BIN\pg_ctl.exe runservice -D C:\pg\data`,
			wantPgCtl: true,
			want:      pgCtlCommand{base: `C:\pg`, data: `C:\pg\data`},
		},
		{
			name:      "pg_ctl.exe outside a bin folder has no base",
			pathName:  `C:\pg\pg_ctl.exe runservice -D C:\pg\data`,
			wantPgCtl: true,
			want:      pgCtlCommand{data: `C:\pg\data`},
		},
		{
			name:      "a bare pg_ctl.exe has no base",
			pathName:  `pg_ctl.exe runservice -D C:\pg\data`,
			wantPgCtl: true,
			want:      pgCtlCommand{data: `C:\pg\data`},
		},
		{
			name:      `-o "-p 0x1538"`,
			pathName:  `C:\pg\bin\pg_ctl.exe runservice -D C:\pg\data -o "-p 0x1538"`,
			wantPgCtl: true,
			want:      pgCtlCommand{base: `C:\pg`, data: `C:\pg\data`, port: 5432},
		},
		{
			name:      `-o "-p 5433"`,
			pathName:  `C:\pg\bin\pg_ctl.exe runservice -D C:\pg\data -o "-p 5433"`,
			wantPgCtl: true,
			want:      pgCtlCommand{base: `C:\pg`, data: `C:\pg\data`, port: 5433},
		},
		{
			name:      `-o "--port=5433"`,
			pathName:  `C:\pg\bin\pg_ctl.exe runservice -D C:\pg\data -o "--port=5433"`,
			wantPgCtl: true,
			want:      pgCtlCommand{base: `C:\pg`, data: `C:\pg\data`, port: 5433},
		},
		{
			name:      `-o "-c port=5433"`,
			pathName:  `C:\pg\bin\pg_ctl.exe runservice -D C:\pg\data -o "-c port=5433"`,
			wantPgCtl: true,
			want:      pgCtlCommand{base: `C:\pg`, data: `C:\pg\data`, port: 5433},
		},
		{
			name:      "--options= with another setting before the port",
			pathName:  `C:\pg\bin\pg_ctl.exe runservice -D C:\pg\data "--options=-c shared_buffers=1GB -p5433"`,
			wantPgCtl: true,
			want:      pgCtlCommand{base: `C:\pg`, data: `C:\pg\data`, port: 5433},
		},
		{
			name:      "the last of two -o ports wins",
			pathName:  `C:\pg\bin\pg_ctl.exe runservice -D C:\pg\data -o "-p 5433" -o "-c PORT=5434"`,
			wantPgCtl: true,
			want:      pgCtlCommand{base: `C:\pg`, data: `C:\pg\data`, port: 5434},
		},
		{
			name:      "-o without a port",
			pathName:  `C:\pg\bin\pg_ctl.exe runservice -D C:\pg\data -o "-c shared_buffers=1GB"`,
			wantPgCtl: true,
			want:      pgCtlCommand{base: `C:\pg`, data: `C:\pg\data`},
		},
		{
			name:        "-o with a port that is not a number",
			pathName:    `C:\pg\bin\pg_ctl.exe runservice -D C:\pg\data -o "-p five"`,
			wantPgCtl:   true,
			want:        pgCtlCommand{base: `C:\pg`, data: `C:\pg\data`},
			wantPortErr: `set port "five", not a port number`,
		},
		{name: "a non-pg_ctl service", pathName: `"C:\Scada\scada.exe" runservice -D C:\pg\data`},
		{name: "register instead of runservice", pathName: `C:\pg\bin\pg_ctl.exe register -D C:\pg\data`},
		{name: "pg_ctl in another file name", pathName: `C:\pg\bin\pg_ctl.exe.bak runservice -D C:\pg\data`},
		{name: "an empty command line", pathName: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, isPgCtl := parsePgCtlCommand(tt.pathName)
			if isPgCtl != tt.wantPgCtl {
				t.Fatalf("isPgCtl = %v, want %v", isPgCtl, tt.wantPgCtl)
			}
			gotErr := got.portErr
			got.portErr = nil
			if got != tt.want {
				t.Errorf("parsePgCtlCommand() = %+v, want %+v", got, tt.want)
			}
			switch {
			case tt.wantPortErr == "" && gotErr != nil:
				t.Errorf("port error = %v, want nil", gotErr)
			case tt.wantPortErr != "" && (gotErr == nil || !strings.Contains(gotErr.Error(), tt.wantPortErr)):
				t.Errorf("port error = %v, want one containing %q", gotErr, tt.wantPortErr)
			}
		})
	}
}

// Detect only queries, so it runs against the real service list of the test machine. Only on
// CI is that list known: there postgresql-x64-17 is the service found. Elsewhere a second 17 or
// an unqueryable postgresql* service is a legitimate other state.
func TestDetectReadsTheServiceList(t *testing.T) {
	result, err := Detect(17)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	t.Logf("Detect(17) = %+v", result)
	if result.State == "" || result.Detail == "" {
		t.Errorf("Detect() = %+v, want a state and a detail", result)
	}
	if os.Getenv("GITHUB_ACTIONS") == "" || !hasService(t, "postgresql-x64-17") {
		return
	}
	if result.State != StateFound || result.Service.Name != "postgresql-x64-17" {
		t.Errorf("Detect() = state %q, service %q, want found, postgresql-x64-17 (detail %q)",
			result.State, result.Service.Name, result.Detail)
	}
}

func hasService(t *testing.T, name string) bool {
	t.Helper()
	handle, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		t.Fatalf("connecting to the service control manager: %v", err)
	}
	defer func() { _ = windows.CloseServiceHandle(handle) }()
	utf16Name, err := windows.UTF16PtrFromString(name)
	if err != nil {
		t.Fatal(err)
	}
	service, err := windows.OpenService(handle, utf16Name, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return false
	}
	_ = windows.CloseServiceHandle(service)
	return true
}
