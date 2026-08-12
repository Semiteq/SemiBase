package provision

import (
	"bytes"
	"strings"
	"testing"
)

// The console helpers gate on package-level state (colorsEnabled,
// consoleWriter), so these tests never run in parallel and restore the state
// they mutate.

func captureConsole(t *testing.T, enabled bool, emit func()) string {
	t.Helper()
	previousEnabled := colorsEnabled
	previousWriter := consoleWriter
	t.Cleanup(func() {
		colorsEnabled = previousEnabled
		consoleWriter = previousWriter
	})
	var buffer bytes.Buffer
	colorsEnabled = enabled
	consoleWriter = &buffer
	emit()
	return buffer.String()
}

func printAllHelpers() {
	step("phase %d", 1)
	ok("done %s", "fine")
	warn("watch %s", "out")
	note("see %s", "docs")
}

func TestHelpersColorsDisabled(t *testing.T) {
	output := captureConsole(t, false, printAllHelpers)
	if strings.Contains(output, "\x1b") {
		t.Errorf("colors disabled: output contains escape bytes: %q", output)
	}
	for _, want := range []string{"== phase 1 ==", "[ OK ] done fine", "[WARN] watch out", "[NOTE] see docs"} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q: %q", want, output)
		}
	}
}

// Force-enables the package flag directly: EnableColors cannot succeed under
// go test because stdout is redirected.
func TestHelpersColorsEnabled(t *testing.T) {
	output := captureConsole(t, true, printAllHelpers)
	if !strings.Contains(output, "\x1b[") {
		t.Errorf("colors enabled: output has no escape sequences: %q", output)
	}
	if !strings.Contains(output, colorReset) {
		t.Errorf("colors enabled: output has no reset sequence: %q", output)
	}
}

func TestEnableColorsNoColorWins(t *testing.T) {
	previousEnabled := colorsEnabled
	t.Cleanup(func() {
		colorsEnabled = previousEnabled
	})
	t.Setenv("NO_COLOR", "1")
	colorsEnabled = true
	EnableColors()
	if colorsEnabled {
		t.Error("NO_COLOR is set but EnableColors left colors enabled")
	}
}
