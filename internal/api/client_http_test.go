package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rootlyhq/rootly-cli/internal/config"
)

// newTestClient creates a Client pointed at the given test server.
func newTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: serverURL,
	}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	return client
}

// --- ListIncidentsCLI ---

func TestListIncidentsCLI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/v1/incidents") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("page[size]") != "10" {
			t.Errorf("expected page size 10, got %s", r.URL.Query().Get("page[size]"))
		}

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"data": []map[string]interface{}{
				{
					"id": "inc-1",
					"attributes": map[string]interface{}{
						"sequential_id": 42,
						"title":         "Test Incident",
						"status":        "started",
						"kind":          "normal",
						"created_at":    "2025-06-15T10:00:00Z",
					},
				},
				{
					"id": "inc-2",
					"attributes": map[string]interface{}{
						"title":      "Another Incident",
						"status":     "resolved",
						"kind":       "normal",
						"created_at": "2025-06-14T10:00:00Z",
					},
				},
			},
			"meta": map[string]interface{}{
				"current_page": 1,
				"total_pages":  2,
				"total_count":  15,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.ListIncidentsCLI(context.Background(), 1, 10, "-created_at", nil)
	if err != nil {
		t.Fatalf("ListIncidentsCLI returned error: %v", err)
	}

	if len(result.Incidents) != 2 {
		t.Fatalf("expected 2 incidents, got %d", len(result.Incidents))
	}
	if result.Incidents[0].SequentialID != "INC-42" {
		t.Errorf("SequentialID = %q, want %q", result.Incidents[0].SequentialID, "INC-42")
	}
	if result.Incidents[0].Title != "Test Incident" {
		t.Errorf("Title = %q, want %q", result.Incidents[0].Title, "Test Incident")
	}
	if result.Pagination.TotalPages != 2 {
		t.Errorf("TotalPages = %d, want 2", result.Pagination.TotalPages)
	}
	if result.Pagination.TotalCount != 15 {
		t.Errorf("TotalCount = %d, want 15", result.Pagination.TotalCount)
	}
}

func TestListIncidentsCLIWithFilters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify filters are passed
		query := r.URL.RawQuery
		if !strings.Contains(query, "filter[status]=started") {
			t.Errorf("expected status filter, got query: %s", query)
		}

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{},
			"meta": map[string]interface{}{"current_page": 1, "total_pages": 1, "total_count": 0},
		})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	filters := map[string]string{"status": "started"}
	result, err := client.ListIncidentsCLI(context.Background(), 1, 25, "", filters)
	if err != nil {
		t.Fatalf("ListIncidentsCLI returned error: %v", err)
	}
	if len(result.Incidents) != 0 {
		t.Fatalf("expected 0 incidents, got %d", len(result.Incidents))
	}
}

func TestListIncidentsCLIPageSizeCap(t *testing.T) {
	var receivedPageSize string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPageSize = r.URL.Query().Get("page[size]")
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{},
			"meta": map[string]interface{}{"current_page": 1, "total_pages": 1, "total_count": 0},
		})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	// pageSize 0 defaults to 25
	_, _ = client.ListIncidentsCLI(context.Background(), 1, 0, "", nil)
	if receivedPageSize != "25" {
		t.Errorf("default page size = %q, want 25", receivedPageSize)
	}

	// pageSize > 100 capped to 100
	_, _ = client.ListIncidentsCLI(context.Background(), 1, 200, "", nil)
	if receivedPageSize != "100" {
		t.Errorf("capped page size = %q, want 100", receivedPageSize)
	}
}

func TestListIncidentsCLIUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.ListIncidentsCLI(context.Background(), 1, 25, "", nil)
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "invalid API token") {
		t.Errorf("error = %q, want 'invalid API token'", err.Error())
	}
}

func TestListIncidentsCLIForbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.ListIncidentsCLI(context.Background(), 1, 25, "", nil)
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Errorf("error = %q, want 'access denied'", err.Error())
	}
}

// --- GetIncidentByID ---

func TestGetIncidentByID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/v1/incidents/inc-123") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"id": "inc-123",
				"attributes": map[string]interface{}{
					"sequential_id": 99,
					"title":         "Production Outage",
					"summary":       "DB is down",
					"status":        "started",
					"kind":          "normal",
					"created_at":    "2025-06-15T10:00:00Z",
					"url":           "https://rootly.com/incidents/99",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	inc, err := client.GetIncidentByID(context.Background(), "inc-123")
	if err != nil {
		t.Fatalf("GetIncidentByID returned error: %v", err)
	}

	if inc.ID != "inc-123" {
		t.Errorf("ID = %q, want %q", inc.ID, "inc-123")
	}
	if inc.SequentialID != "INC-99" {
		t.Errorf("SequentialID = %q, want %q", inc.SequentialID, "INC-99")
	}
	if inc.Title != "Production Outage" {
		t.Errorf("Title = %q, want %q", inc.Title, "Production Outage")
	}
	if inc.URL != "https://rootly.com/incidents/99" {
		t.Errorf("URL = %q, want %q", inc.URL, "https://rootly.com/incidents/99")
	}
}

func TestGetIncidentByIDNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.GetIncidentByID(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "incident not found") {
		t.Errorf("error = %q, want 'incident not found'", err.Error())
	}
}

// --- CreateIncident ---

func TestCreateIncident(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusCreated)
		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"id": "inc-new",
				"attributes": map[string]interface{}{
					"sequential_id": 100,
					"title":         "New Incident",
					"status":        "started",
					"kind":          "normal",
					"created_at":    "2025-06-15T12:00:00Z",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	inc, err := client.CreateIncident(context.Background(), "New Incident", map[string]string{
		"summary": "Test summary",
		"status":  "started",
	})
	if err != nil {
		t.Fatalf("CreateIncident returned error: %v", err)
	}

	if inc.Title != "New Incident" {
		t.Errorf("Title = %q, want %q", inc.Title, "New Incident")
	}
	if inc.SequentialID != "INC-100" {
		t.Errorf("SequentialID = %q, want %q", inc.SequentialID, "INC-100")
	}
}

func TestCreateIncidentUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.CreateIncident(context.Background(), "Test", nil)
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "invalid API token") {
		t.Errorf("error = %q, want 'invalid API token'", err.Error())
	}
}

// --- DeleteIncident ---

func TestDeleteIncident(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/v1/incidents/inc-del") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.DeleteIncident(context.Background(), "inc-del")
	if err != nil {
		t.Fatalf("DeleteIncident returned error: %v", err)
	}
}

func TestDeleteIncidentNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.DeleteIncident(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "incident not found") {
		t.Errorf("error = %q, want 'incident not found'", err.Error())
	}
}

func TestDeleteIncidentForbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.DeleteIncident(context.Background(), "inc-1")
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Errorf("error = %q, want 'access denied'", err.Error())
	}
}

// --- Auth header verification ---

func TestAuthorizationHeader(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{},
			"meta": map[string]interface{}{"current_page": 1, "total_pages": 1, "total_count": 0},
		})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, _ = client.ListIncidentsCLI(context.Background(), 1, 25, "", nil)

	if receivedAuth != "Bearer test-key" {
		t.Errorf("Authorization = %q, want %q", receivedAuth, "Bearer test-key")
	}
}

// --- Server error ---

func TestListIncidentsCLIServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.ListIncidentsCLI(context.Background(), 1, 25, "", nil)
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Errorf("error = %q, want 'status 500'", err.Error())
	}
}
