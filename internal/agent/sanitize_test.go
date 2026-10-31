package agent

import (
	"strings"
	"testing"
)

func TestSanitizeTerminalTextRemovesControlsAndBidiCharacters(t *testing.T) {
	input := "\x1b]52;c;ZXZpbA==\x07\x1b[2J\x1b]8;;https://evil.example\x07link\x1b]8;;\x07\r\x7f\u0080\u009f\u061c\u200e\u200f\u202a\u202e\u2066\u2069\n\tcafé 😀"
	want := "link\n\tcafé 😀"

	if got := sanitizeTerminalText(input); got != want {
		t.Errorf("sanitizeTerminalText() = %q, want %q", got, want)
	}
}

func TestStatusResponseErrorSanitizesServerMessage(t *testing.T) {
	err := StatusResponseError(400, []byte(`{"errors":[{"title":"bad \u001b]52;c;ZXZpbA==\u0007 \u001b[2J"}]}`))
	if err == nil {
		t.Fatal("StatusResponseError() returned nil")
	}
	for _, unwanted := range []string{"]52;", "[2J", "\x1b"} {
		if strings.Contains(err.Error(), unwanted) {
			t.Errorf("error = %q, want no %q", err, unwanted)
		}
	}
}

func TestSanitizeTerminalTextRemovesC1ControlsIndividually(t *testing.T) {
	if got, want := sanitizeTerminalText("\u009dvisible\u009c"), "visible"; got != want {
		t.Errorf("sanitizeTerminalText() = %q, want %q", got, want)
	}
	if got, want := sanitizeTerminalText(string([]byte{0x9b, 'v', 'i', 's', 'i', 'b', 'l', 'e'})), "visible"; got != want {
		t.Errorf("sanitizeTerminalText() with a raw C1 byte = %q, want %q", got, want)
	}
}
