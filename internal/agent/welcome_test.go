package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/uuid"
	"github.com/oapi-codegen/nullable"
	rootly "github.com/rootlyhq/rootly-go"
)

func TestWelcomeBannerIncludesVersionUserTeamAndHost(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{
		NoColor: true,
		Version: "v1.2.3",
		APIHost: "api.rootly.com",
		CurrentUser: currentUserForWelcome(
			"Avery Rootly",
			"avery@example.com",
			"[Incident Response] Avery Rootly",
		),
	})

	banner := ansi.Strip(model.welcomeBanner())
	for _, expected := range []string{
		"        ▄",
		"       ███",
		"   ███ ▀█▀ ███",
		"    ▀█▀   ▀█▀",
		" ▄██▄   ▄   ▄██▄",
		"  ▀█▀  ███  ▀█▀",
		" ▄▄▄    ▀    ▄▄▄",
		"███████▄ ▄███████",
		"     ▀▀███▀▀",
		"       ███",
		"Rootly AI",
		"v1.2.3",
		"Avery Rootly · Incident Response",
		"api.rootly.com",
	} {
		if !strings.Contains(banner, expected) {
			t.Errorf("welcome banner = %q, want %q", banner, expected)
		}
	}
	if strings.Contains(banner, "vv1.2.3") {
		t.Errorf("welcome banner = %q, want one leading v", banner)
	}
	bannerLines := strings.Split(strings.TrimSuffix(banner, "\n"), "\n")
	if len(bannerLines) != welcomeLogoHeight {
		t.Errorf("welcome banner rows = %d, want %d", len(bannerLines), welcomeLogoHeight)
	}
	for index, line := range strings.Split(sproutLogo(true), "\n") {
		if width := lipgloss.Width(line); width != welcomeLogoWidth {
			t.Errorf("sprout row %d width = %d, want %d", index+1, width, welcomeLogoWidth)
		}
	}
	model.Init()
	if !model.welcomePrinted {
		t.Error("no-color welcome banner was not printed during Init")
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
	model.Init()
	if strings.Contains(banner, "\x1b") || model.welcomeAnimating || !model.welcomePrinted {
		t.Errorf("no-color welcome banner = %q, animating = %t", banner, model.welcomeAnimating)
	}
	if !strings.Contains(banner, "Avery Rootly") {
		t.Errorf("welcome banner = %q, want safe user name text", banner)
	}
}

func TestWelcomeBannerAnimationSettlesAndPrintsOnce(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{Version: "1.2.3"})
	animationDuration := time.Duration(welcomeSettleFrame) * welcomeBannerFrameInterval
	if animationDuration <= time.Second || animationDuration > 2200*time.Millisecond {
		t.Fatalf("welcome animation duration = %s, want >1s and <=2.2s", animationDuration)
	}
	_, _ = model.Update(tea.ColorProfileMsg{Profile: colorprofile.TrueColor})
	model.Init()
	if !model.welcomeAnimating || model.welcomePrinted {
		t.Fatalf("animation started = %t, banner printed = %t; want animation without printing", model.welcomeAnimating, model.welcomePrinted)
	}
	view := ansi.Strip(model.View().Content)
	if !strings.Contains(view, "Rootly AI") || strings.HasPrefix(view, "\n") {
		t.Errorf("animated view = %q, want inline banner and no leading padding", view)
	}

	for frame := 0; frame < welcomeSettleFrame-1; frame++ {
		_, _ = model.Update(welcomeBannerTickMsg{})
		if !model.welcomeAnimating || model.welcomePrinted {
			t.Fatalf("animation settled early at frame %d", frame+1)
		}
	}
	_, command := model.Update(welcomeBannerTickMsg{})
	if model.welcomeAnimating || !model.welcomePrinted {
		t.Fatal("animation did not settle at the expected frame")
	}
	if got := strings.Count(commandSequenceOutput(command), "Rootly AI"); got != 1 {
		t.Errorf("settled banner command printed Rootly AI %d times, want once", got)
	}
	_, _ = model.Update(welcomeBannerTickMsg{})
	if !model.welcomePrinted {
		t.Error("an extra tick unset the printed banner state")
	}
}

func TestEarlyPromptSettlesWelcomeBannerBeforePromptEcho(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{responses: [][]rootly.AiChatStreamEvent{{}}}, TUIOptions{})
	model.width = 100
	model.Init()
	_, _ = model.Update(tea.ColorProfileMsg{Profile: colorprofile.TrueColor})
	model.composer.SetValue("hi")

	_ = model.submit()
	if model.welcomeAnimating || !model.welcomePrinted {
		t.Fatal("early prompt did not settle the welcome banner")
	}
}

func TestQuitLeavesNoPinnedFrame(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.width = 100
	model.height = 30
	model.Init()
	model.quitCommand()

	if !model.quitting {
		t.Fatal("quit command did not mark the model as quitting")
	}
	if view := model.View().Content; view != "" {
		t.Errorf("quitting view = %q, want empty view", view)
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

func TestTUIViewDoesNotPadComposerToTerminalBottom(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.width = 100
	model.height = 24

	view := model.View().Content
	if got := lipgloss.Height(view); got != 4 {
		t.Fatalf("idle view height = %d, want frame height 4", got)
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

func TestTUIConfirmingViewHasNoPaddingOrComposerHint(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.height = 24
	model.state = tuiConfirming
	model.pending = &rootly.AiChatStreamConfirmationRequired{Question: "Approve this action?"}

	view := model.View().Content
	if got := lipgloss.Height(view); got >= model.height {
		t.Fatalf("confirming view height = %d, want no padding within %d", got, model.height)
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
