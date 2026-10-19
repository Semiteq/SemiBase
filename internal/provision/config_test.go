package provision

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestRestartNeeded(t *testing.T) {
	tests := []struct {
		name    string
		pending []string
		service string
		want    bool
	}{
		{"nothing pending, no service", nil, "", false},
		{"nothing pending, service named", nil, "postgresql-x64-17", false},
		{"pending, no service", []string{"shared_buffers"}, "", false},
		{"pending, service named", []string{"listen_addresses", "shared_buffers"}, "postgresql-x64-17", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := restartNeeded(tt.pending, tt.service); got != tt.want {
				t.Errorf("restartNeeded(%v, %q) = %v, want %v", tt.pending, tt.service, got, tt.want)
			}
		})
	}
}

var errRefused = errors.New("connection refused")

// failingDialer returns a dialer that fails failures times, then returns conn, and its call count.
func failingDialer(failures int, conn *pgx.Conn) (func(context.Context) (*pgx.Conn, error), *int) {
	calls := 0
	return func(context.Context) (*pgx.Conn, error) {
		calls++
		if calls <= failures {
			return nil, errRefused
		}
		return conn, nil
	}, &calls
}

func TestWaitReady(t *testing.T) {
	conn := &pgx.Conn{}
	tests := []struct {
		name      string
		failures  int
		timeout   time.Duration
		wantCalls int
		wantErr   bool
	}{
		{"succeeds at once", 0, time.Second, 1, false},
		{"succeeds after three failures", 3, time.Second, 4, false},
		{"times out with the last error", 1 << 30, 50 * time.Millisecond, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dial, calls := failingDialer(tt.failures, conn)
			got, err := waitReady(context.Background(), tt.timeout, time.Millisecond, dial)
			if tt.wantErr {
				if !errors.Is(err, errRefused) {
					t.Fatalf("waitReady() error = %v, want it to wrap %v", err, errRefused)
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
			if *calls != tt.wantCalls {
				t.Errorf("dial called %d times, want %d", *calls, tt.wantCalls)
			}
		})
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
	if calls != 2 {
		t.Errorf("dial called %d times after the cancellation, want 2", calls)
	}
}
