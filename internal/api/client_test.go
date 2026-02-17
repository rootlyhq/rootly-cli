package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/rootlyhq/rootly-tui/internal/config"
)

// setupTestEnv sets up a temporary home directory for test isolation.
// On Unix, sets HOME; on Windows, sets USERPROFILE (used by os.UserHomeDir).
// Returns a cleanup function that should be deferred.
func setupTestEnv(t *testing.T) func() {
	t.Helper()
	tmpDir := t.TempDir()

	if runtime.GOOS == "windows" {
		originalUserProfile := os.Getenv("USERPROFILE")
		os.Setenv("USERPROFILE", tmpDir)
		return func() {
			os.Setenv("USERPROFILE", originalUserProfile)
		}
	}

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	return func() {
		os.Setenv("HOME", originalHome)
	}
}

func TestNewClient(t *testing.T) {
	defer setupTestEnv(t)()

	cfg := &config.Config{
		APIKey:   "test-api-key",
		Endpoint: "api.rootly.com",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	if client == nil {
		t.Fatal("expected client to be non-nil")
	}
}

func TestNewClientWithHTTPS(t *testing.T) {
	defer setupTestEnv(t)()

	tests := []struct {
		name     string
		endpoint string
	}{
		{"hostname only", "api.rootly.com"},
		{"with https", "https://api.rootly.com"},
		{"with http", "http://localhost:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				APIKey:   "test-key",
				Endpoint: tt.endpoint,
			}

			client, err := NewClient(cfg)
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}
			defer client.Close()

			if client == nil {
				t.Fatal("expected client to be non-nil")
			}
		})
	}
}

func TestUserAgentHeader(t *testing.T) {
	defer setupTestEnv(t)()

	// Set a test version
	Version = "1.2.3"

	var receivedUserAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUserAgent = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"id":   "123",
				"type": "users",
				"attributes": map[string]interface{}{
					"name":  "Test User",
					"email": "test@example.com",
				},
			},
		})
	}))
	defer server.Close()

	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: server.URL,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	// Make a request to trigger the header
	_ = client.ValidateAPIKey(context.Background())

	expectedUserAgent := "rootly-tui/1.2.3"
	if receivedUserAgent != expectedUserAgent {
		t.Errorf("User-Agent = %q, want %q", receivedUserAgent, expectedUserAgent)
	}
}

func TestValidateAPIKey(t *testing.T) {
	defer setupTestEnv(t)()

	// Create mock server for /v1/users/me endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check authorization header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer valid-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Check that it's the users/me endpoint
		if r.URL.Path != "/v1/users/me" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// Return valid user response
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"id":   "123",
				"type": "users",
				"attributes": map[string]interface{}{
					"name":  "Test User",
					"email": "test@example.com",
				},
			},
		})
	}))
	defer server.Close()

	tests := []struct {
		name    string
		apiKey  string
		wantErr bool
	}{
		{"valid key", "valid-key", false},
		{"invalid key", "invalid-key", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				APIKey:   tt.apiKey,
				Endpoint: server.URL,
			}

			client, err := NewClient(cfg)
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}
			defer client.Close()

			err = client.ValidateAPIKey(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAPIKey() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestListIncidents(t *testing.T) {
	defer setupTestEnv(t)()

	// Create mock server that returns incidents
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify path
		if r.URL.Path != "/v1/incidents" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)

		response := map[string]interface{}{
			"data": []map[string]interface{}{
				{
					"id": "inc_001",
					"attributes": map[string]interface{}{
						"title":      "Test Incident 1",
						"summary":    "This is a test incident",
						"status":     "in_progress",
						"kind":       "incident",
						"created_at": "2025-01-01T10:00:00Z",
					},
				},
				{
					"id": "inc_002",
					"attributes": map[string]interface{}{
						"title":      "Test Incident 2",
						"summary":    "Another test incident",
						"status":     "resolved",
						"kind":       "incident",
						"created_at": "2025-01-01T09:00:00Z",
					},
				},
			},
			"meta": map[string]interface{}{
				"current_page": 1,
				"next_page":    2,
				"prev_page":    nil,
				"total_count":  50,
				"total_pages":  5,
			},
		}

		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: server.URL,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	result, err := client.ListIncidents(context.Background(), 1, "")
	if err != nil {
		t.Fatalf("ListIncidents() error = %v", err)
	}

	if len(result.Incidents) != 2 {
		t.Errorf("expected 2 incidents, got %d", len(result.Incidents))
	}

	if result.Incidents[0].ID != "inc_001" {
		t.Errorf("expected first incident ID 'inc_001', got '%s'", result.Incidents[0].ID)
	}

	if result.Incidents[0].Status != "in_progress" {
		t.Errorf("expected status 'in_progress', got '%s'", result.Incidents[0].Status)
	}

	if result.Pagination.CurrentPage != 1 {
		t.Errorf("expected current page 1, got %d", result.Pagination.CurrentPage)
	}

	if result.Pagination.TotalPages != 5 {
		t.Errorf("expected total pages 5, got %d", result.Pagination.TotalPages)
	}

	if result.Pagination.TotalCount != 50 {
		t.Errorf("expected total count 50, got %d", result.Pagination.TotalCount)
	}

	if !result.Pagination.HasNext {
		t.Error("expected HasNext to be true")
	}
}

func TestListAlerts(t *testing.T) {
	defer setupTestEnv(t)()

	// Create mock server that returns alerts
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify path
		if r.URL.Path != "/v1/alerts" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)

		response := map[string]interface{}{
			"data": []map[string]interface{}{
				{
					"id": "alert_001",
					"attributes": map[string]interface{}{
						"short_id":   "ALT001",
						"summary":    "High CPU Usage",
						"status":     "triggered",
						"source":     "datadog",
						"created_at": "2025-01-01T10:00:00Z",
						"updated_at": "2025-01-01T10:00:00Z",
					},
				},
				{
					"id": "alert_002",
					"attributes": map[string]interface{}{
						"short_id":    "ALT002",
						"summary":     "Memory Warning",
						"description": "Memory usage is high",
						"status":      "acknowledged",
						"source":      "grafana",
						"created_at":  "2025-01-01T09:00:00Z",
						"updated_at":  "2025-01-01T09:00:00Z",
					},
				},
			},
			"links": map[string]interface{}{
				"first": "http://test/v1/alerts?page=1",
				"last":  "http://test/v1/alerts?page=5",
				"self":  "http://test/v1/alerts?page=1",
			},
			"meta": map[string]interface{}{
				"current_page": 1,
				"next_page":    2,
				"prev_page":    nil,
				"total_count":  50,
				"total_pages":  5,
			},
		}

		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: server.URL,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	result, err := client.ListAlerts(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListAlerts() error = %v", err)
	}

	if len(result.Alerts) != 2 {
		t.Errorf("expected 2 alerts, got %d", len(result.Alerts))
	}

	if result.Alerts[0].ID != "alert_001" {
		t.Errorf("expected first alert ID 'alert_001', got '%s'", result.Alerts[0].ID)
	}

	if result.Alerts[0].Source != "datadog" {
		t.Errorf("expected source 'datadog', got '%s'", result.Alerts[0].Source)
	}

	if result.Alerts[1].Description != "Memory usage is high" {
		t.Errorf("expected description 'Memory usage is high', got '%s'", result.Alerts[1].Description)
	}

	if result.Pagination.CurrentPage != 1 {
		t.Errorf("expected current page 1, got %d", result.Pagination.CurrentPage)
	}

	if result.Pagination.TotalPages != 5 {
		t.Errorf("expected total pages 5, got %d", result.Pagination.TotalPages)
	}

	if result.Pagination.TotalCount != 50 {
		t.Errorf("expected total count 50, got %d", result.Pagination.TotalCount)
	}

	if !result.Pagination.HasNext {
		t.Error("expected HasNext to be true")
	}
}

func TestListIncidentsError(t *testing.T) {
	defer setupTestEnv(t)()

	// Create mock server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: server.URL,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	_, err = client.ListIncidents(context.Background(), 1, "")
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestMockIncidents(t *testing.T) {
	incidents := MockIncidents()

	if len(incidents) == 0 {
		t.Error("expected mock incidents to be non-empty")
	}

	// Verify first incident has required fields
	inc := incidents[0]
	if inc.ID == "" {
		t.Error("expected incident ID to be non-empty")
	}
	if inc.Summary == "" {
		t.Error("expected incident summary to be non-empty")
	}
	if inc.Status == "" {
		t.Error("expected incident status to be non-empty")
	}
	if inc.Severity == "" {
		t.Error("expected incident severity to be non-empty")
	}
}

func TestMockAlerts(t *testing.T) {
	alerts := MockAlerts()

	if len(alerts) == 0 {
		t.Error("expected mock alerts to be non-empty")
	}

	// Verify first alert has required fields
	alert := alerts[0]
	if alert.ID == "" {
		t.Error("expected alert ID to be non-empty")
	}
	if alert.Summary == "" {
		t.Error("expected alert summary to be non-empty")
	}
	if alert.Status == "" {
		t.Error("expected alert status to be non-empty")
	}
	if alert.Source == "" {
		t.Error("expected alert source to be non-empty")
	}
}

func TestClearCache(t *testing.T) {
	defer setupTestEnv(t)()

	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: "api.rootly.com",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	// Skip if cache is nil (fallback mode)
	if client.cache == nil {
		t.Skip("persistent cache not available in test environment")
	}

	// Add something to cache
	client.cache.Set("test-key", "test-value")

	// Verify it's there
	if _, ok := client.cache.Get("test-key"); !ok {
		t.Error("expected cache to have test-key")
	}

	// Clear cache
	client.ClearCache()

	// Verify it's gone
	if _, ok := client.cache.Get("test-key"); ok {
		t.Error("expected cache to be cleared")
	}
}

func TestParseTimePtr(t *testing.T) {
	tests := []struct {
		name    string
		input   *string
		wantNil bool
	}{
		{"nil input", nil, true},
		{"empty string", strPtr(""), true},
		{"valid RFC3339", strPtr("2025-01-01T10:00:00Z"), false},
		{"invalid format", strPtr("not a date"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTimePtr(tt.input)
			if tt.wantNil && result != nil {
				t.Errorf("expected nil, got %v", result)
			}
			if !tt.wantNil && result == nil {
				t.Error("expected non-nil result")
			}
		})
	}
}

func strPtr(s string) *string {
	return &s
}

func TestListIncidentsWithCache(t *testing.T) {
	defer setupTestEnv(t)()

	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		response := map[string]interface{}{
			"data": []map[string]interface{}{
				{
					"id": "inc_001",
					"attributes": map[string]interface{}{
						"title":      "Test Incident",
						"status":     "in_progress",
						"created_at": "2025-01-01T10:00:00Z",
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: server.URL,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	// Skip if cache is nil (fallback mode)
	if client.cache == nil {
		t.Skip("persistent cache not available in test environment")
	}

	// First call
	_, err = client.ListIncidents(context.Background(), 1, "")
	if err != nil {
		t.Fatalf("first ListIncidents() error = %v", err)
	}

	// Second call should hit cache
	_, err = client.ListIncidents(context.Background(), 1, "")
	if err != nil {
		t.Fatalf("second ListIncidents() error = %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected 1 API call (cached), got %d", callCount)
	}
}

func TestListAlertsWithLabels(t *testing.T) {
	defer setupTestEnv(t)()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)

		response := map[string]interface{}{
			"data": []map[string]interface{}{
				{
					"id": "alert_001",
					"attributes": map[string]interface{}{
						"summary":    "Alert with labels",
						"status":     "triggered",
						"source":     "datadog",
						"created_at": "2025-01-01T10:00:00Z",
						"labels": []map[string]interface{}{
							{"key": "priority", "value": "high"},
							{"key": "count", "value": 42},     // numeric value
							{"key": "enabled", "value": true}, // boolean value
						},
						"services": []map[string]interface{}{
							{"name": "api-server"},
						},
						"environments": []map[string]interface{}{
							{"name": "production"},
						},
						"groups": []map[string]interface{}{
							{"name": "platform-team"},
						},
					},
				},
			},
		}

		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: server.URL,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	result, err := client.ListAlerts(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListAlerts() error = %v", err)
	}

	if len(result.Alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(result.Alerts))
	}

	alert := result.Alerts[0]

	// Check labels were parsed correctly
	if alert.Labels["priority"] != "high" {
		t.Errorf("expected label priority=high, got %s", alert.Labels["priority"])
	}
	if alert.Labels["count"] != "42" {
		t.Errorf("expected label count=42, got %s", alert.Labels["count"])
	}
	if alert.Labels["enabled"] != "true" {
		t.Errorf("expected label enabled=true, got %s", alert.Labels["enabled"])
	}

	// Check services, environments, groups
	if len(alert.Services) != 1 || alert.Services[0] != "api-server" {
		t.Errorf("expected services=[api-server], got %v", alert.Services)
	}
	if len(alert.Environments) != 1 || alert.Environments[0] != "production" {
		t.Errorf("expected environments=[production], got %v", alert.Environments)
	}
	if len(alert.Groups) != 1 || alert.Groups[0] != "platform-team" {
		t.Errorf("expected groups=[platform-team], got %v", alert.Groups)
	}
}

func TestListIncidentsWithTimestamps(t *testing.T) {
	defer setupTestEnv(t)()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)

		response := map[string]interface{}{
			"data": []map[string]interface{}{
				{
					"id": "inc_full",
					"attributes": map[string]interface{}{
						"sequential_id":     123,
						"title":             "Full Incident",
						"summary":           "Complete incident",
						"status":            "resolved",
						"kind":              "incident",
						"created_at":        "2025-01-01T10:00:00Z",
						"started_at":        "2025-01-01T10:01:00Z",
						"detected_at":       "2025-01-01T10:02:00Z",
						"acknowledged_at":   "2025-01-01T10:03:00Z",
						"mitigated_at":      "2025-01-01T10:04:00Z",
						"resolved_at":       "2025-01-01T10:05:00Z",
						"slack_channel_url": "https://slack.com/channel",
						"jira_issue_url":    "https://jira.com/issue",
						"severity": map[string]interface{}{
							"data": map[string]interface{}{
								"attributes": map[string]interface{}{
									"name": "critical",
								},
							},
						},
					},
				},
			},
		}

		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: server.URL,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	result, err := client.ListIncidents(context.Background(), 1, "")
	if err != nil {
		t.Fatalf("ListIncidents() error = %v", err)
	}

	if len(result.Incidents) != 1 {
		t.Fatalf("expected 1 incident, got %d", len(result.Incidents))
	}

	inc := result.Incidents[0]

	if inc.SequentialID != "INC-123" {
		t.Errorf("expected SequentialID=INC-123, got %s", inc.SequentialID)
	}
	if inc.Severity != "critical" {
		t.Errorf("expected Severity=critical, got %s", inc.Severity)
	}
	if inc.SlackChannelURL != "https://slack.com/channel" {
		t.Errorf("expected SlackChannelURL, got %s", inc.SlackChannelURL)
	}
	if inc.JiraIssueURL != "https://jira.com/issue" {
		t.Errorf("expected JiraIssueURL, got %s", inc.JiraIssueURL)
	}
	if inc.StartedAt == nil {
		t.Error("expected StartedAt to be set")
	}
	if inc.ResolvedAt == nil {
		t.Error("expected ResolvedAt to be set")
	}
	if inc.DetectedAt == nil {
		t.Error("expected DetectedAt to be set")
	}
	if inc.AcknowledgedAt == nil {
		t.Error("expected AcknowledgedAt to be set")
	}
	if inc.MitigatedAt == nil {
		t.Error("expected MitigatedAt to be set")
	}
}

func TestListAlertsError(t *testing.T) {
	defer setupTestEnv(t)()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: server.URL,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	_, err = client.ListAlerts(context.Background(), 1)
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestGetIncident(t *testing.T) {
	defer setupTestEnv(t)()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request path includes the incident ID
		if !strings.Contains(r.URL.Path, "/v1/incidents/inc_123") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		// Verify includes are requested
		if !strings.Contains(r.URL.RawQuery, "include=") {
			t.Error("expected include parameter in query")
		}

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)

		response := map[string]interface{}{
			"data": map[string]interface{}{
				"id": "inc_123",
				"attributes": map[string]interface{}{
					"sequential_id":     456,
					"title":             "Database Outage",
					"summary":           "Production database went down",
					"status":            "resolved",
					"kind":              "incident",
					"url":               "https://rootly.io/incidents/inc_123",
					"created_at":        "2025-01-01T10:00:00Z",
					"updated_at":        "2025-01-01T12:00:00Z",
					"started_at":        "2025-01-01T10:01:00Z",
					"resolved_at":       "2025-01-01T11:00:00Z",
					"slack_channel_url": "https://slack.com/channel",
					"severity": map[string]interface{}{
						"data": map[string]interface{}{
							"attributes": map[string]interface{}{
								"name": "critical",
							},
						},
					},
					"services": map[string]interface{}{
						"data": []map[string]interface{}{
							{"attributes": map[string]interface{}{"name": "api-server"}},
						},
					},
					"causes": map[string]interface{}{
						"data": []map[string]interface{}{
							{"attributes": map[string]interface{}{"name": "Configuration Error"}},
						},
					},
					"incident_types": map[string]interface{}{
						"data": []map[string]interface{}{
							{"attributes": map[string]interface{}{"name": "Infrastructure"}},
						},
					},
					"user": map[string]interface{}{
						"data": map[string]interface{}{
							"attributes": map[string]interface{}{
								"name":  "Creator User",
								"email": "creator@example.com",
							},
						},
					},
				},
			},
			"included": []map[string]interface{}{
				{
					"id":   "role_1",
					"type": "incident_role_assignments",
					"attributes": map[string]interface{}{
						"incident_role": map[string]interface{}{
							"data": map[string]interface{}{
								"attributes": map[string]interface{}{
									"name": "Commander",
								},
							},
						},
						"user": map[string]interface{}{
							"data": map[string]interface{}{
								"attributes": map[string]interface{}{
									"name":  "John Doe",
									"email": "john.doe@example.com",
								},
							},
						},
					},
				},
				{
					"id":   "role_2",
					"type": "incident_role_assignments",
					"attributes": map[string]interface{}{
						"incident_role": map[string]interface{}{
							"data": map[string]interface{}{
								"attributes": map[string]interface{}{
									"name": "Communications Lead",
								},
							},
						},
						"user": map[string]interface{}{
							"data": map[string]interface{}{
								"attributes": map[string]interface{}{
									"name":  "Jane Smith",
									"email": "jane.smith@example.com",
								},
							},
						},
					},
				},
			},
		}

		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: server.URL,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	// Use a fixed time for cache key - matches updated_at in test fixture
	updatedAt, _ := time.Parse(time.RFC3339, "2025-01-01T12:00:00Z")
	incident, err := client.GetIncident(context.Background(), "inc_123", updatedAt)
	if err != nil {
		t.Fatalf("GetIncident() error = %v", err)
	}

	// Verify basic fields
	if incident.ID != "inc_123" {
		t.Errorf("expected ID=inc_123, got %s", incident.ID)
	}
	if incident.SequentialID != "INC-456" {
		t.Errorf("expected SequentialID=INC-456, got %s", incident.SequentialID)
	}
	if incident.Title != "Database Outage" {
		t.Errorf("expected Title='Database Outage', got %s", incident.Title)
	}
	if incident.Status != "resolved" {
		t.Errorf("expected Status=resolved, got %s", incident.Status)
	}
	if incident.Severity != "critical" {
		t.Errorf("expected Severity=critical, got %s", incident.Severity)
	}

	// Verify detail fields
	if !incident.DetailLoaded {
		t.Error("expected DetailLoaded=true")
	}
	if incident.URL != "https://rootly.io/incidents/inc_123" {
		t.Errorf("expected URL, got %s", incident.URL)
	}
	if incident.CommanderName != "John Doe" {
		t.Errorf("expected CommanderName='John Doe', got %s", incident.CommanderName)
	}
	if incident.CommunicatorName != "Jane Smith" {
		t.Errorf("expected CommunicatorName='Jane Smith', got %s", incident.CommunicatorName)
	}
	if len(incident.Roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(incident.Roles))
	}
	// Check email is populated
	for _, role := range incident.Roles {
		if role.Name == "Commander" && role.UserEmail != "john.doe@example.com" {
			t.Errorf("expected Commander email='john.doe@example.com', got %s", role.UserEmail)
		}
		if role.Name == "Communications Lead" && role.UserEmail != "jane.smith@example.com" {
			t.Errorf("expected Communications Lead email='jane.smith@example.com', got %s", role.UserEmail)
		}
	}
	// Check creator is populated
	if incident.CreatedByName != "Creator User" {
		t.Errorf("expected CreatedByName='Creator User', got %s", incident.CreatedByName)
	}
	if incident.CreatedByEmail != "creator@example.com" {
		t.Errorf("expected CreatedByEmail='creator@example.com', got %s", incident.CreatedByEmail)
	}
	if len(incident.Causes) != 1 || incident.Causes[0] != "Configuration Error" {
		t.Errorf("expected Causes=['Configuration Error'], got %v", incident.Causes)
	}
	if len(incident.IncidentTypes) != 1 || incident.IncidentTypes[0] != "Infrastructure" {
		t.Errorf("expected IncidentTypes=['Infrastructure'], got %v", incident.IncidentTypes)
	}
	if len(incident.Services) != 1 || incident.Services[0] != "api-server" {
		t.Errorf("expected Services=['api-server'], got %v", incident.Services)
	}
	if incident.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set")
	}
}

func TestGetAlert(t *testing.T) {
	defer setupTestEnv(t)()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request path includes the alert ID
		if !strings.Contains(r.URL.Path, "/v1/alerts/alert_123") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		// Verify includes are requested
		if !strings.Contains(r.URL.RawQuery, "include=") {
			t.Error("expected include parameter in query")
		}

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)

		response := map[string]interface{}{
			"data": map[string]interface{}{
				"id": "alert_123",
				"attributes": map[string]interface{}{
					"short_id":     "ABC123",
					"summary":      "High CPU Usage",
					"description":  "CPU usage exceeded 90%",
					"status":       "triggered",
					"source":       "datadog",
					"external_url": "https://datadog.com/alert/123",
					"created_at":   "2025-01-01T10:00:00Z",
					"updated_at":   "2025-01-01T10:30:00Z",
					"started_at":   "2025-01-01T10:00:00Z",
					"labels": []map[string]interface{}{
						{"key": "severity", "value": "high"},
					},
					"services": []map[string]interface{}{
						{"name": "web-service"},
					},
					"environments": []map[string]interface{}{
						{"name": "production"},
					},
					"groups": []map[string]interface{}{
						{"name": "platform-team"},
					},
					"responders": []map[string]interface{}{
						{
							"id": 123,
							"attributes": map[string]interface{}{
								"user": map[string]interface{}{
									"data": map[string]interface{}{
										"attributes": map[string]interface{}{
											"name": "On-call Engineer",
										},
									},
								},
							},
						},
					},
					"alert_urgency": map[string]interface{}{
						"data": map[string]interface{}{
							"attributes": map[string]interface{}{
								"name": "High",
							},
						},
					},
				},
			},
		}

		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: server.URL,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	// Use a fixed time for cache key - matches updated_at in test fixture
	updatedAt, _ := time.Parse(time.RFC3339, "2025-01-01T10:30:00Z")
	alert, err := client.GetAlert(context.Background(), "alert_123", updatedAt)
	if err != nil {
		t.Fatalf("GetAlert() error = %v", err)
	}

	// Verify basic fields
	if alert.ID != "alert_123" {
		t.Errorf("expected ID=alert_123, got %s", alert.ID)
	}
	if alert.ShortID != "ABC123" {
		t.Errorf("expected ShortID=ABC123, got %s", alert.ShortID)
	}
	if alert.Summary != "High CPU Usage" {
		t.Errorf("expected Summary='High CPU Usage', got %s", alert.Summary)
	}
	if alert.Status != "triggered" {
		t.Errorf("expected Status=triggered, got %s", alert.Status)
	}
	if alert.Source != "datadog" {
		t.Errorf("expected Source=datadog, got %s", alert.Source)
	}

	// Verify detail fields
	if !alert.DetailLoaded {
		t.Error("expected DetailLoaded=true")
	}
	if alert.Urgency != "High" {
		t.Errorf("expected Urgency='High', got %s", alert.Urgency)
	}
	if len(alert.Responders) != 1 || alert.Responders[0] != "On-call Engineer" {
		t.Errorf("expected Responders=['On-call Engineer'], got %v", alert.Responders)
	}
	if len(alert.Services) != 1 || alert.Services[0] != "web-service" {
		t.Errorf("expected Services=['web-service'], got %v", alert.Services)
	}
	if len(alert.Environments) != 1 || alert.Environments[0] != "production" {
		t.Errorf("expected Environments=['production'], got %v", alert.Environments)
	}
	if len(alert.Groups) != 1 || alert.Groups[0] != "platform-team" {
		t.Errorf("expected Groups=['platform-team'], got %v", alert.Groups)
	}
	if alert.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set")
	}
}

func TestGetIncidentError(t *testing.T) {
	defer setupTestEnv(t)()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: server.URL,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	_, err = client.GetIncident(context.Background(), "nonexistent", time.Now())
	if err == nil {
		t.Error("expected error for 404 response")
	}
}

func TestGetAlertError(t *testing.T) {
	defer setupTestEnv(t)()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: server.URL,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	_, err = client.GetAlert(context.Background(), "nonexistent", time.Now())
	if err == nil {
		t.Error("expected error for 404 response")
	}
}

func TestListIncidentsInvalidJSON(t *testing.T) {
	defer setupTestEnv(t)()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: server.URL,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	_, err = client.ListIncidents(context.Background(), 1, "")
	if err == nil {
		t.Error("expected error for invalid JSON response")
	}
}

func TestListAlertsInvalidJSON(t *testing.T) {
	defer setupTestEnv(t)()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: server.URL,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	_, err = client.ListAlerts(context.Background(), 1)
	if err == nil {
		t.Error("expected error for invalid JSON response")
	}
}

func TestGetIncidentInvalidJSON(t *testing.T) {
	defer setupTestEnv(t)()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: server.URL,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	_, err = client.GetIncident(context.Background(), "inc_123", time.Now())
	if err == nil {
		t.Error("expected error for invalid JSON response")
	}
}

func TestGetAlertInvalidJSON(t *testing.T) {
	defer setupTestEnv(t)()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: server.URL,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	_, err = client.GetAlert(context.Background(), "alert_123", time.Now())
	if err == nil {
		t.Error("expected error for invalid JSON response")
	}
}

func TestValidateAPIKeyHTTPError(t *testing.T) {
	defer setupTestEnv(t)()

	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: "http://invalid.nonexistent.host:99999",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	err = client.ValidateAPIKey(context.Background())
	if err == nil {
		t.Error("expected error for unreachable host")
	}
}

func TestListIncidentsHTTPError(t *testing.T) {
	defer setupTestEnv(t)()

	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: "http://invalid.nonexistent.host:99999",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	_, err = client.ListIncidents(context.Background(), 1, "")
	if err == nil {
		t.Error("expected error for unreachable host")
	}
}

func TestListAlertsHTTPError(t *testing.T) {
	defer setupTestEnv(t)()

	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: "http://invalid.nonexistent.host:99999",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	_, err = client.ListAlerts(context.Background(), 1)
	if err == nil {
		t.Error("expected error for unreachable host")
	}
}

func TestGetIncidentHTTPError(t *testing.T) {
	defer setupTestEnv(t)()

	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: "http://invalid.nonexistent.host:99999",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	_, err = client.GetIncident(context.Background(), "inc_123", time.Now())
	if err == nil {
		t.Error("expected error for unreachable host")
	}
}

func TestGetAlertHTTPError(t *testing.T) {
	defer setupTestEnv(t)()

	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: "http://invalid.nonexistent.host:99999",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	_, err = client.GetAlert(context.Background(), "alert_123", time.Now())
	if err == nil {
		t.Error("expected error for unreachable host")
	}
}

func TestListIncidentsWithPagination(t *testing.T) {
	defer setupTestEnv(t)()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check pagination query params
		pageNum := r.URL.Query().Get("page[number]")

		if pageNum != "2" {
			t.Errorf("expected page[number]=2, got %s", pageNum)
		}

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)

		response := map[string]interface{}{
			"data": []map[string]interface{}{},
			"meta": map[string]interface{}{
				"current_page": 2,
			},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: server.URL,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	result, err := client.ListIncidents(context.Background(), 2, "")
	if err != nil {
		t.Fatalf("ListIncidents() error = %v", err)
	}

	if result.Pagination.CurrentPage != 2 {
		t.Errorf("expected CurrentPage=2, got %d", result.Pagination.CurrentPage)
	}
}

func TestListAlertsWithPagination(t *testing.T) {
	defer setupTestEnv(t)()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)

		response := map[string]interface{}{
			"data": []map[string]interface{}{},
			"meta": map[string]interface{}{
				"current_page": 3,
				"total_pages":  10,
				"total_count":  500,
			},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: server.URL,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	result, err := client.ListAlerts(context.Background(), 3)
	if err != nil {
		t.Fatalf("ListAlerts() error = %v", err)
	}

	if result.Pagination.CurrentPage != 3 {
		t.Errorf("expected CurrentPage=3, got %d", result.Pagination.CurrentPage)
	}
}

func TestIncidentsWithEmptyData(t *testing.T) {
	defer setupTestEnv(t)()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)

		response := map[string]interface{}{
			"data": []map[string]interface{}{
				{
					"id": "inc_empty",
					"attributes": map[string]interface{}{
						"title":      "Minimal Incident",
						"status":     "started",
						"created_at": "2025-01-01T10:00:00Z",
						// Missing optional fields: summary, severity, timestamps, etc.
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: server.URL,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	result, err := client.ListIncidents(context.Background(), 1, "")
	if err != nil {
		t.Fatalf("ListIncidents() error = %v", err)
	}

	if len(result.Incidents) != 1 {
		t.Fatalf("expected 1 incident, got %d", len(result.Incidents))
	}

	inc := result.Incidents[0]
	if inc.ID != "inc_empty" {
		t.Errorf("expected ID=inc_empty, got %s", inc.ID)
	}
	// Verify nil optional fields don't cause issues
	if inc.StartedAt != nil {
		t.Error("expected StartedAt to be nil")
	}
	if inc.ResolvedAt != nil {
		t.Error("expected ResolvedAt to be nil")
	}
}

func TestAlertsWithEmptyData(t *testing.T) {
	defer setupTestEnv(t)()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)

		response := map[string]interface{}{
			"data": []map[string]interface{}{
				{
					"id": "alert_empty",
					"attributes": map[string]interface{}{
						"summary":    "Minimal Alert",
						"status":     "triggered",
						"source":     "custom",
						"created_at": "2025-01-01T10:00:00Z",
						// Missing optional fields: description, labels, services, etc.
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	cfg := &config.Config{
		APIKey:   "test-key",
		Endpoint: server.URL,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	result, err := client.ListAlerts(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListAlerts() error = %v", err)
	}

	if len(result.Alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(result.Alerts))
	}

	alert := result.Alerts[0]
	if alert.ID != "alert_empty" {
		t.Errorf("expected ID=alert_empty, got %s", alert.ID)
	}
	if len(alert.Labels) != 0 {
		t.Errorf("expected empty labels, got %d", len(alert.Labels))
	}
	if len(alert.Services) != 0 {
		t.Errorf("expected empty services, got %d", len(alert.Services))
	}
}

func TestIncidentDurationMethods(t *testing.T) {
	baseTime := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		incident Incident
		method   func(Incident) float64
		expected float64
	}{
		{
			name: "TimeToDetection - 1 hour",
			incident: Incident{
				StartedAt:  ptrTime(baseTime),
				DetectedAt: ptrTime(baseTime.Add(1 * time.Hour)),
			},
			method:   func(i Incident) float64 { return i.TimeToDetection() },
			expected: 1.0,
		},
		{
			name: "TimeToDetection - no detected_at",
			incident: Incident{
				StartedAt: ptrTime(baseTime),
			},
			method:   func(i Incident) float64 { return i.TimeToDetection() },
			expected: 0,
		},
		{
			name: "TimeToAcknowledge - 30 minutes",
			incident: Incident{
				StartedAt:      ptrTime(baseTime),
				AcknowledgedAt: ptrTime(baseTime.Add(30 * time.Minute)),
			},
			method:   func(i Incident) float64 { return i.TimeToAcknowledge() },
			expected: 0.5,
		},
		{
			name: "TimeToMitigation - 2 hours",
			incident: Incident{
				StartedAt:   ptrTime(baseTime),
				MitigatedAt: ptrTime(baseTime.Add(2 * time.Hour)),
			},
			method:   func(i Incident) float64 { return i.TimeToMitigation() },
			expected: 2.0,
		},
		{
			name: "TimeToResolution - 3.5 hours",
			incident: Incident{
				StartedAt:  ptrTime(baseTime),
				ResolvedAt: ptrTime(baseTime.Add(3*time.Hour + 30*time.Minute)),
			},
			method:   func(i Incident) float64 { return i.TimeToResolution() },
			expected: 3.5,
		},
		{
			name: "TimeToClose - 5 hours",
			incident: Incident{
				StartedAt: ptrTime(baseTime),
				ClosedAt:  ptrTime(baseTime.Add(5 * time.Hour)),
			},
			method:   func(i Incident) float64 { return i.TimeToClose() },
			expected: 5.0,
		},
		{
			name: "TimeToTriage - 15 minutes",
			incident: Incident{
				InTriageAt: ptrTime(baseTime),
				StartedAt:  ptrTime(baseTime.Add(15 * time.Minute)),
			},
			method:   func(i Incident) float64 { return i.TimeToTriage() },
			expected: 0.25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.method(tt.incident)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIncidentDuration(t *testing.T) {
	baseTime := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	t.Run("Duration with resolved_at - 1 hour", func(t *testing.T) {
		incident := Incident{
			StartedAt:  ptrTime(baseTime),
			ResolvedAt: ptrTime(baseTime.Add(1 * time.Hour)),
		}
		result := incident.Duration()
		if result != 3600 {
			t.Errorf("expected 3600, got %v", result)
		}
	})

	t.Run("Duration no start time returns 0", func(t *testing.T) {
		incident := Incident{
			ResolvedAt: ptrTime(baseTime.Add(1 * time.Hour)),
		}
		result := incident.Duration()
		if result != 0 {
			t.Errorf("expected 0, got %v", result)
		}
	})

	t.Run("Duration with cancelled_at", func(t *testing.T) {
		incident := Incident{
			StartedAt:   ptrTime(baseTime),
			CancelledAt: ptrTime(baseTime.Add(2 * time.Hour)),
		}
		result := incident.Duration()
		if result != 7200 {
			t.Errorf("expected 7200, got %v", result)
		}
	})
}

func TestIncidentMaintenanceDuration(t *testing.T) {
	baseTime := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		incident Incident
		expected int64
	}{
		{
			name: "Maintenance duration - 2 hours",
			incident: Incident{
				ScheduledFor:   ptrTime(baseTime),
				ScheduledUntil: ptrTime(baseTime.Add(2 * time.Hour)),
			},
			expected: 7200,
		},
		{
			name: "No scheduled_until",
			incident: Incident{
				ScheduledFor: ptrTime(baseTime),
			},
			expected: 0,
		},
		{
			name: "No scheduled_for",
			incident: Incident{
				ScheduledUntil: ptrTime(baseTime.Add(2 * time.Hour)),
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.incident.MaintenanceDuration()
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIncidentInTriageDuration(t *testing.T) {
	baseTime := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	t.Run("In triage duration - 45 minutes", func(t *testing.T) {
		incident := Incident{
			InTriageAt: ptrTime(baseTime),
			StartedAt:  ptrTime(baseTime.Add(45 * time.Minute)),
		}
		result := incident.InTriageDuration()
		if result != 2700 {
			t.Errorf("expected 2700, got %v", result)
		}
	})

	t.Run("No in_triage_at returns 0", func(t *testing.T) {
		incident := Incident{
			StartedAt: ptrTime(baseTime.Add(45 * time.Minute)),
		}
		result := incident.InTriageDuration()
		if result != 0 {
			t.Errorf("expected 0, got %v", result)
		}
	})
}

func TestTruncateToTwoDecimals(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{1.234567, 1.23},
		{1.999, 1.99},
		{2.0, 2.0},
		{0.125, 0.12},
		{10.555, 10.55},
	}

	for _, tt := range tests {
		result := truncateToTwoDecimals(tt.input)
		if result != tt.expected {
			t.Errorf("truncateToTwoDecimals(%v) = %v, expected %v", tt.input, result, tt.expected)
		}
	}
}

// ptrTime is a helper to create a pointer to a time.Time
func ptrTime(t time.Time) *time.Time {
	return &t
}
