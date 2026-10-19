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

const (
	// a shutdown checkpoint with max_wal_size = 8GB can be long
	stopTimeout       = 3 * time.Minute
	startTimeout      = 2 * time.Minute
	statePollInterval = 500 * time.Millisecond
)

type managedService struct {
	name    string
	manager *mgr.Mgr
	handle  *mgr.Service
}

// openService refuses a service the restart could not complete: missing, out of reach,
// Disabled, or not running, which means it is not the server the run connects to.
func openService(name string) (*managedService, error) {
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
	service := &managedService{name: name, manager: manager, handle: handle}
	if err := service.checkRestartable(); err != nil {
		service.close()
		return nil, err
	}
	return service, nil
}

func (s *managedService) checkRestartable() error {
	config, err := s.handle.Config()
	if err != nil {
		return fmt.Errorf("reading the configuration of service %s: %w", s.name, err)
	}
	if config.StartType == mgr.StartDisabled {
		return fmt.Errorf("service %s is Disabled, so it could not be started again after the stop; "+
			"set its start type to Automatic or Manual", s.name)
	}
	status, err := s.handle.Query()
	if err != nil {
		return fmt.Errorf("querying service %s: %w", s.name, err)
	}
	if status.State != svc.Running {
		return fmt.Errorf("service %s is %s, not running, so it is not the server this run "+
			"connects to", s.name, stateName(status.State))
	}
	return nil
}

func (s *managedService) close() {
	_ = s.handle.Close()
	_ = s.manager.Disconnect()
}

// restart sends Start whenever Stop was accepted, detached from the command's cancellation,
// so an interrupt or a timeout never leaves the service stopped.
func (s *managedService) restart(ctx context.Context) error {
	if _, err := s.handle.Control(svc.Stop); err != nil {
		return fmt.Errorf("stopping service %s: %w", s.name, err)
	}
	stopErr := s.waitFor(ctx, svc.Stopped, stopTimeout)
	if stopErr == nil {
		ok("service %s stopped", s.name)
	}
	startErr := s.start(context.WithoutCancel(ctx))
	return errors.Join(stopErr, startErr)
}

// start first waits out a stop the caller abandoned: StartService refuses a stopping service.
func (s *managedService) start(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, startTimeout)
	defer cancel()
	if err := s.waitFor(ctx, svc.Stopped, startTimeout); err != nil {
		return err
	}
	if err := s.handle.Start(); err != nil {
		return fmt.Errorf("starting service %s: %w", s.name, err)
	}
	if err := s.waitFor(ctx, svc.Running, startTimeout); err != nil {
		return err
	}
	ok("service %s running", s.name)
	return nil
}

func (s *managedService) waitFor(ctx context.Context, want svc.State, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		status, err := s.handle.Query()
		if err != nil {
			return fmt.Errorf("querying service %s: %w", s.name, err)
		}
		if status.State == want {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("service %s did not reach %s within %s, last state %s: %w",
				s.name, stateName(want), timeout, stateName(status.State), ctx.Err())
		case <-time.After(statePollInterval):
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
