package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	rootly "github.com/rootlyhq/rootly-go"

	"github.com/rootlyhq/rootly-cli/internal/config"
	"github.com/rootlyhq/rootly-cli/internal/mcp"
)

func TestStreamAddsClientToolsBody(t *testing.T) {
	var body map[string]any
	var query, contentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		query = r.URL.Query().Get("message")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: done\ndata: {}\n\n")
	}))
	defer server.Close()
	client, err := NewClient(&config.Config{APIKey: "test-token", Endpoint: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	message := "hi"
	tools := []mcp.ClientTool{{Name: "local_fs_read", Description: "read", InputSchema: json.RawMessage(`{"type":"object"}`), ReadOnly: true}}
	events, err := client.StreamAiChatEvents(WithClientTools(context.Background(), tools), &rootly.StreamAiChatParams{Message: &message})
	if err != nil {
		t.Fatal(err)
	}
	for range events {
	}
	sent, _ := body["client_tools"].([]any)
	if query != "hi" || len(sent) != 1 || sent[0].(map[string]any)["read_only"] != true || contentType != "application/vnd.api+json" {
		t.Fatalf("query=%q content-type=%q body=%v", query, contentType, body)
	}
}

func TestPostToolResultAndConnectors(t *testing.T) {
	var posted ToolResult
	var connectorQuery, postedContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/ai/chat/tool_results":
			postedContentType = r.Header.Get("Content-Type")
			_ = json.NewDecoder(r.Body).Decode(&posted)
			if posted.CallID == "missing" {
				w.Header().Set("Content-Type", "application/vnd.api+json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"errors":[{"title":"Unknown call"}]}`)
				return
			}
			w.WriteHeader(http.StatusAccepted)
		case "/v1/ai/connectors":
			connectorQuery = r.URL.RawQuery
			_, _ = io.WriteString(w, `{"data":[{"id":"c1","type":"ai_connectors","attributes":{"name":"Datadog","provider_type":"datadog","status":"connected","available_to_agent":true,"tool_count":12,"manage_url":"https://rootly.com/x"}},{"id":"c2","type":"ai_connectors","attributes":{"name":"Custom","provider_type":"custom","status":"error","available_to_agent":false,"tool_count":null,"manage_url":""}}],"meta":{"current_page":1,"next_page":2,"total_pages":2}}`)
		}
	}))
	defer server.Close()
	client, err := NewClient(&config.Config{APIKey: "test-token", Endpoint: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.PostAiChatToolResult(context.Background(), ToolResult{SessionID: "s", CallID: "c", Output: "User declined", IsError: true}); err != nil {
		t.Fatal(err)
	}
	if posted.Output != "User declined" || !posted.IsError || posted.SessionID != "s" || postedContentType != "application/vnd.api+json" {
		t.Fatalf("posted = %+v", posted)
	}
	if err := client.PostAiChatToolResult(context.Background(), ToolResult{CallID: "missing"}); err == nil || !strings.Contains(err.Error(), "Unknown call") {
		t.Fatalf("err = %v", err)
	}
	page, err := client.ListAIConnectors(context.Background(), 1, 25)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(connectorQuery, "page%5Bnumber%5D=1") && !strings.Contains(connectorQuery, "page[number]=1") {
		t.Fatalf("query = %q", connectorQuery)
	}
	if len(page.Connectors) != 2 || page.NextPage != 2 || *page.Connectors[0].ToolCount != 12 || page.Connectors[1].ToolCount != nil {
		t.Fatalf("page = %+v", page)
	}
}
