package timeformat

import (
	"strings"
	"testing"
	"time"
)

func TestFormatTime(t *testing.T) {
	ts := time.Date(2025, 3, 15, 14, 30, 0, 0, time.UTC)
	result := FormatTime(ts)

	// Should contain the date components
	if !strings.Contains(result, "Mar") {
		t.Errorf("expected month 'Mar' in %q", result)
	}
	if !strings.Contains(result, "15") {
		t.Errorf("expected day '15' in %q", result)
	}
	if !strings.Contains(result, "2025") {
		t.Errorf("expected year '2025' in %q", result)
	}
	// Should contain AM/PM
	if !strings.Contains(result, "AM") && !strings.Contains(result, "PM") {
		t.Errorf("expected AM or PM in %q", result)
	}
	// Should contain a timezone abbreviation (at least 2 uppercase chars at the end)
	parts := strings.Fields(result)
	tz := parts[len(parts)-1]
	if len(tz) < 2 {
		t.Errorf("expected timezone abbreviation at end, got %q in %q", tz, result)
	}
}

func TestFormatTimeZero(t *testing.T) {
	result := FormatTime(time.Time{})
	if result != "-" {
		t.Errorf("expected '-' for zero time, got %q", result)
	}
}

func TestFormatTimePtr(t *testing.T) {
	ts := time.Date(2025, 6, 1, 9, 0, 0, 0, time.UTC)
	result := FormatTimePtr(&ts)

	if !strings.Contains(result, "Jun") {
		t.Errorf("expected 'Jun' in %q", result)
	}
	if !strings.Contains(result, "2025") {
		t.Errorf("expected '2025' in %q", result)
	}
}

func TestFormatTimePtrNil(t *testing.T) {
	result := FormatTimePtr(nil)
	if result != "-" {
		t.Errorf("expected '-' for nil, got %q", result)
	}
}

func TestFormatTimeWithFallback(t *testing.T) {
	fallback := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	primary := time.Date(2025, 7, 15, 12, 0, 0, 0, time.UTC)

	// With primary set — should use primary
	result := FormatTimeWithFallback(&primary, fallback)
	if !strings.Contains(result, "Jul") && !strings.Contains(result, "2025") {
		t.Errorf("expected primary time in result, got %q", result)
	}

	// With nil primary — should use fallback
	result = FormatTimeWithFallback(nil, fallback)
	if !strings.Contains(result, "Jan") && !strings.Contains(result, "2025") {
		t.Errorf("expected fallback time in result, got %q", result)
	}

	// Primary should differ from fallback
	resultPrimary := FormatTimeWithFallback(&primary, fallback)
	resultFallback := FormatTimeWithFallback(nil, fallback)
	if resultPrimary == resultFallback {
		t.Errorf("expected different results for primary vs fallback")
	}
}

func TestFormatTimeIncludesTimezone(t *testing.T) {
	ts := time.Date(2025, 3, 15, 14, 30, 0, 0, time.UTC)
	result := FormatTime(ts)

	// The result should end with a timezone abbreviation
	parts := strings.Fields(result)
	if len(parts) < 4 {
		t.Fatalf("expected at least 4 parts in %q", result)
	}
	tz := parts[len(parts)-1]
	// Timezone abbreviation should be all uppercase letters (e.g. UTC, PST, EST)
	for _, c := range tz {
		if c < 'A' || c > 'Z' {
			// Some timezones use +/- offsets, which is also fine
			if c != '+' && c != '-' && (c < '0' || c > '9') {
				t.Errorf("unexpected character %q in timezone %q from result %q", string(c), tz, result)
			}
		}
	}
}

func TestDetectLocation(t *testing.T) {
	loc := detectLocation()
	if loc == nil {
		t.Fatal("detectLocation returned nil")
	}
	// Should be a valid location that can format times
	ts := time.Now().In(loc)
	if ts.Location() != loc {
		t.Errorf("expected location %v, got %v", loc, ts.Location())
	}
}

func TestHumanLayout(t *testing.T) {
	// Verify the layout produces expected output with a known time and timezone
	ts := time.Date(2025, 3, 15, 14, 30, 0, 0, time.UTC)
	result := ts.Format(HumanLayout)
	expected := "Mar 15, 2025 02:30 PM UTC"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}
