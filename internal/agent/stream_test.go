package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"strings"
	"testing"

	rootly "github.com/rootlyhq/rootly-go"
)

type fakeStreamer struct {
	responses [][]rootly.AiChatStreamEvent
	calls     []*rootly.StreamAiChatParams
}

func (f *fakeStreamer) StreamAiChatEvents(_ context.Context, params *rootly.StreamAiChatParams) (iter.Seq2[rootly.AiChatStreamEvent, error], error) {
	f.calls = append(f.calls, params)
	if len(f.responses) == 0 {
		return nil, errors.New("no fake stream response")
	}
	response := f.responses[0]
	f.responses = f.responses[1:]
	return func(yield func(rootly.AiChatStreamEvent, error) bool) {
		for _, event := range response {
			if !yield(event, nil) {
				return
			}
		}
	}, nil
}

func eventOf(eventType, data string) rootly.AiChatStreamEvent {
	return rootly.AiChatStreamEvent{Type: eventType, Data: json.RawMessage(data)}
}

func TestStreamTurnDecodesKnownEventsAndIgnoresUnknown(t *testing.T) {
	streamer := &fakeStreamer{responses: [][]rootly.AiChatStreamEvent{{
		eventOf(rootly.AiChatStreamEventText, `{"content":"hello"}`),
		eventOf("future_event", `{"anything":true}`),
		eventOf(rootly.AiChatStreamEventDone, `{"status":"complete"}`),
	}}}
	var received []Event
	if err := StreamTurn(context.Background(), streamer, &rootly.StreamAiChatParams{}, func(event Event) bool {
		received = append(received, event)
		return true
	}); err != nil {
		t.Fatalf("StreamTurn() error = %v", err)
	}
	if len(received) != 2 {
		t.Fatalf("received %d events, want 2", len(received))
	}
	if received[0].Text == nil || received[0].Text.Content != "hello" {
		t.Errorf("text event = %#v, want decoded content", received[0])
	}
	if received[1].Done == nil || received[1].Done.Status != "complete" {
		t.Errorf("done event = %#v, want decoded status", received[1])
	}
}

func TestDescribeError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "unauthorized",
			err:  fmt.Errorf(`unexpected status 401 Unauthorized: {"errors":[{"title":"Invalid token"}]}`),
			want: "not authenticated: run `rootly login` or set ROOTLY_API_KEY",
		},
		{
			name: "oauth ai chat scope",
			err:  fmt.Errorf(`unexpected status 403 Forbidden: {"errors":[{"title":"OAuth token requires ai.chat:write scope for this action"}]}`),
			want: "OAuth token requires ai.chat:write scope for this action. Run `rootly logout && rootly login` to refresh your scopes",
		},
		{
			name: "rate limit",
			err:  fmt.Errorf("unexpected status 429 Too Many Requests: rate limited"),
			want: "rate limit exceeded (60/min); please try again shortly",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := describeError(tt.err).Error(); got != tt.want {
				t.Errorf("describeError() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStreamTurnDescribesRequestErrors(t *testing.T) {
	streamer := streamError{err: fmt.Errorf("unexpected status 401 Unauthorized: auth failed")}
	err := StreamTurn(context.Background(), streamer, &rootly.StreamAiChatParams{}, func(Event) bool {
		return true
	})
	if err == nil || !strings.Contains(err.Error(), "not authenticated") {
		t.Fatalf("StreamTurn() error = %v, want authentication guidance", err)
	}
}

type streamError struct {
	err error
}

func (s streamError) StreamAiChatEvents(context.Context, *rootly.StreamAiChatParams) (iter.Seq2[rootly.AiChatStreamEvent, error], error) {
	return nil, s.err
}
