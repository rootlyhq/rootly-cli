package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/uuid"
	"github.com/oapi-codegen/nullable"
	rootly "github.com/rootlyhq/rootly-go"
)

func TestWelcomeBannerIncludesVersionUserTeamAndHost(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{
		NoColor: true,
		Version: "1.2.3",
		APIHost: "api.rootly.com",
		CurrentUser: currentUserForWelcome(
			"Avery Rootly",
			"avery@example.com",
			"[Incident Response] Avery Rootly",
		),
	})

	banner := ansi.Strip(model.welcomeBanner())
	for _, expected := range []string{
		" ▄▀▀▀▄",
		"█ ● ● █",
		" ▀▄▄▄▀",
		"Rootly AI",
		"v1.2.3",
		"Avery Rootly · Incident Response",
		"api.rootly.com",
	} {
		if !strings.Contains(banner, expected) {
			t.Errorf("welcome banner = %q, want %q", banner, expected)
		}
	}
}

func TestWelcomeBannerNoColorAndSanitizesUser(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{
		NoColor: true,
		Version: "dev",
		APIHost: "api.rootly.com",
		CurrentUser: currentUserForWelcome(
			"Avery \x1b]52;c;ZXZpbA==\x07\x1b[2JRootly",
			"avery@example.com",
			"",
		),
	})

	banner := model.welcomeBanner()
	for _, unwanted := range []string{"\x1b", "]52;", "[2J"} {
		if strings.Contains(banner, unwanted) {
			t.Errorf("welcome banner = %q, want no %q", banner, unwanted)
		}
	}
	if strings.Contains(banner, "vdev") {
		t.Errorf("welcome banner = %q, want development version omitted", banner)
	}
	if !strings.Contains(banner, "Avery Rootly") {
		t.Errorf("welcome banner = %q, want safe user name text", banner)
	}
}

func TestWelcomeBannerWithNilUserStillShowsTitleAndHost(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{
		NoColor: true,
		APIHost: "api.rootly.com",
	})

	banner := model.welcomeBanner()
	if !strings.Contains(banner, "Rootly AI") || !strings.Contains(banner, "api.rootly.com") {
		t.Errorf("welcome banner = %q, want title and API host", banner)
	}
	if strings.Contains(banner, " · ") {
		t.Errorf("welcome banner = %q, want no user row text when user is nil", banner)
	}
}

func TestTUIViewPinsComposerHintToTerminalBottom(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.width = 100
	model.height = 24

	view := model.View().Content
	if got := lipgloss.Height(view); got != model.height {
		t.Fatalf("idle view height = %d, want %d", got, model.height)
	}
	if got := lastLine(view); got != "/ for commands · exit to quit" {
		t.Errorf("last idle view line = %q, want composer hint", got)
	}
	lines := strings.Split(view, "\n")
	if !strings.Contains(lines[0], "▄▀▀▀▄") || !strings.Contains(lines[0], "Rootly AI") {
		t.Errorf("first idle view line = %q, want the welcome logo and title", lines[0])
	}
	if lines[3] != "" {
		t.Errorf("line after welcome banner = %q, want one blank line", lines[3])
	}
	rule := strings.Repeat("─", model.width)
	if got := strings.Count(view, rule); got != 2 {
		t.Errorf("idle view has %d full-width rules, want 2: %q", got, view)
	}
}

func TestTUIViewDoesNotPadTallBody(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.width = 100
	model.height = 24
	model.status = strings.Repeat("status line\n", 30)

	view := model.View().Content
	if got := lipgloss.Height(view); got <= model.height {
		t.Errorf("tall view height = %d, want greater than %d", got, model.height)
	}
	if strings.HasPrefix(view, "\n") {
		t.Errorf("tall view has leading padding: %q", view)
	}
}

func TestTUIConfirmingViewAnchorsCardWithoutComposerHint(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.height = 24
	model.state = tuiConfirming
	model.pending = &rootly.AiChatStreamConfirmationRequired{Question: "Approve this action?"}

	view := model.View().Content
	if got := lipgloss.Height(view); got != model.height {
		t.Fatalf("confirming view height = %d, want %d", got, model.height)
	}
	if got := lastLine(view); !strings.Contains(got, "y approve") {
		t.Errorf("last confirming view line = %q, want confirmation card footer", got)
	}
	if strings.Contains(view, "/ for commands · exit to quit") {
		t.Errorf("confirming view includes composer hint: %q", view)
	}
}

func TestTUIStatusVerbFooterAndSpinner(t *testing.T) {
	now := time.Date(2025, time.May, 3, 15, 4, 5, 0, time.UTC)
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.now = func() time.Time { return now }
	model.verbIndex = func(int) int { return 3 }
	sessionID := testSessionIDValue()
	model.sessionID = &sessionID
	model.startNewTurn()
	now = now.Add(2 * time.Second)

	if got, want := model.turnFooter(), fmt.Sprintf("✻ Untangled for 2s · done 3:04 PM · session %s", shortSessionID(testSessionID)); got != want {
		t.Errorf("turn footer = %q, want %q", got, want)
	}

	model.state = tuiStreaming
	model.startedAt = model.turnStartedAt
	if view := ansi.Strip(model.View().Content); !strings.Contains(view, "Untangling…") {
		t.Errorf("streaming view = %q, want active verb", view)
	}
	model.status = "Get incidents..."
	view := ansi.Strip(model.View().Content)
	if !strings.Contains(view, "Get incidents...") || strings.Contains(view, "Untangling…") {
		t.Errorf("streaming view = %q, want server status to override active verb", view)
	}

	now = model.turnStartedAt.Add(65 * time.Second)
	if got, want := model.turnFooter(), fmt.Sprintf("✻ Untangled for 1m 5s · done 3:05 PM · session %s", shortSessionID(testSessionID)); got != want {
		t.Errorf("long turn footer = %q, want %q", got, want)
	}
}

func currentUserForWelcome(name, email, fullNameWithTeam string) *rootly.CurrentUserResponse {
	user := &rootly.CurrentUserResponse{}
	user.Data.Attributes.Email = email
	user.Data.Attributes.FullName = nullable.NewNullableWithValue(name)
	if fullNameWithTeam != "" {
		user.Data.Attributes.FullNameWithTeam = nullable.NewNullableWithValue(fullNameWithTeam)
	}
	return user
}

func testSessionIDValue() uuid.UUID {
	return uuid.MustParse(testSessionID)
}

func lastLine(text string) string {
	lines := strings.Split(text, "\n")
	return lines[len(lines)-1]
}
