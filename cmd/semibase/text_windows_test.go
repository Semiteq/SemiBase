//go:build windows

package main

import "testing"

func TestDecodeCodePage(t *testing.T) {
	// the failed-login message a PostgreSQL 17 with Russian lc_messages sends, in CP1251
	server := "\xc2\xc0\xc6\xcd\xce: \xef\xee\xeb\xfc\xe7\xee\xe2\xe0\xf2\xe5\xeb\xfc \"postgres\" " +
		"\xed\xe5 \xef\xf0\xee\xf8\xb8\xeb"
	want := "ВАЖНО: пользователь \"postgres\" не прошёл"
	got := readableText(server, func(run []byte) (string, bool) { return decodeCodePage(1251, run) })
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
