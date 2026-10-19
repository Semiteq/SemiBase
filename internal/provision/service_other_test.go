//go:build !windows

package provision

import (
	"runtime"
	"strings"
	"testing"
)

func TestOpenServiceIsRefusedOffWindows(t *testing.T) {
	service, err := openService("postgresql-x64-17")
	if err == nil {
		service.close()
		t.Fatal("openService() succeeded off Windows")
	}
	if want := "not supported on " + runtime.GOOS; !strings.Contains(err.Error(), want) {
		t.Errorf("openService() error = %q, want it to contain %q", err, want)
	}
}
