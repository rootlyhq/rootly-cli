package agent

import (
	"bytes"
	"context"
	"encoding/json"
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

func TestRunPrintTextSanitizesAnswerAndStatus(t *testing.T) {
	streamer := &fakeStreamer{responses: [][]rootly.AiChatStreamEvent{{
		eventOf(rootly.AiChatStreamEventStatus, `{"status":"\u001b]52;c;ZXZpbA==\u0007Status"}`),
		eventOf(rootly.AiChatStreamEventText, `{"content":"Answer \u001b]52;c;ZXZpbA==\u0007 \u001b[2J \u001b]8;;https://evil.example\u0007"}`),
		eventOf(rootly.AiChatStreamEventDone, `{"status":"complete"}`),
	}}}
	var stdout, stderr bytes.Buffer
	options := newPrintOptions(&stdout, &stderr)
	options.IsStderrTTY = func() bool { return true }

	if err := RunPrint(context.Background(), streamer, options); err != nil {
		t.Fatalf("RunPrint() error = %v", err)
	}
	for label, output := range map[string]string{"stdout": stdout.String(), "stderr": stderr.String()} {
		for _, unwanted := range []string{"]52;", "[2J", "\x1b]8;;"} {
			if strings.Contains(output, unwanted) {
				t.Errorf("%s = %q, want no %q", label, output, unwanted)
			}
		}
	}
	if !strings.Contains(stdout.String(), "Answer") || !strings.Contains(stderr.String(), "Status") {
		t.Errorf("stdout/stderr = %q / %q, want sanitized answer and status", stdout.String(), stderr.String())
	}
}

func TestRunPrintJSONAndStreamJSON(t *testing.T) {
	for _, outputFormat := range []string{"json", "stream-json"} {
		t.Run(outputFormat, func(t *testing.T) {
			streamer := &fakeStreamer{responses: [][]rootly.AiChatStreamEvent{{
				eventOf(rootly.AiChatStreamEventText, `{"content":"answer"}`),
				eventOf(rootly.AiChatStreamEventDone, `{"status":"complete"}`),
			}}}
			var stdout, stderr bytes.Buffer
			options := newPrintOptions(&stdout, &stderr)
			options.OutputFormat = outputFormat
			if err := RunPrint(context.Background(), streamer, options); err != nil {
				t.Fatalf("RunPrint() error = %v", err)
			}
			lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
			wantLines := 1
			if outputFormat == "stream-json" {
				wantLines = 3
				var event map[string]any
				if err := json.Unmarshal([]byte(lines[0]), &event); err != nil {
					t.Fatalf("event line is invalid JSON: %v", err)
				}
				if event["type"] != string(rootly.AiChatStreamEventText) {
					t.Errorf("event type = %v, want %q", event["type"], rootly.AiChatStreamEventText)
				}
			}
			if len(lines) != wantLines {
				t.Fatalf("stdout lines = %d, want %d: %q", len(lines), wantLines, stdout.String())
			}
			var result map[string]any
			if err := json.Unmarshal([]byte(lines[len(lines)-1]), &result); err != nil {
				t.Fatalf("result line is invalid JSON: %v", err)
			}
			if outputFormat == "stream-json" && result["type"] != "result" {
				t.Errorf("final type = %v, want result", result["type"])
			}
			if result["result"] != "answer" || result["status"] != "complete" {
				t.Errorf("result = %#v, want answer and complete status", result)
			}
			if stderr.Len() != 0 {
				t.Errorf("stderr = %q, want no progress output", stderr.String())
			}
		})
	}
}

func TestRunPrintJSONConfirmationRequiredIsEmittedBeforeError(t *testing.T) {
	streamer := &fakeStreamer{responses: [][]rootly.AiChatStreamEvent{{
		eventOf(rootly.AiChatStreamEventSessionID, `{"session_id":"`+testSessionID+`"}`),
		eventOf(rootly.AiChatStreamEventConfirmationRequired, `{"checkpoint_id":"`+testCheckpointID+`","question":"Approve?","preview":{"title":"Create"},"expires_at":"2099-01-01T00:00:00Z"}`),
		eventOf(rootly.AiChatStreamEventDone, `{"status":"confirmation_required"}`),
	}}}
	var stdout, stderr bytes.Buffer
	options := newPrintOptions(&stdout, &stderr)
	options.OutputFormat = "json"

	if err := RunPrint(context.Background(), streamer, options); err == nil {
		t.Fatal("RunPrint() returned nil, want non-interactive confirmation error")
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("stdout is not a JSON result: %v (%q)", err, stdout.String())
	}
	confirmation, ok := result["confirmation"].(map[string]any)
	if result["status"] != "confirmation_required" || !ok || confirmation["question"] != "Approve?" {
		t.Errorf("result = %#v, want confirmation_required with question", result)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want no JSON-mode progress", stderr.String())
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

func TestRunPrintTreatsMissingDoneAsError(t *testing.T) {
	for _, outputFormat := range []string{"text", "json"} {
		t.Run(outputFormat, func(t *testing.T) {
			streamer := &fakeStreamer{responses: [][]rootly.AiChatStreamEvent{{
				eventOf(rootly.AiChatStreamEventText, `{"content":"Partial answer"}`),
			}}}
			var stdout, stderr bytes.Buffer
			options := newPrintOptions(&stdout, &stderr)
			options.OutputFormat = outputFormat

			err := RunPrint(context.Background(), streamer, options)
			if err == nil || err.Error() != "AI chat stream ended before the turn finished" {
				t.Fatalf("RunPrint() error = %v, want truncated-stream error", err)
			}
			if outputFormat == "json" {
				var result map[string]any
				if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
					t.Fatalf("stdout is not a JSON result: %v (%q)", err, stdout.String())
				}
				if result["status"] != "error" || result["result"] != "Partial answer" {
					t.Errorf("result = %#v, want error status and retained partial answer", result)
				}
			}
		})
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

func TestRunPrintConfirmationSeparatesResumedText(t *testing.T) {
	streamer := &fakeStreamer{responses: [][]rootly.AiChatStreamEvent{
		{
			eventOf(rootly.AiChatStreamEventSessionID, `{"session_id":"`+testSessionID+`"}`),
			eventOf(rootly.AiChatStreamEventText, `{"content":"A"}`),
			eventOf(rootly.AiChatStreamEventConfirmationRequired, `{"checkpoint_id":"`+testCheckpointID+`","question":"Continue?","expires_at":"2099-01-01T00:00:00Z"}`),
			eventOf(rootly.AiChatStreamEventDone, `{"status":"confirmation_required"}`),
		},
		{
			eventOf(rootly.AiChatStreamEventText, `{"content":"B"}`),
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
	if got := stdout.String(); got != "A\n\nB\n" {
		t.Errorf("stdout = %q, want resumed text separated by a blank line", got)
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
	if got := stdout.String(); got != "" {
		t.Errorf("stdout = %q, want no answer for cancellation without replay", got)
	}
	if !strings.Contains(stderr.String(), "Action cancelled.") || strings.Contains(stderr.String(), "turn halted") {
		t.Errorf("stderr = %q, want cancellation note without halted status", stderr.String())
	}
}

func TestRunPrintExpiredConfirmationDoesNotResume(t *testing.T) {
	streamer := &fakeStreamer{responses: [][]rootly.AiChatStreamEvent{{
		eventOf(rootly.AiChatStreamEventSessionID, `{"session_id":"`+testSessionID+`"}`),
		eventOf(rootly.AiChatStreamEventConfirmationRequired, `{"checkpoint_id":"`+testCheckpointID+`","question":"Delete the service?","expires_at":"2000-01-01T00:00:00Z"}`),
		eventOf(rootly.AiChatStreamEventDone, `{"status":"confirmation_required"}`),
	}}}
	var stdout, stderr bytes.Buffer
	options := newPrintOptions(&stdout, &stderr)
	options.Stdin = strings.NewReader("y\n")
	options.IsStdinTTY = func() bool { return true }
	options.IsStderrTTY = func() bool { return true }

	if err := RunPrint(context.Background(), streamer, options); err != nil {
		t.Fatalf("RunPrint() error = %v", err)
	}
	if got := len(streamer.calls); got != 1 {
		t.Errorf("stream calls = %d, want only the original request", got)
	}
	if !strings.Contains(stderr.String(), "Confirmation expired; no action was taken.") {
		t.Errorf("stderr = %q, want expiration message", stderr.String())
	}
}

func TestRunPrintConfirmationResumesWithConfirmedStatus(t *testing.T) {
	streamer := &fakeStreamer{responses: [][]rootly.AiChatStreamEvent{
		{
			eventOf(rootly.AiChatStreamEventSessionID, `{"session_id":"`+testSessionID+`"}`),
			eventOf(rootly.AiChatStreamEventConfirmationRequired, `{"checkpoint_id":"`+testCheckpointID+`","question":"Create the service?","expires_at":"2099-01-01T00:00:00Z"}`),
			eventOf(rootly.AiChatStreamEventDone, `{"status":"confirmation_required"}`),
		},
		{
			eventOf(rootly.AiChatStreamEventText, `{"content":"Created."}`),
			eventOf(rootly.AiChatStreamEventDone, `{"status":"confirmed"}`),
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
		t.Errorf("stdout = %q, want completed answer", got)
	}
	if strings.Contains(stderr.String(), "turn halted") {
		t.Errorf("stderr = %q, confirmed should be treated as complete", stderr.String())
	}
}

func TestRunPrintExpiredStatus(t *testing.T) {
	streamer := &fakeStreamer{responses: [][]rootly.AiChatStreamEvent{{
		eventOf(rootly.AiChatStreamEventDone, `{"status":"expired"}`),
	}}}
	var stdout, stderr bytes.Buffer
	if err := RunPrint(context.Background(), streamer, newPrintOptions(&stdout, &stderr)); err != nil {
		t.Fatalf("RunPrint() error = %v", err)
	}
	if got := stdout.String(); got != "" {
		t.Errorf("stdout = %q, want no answer", got)
	}
	if got := stderr.String(); got != "Confirmation expired; no action was taken.\n" {
		t.Errorf("stderr = %q, want expiration message", got)
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

func TestRunPrintPreservesSkillSlashPrompt(t *testing.T) {
	prompt := "/incident-retro summarize INC-8"
	streamer := &fakeStreamer{responses: [][]rootly.AiChatStreamEvent{{
		eventOf(rootly.AiChatStreamEventDone, `{"status":"complete"}`),
	}}}
	var stdout, stderr bytes.Buffer
	options := newPrintOptions(&stdout, &stderr)
	options.Prompt = prompt
	if err := RunPrint(context.Background(), streamer, options); err != nil {
		t.Fatalf("RunPrint() error = %v", err)
	}
	if len(streamer.calls) != 1 || streamer.calls[0].Message == nil || *streamer.calls[0].Message != prompt {
		t.Errorf("print prompt = %#v, want original prompt %q", streamer.calls, prompt)
	}
}
