package agent

import (
	"fmt"
	"strings"
	"time"

	rootly "github.com/rootlyhq/rootly-go"
)

type pendingActivity struct {
	title     string
	startedAt time.Time
}

type activityTracker struct {
	pending      *pendingActivity
	taskStarted  map[string]time.Time
	taskFinished map[string]bool
	now          func() time.Time
}

func (tracker *activityTracker) Handle(event Event) []string {
	tracker.initialize()
	switch event.Type {
	case rootly.AiChatStreamEventStatus:
		lines := tracker.finishPending(true)
		if event.Status != nil {
			if title, ok := statusActivityTitle(event.Status.Status); ok {
				tracker.pending = &pendingActivity{title: title, startedAt: tracker.now()}
			}
		}
		return lines
	case rootly.AiChatStreamEventText:
		return tracker.finishPending(true)
	case rootly.AiChatStreamEventDone:
		if event.Done != nil && event.Done.Status == "confirmation_required" {
			tracker.pending = nil
			return nil
		}
		return tracker.finishPending(event.Done != nil && event.Done.Status == "complete")
	case rootly.AiChatStreamEventConfirmationRequired:
		tracker.pending = nil
	case rootly.AiChatStreamEventError:
		return tracker.finishPending(false)
	case rootly.AiChatStreamEventTaskUpdate:
		return tracker.handleTaskUpdate(event.TaskUpdate)
	}
	return nil
}

func (tracker *activityTracker) Finish(success bool) []string {
	tracker.initialize()
	return tracker.finishPending(success)
}

func (tracker *activityTracker) Reset() {
	tracker.pending = nil
	tracker.taskStarted = make(map[string]time.Time)
	tracker.taskFinished = make(map[string]bool)
}

func (tracker *activityTracker) initialize() {
	if tracker.now == nil {
		tracker.now = time.Now
	}
	if tracker.taskStarted == nil {
		tracker.taskStarted = make(map[string]time.Time)
	}
	if tracker.taskFinished == nil {
		tracker.taskFinished = make(map[string]bool)
	}
}

func (tracker *activityTracker) finishPending(success bool) []string {
	if tracker.pending == nil {
		return nil
	}
	glyph := "✓"
	if !success {
		glyph = "✗"
	}
	line := fmt.Sprintf("%s %s (%.1fs)", glyph, tracker.pending.title, tracker.now().Sub(tracker.pending.startedAt).Seconds())
	tracker.pending = nil
	return []string{line}
}

func (tracker *activityTracker) handleTaskUpdate(task *rootly.AiChatStreamTaskUpdate) []string {
	if task == nil {
		return nil
	}
	if _, exists := tracker.taskStarted[task.ID]; !exists {
		tracker.taskStarted[task.ID] = tracker.now()
	}
	status := strings.ToLower(task.Status)
	if status == "in_progress" || status == "pending" || status == "running" || tracker.taskFinished[task.ID] {
		return nil
	}
	tracker.taskFinished[task.ID] = true
	glyph := "✓"
	if strings.Contains(status, "fail") || strings.Contains(status, "error") || strings.Contains(status, "cancel") {
		glyph = "✗"
	}
	if hasActivityDuration(task.Title) {
		return []string{fmt.Sprintf("%s %s", glyph, task.Title)}
	}
	elapsed := tracker.now().Sub(tracker.taskStarted[task.ID]).Seconds()
	return []string{fmt.Sprintf("%s %s (%.1fs)", glyph, task.Title, elapsed)}
}

func statusActivityTitle(status string) (string, bool) {
	status = strings.TrimSpace(status)
	if !strings.HasSuffix(status, "...") || strings.EqualFold(status, "Composing response...") {
		return "", false
	}
	title := strings.TrimSpace(strings.TrimSuffix(status, "..."))
	return title, title != ""
}

func hasActivityDuration(title string) bool {
	if !strings.HasSuffix(title, ")") {
		return false
	}
	start := strings.LastIndex(title, " (")
	if start < 0 {
		return false
	}
	_, err := time.ParseDuration(title[start+2 : len(title)-1])
	return err == nil
}
