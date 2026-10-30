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
		"        ▄█▄",
		"    ▄▄  ███  ▄▄",
		"    ▀██  ▀  ██▀",
		" ▄▄▄▄    ▄    ▄▄▄▄",
		"  ▀▀▀   ███   ▀▀▀",
		"▄▄▄▄▄▄▄  ▀  ▄▄▄▄▄▄▄",
		"▀▀▀▀▀▀▀██▄██▀▀▀▀▀▀▀",
		"        ███",
		"Rootly AI",
		"v1.2.3",
		"Avery Rootly · Incident Response",
		"api.rootly.com",
	} {
		if !strings.Contains(banner, expected) {
			t.Errorf("welcome banner = %q, want %q", banner, expected)
		}
	}
	bannerLines := strings.Split(strings.TrimSuffix(banner, "\n"), "\n")
	if len(bannerLines) != 8 {
		t.Errorf("welcome banner rows = %d, want 8", len(bannerLines))
	}
	for index, line := range strings.Split(sproutLogo(true), "\n") {
		if width := lipgloss.Width(line); width != 19 {
			t.Errorf("sprout row %d width = %d, want 19", index+1, width)
		}
	}
	model.Init()
	if model.printedRows != 9 {
		t.Errorf("printed rows after welcome Init = %d, want 9 including the banner's final blank row", model.printedRows)
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
	if strings.Contains(view, "Rootly AI") {
		t.Errorf("idle view contains welcome banner, which should be printed once to scrollback: %q", view)
	}
	rule := strings.Repeat("─", model.width)
	if got := strings.Count(view, rule); got != 2 {
		t.Errorf("idle view has %d full-width rules, want 2: %q", got, view)
	}
}

func TestTUIViewPaddingAccountsForPrintedAndWrappedRows(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.width = 20
	model.height = 24
	model.println("one\ntwo")

	if model.printedRows != 2 {
		t.Fatalf("printed rows = %d, want 2", model.printedRows)
	}
	if got := lipgloss.Height(model.View().Content); got != model.height-2 {
		t.Errorf("view height after two printed rows = %d, want %d", got, model.height-2)
	}

	wrapped := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	wrapped.width = 20
	wrapped.height = 24
	wrapped.println(strings.Repeat("x", 41))
	if wrapped.printedRows != 3 {
		t.Fatalf("printed rows for a wrapped line = %d, want 3", wrapped.printedRows)
	}
	if got := lipgloss.Height(wrapped.View().Content); got != wrapped.height-3 {
		t.Errorf("view height after wrapped output = %d, want %d", got, wrapped.height-3)
	}

	full := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	full.width = 20
	full.height = 24
	full.printedRows = 24
	bodyHeight := lipgloss.Height(full.View().Content)
	full.printedRows = 0
	full.println(strings.Repeat("row\n", 24))
	if got := lipgloss.Height(full.View().Content); got != bodyHeight {
		t.Errorf("view height after filling scrollback = %d, want body height %d", got, bodyHeight)
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
