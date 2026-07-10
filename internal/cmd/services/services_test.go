package services

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

func TestServiceDetailRows(t *testing.T) {
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	svc := &api.Service{
		ID:                 "svc-123",
		Name:               "api-gateway",
		Slug:               "api-gateway",
		Description:        "Main API gateway",
		Color:              "#FF5733",
		EscalationPolicyID: "ep-456",
		OwnerTeamName:      "Platform",
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	rows := serviceDetailRows(svc)

	if len(rows) == 0 {
		t.Fatal("expected non-empty rows")
	}

	if findRow(rows, "ID") != "svc-123" {
		t.Errorf("ID = %q, want %q", findRow(rows, "ID"), "svc-123")
	}
	if findRow(rows, "Name") != "api-gateway" {
		t.Errorf("Name = %q, want %q", findRow(rows, "Name"), "api-gateway")
	}
	if findRow(rows, "Description") != "Main API gateway" {
		t.Errorf("Description = %q, want %q", findRow(rows, "Description"), "Main API gateway")
	}
	if findRow(rows, "Escalation Policy") != "ep-456" {
		t.Errorf("Escalation Policy = %q, want %q", findRow(rows, "Escalation Policy"), "ep-456")
	}
	if findRow(rows, "Owner Team") != "Platform" {
		t.Errorf("Owner Team = %q, want %q", findRow(rows, "Owner Team"), "Platform")
	}
	if findRow(rows, "Color") != "#FF5733" {
		t.Errorf("Color = %q, want %q", findRow(rows, "Color"), "#FF5733")
	}
}

func TestServiceDetailRowsMinimal(t *testing.T) {
	svc := &api.Service{
		ID:   "svc-min",
		Name: "basic",
		Slug: "basic",
	}

	rows := serviceDetailRows(svc)

	// Should have exactly 3 rows (ID, Name, Slug) - no optional fields
	if len(rows) != 3 {
		t.Errorf("expected 3 rows, got %d", len(rows))
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
