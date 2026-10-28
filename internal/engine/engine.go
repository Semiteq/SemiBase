// Package engine finds the PostgreSQL server a Windows service runs and classifies the set of
// such services for Setup.
package engine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultPort = 5432

// State is Setup's view of the pg_ctl services on the machine.
type State string

const (
	StateAmbiguous    State = "ambiguous"
	StateIncomplete   State = "incomplete"
	StateUnreadable   State = "unreadable"
	StatePortMismatch State = "port-mismatch"
	StateFound        State = "found"
	StateOtherMajor   State = "other-major"
	StateFresh        State = "fresh"
)

// Service is one pg_ctl service and what its data folder says about the server it runs.
type Service struct {
	Name string
	Base string
	Data string
	// Major is 0 when PG_VERSION is missing or unreadable.
	Major   int
	Running bool
	Port    int
	// PortFile is the configuration file that set Port; empty when the command line or the default did.
	PortFile          string
	PortOnCommandLine bool
	// Listening is the port in postmaster.pid, read only when Running.
	Listening   int
	Uninstaller bool
	// Problem is the detail line of a service that points at no PostgreSQL install Setup can place.
	Problem string
	// Unreadable is what Setup could not read of a candidate service.
	Unreadable string
}

// Result is the classification of all pg_ctl services against the expected major.
type Result struct {
	State State
	// Service is the one candidate in the incomplete, unreadable, port-mismatch and found states.
	Service Service
	// Services names the candidates in the ambiguous state.
	Services []string
	Detail   string
}

// pgCtlCommand is what a pg_ctl.exe runservice command line says about the server it starts.
type pgCtlCommand struct {
	base string
	data string
	// port is the port the -o options pass to the server, 0 when they pass none.
	port    int
	portErr error
}

// describe reads the data folder that a pg_ctl service command line names.
func describe(name string, command pgCtlCommand, running bool) Service {
	service := Service{Name: name, Base: command.base, Data: command.data, Running: running}
	if command.base != "" {
		_, err := os.Stat(filepath.Join(command.base, "uninstall-postgresql.exe"))
		service.Uninstaller = err == nil
	}
	if command.data == "" {
		service.Problem = fmt.Sprintf("service %s runs pg_ctl.exe with no -D", name)
		return service
	}
	major, err := readMajor(command.data)
	if err != nil {
		service.Problem = fmt.Sprintf("service %s: %v", name, err)
		return service
	}
	service.Major = major
	switch {
	case command.portErr != nil:
		service.Unreadable = command.portErr.Error()
		return service
	case command.port > 0:
		service.Port, service.PortOnCommandLine = command.port, true
	default:
		service.Port, service.PortFile, err = configuredPort(command.data)
		if err != nil {
			service.Unreadable = err.Error()
			return service
		}
	}
	if running {
		service.Listening, err = listeningPort(command.data)
		if err != nil {
			service.Unreadable = fmt.Sprintf("no listening port while the service runs: %v", err)
		}
	}
	return service
}

// collectServices reads each listed service. A service whose name starts with postgresql in any
// letter case and that cannot be queried is kept as a candidate Setup cannot read, so Setup never
// installs a second server beside it.
func collectServices(names []string, read func(name string) (Service, bool, error)) []Service {
	var services []Service
	for _, name := range names {
		service, isPgCtl, err := read(name)
		switch {
		case err != nil:
			if strings.HasPrefix(strings.ToLower(name), "postgresql") {
				services = append(services,
					Service{Name: name, Unreadable: strings.TrimSuffix(err.Error(), ".")})
			}
		case isPgCtl:
			services = append(services, service)
		}
	}
	return services
}

// readMajor returns the major version on the first line of <data>\PG_VERSION: the whole line from
// PostgreSQL 10 on, the part before the dot of the N.M that 9.6 and earlier write.
func readMajor(data string) (int, error) {
	path := filepath.Join(data, "PG_VERSION")
	content, err := readFile(path)
	if err != nil {
		return 0, err
	}
	firstLine, _, _ := strings.Cut(string(content), "\n")
	version := strings.TrimSpace(firstLine)
	majorPart, minorPart, hasMinor := strings.Cut(version, ".")
	if !isDigits(majorPart) || hasMinor && !isDigits(minorPart) {
		return 0, fmt.Errorf("%s holds %q, not a major version", path, version)
	}
	major, err := strconv.Atoi(majorPart)
	if err != nil || major <= 0 {
		return 0, fmt.Errorf("%s holds %q, not a major version", path, version)
	}
	return major, nil
}

func isDigits(text string) bool {
	return text != "" && strings.Trim(text, "0123456789") == ""
}

// listeningPort returns the port on line 4 of <data>\postmaster.pid.
func listeningPort(data string) (int, error) {
	path := filepath.Join(data, "postmaster.pid")
	content, err := readFile(path)
	if err != nil {
		return 0, err
	}
	lines := strings.Split(string(content), "\n")
	if len(lines) < 4 {
		return 0, fmt.Errorf("%s has no line 4", path)
	}
	port, err := strconv.Atoi(strings.TrimSpace(lines[3]))
	if err != nil || port <= 0 {
		return 0, fmt.Errorf("%s holds %q on line 4, not a port", path, strings.TrimSpace(lines[3]))
	}
	return port, nil
}

// parsePort reads a port setting as PostgreSQL's parse_int does, strtol with base 0, so 0x1538
// and the octal 012470 are ports too.
func parsePort(value string) (int, bool) {
	trimmed := strings.TrimSpace(value)
	lower := strings.ToLower(trimmed)
	// strconv's base 0 also takes Go's 0b, 0o and _ forms, which strtol does not
	if strings.Contains(lower, "_") || strings.HasPrefix(strings.TrimLeft(lower, "+-"), "0b") ||
		strings.HasPrefix(strings.TrimLeft(lower, "+-"), "0o") {
		return 0, false
	}
	port, err := strconv.ParseInt(trimmed, 0, 32)
	if err != nil || port < 1 || port > 65535 {
		return 0, false
	}
	return int(port), true
}

func readFile(path string) ([]byte, error) {
	content, err := os.ReadFile(path) //nolint:gosec // the paths come from the service list by design
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("no file %s", path)
	}
	if err != nil {
		if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
			err = pathErr.Err
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return content, nil
}

// Classify applies the state table: the first matching row wins. A candidate is a service whose
// major is expectedMajor or unknown.
func Classify(services []Service, expectedMajor int) Result {
	var candidates []Service
	for _, service := range services {
		if service.Major == expectedMajor || service.Major == 0 {
			candidates = append(candidates, service)
		}
	}
	switch {
	case len(candidates) > 1:
		names := make([]string, len(candidates))
		for i, candidate := range candidates {
			names[i] = candidate.Name
		}
		return Result{
			State:    StateAmbiguous,
			Services: names,
			Detail: fmt.Sprintf("services %s run PostgreSQL %d or a server of unknown major",
				strings.Join(names, ", "), expectedMajor),
		}
	case len(candidates) == 1:
		return classifyCandidate(candidates[0])
	case len(services) > 0:
		described := make([]string, len(services))
		for i, service := range services {
			described[i] = fmt.Sprintf("%s runs PostgreSQL %d", service.Name, service.Major)
		}
		return Result{
			State: StateOtherMajor,
			Detail: fmt.Sprintf("no service runs PostgreSQL %d; %s",
				expectedMajor, strings.Join(described, ", ")),
		}
	default:
		return Result{State: StateFresh, Detail: "no service runs pg_ctl.exe runservice"}
	}
}

func classifyCandidate(service Service) Result {
	switch {
	case service.Problem != "":
		return Result{State: StateIncomplete, Service: service, Detail: service.Problem}
	case service.Unreadable != "":
		return Result{
			State:   StateUnreadable,
			Service: service,
			Detail:  fmt.Sprintf("service %s: %s", service.Name, service.Unreadable),
		}
	case service.Running && service.Listening != service.Port:
		return Result{
			State:   StatePortMismatch,
			Service: service,
			Detail: fmt.Sprintf("service %s listens on port %d, and %s sets port %d",
				service.Name, service.Listening, portSourceName(service), service.Port),
		}
	default:
		return Result{
			State:   StateFound,
			Service: service,
			Detail: fmt.Sprintf("service %s runs PostgreSQL %d from %s on port %d",
				service.Name, service.Major, service.Data, service.Port),
		}
	}
}

func portSourceName(service Service) string {
	switch {
	case service.PortOnCommandLine:
		return "the service command line"
	case service.PortFile == "":
		return "the default"
	default:
		return service.PortFile
	}
}

// maxIncludeDepth is PostgreSQL's CONF_FILE_MAX_DEPTH.
const maxIncludeDepth = 10

// configuredPort returns the last active port setting of postgresql.conf, its include files and
// postgresql.auto.conf in the order PostgreSQL reads them, and the file that set it. With no
// setting it returns defaultPort and an empty file.
func configuredPort(data string) (port int, file string, err error) {
	reader := confReader{port: defaultPort}
	if err := reader.read(filepath.Join(data, "postgresql.conf"), 0); err != nil {
		return 0, "", err
	}
	auto := filepath.Join(data, "postgresql.auto.conf")
	if err := reader.readIfExists(auto, 0); err != nil {
		return 0, "", err
	}
	return reader.port, reader.file, nil
}

type confReader struct {
	port int
	file string
}

func (r *confReader) readIfExists(path string, depth int) error {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return r.read(path, depth)
}

func (r *confReader) read(path string, depth int) error {
	if depth > maxIncludeDepth {
		return fmt.Errorf("%s: includes nest deeper than %d files", path, maxIncludeDepth)
	}
	content, err := readFile(path)
	if err != nil {
		return err
	}
	for number, line := range strings.Split(string(content), "\n") {
		name, value, ok, err := parseConfLine(line)
		if err != nil {
			return fmt.Errorf("%s line %d: %w", path, number+1, err)
		}
		if !ok {
			continue
		}
		if err := r.apply(path, number+1, name, value, depth); err != nil {
			return err
		}
	}
	return nil
}

func (r *confReader) apply(path string, number int, name, value string, depth int) error {
	resolve := func(target string) string {
		if filepath.IsAbs(target) {
			return target
		}
		return filepath.Join(filepath.Dir(path), target)
	}
	switch strings.ToLower(name) {
	case "include":
		return r.read(resolve(value), depth+1)
	case "include_if_exists":
		return r.readIfExists(resolve(value), depth+1)
	case "include_dir":
		return r.readDir(resolve(value), depth+1)
	case "port":
		port, ok := parsePort(value)
		if !ok {
			return fmt.Errorf("%s line %d: port %q is not a port number", path, number, value)
		}
		r.port, r.file = port, path
	}
	return nil
}

func (r *confReader) readDir(dir string, depth int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("no folder %s", dir)
		}
		return fmt.Errorf("reading folder %s: %w", dir, err)
	}
	// os.ReadDir sorts by name, the order PostgreSQL reads an include_dir in
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".conf") {
			continue
		}
		if err := r.read(filepath.Join(dir, name), depth); err != nil {
			return err
		}
	}
	return nil
}

// parseConfLine splits one postgresql.conf line into a setting name and its value; ok is false
// for a blank or comment line.
func parseConfLine(line string) (name, value string, ok bool, err error) {
	rest := strings.TrimLeft(line, " \t\r\f")
	if rest == "" || rest[0] == '#' {
		return "", "", false, nil
	}
	end := strings.IndexAny(rest, " \t\r\f=#'")
	if end < 0 {
		return "", "", false, errors.New("syntax error: a setting with no value")
	}
	name = rest[:end]
	rest = strings.TrimLeft(rest[end:], " \t\r\f")
	if strings.HasPrefix(rest, "=") {
		rest = strings.TrimLeft(rest[1:], " \t\r\f")
	}
	switch {
	case name == "":
		return "", "", false, errors.New("syntax error: a line with no setting name")
	case rest == "" || rest[0] == '#':
		return "", "", false, fmt.Errorf("syntax error: setting %s has no value", name)
	case rest[0] == '\'':
		value, rest, err = scanQuoted(rest)
		if err != nil {
			return "", "", false, err
		}
	default:
		end = strings.IndexAny(rest, " \t\r\f#")
		if end < 0 {
			end = len(rest)
		}
		value, rest = rest[:end], rest[end:]
	}
	rest = strings.TrimLeft(rest, " \t\r\f")
	if rest != "" && rest[0] != '#' {
		return "", "", false, fmt.Errorf("syntax error after the value of %s", name)
	}
	return name, value, true, nil
}

// scanQuoted reads a quoted value: a doubled quote is a quote, and a backslash takes the next
// character as it is. PostgreSQL's GUC_scanstr also decodes \n, \t and octal escapes, which no
// port or include path this reader looks at needs.
func scanQuoted(text string) (value, rest string, err error) {
	var builder strings.Builder
	for i := 1; i < len(text); i++ {
		switch c := text[i]; {
		case c == '\'' && i+1 < len(text) && text[i+1] == '\'':
			builder.WriteByte('\'')
			i++
		case c == '\'':
			return builder.String(), text[i+1:], nil
		case c == '\\' && i+1 < len(text):
			i++
			builder.WriteByte(text[i])
		case c == '\n':
			return "", "", errors.New("syntax error: an unterminated quoted value")
		default:
			builder.WriteByte(c)
		}
	}
	return "", "", errors.New("syntax error: an unterminated quoted value")
}
