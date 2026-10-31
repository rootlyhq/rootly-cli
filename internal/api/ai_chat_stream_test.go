package api

import (
	"context"
	"fmt"
	"iter"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	rootly "github.com/rootlyhq/rootly-go"

	"github.com/rootlyhq/rootly-cli/internal/config"
)

func TestStreamAiChatEvents(t *testing.T) {
	sessionID := uuid.MustParse("39fd7596-152c-43d3-afce-48b792ae386d")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/ai/chat/stream" {
			t.Errorf("path = %q, want /v1/ai/chat/stream", r.URL.Path)
		}
		if got := r.URL.Query().Get("message"); got != "Explain this" {
			t.Errorf("message = %q, want Explain this", got)
		}
		if got := r.URL.Query().Get("session_id"); got != sessionID.String() {
			t.Errorf("session_id = %q, want %q", got, sessionID)
		}
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Errorf("Accept = %q, want text/event-stream", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q, want bearer token", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: text\ndata: {\"content\":\"Hello\"}\n\n")
	}))
	defer server.Close()

	client, err := NewClient(&config.Config{APIKey: "test-token", Endpoint: server.URL})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	message := "Explain this"
	events, err := client.StreamAiChatEvents(context.Background(), &rootly.StreamAiChatParams{
		Message:   &message,
		SessionID: &sessionID,
	})
	if err != nil {
		t.Fatalf("StreamAiChatEvents() error = %v", err)
	}

	count := 0
	for event, err := range events {
		if err != nil {
			t.Fatalf("stream event error = %v", err)
		}
		if event.Type != rootly.AiChatStreamEventText {
			t.Errorf("event type = %q, want text", event.Type)
		}
		var payload rootly.AiChatStreamText
		if err := event.Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if payload.Content != "Hello" {
			t.Errorf("content = %q, want Hello", payload.Content)
		}
		count++
	}
	if count != 1 {
		t.Errorf("event count = %d, want 1", count)
	}
}

func TestDebugTransportDoesNotConsumeEventStream(t *testing.T) {
	unblock := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: text\ndata: {\"content\":\"first\"}\n\n")
		w.(http.Flusher).Flush()
		<-unblock
	}))
	defer server.Close()
	defer close(unblock)

	client, err := NewClient(&config.Config{APIKey: "test-token", Endpoint: server.URL, Debug: true})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	message := "stream"

	type streamResult struct {
		events iter.Seq2[rootly.AiChatStreamEvent, error]
		err    error
	}
	resultCh := make(chan streamResult, 1)
	go func() {
		events, err := client.StreamAiChatEvents(context.Background(), &rootly.StreamAiChatParams{Message: &message})
		resultCh <- streamResult{events: events, err: err}
	}()

	var result streamResult
	select {
	case result = <-resultCh:
	case <-time.After(time.Second):
		t.Fatal("StreamAiChatEvents blocked while receiving response headers")
	}
	if result.err != nil {
		t.Fatalf("StreamAiChatEvents() error = %v", result.err)
	}

	eventCh := make(chan rootly.AiChatStreamEvent, 1)
	errCh := make(chan error, 1)
	go func() {
		for event, err := range result.events {
			if err != nil {
				errCh <- err
				return
			}
			eventCh <- event
			return
		}
	}()

	select {
	case event := <-eventCh:
		var payload rootly.AiChatStreamText
		if err := event.Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if payload.Content != "first" {
			t.Errorf("content = %q, want first", payload.Content)
		}
	case err := <-errCh:
		t.Fatalf("stream event error = %v", err)
	case <-time.After(time.Second):
		t.Fatal("first event was not readable before the server was unblocked")
	}
}
