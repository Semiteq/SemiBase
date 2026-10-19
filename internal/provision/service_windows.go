//go:build windows

package provision

import (
	"context"
	"errors"
	"fmt"
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
}

type serviceTimings struct {
	// a shutdown checkpoint with max_wal_size = 8GB can be long
	stop  time.Duration
	start time.Duration
	poll  time.Duration
}

func defaultServiceTimings() serviceTimings {
	return serviceTimings{stop: 3 * time.Minute, start: 2 * time.Minute, poll: 500 * time.Millisecond}
}

type managedService struct {
	name    string
	control serviceControl
	timings serviceTimings
	release func()
}

// openService refuses a service the restart could not complete: missing, out of reach,
// Disabled, or not running once a start in progress has finished.
func openService(ctx context.Context, name string) (*managedService, error) {
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
	if err := refuseDisabled(handle, name); err != nil {
		service.close()
		return nil, err
	}
	if err := service.awaitRunning(ctx); err != nil {
		service.close()
		return nil, err
	}
	return service, nil
}

func refuseDisabled(handle *mgr.Service, name string) error {
	config, err := handle.Config()
	if err != nil {
		return fmt.Errorf("reading the configuration of service %s: %w", name, err)
	}
	if config.StartType == mgr.StartDisabled {
		return fmt.Errorf("service %s is Disabled, so it could not be started again after the stop; "+
			"set its start type to Automatic or Manual", name)
	}
	return nil
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

// restart detaches from the command's cancellation once the Stop is accepted: the Stopped wait,
// the Start and the Running wait each run to their own bound, so an interrupt or the command
// timeout does not leave the service stopped. Residual cases: docs/architecture/provisioning.md.
func (s *managedService) restart(ctx context.Context) error {
	if _, err := s.control.Control(svc.Stop); err != nil {
		return fmt.Errorf("stopping service %s: %w", s.name, err)
	}
	detached := context.WithoutCancel(ctx)
	if err := s.awaitStopped(detached); err != nil {
		return err
	}
	ok("service %s stopped", s.name)
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

func (s *managedService) awaitStopped(ctx context.Context) error {
	waitCtx, cancel := context.WithTimeout(ctx, s.timings.stop)
	defer cancel()
	state, err := s.pollUntil(waitCtx, func(state svc.State) bool { return state == svc.Stopped })
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

// awaitStarted fails at once on Stopped from the second query on: a postmaster that cannot start
// on the new settings exits, and the service falls back to Stopped.
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
		return fmt.Errorf("service %s stopped again after the Start: the server did not start on the "+
			"new settings, which stay in postgresql.auto.conf. Read the server log in the data "+
			"directory's log folder and the Windows Application event log", s.name)
	}
	return nil
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
