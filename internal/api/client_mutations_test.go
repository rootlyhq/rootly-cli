package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- UpdateIncident ---

func TestUpdateIncident(t *testing.T) {
	var receivedMethod string
	var receivedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)

		if !strings.Contains(r.URL.Path, "/v1/incidents/inc-1") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"id": "inc-1",
				"attributes": map[string]interface{}{
					"sequential_id": 42,
					"title":         "Updated Title",
					"status":        "mitigated",
					"kind":          "normal",
					"created_at":    "2025-06-15T10:00:00Z",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	inc, err := client.UpdateIncident(context.Background(), "inc-1", map[string]string{
		"title":  "Updated Title",
		"status": "mitigated",
	})
	if err != nil {
		t.Fatalf("UpdateIncident returned error: %v", err)
	}

	if receivedMethod != "PUT" {
		t.Errorf("method = %q, want PUT", receivedMethod)
	}
	if inc.Title != "Updated Title" {
		t.Errorf("Title = %q, want %q", inc.Title, "Updated Title")
	}
	if inc.SequentialID != "INC-42" {
		t.Errorf("SequentialID = %q, want %q", inc.SequentialID, "INC-42")
	}

	// Verify request body structure
	data := receivedBody["data"].(map[string]interface{})
	attrs := data["attributes"].(map[string]interface{})
	if attrs["title"] != "Updated Title" {
		t.Errorf("request title = %q, want %q", attrs["title"], "Updated Title")
	}
	if attrs["status"] != "mitigated" {
		t.Errorf("request status = %q, want %q", attrs["status"], "mitigated")
	}
}

func TestUpdateIncidentNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.UpdateIncident(context.Background(), "nonexistent", map[string]string{"title": "x"})
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "incident not found") {
		t.Errorf("error = %q, want 'incident not found'", err.Error())
	}
}

func TestUpdateIncidentForbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.UpdateIncident(context.Background(), "inc-1", map[string]string{"title": "x"})
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Errorf("error = %q, want 'access denied'", err.Error())
	}
}

// --- CreateAlertCLI ---

func TestCreateAlertCLI(t *testing.T) {
	var receivedMethod string
	var receivedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)

		if !strings.Contains(r.URL.Path, "/v1/alerts") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusCreated)
		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"id": "alert-new",
				"attributes": map[string]interface{}{
					"summary":    "New Alert",
					"status":     "triggered",
					"source":     "datadog",
					"created_at": "2025-06-15T12:00:00Z",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	alert, err := client.CreateAlertCLI(context.Background(), "New Alert", map[string]string{
		"source":      "datadog",
		"description": "High CPU",
	})
	if err != nil {
		t.Fatalf("CreateAlertCLI returned error: %v", err)
	}

	if receivedMethod != "POST" {
		t.Errorf("method = %q, want POST", receivedMethod)
	}
	if alert.Summary != "New Alert" {
		t.Errorf("Summary = %q, want %q", alert.Summary, "New Alert")
	}
	if alert.Status != "triggered" {
		t.Errorf("Status = %q, want %q", alert.Status, "triggered")
	}

	// Verify request body
	data := receivedBody["data"].(map[string]interface{})
	attrs := data["attributes"].(map[string]interface{})
	if attrs["summary"] != "New Alert" {
		t.Errorf("request summary = %q, want %q", attrs["summary"], "New Alert")
	}
	if attrs["source"] != "datadog" {
		t.Errorf("request source = %q, want %q", attrs["source"], "datadog")
	}
}

func TestCreateAlertCLIUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.CreateAlertCLI(context.Background(), "Test", nil)
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "invalid API token") {
		t.Errorf("error = %q, want 'invalid API token'", err.Error())
	}
}

// --- UpdateAlertCLI ---

func TestUpdateAlertCLI(t *testing.T) {
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		if !strings.Contains(r.URL.Path, "/v1/alerts/alert-1") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"id": "alert-1",
				"attributes": map[string]interface{}{
					"summary":    "Updated Alert",
					"status":     "acknowledged",
					"created_at": "2025-06-15T10:00:00Z",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	alert, err := client.UpdateAlertCLI(context.Background(), "alert-1", map[string]string{
		"summary": "Updated Alert",
		"status":  "acknowledged",
	})
	if err != nil {
		t.Fatalf("UpdateAlertCLI returned error: %v", err)
	}

	if receivedMethod != "PUT" {
		t.Errorf("method = %q, want PUT", receivedMethod)
	}
	if alert.Summary != "Updated Alert" {
		t.Errorf("Summary = %q, want %q", alert.Summary, "Updated Alert")
	}
}

func TestUpdateAlertCLINotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.UpdateAlertCLI(context.Background(), "nonexistent", map[string]string{"summary": "x"})
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "alert not found") {
		t.Errorf("error = %q, want 'alert not found'", err.Error())
	}
}

func TestUpdateAlertCLIForbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.UpdateAlertCLI(context.Background(), "alert-1", map[string]string{"summary": "x"})
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Errorf("error = %q, want 'access denied'", err.Error())
	}
}

// --- AcknowledgeAlertCLI ---

func TestAcknowledgeAlertCLI(t *testing.T) {
	var receivedMethod, receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.AcknowledgeAlertCLI(context.Background(), "alert-1")
	if err != nil {
		t.Fatalf("AcknowledgeAlertCLI returned error: %v", err)
	}

	if receivedMethod != "POST" {
		t.Errorf("method = %q, want POST", receivedMethod)
	}
	if !strings.Contains(receivedPath, "/v1/alerts/alert-1/acknowledge") {
		t.Errorf("path = %q, want to contain '/v1/alerts/alert-1/acknowledge'", receivedPath)
	}
}

func TestAcknowledgeAlertCLINotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.AcknowledgeAlertCLI(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "alert not found") {
		t.Errorf("error = %q, want 'alert not found'", err.Error())
	}
}

func TestAcknowledgeAlertCLIUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.AcknowledgeAlertCLI(context.Background(), "alert-1")
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "invalid API token") {
		t.Errorf("error = %q, want 'invalid API token'", err.Error())
	}
}

// --- ResolveAlertCLI ---

func TestResolveAlertCLINoBody(t *testing.T) {
	var receivedMethod, receivedPath string
	var receivedBodyBytes []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		receivedBodyBytes, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.ResolveAlertCLI(context.Background(), "alert-1", "", false)
	if err != nil {
		t.Fatalf("ResolveAlertCLI returned error: %v", err)
	}

	if receivedMethod != "POST" {
		t.Errorf("method = %q, want POST", receivedMethod)
	}
	if !strings.Contains(receivedPath, "/v1/alerts/alert-1/resolve") {
		t.Errorf("path = %q, want to contain '/v1/alerts/alert-1/resolve'", receivedPath)
	}
	if len(receivedBodyBytes) != 0 {
		t.Errorf("expected empty body, got %q", string(receivedBodyBytes))
	}
}

func TestResolveAlertCLIWithBody(t *testing.T) {
	var receivedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.ResolveAlertCLI(context.Background(), "alert-1", "Fixed the issue", true)
	if err != nil {
		t.Fatalf("ResolveAlertCLI returned error: %v", err)
	}

	// Verify body was sent with resolution_message and resolve_related_incidents
	data := receivedBody["data"].(map[string]interface{})
	attrs := data["attributes"].(map[string]interface{})
	if attrs["resolution_message"] != "Fixed the issue" {
		t.Errorf("resolution_message = %q, want %q", attrs["resolution_message"], "Fixed the issue")
	}
	if attrs["resolve_related_incidents"] != true {
		t.Errorf("resolve_related_incidents = %v, want true", attrs["resolve_related_incidents"])
	}
}

func TestResolveAlertCLINotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.ResolveAlertCLI(context.Background(), "nonexistent", "", false)
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "alert not found") {
		t.Errorf("error = %q, want 'alert not found'", err.Error())
	}
}

func TestResolveAlertCLIForbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.ResolveAlertCLI(context.Background(), "alert-1", "", false)
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Errorf("error = %q, want 'access denied'", err.Error())
	}
}

// --- CreateService ---

func TestCreateService(t *testing.T) {
	var receivedMethod string
	var receivedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusCreated)
		desc := "A test service"
		color := "#ff0000"
		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"id": "svc-new",
				"attributes": map[string]interface{}{
					"name":        "API Gateway",
					"slug":        "api-gateway",
					"description": desc,
					"color":       color,
					"created_at":  "2025-06-15T12:00:00Z",
					"updated_at":  "2025-06-15T12:00:00Z",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	svc, err := client.CreateService(context.Background(), "API Gateway", map[string]string{
		"description": "A test service",
		"color":       "#ff0000",
	})
	if err != nil {
		t.Fatalf("CreateService returned error: %v", err)
	}

	if receivedMethod != "POST" {
		t.Errorf("method = %q, want POST", receivedMethod)
	}
	if svc.Name != "API Gateway" {
		t.Errorf("Name = %q, want %q", svc.Name, "API Gateway")
	}
	if svc.Slug != "api-gateway" {
		t.Errorf("Slug = %q, want %q", svc.Slug, "api-gateway")
	}
	if svc.Description != "A test service" {
		t.Errorf("Description = %q, want %q", svc.Description, "A test service")
	}
	if svc.Color != "#ff0000" {
		t.Errorf("Color = %q, want %q", svc.Color, "#ff0000")
	}

	// Verify request body
	data := receivedBody["data"].(map[string]interface{})
	attrs := data["attributes"].(map[string]interface{})
	if attrs["name"] != "API Gateway" {
		t.Errorf("request name = %q, want %q", attrs["name"], "API Gateway")
	}
}

func TestCreateServiceUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.CreateService(context.Background(), "Test", nil)
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "invalid API token") {
		t.Errorf("error = %q, want 'invalid API token'", err.Error())
	}
}

func TestCreateServiceForbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.CreateService(context.Background(), "Test", nil)
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Errorf("error = %q, want 'access denied'", err.Error())
	}
}

// --- UpdateService ---

func TestUpdateService(t *testing.T) {
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		if !strings.Contains(r.URL.Path, "/v1/services/svc-1") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"id": "svc-1",
				"attributes": map[string]interface{}{
					"name":       "Updated Service",
					"slug":       "updated-service",
					"created_at": "2025-06-15T10:00:00Z",
					"updated_at": "2025-06-15T14:00:00Z",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	svc, err := client.UpdateService(context.Background(), "svc-1", map[string]string{
		"name": "Updated Service",
	})
	if err != nil {
		t.Fatalf("UpdateService returned error: %v", err)
	}

	if receivedMethod != "PUT" {
		t.Errorf("method = %q, want PUT", receivedMethod)
	}
	if svc.Name != "Updated Service" {
		t.Errorf("Name = %q, want %q", svc.Name, "Updated Service")
	}
}

func TestUpdateServiceNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.UpdateService(context.Background(), "nonexistent", map[string]string{"name": "x"})
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "service not found") {
		t.Errorf("error = %q, want 'service not found'", err.Error())
	}
}

// --- CreateTeam ---

func TestCreateTeam(t *testing.T) {
	var receivedMethod string
	var receivedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusCreated)
		desc := "The platform team"
		color := "#00ff00"
		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"id": "team-new",
				"attributes": map[string]interface{}{
					"name":        "Platform",
					"slug":        "platform",
					"description": desc,
					"color":       color,
					"created_at":  "2025-06-15T12:00:00Z",
					"updated_at":  "2025-06-15T12:00:00Z",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	team, err := client.CreateTeam(context.Background(), "Platform", map[string]string{
		"description": "The platform team",
		"color":       "#00ff00",
	})
	if err != nil {
		t.Fatalf("CreateTeam returned error: %v", err)
	}

	if receivedMethod != "POST" {
		t.Errorf("method = %q, want POST", receivedMethod)
	}
	if team.Name != "Platform" {
		t.Errorf("Name = %q, want %q", team.Name, "Platform")
	}
	if team.Slug != "platform" {
		t.Errorf("Slug = %q, want %q", team.Slug, "platform")
	}
	if team.Description != "The platform team" {
		t.Errorf("Description = %q, want %q", team.Description, "The platform team")
	}
	if team.Color != "#00ff00" {
		t.Errorf("Color = %q, want %q", team.Color, "#00ff00")
	}
}

func TestCreateTeamUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.CreateTeam(context.Background(), "Test", nil)
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "invalid API token") {
		t.Errorf("error = %q, want 'invalid API token'", err.Error())
	}
}

// --- UpdateTeam ---

func TestUpdateTeam(t *testing.T) {
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		if !strings.Contains(r.URL.Path, "/v1/teams/team-1") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"id": "team-1",
				"attributes": map[string]interface{}{
					"name":       "Updated Team",
					"slug":       "updated-team",
					"created_at": "2025-06-15T10:00:00Z",
					"updated_at": "2025-06-15T14:00:00Z",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	team, err := client.UpdateTeam(context.Background(), "team-1", map[string]string{
		"name": "Updated Team",
	})
	if err != nil {
		t.Fatalf("UpdateTeam returned error: %v", err)
	}

	// UpdateTeam uses PATCH, not PUT
	if receivedMethod != "PATCH" {
		t.Errorf("method = %q, want PATCH", receivedMethod)
	}
	if team.Name != "Updated Team" {
		t.Errorf("Name = %q, want %q", team.Name, "Updated Team")
	}
}

func TestUpdateTeamNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.UpdateTeam(context.Background(), "nonexistent", map[string]string{"name": "x"})
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "team not found") {
		t.Errorf("error = %q, want 'team not found'", err.Error())
	}
}

func TestUpdateTeamForbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.UpdateTeam(context.Background(), "team-1", map[string]string{"name": "x"})
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Errorf("error = %q, want 'access denied'", err.Error())
	}
}
