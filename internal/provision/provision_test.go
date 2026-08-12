package provision

import "testing"

func TestComputeSettings(t *testing.T) {
	settings := computeSettings(16384)

	byName := map[string]string{}
	for _, s := range settings {
		byName[s.Name] = s.Value
	}

	tests := []struct {
		name string
		want string
	}{
		{"shared_buffers", "4096MB"},
		{"effective_cache_size", "8192MB"},
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
	if len(settings) != len(tests) {
		t.Fatalf("got %d settings, want %d", len(settings), len(tests))
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, present := byName[tt.name]
			if !present {
				t.Fatalf("setting %s is missing", tt.name)
			}
			if got != tt.want {
				t.Errorf("%s = %s, want %s", tt.name, got, tt.want)
			}
		})
	}
}

func TestEscapeLiteral(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain", "secret", "secret"},
		{"single quote", "o'brien", "o''brien"},
		{"injection attempt", "'; DROP ROLE postgres; --", "''; DROP ROLE postgres; --"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapeLiteral(tt.input); got != tt.want {
				t.Errorf("escapeLiteral(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
