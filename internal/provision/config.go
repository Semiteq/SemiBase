package provision

import (
	"context"
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// setting is one ALTER SYSTEM delta. Order is kept for readable output.
type setting struct {
	Name  string
	Value string
}

// computeSettings derives the configuration deltas for a machine with the given
// amount of physical memory. The rationale for every value is in
// docs/architecture/configuration.md.
func computeSettings(totalMemoryMB int) []setting {
	return []setting{
		{"shared_buffers", fmt.Sprintf("%dMB", totalMemoryMB/4)},
		{"effective_cache_size", fmt.Sprintf("%dMB", totalMemoryMB/2)},
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
}

// memoryStatusEx mirrors the Win32 MEMORYSTATUSEX structure.
type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

var globalMemoryStatusEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

func totalPhysicalMemoryMB() (int, error) {
	var status memoryStatusEx
	status.Length = uint32(unsafe.Sizeof(status))
	//nolint:gosec // the raw pointer is the documented GlobalMemoryStatusEx calling convention
	returned, _, callError := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&status)))
	if returned == 0 {
		return 0, fmt.Errorf("querying physical memory: %w", callError)
	}
	//nolint:gosec // physical memory in megabytes fits int on windows/amd64
	return int(status.TotalPhys / (1024 * 1024)), nil
}

// Config applies the server configuration deltas through ALTER SYSTEM and
// restarts the Windows service when one is named.
func (o Options) Config(ctx context.Context) error {
	if err := o.Validate(); err != nil {
		return err
	}
	step("Phase config: ALTER SYSTEM deltas")
	totalMB, err := totalPhysicalMemoryMB()
	if err != nil {
		return err
	}

	conn, err := o.connect(ctx, "postgres", o.SuperUser, o.SuperPassword)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	for _, s := range computeSettings(totalMB) {
		statement := fmt.Sprintf("ALTER SYSTEM SET %s = '%s'", s.Name, s.Value)
		if _, err := conn.Exec(ctx, statement); err != nil {
			return fmt.Errorf("%s: %w", statement, err)
		}
		ok("ALTER SYSTEM SET %s = '%s'", s.Name, s.Value)
	}

	if o.ServiceName == "" {
		note("settings written to postgresql.auto.conf; restart the PostgreSQL service to apply.")
		return nil
	}
	if err := restartService(ctx, o.ServiceName); err != nil {
		return err
	}
	ok("service %s restarted", o.ServiceName)
	return nil
}

const servicePollInterval = 500 * time.Millisecond

// restartService stops and starts the named Windows service through the
// service manager.
func restartService(ctx context.Context, name string) error {
	manager, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connecting to the service manager: %w", err)
	}
	defer manager.Disconnect()

	service, err := manager.OpenService(name)
	if err != nil {
		return fmt.Errorf("opening service %s: %w", name, err)
	}
	defer service.Close()

	if err := stopService(ctx, service, name); err != nil {
		return err
	}
	return startService(ctx, service, name)
}

// stopService requests a stop and waits until the service reports Stopped.
// An already-stopped service is left as-is (Control(svc.Stop) would fail with
// ERROR_SERVICE_NOT_ACTIVE), so the restart stays idempotent.
func stopService(ctx context.Context, service *mgr.Service, name string) error {
	status, err := service.Query()
	if err != nil {
		return fmt.Errorf("querying service %s: %w", name, err)
	}
	if status.State == svc.Stopped {
		return nil
	}
	if status.State != svc.StopPending {
		if status, err = service.Control(svc.Stop); err != nil {
			return fmt.Errorf("stopping service %s: %w", name, err)
		}
	}
	for status.State != svc.Stopped {
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for service %s to stop: %w", name, ctx.Err())
		case <-time.After(servicePollInterval):
		}
		if status, err = service.Query(); err != nil {
			return fmt.Errorf("querying service %s: %w", name, err)
		}
	}
	return nil
}

// startService starts the service and waits until it reports Running.
// Win32 StartService returns at START_PENDING, so returning right after
// Start() would report success while PostgreSQL may still refuse to come up
// on a value the config command just wrote; the service falling back to
// Stopped signals that failure.
func startService(ctx context.Context, service *mgr.Service, name string) error {
	if err := service.Start(); err != nil {
		return fmt.Errorf("starting service %s: %w", name, err)
	}
	for {
		status, err := service.Query()
		if err != nil {
			return fmt.Errorf("querying service %s: %w", name, err)
		}
		switch status.State {
		case svc.Running:
			return nil
		case svc.Stopped:
			return fmt.Errorf("service %s stopped right after start; check the PostgreSQL log", name)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for service %s to start: %w", name, ctx.Err())
		case <-time.After(servicePollInterval):
		}
	}
}
