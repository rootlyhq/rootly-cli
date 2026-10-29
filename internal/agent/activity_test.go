package agent

import (
	"bytes"
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

func TestActivityTrackerKeepsRepeatedTitlesAsSeparateActivities(t *testing.T) {
	tracker := activityTracker{}
	var lines []string

	lines = append(lines, tracker.Handle(activityStatusEvent("Get incidents..."))...)
	lines = append(lines, tracker.Handle(activityStatusEvent("Get incidents..."))...)
	lines = append(lines, tracker.Handle(activityDoneEvent("complete"))...)

	if len(lines) != 2 || !strings.HasPrefix(lines[0], "✓ Get incidents (") || !strings.HasPrefix(lines[1], "✓ Get incidents (") {
		t.Fatalf("activity lines = %#v, want two separate successful activities", lines)
	}
}

func TestActivityTrackerConfirmationDropsTextFinalization(t *testing.T) {
	tracker := activityTracker{}
	var beforeResume []string

	beforeResume = append(beforeResume, tracker.Handle(activityStatusEvent("A..."))...)
	beforeResume = append(beforeResume, tracker.Handle(Event{Type: rootly.AiChatStreamEventText})...)
	beforeResume = append(beforeResume, tracker.Handle(Event{Type: rootly.AiChatStreamEventConfirmationRequired})...)
	beforeResume = append(beforeResume, tracker.Handle(activityDoneEvent("confirmation_required"))...)
	if len(beforeResume) != 0 {
		t.Fatalf("activity lines before resume = %#v, want none", beforeResume)
	}

	var afterResume []string
	afterResume = append(afterResume, tracker.Handle(activityStatusEvent("A..."))...)
	afterResume = append(afterResume, tracker.Handle(Event{Type: rootly.AiChatStreamEventText})...)
	afterResume = append(afterResume, tracker.Handle(activityDoneEvent("complete"))...)
	if len(afterResume) != 1 || !strings.HasPrefix(afterResume[0], "✓ A (") {
		t.Fatalf("activity lines after resume = %#v, want one successful A line", afterResume)
	}
}

func TestActivityTrackerFlushesCompletedActivityBeforeConfirmation(t *testing.T) {
	tracker := activityTracker{}
	tracker.Handle(activityStatusEvent("A..."))

	flushed := tracker.Handle(activityStatusEvent("B..."))
	if len(flushed) != 1 || !strings.HasPrefix(flushed[0], "✓ A (") {
		t.Fatalf("activity lines on B status = %#v, want one successful A line", flushed)
	}

	var afterConfirmation []string
	afterConfirmation = append(afterConfirmation, tracker.Handle(Event{Type: rootly.AiChatStreamEventText})...)
	afterConfirmation = append(afterConfirmation, tracker.Handle(Event{Type: rootly.AiChatStreamEventConfirmationRequired})...)
	afterConfirmation = append(afterConfirmation, tracker.Handle(activityDoneEvent("confirmation_required"))...)
	if len(afterConfirmation) != 0 {
		t.Fatalf("activity lines for B = %#v, want none", afterConfirmation)
	}
}

func TestActivityTrackerFinishFlushesStatusFinalization(t *testing.T) {
	tracker := activityTracker{}
	tracker.Handle(activityStatusEvent("A..."))
	tracker.Handle(activityStatusEvent("Composing response..."))

	lines := tracker.Finish(true)
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "✓ A (") {
		t.Fatalf("activity lines on Finish() = %#v, want retained successful A line", lines)
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

func TestRunPrintDefersConfirmationActivityUntilApproval(t *testing.T) {
	streamer := &fakeStreamer{responses: [][]rootly.AiChatStreamEvent{
		{
			eventOf(rootly.AiChatStreamEventSessionID, `{"session_id":"`+testSessionID+`"}`),
			eventOf(rootly.AiChatStreamEventStatus, `{"status":"Create action item..."}`),
			eventOf(rootly.AiChatStreamEventStatus, `{"status":"Composing response..."}`),
			eventOf(rootly.AiChatStreamEventText, `{"content":"Ready for approval."}`),
			eventOf(rootly.AiChatStreamEventConfirmationRequired, `{"checkpoint_id":"`+testCheckpointID+`","question":"Create the action item?","preview":{"title":"Create action item"},"expires_at":"2099-01-01T00:00:00Z"}`),
			eventOf(rootly.AiChatStreamEventDone, `{"status":"confirmation_required"}`),
		},
		{
			eventOf(rootly.AiChatStreamEventStatus, `{"status":"Create action item..."}`),
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
	activity := "✓ Create action item ("
	if got := strings.Count(stderr.String(), activity); got != 1 {
		t.Fatalf("activity output count = %d, want one: %q", got, stderr.String())
	}
	if strings.Index(stderr.String(), activity) < strings.Index(stderr.String(), "Approve? [y/N]") {
		t.Errorf("activity was printed before approval: %q", stderr.String())
	}
}

func TestRunPrintFlushesStatusFinalizationAtConfirmation(t *testing.T) {
	streamer := &fakeStreamer{responses: [][]rootly.AiChatStreamEvent{
		{
			eventOf(rootly.AiChatStreamEventSessionID, `{"session_id":"`+testSessionID+`"}`),
			eventOf(rootly.AiChatStreamEventStatus, `{"status":"Create action item..."}`),
			eventOf(rootly.AiChatStreamEventStatus, `{"status":"Save incident..."}`),
			eventOf(rootly.AiChatStreamEventConfirmationRequired, `{"checkpoint_id":"`+testCheckpointID+`","question":"Create the action item?","preview":{"title":"Create action item"},"expires_at":"2099-01-01T00:00:00Z"}`),
			eventOf(rootly.AiChatStreamEventDone, `{"status":"confirmation_required"}`),
		},
		{
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
	activity := "✓ Create action item ("
	activityIndex := strings.Index(stderr.String(), activity)
	promptIndex := strings.Index(stderr.String(), "Approve? [y/N]")
	if activityIndex < 0 || promptIndex < 0 || activityIndex > promptIndex {
		t.Errorf("stderr = %q, want completed tool activity before the approval prompt", stderr.String())
	}
	if strings.Contains(stderr.String(), "✓ Save incident (") {
		t.Errorf("stderr = %q, want pending action activity dropped", stderr.String())
	}
}

func activityStatusEvent(status string) Event {
	return Event{Type: rootly.AiChatStreamEventStatus, Status: &rootly.AiChatStreamStatus{Status: status}}
}

func activityDoneEvent(status string) Event {
	return Event{Type: rootly.AiChatStreamEventDone, Done: &rootly.AiChatStreamDone{Status: status}}
}
