//go:build windows

package engine

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// Detect reads the Windows service list with query-only rights, so it runs unelevated, and
// classifies its pg_ctl services. It controls no service.
func Detect(expectedMajor int) (Result, error) {
	handle, err := windows.OpenSCManager(nil, nil,
		windows.SC_MANAGER_CONNECT|windows.SC_MANAGER_ENUMERATE_SERVICE)
	if err != nil {
		return Result{}, fmt.Errorf("connecting to the service control manager: %w", err)
	}
	manager := &mgr.Mgr{Handle: handle}
	defer func() { _ = manager.Disconnect() }()
	names, err := manager.ListServices()
	if err != nil {
		return Result{}, fmt.Errorf("listing the services: %w", err)
	}
	services := collectServices(names, func(name string) (Service, bool, error) {
		return readService(handle, name)
	})
	return Classify(services, expectedMajor), nil
}

// readService reports isPgCtl false for a service whose command line does not run
// pg_ctl.exe runservice.
func readService(manager windows.Handle, name string) (service Service, isPgCtl bool, err error) {
	utf16Name, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return Service{}, false, err
	}
	handle, err := windows.OpenService(manager, utf16Name,
		windows.SERVICE_QUERY_CONFIG|windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return Service{}, false, fmt.Errorf("opening the service: %w", err)
	}
	opened := &mgr.Service{Name: name, Handle: handle}
	defer func() { _ = opened.Close() }()
	config, err := opened.Config()
	if err != nil {
		return Service{}, false, fmt.Errorf("reading the service configuration: %w", err)
	}
	command, isPgCtl := parsePgCtlCommand(config.BinaryPathName)
	if !isPgCtl {
		return Service{}, false, nil
	}
	status, err := opened.Query()
	if err != nil {
		return Service{}, false, fmt.Errorf("reading the service state: %w", err)
	}
	return describe(name, command, status.State == svc.Running), true, nil
}

// parsePgCtlCommand sets base only when pg_ctl.exe sits in a folder named bin, the layout whose
// parent holds an EDB uninstaller.
func parsePgCtlCommand(pathName string) (pgCtlCommand, bool) {
	args, err := windows.DecomposeCommandLine(pathName)
	if err != nil || len(args) == 0 || !strings.EqualFold(filepath.Base(args[0]), "pg_ctl.exe") ||
		!slices.Contains(args[1:], "runservice") {
		return pgCtlCommand{}, false
	}
	var command pgCtlCommand
	if bin := filepath.Dir(args[0]); strings.EqualFold(filepath.Base(bin), "bin") {
		command.base = filepath.Dir(bin)
	}
	command.data, _ = dataDirectoryArgument(args)
	for _, options := range optionsValues(args) {
		port, err := optionsPort(options)
		if err != nil {
			command.portErr = err
		} else if port > 0 {
			command.port = port
		}
	}
	return command, true
}

// DataDirectoryFlag reads the -D argument of a service command line, as pg_ctl register writes it.
func DataDirectoryFlag(pathName string) (string, bool) {
	args, err := windows.DecomposeCommandLine(pathName)
	if err != nil {
		return "", false
	}
	return dataDirectoryArgument(args)
}

func dataDirectoryArgument(args []string) (string, bool) {
	for i, arg := range args {
		switch {
		case arg == "-D" && i+1 < len(args):
			return args[i+1], true
		case strings.HasPrefix(arg, "-D") && len(arg) > len("-D"):
			return strings.TrimPrefix(arg, "-D"), true
		}
	}
	return "", false
}

// optionsValues returns the values of every -o and --options argument; pg_ctl joins them all
// into the server's command line.
func optionsValues(args []string) []string {
	var values []string
	for i, arg := range args {
		switch {
		case (arg == "-o" || arg == "--options") && i+1 < len(args):
			values = append(values, args[i+1])
		case strings.HasPrefix(arg, "--options="):
			values = append(values, strings.TrimPrefix(arg, "--options="))
		case strings.HasPrefix(arg, "-o") && len(arg) > len("-o"):
			values = append(values, strings.TrimPrefix(arg, "-o"))
		}
	}
	return values
}

// optionsPort returns the last port a server options string sets, 0 when it sets none.
func optionsPort(options string) (int, error) {
	// the placeholder program name keeps the first option out of CommandLineToArgv's
	// program-name rules
	args, err := windows.DecomposeCommandLine("postgres " + options)
	if err != nil {
		return 0, fmt.Errorf("server options %q: %w", options, err)
	}
	args = args[1:]
	port := ""
	for i, arg := range args {
		next := ""
		if i+1 < len(args) {
			next = args[i+1]
		}
		switch {
		case arg == "-p":
			port = next
		case strings.HasPrefix(arg, "-p"):
			port = strings.TrimPrefix(arg, "-p")
		case strings.HasPrefix(arg, "--port="):
			port = strings.TrimPrefix(arg, "--port=")
		case arg == "-c" && isPortSetting(next):
			_, port, _ = strings.Cut(next, "=")
		case strings.HasPrefix(arg, "-c") && isPortSetting(strings.TrimPrefix(arg, "-c")):
			_, port, _ = strings.Cut(strings.TrimPrefix(arg, "-c"), "=")
		}
	}
	if port == "" {
		return 0, nil
	}
	number, ok := parsePort(port)
	if !ok {
		return 0, fmt.Errorf("server options %q set port %q, not a port number", options, port)
	}
	return number, nil
}

func isPortSetting(setting string) bool {
	name, _, found := strings.Cut(setting, "=")
	return found && strings.EqualFold(name, "port")
}
