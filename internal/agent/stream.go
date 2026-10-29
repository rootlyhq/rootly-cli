package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"strings"

	rootly "github.com/rootlyhq/rootly-go"
)

type Streamer interface {
	StreamAiChatEvents(context.Context, *rootly.StreamAiChatParams) (iter.Seq2[rootly.AiChatStreamEvent, error], error)
}

type Event struct {
	Type                 string
	SessionID            *rootly.AiChatStreamSessionID
	Text                 *rootly.AiChatStreamText
	Status               *rootly.AiChatStreamStatus
	TaskUpdate           *rootly.AiChatStreamTaskUpdate
	SkillLoaded          *rootly.AiChatStreamSkillLoaded
	ConfirmationRequired *rootly.AiChatStreamConfirmationRequired
	CheckpointResolved   *rootly.AiChatStreamCheckpointResolved
	Error                *rootly.AiChatStreamError
	Done                 *rootly.AiChatStreamDone
}

func StreamTurn(ctx context.Context, streamer Streamer, params *rootly.StreamAiChatParams, yield func(Event) bool) error {
	events, err := streamer.StreamAiChatEvents(ctx, params)
	if err != nil {
		return describeError(err)
	}

	for event, err := range events {
		if err != nil {
			return describeError(err)
		}
		decoded, known, err := decodeEvent(event)
		if err != nil {
			return err
		}
		if known && !yield(decoded) {
			return nil
		}
	}

	return nil
}

func decodeEvent(event rootly.AiChatStreamEvent) (Event, bool, error) {
	decoded := Event{Type: event.Type}
	var payload any
	switch event.Type {
	case rootly.AiChatStreamEventSessionID:
		payload = &rootly.AiChatStreamSessionID{}
	case rootly.AiChatStreamEventText:
		payload = &rootly.AiChatStreamText{}
	case rootly.AiChatStreamEventStatus:
		payload = &rootly.AiChatStreamStatus{}
	case rootly.AiChatStreamEventTaskUpdate:
		payload = &rootly.AiChatStreamTaskUpdate{}
	case rootly.AiChatStreamEventSkillLoaded:
		payload = &rootly.AiChatStreamSkillLoaded{}
	case rootly.AiChatStreamEventConfirmationRequired:
		payload = &rootly.AiChatStreamConfirmationRequired{}
	case rootly.AiChatStreamEventCheckpointResolved:
		payload = &rootly.AiChatStreamCheckpointResolved{}
	case rootly.AiChatStreamEventError:
		payload = &rootly.AiChatStreamError{}
	case rootly.AiChatStreamEventDone:
		payload = &rootly.AiChatStreamDone{}
	default:
		return Event{}, false, nil
	}

	if err := event.Decode(payload); err != nil {
		return Event{}, false, fmt.Errorf("decode %s event: %w", event.Type, err)
	}

	switch value := payload.(type) {
	case *rootly.AiChatStreamSessionID:
		decoded.SessionID = value
	case *rootly.AiChatStreamText:
		decoded.Text = value
	case *rootly.AiChatStreamStatus:
		decoded.Status = value
	case *rootly.AiChatStreamTaskUpdate:
		decoded.TaskUpdate = value
	case *rootly.AiChatStreamSkillLoaded:
		decoded.SkillLoaded = value
	case *rootly.AiChatStreamConfirmationRequired:
		decoded.ConfirmationRequired = value
	case *rootly.AiChatStreamCheckpointResolved:
		decoded.CheckpointResolved = value
	case *rootly.AiChatStreamError:
		decoded.Error = value
	case *rootly.AiChatStreamDone:
		decoded.Done = value
	}

	return decoded, true, nil
}

func describeError(err error) error {
	const prefix = "unexpected status "
	message := err.Error()
	if !strings.HasPrefix(message, prefix) {
		return err
	}

	statusAndBody := strings.TrimPrefix(message, prefix)
	statusText, body, hasBody := strings.Cut(statusAndBody, ": ")
	var status int
	if _, scanErr := fmt.Sscanf(statusText, "%d", &status); scanErr != nil {
		return err
	}

	title := ""
	if hasBody {
		var response struct {
			Errors []struct {
				Title string `json:"title"`
			} `json:"errors"`
		}
		if json.Unmarshal([]byte(body), &response) == nil && len(response.Errors) > 0 {
			title = response.Errors[0].Title
		}
	}

	switch status {
	case 401:
		return fmt.Errorf("not authenticated: run `rootly login` or set ROOTLY_API_KEY")
	case 403:
		if strings.Contains(strings.ToLower(title+" "+body), "ai.chat:write") {
			return fmt.Errorf("OAuth token requires ai.chat:write scope for this action. Run `rootly logout && rootly login` to refresh your scopes")
		}
	case 429:
		return fmt.Errorf("rate limit exceeded (60/min); please try again shortly")
	}

	if title != "" {
		return fmt.Errorf("%s", title)
	}
	if hasBody && body != "" {
		return fmt.Errorf("request failed with status %d: %s", status, body)
	}
	return fmt.Errorf("request failed with status %d", status)
}
