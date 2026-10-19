//go:build !windows

package provision

import (
	"context"
	"errors"
	"fmt"
	"runtime"
)

type managedService struct{}

func openService(_ context.Context, name string) (*managedService, error) {
	return nil, fmt.Errorf("--restart-service %s: service restart is not supported on %s", name, runtime.GOOS)
}

func (*managedService) close() {}

func (*managedService) restart(context.Context) error {
	return errors.New("service restart is not supported on " + runtime.GOOS)
}
