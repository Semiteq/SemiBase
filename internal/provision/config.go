package provision

import (
	"context"
	"fmt"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
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
	returned, _, callError := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&status)))
	if returned == 0 {
		return 0, fmt.Errorf("querying physical memory: %w", callError)
	}
	return int(status.TotalPhys / (1024 * 1024)), nil
}

// Config applies the server configuration deltas through ALTER SYSTEM and
// restarts the Windows service when one is named.
func (o Options) Config(ctx context.Context) error {
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
	if err := restartService(o.ServiceName); err != nil {
		return err
	}
	ok("service %s restarted", o.ServiceName)
	return nil
}

func restartService(name string) error {
	if output, err := exec.Command("net", "stop", name).CombinedOutput(); err != nil {
		return fmt.Errorf("net stop %s: %w\n%s", name, err, output)
	}
	if output, err := exec.Command("net", "start", name).CombinedOutput(); err != nil {
		return fmt.Errorf("net start %s: %w\n%s", name, err, output)
	}
	return nil
}
