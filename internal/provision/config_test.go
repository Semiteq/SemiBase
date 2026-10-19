package provision

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestRestartAdvice(t *testing.T) {
	const loopbackNote = "from this machine only"
	tests := []struct {
		name         string
		pending      []string
		wantLoopback bool
	}{
		{"shared_buffers alone", []string{"shared_buffers"}, false},
		{"listen_addresses pending", []string{"listen_addresses", "shared_buffers"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := restartAdvice(tt.pending)
			if !strings.Contains(got, "Restart the PostgreSQL service") {
				t.Errorf("restartAdvice(%v) = %q, want the restart instruction", tt.pending, got)
			}
			if hasNote := strings.Contains(got, loopbackNote); hasNote != tt.wantLoopback {
				t.Errorf("restartAdvice(%v) = %q, loopback note %v, want %v", tt.pending, got, hasNote, tt.wantLoopback)
			}
		})
	}
}

var errRefused = errors.New("connection refused")

// scriptedDialer returns a dialer that fails with failure failures times, then returns conn, and
// its call count.
func scriptedDialer(failures int, failure error, conn *pgx.Conn) (func(context.Context) (*pgx.Conn, error), *int) {
	calls := 0
	return func(context.Context) (*pgx.Conn, error) {
		calls++
		if calls <= failures {
			return nil, failure
		}
		return conn, nil
	}, &calls
}

func TestWaitReady(t *testing.T) {
	conn := &pgx.Conn{}
	wrongPassword := &pgconn.PgError{Code: "28P01", Message: "password authentication failed"}
	startingUp := &pgconn.PgError{Code: cannotConnectNow, Message: "the database system is starting up"}
	tests := []struct {
		name      string
		failures  int
		failure   error
		timeout   time.Duration
		wantErr   error
		wantCalls func(calls int) bool
	}{
		{"succeeds at once", 0, nil, time.Second, nil, func(calls int) bool { return calls == 1 }},
		{"succeeds after three refusals", 3, errRefused, time.Second, nil, func(calls int) bool { return calls == 4 }},
		{"retries 57P03", 2, startingUp, time.Second, nil, func(calls int) bool { return calls == 3 }},
		{
			"times out with the last connect error", 1 << 30, errRefused, 50 * time.Millisecond, errRefused,
			func(calls int) bool { return calls > 1 },
		},
		{
			"gives up at once on another server error", 1 << 30, wrongPassword, time.Minute, wrongPassword,
			func(calls int) bool { return calls == 1 },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dial, calls := scriptedDialer(tt.failures, tt.failure, conn)
			got, err := waitReady(context.Background(), tt.timeout, time.Millisecond, dial)
			if !tt.wantCalls(*calls) {
				t.Errorf("dial called %d times", *calls)
			}
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("waitReady() error = %v, want it to wrap %v", err, tt.wantErr)
				}
				if got != nil {
					t.Errorf("waitReady() returned a connection with error %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("waitReady() error = %v", err)
			}
			if got != conn {
				t.Errorf("waitReady() returned %p, want the dialed connection %p", got, conn)
			}
		})
	}
}

// a dial in flight at the deadline fails with the context's error; the message keeps the refusal
func TestWaitReadyKeepsTheLastRealErrorAtTheDeadline(t *testing.T) {
	calls := 0
	dial := func(ctx context.Context) (*pgx.Conn, error) {
		calls++
		if calls == 1 {
			return nil, errRefused
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	_, err := waitReady(context.Background(), 50*time.Millisecond, time.Millisecond, dial)
	if !errors.Is(err, errRefused) {
		t.Fatalf("waitReady() error = %v, want it to wrap %v", err, errRefused)
	}
}

func TestWaitReadyStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	dial := func(context.Context) (*pgx.Conn, error) {
		calls++
		if calls == 2 {
			cancel()
		}
		return nil, errRefused
	}
	_, err := waitReady(ctx, time.Minute, time.Millisecond, dial)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waitReady() error = %v, want it to wrap %v", err, context.Canceled)
	}
	if !errors.Is(err, errRefused) {
		t.Errorf("waitReady() error = %v, want it to keep the last connect error %v", err, errRefused)
	}
	if calls != 2 {
		t.Errorf("dial called %d times, want 2: no dial after the cancellation", calls)
	}
}
