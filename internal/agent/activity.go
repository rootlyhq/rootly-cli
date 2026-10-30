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

type completedActivity struct {
	line                        string
	finalizedByText             bool
	finalizedByQualifyingStatus bool
}

type activityTracker struct {
	pending      *pendingActivity
	completed    []completedActivity
	taskStarted  map[string]time.Time
	taskFinished map[string]bool
	now          func() time.Time
}

func (tracker *activityTracker) Handle(event Event) []string {
	tracker.initialize()
	switch event.Type {
	case rootly.AiChatStreamEventStatus:
		if event.Status == nil {
			return nil
		}
		title, startsActivity := statusActivityTitle(event.Status.Status)
		tracker.finishPending(true, false, startsActivity)
		if startsActivity {
			lines := tracker.flushCompleted()
			tracker.pending = &pendingActivity{title: title, startedAt: tracker.now()}
			return lines
		}
	case rootly.AiChatStreamEventText:
		tracker.finishPending(true, true, false)
	case rootly.AiChatStreamEventDone:
		if event.Done != nil && event.Done.Status == "confirmation_required" {
			tracker.dropConfirmation()
			return nil
		}
		tracker.finishPending(event.Done != nil && isSuccessfulDoneStatus(event.Done.Status), false, false)
		return tracker.flushCompleted()
	case rootly.AiChatStreamEventConfirmationRequired:
		tracker.dropConfirmation()
	case rootly.AiChatStreamEventError:
		tracker.finishPending(false, false, false)
		return tracker.flushCompleted()
	case rootly.AiChatStreamEventTaskUpdate:
		return tracker.handleTaskUpdate(event.TaskUpdate)
	}
	return nil
}

func isSuccessfulDoneStatus(status string) bool {
	return status == "complete" || status == "confirmed"
}

func (tracker *activityTracker) Finish(success bool) []string {
	tracker.initialize()
	tracker.finishPending(success, false, false)
	return tracker.flushCompleted()
}

func (tracker *activityTracker) Reset() {
	tracker.pending = nil
	tracker.completed = nil
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

func (tracker *activityTracker) finishPending(success, finalizedByText, finalizedByQualifyingStatus bool) {
	if tracker.pending == nil {
		return
	}
	glyph := "✓"
	if !success {
		glyph = "✗"
	}
	line := fmt.Sprintf("%s %s (%.1fs)", glyph, tracker.pending.title, tracker.now().Sub(tracker.pending.startedAt).Seconds())
	tracker.completed = append(tracker.completed, completedActivity{
		line:                        line,
		finalizedByText:             finalizedByText,
		finalizedByQualifyingStatus: finalizedByQualifyingStatus,
	})
	tracker.pending = nil
}

func (tracker *activityTracker) flushCompleted() []string {
	if len(tracker.completed) == 0 {
		return nil
	}
	lines := make([]string, len(tracker.completed))
	for index, activity := range tracker.completed {
		lines[index] = activity.line
	}
	tracker.completed = nil
	return lines
}

func (tracker *activityTracker) dropConfirmation() {
	tracker.pending = nil
	for len(tracker.completed) > 0 {
		last := tracker.completed[len(tracker.completed)-1]
		if !last.finalizedByText && last.finalizedByQualifyingStatus {
			break
		}
		tracker.completed = tracker.completed[:len(tracker.completed)-1]
	}
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
