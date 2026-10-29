package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	rootly "github.com/rootlyhq/rootly-go"
)

func TestActivityTrackerStatusSequence(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tracker := activityTracker{now: func() time.Time { return now }}
	var lines []string

	lines = append(lines, tracker.Handle(Event{
		Type:   rootly.AiChatStreamEventStatus,
		Status: &rootly.AiChatStreamStatus{Status: "Get incidents..."},
	})...)
	now = now.Add(1200 * time.Millisecond)
	lines = append(lines, tracker.Handle(Event{
		Type:   rootly.AiChatStreamEventStatus,
		Status: &rootly.AiChatStreamStatus{Status: "Composing response..."},
	})...)
	lines = append(lines, tracker.Handle(Event{Type: rootly.AiChatStreamEventText})...)
	lines = append(lines, tracker.Handle(Event{
		Type: rootly.AiChatStreamEventDone,
		Done: &rootly.AiChatStreamDone{Status: "complete"},
	})...)

	if len(lines) != 1 || !strings.HasPrefix(lines[0], "✓ Get incidents (") {
		t.Fatalf("activity lines = %#v, want one successful Get incidents line", lines)
	}
}

func TestActivityTrackerCompletesConsecutiveStatusesInOrder(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tracker := activityTracker{now: func() time.Time { return now }}
	var lines []string

	lines = append(lines, tracker.Handle(activityStatusEvent("Get incidents..."))...)
	now = now.Add(time.Second)
	lines = append(lines, tracker.Handle(activityStatusEvent("Search alerts..."))...)
	now = now.Add(time.Second)
	lines = append(lines, tracker.Handle(activityDoneEvent("complete"))...)

	if len(lines) != 2 || !strings.HasPrefix(lines[0], "✓ Get incidents (") || !strings.HasPrefix(lines[1], "✓ Search alerts (") {
		t.Fatalf("activity lines = %#v, want two successful lines in order", lines)
	}
}

func TestActivityTrackerErrorAndConfirmation(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		tracker := activityTracker{}
		tracker.Handle(activityStatusEvent("Get incidents..."))
		lines := tracker.Handle(Event{Type: rootly.AiChatStreamEventError, Error: &rootly.AiChatStreamError{Message: "failed"}})
		if len(lines) != 1 || !strings.HasPrefix(lines[0], "✗ Get incidents (") {
			t.Fatalf("activity lines = %#v, want failed Get incidents line", lines)
		}
	})

	t.Run("confirmation required", func(t *testing.T) {
		tracker := activityTracker{}
		tracker.Handle(activityStatusEvent("Create action item..."))
		lines := tracker.Handle(Event{Type: rootly.AiChatStreamEventConfirmationRequired})
		lines = append(lines, tracker.Handle(activityDoneEvent("confirmation_required"))...)
		if len(lines) != 0 {
			t.Fatalf("activity lines = %#v, want pending activity dropped", lines)
		}
	})
}

func TestActivityTrackerDoesNotDuplicateTaskDuration(t *testing.T) {
	tracker := activityTracker{}
	tracker.Handle(Event{
		Type:       rootly.AiChatStreamEventTaskUpdate,
		TaskUpdate: &rootly.AiChatStreamTaskUpdate{ID: "task-1", Title: "Searched incidents (1.2s)", Status: "in_progress"},
	})
	lines := tracker.Handle(Event{
		Type:       rootly.AiChatStreamEventTaskUpdate,
		TaskUpdate: &rootly.AiChatStreamTaskUpdate{ID: "task-1", Title: "Searched incidents (1.2s)", Status: "complete"},
	})
	if len(lines) != 1 || lines[0] != "✓ Searched incidents (1.2s)" {
		t.Fatalf("activity lines = %#v, want one line without duplicate duration", lines)
	}
}

func TestRunPrintWritesStatusActivityOnlyToTTYTextStderr(t *testing.T) {
	for _, test := range []struct {
		name         string
		format       string
		stderrTTY    bool
		wantActivity bool
	}{
		{name: "TTY text", format: "text", stderrTTY: true, wantActivity: true},
		{name: "piped text", format: "text"},
		{name: "TTY JSON", format: "json", stderrTTY: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			streamer := &fakeStreamer{responses: [][]rootly.AiChatStreamEvent{{
				eventOf(rootly.AiChatStreamEventStatus, `{"status":"Get incidents..."}`),
				eventOf(rootly.AiChatStreamEventStatus, `{"status":"Composing response..."}`),
				eventOf(rootly.AiChatStreamEventText, `{"content":"Done"}`),
				eventOf(rootly.AiChatStreamEventDone, `{"status":"complete"}`),
			}}}
			var stdout, stderr strings.Builder
			options := PrintOptions{
				Prompt:       "list incidents",
				Stdin:        strings.NewReader(""),
				Stdout:       &stdout,
				Stderr:       &stderr,
				OutputFormat: test.format,
				IsStdinTTY:   func() bool { return true },
				IsStderrTTY:  func() bool { return test.stderrTTY },
			}
			if err := RunPrint(context.Background(), streamer, options); err != nil {
				t.Fatalf("RunPrint() error = %v", err)
			}
			gotActivity := strings.Contains(stderr.String(), "✓ Get incidents (")
			if gotActivity != test.wantActivity {
				t.Errorf("stderr = %q, want activity output %v", stderr.String(), test.wantActivity)
			}
			if strings.Contains(stdout.String(), "✓ Get incidents") {
				t.Errorf("stdout = %q, activity must never be printed to stdout", stdout.String())
			}
		})
	}
}

func activityStatusEvent(status string) Event {
	return Event{Type: rootly.AiChatStreamEventStatus, Status: &rootly.AiChatStreamStatus{Status: status}}
}

func activityDoneEvent(status string) Event {
	return Event{Type: rootly.AiChatStreamEventDone, Done: &rootly.AiChatStreamDone{Status: status}}
}
