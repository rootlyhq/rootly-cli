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
	"charm.land/lipgloss/v2"
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

func TestTUIModelCommitsStreamedBlocksIntoTranscript(t *testing.T) {
	model := newSizedTUITestModel(80, 24)
	model.state = tuiStreaming
	model.turn = 1
	model.startNewTurn()

	_, cmd := model.Update(streamMessage{turn: 1, event: Event{
		Type: rootly.AiChatStreamEventText,
		Text: &rootly.AiChatStreamText{Content: "First block.\n\nSecond"},
	}})
	_ = cmd
	if got := ansi.Strip(model.Transcript()); strings.Count(got, "First block.") != 1 || strings.Count(got, "Second") != 1 {
		t.Errorf("transcript after first chunk = %q, want committed and live text once", got)
	}

	_, _ = model.Update(streamMessage{turn: 1, event: Event{
		Type: rootly.AiChatStreamEventText,
		Text: &rootly.AiChatStreamText{Content: " block.\n\nTail"},
	}})
	if got := ansi.Strip(model.Transcript()); strings.Count(got, "First block.") != 1 || strings.Count(got, "Second block.") != 1 || strings.Count(got, "Tail") != 1 {
		t.Errorf("transcript after second chunk = %q, want each block once", got)
	}

	model.handleEvent(activityDoneEvent("complete"))
	_, _ = model.Update(streamMessage{turn: 1, done: true})
	transcript := ansi.Strip(model.Transcript())
	for _, block := range []string{"First block.", "Second block.", "Tail"} {
		if got := strings.Count(transcript, block); got != 1 {
			t.Errorf("transcript contains %q %d times, want once: %q", block, got, transcript)
		}
	}
	if got := model.answer.String(); got != "First block.\n\nSecond block.\n\nTail" {
		t.Errorf("full answer = %q, want complete untruncated Markdown", got)
	}
}

func TestTUIStreamedMarkdownKeepsBlankLinesAcrossBlocks(t *testing.T) {
	for _, test := range []struct {
		name string
		done bool
	}{
		{name: "completed"},
		{name: "live"},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
			model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
			model.state = tuiStreaming
			model.turn = 1
			model.startNewTurn()

			for _, chunk := range []string{
				"Para one.\n\n",
				"Para two.\n\n",
				"```sh\necho hi\n\necho bye\n```\n\nAfter.",
			} {
				_, _ = model.Update(streamMessage{turn: 1, event: Event{
					Type: rootly.AiChatStreamEventText,
					Text: &rootly.AiChatStreamText{Content: chunk},
				}})
			}

			if test.done {
				model.handleEvent(activityDoneEvent("complete"))
				_, _ = model.Update(streamMessage{turn: 1, done: true})
			} else if got := model.uncommittedAnswer(); got != "After." {
				t.Fatalf("live remainder = %q, want After.", got)
			}

			transcript := ansi.Strip(model.Transcript())
			assertStreamedMarkdownBlockSpacing(t, transcript)
			if !test.done {
				assertStreamedMarkdownBlockSpacing(t, ansi.Strip(model.View().Content))
			}
		})
	}
}

func assertStreamedMarkdownBlockSpacing(t *testing.T, output string) {
	t.Helper()
	lines := strings.Split(output, "\n")
	findLine := func(fragment string, start int) int {
		for index := start; index < len(lines); index++ {
			if strings.Contains(strings.TrimSpace(lines[index]), fragment) {
				return index
			}
		}
		return -1
	}

	paragraphOne := findLine("● Para one.", 0)
	paragraphTwo := findLine("Para two.", paragraphOne+1)
	echoHi := findLine("echo hi", paragraphTwo+1)
	echoBye := findLine("echo bye", echoHi+1)
	after := findLine("After.", echoBye+1)
	if paragraphOne < 0 || paragraphTwo < 0 || echoHi < 0 || echoBye < 0 || after < 0 {
		t.Fatalf("streamed answer content missing from output: %q", output)
	}
	for index, want := range map[int]string{
		paragraphOne: "● Para one.",
		paragraphTwo: "  Para two.",
		after:        "  After.",
	} {
		if got := strings.TrimRight(lines[index], " "); got != want {
			t.Errorf("answer line = %q, want %q", got, want)
		}
	}

	for _, pair := range [][2]int{{paragraphOne, paragraphTwo}, {echoHi, echoBye}} {
		if pair[1]-pair[0] != 2 || strings.TrimSpace(lines[pair[0]+1]) != "" {
			t.Errorf("lines between %q and %q = %q, want exactly one blank row", lines[pair[0]], lines[pair[1]], lines[pair[0]+1:pair[1]])
		}
	}
	if strings.TrimSpace(lines[paragraphTwo+1]) != "" {
		t.Errorf("paragraph/code separator = %q, want one blank row", lines[paragraphTwo+1])
	}
	if after-echoBye < 2 || strings.TrimSpace(lines[after-1]) != "" {
		t.Errorf("code/paragraph separator = %q, want one blank row before After.", lines[echoBye+1:after])
	}

	blankRun := 0
	for _, line := range lines[paragraphOne : after+1] {
		if strings.TrimSpace(line) == "" {
			blankRun++
			if blankRun > 1 {
				t.Errorf("answer contains multiple adjacent blank rows: %q", lines[paragraphOne:after+1])
				return
			}
		} else {
			blankRun = 0
		}
	}
}

func TestTUIStreamedFortyFiveStepsPreserveFenceAndBlankLine(t *testing.T) {
	model := newSizedTUITestModel(125, 34)
	model.state = tuiStreaming
	model.turn = 1
	model.startNewTurn()
	var answer strings.Builder
	for step := 1; step <= 45; step++ {
		fmt.Fprintf(&answer, "Step %d: perform the action.\n\n", step)
	}
	answer.WriteString("```sh\nprintf 'first command'\n\necho second command\n```\n\n")
	_, _ = model.Update(streamMessage{turn: 1, event: Event{
		Type: rootly.AiChatStreamEventText,
		Text: &rootly.AiChatStreamText{Content: answer.String()},
	}})
	model.handleEvent(activityDoneEvent("complete"))
	_, _ = model.Update(streamMessage{turn: 1, done: true})
	transcript := ansi.Strip(model.Transcript())
	for step := 1; step <= 45; step++ {
		marker := fmt.Sprintf("Step %d:", step)
		if got := strings.Count(transcript, marker); got != 1 {
			t.Errorf("%q appears %d times in transcript, want once", marker, got)
		}
	}
	for _, fragment := range []string{"```sh", "printf 'first command'", "echo second command"} {
		if !strings.Contains(transcript, fragment) {
			t.Errorf("transcript lost fenced content %q: %q", fragment, transcript)
		}
	}
	if !strings.Contains(transcript, "printf 'first command'\n  \n  echo") {
		t.Errorf("transcript lost the blank line inside the fenced block: %q", transcript)
	}
}

func TestTUIFixedFooterAtCommonTerminalSizes(t *testing.T) {
	for _, size := range []struct{ width, height int }{{80, 24}, {125, 34}} {
		for _, blocks := range []int{0, 60} {
			model := newSizedTUITestModel(size.width, size.height)
			for index := 0; index < blocks; index++ {
				model.println(fmt.Sprintf("transcript block %02d", index+1))
			}
			lines := strings.Split(model.View().Content, "\n")
			if len(lines) != size.height {
				t.Fatalf("%dx%d with %d blocks rendered %d rows", size.width, size.height, blocks, len(lines))
			}
			if got := ansi.Strip(lines[size.height-1]); !strings.Contains(got, "/ for commands · exit to quit") {
				t.Errorf("%dx%d hint row = %q", size.width, size.height, got)
			}
			if got := ansi.Strip(lines[size.height-2]); !strings.Contains(got, "─") {
				t.Errorf("%dx%d bottom rule = %q", size.width, size.height, got)
			}
			if got := ansi.Strip(lines[size.height-3]); !strings.Contains(got, "Ask Rootly") {
				t.Errorf("%dx%d composer row = %q", size.width, size.height, got)
			}
		}
	}
}

func setTUIStreamingTestClock(model *TUIModel) {
	now := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	model.now = func() time.Time { return now }
	model.startedAt = now.Add(-3 * time.Second)
	model.turnStartedAt = model.startedAt
	model.turnVerbIndex = 0
}

func TestTUIStreamingStatusIsPinnedAndRemovedAfterDone(t *testing.T) {
	model := newSizedTUITestModel(80, 24)
	setTUIStreamingTestClock(model)
	model.state = tuiStreaming

	footer := strings.Split(model.footerContent(), "\n")
	if len(footer) != 6 {
		t.Fatalf("streaming footer has %d rows, want 6: %q", len(footer), footer)
	}
	if got := ansi.Strip(footer[0]); !strings.Contains(got, "Triaging… · 3s") {
		t.Errorf("streaming footer status = %q", got)
	}
	if footer[1] != "" {
		t.Errorf("row between status and rule = %q, want blank", footer[1])
	}
	if got := ansi.Strip(footer[2]); !strings.Contains(got, "─") {
		t.Errorf("streaming footer top rule = %q", got)
	}
	if got := ansi.Strip(model.liveSection()); strings.Contains(got, "Triaging") {
		t.Errorf("live section contains streaming status: %q", got)
	}
	if got := ansi.Strip(model.transcriptContent()); strings.Contains(got, "Triaging") {
		t.Errorf("transcript content contains streaming status: %q", got)
	}
	if got := ansi.Strip(model.Transcript()); strings.Contains(got, "Triaging") {
		t.Errorf("exit transcript contains streaming status: %q", got)
	}

	model.currentPrompt = "Question"
	model.answer.WriteString("Answer")
	model.doneReceived = true
	model.doneStatus = "complete"
	_, _ = model.finishTurn(nil)

	footer = strings.Split(model.footerContent(), "\n")
	if len(footer) != 4 {
		t.Fatalf("completed footer has %d rows, want 4: %q", len(footer), footer)
	}
	if got := ansi.Strip(footer[0]); !strings.Contains(got, "─") || strings.Contains(got, "Triaging") {
		t.Errorf("completed footer starts with %q, want the rule and no status", got)
	}
	transcript := ansi.Strip(model.Transcript())
	if !strings.Contains(transcript, "✻ Triaged for 3s") {
		t.Errorf("completed transcript lost turn footer: %q", transcript)
	}
	if strings.Contains(transcript, "Triaging") {
		t.Errorf("completed transcript contains streaming status: %q", transcript)
	}
}

func TestTUIStreamingFooterHeightPreservesViewportPosition(t *testing.T) {
	bottomModel := newSizedTUITestModel(80, 24)
	for index := 0; index < 60; index++ {
		bottomModel.println(fmt.Sprintf("history block %02d", index+1))
	}
	bottomModel.View()
	if !bottomModel.viewport.AtBottom() {
		t.Fatal("viewport did not start at the bottom")
	}
	setTUIStreamingTestClock(bottomModel)
	bottomModel.state = tuiStreaming
	bottomModel.View()
	if !bottomModel.viewport.AtBottom() {
		t.Fatal("viewport left the bottom when the streaming footer grew")
	}

	scrolledModel := newSizedTUITestModel(80, 24)
	for index := 0; index < 60; index++ {
		scrolledModel.println(fmt.Sprintf("history block %02d", index+1))
	}
	scrolledModel.View()
	scrolledModel.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	offset := scrolledModel.viewport.YOffset()
	if offset == 0 {
		t.Fatal("PgUp did not scroll the viewport")
	}
	setTUIStreamingTestClock(scrolledModel)
	scrolledModel.state = tuiStreaming
	scrolledModel.View()
	if got := scrolledModel.viewport.YOffset(); got != offset {
		t.Errorf("footer growth moved scrolled offset from %d to %d", offset, got)
	}
}

func TestTUIStreamingFooterStaysVisibleWhileScrolled(t *testing.T) {
	const height = 24
	model := newSizedTUITestModel(80, height)
	setTUIStreamingTestClock(model)
	model.state = tuiStreaming
	model.answer.WriteString(strings.Repeat("streamed answer paragraph with enough text to wrap\n\n", 40))
	model.View()
	model.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	offset := model.viewport.YOffset()
	if offset == 0 {
		t.Fatal("PgUp did not scroll the streamed answer")
	}

	model.answer.WriteString("additional streamed answer")
	view := model.View().Content
	if got := model.viewport.YOffset(); got != offset {
		t.Errorf("new streamed content moved scrolled offset from %d to %d", offset, got)
	}
	lines := strings.Split(view, "\n")
	if len(lines) != height {
		t.Fatalf("view rendered %d rows, want %d", len(lines), height)
	}
	statusRow := height - 6
	if got := ansi.Strip(lines[statusRow]); !strings.Contains(got, "Triaging… · 3s") {
		t.Errorf("row %d = %q, want streaming status", statusRow, got)
	}
	if got := ansi.Strip(lines[height-3]); !strings.Contains(got, "Ask Rootly") {
		t.Errorf("composer row = %q", got)
	}
	if got := ansi.Strip(model.transcriptContent()); strings.Contains(got, "Triaging") {
		t.Errorf("streaming transcript contains status line: %q", got)
	}
}

func TestTUIStreamingStatusTruncatesAtNarrowWidth(t *testing.T) {
	model := newSizedTUITestModel(20, 12)
	setTUIStreamingTestClock(model)
	model.state = tuiStreaming
	model.status = strings.Repeat("streaming status ", 5)

	if got := lipgloss.Width(model.streamingStatusLine()); got != model.width-1 {
		t.Errorf("streaming status width = %d, want %d", got, model.width-1)
	}
}

func TestTUIViewportFollowControls(t *testing.T) {
	model := newSizedTUITestModel(80, 24)
	for index := 0; index < 60; index++ {
		model.println(fmt.Sprintf("history block %02d", index+1))
	}
	model.View()
	model.println("bottom-followed")
	model.View()
	if !strings.Contains(model.viewport.GetContent(), "bottom-followed") || !model.viewport.AtBottom() {
		t.Fatal("viewport did not follow appended content while at bottom")
	}

	model.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	offset := model.viewport.YOffset()
	if offset == 0 {
		t.Fatal("PgUp did not scroll the viewport")
	}
	model.println("new output while scrolled")
	view := ansi.Strip(model.View().Content)
	if model.viewport.YOffset() != offset {
		t.Errorf("append moved scrolled offset from %d to %d", offset, model.viewport.YOffset())
	}
	if !strings.Contains(view, "↓ new output · ctrl+end to jump") {
		t.Errorf("scrolled hint = %q, want new-output indicator", lastLine(view))
	}

	model.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	view = ansi.Strip(model.View().Content)
	if !model.viewport.AtBottom() || strings.Contains(view, "↓ new output") {
		t.Errorf("End did not return to bottom and clear the indicator")
	}

	model.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if model.viewport.AtBottom() {
		t.Fatal("mouse wheel up did not scroll away from the bottom")
	}
	offset = model.viewport.YOffset()
	for _, text := range []string{"j", "k", " "} {
		model.Update(tea.KeyPressMsg{Code: rune(text[0]), Text: text})
	}
	if model.viewport.YOffset() != offset {
		t.Errorf("typed j/k/space moved viewport from %d to %d", offset, model.viewport.YOffset())
	}
	if got := model.composer.Value(); got != "jk " {
		t.Errorf("composer value = %q, want typed j/k/space", got)
	}

	model.composer.SetValue("end")
	model.composer.CursorStart()
	offset = model.viewport.YOffset()
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	if model.viewport.YOffset() != offset {
		t.Errorf("plain End moved viewport from %d to %d while composer had text", offset, model.viewport.YOffset())
	}
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if got := model.composer.Value(); got != "en" {
		t.Errorf("Backspace after End left composer value %q, want cursor at line end", got)
	}

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyHome, Mod: tea.ModCtrl})
	if model.viewport.YOffset() != 0 {
		t.Error("Ctrl+Home did not scroll the viewport to the top with composer text")
	}
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnd, Mod: tea.ModCtrl})
	if !model.viewport.AtBottom() {
		t.Error("Ctrl+End did not scroll the viewport to the bottom with composer text")
	}
}

func TestTUISessionPickerConsumesPageDownWithoutScrollingViewport(t *testing.T) {
	model := newSizedTUITestModel(80, 24)
	for index := 0; index < 60; index++ {
		model.println(fmt.Sprintf("history block %02d", index+1))
	}
	model.View()
	model.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})

	sessions := make([]rootly.AiChatSession, 48)
	for index := range sessions {
		sessions[index] = testSession(uuid.NewString(), fmt.Sprintf("Session %02d", index+1))
	}
	apiClient := &fakeTUIAPI{sessionPages: []*rootly.ListAiChatSessionsResponse{
		testSessionPage(sessions, false, ""),
	}}
	model.api = apiClient
	pageCommand := model.openSessionPicker()
	result := pageCommand()
	pageMessage, ok := result.(sessionPageMsg)
	if !ok {
		t.Fatalf("picker command result = %T, want sessionPageMsg", result)
	}
	_, _ = model.Update(pageMessage)
	model.View()

	if model.sessionPicker.Paginator.TotalPages < 2 {
		t.Fatalf("picker total pages = %d, want multiple pages", model.sessionPicker.Paginator.TotalPages)
	}
	page := model.sessionPicker.Paginator.Page
	offset := model.viewport.YOffset()
	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	if model.sessionPicker.Paginator.Page <= page {
		t.Errorf("picker page = %d after PgDn, want a page after %d", model.sessionPicker.Paginator.Page, page)
	}
	if model.viewport.YOffset() != offset {
		t.Errorf("PgDn moved viewport from %d to %d while picker was open", offset, model.viewport.YOffset())
	}
}

func newSizedTUITestModel(width, height int) *TUIModel {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return model
}

func TestLastTopLevelBlockBoundaryIgnoresFencedBlankLines(t *testing.T) {
	for _, marker := range []string{"```", "~~~"} {
		answer := "Intro.\n\n" + marker + "go\nfirst\n\nsecond\n" + marker + "\n\nTail"
		boundary := lastTopLevelBlockBoundary(answer, 0)
		if got := answer[boundary:]; got != "Tail" {
			t.Errorf("tail after %s fence = %q, want Tail", marker, got)
		}
		if block := answer[:boundary]; !strings.Contains(block, "first\n\nsecond") {
			t.Errorf("committed block split %s fence contents: %q", marker, block)
		}
	}
}

func TestStreamedOrderedListKeepsNumberAcrossCommittedBlocks(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
	model.state = tuiStreaming
	model.turn = 1

	_, _ = model.Update(streamMessage{turn: 1, event: Event{
		Type: rootly.AiChatStreamEventText,
		Text: &rootly.AiChatStreamText{Content: "1. First item\n\n"},
	}})
	_, _ = model.Update(streamMessage{turn: 1, event: Event{
		Type: rootly.AiChatStreamEventText,
		Text: &rootly.AiChatStreamText{Content: "2. Second item\n\n3. Third item"},
	}})
	got := ansi.Strip(model.Transcript())
	if !strings.Contains(got, "1. First item") || !strings.Contains(got, "2. Second item") {
		t.Errorf("printed list blocks lost their explicit numbering: %q", got)
	}
}

func TestCtrlCPrintsOnlyUncommittedStreamRemainder(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.state = tuiStreaming
	model.answer.WriteString("Committed block\n\nUncommitted tail")
	model.committedOffset = len("Committed block\n\n")

	_, cmd := model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	_ = cmd
	output := ansi.Strip(model.Transcript())
	if strings.Contains(output, "Committed block") || strings.Count(output, "Uncommitted tail") != 1 {
		t.Errorf("Ctrl+C output = %q, want only the uncommitted remainder", output)
	}
	if strings.Count(output, "Stopped locally.") != 1 {
		t.Errorf("Ctrl+C notice count in %q, want one", output)
	}
}

func TestConfirmationRequiredPrintsOnlyUncommittedStreamRemainder(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.state = tuiStreaming
	model.answer.WriteString("Committed block\n\nUncommitted tail")
	model.committedOffset = len("Committed block\n\n")
	model.pending = &rootly.AiChatStreamConfirmationRequired{Question: "Approve?"}
	model.handleEvent(activityDoneEvent("confirmation_required"))

	_, cmd := model.Update(streamMessage{turn: model.turn, done: true})
	_ = cmd
	output := ansi.Strip(model.Transcript())
	if strings.Contains(output, "Committed block") || strings.Count(output, "Uncommitted tail") != 1 {
		t.Errorf("confirmation output = %q, want only the uncommitted remainder", output)
	}
	if model.state != tuiConfirming {
		t.Errorf("state = %v, want confirming", model.state)
	}
}

func TestResumedHistoryPrintsFullAssistantMessageRegardlessOfCommitOffset(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.committedOffset = len("old answer")

	model.printHistory([]rootly.AiChatSessionMessage{{
		Role: rootly.AiChatSessionMessageRole("assistant"), Content: "historical answer",
	}})
	output := ansi.Strip(model.Transcript())
	if got := strings.Count(output, "historical answer"); got != 1 {
		t.Errorf("history output = %q, want the complete assistant message once", output)
	}
}

func TestTUIViewClipsTallContentAboveComposer(t *testing.T) {
	model := newSizedTUITestModel(40, 10)
	model.status = strings.Repeat("long status content that wraps around the terminal width\n", 30)

	view := model.View().Content
	if got := lipgloss.Height(view); got != 10 {
		t.Fatalf("full-screen view height = %d, want 10", got)
	}
	lines := strings.Split(view, "\n")
	if got := lines[len(lines)-1]; got != "/ for commands · exit to quit" {
		t.Errorf("last row = %q, want composer hint", got)
	}
	if !strings.Contains(lines[len(lines)-3], "❯ ") {
		t.Errorf("composer is not visible above the bottom rule: %q", view)
	}
}

func TestTUIViewBoundsLiveAnswerAndAvoidsTerminalWidthLines(t *testing.T) {
	model := newSizedTUITestModel(40, 24)
	model.state = tuiStreaming
	model.startedAt = time.Now()
	for step := 1; step <= 45; step++ {
		_, _ = fmt.Fprintf(&model.answer, "%d. Step %d details %s\n\n", step, step, strings.Repeat("x", 52))
	}
	_, _ = fmt.Fprintf(&model.answer, "```sh\n%s\n\n%s\n```\n45. Final marker\n", strings.Repeat("a", 80), strings.Repeat("b", 80))

	view := model.View().Content
	rendererRows := strings.Count(view, "\n") + 1
	if rendererRows != model.height {
		t.Fatalf("renderer view rows = %d, want terminal height %d", rendererRows, model.height)
	}
	if !strings.Contains(view, "45. Final marker") {
		t.Errorf("view tail = %q, want the final answer marker", view)
	}
	if strings.Contains(view, "1. Step 1 details") {
		t.Errorf("view retained the start of the long live answer: %q", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > model.width {
			t.Errorf("view line width = %d, want at most terminal width %d: %q", got, model.width, line)
		}
	}
}

func TestTUIRenderedAnswerLinesStayInsideTerminalWidth(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.width = 40

	rendered := model.renderAssistantAnswerPart(strings.Repeat("x", model.width-2), true)
	for _, line := range strings.Split(rendered, "\n") {
		if got := lipgloss.Width(line); got >= model.width {
			t.Errorf("printed answer line width = %d, want less than terminal width %d: %q", got, model.width, line)
		}
	}
}

func TestTUIAnswerBulletStartsOnFirstRenderedListLine(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
	model.width = 80

	rendered := model.renderAssistantAnswerPart("1. a\n2. b", true)
	lines := strings.Split(ansi.Strip(rendered), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "● 1. a" {
		t.Errorf("rendered answer = %q, want first line to begin with bullet and first list item", rendered)
	}
}

func TestTUIBindingHeaderIsPrintedOnceAfterWelcomeSettles(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{
		Incident: &api.Incident{SequentialID: "INC-8", Title: "Database latency"},
	})
	model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	header := "INC-8 · Database latency"
	if got := strings.Count(ansi.Strip(model.welcomeBanner()), header); got != 1 {
		t.Errorf("header context occurs %d times, want once: %q", got, model.welcomeBanner())
	}
	status := model.statusLines()
	if strings.Contains(status, "Chatting about") || !strings.Contains(status, "binding INC-8: Database latency") {
		t.Errorf("status = %q, want binding as a field without repeating the header", status)
	}
}

func TestTUIModelDonePrintsAndReturnsIdle(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
	model.state = tuiStreaming
	model.answer.WriteString("assistant reply")
	model.doneStatus = "complete"
	model.doneReceived = true

	updated, cmd := model.Update(streamMessage{done: true})
	if got := updated.(*TUIModel).state; got != tuiIdle {
		t.Errorf("state = %v, want idle", got)
	}
	if cmd != nil {
		t.Fatalf("done returned %T, want transcript-only update", cmd)
	}
	if got := ansi.Strip(model.Transcript()); !strings.Contains(got, "assistant reply") {
		t.Errorf("transcript = %q, want completed answer", got)
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
	_ = cmd
	output := strings.Join(strings.Fields(ansi.Strip(model.Transcript())), " ")
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
		if width := ansi.StringWidth(line); width != 39 {
			t.Errorf("prompt echo width = %d, want 39: %q", width, line)
		}
	}
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	answer := model.renderAssistantAnswer("Hello\nworld")
	if answer != "● Hello\n  world" || strings.Contains(answer, "Assistant:") {
		t.Errorf("assistant transcript = %q, want bullet and indented continuation without label", answer)
	}
}

func TestTUITranscriptIncludesHeaderAndConversation(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.println("❯ question")
	model.println("completed answer")
	model.state = tuiStreaming
	model.answer.WriteString("live answer")

	transcript := model.Transcript()
	for _, text := range []string{"New session", "❯ question", "completed answer", "live answer"} {
		if !strings.Contains(transcript, text) {
			t.Errorf("transcript = %q, missing %q", transcript, text)
		}
	}
}

func TestTUITranscriptSpacingAcrossTurns(t *testing.T) {
	model := newSizedTUITestModel(80, 24)
	model.println("❯ first question")
	model.println("● first answer")
	model.println("✻ Triaged for 2s · done")
	model.println("❯ second question")
	model.View()

	for name, output := range map[string]string{
		"viewport":   model.viewport.GetContent(),
		"transcript": model.Transcript(),
	} {
		lines := strings.Split(strings.TrimRight(ansi.Strip(output), " \n"), "\n")
		headerIndex := -1
		for index, line := range lines {
			if strings.Contains(line, "New session") {
				headerIndex = index
				break
			}
		}
		if headerIndex < 0 {
			t.Fatalf("%s output has no session header: %q", name, output)
		}
		want := []string{
			"New session",
			"",
			"❯ first question",
			"● first answer",
			"✻ Triaged for 2s · done",
			"",
			"❯ second question",
		}
		for offset, expected := range want {
			lineIndex := headerIndex + offset
			if lineIndex >= len(lines) {
				t.Fatalf("%s output ended before line %q: %q", name, expected, output)
			}
			got := strings.TrimSpace(lines[lineIndex])
			if expected == "New session" {
				if !strings.HasSuffix(got, expected) {
					t.Errorf("%s line %d = %q, want header ending in %q", name, lineIndex+1, got, expected)
				}
			} else if got != expected {
				t.Errorf("%s line %d = %q, want %q", name, lineIndex+1, got, expected)
			}
		}
		for index := 0; index+1 < len(lines); index++ {
			if strings.TrimSpace(lines[index]) == "" && strings.TrimSpace(lines[index+1]) == "" {
				t.Errorf("%s output has adjacent blank lines at %d: %q", name, index+1, output)
				break
			}
		}
	}
}

func TestTUIHelpMentionsViewportNavigationAndTextSelection(t *testing.T) {
	helpText := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{}).helpView()
	for _, text := range []string{
		"PgUp/PgDn/wheel scroll · Ctrl+Home/Ctrl+End scroll · Home/End scroll when composer is empty",
		"Hold Option (macOS) / Shift to select text",
	} {
		if !strings.Contains(helpText, text) {
			t.Errorf("help is missing %q", text)
		}
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
	historyModel.printHistory([]rootly.AiChatSessionMessage{
		{Role: rootly.AiChatSessionMessageRole("user"), Content: "Prompt " + attack},
		{Role: rootly.AiChatSessionMessageRole("assistant"), Content: "Answer " + attack},
	})
	history := historyModel.Transcript()
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
	_, _ = model.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	if model.composer.MaxHeight != 6 {
		t.Errorf("composer max height after resize = %d, want 6", model.composer.MaxHeight)
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
	model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	messages := make([]rootly.AiChatSessionMessage, 0, 40)
	for index := 0; index < 20; index++ {
		messages = append(messages,
			rootly.AiChatSessionMessage{Role: rootly.AiChatSessionMessageRole("user"), Content: fmt.Sprintf("older prompt %d", index)},
			rootly.AiChatSessionMessage{Role: rootly.AiChatSessionMessageRole("assistant"), Content: strings.Repeat("historical answer ", 40)},
		)
	}

	updated, _ := model.Update(historyLoadedMsg{
		request:  1,
		title:    "Resumed session",
		messages: messages,
	})
	if updated.(*TUIModel).state != tuiIdle {
		t.Fatalf("state = %v, want idle after history loads", updated.(*TUIModel).state)
	}
	if view := ansi.Strip(model.View().Content); !strings.Contains(view, "❯ Ask Rootly...") || !strings.Contains(view, "/ for commands · exit to quit") {
		t.Errorf("view after history load = %q, want composer visible", view)
	}
}

func TestTUIStreamingTrimsOnlyIncompleteColorLinks(t *testing.T) {
	const partial = "see [Rotate](http://x?a_b"
	colorModel := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
	colorModel.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
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

	rawModel := newSizedTUITestModel(80, 24)
	rawModel.state = tuiStreaming
	rawModel.answer.WriteString(partial)
	if view := rawModel.View().Content; !strings.Contains(view, partial) {
		t.Errorf("no-color streaming view = %q, want raw Markdown %q", view, partial)
	}

	const completeURL = "http://x?a_b"
	completeModel := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
	completeModel.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
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
			ApplicationVndAPIJSON200: &rootly.AiChatSessionMessageList{Title: "Incident review\x1b[2J"},
		},
	}}
	sessionID := uuid.MustParse(testSessionID)
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{
		NoColor:   true,
		API:       apiClient,
		SessionID: &sessionID,
	})
	initSequence := reflect.ValueOf(model.Init()())
	if initSequence.Kind() != reflect.Slice || initSequence.Len() != 2 {
		t.Fatalf("initial resume commands = %v, want composer focus and history load", initSequence)
	}

	history := model.loadHistory(sessionID)().(historyLoadedMsg)
	updated, _ := model.Update(history)
	resumed := updated.(*TUIModel)
	if resumed.sessionTitle != "Incident review" {
		t.Fatalf("session title = %q, want sanitized history title", resumed.sessionTitle)
	}
	if output := ansi.Strip(resumed.Transcript()); !strings.Contains(output, "Incident review") {
		t.Errorf("resume transcript = %q, want the loaded session title in the header", output)
	}
	if status := resumed.statusLines(); !strings.Contains(status, "title Incident review") {
		t.Errorf("status lines = %q, want loaded session title", status)
	}
	if got := strings.Count(ansi.Strip(resumed.Transcript()), "Incident review"); got != 1 {
		t.Errorf("resumed title appears %d times in transcript, want one header context", got)
	}
}

func TestTUIResumeByIDFallsBackToShortIDWithoutTitle(t *testing.T) {
	for _, test := range []struct {
		name  string
		title string
	}{
		{name: "absent"},
		{name: "untitled", title: "Untitled"},
	} {
		t.Run(test.name, func(t *testing.T) {
			apiClient := &fakeTUIAPI{messagePages: []*rootly.ListAiChatSessionMessagesResponse{
				{
					ApplicationVndAPIJSON200: &rootly.AiChatSessionMessageList{Title: test.title},
				},
			}}
			sessionID := uuid.MustParse(testSessionID)
			model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{
				NoColor:   true,
				API:       apiClient,
				SessionID: &sessionID,
			})

			history := model.loadHistory(sessionID)().(historyLoadedMsg)
			updated, _ := model.Update(history)
			resumed := updated.(*TUIModel)
			if resumed.sessionTitle != "" {
				t.Fatalf("session title = %q, want empty fallback state", resumed.sessionTitle)
			}
			if got, want := resumed.sessionHeaderContext(), shortSessionID(testSessionID); got != want {
				t.Errorf("resume header context = %q, want %q", got, want)
			}
		})
	}
}

func TestTUITaskActivityIsTerminalAndDeduplicated(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.handleEvent(Event{Type: rootly.AiChatStreamEventTaskUpdate, TaskUpdate: &rootly.AiChatStreamTaskUpdate{ID: "task-1", Title: "Create incident", Status: "in_progress"}})
	model.handleEvent(Event{Type: rootly.AiChatStreamEventTaskUpdate, TaskUpdate: &rootly.AiChatStreamTaskUpdate{ID: "task-1", Title: "Create incident", Status: "complete"}})
	model.handleEvent(Event{Type: rootly.AiChatStreamEventTaskUpdate, TaskUpdate: &rootly.AiChatStreamTaskUpdate{ID: "task-1", Title: "Create incident", Status: "complete"}})
	model.handleEvent(Event{Type: rootly.AiChatStreamEventTaskUpdate, TaskUpdate: &rootly.AiChatStreamTaskUpdate{ID: "task-2", Title: "Notify owner", Status: "cancelled"}})
	model.handleEvent(Event{Type: rootly.AiChatStreamEventTaskUpdate, TaskUpdate: &rootly.AiChatStreamTaskUpdate{ID: "task-3", Title: "Searched incidents (1.2s)", Status: "complete"}})
	got := ansi.Strip(model.Transcript())
	if strings.Count(got, "✓ Create incident") != 1 || strings.Count(got, "✗ Notify owner") != 1 {
		t.Errorf("task lines = %q; want one success and one failure glyph", got)
	}
	if !strings.Contains(got, "✓ Searched incidents (1.2s)") || strings.Contains(got, "(1.2s) (") {
		t.Errorf("duration task line = %q, want title duration only", got)
	}
}

func TestTUIStatusActivityIsPersisted(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.handleEvent(activityStatusEvent("Get incidents..."))
	model.handleEvent(activityStatusEvent("Composing response..."))
	model.handleEvent(Event{Type: rootly.AiChatStreamEventText, Text: &rootly.AiChatStreamText{Content: "Answer"}})
	model.handleEvent(activityDoneEvent("complete"))
	if got := ansi.Strip(model.Transcript()); !strings.Contains(got, "✓ Get incidents (") {
		t.Errorf("activity transcript = %q, want successful Get incidents", got)
	}
}

func TestTUIFinishTurnTreatsConfirmedActivityAsSuccessful(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.activities.Handle(activityStatusEvent("Get incidents..."))
	model.doneStatus = "confirmed"
	model.doneReceived = true

	_, _ = model.finishTurn(nil)
	if output := ansi.Strip(model.Transcript()); !strings.Contains(output, "✓ Get incidents (") {
		t.Errorf("finish output = %q, want successful activity line", output)
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

	_, _ = model.finishTurn(nil)
	if output := ansi.Strip(model.Transcript()); strings.Contains(output, "✓ Create action item (") {
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

	_, _ = model.finishTurn(nil)
	if output := ansi.Strip(model.Transcript()); strings.Count(output, "✓ Create action item (") != 1 {
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

	_, _ = model.finishTurn(nil)
	output := ansi.Strip(model.Transcript())
	if strings.Count(output, "✓ A (") != 1 || strings.Contains(output, "✓ B (") {
		t.Fatalf("confirmation output = %q, want retained A line and no B line", output)
	}
}

func TestTUIFinishTurnAppendsAnswerToTranscript(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	model.println(renderPromptEcho("Question", model.width, model.noColor))
	model.answer.WriteString("Answer")
	model.currentPrompt = "Question"
	model.doneStatus = "complete"
	model.doneReceived = true

	_, cmd := model.finishTurn(nil)
	if cmd != nil {
		t.Fatalf("finishTurn() command = %T, want no inline print command", cmd)
	}
	if got := ansi.Strip(model.Transcript()); !strings.Contains(got, "Answer") || !strings.Contains(got, "Question") {
		t.Errorf("transcript = %q, want prompt and answer", got)
	}
}

func TestTUISessionCommandsRequireSessionAndMapErrors(t *testing.T) {
	apiClient := &fakeTUIAPI{}
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{API: apiClient})
	_ = model.runSlashCommand("/rename name")
	if output := ansi.Strip(model.Transcript()); !strings.Contains(output, "No session yet: send a message first.") {
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
			_ = model.handleSessionAction(result)
			if output := strings.Join(strings.Fields(ansi.Strip(model.Transcript())), " "); !strings.Contains(output, want) {
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
	_ = model.handleSessionAction(result)
	if output := strings.Join(strings.Fields(ansi.Strip(model.Transcript())), " "); !strings.Contains(output, want) {
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
			_ = model.runSlashCommand(test.command)
			if output := ansi.Strip(model.Transcript()); !strings.Contains(output, test.want) {
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
	model.conversation = []transcriptTurn{{user: "question", assistant: "answer"}}

	if cmd := model.exportTranscript(path); cmd != nil {
		t.Fatalf("export command = %T, want transcript-only update", cmd)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read exported file: %v", err)
	}
	if !strings.Contains(string(got), "## You\n\nquestion") || !strings.Contains(string(got), "## Rootly\n\nanswer") {
		t.Errorf("export = %q, want both transcript sections", got)
	}
	if cmd := model.exportTranscript(path); cmd != nil {
		t.Fatalf("second export command = %T, want transcript-only update", cmd)
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
	if view := model.View().Content; view != "" {
		t.Errorf("quitting view = %q, want empty view", view)
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
	_ = cmd
	if output := ansi.Strip(model.Transcript()); !strings.Contains(output, "Confirmation expired; no action was taken.") {
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
	_ = cmd
	if output := ansi.Strip(model.Transcript()); !strings.Contains(output, "Confirmation expired; no action was taken.") {
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
		name         string
		message      streamMessage
		doneStatus   string
		doneReceived bool
		wantMessage  string
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
			name:         "done error",
			message:      streamMessage{done: true},
			doneStatus:   "error",
			doneReceived: true,
			wantMessage:  "AI chat turn failed",
		},
		{
			name:        "truncated stream",
			message:     streamMessage{done: true},
			wantMessage: "AI chat stream ended before the turn finished",
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
			model.doneReceived = test.doneReceived
			model.answer.WriteString("partial answer")

			_, _ = model.Update(test.message)
			output := ansi.Strip(model.Transcript())
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

	_, _ = model.Update(streamMessage{done: true, err: errors.New("stream failed")})
	output := model.Transcript()
	if strings.Contains(output, "\x1b[") {
		t.Errorf("scrollback output contains ANSI escape sequence: %q", output)
	}
	if !strings.Contains(output, "# partial answer") || !strings.Contains(output, "stream failed") {
		t.Errorf("scrollback output = %q, want raw Markdown and error", output)
	}
}

func TestTUIModelCachesMarkdownRenderingAcrossViews(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{})
	model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
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
			model.doneReceived = true
			model.answer.WriteString("reply")

			_, _ = model.Update(streamMessage{done: true})
			output := ansi.Strip(model.Transcript())
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
	model.resumedSession = true
	model.sessionTitle = "Old session"
	model.println("old history")
	model.conversation = []transcriptTurn{{user: "old question", assistant: "old answer"}}
	model.composer.SetValue("/new")

	_, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.sessionID != nil {
		t.Errorf("session id = %v, want nil", model.sessionID)
	}
	if got := ansi.Strip(model.Transcript()); strings.Contains(got, "old history") || strings.Contains(got, "Old session") || !strings.Contains(got, "New session") {
		t.Errorf("new-session transcript = %q, want only a fresh header", got)
	}
	if len(model.conversation) != 0 {
		t.Errorf("conversation after /new = %#v, want empty", model.conversation)
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
