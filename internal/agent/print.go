package agent

import (
	"bufio"
	"bytes"
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
)

type PrintOptions struct {
	Prompt      string
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
	NoColor     bool
	Quiet       bool
	IsStdinTTY  func() bool
	IsStdoutTTY func() bool
	IsStderrTTY func() bool
}

func RunPrint(ctx context.Context, streamer Streamer, options PrintOptions) error {
	prompt, err := readPrompt(options)
	if err != nil {
		return err
	}
	if strings.TrimSpace(prompt) == "" {
		return errors.New("message is required")
	}

	message := prompt
	params := &rootly.StreamAiChatParams{Message: &message}
	var sessionID *uuid.UUID
	var answer strings.Builder

	for {
		resumeTextSeparator := params.CheckpointID != nil
		var confirmation *rootly.AiChatStreamConfirmationRequired
		var doneStatus string
		var streamError error
		err := StreamTurn(ctx, streamer, params, func(event Event) bool {
			switch event.Type {
			case rootly.AiChatStreamEventSessionID:
				if event.SessionID != nil {
					id, parseErr := uuid.Parse(event.SessionID.SessionID)
					if parseErr == nil {
						sessionID = &id
					}
				}
			case rootly.AiChatStreamEventText:
				if event.Text != nil {
					if resumeTextSeparator {
						if answer.Len() > 0 && !strings.HasSuffix(answer.String(), "\n") {
							answer.WriteString("\n\n")
						}
						resumeTextSeparator = false
					}
					answer.WriteString(event.Text.Content)
				}
			case rootly.AiChatStreamEventStatus:
				if event.Status != nil {
					writeProgress(options, event.Status.Status)
				}
			case rootly.AiChatStreamEventTaskUpdate:
				if event.TaskUpdate != nil {
					writeProgress(options, fmt.Sprintf("task: %s (%s)", event.TaskUpdate.Title, event.TaskUpdate.Status))
				}
			case rootly.AiChatStreamEventConfirmationRequired:
				confirmation = event.ConfirmationRequired
			case rootly.AiChatStreamEventError:
				if event.Error != nil {
					streamError = errors.New(event.Error.Message)
					return false
				}
			case rootly.AiChatStreamEventDone:
				if event.Done != nil {
					doneStatus = event.Done.Status
				}
			}
			return true
		})
		if err != nil {
			return err
		}
		if streamError != nil {
			return streamError
		}
		if doneStatus == "error" {
			return errors.New("AI chat turn failed")
		}
		if confirmation != nil || doneStatus == "confirmation_required" {
			if confirmation == nil {
				return errors.New("action requires approval (run \"rootly agent\" interactively)")
			}
			if !terminal(options.IsStdinTTY) || !terminal(options.IsStderrTTY) {
				return fmt.Errorf("action requires approval: %s (run \"rootly agent\" interactively)", confirmation.Question)
			}

			decision, err := askForApproval(options, confirmation)
			if err != nil {
				return err
			}
			checkpointID, err := uuid.Parse(confirmation.CheckpointID)
			if err != nil {
				return fmt.Errorf("invalid confirmation checkpoint: %w", err)
			}
			if sessionID == nil {
				return errors.New("confirmation checkpoint did not include a session id")
			}
			if expired(confirmation) {
				return finishPrintTurn(options, answer.String(), "expired")
			}
			params = &rootly.StreamAiChatParams{
				SessionID:    sessionID,
				CheckpointID: &checkpointID,
				Decision:     &decision,
			}
			continue
		}

		return finishPrintTurn(options, answer.String(), doneStatus)
	}
}

func finishPrintTurn(options PrintOptions, answer, doneStatus string) error {
	if strings.TrimSpace(answer) != "" {
		if err := writeAnswer(options, answer); err != nil {
			return err
		}
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
	if !options.Quiet && terminal(options.IsStderrTTY) {
		_, _ = fmt.Fprintln(options.Stderr, line)
	}
}

func askForApproval(options PrintOptions, confirmation *rootly.AiChatStreamConfirmationRequired) (rootly.StreamAiChatParamsDecision, error) {
	_, _ = fmt.Fprintf(options.Stderr, "%s\n", confirmation.Question)
	if len(confirmation.Preview) > 0 {
		var preview bytes.Buffer
		if err := json.Indent(&preview, confirmation.Preview, "", "  "); err != nil {
			preview.Write(confirmation.Preview)
		}
		_, _ = fmt.Fprintf(options.Stderr, "%s\n", indent(preview.String(), "  "))
	}
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
		rendered, err := renderer.Render(answer)
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
