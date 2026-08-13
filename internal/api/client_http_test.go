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

		var requestBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		data := requestBody["data"].(map[string]interface{})
		attrs := data["attributes"].(map[string]interface{})
		for key, want := range map[string]string{
			"service_ids":       "api-gateway",
			"incident_type_ids": "customer-impacting",
			"functionality_ids": "checkout",
			"environment_ids":   "production",
			"group_ids":         "platform",
			"cause_ids":         "deployment",
		} {
			values := attrs[key].([]interface{})
			if len(values) == 0 || values[0] != want {
				t.Errorf("request %s = %v, want first value %s", key, values, want)
			}
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
	inc, err := client.CreateIncident(context.Background(), "New Incident", map[string]interface{}{
		"summary":           "Test summary",
		"status":            "started",
		"service_ids":       []string{"api-gateway", "payments"},
		"incident_type_ids": []string{"customer-impacting"},
		"functionality_ids": []string{"checkout"},
		"environment_ids":   []string{"production"},
		"group_ids":         []string{"platform"},
		"cause_ids":         []string{"deployment"},
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

// --- GetIncidentByID with full detail relationships ---

// fullDetailIncidentServer returns a test server that responds with a fully-populated incident.
func fullDetailIncidentServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"id": "inc-full",
				"attributes": map[string]interface{}{
					"sequential_id":      55,
					"title":              "Full Detail Incident",
					"summary":            "Everything populated",
					"status":             "resolved",
					"kind":               "normal",
					"private":            true,
					"created_at":         "2025-06-15T10:00:00Z",
					"updated_at":         "2025-06-15T14:00:00Z",
					"started_at":         "2025-06-15T10:05:00Z",
					"resolved_at":        "2025-06-15T13:00:00Z",
					"mitigated_at":       "2025-06-15T12:00:00Z",
					"url":                "https://rootly.com/incidents/55",
					"short_url":          "https://rootly.io/i/55",
					"source":             "datadog",
					"mitigation_message": "Restarted DB replicas",
					"resolution_message": "Root cause: OOM killer",
					"slack_channel_name": "inc-55-full-detail",
					"slack_channel_url":  "https://slack.com/channel/inc-55",
					"jira_issue_url":     "https://jira.com/PROJ-123",
					"github_issue_url":   "https://github.com/org/repo/issues/42",
					"labels": []map[string]interface{}{
						{"key": "env", "value": "production"},
						{"key": "region", "value": "us-east-1"},
					},
					"severity": map[string]interface{}{
						"data": map[string]interface{}{
							"attributes": map[string]interface{}{"name": "sev0"},
						},
					},
					"commander": map[string]interface{}{
						"data": map[string]interface{}{
							"attributes": map[string]interface{}{"name": "Alice Commander"},
						},
					},
					"communicator": map[string]interface{}{
						"data": map[string]interface{}{
							"attributes": map[string]interface{}{"name": "Bob Communicator"},
						},
					},
					"user": map[string]interface{}{
						"data": map[string]interface{}{
							"attributes": map[string]interface{}{"full_name": "Charlie Creator", "email": "charlie@example.com"},
						},
					},
					"started_by": map[string]interface{}{
						"data": map[string]interface{}{
							"attributes": map[string]interface{}{"full_name": "Dave Starter", "email": "dave@example.com"},
						},
					},
					"mitigated_by": map[string]interface{}{
						"data": map[string]interface{}{
							"attributes": map[string]interface{}{"full_name": "Eve Mitigator", "email": "eve@example.com"},
						},
					},
					"resolved_by": map[string]interface{}{
						"data": map[string]interface{}{
							"attributes": map[string]interface{}{"full_name": "Frank Resolver", "email": "frank@example.com"},
						},
					},
					"roles": []map[string]interface{}{
						{"attributes": map[string]interface{}{
							"name": "Scribe",
							"user": map[string]interface{}{
								"data": map[string]interface{}{
									"attributes": map[string]interface{}{"full_name": "Grace Scribe", "email": "grace@example.com"},
								},
							},
						}},
					},
					"causes": []map[string]interface{}{
						{"attributes": map[string]interface{}{"name": "Memory leak"}},
						{"attributes": map[string]interface{}{"name": "Config drift"}},
					},
					"incident_types": []map[string]interface{}{
						{"attributes": map[string]interface{}{"name": "Infrastructure"}},
					},
					"functionalities": []map[string]interface{}{
						{"attributes": map[string]interface{}{"name": "API Requests"}},
					},
					"services": map[string]interface{}{
						"data": []map[string]interface{}{
							{"attributes": map[string]interface{}{"name": "api-gateway"}},
							{"attributes": map[string]interface{}{"name": "auth-service"}},
						},
					},
					"environments": map[string]interface{}{
						"data": []map[string]interface{}{
							{"attributes": map[string]interface{}{"name": "production"}},
						},
					},
					"groups": map[string]interface{}{
						"data": []map[string]interface{}{
							{"attributes": map[string]interface{}{"name": "Platform"}},
						},
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func TestGetIncidentByIDDetailBasicFields(t *testing.T) {
	server := fullDetailIncidentServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	inc, err := client.GetIncidentByID(context.Background(), "inc-full")
	if err != nil {
		t.Fatalf("returned error: %v", err)
	}

	if inc.SequentialID != "INC-55" {
		t.Errorf("SequentialID = %q, want %q", inc.SequentialID, "INC-55")
	}
	if inc.Severity != "sev0" {
		t.Errorf("Severity = %q, want %q", inc.Severity, "sev0")
	}
	if !inc.Private {
		t.Error("Private = false, want true")
	}
	if !inc.DetailLoaded {
		t.Error("DetailLoaded = false, want true")
	}
	if inc.URL != "https://rootly.com/incidents/55" {
		t.Errorf("URL = %q", inc.URL)
	}
	if inc.ShortURL != "https://rootly.io/i/55" {
		t.Errorf("ShortURL = %q", inc.ShortURL)
	}
	if inc.Source != "datadog" {
		t.Errorf("Source = %q", inc.Source)
	}
	if inc.MitigationMessage != "Restarted DB replicas" {
		t.Errorf("MitigationMessage = %q", inc.MitigationMessage)
	}
	if inc.ResolutionMessage != "Root cause: OOM killer" {
		t.Errorf("ResolutionMessage = %q", inc.ResolutionMessage)
	}
}

func TestGetIncidentByIDDetailLinks(t *testing.T) {
	server := fullDetailIncidentServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	inc, err := client.GetIncidentByID(context.Background(), "inc-full")
	if err != nil {
		t.Fatalf("returned error: %v", err)
	}

	if inc.SlackChannelURL != "https://slack.com/channel/inc-55" {
		t.Errorf("SlackChannelURL = %q", inc.SlackChannelURL)
	}
	if inc.JiraIssueURL != "https://jira.com/PROJ-123" {
		t.Errorf("JiraIssueURL = %q", inc.JiraIssueURL)
	}
	if inc.GithubIssueURL != "https://github.com/org/repo/issues/42" {
		t.Errorf("GithubIssueURL = %q", inc.GithubIssueURL)
	}
	if inc.Labels["env"] != "production" {
		t.Errorf("Labels[env] = %q", inc.Labels["env"])
	}
	if inc.Labels["region"] != "us-east-1" {
		t.Errorf("Labels[region] = %q", inc.Labels["region"])
	}
}

func TestGetIncidentByIDDetailRelationships(t *testing.T) {
	server := fullDetailIncidentServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	inc, err := client.GetIncidentByID(context.Background(), "inc-full")
	if err != nil {
		t.Fatalf("returned error: %v", err)
	}

	if inc.CommanderName != "Alice Commander" {
		t.Errorf("CommanderName = %q", inc.CommanderName)
	}
	if inc.CommunicatorName != "Bob Communicator" {
		t.Errorf("CommunicatorName = %q", inc.CommunicatorName)
	}
	if inc.CreatedByName != "Charlie Creator" {
		t.Errorf("CreatedByName = %q", inc.CreatedByName)
	}
	if inc.CreatedByEmail != "charlie@example.com" {
		t.Errorf("CreatedByEmail = %q", inc.CreatedByEmail)
	}
	if inc.StartedByName != "Dave Starter" {
		t.Errorf("StartedByName = %q", inc.StartedByName)
	}
	if inc.MitigatedByName != "Eve Mitigator" {
		t.Errorf("MitigatedByName = %q", inc.MitigatedByName)
	}
	if inc.ResolvedByName != "Frank Resolver" {
		t.Errorf("ResolvedByName = %q", inc.ResolvedByName)
	}
}

func TestGetIncidentByIDDetailCollections(t *testing.T) {
	server := fullDetailIncidentServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	inc, err := client.GetIncidentByID(context.Background(), "inc-full")
	if err != nil {
		t.Fatalf("returned error: %v", err)
	}

	// Roles
	if len(inc.Roles) != 1 {
		t.Fatalf("Roles count = %d, want 1", len(inc.Roles))
	}
	if inc.Roles[0].Name != "Scribe" || inc.Roles[0].UserName != "Grace Scribe" {
		t.Errorf("Roles[0] = {%q, %q}", inc.Roles[0].Name, inc.Roles[0].UserName)
	}
	if len(inc.Causes) != 2 || inc.Causes[0] != "Memory leak" {
		t.Errorf("Causes = %v", inc.Causes)
	}
	if len(inc.IncidentTypes) != 1 || inc.IncidentTypes[0] != "Infrastructure" {
		t.Errorf("IncidentTypes = %v", inc.IncidentTypes)
	}
	if len(inc.Functionalities) != 1 || inc.Functionalities[0] != "API Requests" {
		t.Errorf("Functionalities = %v", inc.Functionalities)
	}
	if len(inc.Services) != 2 || inc.Services[0] != "api-gateway" {
		t.Errorf("Services = %v", inc.Services)
	}
	if len(inc.Environments) != 1 || inc.Environments[0] != "production" {
		t.Errorf("Environments = %v", inc.Environments)
	}
	if len(inc.Teams) != 1 || inc.Teams[0] != "Platform" {
		t.Errorf("Teams = %v", inc.Teams)
	}
	if inc.StartedAt == nil || inc.ResolvedAt == nil || inc.MitigatedAt == nil {
		t.Error("timestamps should be non-nil")
	}
}
