package agent

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/google/uuid"
	rootly "github.com/rootlyhq/rootly-go"
)

type blockingStreamer struct {
	started chan struct{}
}

func (s *blockingStreamer) StreamAiChatEvents(ctx context.Context, _ *rootly.StreamAiChatParams) (iter.Seq2[rootly.AiChatStreamEvent, error], error) {
	s.started <- struct{}{}
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestTUIModelAccumulatesTextChunks(t *testing.T) {
	model := NewTUIModel(context.Background(), &fakeStreamer{})
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
	model := NewTUIModel(context.Background(), &fakeStreamer{})
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
	model := NewTUIModel(context.Background(), &fakeStreamer{})
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
	if !strings.Contains(output, "partial") || !strings.Contains(output, "answer") || !strings.Contains(output, "Cancelled.") {
		t.Errorf("scrollback output = %q, want partial answer and cancellation line", output)
	}
}

func TestTUIModelDropsStaleStreamMessages(t *testing.T) {
	streamer := &blockingStreamer{started: make(chan struct{}, 2)}
	model := NewTUIModel(context.Background(), streamer)

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
	model := NewTUIModel(context.Background(), &fakeStreamer{})
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
	model := NewTUIModel(context.Background(), &fakeStreamer{})
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
	model := NewTUIModel(context.Background(), &fakeStreamer{})
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
			model := NewTUIModel(context.Background(), &fakeStreamer{})
			model.state = tuiStreaming
			model.doneStatus = test.doneStatus
			model.answer.WriteString("partial answer")

			_, cmd := model.Update(test.message)
			output := commandOutput(cmd)
			if !strings.Contains(output, "partial") || !strings.Contains(output, "answer") || !strings.Contains(output, test.wantMessage) {
				t.Errorf("scrollback output = %q, want partial answer and %q", output, test.wantMessage)
			}
			if model.state != tuiIdle {
				t.Errorf("state = %v, want idle", model.state)
			}
		})
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
			model := NewTUIModel(context.Background(), &fakeStreamer{})
			model.state = tuiStreaming
			model.doneStatus = test.status
			model.answer.WriteString("reply")

			_, cmd := model.Update(streamMessage{done: true})
			output := commandOutput(cmd)
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
	model := NewTUIModel(context.Background(), &fakeStreamer{})
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
		model := NewTUIModel(context.Background(), &fakeStreamer{})
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
	return fmt.Sprintf("%#v", cmd())
}
