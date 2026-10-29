package agent

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/google/uuid"
	rootly "github.com/rootlyhq/rootly-go"
)

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
	called := false
	model.cancel = func() { called = true }

	_, _ = model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !called {
		t.Error("Ctrl+C did not call the stream cancel function")
	}
	if model.state != tuiIdle || model.status != "cancelled" {
		t.Errorf("state/status = %v/%q, want idle/cancelled", model.state, model.status)
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
