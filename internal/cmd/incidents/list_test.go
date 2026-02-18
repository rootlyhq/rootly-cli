package incidents

import (
	"testing"
)

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"needs truncation", "hello world", 8, "hello..."},
		{"very short max", "hello", 2, "he"},
		{"max 3", "hello", 3, "hel"},
		{"max 4", "hello world", 4, "h..."},
		{"empty string", "", 5, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateString(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name    string
		seconds int64
		want    string
	}{
		{"zero", 0, "-"},
		{"seconds only", 30, "30s"},
		{"one minute", 60, "1m"},
		{"minutes only", 2700, "45m"},
		{"hours and minutes", 9000, "2h 30m"},
		{"hours only", 7200, "2h"},
		{"days and hours", 93600, "1d 2h"},
		{"days only", 86400, "1d"},
		{"multiple days", 259200, "3d"},
		{"complex duration", 90061, "1d 1h"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.seconds)
			if got != tt.want {
				t.Errorf("formatDuration(%d) = %q, want %q", tt.seconds, got, tt.want)
			}
		})
	}
}
