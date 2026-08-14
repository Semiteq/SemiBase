package main

import (
	"os"
	"strings"
	"testing"
)

func TestApplyEnv(t *testing.T) {
	content := strings.NewReader(strings.Join([]string{
		"# comment",
		"",
		"SEMIBASE_TEST_PLAIN=secret",
		"SEMIBASE_TEST_QUOTED=\"qu oted\"",
		"SEMIBASE_TEST_TRAILING_QUOTE=abc'",
		"SEMIBASE_TEST_LEADING_QUOTE=\"abc",
		"SEMIBASE_TEST_SPACED = padded ",
		"SEMIBASE_TEST_PRESET=from-file",
		"not-a-pair",
		"=no-name",
	}, "\n"))

	t.Setenv("SEMIBASE_TEST_PRESET", "from-process")
	unsetEnvForTest(t,
		"SEMIBASE_TEST_PLAIN",
		"SEMIBASE_TEST_QUOTED",
		"SEMIBASE_TEST_TRAILING_QUOTE",
		"SEMIBASE_TEST_LEADING_QUOTE",
		"SEMIBASE_TEST_SPACED",
	)

	if err := applyEnv(content); err != nil {
		t.Fatalf("applyEnv returned %v, want nil", err)
	}

	tests := []struct {
		name string
		want string
	}{
		{"SEMIBASE_TEST_PLAIN", "secret"},
		{"SEMIBASE_TEST_QUOTED", "qu oted"},
		{"SEMIBASE_TEST_TRAILING_QUOTE", "abc'"},
		{"SEMIBASE_TEST_LEADING_QUOTE", "\"abc"},
		{"SEMIBASE_TEST_SPACED", "padded"},
		{"SEMIBASE_TEST_PRESET", "from-process"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := os.Getenv(tt.name); got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestLoadDotEnvAbsentFileIsSilent(t *testing.T) {
	t.Chdir(t.TempDir())
	buffer := captureErrorOutput(t)

	loadDotEnv()

	if got := buffer.String(); got != "" {
		t.Errorf("loadDotEnv wrote %q for an absent file, want no output", got)
	}
}

func TestLoadDotEnvDirectoryReportsReadError(t *testing.T) {
	t.Chdir(t.TempDir())
	// On Windows os.Open succeeds on a directory; the read fails, exercising
	// the scanner.Err() warning path.
	if err := os.Mkdir(dotEnvName, 0o755); err != nil {
		t.Fatal(err)
	}
	buffer := captureErrorOutput(t)

	loadDotEnv()

	got := buffer.String()
	if !strings.Contains(got, "warning:") || !strings.Contains(got, dotEnvName) {
		t.Errorf("loadDotEnv output = %q, want a warning naming %s", got, dotEnvName)
	}
}
