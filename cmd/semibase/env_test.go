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
		"SEMIBASE_TEST_SPACED = padded ",
		"SEMIBASE_TEST_PRESET=from-file",
		"not-a-pair",
		"=no-name",
	}, "\n"))

	t.Setenv("SEMIBASE_TEST_PRESET", "from-process")
	for _, name := range []string{"SEMIBASE_TEST_PLAIN", "SEMIBASE_TEST_QUOTED", "SEMIBASE_TEST_SPACED"} {
		t.Setenv(name, "")
		os.Unsetenv(name)
	}

	applyEnv(content)

	tests := []struct {
		name string
		want string
	}{
		{"SEMIBASE_TEST_PLAIN", "secret"},
		{"SEMIBASE_TEST_QUOTED", "qu oted"},
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
