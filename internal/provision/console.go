package provision

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/windows"
)

const (
	colorReset  = "\x1b[0m"
	colorWhite  = "\x1b[97m"
	colorGreen  = "\x1b[32m"
	colorYellow = "\x1b[33m"
	colorCyan   = "\x1b[36m"
)

var (
	colorsEnabled bool
	consoleWriter io.Writer = os.Stdout
)

// SetConsoleOutput redirects the console helpers to writer and returns the
// previous writer. Tests outside the package use it to keep step banners out
// of their output; production code never calls it.
func SetConsoleOutput(writer io.Writer) io.Writer {
	previous := consoleWriter
	consoleWriter = writer
	return previous
}

// EnableColors switches the Windows console into virtual-terminal mode so ANSI
// colors render, and records the result: a redirected or legacy console, a
// failed mode switch, or a set NO_COLOR variable keeps output plain.
func EnableColors() {
	colorsEnabled = false
	if _, noColor := os.LookupEnv("NO_COLOR"); noColor {
		return
	}
	handle := windows.Handle(os.Stdout.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return
	}
	if err := windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
		return
	}
	colorsEnabled = true
}

func color(code string) string {
	if !colorsEnabled {
		return ""
	}
	return code
}

func step(format string, args ...any) {
	fmt.Fprintf(consoleWriter, "\n%s== %s ==%s\n", color(colorWhite), fmt.Sprintf(format, args...), color(colorReset))
}

func ok(format string, args ...any) {
	fmt.Fprintf(consoleWriter, "  %s[ OK ]%s %s\n", color(colorGreen), color(colorReset), fmt.Sprintf(format, args...))
}

func warn(format string, args ...any) {
	fmt.Fprintf(consoleWriter, "  %s[WARN]%s %s\n", color(colorYellow), color(colorReset), fmt.Sprintf(format, args...))
}

func note(format string, args ...any) {
	fmt.Fprintf(consoleWriter, "  %s[NOTE]%s %s\n", color(colorCyan), color(colorReset), fmt.Sprintf(format, args...))
}
