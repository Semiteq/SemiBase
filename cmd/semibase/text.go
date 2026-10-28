package main

import (
	"strings"
	"unicode/utf8"
)

// readableText re-decodes the runs of non-ASCII bytes that are not UTF-8. A server reports a
// failed login before the client encoding applies, so a non-English lc_messages arrives in the
// server's own code page inside an otherwise UTF-8 error string.
func readableText(text string, decode func([]byte) (string, bool)) string {
	if utf8.ValidString(text) {
		return text
	}
	var result strings.Builder
	for len(text) > 0 {
		ascii := strings.IndexFunc(text, func(r rune) bool { return r >= utf8.RuneSelf })
		if ascii < 0 {
			result.WriteString(text)
			break
		}
		result.WriteString(text[:ascii])
		text = text[ascii:]
		end := strings.IndexFunc(text, func(r rune) bool { return r < utf8.RuneSelf })
		if end < 0 {
			end = len(text)
		}
		run := text[:end]
		text = text[end:]
		if utf8.ValidString(run) {
			result.WriteString(run)
		} else if decoded, ok := decode([]byte(run)); ok {
			result.WriteString(decoded)
		} else {
			result.WriteString(strings.ToValidUTF8(run, string(utf8.RuneError)))
		}
	}
	return result.String()
}
