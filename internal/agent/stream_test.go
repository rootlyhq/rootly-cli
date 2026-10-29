package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"strings"
	"testing"
	"time"

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
			err:  fmt.Errorf("wrapped: %w", newStreamStatusError(http.StatusUnauthorized, `{"errors":[{"title":"Invalid token"}]}`, "")),
			want: "not authenticated: run `rootly login` or set ROOTLY_API_KEY",
		},
		{
			name: "oauth ai chat scope",
			err:  newStreamStatusError(http.StatusForbidden, `{"errors":[{"title":"OAuth token requires ai.chat:write scope for this action"}]}`, ""),
			want: "OAuth token requires ai.chat:write scope for this action. Run `rootly logout && rootly login` to refresh your scopes",
		},
		{
			name: "rate limit with retry after",
			err:  newStreamStatusError(http.StatusTooManyRequests, "rate limited", "17"),
			want: "rate limited by Rootly AI: retry in 17s",
		},
		{
			name: "rate limit retry after minimum",
			err:  newStreamStatusError(http.StatusTooManyRequests, "rate limited", "0"),
			want: "rate limited by Rootly AI: retry in 1s",
		},
		{
			name: "rate limit without retry after",
			err:  newStreamStatusError(http.StatusTooManyRequests, "rate limited", ""),
			want: "rate limit exceeded (60/min); please try again shortly",
		},
		{
			name: "internal server title",
			err:  newStreamStatusError(http.StatusInternalServerError, `{"errors":[{"title":"Internal server error"}]}`, ""),
			want: "Internal server error",
		},
		{
			name: "internal server body",
			err:  newStreamStatusError(http.StatusInternalServerError, "temporary failure", ""),
			want: "request failed with status 500: temporary failure",
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

func TestDescribeErrorRetryAfterHTTPDate(t *testing.T) {
	retryDate := time.Now().Add(17 * time.Second).UTC().Format(http.TimeFormat)
	got := describeError(newStreamStatusError(http.StatusTooManyRequests, "rate limited", retryDate)).Error()
	if got != "rate limited by Rootly AI: retry in 17s" && got != "rate limited by Rootly AI: retry in 16s" {
		t.Errorf("describeError() = %q, want retry in 16s or 17s", got)
	}
}

func newStreamStatusError(statusCode int, body, retryAfter string) *rootly.AiChatStreamStatusError {
	header := make(http.Header)
	if retryAfter != "" {
		header.Set("Retry-After", retryAfter)
	}
	return &rootly.AiChatStreamStatusError{
		StatusCode: statusCode,
		Status:     fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode)),
		Header:     header,
		Body:       []byte(body),
	}
}

func TestStreamTurnDescribesRequestErrors(t *testing.T) {
	streamer := streamError{err: newStreamStatusError(http.StatusUnauthorized, "auth failed", "")}
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
