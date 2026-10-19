//go:build windows

package provision

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// serviceControl is the part of *mgr.Service the restart drives, so tests can script the states.
type serviceControl interface {
	Query() (svc.Status, error)
	Control(c svc.Cmd) (svc.Status, error)
	Start(args ...string) error
	Config() (mgr.Config, error)
}

type serviceTimings struct {
	// a shutdown checkpoint with max_wal_size = 8GB can be long
	stop  time.Duration
	start time.Duration
	poll  time.Duration
	// each accepted probe connection costs the server a process
	probe time.Duration
}

func defaultServiceTimings() serviceTimings {
	return serviceTimings{
		stop:  3 * time.Minute,
		start: 2 * time.Minute,
		poll:  500 * time.Millisecond,
		probe: time.Second,
	}
}

// Windows resends a SYN after an RST, so even a refused loopback connect can take seconds
const listenerDialTimeout = 15 * time.Second

type managedService struct {
	name    string
	control serviceControl
	// address is the IP:port the run's connection reached, which listening dials
	address string
	// listening reports whether a TCP connect to address succeeds; false with a nil error means
	// the connect was refused, so no postmaster listens there
	listening func(context.Context) (bool, error)
	timings   serviceTimings
	release   func()
}

// openService refuses a service the restart could not complete or that is not the server the run
// connects to: missing, out of reach, Disabled, not running once a start in progress has
// finished, with no -D in its PathName, or registered for another data directory than the one
// describe reads.
func openService(
	ctx context.Context,
	name string,
	describe func(context.Context) (runningServer, error),
) (*managedService, error) {
	manager, err := mgr.Connect()
	if err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return nil, fmt.Errorf("service %s: access to the service control manager denied; "+
				"run semibase as an administrator", name)
		}
		return nil, fmt.Errorf("service %s: connecting to the service control manager: %w", name, err)
	}
	handle, err := manager.OpenService(name)
	if err != nil {
		_ = manager.Disconnect()
		switch {
		case errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST):
			return nil, fmt.Errorf("service %s does not exist", name)
		case errors.Is(err, windows.ERROR_ACCESS_DENIED):
			return nil, fmt.Errorf("service %s: access denied; run semibase as an administrator", name)
		}
		return nil, fmt.Errorf("opening service %s: %w", name, err)
	}
	service := &managedService{
		name:    name,
		control: handle,
		timings: defaultServiceTimings(),
		release: func() {
			_ = handle.Close()
			_ = manager.Disconnect()
		},
	}
	if err := service.check(ctx, describe); err != nil {
		service.close()
		return nil, err
	}
	service.listening = dialListener(service.address)
	return service, nil
}

func (s *managedService) check(
	ctx context.Context,
	describe func(context.Context) (runningServer, error),
) error {
	config, err := s.control.Config()
	if err != nil {
		return fmt.Errorf("reading the configuration of service %s: %w", s.name, err)
	}
	if config.StartType == mgr.StartDisabled {
		return fmt.Errorf("service %s is Disabled, so it could not be started again after the stop; "+
			"set its start type to Automatic or Manual", s.name)
	}
	if err := s.awaitRunning(ctx); err != nil {
		return err
	}
	return s.confirmServes(ctx, config.BinaryPathName, describe)
}

// confirmServes ties the service to the server behind --host:--port through the data directory:
// stopping another service would leave it stopped, since the probe would keep finding the
// run's server answering.
func (s *managedService) confirmServes(
	ctx context.Context,
	pathName string,
	describe func(context.Context) (runningServer, error),
) error {
	serviceDirectory, found := dataDirectoryFlag(pathName)
	if !found {
		return fmt.Errorf("the PathName of service %s carries no -D, so site cannot tell which data "+
			"directory it runs; site restarts a service registered with pg_ctl register -D", s.name)
	}
	server, err := describe(ctx)
	if err != nil {
		return err
	}
	if !sameDataDirectory(serviceDirectory, server.dataDirectory) {
		return fmt.Errorf("service %s runs data directory %s, but the server on %s uses %s: "+
			"--restart-service names another server. Find the service whose PathName carries "+
			"-D %s with Get-CimInstance Win32_Service", s.name, serviceDirectory, server.address,
			server.dataDirectory, filepath.Clean(server.dataDirectory))
	}
	ok("service %s runs data directory %s, as the server on %s does", s.name, serviceDirectory,
		server.address)
	s.address = server.address
	return nil
}

// dataDirectoryFlag reads the -D argument of a service command line, as pg_ctl register writes it.
func dataDirectoryFlag(pathName string) (string, bool) {
	args, err := windows.DecomposeCommandLine(pathName)
	if err != nil {
		return "", false
	}
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

// sameDataDirectory compares the service's native path with the server's, which PostgreSQL
// reports with forward slashes.
func sameDataDirectory(servicePath, serverPath string) bool {
	return strings.EqualFold(filepath.Clean(servicePath), filepath.Clean(serverPath))
}

func dialListener(address string) func(context.Context) (bool, error) {
	return func(ctx context.Context) (bool, error) {
		dialer := net.Dialer{Timeout: listenerDialTimeout}
		conn, err := dialer.DialContext(ctx, "tcp", address)
		switch {
		case err == nil:
			_ = conn.Close()
			return true, nil
		case errors.Is(err, windows.WSAECONNREFUSED):
			return false, nil
		}
		return false, err
	}
}

// awaitRunning lets a start in progress finish: an installer may run site right after it started
// the service, and an EDB service reports Running only once the server accepts connections.
func (s *managedService) awaitRunning(ctx context.Context) error {
	waitCtx, cancel := context.WithTimeout(ctx, s.timings.start)
	defer cancel()
	state, err := s.pollUntil(waitCtx, func(state svc.State) bool { return state != svc.StartPending })
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("service %s is still start pending after %s: %w", s.name, s.timings.start, err)
	case err != nil:
		return err
	case state != svc.Running:
		return fmt.Errorf("service %s is %s, not running; site restarts a running service only",
			s.name, stateName(state))
	}
	return nil
}

func (s *managedService) close() {
	s.release()
}

// restart detaches from the command's cancellation once the Stop is accepted: the exit wait,
// the Start and the Running wait each run to their own bound, so an interrupt or the command
// timeout does not leave the service stopped. Residual cases: docs/architecture/provisioning.md.
func (s *managedService) restart(ctx context.Context) error {
	if _, err := s.control.Control(svc.Stop); err != nil {
		return fmt.Errorf("stopping service %s: %w", s.name, err)
	}
	detached := context.WithoutCancel(ctx)
	if err := s.awaitServerExit(detached); err != nil {
		return err
	}
	ok("service %s stopped, nothing answers on %s", s.name, s.address)
	if err := s.control.Start(); err != nil {
		return fmt.Errorf("starting service %s: %w; it stays stopped: start it with Start-Service %s",
			s.name, err, s.name)
	}
	if err := s.awaitStarted(detached); err != nil {
		return err
	}
	ok("service %s running", s.name)
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("service %s restarted, then the run stopped: %w", s.name, err)
	}
	return nil
}

// awaitServerExit gives Stopped and the closed port one bound from the Stop: pg_ctl reports
// Stopped after at most a minute even while the postmaster still shuts down, and a Start then
// would meet the old postmaster's lock file.
func (s *managedService) awaitServerExit(ctx context.Context) error {
	waitCtx, cancel := context.WithTimeout(ctx, s.timings.stop)
	defer cancel()
	if err := s.awaitStopped(waitCtx); err != nil {
		return err
	}
	return s.awaitListenerClosed(waitCtx)
}

func (s *managedService) awaitStopped(ctx context.Context) error {
	state, err := s.pollUntil(ctx, func(state svc.State) bool { return state == svc.Stopped })
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("service %s is still %s %s after the Stop and will stay stopped; once "+
			"Get-Service %s shows Stopped, start it with Start-Service %s: %w",
			s.name, stateName(state), s.timings.stop, s.name, s.name, err)
	case err != nil:
		return fmt.Errorf("%w after the Stop; check it with Get-Service %s and, if it is stopped, "+
			"start it with Start-Service %s", err, s.name, s.name)
	}
	return nil
}

func (s *managedService) awaitListenerClosed(ctx context.Context) error {
	answered := false
	var lastErr error
	for {
		answers, err := s.listening(ctx)
		if !answers && err == nil {
			return nil
		}
		// a dial cut short by the deadline says nothing about the server
		if ctx.Err() == nil {
			answered, lastErr = answers, err
		}
		select {
		case <-ctx.Done():
			return s.serverOutlivedStop(answered, lastErr, ctx.Err())
		case <-time.After(s.timings.probe):
		}
	}
}

func (s *managedService) serverOutlivedStop(answered bool, lastErr, cause error) error {
	observed := "the server still answers on " + s.address
	if !answered {
		observed = fmt.Sprintf("connecting to %s still fails without a refusal (%v)", s.address, lastErr)
	}
	return fmt.Errorf("service %s is stopped, but %s %s after the Stop: the old server may still be "+
		"shutting down, so site sends no Start and the service stays stopped. Once a connection to "+
		"%s is refused, start it with Start-Service %s: %w",
		s.name, observed, s.timings.stop, s.address, s.name, cause)
}

// awaitStarted fails at once on Stopped from the second query on: pg_ctl reports Stopped when the
// postmaster exits during its startup or when its own startup wait runs out.
func (s *managedService) awaitStarted(ctx context.Context) error {
	waitCtx, cancel := context.WithTimeout(ctx, s.timings.start)
	defer cancel()
	queries := 0
	state, err := s.pollUntil(waitCtx, func(state svc.State) bool {
		queries++
		return state == svc.Running || (state == svc.Stopped && queries > 1)
	})
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("service %s did not reach running within %s, last state %s: %w",
			s.name, s.timings.start, stateName(state), err)
	case err != nil:
		return err
	case state == svc.Stopped:
		return s.stoppedAfterStart(ctx)
	}
	return nil
}

func (s *managedService) stoppedAfterStart(ctx context.Context) error {
	answers, err := s.listening(ctx)
	switch {
	case answers:
		return fmt.Errorf("service %s stopped again after the Start, but the server answers on %s: "+
			"pg_ctl ran out of its startup wait (60 s unless the service's PathName sets -t) and "+
			"left the server running outside the service. Once it accepts connections, stop it "+
			"with pg_ctl stop -D <the -D in the PathName>, then start it with Start-Service %s",
			s.name, s.address, s.name)
	case err != nil:
		return fmt.Errorf("service %s stopped again after the Start, and checking %s failed: %w. "+
			"Read the server log in the data directory's log folder and the Windows Application "+
			"event log", s.name, s.address, err)
	}
	return fmt.Errorf("service %s stopped again after the Start, and nothing answers on %s: the "+
		"server exited during its startup. Read the server log in the data directory's log folder "+
		"and the Windows Application event log. If it names a setting this run wrote, remove that "+
		"line from postgresql.auto.conf, then start the service with Start-Service %s; a re-run "+
		"of site writes the line again and stops the service the same way until the cause is fixed",
		s.name, s.address, s.name)
}

// pollUntil queries the service until done accepts its state or ctx ends, and returns the last
// state it saw.
func (s *managedService) pollUntil(ctx context.Context, done func(svc.State) bool) (svc.State, error) {
	var last svc.State
	for {
		status, err := s.control.Query()
		if err != nil {
			return last, fmt.Errorf("querying service %s: %w", s.name, err)
		}
		last = status.State
		if done(last) {
			return last, nil
		}
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-time.After(s.timings.poll):
		}
	}
}

func stateName(state svc.State) string {
	switch state {
	case svc.Stopped:
		return "stopped"
	case svc.StartPending:
		return "start pending"
	case svc.StopPending:
		return "stop pending"
	case svc.Running:
		return "running"
	case svc.ContinuePending:
		return "continue pending"
	case svc.PausePending:
		return "pause pending"
	case svc.Paused:
		return "paused"
	}
	return fmt.Sprintf("state %d", state)
}
