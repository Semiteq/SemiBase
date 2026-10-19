package provision

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type setting struct {
	Name  string
	Value string
}

var settings = []setting{
	{"shared_buffers", "2GB"},
	{"effective_cache_size", "4GB"},
	{"work_mem", "64MB"},
	{"maintenance_work_mem", "512MB"},
	{"max_wal_size", "8GB"},
	{"checkpoint_timeout", "30min"},
	{"checkpoint_completion_target", "0.9"},
	{"wal_compression", "on"},
	{"random_page_cost", "1.1"},
	{"log_min_duration_statement", "1000"},
	{"track_io_timing", "on"},
	{"listen_addresses", "localhost"},
}

const (
	readyTimeout  = 60 * time.Second
	readyInterval = time.Second
)

func (o Options) config(ctx context.Context) error {
	if err := o.Validate(); err != nil {
		return err
	}
	step("Phase config: ALTER SYSTEM deltas")

	var service *managedService
	if o.RestartService != "" {
		var err error
		// SA4023: off Windows openService always fails, by design
		if service, err = openService(ctx, o.RestartService, o.describeServer); err != nil { //nolint:staticcheck
			return err
		}
		defer service.close()
	}

	pending, err := o.applySettings(ctx)
	if err != nil {
		return err
	}
	switch {
	case len(pending) > 0 && service != nil:
		return o.restartService(ctx, service, pending)
	case len(pending) > 0:
		note("waiting for a service restart: %s. %s", strings.Join(pending, ", "), restartAdvice(pending))
	default:
		ok("all settings active")
	}
	return nil
}

// runningServer is what a connection to --host:--port learns about the server behind it.
type runningServer struct {
	dataDirectory string
	// address is the IP:port the connection reached, one address of one family
	address string
}

func (o Options) describeServer(ctx context.Context) (runningServer, error) {
	conn, err := o.connectSuperuser(ctx, "postgres")
	if err != nil {
		return runningServer{}, err
	}
	defer conn.Close(ctx)
	var dataDirectory string
	if err := conn.QueryRow(ctx, "SHOW data_directory").Scan(&dataDirectory); err != nil {
		return runningServer{}, fmt.Errorf("reading data_directory: %w", err)
	}
	return runningServer{
		dataDirectory: dataDirectory,
		address:       conn.PgConn().Conn().RemoteAddr().String(),
	}, nil
}

// applySettings closes its connection before returning: a restart must not find it open.
func (o Options) applySettings(ctx context.Context) ([]string, error) {
	conn, err := o.connectSuperuser(ctx, "postgres")
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)

	for _, s := range settings {
		statement := fmt.Sprintf("ALTER SYSTEM SET %s = '%s'", s.Name, s.Value)
		if _, err := conn.Exec(ctx, statement); err != nil {
			return nil, fmt.Errorf("%s: %w", statement, err)
		}
		ok("ALTER SYSTEM SET %s = '%s'", s.Name, s.Value)
	}

	if _, err := conn.Exec(ctx, "SELECT pg_reload_conf()"); err != nil {
		return nil, fmt.Errorf("reloading the configuration: %w", err)
	}
	// pg_reload_conf signals asynchronously; the pause lets the server processes apply
	// the new files before pending_restart is read
	if _, err := conn.Exec(ctx, "SELECT pg_sleep(0.5)"); err != nil {
		return nil, fmt.Errorf("waiting for the reload: %w", err)
	}
	ok("configuration reloaded")

	return pendingRestartSettings(ctx, conn)
}

// restartAdvice is the operator's next step for settings that wait for a restart.
func restartAdvice(pending []string) string {
	advice := "Restart the PostgreSQL service or reboot the machine."
	if slices.Contains(pending, "listen_addresses") {
		advice += " After the restart the server accepts connections from this machine only."
	}
	return advice
}

func (o Options) restartService(ctx context.Context, service *managedService, pending []string) error {
	step("restarting service %s: %s waiting for a restart", o.RestartService, strings.Join(pending, ", "))
	if err := service.restart(ctx); err != nil {
		return err
	}
	conn, err := waitReady(ctx, readyTimeout, readyInterval, func(ctx context.Context) (*pgx.Conn, error) {
		return o.connectSuperuser(ctx, "postgres")
	})
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	ok("server accepting connections")

	stillPending, err := pendingRestartSettings(ctx, conn)
	if err != nil {
		return err
	}
	if len(stillPending) > 0 {
		return fmt.Errorf("after restarting service %s, settings still wait for a restart: %s",
			o.RestartService, strings.Join(stillPending, ", "))
	}
	ok("all settings active")
	return nil
}

// waitReady dials at once, then every interval, until a dial succeeds or timeout elapses. Network
// errors and 57P03 are retried; any other server error is final.
func waitReady(
	ctx context.Context,
	timeout, interval time.Duration,
	dial func(context.Context) (*pgx.Conn, error),
) (*pgx.Conn, error) {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var lastErr error
	for {
		conn, err := dial(waitCtx)
		if err == nil {
			return conn, nil
		}
		if waitCtx.Err() != nil {
			// a dial cut short by the deadline reports the context, not why the server refused
			if lastErr == nil {
				lastErr = err
			}
			return nil, waitReadyFailure(ctx, timeout, lastErr)
		}
		if !retryableConnectError(err) {
			return nil, err
		}
		lastErr = err
		select {
		case <-waitCtx.Done():
			return nil, waitReadyFailure(ctx, timeout, lastErr)
		case <-time.After(interval):
		}
	}
}

func waitReadyFailure(ctx context.Context, timeout time.Duration, lastErr error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("waiting for the server to accept connections: %w; last connect error: %w",
			ctx.Err(), lastErr)
	}
	return fmt.Errorf("the server accepted no connection within %s: %w", timeout, lastErr)
}

const cannotConnectNow = "57P03"

func retryableConnectError(err error) bool {
	var pgErr *pgconn.PgError
	return !errors.As(err, &pgErr) || pgErr.Code == cannotConnectNow
}

func pendingRestartSettings(ctx context.Context, conn *pgx.Conn) ([]string, error) {
	rows, err := conn.Query(ctx, "SELECT name FROM pg_settings WHERE pending_restart ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("reading pending_restart settings: %w", err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("reading pending_restart settings: %w", err)
	}
	return names, nil
}
