package provision

import (
	"fmt"
	"io"
	"os"
)

var consoleWriter io.Writer = os.Stdout

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
