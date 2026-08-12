package provision

import (
	"fmt"
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

// EnableColors switches the Windows console into virtual-terminal mode so ANSI
// colors render. Best effort: a redirected or legacy console stays plain.
func EnableColors() {
	handle := windows.Handle(os.Stdout.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return
	}
	_ = windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
}

func step(format string, args ...any) {
	fmt.Printf("\n%s== %s ==%s\n", colorWhite, fmt.Sprintf(format, args...), colorReset)
}

func ok(format string, args ...any) {
	fmt.Printf("  %s[ OK ]%s %s\n", colorGreen, colorReset, fmt.Sprintf(format, args...))
}

func warn(format string, args ...any) {
	fmt.Printf("  %s[WARN]%s %s\n", colorYellow, colorReset, fmt.Sprintf(format, args...))
}

func note(format string, args ...any) {
	fmt.Printf("  %s[NOTE]%s %s\n", colorCyan, colorReset, fmt.Sprintf(format, args...))
}
