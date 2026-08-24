package provision

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
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
}

func (o Options) Config(ctx context.Context) error {
	if err := o.Validate(); err != nil {
		return err
	}
	step("Phase config: ALTER SYSTEM deltas")

	conn, err := o.connect(ctx, "postgres", o.SuperUser, o.SuperPassword)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	for _, s := range settings {
		statement := fmt.Sprintf("ALTER SYSTEM SET %s = '%s'", s.Name, s.Value)
		if _, err := conn.Exec(ctx, statement); err != nil {
			return fmt.Errorf("%s: %w", statement, err)
		}
		ok("ALTER SYSTEM SET %s = '%s'", s.Name, s.Value)
	}

	if _, err := conn.Exec(ctx, "SELECT pg_reload_conf()"); err != nil {
		return fmt.Errorf("reloading the configuration: %w", err)
	}
	// pg_reload_conf signals asynchronously; the pause lets the server processes apply
	// the new files before pending_restart is read
	if _, err := conn.Exec(ctx, "SELECT pg_sleep(0.5)"); err != nil {
		return fmt.Errorf("waiting for the reload: %w", err)
	}
	ok("configuration reloaded")

	pending, err := pendingRestartSettings(ctx, conn)
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		ok("all settings active")
		return nil
	}
	note("waiting for a service restart: %s. Restart the PostgreSQL service or reboot the machine.",
		strings.Join(pending, ", "))
	return nil
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
