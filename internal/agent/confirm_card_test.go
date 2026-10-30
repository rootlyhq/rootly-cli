package agent

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	rootly "github.com/rootlyhq/rootly-go"
)

func TestRenderConfirmationCard(t *testing.T) {
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	confirmation := &rootly.AiChatStreamConfirmationRequired{
		Question: "Create the incident?",
		Preview: []byte(`{
			"title":"Incident",
			"summary":"Create an incident from this alert.",
			"context_text":"Triggered by the on-call team.",
			"fields":[
				{"label":"Title","value":"Database unavailable"},
				{"label":"Severity","value":"P1","inferred":true}
			],
			"destructive":true,
			"inferred_fields":["Owner"]
		}`),
		MissingFields: []byte(`[{"label":"Service"},{"name":"Environment"}]`),
		ExpiresAt:     now.Add(4*time.Minute + 2*time.Second).Format(time.RFC3339),
	}

	colored := renderConfirmationCard(confirmation, 40, false, now)
	flattened := strings.Join(strings.Fields(ansi.Strip(colored)), " ")
	for _, expected := range []string{
		"Create the incident?",
		"Incident",
		"Create an incident from this",
		"alert.",
		"Triggered by the on-call team.",
		"Title: Database unavailable",
		"Severity: P1",
		"(inferred)",
		"Missing: Service, Environment",
		"DESTRUCTIVE",
		"expires in 5m",
		"╭",
	} {
		if !strings.Contains(flattened, expected) {
			t.Errorf("colored card = %q, want it to contain %q", colored, expected)
		}
	}
	for _, line := range strings.Split(colored, "\n") {
		if width := lipgloss.Width(line); width > 40 {
			t.Errorf("colored card line width = %d, want at most 40: %q", width, line)
		}
	}

	plain := renderConfirmationCard(confirmation, 40, true, now)
	if strings.Contains(plain, "\x1b[") || strings.Contains(plain, "╭") {
		t.Errorf("no-color card contains terminal styling: %q", plain)
	}
	if !strings.HasPrefix(plain, "  Create the incident?") || !strings.Contains(plain, "  Severity: P1 (inferred)") {
		t.Errorf("no-color card = %q, want indented plain text", plain)
	}
}

func TestRenderConfirmationCardFallsBackToIndentedJSON(t *testing.T) {
	for _, preview := range [][]byte{
		[]byte(`{"name":"api"}`),
		[]byte(`not-json`),
	} {
		confirmation := &rootly.AiChatStreamConfirmationRequired{
			Question: "Approve?",
			Preview:  preview,
		}
		got := renderConfirmationCard(confirmation, 80, true, time.Now())
		if !strings.HasPrefix(got, "  Approve?\n  ") {
			t.Errorf("card = %q, want question and indented fallback", got)
		}
	}
}

func TestRenderConfirmationCardRendersMissingFieldsWithoutPreviewFields(t *testing.T) {
	confirmation := &rootly.AiChatStreamConfirmationRequired{
		Question:      "Approve?",
		Preview:       []byte(`{"title":"Create"}`),
		MissingFields: []byte(`[{"label":"Service \u001b[2J"}]`),
	}

	got := renderConfirmationCard(confirmation, 80, true, time.Now())
	if !strings.Contains(got, "Missing: Service") {
		t.Errorf("card = %q, want missing fields in the fallback branch", got)
	}
	if strings.Contains(got, "\x1b") {
		t.Errorf("card = %q, want sanitized missing fields", got)
	}
}

func TestRenderConfirmationCardOmitsExpiryWhenUnset(t *testing.T) {
	got := renderConfirmationCard(&rootly.AiChatStreamConfirmationRequired{
		Question: "Approve?",
	}, 80, true, time.Now())
	if strings.Contains(got, "expires in") {
		t.Errorf("card = %q, want no expiry countdown", got)
	}
}

func TestRenderConfirmationCardSanitizesTerminalControls(t *testing.T) {
	const attack = "\x1b]52;c;ZXZpbA==\x07\x1b[2J\x1b]8;;https://evil.example\x07"
	confirmation := &rootly.AiChatStreamConfirmationRequired{
		Question: "Approve " + attack,
		Preview: []byte(`{
			"title":"Title \u001b]52;c;ZXZpbA==\u0007\u001b[2J",
			"summary":"Summary \u001b]52;c;ZXZpbA==\u0007",
			"context_text":"Context \u001b[2J",
			"fields":[{"label":"Label \u001b]52;c;ZXZpbA==\u0007","value":{"key":"Value \u001b]52;c;ZXZpbA==\u0007"}}]
		}`),
		MissingFields: []byte(`["Missing \u001b]52;c;ZXZpbA==\u0007"]`),
	}
	card := renderConfirmationCard(confirmation, 80, true, time.Now())
	for _, unwanted := range []string{"]52;", "[2J", "\x1b]8;;", "\x1b"} {
		if strings.Contains(card, unwanted) {
			t.Errorf("confirmation card = %q, want no %q", card, unwanted)
		}
	}
}
