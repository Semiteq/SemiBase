//go:build windows

package provision

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"
)

type probeResult struct {
	answers bool
	err     error
}

var (
	answering = probeResult{answers: true}
	refusing  = probeResult{}
)

// fakeService answers Query with states and the listener probe with probes, each in order and
// repeating its last entry; no probes means a refused connect.
type fakeService struct {
	states     []svc.State
	probes     []probeResult
	queries    int
	probeCalls int
	controlErr error
	startErr   error
	stops      int
	starts     int
	// probe calls made before the Start, and whether the last of them saw the port refused
	probesAtStart     int
	startAfterRefusal bool
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
	f.probesAtStart = f.probeCalls
	f.startAfterRefusal = f.probeCalls > 0 && f.lastProbe() == refusing
	return f.startErr
}

func (f *fakeService) listening(context.Context) (bool, error) {
	f.probeCalls++
	result := f.lastProbe()
	return result.answers, result.err
}

func (f *fakeService) lastProbe() probeResult {
	if len(f.probes) == 0 {
		return refusing
	}
	return f.probes[min(f.probeCalls, len(f.probes))-1]
}

func newFakeManagedService(fake *fakeService) *managedService {
	return &managedService{
		name:      "pg",
		control:   fake,
		address:   "localhost:5432",
		listening: fake.listening,
		timings: serviceTimings{
			stop: 50 * time.Millisecond, start: 50 * time.Millisecond,
			poll: time.Millisecond, probe: time.Millisecond,
		},
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
		name              string
		ctx               context.Context
		states            []svc.State
		probes            []probeResult
		controlErr        error
		startErr          error
		wantStarts        int
		wantProbesAtStart int
		wantErr           []string
		wantNotErr        string
		wantCause         error
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
			name: "Stopped while the server still answers: Start waits for the refusal", ctx: context.Background(),
			states: fullRestart, probes: []probeResult{answering, answering, refusing},
			wantStarts: 1, wantProbesAtStart: 3,
		},
		{
			name: "a probe error is retried until the refusal", ctx: context.Background(),
			states: fullRestart, probes: []probeResult{{err: errors.New("i/o timeout")}, refusing},
			wantStarts: 1, wantProbesAtStart: 2,
		},
		{
			name: "a server that outlives the stop bound gets no Start", ctx: context.Background(),
			states: []svc.State{svc.Stopped}, probes: []probeResult{answering},
			wantErr:   []string{"still answers on localhost:5432", "sends no Start", "Start-Service pg"},
			wantCause: context.DeadlineExceeded,
		},
		{
			name: "a probe that never gets a refusal gets no Start", ctx: context.Background(),
			states: []svc.State{svc.Stopped}, probes: []probeResult{{err: errors.New("i/o timeout")}},
			wantErr:   []string{"without a refusal (i/o timeout)", "Start-Service pg"},
			wantCause: context.DeadlineExceeded,
		},
		{
			name: "a refused Start says how to start it", ctx: context.Background(),
			states: []svc.State{svc.Stopped}, startErr: errors.New("start refused"), wantStarts: 1,
			wantErr: []string{"stays stopped", "Start-Service pg"},
		},
		{
			name: "a server that exits during its startup fails at once", ctx: context.Background(),
			states: []svc.State{svc.Stopped, svc.StartPending, svc.Stopped}, wantStarts: 1,
			wantErr: []string{"stopped again after the Start", "nothing answers", "postgresql.auto.conf"},
		},
		{
			name: "a startup pg_ctl stopped waiting for is not blamed on the settings", ctx: context.Background(),
			states: []svc.State{svc.Stopped, svc.StartPending, svc.Stopped},
			probes: []probeResult{refusing, answering}, wantStarts: 1,
			wantErr:    []string{"stopped again after the Start", "outside the service", "pg_ctl stop -D"},
			wantNotErr: "postgresql.auto.conf",
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
			fake := &fakeService{
				states: tt.states, probes: tt.probes, controlErr: tt.controlErr, startErr: tt.startErr,
			}
			err := newFakeManagedService(fake).restart(tt.ctx)
			if fake.stops != 1 {
				t.Errorf("Stop sent %d times, want 1", fake.stops)
			}
			if fake.starts != tt.wantStarts {
				t.Errorf("Start sent %d times, want %d", fake.starts, tt.wantStarts)
			}
			if fake.starts > 0 && !fake.startAfterRefusal {
				t.Errorf("Start sent after %d probes, the last not a refusal", fake.probesAtStart)
			}
			if tt.wantProbesAtStart > 0 && fake.probesAtStart != tt.wantProbesAtStart {
				t.Errorf("Start sent after %d probes, want %d", fake.probesAtStart, tt.wantProbesAtStart)
			}
			assertServiceError(t, err, tt.wantErr, tt.wantCause)
			if tt.wantNotErr != "" && err != nil && strings.Contains(err.Error(), tt.wantNotErr) {
				t.Errorf("error = %q, want it not to contain %q", err, tt.wantNotErr)
			}
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

// localhost resolves to ::1 first on Windows, so the listener on 127.0.0.1 also proves that a
// refusal on one address does not hide an answer on the other
func TestDialListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	probe := dialListener(net.JoinHostPort("localhost", port))

	if answers, err := probe(context.Background()); !answers || err != nil {
		t.Errorf("with a listener: answers = %v, error = %v, want true, nil", answers, err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if answers, err := probe(context.Background()); answers || err != nil {
		t.Errorf("after the listener closed: answers = %v, error = %v, want false, nil", answers, err)
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
