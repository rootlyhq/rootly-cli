package agent

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/uuid"
	"github.com/oapi-codegen/nullable"
	rootly "github.com/rootlyhq/rootly-go"

	"github.com/rootlyhq/rootly-cli/internal/mcp"
)

func TestFocusReportingSwapsCursorWithoutTouchingComposer(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model.Init()
	model.composer.SetValue("draft")

	if view := model.View(); !view.ReportFocus {
		t.Fatal("View().ReportFocus = false, want focus reporting on")
	}
	model.Update(tea.BlurMsg{})
	if !model.terminalBlurred {
		t.Fatal("terminalBlurred = false after BlurMsg")
	}
	if !strings.Contains(ansi.Strip(model.composerView()), blurredCursorGlyph) {
		t.Errorf("blurred composer = %q, want hollow cursor %q", model.composerView(), blurredCursorGlyph)
	}
	model.Update(tea.FocusMsg{})
	if model.terminalBlurred {
		t.Fatal("terminalBlurred = true after FocusMsg")
	}
	if strings.Contains(model.composerView(), blurredCursorGlyph) {
		t.Errorf("focused composer = %q, want no hollow cursor", model.composerView())
	}
	if got := model.composer.Value(); got != "draft" {
		t.Errorf("composer value = %q after focus events, want %q", got, "draft")
	}
	if !model.composer.Focused() {
		t.Error("composer lost focus after terminal focus events")
	}
}

func TestOverlayGlyphPlacesCursor(t *testing.T) {
	if got := overlayBlurredCursor("ab\ncd", 1, 1); got != "ab\nc▢" {
		t.Errorf("overlayBlurredCursor() = %q", got)
	}
	if got := overlayBlurredCursor("ab", 4, 0); got != "ab  ▢" {
		t.Errorf("overlayBlurredCursor() past end = %q", got)
	}
	if got := overlayBlurredCursor("ab", 0, 3); got != "ab" {
		t.Errorf("overlayBlurredCursor() out of range = %q", got)
	}
}

func TestTrailingBackslashEnterInsertsNewline(t *testing.T) {
	streamer := &fakeStreamer{}
	model := NewTUIModel(context.Background(), streamer, TUIOptions{NoColor: true})
	model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model.Init()
	model.composer.SetValue(`first line\`)

	model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := model.composer.Value(); got != "first line\n" {
		t.Fatalf("composer value = %q, want continuation newline", got)
	}
	if len(streamer.calls) != 0 || strings.Contains(model.Transcript(), "first line") {
		t.Fatalf("continuation submitted the prompt (calls=%d)", len(streamer.calls))
	}

	model.composer.SetValue(`path C:\\`)
	if model.continueLine() {
		t.Error("escaped backslash continued the line, want submit")
	}
	model.composer.SetValue(`path C:\\\`)
	if !model.continueLine() || model.composer.Value() != "path C:\\\\\n" {
		t.Errorf("three trailing backslashes = %q, want escaped pair plus newline", model.composer.Value())
	}
}

func TestComposerOverflowShowsMoreLinesHint(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	model.Init()
	lines := make([]string, 10)
	for index := range lines {
		lines[index] = "line"
	}
	model.composer.SetValue(strings.Join(lines, "\n"))
	model.composer.CursorEnd()
	model.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	hidden := model.composer.ScrollYOffset()
	if hidden == 0 {
		t.Fatalf("composer ScrollYOffset = 0 with %d lines and height %d", len(lines), model.composer.Height())
	}
	footer := ansi.Strip(model.footerContent())
	if want := moreLinesHint(hidden); !strings.Contains(footer, want) {
		t.Errorf("footer = %q, want %q", footer, want)
	}
	if got := moreLinesHint(1); got != "↑ 1 more line" {
		t.Errorf("moreLinesHint(1) = %q", got)
	}
	if got := moreLinesHint(4); got != "↑ 4 more lines" {
		t.Errorf("moreLinesHint(4) = %q", got)
	}

	model.composer.SetValue("short")
	if footer := ansi.Strip(model.footerContent()); strings.Contains(footer, "more line") {
		t.Errorf("footer = %q, want no hint when everything fits", footer)
	}
}

func TestBulletizeWideTablesOnlyWhenWiderThanTerminal(t *testing.T) {
	table := "| Service | Owner | Status |\n|---|---|---|\n| api | Platform team | degraded |\n| web | Frontend | ok |"

	if got := bulletizeWideTables(table, 80); got != table {
		t.Errorf("narrow table changed:\n%s", got)
	}
	got := bulletizeWideTables("Summary\n\n"+table+"\n\nDone", 20)
	want := "Summary\n\n- **api** · Owner: Platform team · Status: degraded\n- **web** · Owner: Frontend · Status: ok\n\nDone"
	if got != want {
		t.Errorf("wide table =\n%s\nwant\n%s", got, want)
	}

	fenced := "```\n" + table + "\n```"
	if got := bulletizeWideTables(fenced, 10); got != fenced {
		t.Errorf("fenced table changed:\n%s", got)
	}
	if got := bulletizeWideTables("a | b\nnot a separator", 2); got != "a | b\nnot a separator" {
		t.Errorf("non-table changed: %q", got)
	}
}

func TestBulletizeWideTablesEdgeCases(t *testing.T) {
	headerOnly := "| A very long header | Another very long header |\n|---|---|"
	if got := bulletizeWideTables(headerOnly, 10); got != headerOnly {
		t.Errorf("header-only table changed:\n%s", got)
	}

	escaped := "| Rule | Pattern | Owner |\n|---|---|---|\n| split | a \\| b | Platform team |"
	want := "- **split** · Pattern: a | b · Owner: Platform team"
	if got := bulletizeWideTables(escaped, 10); got != want {
		t.Errorf("escaped pipe table =\n%s\nwant\n%s", got, want)
	}

	doubled := "| Path | Owner |\n|---|---|\n| C:\\\\ | Platform team |"
	wantDoubled := "- **C:\\\\** · Owner: Platform team"
	if got := bulletizeWideTables(doubled, 10); got != wantDoubled {
		t.Errorf("doubled backslash table =\n%s\nwant\n%s", got, wantDoubled)
	}
	if got := tableCells(`| a \\| b |`); len(got) != 2 || got[0] != `a \\` || got[1] != "b" {
		t.Errorf("tableCells with escaped backslash = %q", got)
	}

	table := "| Service | Owner | Status |\n|---|---|---|\n| api | Platform team | degraded |"
	longFence := "````md\n```\n" + table + "\n````"
	if got := bulletizeWideTables(longFence, 10); got != longFence {
		t.Errorf("table inside four-backtick fence changed:\n%s", got)
	}
}

func TestPrepareAssistantMarkdownBulletizesWideTables(t *testing.T) {
	table := "| Service | Owner |\n|---|---|\n| api | Platform team |"
	prepared, _ := prepareAssistantMarkdown(table, markdownOpts{width: 12})
	if !strings.Contains(prepared, "- **api** · Owner: Platform team") {
		t.Errorf("prepared = %q, want bullets", prepared)
	}
}

func TestFailedActivityLineHasRedCross(t *testing.T) {
	colored := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
	line := colored.activityLine("✗ Searching incidents (1.0s)")
	if !strings.Contains(line, "\x1b[31m✗") {
		t.Errorf("activity line = %q, want red ✗", line)
	}
	if ok := colored.activityLine("✓ Searching incidents (1.0s)"); strings.Contains(ok, "\x1b[31m") {
		t.Errorf("success line = %q, want no red", ok)
	}
	plain := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	if got := plain.activityLine("✗ Searching incidents (1.0s)"); got != "✗ Searching incidents (1.0s)" {
		t.Errorf("no-color failed line = %q, want plain", got)
	}
}

func TestLocalToolsAvailableStatusShownOnce(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.sentTools = 3
	accepted := 3
	model.noteClientToolsAccepted(&accepted)
	model.noteClientToolsAccepted(&accepted)
	transcript := model.Transcript()
	if got := strings.Count(transcript, "3 local tools available"); got != 1 {
		t.Errorf("status shown %d times, want once:\n%s", got, transcript)
	}
	if strings.Contains(transcript, localToolsOffMessage) {
		t.Errorf("transcript = %q, want no flag-off notice", transcript)
	}

	none := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	none.noteClientToolsAccepted(&accepted)
	if strings.Contains(none.Transcript(), "local tool") {
		t.Error("status shown when no tools were sent")
	}
	if got := localToolsAvailableMessage(1); got != "1 local tool available" {
		t.Errorf("localToolsAvailableMessage(1) = %q", got)
	}
}

func TestResumeMissingSessionStartsFresh(t *testing.T) {
	sessionID := uuid.New()
	fake := &fakeTUIAPI{messagePages: []*rootly.ListAiChatSessionMessagesResponse{{
		Body:         []byte(`{"errors":[{"title":"Not found"}]}`),
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
	}}}
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true, API: fake, SessionID: &sessionID, SessionTitle: "Old"})
	model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	msg := model.loadHistory(sessionID)()
	loaded, ok := msg.(historyLoadedMsg)
	if !ok || !errors.Is(loaded.err, errSessionNotFound) {
		t.Fatalf("loadHistory() = %#v, want errSessionNotFound", msg)
	}
	model.Update(msg)
	if model.sessionID != nil {
		t.Errorf("sessionID = %v, want a fresh session", model.sessionID)
	}
	transcript := model.Transcript()
	if !strings.Contains(transcript, sessionNotFoundMessage) {
		t.Errorf("transcript = %q, want %q", transcript, sessionNotFoundMessage)
	}
	if strings.Contains(transcript, "Could not load session history") {
		t.Errorf("transcript = %q, want no generic history error", transcript)
	}
}

func TestStartupReportsUnreachableMCPServersOnce(t *testing.T) {
	manager := &fakeMCPManager{views: []mcp.ServerView{
		{Server: mcp.Server{Name: "fs"}, Status: mcp.StatusConnected},
		{Server: mcp.Server{Name: "broken"}, Status: mcp.StatusError, Detail: "exec: \"nope\": executable\nfile not found"},
		{Server: mcp.Server{Name: "quiet"}, Status: mcp.StatusError, Detail: "", SessionOff: true},
		{Server: mcp.Server{Name: "blank"}, Status: mcp.StatusError},
	}, changes: make(chan struct{})}
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true, MCP: manager})
	model.Update(mcpStartupCheckMsg{})
	model.Update(mcpChangedMsg{})

	transcript := model.Transcript()
	for _, want := range []string{
		`MCP 'broken' unreachable: exec: "nope": executable file not found — open /mcp`,
		"MCP 'blank' unreachable: connection failed — open /mcp",
	} {
		if got := strings.Count(transcript, want); got != 1 {
			t.Errorf("transcript has %q %d times, want once:\n%s", want, got, transcript)
		}
	}
	for _, unwanted := range []string{"'fs'", "'quiet'"} {
		if strings.Contains(transcript, unwanted) {
			t.Errorf("transcript = %q, want no line for %s", transcript, unwanted)
		}
	}
}

func TestWelcomeShowsFirstNameWorkingDirAndCapability(t *testing.T) {
	user := currentUserForWelcome("Avery Rootly", "avery@example.com", "")
	user.Data.Attributes.FirstName = nullable.NewNullableWithValue("Ave")
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true, CurrentUser: user, APIHost: "api.rootly.com", WorkingDir: "/srv/ops"})
	model.width = 100
	rows := model.welcomeTextRows()
	for _, want := range []string{"Welcome back, Ave", "/srv/ops", "api.rootly.com", welcomeCapabilityText} {
		if !strings.Contains(rows, want) {
			t.Errorf("welcome rows = %q, want %q", rows, want)
		}
	}
	model.width = 40
	if strings.Contains(model.welcomeTextRows(), welcomeCapabilityText) {
		t.Error("capability line shown when it does not fit")
	}
}

func TestPrintExitErrorCodes(t *testing.T) {
	var coder interface{ ExitCode() int }
	if err := PrintExitError(NotAuthenticatedError("not authenticated")); !errors.As(err, &coder) || coder.ExitCode() != ExitNotAuthenticated {
		t.Errorf("PrintExitError(not authenticated) = %v, want exit code 2", err)
	}
	generic := errors.New("boom")
	if err := PrintExitError(generic); err != generic {
		t.Errorf("PrintExitError(generic) = %v, want unchanged", err)
	}
	if err := PrintExitError(nil); err != nil {
		t.Errorf("PrintExitError(nil) = %v", err)
	}
	if !IsNotAuthenticated(StatusResponseError(http.StatusUnauthorized, nil)) {
		t.Error("401 stream error is not classified as not authenticated")
	}
}

func TestStaleMissingSessionReplyKeepsNewChat(t *testing.T) {
	sessionID := uuid.New()
	fake := &fakeTUIAPI{messagePages: []*rootly.ListAiChatSessionMessagesResponse{{
		Body:         []byte(`{"errors":[{"title":"Not found"}]}`),
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
	}}}
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true, API: fake, SessionID: &sessionID, SessionTitle: "Old"})
	model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	pending := model.loadHistory(sessionID)
	model.startNewSession()
	newSession := uuid.New()
	model.sessionID = &newSession
	model.Update(pending())
	if model.sessionID == nil || *model.sessionID != newSession {
		t.Fatalf("sessionID = %v, want the new chat %v kept", model.sessionID, newSession)
	}
	if strings.Contains(model.Transcript(), sessionNotFoundMessage) {
		t.Errorf("stale 404 printed %q", sessionNotFoundMessage)
	}
}

func TestStartupWaitsForConnectingViews(t *testing.T) {
	manager := &fakeMCPManager{views: []mcp.ServerView{
		{Server: mcp.Server{Name: "slow"}, Status: mcp.StatusConnecting},
		{Server: mcp.Server{Name: "broken"}, Status: mcp.StatusError, Detail: "boom"},
	}, changes: make(chan struct{})}
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true, MCP: manager})
	model.Update(mcpStartupCheckMsg{})
	if strings.Contains(model.Transcript(), "unreachable") {
		t.Fatalf("reported before every server settled:\n%s", model.Transcript())
	}
	manager.views[0] = mcp.ServerView{Server: mcp.Server{Name: "slow"}, Status: mcp.StatusError, Detail: "timeout"}
	model.Update(mcpChangedMsg{})
	transcript := model.Transcript()
	for _, want := range []string{"MCP 'slow' unreachable: timeout", "MCP 'broken' unreachable: boom"} {
		if strings.Count(transcript, want) != 1 {
			t.Errorf("transcript missing %q once:\n%s", want, transcript)
		}
	}
}

func TestBulletizeWideTablesAfterLongerFenceCloser(t *testing.T) {
	table := "| Service | Owner | Status |\n|---|---|---|\n| api | Platform team | degraded |"
	got := bulletizeWideTables("```\ncode\n````\n"+table, 10)
	if !strings.Contains(got, "- **api** · Owner: Platform team · Status: degraded") {
		t.Errorf("table after a longer fence closer was not converted:\n%s", got)
	}
}

func TestBulletizeWideTablesFenceRules(t *testing.T) {
	table := "| Service | Owner | Status |\n|---|---|---|\n| api | Platform team | degraded |"
	bullet := "- **api** · Owner: Platform team · Status: degraded"
	cases := map[string]struct {
		markdown string
		convert  bool
	}{
		"inner shorter fence stays inside": {"````\n```\n" + table + "\n````", false},
		"longer closer closes":             {"```\ncode\n````\n" + table, true},
		"indented code is not a fence":     {"    ```\n\n" + table, true},
		"indented closer does not close":   {"```\n    ```\n" + table + "\n```", false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := bulletizeWideTables(tc.markdown, 10)
			if strings.Contains(got, bullet) != tc.convert {
				t.Errorf("convert=%v, got:\n%s", tc.convert, got)
			}
		})
	}
}

func TestBulletizeWideTablesKeepsEscapedPipeInItsColumn(t *testing.T) {
	got := bulletizeWideTables("| Service | Status |\n|---|---|\n| api\\|worker | degraded |", 10)
	if !strings.Contains(got, "- **api|worker** · Status: degraded") {
		t.Errorf("escaped pipe split the row:\n%s", got)
	}
}

func TestSendWaitsForPendingHistory(t *testing.T) {
	sessionID := uuid.New()
	fake := &fakeTUIAPI{messagePages: []*rootly.ListAiChatSessionMessagesResponse{{
		Body:         []byte(`{"errors":[{"title":"Not found"}]}`),
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
	}}}
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true, API: fake, SessionID: &sessionID})
	model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	pending := model.loadHistory(sessionID)
	model.composer.SetValue("hello")
	if cmd := model.submit(); cmd != nil || model.state == tuiStreaming {
		t.Fatal("a prompt sent before resumed history loads must wait")
	}
	if model.queuedPrompt.message != "hello" || model.status != "Loading session history…" {
		t.Fatalf("queued %q status %q", model.queuedPrompt, model.status)
	}
	_, cmd := model.Update(pending())
	if model.historyPending || model.queuedPrompt != (queuedSend{}) {
		t.Fatal("settled history should clear the pending flag and the queue")
	}
	if cmd == nil || model.state != tuiStreaming || model.currentPrompt != "hello" {
		t.Fatal("the queued prompt should send once history has settled")
	}
}

func TestSkillPromptWaitsForPendingHistory(t *testing.T) {
	sessionID := uuid.New()
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true, API: &fakeTUIAPI{}, SessionID: &sessionID})
	model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model.loadHistory(sessionID)
	if cmd := model.submitPrompt("expanded skill prompt"); cmd != nil || model.state == tuiStreaming {
		t.Fatal("skill prompts must wait for resumed history too")
	}
	model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if model.queuedPrompt != (queuedSend{}) {
		t.Fatal("Ctrl+C should drop a queued prompt")
	}
}
