package teams

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

func TestTeamDetailRows(t *testing.T) {
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	team := &api.Team{
		ID:          "team-123",
		Name:        "Platform",
		Slug:        "platform",
		Description: "Platform engineering team",
		Color:       "#3498DB",
		Users:       []string{"Alice", "Bob", "Charlie"},
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	rows := teamDetailRows(team)

	if len(rows) == 0 {
		t.Fatal("expected non-empty rows")
	}

	if findRow(rows, "ID") != "team-123" {
		t.Errorf("ID = %q, want %q", findRow(rows, "ID"), "team-123")
	}
	if findRow(rows, "Name") != "Platform" {
		t.Errorf("Name = %q, want %q", findRow(rows, "Name"), "Platform")
	}
	if findRow(rows, "Users") != "Alice, Bob, Charlie" {
		t.Errorf("Users = %q, want %q", findRow(rows, "Users"), "Alice, Bob, Charlie")
	}
	if findRow(rows, "Color") != "#3498DB" {
		t.Errorf("Color = %q, want %q", findRow(rows, "Color"), "#3498DB")
	}
}

func TestTeamDetailRowsMinimal(t *testing.T) {
	team := &api.Team{
		ID:   "team-min",
		Name: "basic",
		Slug: "basic",
	}

	rows := teamDetailRows(team)

	// Should have exactly 3 rows (ID, Name, Slug)
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
