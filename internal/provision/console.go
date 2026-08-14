package provision

import (
	"fmt"
	"io"
	"os"
)

var consoleWriter io.Writer = os.Stdout

// SetConsoleOutput redirects the console helpers to writer and returns the
// previous writer. Tests outside the package use it to keep step banners out
// of their output; production code never calls it.
func SetConsoleOutput(writer io.Writer) io.Writer {
	previous := consoleWriter
	consoleWriter = writer
	return previous
}

func step(format string, args ...any) {
	fmt.Fprintf(consoleWriter, "\n== %s ==\n", fmt.Sprintf(format, args...))
}

func ok(format string, args ...any) {
	fmt.Fprintf(consoleWriter, "  [ OK ] %s\n", fmt.Sprintf(format, args...))
}

func warn(format string, args ...any) {
	fmt.Fprintf(consoleWriter, "  [WARN] %s\n", fmt.Sprintf(format, args...))
}

func note(format string, args ...any) {
	fmt.Fprintf(consoleWriter, "  [NOTE] %s\n", fmt.Sprintf(format, args...))
}
