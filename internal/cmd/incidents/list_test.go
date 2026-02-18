package incidents

import (
	"strings"
	"testing"
	"time"

	"github.com/rootlyhq/rootly-cli/internal/api"
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

func TestIncidentDetailRows(t *testing.T) {
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	started := now.Add(-2 * time.Hour)
	resolved := now.Add(-1 * time.Hour)
	inc := &api.Incident{
		ID:              "inc-raw",
		SequentialID:    "INC-42",
		Title:           "Database Outage",
		Status:          "resolved",
		Severity:        "sev0",
		Summary:         "Major DB failure",
		Kind:            "normal",
		URL:             "https://rootly.com/incidents/42",
		CreatedAt:       now,
		StartedAt:       &started,
		ResolvedAt:      &resolved,
		CommanderName:   "Alice",
		Services:        []string{"api-gateway", "auth"},
		Teams:           []string{"Platform"},
		SlackChannelURL: "https://slack.com/channel",
		Private:         true,
		Source:          "datadog",
		Labels:          map[string]string{"env": "prod"},
	}

	rows := incidentDetailRows(inc)

	if len(rows) == 0 {
		t.Fatal("expected non-empty rows")
	}

	// Check sequential ID is used
	if findRow(rows, "ID") != "INC-42" {
		t.Errorf("ID = %q, want %q", findRow(rows, "ID"), "INC-42")
	}
	if findRow(rows, "Title") != "Database Outage" {
		t.Errorf("Title = %q, want %q", findRow(rows, "Title"), "Database Outage")
	}
	if findRow(rows, "Commander") != "Alice" {
		t.Errorf("Commander = %q, want %q", findRow(rows, "Commander"), "Alice")
	}
	if findRow(rows, "Services") != "api-gateway, auth" {
		t.Errorf("Services = %q, want %q", findRow(rows, "Services"), "api-gateway, auth")
	}
	if findRow(rows, "Private") != "true" {
		t.Errorf("Private = %q, want %q", findRow(rows, "Private"), "true")
	}
	// Labels should contain env=prod
	labels := findRow(rows, "Labels")
	if !strings.Contains(labels, "env=prod") {
		t.Errorf("Labels = %q, want to contain %q", labels, "env=prod")
	}
}

func TestIncidentDetailRowsFallbackID(t *testing.T) {
	inc := &api.Incident{
		ID:     "raw-uuid",
		Status: "started",
	}

	rows := incidentDetailRows(inc)
	if findRow(rows, "ID") != "raw-uuid" {
		t.Errorf("ID = %q, want %q", findRow(rows, "ID"), "raw-uuid")
	}
}

func findRow(rows [][]string, field string) string {
	for _, r := range rows {
		if len(r) >= 2 && r[0] == field {
			return r[1]
		}
	}
	return ""
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
