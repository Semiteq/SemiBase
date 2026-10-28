//go:build windows

package main

import (
	"math"

	"golang.org/x/sys/windows"
)

// decodeANSI reads bytes in the system ANSI code page, the one a local PostgreSQL service
// writes its messages in.
func decodeANSI(text []byte) (string, bool) {
	return decodeCodePage(windows.GetACP(), text)
}

func decodeCodePage(codePage uint32, text []byte) (string, bool) {
	if len(text) == 0 {
		return "", true
	}
	if len(text) > math.MaxInt32 {
		return "", false
	}
	length := int32(len(text)) //nolint:gosec // bounded by the MaxInt32 check above
	size, err := windows.MultiByteToWideChar(codePage, 0, &text[0], length, nil, 0)
	if err != nil || size == 0 {
		return "", false
	}
	wide := make([]uint16, size)
	if _, err := windows.MultiByteToWideChar(codePage, 0, &text[0], length, &wide[0], size); err != nil {
		return "", false
	}
	return windows.UTF16ToString(wide), true
}
