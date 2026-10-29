package agent

import (
	"bytes"
	"context"
	"strings"
	"testing"

	rootly "github.com/rootlyhq/rootly-go"
)

const testSessionID = "39fd7596-152c-43d3-afce-48b792ae386d"
const testCheckpointID = "8a348996-dfea-4c76-8c50-cfd0f31f8e5f"

func TestRunPrintJoinsTextToStdout(t *testing.T) {
	streamer := &fakeStreamer{responses: [][]rootly.AiChatStreamEvent{{
		eventOf(rootly.AiChatStreamEventText, `{"content":"Hello "}`),
		eventOf(rootly.AiChatStreamEventText, `{"content":"there."}`),
		eventOf(rootly.AiChatStreamEventDone, `{"status":"complete"}`),
	}}}
	var stdout, stderr bytes.Buffer
	options := newPrintOptions(&stdout, &stderr)

	if err := RunPrint(context.Background(), streamer, options); err != nil {
		t.Fatalf("RunPrint() error = %v", err)
	}
	if got := stdout.String(); got != "Hello there.\n" {
		t.Errorf("stdout = %q, want joined response", got)
	}
}

func TestRunPrintReturnsErrorEvent(t *testing.T) {
	streamer := &fakeStreamer{responses: [][]rootly.AiChatStreamEvent{{
		eventOf(rootly.AiChatStreamEventError, `{"message":"the turn failed"}`),
		eventOf(rootly.AiChatStreamEventDone, `{"status":"error"}`),
	}}}
	var stdout, stderr bytes.Buffer
	err := RunPrint(context.Background(), streamer, newPrintOptions(&stdout, &stderr))
	if err == nil || err.Error() != "the turn failed" {
		t.Fatalf("RunPrint() error = %v, want the event error", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want no final answer", stdout.String())
	}
}

func TestRunPrintHaltedStatusPrintsAnswerAndNote(t *testing.T) {
	streamer := &fakeStreamer{responses: [][]rootly.AiChatStreamEvent{{
		eventOf(rootly.AiChatStreamEventText, `{"content":"Partial answer"}`),
		eventOf(rootly.AiChatStreamEventDone, `{"status":"interrupted"}`),
	}}}
	var stdout, stderr bytes.Buffer
	if err := RunPrint(context.Background(), streamer, newPrintOptions(&stdout, &stderr)); err != nil {
		t.Fatalf("RunPrint() error = %v", err)
	}
	if got := stdout.String(); got != "Partial answer\n" {
		t.Errorf("stdout = %q, want partial answer", got)
	}
	if got := stderr.String(); got != "turn halted: interrupted\n" {
		t.Errorf("stderr = %q, want halt note", got)
	}
}

func TestRunPrintConfirmationResumesWithConfirm(t *testing.T) {
	streamer := &fakeStreamer{responses: [][]rootly.AiChatStreamEvent{
		{
			eventOf(rootly.AiChatStreamEventSessionID, `{"session_id":"`+testSessionID+`"}`),
			eventOf(rootly.AiChatStreamEventConfirmationRequired, `{"checkpoint_id":"`+testCheckpointID+`","question":"Create the service?","preview":{"name":"api"},"expires_at":"2099-01-01T00:00:00Z"}`),
			eventOf(rootly.AiChatStreamEventDone, `{"status":"confirmation_required"}`),
		},
		{
			eventOf(rootly.AiChatStreamEventText, `{"content":"Created."}`),
			eventOf(rootly.AiChatStreamEventDone, `{"status":"complete"}`),
		},
	}}
	var stdout, stderr bytes.Buffer
	options := newPrintOptions(&stdout, &stderr)
	options.Stdin = strings.NewReader("y\n")
	options.IsStdinTTY = func() bool { return true }
	options.IsStderrTTY = func() bool { return true }

	if err := RunPrint(context.Background(), streamer, options); err != nil {
		t.Fatalf("RunPrint() error = %v", err)
	}
	if got := stdout.String(); got != "Created.\n" {
		t.Errorf("stdout = %q, want replayed response", got)
	}
	if len(streamer.calls) != 2 {
		t.Fatalf("stream calls = %d, want 2", len(streamer.calls))
	}
	resume := streamer.calls[1]
	if resume.Message != nil {
		t.Errorf("resume message = %v, want nil", resume.Message)
	}
	if resume.SessionID == nil || resume.SessionID.String() != testSessionID {
		t.Errorf("resume session = %v, want %s", resume.SessionID, testSessionID)
	}
	if resume.CheckpointID == nil || resume.CheckpointID.String() != testCheckpointID {
		t.Errorf("resume checkpoint = %v, want %s", resume.CheckpointID, testCheckpointID)
	}
	if resume.Decision == nil || *resume.Decision != rootly.StreamAiChatParamsDecisionConfirm {
		t.Errorf("resume decision = %v, want confirm", resume.Decision)
	}
	if !strings.Contains(stderr.String(), "Approve? [y/N]") || !strings.Contains(stderr.String(), `"name": "api"`) {
		t.Errorf("stderr = %q, want question prompt and indented preview", stderr.String())
	}
}

func TestRunPrintConfirmationResumesWithCancel(t *testing.T) {
	streamer := &fakeStreamer{responses: [][]rootly.AiChatStreamEvent{
		{
			eventOf(rootly.AiChatStreamEventSessionID, `{"session_id":"`+testSessionID+`"}`),
			eventOf(rootly.AiChatStreamEventConfirmationRequired, `{"checkpoint_id":"`+testCheckpointID+`","question":"Delete the service?","expires_at":"2099-01-01T00:00:00Z"}`),
			eventOf(rootly.AiChatStreamEventDone, `{"status":"confirmation_required"}`),
		},
		{eventOf(rootly.AiChatStreamEventDone, `{"status":"cancelled"}`)},
	}}
	var stdout, stderr bytes.Buffer
	options := newPrintOptions(&stdout, &stderr)
	options.Stdin = strings.NewReader("n\n")
	options.IsStdinTTY = func() bool { return true }
	options.IsStderrTTY = func() bool { return true }

	if err := RunPrint(context.Background(), streamer, options); err != nil {
		t.Fatalf("RunPrint() error = %v", err)
	}
	if got := streamer.calls[1].Decision; got == nil || *got != rootly.StreamAiChatParamsDecisionCancel {
		t.Errorf("resume decision = %v, want cancel", got)
	}
}

func TestRunPrintNonInteractiveConfirmationReturnsQuestion(t *testing.T) {
	streamer := &fakeStreamer{responses: [][]rootly.AiChatStreamEvent{{
		eventOf(rootly.AiChatStreamEventConfirmationRequired, `{"checkpoint_id":"`+testCheckpointID+`","question":"Approve sending the message?","expires_at":"2099-01-01T00:00:00Z"}`),
		eventOf(rootly.AiChatStreamEventDone, `{"status":"confirmation_required"}`),
	}}}
	var stdout, stderr bytes.Buffer
	err := RunPrint(context.Background(), streamer, newPrintOptions(&stdout, &stderr))
	if err == nil || !strings.Contains(err.Error(), "Approve sending the message?") {
		t.Fatalf("RunPrint() error = %v, want question in error", err)
	}
}

func TestRunPrintAppendsStdinToPrompt(t *testing.T) {
	streamer := &fakeStreamer{responses: [][]rootly.AiChatStreamEvent{{
		eventOf(rootly.AiChatStreamEventDone, `{"status":"complete"}`),
	}}}
	var stdout, stderr bytes.Buffer
	options := newPrintOptions(&stdout, &stderr)
	options.Prompt = "Explain this"
	options.Stdin = strings.NewReader("log contents")
	options.IsStdinTTY = func() bool { return false }

	if err := RunPrint(context.Background(), streamer, options); err != nil {
		t.Fatalf("RunPrint() error = %v", err)
	}
	if got, want := *streamer.calls[0].Message, "Explain this\n\nlog contents"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func newPrintOptions(stdout, stderr *bytes.Buffer) PrintOptions {
	return PrintOptions{
		Prompt:      "test prompt",
		Stdin:       strings.NewReader(""),
		Stdout:      stdout,
		Stderr:      stderr,
		IsStdinTTY:  func() bool { return true },
		IsStdoutTTY: func() bool { return false },
		IsStderrTTY: func() bool { return false },
	}
}
