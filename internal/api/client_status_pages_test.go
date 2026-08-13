package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func statusPageEventResponse() string {
	return `{
		"data": {
			"id": "event-1",
			"attributes": {
				"event": "We are investigating.",
				"status": "investigating",
				"status_page_id": "page-1",
				"notify_subscribers": true,
				"started_at": "2026-08-12T12:00:00Z",
				"created_at": "2026-08-12T12:00:00Z",
				"updated_at": "2026-08-12T12:00:00Z"
			}
		}
	}`
}

func TestListStatusPagesCLI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/status-pages" {
			t.Errorf("path = %s, want /v1/status-pages", r.URL.Path)
		}
		if got := r.URL.Query().Get("filter[slug]"); got != "public-status" {
			t.Errorf("slug filter = %q, want public-status", got)
		}
		if got := r.URL.Query().Get("filter[name]"); got != "Public" {
			t.Errorf("name filter = %q, want Public", got)
		}
		_, _ = w.Write([]byte(`{
			"data": [{"id": "page-1", "attributes": {
				"title": "Public Status", "slug": "public-status", "description": "Customer updates",
				"enabled": true, "public": true, "created_at": "2026-08-12T12:00:00Z"
			}}],
			"meta": {"current_page": 1, "total_pages": 1, "total_count": 1}
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.ListStatusPagesCLI(context.Background(), 1, 25, "-created_at", map[string]string{"name": "Public", "slug": "public-status"})
	if err != nil {
		t.Fatalf("ListStatusPagesCLI returned error: %v", err)
	}
	if len(result.StatusPages) != 1 || result.StatusPages[0].Slug != "public-status" {
		t.Fatalf("status pages = %+v, want public-status", result.StatusPages)
	}
	if !result.StatusPages[0].Public || !result.StatusPages[0].Enabled {
		t.Error("status page should be public and enabled")
	}
}

func TestStatusPageListsRejectNegativePagination(t *testing.T) {
	client := &Client{}
	if _, err := client.ListStatusPagesCLI(context.Background(), -1, 25, "", nil); err == nil || !strings.Contains(err.Error(), "page must be at least 1") {
		t.Fatalf("status-page list error = %v, want minimum page validation", err)
	}
	if _, err := client.ListStatusPageEventsCLI(context.Background(), "42", 1, -1); err == nil || !strings.Contains(err.Error(), "page size must not be negative") {
		t.Fatalf("event list error = %v, want non-negative page-size validation", err)
	}
}

func TestListStatusPageEventsCLI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/incidents/42/status-page-events" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"data": [{"id": "event-1", "attributes": {
				"event": "Monitoring", "status": "monitoring", "status_page_id": "page-1",
				"started_at": "2026-08-12T12:00:00Z", "created_at": "2026-08-12T12:00:00Z", "updated_at": "2026-08-12T12:30:00Z"
			}}],
			"meta": {"current_page": 1, "total_pages": 1, "total_count": 1}
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.ListStatusPageEventsCLI(context.Background(), "42", 1, 25)
	if err != nil {
		t.Fatalf("ListStatusPageEventsCLI returned error: %v", err)
	}
	if len(result.Events) != 1 || result.Events[0].Status != "monitoring" {
		t.Fatalf("events = %+v, want monitoring event", result.Events)
	}
}

func TestCreateStatusPageEventCLI(t *testing.T) {
	var requestBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/incidents/42/status-page-events" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &requestBody)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(statusPageEventResponse()))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	startedAt := time.Date(2026, time.August, 12, 11, 30, 0, 0, time.UTC)
	event, err := client.CreateStatusPageEventCLI(context.Background(), "42", CreateStatusPageEventOpts{
		StatusPageID: "page-1", Status: "investigating", Message: "We are investigating.", NotifySubscribers: true, StartedAt: &startedAt,
	})
	if err != nil {
		t.Fatalf("CreateStatusPageEventCLI returned error: %v", err)
	}
	attributes := requestBody["data"].(map[string]interface{})["attributes"].(map[string]interface{})
	if attributes["status_page_id"] != "page-1" || attributes["status"] != "investigating" || attributes["notify_subscribers"] != true || attributes["started_at"] != "2026-08-12T11:30:00Z" {
		t.Errorf("unexpected attributes: %+v", attributes)
	}
	if event.ID != "event-1" {
		t.Errorf("event ID = %q, want event-1", event.ID)
	}
}

func TestUpdateStatusPageEventCLI(t *testing.T) {
	var requestBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/v1/status-page-events/event-1" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &requestBody)
		_, _ = w.Write([]byte(statusPageEventResponse()))
	}))
	defer server.Close()

	status := "resolved"
	message := "Resolved."
	startedAt := time.Date(2026, time.August, 12, 12, 15, 0, 0, time.UTC)
	client := newTestClient(t, server.URL)
	_, err := client.UpdateStatusPageEventCLI(context.Background(), "event-1", StatusPageEventOpts{
		Status: &status, Message: &message, StartedAt: &startedAt,
	})
	if err != nil {
		t.Fatalf("UpdateStatusPageEventCLI returned error: %v", err)
	}
	attributes := requestBody["data"].(map[string]interface{})["attributes"].(map[string]interface{})
	if attributes["status"] != "resolved" || attributes["event"] != "Resolved." || attributes["started_at"] != "2026-08-12T12:15:00Z" {
		t.Errorf("unexpected attributes: %+v", attributes)
	}
}

func TestStatusPageEventValidationError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"errors":[{"title":"Status is not valid for this incident","status":"422"}]}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.UpdateStatusPageEventCLI(context.Background(), "event-1", StatusPageEventOpts{})
	if err == nil || !strings.Contains(err.Error(), "Status is not valid for this incident") {
		t.Fatalf("error = %v, want server validation title", err)
	}
}
