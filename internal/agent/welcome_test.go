package agent

import (
	"context"
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

	"github.com/rootlyhq/rootly-cli/internal/api"
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
		"     ▄█       ",
		"    ▄▀ █      ",
		"▀▀▀▀▀  █ ▄▀▀▀▀",
		"       ▀█     ",
		"Rootly AI",
		"v1.2.3",
		"Avery Rootly · Incident Response",
		"api.rootly.com",
		"New session",
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
	for index, expected := range welcomeLogoLines {
		if !strings.HasPrefix(bannerLines[index], expected) {
			t.Errorf("welcome banner row %d = %q, want logo row %q at its start", index+1, bannerLines[index], expected)
		}
	}
	for index, line := range strings.Split(pulseLogo(true), "\n") {
		if width := lipgloss.Width(line); width != welcomeLogoWidth {
			t.Errorf("pulse row %d width = %d, want %d", index+1, width, welcomeLogoWidth)
		}
	}
	model.Init()
	if model.welcomeAnimating {
		t.Error("no-color welcome banner should be static")
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

	textRows := model.welcomeTextRows()
	if strings.Contains(textRows, "\x1b") {
		t.Errorf("welcome text rows = %q, want no ANSI escapes in no-color mode", textRows)
	}

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
	if strings.Contains(banner, "\x1b") || model.welcomeAnimating {
		t.Errorf("no-color welcome banner = %q, animating = %t", banner, model.welcomeAnimating)
	}
	if !strings.Contains(banner, "Avery Rootly") {
		t.Errorf("welcome banner = %q, want safe user name text", banner)
	}
}

func TestWelcomeTitleStyling(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{Version: "1.2.3"})
	title := strings.Split(model.welcomeTextRows(), "\n")[0]
	if got := ansi.Strip(title); got != "✦ Rootly AI v1.2.3" {
		t.Fatalf("plain title = %q, want title and version", got)
	}

	orange := strings.Index(title, "\x1b[38;2;245;162;79m")
	bold := strings.Index(title, "\x1b[1m")
	faint := strings.Index(title, "\x1b[2m")
	if orange < 0 || bold < 0 {
		t.Fatalf("title = %q, want orange and bold SGR styling", title)
	}
	if faint < 0 || faint < orange || faint < bold {
		t.Errorf("title = %q, want only the version faint after the accented title", title)
	}
}

func TestWelcomeBannerAnimationSettlesInPlace(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{Version: "1.2.3"})
	animationDuration := time.Duration(welcomeSettleFrame) * welcomeBannerFrameInterval
	if animationDuration <= 0 || animationDuration > 2200*time.Millisecond {
		t.Fatalf("welcome animation duration = %s, want >0 and <=2.2s", animationDuration)
	}
	_, _ = model.Update(tea.ColorProfileMsg{Profile: colorprofile.TrueColor})
	model.Init()
	if !model.welcomeAnimating {
		t.Fatal("welcome animation did not start")
	}
	_, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	view := ansi.Strip(model.View().Content)
	if !strings.Contains(view, "Rootly AI") || strings.HasPrefix(view, "\n") {
		t.Errorf("animated view = %q, want in-place header and no leading padding", view)
	}

	for frame := 0; frame < welcomeSettleFrame-1; frame++ {
		_, _ = model.Update(welcomeBannerTickMsg{})
		if !model.welcomeAnimating {
			t.Fatalf("animation settled early at frame %d", frame+1)
		}
	}
	_, _ = model.Update(welcomeBannerTickMsg{})
	if model.welcomeAnimating {
		t.Fatal("animation did not settle at the expected frame")
	}
	_, _ = model.Update(welcomeBannerTickMsg{})
	if model.welcomeAnimating {
		t.Error("an extra tick restarted the animation")
	}
}

func TestWelcomeAnimationRestartsForNewSessionCommands(t *testing.T) {
	for _, command := range []string{"/new", "/clear"} {
		t.Run(command, func(t *testing.T) {
			model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
			_, _ = model.Update(tea.ColorProfileMsg{Profile: colorprofile.TrueColor})
			model.Init()
			settleWelcomeAnimationForTest(t, model)

			cmd := model.runSlashCommand(command)
			if !model.welcomeAnimating || model.welcomeFrame != 0 || cmd == nil {
				t.Fatalf("after %s: animating=%t frame=%d cmd=%v; want restarted animation and tick", command, model.welcomeAnimating, model.welcomeFrame, cmd)
			}
			for range welcomeSettleFrame {
				_, _ = model.Update(welcomeBannerTickMsg{generation: model.welcomeGeneration})
			}
			if model.welcomeAnimating {
				t.Errorf("animation did not settle after %d current-generation ticks", welcomeSettleFrame)
			}
		})
	}
}

func TestWelcomeAnimationRestartIgnoresStaleTick(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
	_, _ = model.Update(tea.ColorProfileMsg{Profile: colorprofile.TrueColor})
	model.Init()
	if !model.welcomeAnimating {
		t.Fatal("welcome animation did not start")
	}
	oldGeneration := model.welcomeGeneration

	if cmd := model.restartWelcomeAnimation(); cmd == nil {
		t.Fatal("restart returned no tick command")
	}
	if model.welcomeGeneration == oldGeneration || model.welcomeFrame != 0 || !model.welcomeAnimating {
		t.Fatalf("restart state: generation=%d frame=%d animating=%t", model.welcomeGeneration, model.welcomeFrame, model.welcomeAnimating)
	}
	_, _ = model.Update(welcomeBannerTickMsg{generation: oldGeneration})
	if model.welcomeFrame != 0 {
		t.Errorf("stale tick advanced frame to %d, want 0", model.welcomeFrame)
	}
}

func TestWelcomeAnimationDoesNotRestartWithoutColor(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.Init()
	if cmd := model.runSlashCommand("/new"); cmd != nil {
		t.Errorf("no-color /new command = %v, want no tick", cmd)
	}
	if model.welcomeAnimating {
		t.Error("no-color /new started the welcome animation")
	}
}

func settleWelcomeAnimationForTest(t *testing.T, model *TUIModel) {
	t.Helper()
	for frame := 0; frame < welcomeSettleFrame; frame++ {
		_, _ = model.Update(welcomeBannerTickMsg{generation: model.welcomeGeneration})
	}
	if model.welcomeAnimating {
		t.Fatalf("welcome animation remained active after %d ticks", welcomeSettleFrame)
	}
}

func TestWelcomeGlimmerReachesLogoEdges(t *testing.T) {
	topLeftColumn := -1
	for column, character := range []rune(welcomeLogoLines[0]) {
		if character != ' ' {
			topLeftColumn = column
			break
		}
	}
	bottomRow := len(welcomeLogoLines) - 1
	bottomRightColumn := -1
	for column, character := range []rune(welcomeLogoLines[bottomRow]) {
		if character != ' ' {
			bottomRightColumn = column
		}
	}
	if topLeftColumn < 0 || bottomRightColumn < 0 {
		t.Fatal("welcome logo has no edge cells")
	}

	for _, cell := range []struct {
		name   string
		row    int
		column int
	}{
		{name: "top-left", row: 0, column: topLeftColumn},
		{name: "bottom-right", row: bottomRow, column: bottomRightColumn},
	} {
		t.Run(cell.name, func(t *testing.T) {
			for frame := welcomeDrawFrames + 1; frame <= welcomeDrawFrames+welcomeGlimmerFrames(); frame++ {
				if color := welcomeCellColor(frame, cell.row, cell.column); color != "" && color != "#7C5CE6" {
					return
				}
			}
			t.Errorf("cell (%d,%d) never gets a glimmer color", cell.row, cell.column)
		})
	}
}

func TestWelcomePulseDrawsLeftToRight(t *testing.T) {
	midDrawFrame := welcomeDrawFrames / 2
	if color := welcomeCellColor(midDrawFrame, 2, 0); color == "" {
		t.Error("left edge of pulse is still blank midway through drawing")
	}
	if color := welcomeCellColor(midDrawFrame, 2, 13); color != "" {
		t.Errorf("right edge of pulse has color %q midway through drawing, want blank", color)
	}
	for _, column := range []int{0, 13} {
		if color := welcomeCellColor(welcomeSettleFrame, 2, column); color != "#7C5CE6" {
			t.Errorf("settled cell (2,%d) color = %q, want #7C5CE6", column, color)
		}
	}
}

func TestWelcomeGlimmerStartsAtLeftEdge(t *testing.T) {
	for frame := welcomeDrawFrames + 1; frame <= welcomeDrawFrames+welcomeGlimmerFrames(); frame++ {
		color := welcomeCellColor(frame, 2, 0)
		if color == "" || color == "#7C5CE6" {
			continue
		}
		if rightColor := welcomeCellColor(frame, 2, 13); rightColor != "#7C5CE6" {
			t.Errorf("right edge glimmer color = %q at frame %d, want settled purple while beat starts left", rightColor, frame)
		}
		return
	}
	t.Fatal("no glimmer frame lights the left edge with a non-settled color")
}

func TestEarlyPromptSettlesWelcomeBannerBeforePromptEcho(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{responses: [][]rootly.AiChatStreamEvent{{}}}, TUIOptions{})
	model.width = 100
	model.Init()
	_, _ = model.Update(tea.ColorProfileMsg{Profile: colorprofile.TrueColor})
	model.composer.SetValue("hi")

	_ = model.submit()
	if model.welcomeAnimating {
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

func TestTUIViewUsesAltScreenAndFixedFooterDuringConfirmation(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	view := model.View()
	if !view.AltScreen || view.MouseMode != tea.MouseModeCellMotion {
		t.Errorf("view alt screen/mouse mode = %t/%v, want alt screen and cell-motion mouse", view.AltScreen, view.MouseMode)
	}
	model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model.state = tuiConfirming
	model.pending = &rootly.AiChatStreamConfirmationRequired{Question: "Approve this action?"}

	content := model.View().Content
	if got := lipgloss.Height(content); got != model.height {
		t.Fatalf("confirming view height = %d, want %d", got, model.height)
	}
	if got := lastLine(content); !strings.Contains(got, "/ for commands · exit to quit") {
		t.Errorf("last confirming row = %q, want fixed composer hint", got)
	}
	if !strings.Contains(content, "Approve this action?") {
		t.Errorf("confirming view = %q, want confirmation card", content)
	}
}

func TestWelcomeHeaderContextUsesSessionBoundaryAndBinding(t *testing.T) {
	sessionID := testSessionIDValue()
	tests := []struct {
		name    string
		options TUIOptions
		want    string
	}{
		{name: "new", want: "New session"},
		{name: "resumed title", options: TUIOptions{SessionID: &sessionID, SessionTitle: "Incident review"}, want: "Incident review"},
		{name: "incident binding", options: TUIOptions{Incident: &api.Incident{SequentialID: "INC-123", Title: "Database latency"}}, want: "INC-123 · Database latency"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := test.options
			options.NoColor = true
			model := NewTUIModel(context.Background(), &fakeStreamer{}, options)
			lines := strings.Split(model.welcomeTextRows(), "\n")
			if len(lines) != 4 {
				t.Fatalf("header text rows = %d, want four: %q", len(lines), lines)
			}
			if lines[3] != test.want {
				t.Errorf("header context = %q, want %q", lines[3], test.want)
			}
		})
	}
}

func TestWelcomeHeaderOmitsLogoBelowFiftyColumns(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.width = 49
	lines := strings.Split(strings.TrimSuffix(model.welcomeBanner(), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("narrow header rows = %d, want four: %q", len(lines), lines)
	}
	if strings.Contains(strings.Join(lines, "\n"), "▄") {
		t.Errorf("narrow header includes logo: %q", lines)
	}
}

func TestTUIStatusVerbFooterAndSpinner(t *testing.T) {
	now := time.Date(2025, time.May, 3, 15, 4, 5, 0, time.UTC)
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model.now = func() time.Time { return now }
	model.verbIndex = func(int) int { return 3 }
	sessionID := testSessionIDValue()
	model.sessionID = &sessionID
	model.startNewTurn()
	now = now.Add(2 * time.Second)

	if got, want := model.turnFooter(), "✻ Untangled for 2s · done 3:04 PM"; got != want {
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
	if got, want := model.turnFooter(), "✻ Untangled for 1m 5s · done 3:05 PM"; got != want {
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
