package timeformat

import (
	"time"
)

// HumanLayout is the format used for human-readable date/time output.
const HumanLayout = "Jan 02, 2006 03:04 PM MST"

// location is the detected system timezone, resolved once at init.
var location *time.Location

func init() {
	location = detectLocation()
}

// detectLocation returns the local system timezone.
func detectLocation() *time.Location {
	zone, _ := time.Now().Zone()
	if loc, err := time.LoadLocation(zone); err == nil {
		return loc
	}
	return time.Local
}

// FormatTime formats a time.Time in human-readable format with system timezone.
func FormatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.In(location).Format(HumanLayout)
}

// FormatTimePtr formats a *time.Time, returning "-" if nil.
func FormatTimePtr(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return FormatTime(*t)
}

// FormatTimeWithFallback formats a time pointer, falling back to a default time.
func FormatTimeWithFallback(primary *time.Time, fallback time.Time) string {
	if primary != nil {
		return FormatTime(*primary)
	}
	return FormatTime(fallback)
}
