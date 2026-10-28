//go:build !windows

package engine

import "errors"

// Detect needs the Windows service list, so it fails on every other system.
func Detect(_ int) (Result, error) {
	return Result{}, errors.New("engine detection needs the Windows service list")
}
