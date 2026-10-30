package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/uuid"
	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"
	rootly "github.com/rootlyhq/rootly-go"

	"github.com/rootlyhq/rootly-cli/internal/api"
)

type blockingStreamer struct {
	started chan struct{}
}

type fakeTUIAPI struct {
	sessionPages  []*rootly.ListAiChatSessionsResponse
	listParams    []*rootly.ListAiChatSessionsParams
	messagePages  []*rootly.ListAiChatSessionMessagesResponse
	messageParams []*rootly.ListAiChatSessionMessagesParams
	updateBodies  []rootly.UpdateAiChatSessionApplicationVndAPIPlusJSONRequestBody
	updateStatus  int
	deleteCalls   int
	deleteStatus  int
}

func (f *fakeTUIAPI) GetCurrentUserWithResponse(context.Context) (*rootly.GetCurrentUserResponse, error) {
	return nil, nil
}

func (f *fakeTUIAPI) ListAiChatSessionsWithResponse(_ context.Context, params *rootly.ListAiChatSessionsParams) (*rootly.ListAiChatSessionsResponse, error) {
	f.listParams = append(f.listParams, params)
	if len(f.sessionPages) == 0 {
		return testSessionPage(nil, false, ""), nil
	}
	response := f.sessionPages[0]
	f.sessionPages = f.sessionPages[1:]
	return response, nil
}

func (f *fakeTUIAPI) ListAiChatSessionMessagesWithResponse(_ context.Context, _ openapi_types.UUID, params *rootly.ListAiChatSessionMessagesParams) (*rootly.ListAiChatSessionMessagesResponse, error) {
	f.messageParams = append(f.messageParams, params)
	if len(f.messagePages) == 0 {
		return nil, errors.New("history not configured")
	}
	response := f.messagePages[0]
	f.messagePages = f.messagePages[1:]
	return response, nil
}

func (f *fakeTUIAPI) UpdateAiChatSessionWithApplicationVndAPIPlusJSONBodyWithResponse(_ context.Context, _ rootly.ID, body rootly.UpdateAiChatSessionApplicationVndAPIPlusJSONRequestBody) (*rootly.UpdateAiChatSessionResponse, error) {
	f.updateBodies = append(f.updateBodies, body)
	status := f.updateStatus
	if status == 0 {
		status = http.StatusOK
	}
	return &rootly.UpdateAiChatSessionResponse{HTTPResponse: &http.Response{StatusCode: status}, Body: []byte(`{"errors":[{"title":"ai.chat:write required"}]}`)}, nil
}

func (f *fakeTUIAPI) DeleteAiChatSessionWithResponse(context.Context, rootly.ID) (*rootly.DeleteAiChatSessionResponse, error) {
	f.deleteCalls++
	status := f.deleteStatus
	if status == 0 {
		status = http.StatusNoContent
	}
	return &rootly.DeleteAiChatSessionResponse{
		HTTPResponse: &http.Response{StatusCode: status},
		Body:         []byte(`{"errors":[{"title":"Session not found"}]}`),
	}, nil
}

func (*fakeTUIAPI) GetIncidentByID(context.Context, string) (*api.Incident, error) {
	return nil, errors.New("incident not found")
}

func (*fakeTUIAPI) GetAlertByID(context.Context, string) (*api.Alert, error) {
	return nil, errors.New("alert not found")
}

func (*fakeTUIAPI) APIHost() string { return "api.example.test" }

func testSessionPage(sessions []rootly.AiChatSession, hasMore bool, before string) *rootly.ListAiChatSessionsResponse {
	data := &rootly.AiChatSessionList{Sessions: sessions}
	data.Meta.Pagination.HasMore = hasMore
	if before == "" {
		data.Meta.Pagination.NextBefore = nullable.NewNullNullable[string]()
	} else {
		data.Meta.Pagination.NextBefore = nullable.NewNullableWithValue(before)
	}
	return &rootly.ListAiChatSessionsResponse{
		HTTPResponse:             &http.Response{StatusCode: http.StatusOK},
		ApplicationVndAPIJSON200: data,
	}
}

func testHistoryPage(messages []rootly.AiChatSessionMessage, pages int) *rootly.ListAiChatSessionMessagesResponse {
	return &rootly.ListAiChatSessionMessagesResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		ApplicationVndAPIJSON200: &rootly.AiChatSessionMessageList{
			Messages: messages,
			Meta:     map[string]interface{}{"pagination": map[string]interface{}{"pages": pages}},
		},
	}
}

func testSession(id, title string) rootly.AiChatSession {
	return rootly.AiChatSession{
		ID: uuid.MustParse(id), Title: title, LastActiveAt: time.Now(),
	}
}

func (s *blockingStreamer) StreamAiChatEvents(ctx context.Context, _ *rootly.StreamAiChatParams) (iter.Seq2[rootly.AiChatStreamEvent, error], error) {
	s.started <- struct{}{}
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestTUIModelAccumulatesTextChunks(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
	model.state = tuiStreaming
	model.stream = make(chan streamMessage, 1)

	for _, content := range []string{"first ", "second"} {
		event := Event{Type: rootly.AiChatStreamEventText, Text: &rootly.AiChatStreamText{Content: content}}
		_, _ = model.Update(streamMessage{event: event})
	}
	if got := model.answer.String(); got != "first second" {
		t.Errorf("answer = %q, want accumulated chunks", got)
	}
}

func TestTUIModelDonePrintsAndReturnsIdle(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
	model.state = tuiStreaming
	model.answer.WriteString("assistant reply")
	model.doneStatus = "complete"

	updated, cmd := model.Update(streamMessage{done: true})
	if got := updated.(*TUIModel).state; got != tuiIdle {
		t.Errorf("state = %v, want idle", got)
	}
	if cmd == nil {
		t.Fatal("done did not return a print command")
	}
}

func TestTUIModelCtrlCCancelsStreaming(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
	model.state = tuiStreaming
	model.answer.WriteString("partial answer")
	called := false
	model.cancel = func() { called = true }

	_, cmd := model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !called {
		t.Error("Ctrl+C did not call the stream cancel function")
	}
	if model.state != tuiIdle || model.status != "" {
		t.Errorf("state/status = %v/%q, want idle with no status", model.state, model.status)
	}
	output := commandOutput(cmd)
	if !strings.Contains(output, "partial") || !strings.Contains(output, "answer") || !strings.Contains(output, "Stopped locally. The server may still finish this turn and save it to the session.") {
		t.Errorf("scrollback output = %q, want partial answer and cancellation line", output)
	}
}

func TestTUITranscriptUsesClaudeCodeStyle(t *testing.T) {
	plain := renderPromptEcho("hello\nworld", 40, true)
	if plain != "❯ hello\n  world" {
		t.Errorf("plain prompt echo = %q, want prompt with indented continuation", plain)
	}
	colored := renderPromptEcho("hello\nworld", 40, false)
	for _, line := range strings.Split(colored, "\n") {
		if width := ansi.StringWidth(line); width != 40 {
			t.Errorf("prompt echo width = %d, want 40: %q", width, line)
		}
	}
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	answer := model.renderAssistantAnswer("Hello\nworld")
	if answer != "● Hello\n  world" || strings.Contains(answer, "Assistant:") {
		t.Errorf("assistant transcript = %q, want bullet and indented continuation without label", answer)
	}
}

func TestTUIAnswerAndHistorySanitizeTerminalControls(t *testing.T) {
	const (
		attack = "\x1b]52;c;ZXZpbA==\x07\x1b[2J\x1b]8;;https://evil.example\x07"
		url    = "https://rootly.com/docs"
	)
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
	model.handleEvent(Event{Type: rootly.AiChatStreamEventText, Text: &rootly.AiChatStreamText{
		Content: "safe " + attack + " [Open docs](" + url + ")",
	}})
	answer := model.renderAssistantAnswer(model.answer.String())
	for _, unwanted := range []string{"]52;", "[2J", "\x1b]8;;https://evil.example"} {
		if strings.Contains(answer, unwanted) {
			t.Errorf("rendered answer = %q, want no %q", answer, unwanted)
		}
	}
	if got := strings.Count(answer, ansi.SetHyperlink(url)); got != 1 {
		t.Errorf("answer link count = %d, want one CLI-generated OSC-8 link: %q", got, answer)
	}

	historyModel := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	cmd := historyModel.printHistory("History "+attack, []rootly.AiChatSessionMessage{
		{Role: rootly.AiChatSessionMessageRole("user"), Content: "Prompt " + attack},
		{Role: rootly.AiChatSessionMessageRole("assistant"), Content: "Answer " + attack},
	})
	history := commandSequenceOutput(cmd)
	for _, unwanted := range []string{"]52;", "[2J", "\x1b"} {
		if strings.Contains(history, unwanted) {
			t.Errorf("printed history = %q, want no %q", history, unwanted)
		}
	}
	historyModel.sessionTitle = "Session " + attack
	status := historyModel.statusLines()
	for _, unwanted := range []string{"]52;", "[2J", "\x1b"} {
		if strings.Contains(status, unwanted) {
			t.Errorf("status output = %q, want no %q", status, unwanted)
		}
	}
}

func TestSessionPickerSanitizesTitle(t *testing.T) {
	item := sessionPickerItem{session: rootly.AiChatSession{
		Title: "\x1b]52;c;ZXZpbA==\x07",
	}}
	if got := item.Title(); got != "Untitled" {
		t.Errorf("session picker title = %q, want Untitled after sanitization", got)
	}
}

func TestTUIComposerStartsAtOneRowAndGrows(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
	model.composer.SetWidth(40)
	emptyRows := len(strings.Split(strings.TrimRight(model.composer.View(), "\n"), "\n"))
	if emptyRows != 1 {
		t.Errorf("empty composer rows = %d, want 1", emptyRows)
	}

	model.composer.SetValue("one\ntwo\nthree\nfour")
	filledRows := len(strings.Split(strings.TrimRight(model.composer.View(), "\n"), "\n"))
	if filledRows < 3 || filledRows > model.composer.MaxHeight {
		t.Errorf("multiline composer rows = %d, want between 3 and %d", filledRows, model.composer.MaxHeight)
	}
	model.printedRows = 5
	_, _ = model.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	if model.composer.MaxHeight != 6 {
		t.Errorf("composer max height after resize = %d, want 6", model.composer.MaxHeight)
	}
	if model.printedRows != 5 {
		t.Errorf("printed rows after resize = %d, want 5", model.printedRows)
	}
}

func TestTUIExitAliasesAndNormalExitPhrase(t *testing.T) {
	for _, prompt := range []string{"exit", " Exit ", "quit"} {
		model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
		model.composer.SetValue(prompt)
		cmd := model.submit()
		if model.state != tuiIdle || cmd == nil {
			t.Errorf("submit(%q) = state %v, command %v; want quit without streaming", prompt, model.state, cmd)
		}
	}
	streamer := &fakeStreamer{}
	model := NewTUIModel(context.Background(), streamer, TUIOptions{})
	model.composer.SetValue("exit now")
	_ = model.submit()
	if model.state != tuiStreaming || model.currentParams == nil || model.currentParams.Message == nil || *model.currentParams.Message != "exit now" {
		t.Errorf("exit now was not submitted as a normal prompt: state=%v params=%#v", model.state, model.currentParams)
	}
	if model.cancel != nil {
		model.cancel()
	}
}

func TestTUICommandAutocompleteTabCompletes(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
	model.composer.SetValue("/res")
	model.refreshCommandMenu()
	if !model.commandMenuOpen {
		t.Fatal("command menu did not open for a matching prefix")
	}
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if got := model.composer.Value(); got != "/resume " {
		t.Errorf("Tab value = %q, want /resume with a trailing space", got)
	}
	if model.commandMenuOpen {
		t.Error("command menu remained open after Tab completion")
	}
}

func TestTUICommandAutocompleteEnterRunsSelectedPrefix(t *testing.T) {
	apiClient := &fakeTUIAPI{sessionPages: []*rootly.ListAiChatSessionsResponse{testSessionPage(nil, false, "")}}
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{API: apiClient})
	model.composer.SetValue("/r")
	model.refreshCommandMenu()

	_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !model.pickerOpen || cmd == nil {
		t.Fatalf("Enter on prefix did not run selected /resume command: picker=%v cmd=%v", model.pickerOpen, cmd)
	}
	result := cmd()
	page, ok := result.(sessionPageMsg)
	if !ok {
		t.Fatalf("resume command result = %T, want session page", result)
	}
	_, _ = model.Update(page)
	if len(apiClient.listParams) != 1 {
		t.Errorf("session list calls = %d, want one selected /resume call", len(apiClient.listParams))
	}
}

func TestTUISessionPickerLoadsMoreAndResumesSelection(t *testing.T) {
	secondID := "db92b891-85c5-4e0c-a2f6-e8d873bfbde3"
	apiClient := &fakeTUIAPI{sessionPages: []*rootly.ListAiChatSessionsResponse{
		testSessionPage([]rootly.AiChatSession{testSession(testSessionID, "First")}, true, "cursor-2"),
		testSessionPage([]rootly.AiChatSession{testSession(secondID, "Second")}, false, ""),
	}}
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{API: apiClient})

	firstPage := model.openSessionPicker()().(sessionPageMsg)
	_, _ = model.Update(firstPage)
	if len(apiClient.listParams) != 1 || apiClient.listParams[0].Before != nil {
		t.Fatalf("first list call params = %#v, want first page", apiClient.listParams)
	}
	if len(model.sessionPicker.Items()) != 2 {
		t.Fatalf("picker items = %d, want session and Load more sentinel", len(model.sessionPicker.Items()))
	}

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	_, loadMoreCmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if loadMoreCmd == nil {
		t.Fatal("selecting Load more returned no fetch command")
	}
	secondPage := loadMoreCmd().(sessionPageMsg)
	_, _ = model.Update(secondPage)
	if len(apiClient.listParams) != 2 || apiClient.listParams[1].Before == nil || *apiClient.listParams[1].Before != "cursor-2" {
		t.Fatalf("next page params = %#v, want Before cursor-2", apiClient.listParams)
	}
	if len(model.sessionPicker.Items()) != 2 {
		t.Fatalf("picker items after more = %d, want two sessions", len(model.sessionPicker.Items()))
	}

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.sessionID == nil || model.sessionID.String() != secondID {
		t.Errorf("selected session = %v, want %s", model.sessionID, secondID)
	}
	if model.sessionTitle != "Second" || model.pickerOpen {
		t.Errorf("selection state title/open = %q/%v, want Second/closed", model.sessionTitle, model.pickerOpen)
	}
}

func TestTUISessionPickerEscapeCancels(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
	model.pickerOpen = true
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.pickerOpen {
		t.Error("Esc left the session picker open")
	}
}

func TestTUIHistoryRecallAndFooter(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.promptHistory = []string{"first", "second"}
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if got := model.composer.Value(); got != "second" {
		t.Errorf("first Up value = %q, want latest prompt", got)
	}
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if got := model.composer.Value(); got != "first" {
		t.Errorf("second Up value = %q, want previous prompt", got)
	}
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if got := model.composer.Value(); got != "second" {
		t.Errorf("Down value = %q, want newer prompt", got)
	}
	sessionID := uuid.MustParse(testSessionID)
	model.sessionID = &sessionID
	model.turnStartedAt = time.Now().Add(-1200 * time.Millisecond)
	if footer := model.turnFooter(); !strings.Contains(footer, "✻ Triaged for ") || !strings.Contains(footer, " · done ") || !strings.Contains(footer, "session "+testSessionID[:8]) {
		t.Errorf("turn footer = %q, want elapsed and short session id", footer)
	}
}

func TestTUIHistoryLoadedKeepsComposerAndSchedulesRedraw(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.historyRequest = 1

	updated, cmd := model.Update(historyLoadedMsg{
		request: 1,
		title:   "Resumed session",
		messages: []rootly.AiChatSessionMessage{
			{Role: rootly.AiChatSessionMessageRole("user"), Content: "older prompt"},
			{Role: rootly.AiChatSessionMessageRole("assistant"), Content: "long answer"},
		},
	})
	if updated.(*TUIModel).state != tuiIdle {
		t.Fatalf("state = %v, want idle after history loads", updated.(*TUIModel).state)
	}
	if cmd == nil {
		t.Fatal("history load did not schedule print and redraw commands")
	}
	if got := fmt.Sprintf("%T", cmd()); !strings.Contains(got, "sequenceMsg") {
		t.Errorf("history command message type = %q, want sequence of prints and redraw", got)
	}
	if view := ansi.Strip(model.View().Content); !strings.Contains(view, "❯ Ask Rootly...") {
		t.Errorf("view after history load = %q, want composer visible", view)
	}
}

func TestTUIStreamingTrimsOnlyIncompleteColorLinks(t *testing.T) {
	const partial = "see [Rotate](http://x?a_b"
	colorModel := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
	colorModel.state = tuiStreaming
	colorModel.answer.WriteString(partial)
	colorView := colorModel.View().Content
	visible := ansi.Strip(colorView)
	for _, unwanted := range []string{"http", "a_b"} {
		if strings.Contains(visible, unwanted) {
			t.Errorf("streaming view = %q, want no %q from incomplete link", visible, unwanted)
		}
	}
	if final := ansi.Strip(colorModel.renderAssistantAnswer(partial)); !strings.Contains(final, "http://x?a_b") {
		t.Errorf("final answer = %q, want unchanged incomplete-link text", final)
	}

	rawModel := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	rawModel.state = tuiStreaming
	rawModel.answer.WriteString(partial)
	if view := rawModel.View().Content; !strings.Contains(view, partial) {
		t.Errorf("no-color streaming view = %q, want raw Markdown %q", view, partial)
	}

	const completeURL = "http://x?a_b"
	completeModel := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
	completeModel.state = tuiStreaming
	completeModel.answer.WriteString("see [Rotate](" + completeURL + ") and more")
	completeView := completeModel.View().Content
	completeVisible := ansi.Strip(completeView)
	if !strings.Contains(completeVisible, "Rotate") || !strings.Contains(completeVisible, "and more") {
		t.Errorf("complete-link streaming view = %q, want link label and trailing text", completeVisible)
	}
	if got := strings.Count(completeView, ansi.SetHyperlink(completeURL)); got != 1 {
		t.Errorf("complete-link count = %d, want one hyperlink", got)
	}
}

func TestLoadSessionHistoryFetchesLastPage(t *testing.T) {
	firstMessage := rootly.AiChatSessionMessage{
		ID:   uuid.MustParse(testSessionID),
		Role: rootly.AiChatSessionMessageRole("user"), Content: "old prompt",
		CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	lastMessage := rootly.AiChatSessionMessage{
		ID:   uuid.MustParse("c532fe53-20b9-49ee-ab28-68e917d09e73"),
		Role: rootly.AiChatSessionMessageRole("assistant"), Content: "latest answer",
		CreatedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	}
	apiClient := &fakeTUIAPI{messagePages: []*rootly.ListAiChatSessionMessagesResponse{
		testHistoryPage([]rootly.AiChatSessionMessage{firstMessage}, 2),
		testHistoryPage([]rootly.AiChatSessionMessage{lastMessage}, 2),
	}}

	messages, _, err := loadSessionHistory(context.Background(), apiClient, uuid.MustParse(testSessionID))
	if err != nil {
		t.Fatalf("loadSessionHistory() error = %v", err)
	}
	if len(apiClient.messageParams) != 2 {
		t.Fatalf("history calls = %d, want first and last page", len(apiClient.messageParams))
	}
	if *apiClient.messageParams[0].PageNumber != 1 || *apiClient.messageParams[0].PageSize != 100 ||
		*apiClient.messageParams[1].PageNumber != 2 || *apiClient.messageParams[1].PageSize != 100 {
		t.Errorf("history page params = %#v, %#v; want pages 1 and 2 with size 100", apiClient.messageParams[0], apiClient.messageParams[1])
	}
	if len(messages) != 2 || messages[0].Content != "old prompt" || messages[1].Content != "latest answer" {
		t.Errorf("history messages = %#v, want first and last page messages in order", messages)
	}
}

func TestTUIResumeByIDLoadsTitleAndPrintsIt(t *testing.T) {
	apiClient := &fakeTUIAPI{messagePages: []*rootly.ListAiChatSessionMessagesResponse{
		{
			Body:                     []byte(`{"title":"Incident review\u001b[2J"}`),
			ApplicationVndAPIJSON200: &rootly.AiChatSessionMessageList{},
		},
	}}
	sessionID := uuid.MustParse(testSessionID)
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{
		NoColor:   true,
		API:       apiClient,
		SessionID: &sessionID,
	})
	initSequence := reflect.ValueOf(model.Init()())
	if initSequence.Kind() != reflect.Slice || initSequence.Len() != 4 {
		t.Fatalf("initial resume commands = %v, want clear, welcome, focus and history before the title loads", initSequence)
	}

	history := model.loadHistory(sessionID, "")().(historyLoadedMsg)
	updated, cmd := model.Update(history)
	resumed := updated.(*TUIModel)
	if resumed.sessionTitle != "Incident review" {
		t.Fatalf("session title = %q, want sanitized history title", resumed.sessionTitle)
	}
	if output := commandSequenceOutput(cmd); !strings.Contains(output, `Resumed "Incident review"`) {
		t.Errorf("resume output = %q, want the loaded session title", output)
	}
	if status := resumed.statusLines(); !strings.Contains(status, "title Incident review") {
		t.Errorf("status lines = %q, want loaded session title", status)
	}
}

func TestTUIResumeByIDFallsBackToShortIDWithoutTitle(t *testing.T) {
	for _, test := range []struct {
		name string
		body []byte
	}{
		{name: "absent", body: []byte(`{}`)},
		{name: "untitled", body: []byte(`{"title":"Untitled"}`)},
	} {
		t.Run(test.name, func(t *testing.T) {
			apiClient := &fakeTUIAPI{messagePages: []*rootly.ListAiChatSessionMessagesResponse{
				{
					Body:                     test.body,
					ApplicationVndAPIJSON200: &rootly.AiChatSessionMessageList{},
				},
			}}
			sessionID := uuid.MustParse(testSessionID)
			model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{
				NoColor:   true,
				API:       apiClient,
				SessionID: &sessionID,
			})

			history := model.loadHistory(sessionID, "")().(historyLoadedMsg)
			updated, cmd := model.Update(history)
			resumed := updated.(*TUIModel)
			if resumed.sessionTitle != "" {
				t.Fatalf("session title = %q, want empty fallback state", resumed.sessionTitle)
			}
			want := fmt.Sprintf("Resumed %q", shortSessionID(testSessionID))
			if output := commandSequenceOutput(cmd); !strings.Contains(output, want) {
				t.Errorf("resume output = %q, want %q", output, want)
			}
		})
	}
}

func TestTUITaskActivityIsTerminalAndDeduplicated(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.handleEvent(Event{Type: rootly.AiChatStreamEventTaskUpdate, TaskUpdate: &rootly.AiChatStreamTaskUpdate{ID: "task-1", Title: "Create incident", Status: "in_progress"}})
	if len(model.activityCommands) != 0 {
		t.Fatalf("activity commands after in_progress = %d, want none", len(model.activityCommands))
	}
	model.handleEvent(Event{Type: rootly.AiChatStreamEventTaskUpdate, TaskUpdate: &rootly.AiChatStreamTaskUpdate{ID: "task-1", Title: "Create incident", Status: "complete"}})
	model.handleEvent(Event{Type: rootly.AiChatStreamEventTaskUpdate, TaskUpdate: &rootly.AiChatStreamTaskUpdate{ID: "task-1", Title: "Create incident", Status: "complete"}})
	model.handleEvent(Event{Type: rootly.AiChatStreamEventTaskUpdate, TaskUpdate: &rootly.AiChatStreamTaskUpdate{ID: "task-2", Title: "Notify owner", Status: "cancelled"}})
	model.handleEvent(Event{Type: rootly.AiChatStreamEventTaskUpdate, TaskUpdate: &rootly.AiChatStreamTaskUpdate{ID: "task-3", Title: "Searched incidents (1.2s)", Status: "complete"}})
	if len(model.activityCommands) != 3 {
		t.Fatalf("activity commands = %d, want one per terminal task", len(model.activityCommands))
	}
	first, second := commandOutput(model.activityCommands[0]), commandOutput(model.activityCommands[1])
	if !strings.Contains(first, "✓ Create incident") || !strings.Contains(second, "✗ Notify owner") {
		t.Errorf("task lines = %q, %q; want success and failure glyphs", first, second)
	}
	if got := commandOutput(model.activityCommands[2]); !strings.Contains(got, "✓ Searched incidents (1.2s)") || strings.Contains(got, "(1.2s) (") {
		t.Errorf("duration task line = %q, want title duration only", got)
	}
}

func TestTUIStatusActivityIsPersisted(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.handleEvent(activityStatusEvent("Get incidents..."))
	if len(model.activityCommands) != 0 {
		t.Fatalf("activity commands after initial status = %d, want none", len(model.activityCommands))
	}
	model.handleEvent(activityStatusEvent("Composing response..."))
	if len(model.activityCommands) != 0 {
		t.Fatalf("activity commands after composing status = %d, want none until the turn finishes", len(model.activityCommands))
	}
	model.handleEvent(Event{Type: rootly.AiChatStreamEventText, Text: &rootly.AiChatStreamText{Content: "Answer"}})
	if len(model.activityCommands) != 0 {
		t.Fatalf("activity commands after text = %d, want none until the turn finishes", len(model.activityCommands))
	}
	model.handleEvent(activityDoneEvent("complete"))
	if len(model.activityCommands) != 1 {
		t.Fatalf("activity commands after turn completion = %d, want one", len(model.activityCommands))
	}
	if got := commandOutput(model.activityCommands[0]); !strings.Contains(got, "✓ Get incidents (") {
		t.Errorf("activity line = %q, want successful Get incidents", got)
	}
}

func TestTUIConfirmationDefersTextFinalizedActivityUntilResumedTurn(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.state = tuiStreaming
	model.handleEvent(activityStatusEvent("Create action item..."))
	model.handleEvent(activityStatusEvent("Composing response..."))
	model.handleEvent(Event{Type: rootly.AiChatStreamEventText, Text: &rootly.AiChatStreamText{Content: "Ready."}})
	model.handleEvent(Event{
		Type: rootly.AiChatStreamEventConfirmationRequired,
		ConfirmationRequired: &rootly.AiChatStreamConfirmationRequired{
			CheckpointID: testCheckpointID,
			Question:     "Approve?",
			ExpiresAt:    "2099-01-01T00:00:00Z",
		},
	})
	model.handleEvent(activityDoneEvent("confirmation_required"))

	_, confirmationCmd := model.finishTurn(nil)
	if output := commandSequenceOutput(confirmationCmd); strings.Contains(output, "✓ Create action item (") {
		t.Fatalf("confirmation output = %q, want no activity before approval", output)
	}
	if model.state != tuiConfirming {
		t.Fatalf("state = %v, want confirming", model.state)
	}

	model.pending = nil
	model.state = tuiStreaming
	model.answer.Reset()
	model.doneStatus = ""
	model.handleEvent(activityStatusEvent("Create action item..."))
	model.handleEvent(Event{Type: rootly.AiChatStreamEventText, Text: &rootly.AiChatStreamText{Content: "Created."}})
	model.handleEvent(activityDoneEvent("complete"))

	_, resumedCmd := model.finishTurn(nil)
	if output := commandSequenceOutput(resumedCmd); strings.Count(output, "✓ Create action item (") != 1 {
		t.Fatalf("resumed output = %q, want exactly one completed activity", output)
	}
}

func TestTUIConfirmationFlushesStatusFinalization(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.state = tuiStreaming
	model.handleEvent(activityStatusEvent("A..."))
	model.handleEvent(activityStatusEvent("B..."))
	model.handleEvent(Event{
		Type: rootly.AiChatStreamEventConfirmationRequired,
		ConfirmationRequired: &rootly.AiChatStreamConfirmationRequired{
			CheckpointID: testCheckpointID,
			Question:     "Approve?",
			ExpiresAt:    "2099-01-01T00:00:00Z",
		},
	})
	model.handleEvent(activityDoneEvent("confirmation_required"))

	_, cmd := model.finishTurn(nil)
	output := commandSequenceOutput(cmd)
	if strings.Count(output, "✓ A (") != 1 || strings.Contains(output, "✓ B (") {
		t.Fatalf("confirmation output = %q, want retained A line and no B line", output)
	}
}

func TestTUIFinishTurnRequestsWindowSizeAfterPrintedAnswer(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.answer.WriteString("Answer")
	model.currentPrompt = "Question"
	model.doneStatus = "complete"

	_, cmd := model.finishTurn(nil)
	if cmd == nil {
		t.Fatal("finishTurn() returned no command")
	}
	result := cmd()
	sequence := reflect.ValueOf(result)
	if sequence.Kind() != reflect.Slice || sequence.Len() == 0 {
		t.Fatalf("finishTurn command result = %T, want a non-empty sequence", result)
	}
	lastCommand, ok := sequence.Index(sequence.Len() - 1).Interface().(tea.Cmd)
	if !ok {
		t.Fatalf("last sequence entry = %T, want tea.Cmd", sequence.Index(sequence.Len()-1).Interface())
	}
	if got, want := reflect.TypeOf(lastCommand()), reflect.TypeOf(tea.RequestWindowSize()); got != want {
		t.Errorf("last command message type = %v, want window-size request type %v", got, want)
	}
}

func TestTUISessionCommandsRequireSessionAndMapErrors(t *testing.T) {
	apiClient := &fakeTUIAPI{}
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{API: apiClient})
	if output := commandOutput(model.runSlashCommand("/rename name")); !strings.Contains(output, "No session yet: send a message first.") {
		t.Errorf("no-session output = %q, want exact no-session message", output)
	}
	if len(apiClient.updateBodies) != 0 {
		t.Fatalf("updates = %d, want no API call without a session", len(apiClient.updateBodies))
	}

	sessionID := uuid.MustParse(testSessionID)
	model.sessionID = &sessionID
	apiClient.updateStatus = http.StatusForbidden
	cmd := model.runSlashCommand("/pin")
	result, ok := cmd().(sessionActionMsg)
	if !ok || result.err == nil || !strings.Contains(result.err.Error(), "ai.chat:write") {
		t.Fatalf("pin result = %#v, want mapped API permission error", result)
	}
	if len(apiClient.updateBodies) != 1 {
		t.Errorf("updates = %d, want one PATCH", len(apiClient.updateBodies))
	}
}

func TestTUISessionActionsMapNotFoundForBoundSessions(t *testing.T) {
	const want = "Session not found. Incident and alert chats can't be renamed, pinned, archived or deleted from the CLI yet."
	apiClient := &fakeTUIAPI{updateStatus: http.StatusNotFound, deleteStatus: http.StatusNotFound}
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{API: apiClient})
	sessionID := uuid.MustParse(testSessionID)
	model.sessionID = &sessionID

	for _, command := range []string{"/rename Updated title", "/pin", "/unpin", "/archive"} {
		t.Run(command, func(t *testing.T) {
			cmd := model.runSlashCommand(command)
			if cmd == nil {
				t.Fatal("session action returned no command")
			}
			msg := cmd()
			result, ok := msg.(sessionActionMsg)
			if !ok {
				t.Fatalf("command result = %T, want sessionActionMsg", msg)
			}
			if result.err == nil {
				t.Fatal("session action error = nil, want not-found error")
			}
			if output := commandOutput(model.handleSessionAction(result)); !strings.Contains(output, want) {
				t.Errorf("session action output = %q, want %q", output, want)
			}
		})
	}

	model.runSlashCommand("/delete")
	_, cmd := model.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd == nil {
		t.Fatal("delete confirmation returned no API command")
	}
	result, ok := cmd().(sessionActionMsg)
	if !ok || result.err == nil {
		t.Fatalf("delete result = %#v, want not-found sessionActionMsg", result)
	}
	if output := commandOutput(model.handleSessionAction(result)); !strings.Contains(output, want) {
		t.Errorf("delete output = %q, want %q", output, want)
	}
}

func TestTUIBindingCommandsShowUsageWithoutID(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
	for _, test := range []struct {
		command string
		want    string
	}{
		{command: "/incident", want: "Usage: /incident <id>"},
		{command: "/alert", want: "Usage: /alert <id>"},
	} {
		t.Run(test.command, func(t *testing.T) {
			if output := commandOutput(model.runSlashCommand(test.command)); !strings.Contains(output, test.want) {
				t.Errorf("output = %q, want %q", output, test.want)
			}
		})
	}
}

func TestTUIExportWritesTranscriptWithoutOverwriting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chat.md")
	sessionID := uuid.MustParse(testSessionID)
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
	model.sessionID = &sessionID
	model.transcript = []transcriptTurn{{user: "question", assistant: "answer"}}

	if cmd := model.exportTranscript(path); cmd == nil {
		t.Fatal("export returned no command")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read exported file: %v", err)
	}
	if !strings.Contains(string(got), "## You\n\nquestion") || !strings.Contains(string(got), "## Rootly\n\nanswer") {
		t.Errorf("export = %q, want both transcript sections", got)
	}
	if cmd := model.exportTranscript(path); cmd == nil {
		t.Fatal("second export returned no command")
	}
	gotAgain, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read exported file after repeated export: %v", err)
	}
	if !bytes.Equal(gotAgain, got) {
		t.Errorf("existing export changed: before %q, after %q", got, gotAgain)
	}
}

func TestTUIModelDropsStaleStreamMessages(t *testing.T) {
	streamer := &blockingStreamer{started: make(chan struct{}, 2)}
	model := NewTUIModel(context.Background(), streamer, TUIOptions{})

	model.startStream(&rootly.StreamAiChatParams{})
	firstTurn := model.turn
	<-streamer.started
	_, _ = model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

	model.startStream(&rootly.StreamAiChatParams{})
	secondTurn := model.turn
	t.Cleanup(func() {
		if model.cancel != nil {
			model.cancel()
		}
	})
	<-streamer.started

	_, _ = model.Update(streamMessage{turn: firstTurn, done: true})
	if model.state != tuiStreaming || model.turn != secondTurn {
		t.Errorf("state/turn = %v/%d, want streaming on turn %d", model.state, model.turn, secondTurn)
	}
}

func TestTUIModelCtrlCTwiceWhileIdleQuits(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
	_, firstCmd := model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if firstCmd != nil || model.status != "press Ctrl+C again to exit" {
		t.Fatalf("first Ctrl+C = %v, %q, want status message and no quit", firstCmd, model.status)
	}
	_, secondCmd := model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if secondCmd == nil {
		t.Fatal("second Ctrl+C did not return tea.Quit")
	}
	if _, ok := secondCmd().(tea.QuitMsg); !ok {
		t.Errorf("second Ctrl+C command result = %T, want tea.QuitMsg", secondCmd())
	}
}

func TestTUIModelConfirmStartsResume(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
	sessionID := uuid.MustParse(testSessionID)
	model.sessionID = &sessionID
	model.state = tuiConfirming
	model.pending = &rootly.AiChatStreamConfirmationRequired{
		CheckpointID: testCheckpointID,
		Question:     "Approve?",
		ExpiresAt:    "2099-01-01T00:00:00Z",
	}

	_, cmd := model.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd == nil || model.state != tuiStreaming {
		t.Fatalf("confirm key returned %v with state %v, want streaming resume", cmd, model.state)
	}
	if model.currentParams == nil || model.currentParams.Decision == nil || *model.currentParams.Decision != rootly.StreamAiChatParamsDecisionConfirm {
		t.Errorf("resume params = %#v, want decision confirm", model.currentParams)
	}
	if model.currentParams.Message != nil {
		t.Errorf("resume message = %v, want nil", model.currentParams.Message)
	}
}

func TestTUIModelExpiredConfirmationDoesNotResume(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
	sessionID := uuid.MustParse(testSessionID)
	model.sessionID = &sessionID
	model.state = tuiConfirming
	model.pending = &rootly.AiChatStreamConfirmationRequired{
		CheckpointID: testCheckpointID,
		Question:     "Approve?",
		ExpiresAt:    "2000-01-01T00:00:00Z",
	}

	_, cmd := model.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if model.state != tuiIdle || model.pending != nil || model.currentParams != nil {
		t.Errorf("expired confirmation state = %v, pending=%v, params=%v; want idle without resume", model.state, model.pending, model.currentParams)
	}
	if output := commandOutput(cmd); !strings.Contains(output, "Confirmation expired; no action was taken.") {
		t.Errorf("scrollback output = %q, want expiration message", output)
	}
}

func TestTUIModelExpiredCancelDoesNotResume(t *testing.T) {
	streamer := &fakeStreamer{}
	model := NewTUIModel(context.Background(), streamer, TUIOptions{})
	sessionID := uuid.MustParse(testSessionID)
	model.sessionID = &sessionID
	model.state = tuiConfirming
	model.pending = &rootly.AiChatStreamConfirmationRequired{
		CheckpointID: testCheckpointID,
		Question:     "Approve?",
		ExpiresAt:    "2000-01-01T00:00:00Z",
	}

	_, cmd := model.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if model.state != tuiIdle || model.pending != nil || len(streamer.calls) != 0 {
		t.Errorf("expired cancel state = %v, pending=%v, calls=%d; want idle without resume", model.state, model.pending, len(streamer.calls))
	}
	if output := commandOutput(cmd); !strings.Contains(output, "Confirmation expired; no action was taken.") {
		t.Errorf("scrollback output = %q, want expiration message", output)
	}
}

func TestTUIModelSeparatesResumedText(t *testing.T) {
	streamer := &fakeStreamer{responses: [][]rootly.AiChatStreamEvent{{}}}
	model := NewTUIModel(context.Background(), streamer, TUIOptions{})
	sessionID := uuid.MustParse(testSessionID)
	model.sessionID = &sessionID
	model.state = tuiConfirming
	model.pending = &rootly.AiChatStreamConfirmationRequired{
		CheckpointID: testCheckpointID,
		ExpiresAt:    "2099-01-01T00:00:00Z",
	}
	model.answer.WriteString("A")

	_ = model.handleConfirmationDecision(rootly.StreamAiChatParamsDecisionConfirm)
	model.handleEvent(Event{Type: rootly.AiChatStreamEventText, Text: &rootly.AiChatStreamText{Content: "B"}})
	if got := model.answer.String(); got != "A\n\nB" {
		t.Errorf("answer = %q, want resumed text separated by a blank line", got)
	}
	if model.cancel != nil {
		model.cancel()
	}
}

func TestTUIModelStreamErrorsPrintPartialAnswer(t *testing.T) {
	tests := []struct {
		name        string
		message     streamMessage
		doneStatus  string
		wantMessage string
	}{
		{
			name: "error event",
			message: streamMessage{event: Event{
				Type:  rootly.AiChatStreamEventError,
				Error: &rootly.AiChatStreamError{Message: "event failed"},
			}},
			wantMessage: "event failed",
		},
		{
			name:        "done error",
			message:     streamMessage{done: true},
			doneStatus:  "error",
			wantMessage: "AI chat turn failed",
		},
		{
			name:        "transport error",
			message:     streamMessage{done: true, err: errors.New("transport failed")},
			wantMessage: "transport failed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
			model.state = tuiStreaming
			model.doneStatus = test.doneStatus
			model.answer.WriteString("partial answer")

			_, cmd := model.Update(test.message)
			output := commandSequenceOutput(cmd)
			if !strings.Contains(output, "partial") || !strings.Contains(output, "answer") || !strings.Contains(output, test.wantMessage) {
				t.Errorf("scrollback output = %q, want partial answer and %q", output, test.wantMessage)
			}
			if model.state != tuiIdle {
				t.Errorf("state = %v, want idle", model.state)
			}
		})
	}
}

func TestTUIModelNoColorErrorHasNoANSI(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.state = tuiStreaming
	model.answer.WriteString("# partial answer")

	_, cmd := model.Update(streamMessage{done: true, err: errors.New("stream failed")})
	output := commandOutput(cmd)
	if strings.Contains(output, "\x1b[") {
		t.Errorf("scrollback output contains ANSI escape sequence: %q", output)
	}
	if !strings.Contains(output, "# partial answer") || !strings.Contains(output, "stream failed") {
		t.Errorf("scrollback output = %q, want raw Markdown and error", output)
	}
}

func TestTUIModelCachesMarkdownRenderingAcrossViews(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
	model.state = tuiStreaming
	model.answer.WriteString("A streamed answer")

	model.View()
	model.View()
	if model.renderCount != 1 {
		t.Errorf("render count = %d, want one render for unchanged answer", model.renderCount)
	}
}

func TestTUIModelResumeStatuses(t *testing.T) {
	tests := []struct {
		status      string
		wantMessage string
		wantStatus  string
	}{
		{status: "cancelled", wantMessage: "Action cancelled."},
		{status: "expired", wantMessage: "Confirmation expired; no action was taken."},
		{status: "confirmed"},
		{status: "pending", wantStatus: "turn halted: pending"},
	}
	for _, test := range tests {
		t.Run(test.status, func(t *testing.T) {
			model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
			model.state = tuiStreaming
			model.doneStatus = test.status
			model.answer.WriteString("reply")

			_, cmd := model.Update(streamMessage{done: true})
			output := commandSequenceOutput(cmd)
			if test.wantMessage != "" && !strings.Contains(output, test.wantMessage) {
				t.Errorf("scrollback output = %q, want %q", output, test.wantMessage)
			}
			if test.status == "confirmed" && !strings.Contains(output, "reply") {
				t.Errorf("confirmed output = %q, want completed answer", output)
			}
			if model.status != test.wantStatus {
				t.Errorf("status = %q, want %q", model.status, test.wantStatus)
			}
		})
	}
}

func TestTUIModelNewCommandClearsSession(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
	sessionID := uuid.MustParse(testSessionID)
	model.sessionID = &sessionID
	model.composer.SetValue("/new")

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.sessionID != nil {
		t.Errorf("session id = %v, want nil", model.sessionID)
	}
}

func TestTUIModelModifiedEnterInsertsNewline(t *testing.T) {
	for _, modifier := range []tea.KeyMod{tea.ModShift, tea.ModAlt} {
		model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
		model.composer.SetValue("question")

		_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: modifier})
		if model.state != tuiIdle {
			t.Errorf("modifier %v submitted the prompt", modifier)
		}
		if !strings.Contains(model.composer.Value(), "\n") {
			t.Errorf("modifier %v did not insert a newline: %q", modifier, model.composer.Value())
		}
	}
}

func commandOutput(cmd tea.Cmd) string {
	if cmd == nil {
		return ""
	}
	return fmt.Sprint(cmd())
}

func commandSequenceOutput(cmd tea.Cmd) string {
	if cmd == nil {
		return ""
	}
	result := cmd()
	sequence := reflect.ValueOf(result)
	if sequence.Kind() != reflect.Slice {
		return fmt.Sprint(result)
	}
	var output strings.Builder
	for index := 0; index < sequence.Len(); index++ {
		command, ok := sequence.Index(index).Interface().(tea.Cmd)
		if ok {
			fmt.Fprint(&output, command())
		}
	}
	return output.String()
}
