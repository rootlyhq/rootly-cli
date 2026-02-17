package views

import (
	"strings"
	"testing"
	"time"
)

func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		seconds  int64
		expected string
	}{
		{"zero seconds", 0, "0s"},
		{"30 seconds", 30, "30s"},
		{"59 seconds", 59, "59s"},
		{"1 minute", 60, "1m"},
		{"1 minute 30 seconds", 90, "1m 30s"},
		{"5 minutes", 300, "5m"},
		{"59 minutes 59 seconds", 3599, "59m 59s"},
		{"1 hour", 3600, "1h"},
		{"1 hour 30 minutes", 5400, "1h 30m"},
		{"2 hours", 7200, "2h"},
		{"2 hours 15 minutes", 8100, "2h 15m"},
		{"24 hours", 86400, "24h"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.seconds)
			if result != tt.expected {
				t.Errorf("formatDuration(%d) = %q, expected %q", tt.seconds, result, tt.expected)
			}
		})
	}
}

func TestFormatHours(t *testing.T) {
	tests := []struct {
		name     string
		hours    float64
		expected string
	}{
		{"zero hours", 0, "0m"},
		{"30 minutes", 0.5, "30m"},
		{"1 hour", 1.0, "1h"},
		{"1.5 hours", 1.5, "1h 30m"},
		{"2 hours", 2.0, "2h"},
		{"2.25 hours", 2.25, "2h 15m"},
		{"24 hours", 24.0, "1d"},
		{"25 hours", 25.0, "1d 1h"},
		{"48 hours", 48.0, "2d"},
		{"49.5 hours", 49.5, "2d 1h"},
		{"72 hours", 72.0, "3d"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatHours(tt.hours)
			if result != tt.expected {
				t.Errorf("formatHours(%v) = %q, expected %q", tt.hours, result, tt.expected)
			}
		})
	}
}

func TestFormatTimeHelper(t *testing.T) {
	// Test that formatTime returns a non-empty string with expected format
	// We can't test exact output due to timezone variations
	testTime := mustParseTime("2025-01-15T10:30:00Z")

	result := formatTime(testTime)

	// Should contain the date
	if !strings.Contains(result, "Jan 15, 2025") {
		t.Errorf("formatTime() should contain 'Jan 15, 2025', got %q", result)
	}

	// Should contain time
	if !strings.Contains(result, ":") {
		t.Errorf("formatTime() should contain time with ':', got %q", result)
	}
}

func TestFormatAlertTimeHelper(t *testing.T) {
	// Test that formatAlertTime returns a non-empty string with expected format
	testTime := mustParseTime("2025-01-15T10:30:00Z")

	result := formatAlertTime(testTime)

	// Should contain the date
	if !strings.Contains(result, "Jan 15, 2025") {
		t.Errorf("formatAlertTime() should contain 'Jan 15, 2025', got %q", result)
	}

	// Should contain time
	if !strings.Contains(result, ":") {
		t.Errorf("formatAlertTime() should contain time with ':', got %q", result)
	}
}

func TestFormatRelativeTime(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		time     time.Time
		expected string
	}{
		{"zero time", time.Time{}, "-"},
		{"now (0 seconds ago)", now, "now"},
		{"30 seconds ago", now.Add(-30 * time.Second), "now"},
		{"59 seconds ago", now.Add(-59 * time.Second), "now"},
		{"1 minute ago", now.Add(-1 * time.Minute), "1m ago"},
		{"5 minutes ago", now.Add(-5 * time.Minute), "5m ago"},
		{"30 minutes ago", now.Add(-30 * time.Minute), "30m ago"},
		{"59 minutes ago", now.Add(-59 * time.Minute), "59m ago"},
		{"1 hour ago", now.Add(-1 * time.Hour), "1h ago"},
		{"2 hours ago", now.Add(-2 * time.Hour), "2h ago"},
		{"23 hours ago", now.Add(-23 * time.Hour), "23h ago"},
		{"1 day ago", now.Add(-24 * time.Hour), "1d ago"},
		{"2 days ago", now.Add(-48 * time.Hour), "2d ago"},
		{"6 days ago", now.Add(-6 * 24 * time.Hour), "6d ago"},
		{"1 week ago", now.Add(-7 * 24 * time.Hour), "1w ago"},
		{"2 weeks ago", now.Add(-14 * 24 * time.Hour), "2w ago"},
		{"3 weeks ago", now.Add(-21 * 24 * time.Hour), "3w ago"},
		{"1 month ago", now.Add(-30 * 24 * time.Hour), "1mo ago"},
		{"2 months ago", now.Add(-60 * 24 * time.Hour), "2mo ago"},
		{"6 months ago", now.Add(-180 * 24 * time.Hour), "6mo ago"},
		{"1 year ago", now.Add(-365 * 24 * time.Hour), "1y ago"},
		{"2 years ago", now.Add(-730 * 24 * time.Hour), "2y ago"},
		{"future time", now.Add(1 * time.Hour), "now"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatRelativeTime(tt.time)
			if result != tt.expected {
				t.Errorf("formatRelativeTime() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestFormatRelativeTimeEdgeCases(t *testing.T) {
	now := time.Now()

	// Test boundary between minutes and hours (60 minutes = 1 hour)
	t.Run("exactly 60 minutes", func(t *testing.T) {
		result := formatRelativeTime(now.Add(-60 * time.Minute))
		if result != "1h ago" {
			t.Errorf("formatRelativeTime(60 min ago) = %q, expected '1h ago'", result)
		}
	})

	// Test boundary between hours and days (24 hours = 1 day)
	t.Run("exactly 24 hours", func(t *testing.T) {
		result := formatRelativeTime(now.Add(-24 * time.Hour))
		if result != "1d ago" {
			t.Errorf("formatRelativeTime(24h ago) = %q, expected '1d ago'", result)
		}
	})

	// Test boundary between days and weeks (7 days = 1 week)
	t.Run("exactly 7 days", func(t *testing.T) {
		result := formatRelativeTime(now.Add(-7 * 24 * time.Hour))
		if result != "1w ago" {
			t.Errorf("formatRelativeTime(7d ago) = %q, expected '1w ago'", result)
		}
	})

	// Test boundary between weeks and months (30 days = 1 month)
	t.Run("exactly 30 days", func(t *testing.T) {
		result := formatRelativeTime(now.Add(-30 * 24 * time.Hour))
		if result != "1mo ago" {
			t.Errorf("formatRelativeTime(30d ago) = %q, expected '1mo ago'", result)
		}
	})

	// Test boundary between months and years (365 days = 1 year)
	t.Run("exactly 365 days", func(t *testing.T) {
		result := formatRelativeTime(now.Add(-365 * 24 * time.Hour))
		if result != "1y ago" {
			t.Errorf("formatRelativeTime(365d ago) = %q, expected '1y ago'", result)
		}
	})
}
