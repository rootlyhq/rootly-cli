package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"charm.land/glamour/v2"
	"github.com/google/uuid"
	rootly "github.com/rootlyhq/rootly-go"

	"github.com/rootlyhq/rootly-cli/internal/api"
)

type PrintOptions struct {
	Prompt       string
	Stdin        io.Reader
	Stdout       io.Writer
	Stderr       io.Writer
	NoColor      bool
	Quiet        bool
	OutputFormat string
	SessionID    *uuid.UUID
	Incident     *api.Incident
	Alert        *api.Alert
	Confirmation *rootly.AiChatStreamConfirmationRequired
	StartedAt    time.Time
	IsStdinTTY   func() bool
	IsStdoutTTY  func() bool
	IsStderrTTY  func() bool
}

func RunPrint(ctx context.Context, streamer Streamer, options PrintOptions) error {
	options, prompt, err := preparePrint(options)
	if err != nil {
		return err
	}

	run := printRun{
		streamer:     streamer,
		options:      options,
		params:       initialPrintParams(prompt, options),
		sessionID:    options.SessionID,
		taskStarted:  make(map[string]time.Time),
		taskFinished: make(map[string]bool),
	}
	return run.run(ctx)
}

func preparePrint(options PrintOptions) (PrintOptions, string, error) {
	if options.OutputFormat == "" {
		options.OutputFormat = "text"
	}
	if options.OutputFormat != "text" && options.OutputFormat != "json" && options.OutputFormat != "stream-json" {
		return options, "", fmt.Errorf("invalid output format %q (expected text, json, or stream-json)", options.OutputFormat)
	}
	options.StartedAt = time.Now()
	prompt, err := readPrompt(options)
	if err != nil {
		return options, "", err
	}
	if strings.TrimSpace(prompt) == "" {
		return options, "", errors.New("message is required")
	}
	return options, prompt, nil
}

func initialPrintParams(prompt string, options PrintOptions) *rootly.StreamAiChatParams {
	message := prompt
	params := &rootly.StreamAiChatParams{Message: &message, SessionID: options.SessionID}
	if options.Incident != nil {
		if id, parseErr := uuid.Parse(options.Incident.ID); parseErr == nil {
			params.IncidentID = &id
		}
	}
	if options.Alert != nil {
		if id, parseErr := uuid.Parse(options.Alert.ID); parseErr == nil {
			params.AlertID = &id
		}
	}
	return params
}

type printRun struct {
	streamer     Streamer
	options      PrintOptions
	params       *rootly.StreamAiChatParams
	sessionID    *uuid.UUID
	answer       strings.Builder
	taskStarted  map[string]time.Time
	taskFinished map[string]bool
	confirmation *rootly.AiChatStreamConfirmationRequired
	doneStatus   string
	streamError  error
}

func (r *printRun) run(ctx context.Context) error {
	for {
		continues, err := r.runTurn(ctx)
		if err != nil || !continues {
			return err
		}
	}
}

func (r *printRun) runTurn(ctx context.Context) (bool, error) {
	r.confirmation = nil
	r.doneStatus = ""
	r.streamError = nil
	resumeTextSeparator := r.params.CheckpointID != nil
	err := StreamTurn(ctx, r.streamer, r.params, func(event Event) bool {
		return r.handleEvent(event, &resumeTextSeparator)
	})
	if err != nil {
		return false, finishPrintError(r.options, r.answer.String(), err)
	}
	if r.streamError != nil {
		return false, finishPrintError(r.options, r.answer.String(), r.streamError)
	}
	if r.doneStatus == "error" {
		return false, finishPrintError(r.options, r.answer.String(), errors.New("AI chat turn failed"))
	}
	if r.confirmation != nil || r.doneStatus == "confirmation_required" {
		return r.resumeConfirmation(r.confirmation)
	}
	r.options.SessionID = r.sessionID
	return false, finishPrintTurn(r.options, r.answer.String(), r.doneStatus)
}

func (r *printRun) handleEvent(event Event, resumeTextSeparator *bool) bool {
	if r.options.OutputFormat == "stream-json" {
		writeStreamEvent(r.options.Stdout, event)
	}
	switch event.Type {
	case rootly.AiChatStreamEventSessionID:
		if event.SessionID != nil {
			id, parseErr := uuid.Parse(event.SessionID.SessionID)
			if parseErr == nil {
				r.sessionID = &id
				r.options.SessionID = &id
			}
		}
	case rootly.AiChatStreamEventText:
		if event.Text != nil {
			if *resumeTextSeparator {
				if r.answer.Len() > 0 && !strings.HasSuffix(r.answer.String(), "\n") {
					r.answer.WriteString("\n\n")
				}
				*resumeTextSeparator = false
			}
			r.answer.WriteString(event.Text.Content)
		}
	case rootly.AiChatStreamEventStatus:
		if event.Status != nil {
			writeProgress(r.options, event.Status.Status)
		}
	case rootly.AiChatStreamEventTaskUpdate:
		if event.TaskUpdate != nil {
			r.writeTaskUpdate(event.TaskUpdate)
		}
	case rootly.AiChatStreamEventSkillLoaded:
		if event.SkillLoaded != nil {
			writeProgress(r.options, faint(r.options.NoColor, "• Loaded skill "+event.SkillLoaded.Skill.Name))
		}
	case rootly.AiChatStreamEventConfirmationRequired:
		r.confirmation = event.ConfirmationRequired
	case rootly.AiChatStreamEventError:
		if event.Error != nil {
			r.streamError = errors.New(event.Error.Message)
			return false
		}
	case rootly.AiChatStreamEventDone:
		if event.Done != nil {
			r.doneStatus = event.Done.Status
		}
	}
	return true
}

func (r *printRun) writeTaskUpdate(task *rootly.AiChatStreamTaskUpdate) {
	writeProgress(r.options, fmt.Sprintf("task: %s (%s)", task.Title, task.Status))
	if _, exists := r.taskStarted[task.ID]; !exists {
		r.taskStarted[task.ID] = time.Now()
	}
	status := strings.ToLower(task.Status)
	if status == "in_progress" || status == "pending" || status == "running" || r.taskFinished[task.ID] {
		return
	}
	glyph := "✓"
	if strings.Contains(status, "fail") || strings.Contains(status, "error") || strings.Contains(status, "cancel") {
		glyph = "✗"
	}
	line := fmt.Sprintf("%s %s (%.1fs)", glyph, task.Title, time.Since(r.taskStarted[task.ID]).Seconds())
	writeProgress(r.options, faint(r.options.NoColor, line))
	r.taskFinished[task.ID] = true
}

func (r *printRun) resumeConfirmation(confirmation *rootly.AiChatStreamConfirmationRequired) (bool, error) {
	if confirmation == nil {
		err := errors.New("action requires approval (run \"rootly agent\" interactively)")
		return false, finishPrintError(r.options, r.answer.String(), err)
	}
	if !terminal(r.options.IsStdinTTY) || !terminal(r.options.IsStderrTTY) {
		if r.options.OutputFormat != "text" {
			r.options.Confirmation = confirmation
			_ = finishPrintTurn(r.options, r.answer.String(), "confirmation_required")
		}
		return false, fmt.Errorf("action requires approval: %s (run \"rootly agent\" interactively)", confirmation.Question)
	}
	decision, err := askForApproval(r.options, confirmation)
	if err != nil {
		return false, err
	}
	checkpointID, err := uuid.Parse(confirmation.CheckpointID)
	if err != nil {
		return false, fmt.Errorf("invalid confirmation checkpoint: %w", err)
	}
	if r.sessionID == nil {
		return false, errors.New("confirmation checkpoint did not include a session id")
	}
	r.options.SessionID = r.sessionID
	if expired(confirmation) {
		return false, finishPrintTurn(r.options, r.answer.String(), "expired")
	}
	r.params = &rootly.StreamAiChatParams{
		SessionID:    r.sessionID,
		CheckpointID: &checkpointID,
		Decision:     &decision,
	}
	return true, nil
}

func finishPrintError(options PrintOptions, answer string, err error) error {
	if options.OutputFormat != "text" {
		if outputErr := finishPrintTurn(options, answer, "error"); outputErr != nil {
			return outputErr
		}
	}
	return err
}

func finishPrintTurn(options PrintOptions, answer, doneStatus string) error {
	if options.OutputFormat == "json" || options.OutputFormat == "stream-json" {
		if doneStatus == "" {
			doneStatus = "complete"
		}
		session := ""
		if options.SessionID != nil {
			session = options.SessionID.String()
		}
		result := map[string]any{
			"session_id":  session,
			"result":      answer,
			"status":      doneStatus,
			"duration_ms": time.Since(options.StartedAt).Milliseconds(),
		}
		if options.OutputFormat == "stream-json" {
			result["type"] = "result"
		}
		if options.Confirmation != nil {
			result["confirmation"] = map[string]any{
				"question":      options.Confirmation.Question,
				"checkpoint_id": options.Confirmation.CheckpointID,
				"preview":       options.Confirmation.Preview,
				"expires_at":    options.Confirmation.ExpiresAt,
			}
		}
		encoder := json.NewEncoder(options.Stdout)
		return encoder.Encode(result)
	}
	if strings.TrimSpace(answer) != "" {
		if err := writeAnswer(options, answer); err != nil {
			return err
		}
	}
	if options.SessionID != nil && terminal(options.IsStderrTTY) {
		_, _ = fmt.Fprintf(options.Stderr, "Resume with: rootly agent --resume %s\n", options.SessionID.String())
	}
	switch doneStatus {
	case "cancelled":
		_, err := fmt.Fprintln(options.Stderr, "Action cancelled.")
		return err
	case "expired":
		_, err := fmt.Fprintln(options.Stderr, "Confirmation expired; no action was taken.")
		return err
	}
	if doneStatus != "" && doneStatus != "complete" && doneStatus != "confirmed" {
		_, err := fmt.Fprintf(options.Stderr, "turn halted: %s\n", doneStatus)
		return err
	}
	return nil
}

func readPrompt(options PrintOptions) (string, error) {
	prompt := options.Prompt
	if options.Stdin == nil || terminal(options.IsStdinTTY) {
		return prompt, nil
	}
	input, err := io.ReadAll(options.Stdin)
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	stdinText := strings.Trim(string(input), "\r\n")
	if stdinText == "" {
		return prompt, nil
	}
	if prompt == "" {
		return stdinText, nil
	}
	return strings.TrimRight(prompt, "\r\n") + "\n\n" + stdinText, nil
}

func writeProgress(options PrintOptions, line string) {
	if options.OutputFormat == "text" && !options.Quiet && terminal(options.IsStderrTTY) {
		_, _ = fmt.Fprintln(options.Stderr, line)
	}
}

func writeStreamEvent(writer io.Writer, event Event) {
	payload := any(nil)
	switch event.Type {
	case rootly.AiChatStreamEventSessionID:
		payload = event.SessionID
	case rootly.AiChatStreamEventText:
		payload = event.Text
	case rootly.AiChatStreamEventStatus:
		payload = event.Status
	case rootly.AiChatStreamEventTaskUpdate:
		payload = event.TaskUpdate
	case rootly.AiChatStreamEventSkillLoaded:
		payload = event.SkillLoaded
	case rootly.AiChatStreamEventConfirmationRequired:
		payload = event.ConfirmationRequired
	case rootly.AiChatStreamEventCheckpointResolved:
		payload = event.CheckpointResolved
	case rootly.AiChatStreamEventError:
		payload = event.Error
	case rootly.AiChatStreamEventDone:
		payload = event.Done
	}
	fields := map[string]any{"type": event.Type}
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err == nil {
			_ = json.Unmarshal(encoded, &fields)
			fields["type"] = event.Type
		}
	}
	_ = json.NewEncoder(writer).Encode(fields)
}

func askForApproval(options PrintOptions, confirmation *rootly.AiChatStreamConfirmationRequired) (rootly.StreamAiChatParamsDecision, error) {
	_, _ = fmt.Fprintf(options.Stderr, "%s\n", renderConfirmationCard(confirmation, 80, options.NoColor, time.Now()))
	_, _ = fmt.Fprint(options.Stderr, "Approve? [y/N] ")
	line, err := bufio.NewReader(options.Stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read approval: %w", err)
	}
	if strings.EqualFold(strings.TrimSpace(line), "y") {
		return rootly.StreamAiChatParamsDecisionConfirm, nil
	}
	return rootly.StreamAiChatParamsDecisionCancel, nil
}

func writeAnswer(options PrintOptions, answer string) error {
	if terminal(options.IsStdoutTTY) && !options.NoColor {
		renderer, err := glamour.NewTermRenderer(glamour.WithEnvironmentConfig(), glamour.WithWordWrap(80))
		if err != nil {
			return err
		}
		rendered, err := renderer.Render(terminalMarkdown(answer, false))
		if err != nil {
			return err
		}
		answer = strings.TrimSpace(rendered)
	}
	_, err := fmt.Fprintln(options.Stdout, answer)
	return err
}

func terminal(check func() bool) bool {
	return check != nil && check()
}

func indent(value, prefix string) string {
	lines := strings.Split(value, "\n")
	for index, line := range lines {
		lines[index] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func expired(confirmation *rootly.AiChatStreamConfirmationRequired) bool {
	if confirmation.ExpiresAt == "" {
		return false
	}
	expiresAt, err := time.Parse(time.RFC3339, confirmation.ExpiresAt)
	return err == nil && time.Now().After(expiresAt)
}
