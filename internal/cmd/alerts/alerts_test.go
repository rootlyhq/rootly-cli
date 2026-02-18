package alerts

import (
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

func TestAlertDetailRows(t *testing.T) {
	now := time.Now()
	alert := &api.Alert{
		ID:           "alert-123",
		ShortID:      "ALR-42",
		Summary:      "High CPU usage",
		Status:       "triggered",
		Source:       "datadog",
		Description:  "CPU at 95%",
		Urgency:      "high",
		URL:          "https://rootly.com/alerts/42",
		ExternalURL:  "https://datadog.com/alert/123",
		ExternalID:   "dd-123",
		CreatedAt:    now,
		Services:     []string{"api-gateway"},
		Environments: []string{"production"},
		Responders:   []string{"Alice", "Bob"},
		Noise:        "not_noise",
	}

	rows := alertDetailRows(alert)

	// Should have rows for all non-empty fields
	if len(rows) == 0 {
		t.Fatal("expected non-empty rows")
	}

	// Check ID uses ShortID
	found := findRow(rows, "ID")
	if found != "ALR-42" {
		t.Errorf("ID = %q, want %q", found, "ALR-42")
	}

	// Check Status
	found = findRow(rows, "Status")
	if found != "triggered" {
		t.Errorf("Status = %q, want %q", found, "triggered")
	}

	// Check Services
	found = findRow(rows, "Services")
	if found != "api-gateway" {
		t.Errorf("Services = %q, want %q", found, "api-gateway")
	}

	// Check Responders
	found = findRow(rows, "Responders")
	if found != "Alice, Bob" {
		t.Errorf("Responders = %q, want %q", found, "Alice, Bob")
	}
}

func TestAlertDetailRowsFallbackID(t *testing.T) {
	alert := &api.Alert{
		ID:     "raw-id-456",
		Status: "resolved",
	}

	rows := alertDetailRows(alert)
	found := findRow(rows, "ID")
	if found != "raw-id-456" {
		t.Errorf("ID = %q, want %q", found, "raw-id-456")
	}
}

func TestAlertDetailRowsWithNotifiedUsers(t *testing.T) {
	alert := &api.Alert{
		ID:     "alert-1",
		Status: "triggered",
		NotifiedUsers: []api.AlertUser{
			{Name: "Alice", Email: "alice@example.com"},
			{Name: "Bob", Email: "bob@example.com"},
		},
	}

	rows := alertDetailRows(alert)
	found := findRow(rows, "Notified Users")
	if found != "Alice, Bob" {
		t.Errorf("Notified Users = %q, want %q", found, "Alice, Bob")
	}
}

func TestAlertDetailRowsWithRelatedIncidents(t *testing.T) {
	alert := &api.Alert{
		ID:     "alert-1",
		Status: "triggered",
		RelatedIncidents: []api.AlertIncident{
			{ID: "inc-1", SequentialID: "INC-100"},
			{ID: "inc-2", SequentialID: ""},
		},
	}

	rows := alertDetailRows(alert)
	found := findRow(rows, "Related Incidents")
	if found != "INC-100, inc-2" {
		t.Errorf("Related Incidents = %q, want %q", found, "INC-100, inc-2")
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
