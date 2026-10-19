//go:build windows

package provision

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"
)

// fakeService answers Query with states in order and repeats the last one.
type fakeService struct {
	states     []svc.State
	queries    int
	controlErr error
	startErr   error
	stops      int
	starts     int
}

func (f *fakeService) Query() (svc.Status, error) {
	state := f.states[min(f.queries, len(f.states)-1)]
	f.queries++
	return svc.Status{State: state}, nil
}

func (f *fakeService) Control(svc.Cmd) (svc.Status, error) {
	f.stops++
	return svc.Status{State: svc.StopPending}, f.controlErr
}

func (f *fakeService) Start(...string) error {
	f.starts++
	return f.startErr
}

func newFakeManagedService(fake *fakeService) *managedService {
	return &managedService{
		name:    "pg",
		control: fake,
		timings: serviceTimings{stop: 50 * time.Millisecond, start: 50 * time.Millisecond, poll: time.Millisecond},
		release: func() {},
	}
}

func cancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func expiredContext() context.Context {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	cancel()
	return ctx
}

var fullRestart = []svc.State{svc.StopPending, svc.StopPending, svc.Stopped, svc.StartPending, svc.Running}

func TestRestart(t *testing.T) {
	tests := []struct {
		name       string
		ctx        context.Context
		states     []svc.State
		controlErr error
		startErr   error
		wantStarts int
		wantErr    []string
		wantCause  error
	}{
		{name: "stops and starts", ctx: context.Background(), states: fullRestart, wantStarts: 1},
		{
			name: "an interrupt after the Stop still starts the service", ctx: cancelledContext(),
			states: fullRestart, wantStarts: 1, wantErr: []string{"restarted, then the run stopped"},
			wantCause: context.Canceled,
		},
		{
			name: "the command timeout after the Stop still starts the service", ctx: expiredContext(),
			states: fullRestart, wantStarts: 1, wantErr: []string{"restarted, then the run stopped"},
			wantCause: context.DeadlineExceeded,
		},
		{
			name: "a refused Stop changes nothing", ctx: context.Background(), states: []svc.State{svc.Running},
			controlErr: errors.New("access denied"), wantErr: []string{"stopping service pg"},
		},
		{
			name: "a stop past its bound says the service will stay stopped", ctx: context.Background(),
			states: []svc.State{svc.StopPending}, wantErr: []string{"will stay stopped", "Start-Service pg"},
			wantCause: context.DeadlineExceeded,
		},
		{
			name: "a refused Start says how to start it", ctx: context.Background(),
			states: []svc.State{svc.Stopped}, startErr: errors.New("start refused"), wantStarts: 1,
			wantErr: []string{"stays stopped", "Start-Service pg"},
		},
		{
			name: "a server that falls back to Stopped fails at once", ctx: context.Background(),
			states: []svc.State{svc.Stopped, svc.StartPending, svc.Stopped}, wantStarts: 1,
			wantErr: []string{"stopped again after the Start", "postgresql.auto.conf"},
		},
		{
			name: "Stopped right after the Start is given one more query", ctx: context.Background(),
			states: []svc.State{svc.Stopped, svc.Stopped, svc.Running}, wantStarts: 1,
		},
		{
			name: "a start past its bound names the last state", ctx: context.Background(),
			states: []svc.State{svc.Stopped, svc.StartPending}, wantStarts: 1,
			wantErr: []string{"did not reach running", "start pending"}, wantCause: context.DeadlineExceeded,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeService{states: tt.states, controlErr: tt.controlErr, startErr: tt.startErr}
			err := newFakeManagedService(fake).restart(tt.ctx)
			if fake.stops != 1 {
				t.Errorf("Stop sent %d times, want 1", fake.stops)
			}
			if fake.starts != tt.wantStarts {
				t.Errorf("Start sent %d times, want %d", fake.starts, tt.wantStarts)
			}
			assertServiceError(t, err, tt.wantErr, tt.wantCause)
		})
	}
}

func TestAwaitRunning(t *testing.T) {
	tests := []struct {
		name      string
		states    []svc.State
		wantErr   []string
		wantCause error
	}{
		{name: "running", states: []svc.State{svc.Running}},
		{name: "a start in progress is waited out", states: []svc.State{svc.StartPending, svc.StartPending, svc.Running}},
		{
			name: "a start past its bound is refused", states: []svc.State{svc.StartPending},
			wantErr: []string{"still start pending"}, wantCause: context.DeadlineExceeded,
		},
		{name: "stopped is refused", states: []svc.State{svc.Stopped}, wantErr: []string{"is stopped, not running"}},
		{name: "stopping is refused", states: []svc.State{svc.StopPending}, wantErr: []string{"is stop pending"}},
		{name: "paused is refused", states: []svc.State{svc.Paused}, wantErr: []string{"is paused"}},
		{
			name: "a start that fails is refused", states: []svc.State{svc.StartPending, svc.Stopped},
			wantErr: []string{"is stopped, not running"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeService{states: tt.states}
			err := newFakeManagedService(fake).awaitRunning(context.Background())
			assertServiceError(t, err, tt.wantErr, tt.wantCause)
			if fake.stops != 0 || fake.starts != 0 {
				t.Errorf("the check sent %d Stop and %d Start, want none", fake.stops, fake.starts)
			}
		})
	}
}

func assertServiceError(t *testing.T, err error, wantText []string, wantCause error) {
	t.Helper()
	if wantText == nil {
		if err != nil {
			t.Fatalf("error = %v, want nil", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("error = nil, want one containing %q", wantText)
	}
	for _, text := range wantText {
		if !strings.Contains(err.Error(), text) {
			t.Errorf("error = %q, want it to contain %q", err, text)
		}
	}
	if wantCause != nil && !errors.Is(err, wantCause) {
		t.Errorf("error = %v, want it to wrap %v", err, wantCause)
	}
}
