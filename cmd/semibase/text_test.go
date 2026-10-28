package main

import "testing"

func TestReadableText(t *testing.T) {
	// brackets each run it is handed, so the test sees which runs were re-decoded
	marker := func(run []byte) (string, bool) { return "<" + string(run) + ">", true }
	refuse := func([]byte) (string, bool) { return "", false }
	tests := []struct {
		name   string
		text   string
		decode func([]byte) (string, bool)
		want   string
	}{
		{"ascii stays", "password authentication failed (SQLSTATE 28P01)", marker,
			"password authentication failed (SQLSTATE 28P01)"},
		{"valid utf-8 stays", "папка C:\\данные", marker, "папка C:\\данные"},
		{"only the invalid run is re-decoded", "folder данные: \xef\xee (x)", marker,
			"folder данные: <\xef\xee> (x)"},
		{"two invalid runs", "\xc2\xc0 \"postgres\" \xed\xe5", marker,
			"<\xc2\xc0> \"postgres\" <\xed\xe5>"},
		{"a run the decoder refuses becomes replacement characters", "a \xff b", refuse,
			"a \uFFFD b"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := readableText(test.text, test.decode); got != test.want {
				t.Errorf("readableText(%q) = %q, want %q", test.text, got, test.want)
			}
		})
	}
}
