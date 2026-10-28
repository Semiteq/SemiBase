//go:build !windows

package main

func decodeANSI([]byte) (string, bool) {
	return "", false
}
