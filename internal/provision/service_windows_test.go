//go:build windows

package provision

import (
	"cmp"
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
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
	config            mgr.Config
	states            []svc.State
	probes            []probeResult
	queries           int
	probeCalls        int
	controlErr        error
	startErr          error
	stops             int
	starts            int
	probesAtStart     int
	startAfterRefusal bool
}

func (f *fakeService) Query() (svc.Status, error) {
	state := f.states[min(f.queries, len(f.states)-1)]
	f.queries++
	return svc.Status{State: state}, nil
}

func (f *fakeService) Config() (mgr.Config, error) {
	return f.config, nil
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
		address:   "[::1]:5432",
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
			wantErr:   []string{"still answers on [::1]:5432", "sends no Start", "Start-Service pg"},
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
			wantErr: []string{
				"stopped again after the Start", "nothing answers", "postgresql.auto.conf",
				"Start-Service pg", "writes the line again",
			},
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

const runnerPathName = `"C:\Program Files\PostgreSQL\17\bin\pg_ctl.exe" runservice -N "pg" -D "C:\PostgreSQL\17\data" -w`

var runnerServer = runningServer{dataDirectory: "C:/PostgreSQL/17/data", address: "[::1]:5432"}

func TestCheck(t *testing.T) {
	tests := []struct {
		name         string
		startType    uint32
		pathName     string
		states       []svc.State
		server       runningServer
		describeErr  error
		wantErr      []string
		wantCause    error
		wantDescribe bool
	}{
		{
			name: "the running service of the run's server", pathName: runnerPathName,
			states: []svc.State{svc.Running}, server: runnerServer, wantDescribe: true,
		},
		{
			name: "a start in progress is waited out", pathName: runnerPathName,
			states: []svc.State{svc.StartPending, svc.StartPending, svc.Running}, server: runnerServer,
			wantDescribe: true,
		},
		{
			name: "Disabled is refused", startType: mgr.StartDisabled, pathName: runnerPathName,
			states: []svc.State{svc.Running}, wantErr: []string{"is Disabled"},
		},
		{
			name: "a start past its bound is refused", pathName: runnerPathName,
			states: []svc.State{svc.StartPending}, wantErr: []string{"still start pending"},
			wantCause: context.DeadlineExceeded,
		},
		{
			name: "stopped is refused", pathName: runnerPathName, states: []svc.State{svc.Stopped},
			wantErr: []string{"is stopped, not running"},
		},
		{
			name: "stopping is refused", pathName: runnerPathName, states: []svc.State{svc.StopPending},
			wantErr: []string{"is stop pending"},
		},
		{
			name: "paused is refused", pathName: runnerPathName, states: []svc.State{svc.Paused},
			wantErr: []string{"is paused"},
		},
		{
			name: "a start that fails is refused", pathName: runnerPathName,
			states: []svc.State{svc.StartPending, svc.Stopped}, wantErr: []string{"is stopped, not running"},
		},
		{
			name: "a service with no -D is refused", pathName: `C:\Scada\scada.exe --service`,
			states: []svc.State{svc.Running}, wantErr: []string{"carries no -D"},
		},
		{
			name: "a service of another data directory is refused", pathName: runnerPathName,
			states: []svc.State{svc.Running}, wantDescribe: true,
			server: runningServer{dataDirectory: "C:/Program Files/PostgreSQL/14/data", address: "[::1]:5432"},
			wantErr: []string{
				`runs data directory C:\PostgreSQL\17\data`, "names another server",
				`-D C:\Program Files\PostgreSQL\14\data`,
			},
		},
		{
			name: "a failed read of the server is returned", pathName: runnerPathName,
			states: []svc.State{svc.Running}, describeErr: errors.New("connection refused"),
			wantDescribe: true, wantErr: []string{"connection refused"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeService{
				config: mgr.Config{StartType: cmp.Or(tt.startType, mgr.StartManual), BinaryPathName: tt.pathName},
				states: tt.states,
			}
			described := false
			describe := func(context.Context) (runningServer, error) {
				described = true
				return tt.server, tt.describeErr
			}
			service := newFakeManagedService(fake)
			service.address = ""
			err := service.check(context.Background(), describe)
			assertServiceError(t, err, tt.wantErr, tt.wantCause)
			if described != tt.wantDescribe {
				t.Errorf("described the server = %v, want %v", described, tt.wantDescribe)
			}
			if err == nil && service.address != tt.server.address {
				t.Errorf("address = %q, want the connection's %q", service.address, tt.server.address)
			}
			if fake.stops != 0 || fake.starts != 0 {
				t.Errorf("the check sent %d Stop and %d Start, want none", fake.stops, fake.starts)
			}
		})
	}
}

func TestDataDirectoryFlag(t *testing.T) {
	tests := []struct {
		name      string
		pathName  string
		want      string
		wantFound bool
	}{
		{
			name:     "pg_ctl register, quoted with spaces",
			pathName: `"C:\Program Files\PostgreSQL\14\bin\pg_ctl.exe" runservice -N "postgresql-14" -D "C:\Program Files\PostgreSQL\14\data" -w`,
			want:     `C:\Program Files\PostgreSQL\14\data`, wantFound: true,
		},
		{
			name:     "unquoted",
			pathName: `C:\pg\bin\pg_ctl.exe runservice -D C:\pg\data -w`,
			want:     `C:\pg\data`, wantFound: true,
		},
		{
			name:     "value attached to the flag",
			pathName: `C:\pg\bin\pg_ctl.exe runservice -DC:\pg\data`,
			want:     `C:\pg\data`, wantFound: true,
		},
		{name: "no -D", pathName: `"C:\pg\bin\pg_ctl.exe" runservice -N "pg" -w`},
		{name: "-D with no value", pathName: `C:\pg\bin\pg_ctl.exe runservice -D`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := dataDirectoryFlag(tt.pathName)
			if got != tt.want || found != tt.wantFound {
				t.Errorf("dataDirectoryFlag() = %q, %v, want %q, %v", got, found, tt.want, tt.wantFound)
			}
		})
	}
}

func TestSameDataDirectory(t *testing.T) {
	const server = "C:/PostgreSQL/17/data"
	tests := []struct {
		name        string
		servicePath string
		want        bool
	}{
		{name: "forward slashes on the server side", servicePath: `C:\PostgreSQL\17\data`, want: true},
		{name: "case", servicePath: `c:\postgresql\17\DATA`, want: true},
		{name: "trailing separator", servicePath: `C:\PostgreSQL\17\data\`, want: true},
		{name: "another directory", servicePath: `C:\Program Files\PostgreSQL\14\data`},
		{name: "a longer name with the same prefix", servicePath: `C:\PostgreSQL\17\data2`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sameDataDirectory(tt.servicePath, server); got != tt.want {
				t.Errorf("sameDataDirectory(%q, %q) = %v, want %v", tt.servicePath, server, got, tt.want)
			}
		})
	}
}

// The probe dials one literal address, so the other loopback family cannot decide the answer.
func TestDialListener(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "::1"} {
		t.Run(host+" answers, then refuses", func(t *testing.T) {
			listener, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
			if err != nil {
				t.Skipf("no %s listener on this machine: %v", host, err)
			}
			probe := dialListener(listener.Addr().String())
			if answers, err := probe(context.Background()); !answers || err != nil {
				t.Errorf("with a listener: answers = %v, error = %v, want true, nil", answers, err)
			}
			if err := listener.Close(); err != nil {
				t.Fatal(err)
			}
			if answers, err := probe(context.Background()); answers || err != nil {
				t.Errorf("after the listener closed: answers = %v, error = %v, want false, nil", answers, err)
			}
		})
	}
	t.Run("a connect error other than a refusal is not taken as gone", func(t *testing.T) {
		answers, err := dialListener("127.0.0.1:0")(context.Background())
		if answers || err == nil {
			t.Errorf("answers = %v, error = %v, want false and an error", answers, err)
		}
	})
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
