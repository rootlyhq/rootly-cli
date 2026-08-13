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

func TestListWorkflowsCLI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if got := r.URL.Query().Get("filter[slug]"); got != "retrospective" {
			t.Errorf("slug filter = %q, want retrospective", got)
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		_, _ = w.Write([]byte(`{
			"data": [{
				"id": "workflow-1",
				"attributes": {
					"name": "Create retrospective",
					"slug": "retrospective",
					"description": "Create the retrospective document",
					"enabled": true,
					"created_at": "2026-08-12T12:00:00Z",
					"updated_at": "2026-08-12T12:00:00Z"
				}
			}],
			"meta": {"current_page": 1, "total_pages": 1, "total_count": 1}
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.ListWorkflowsCLI(context.Background(), 1, 25, "-created_at", map[string]string{"slug": "retrospective"})
	if err != nil {
		t.Fatalf("ListWorkflowsCLI returned error: %v", err)
	}
	if len(result.Workflows) != 1 || result.Workflows[0].Slug != "retrospective" {
		t.Fatalf("workflows = %+v, want retrospective", result.Workflows)
	}
	if !result.Workflows[0].Enabled {
		t.Error("workflow should be enabled")
	}
}

func TestListWorkflowsCLIRejectsNegativePagination(t *testing.T) {
	client := &Client{}
	if _, err := client.ListWorkflowsCLI(context.Background(), -1, 25, "", nil); err == nil || !strings.Contains(err.Error(), "page must be at least 1") {
		t.Fatalf("page error = %v, want minimum page validation", err)
	}
	if _, err := client.ListWorkflowsCLI(context.Background(), 1, -1, "", nil); err == nil || !strings.Contains(err.Error(), "page size must not be negative") {
		t.Fatalf("page-size error = %v, want non-negative validation", err)
	}
}

func TestResolveWorkflowIDBySlug(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("filter[slug][eq]"); got != "retrospective" {
			t.Errorf("exact slug filter = %q, want retrospective", got)
		}
		_, _ = w.Write([]byte(`{
			"data": [{"id": "workflow-1", "attributes": {"name": "Retrospective", "slug": "retrospective"}}],
			"meta": {"current_page": 1, "total_pages": 1, "total_count": 1}
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	id, err := client.ResolveWorkflowID(context.Background(), "retrospective")
	if err != nil {
		t.Fatalf("ResolveWorkflowID returned error: %v", err)
	}
	if id != "workflow-1" {
		t.Errorf("id = %q, want workflow-1", id)
	}
}

func TestResolveWorkflowIDSkipsLookupForUUID(t *testing.T) {
	client := &Client{}
	id := "e5923856-6fe8-4a2c-b0eb-cb783e811d06"
	got, err := client.ResolveWorkflowID(context.Background(), id)
	if err != nil {
		t.Fatalf("ResolveWorkflowID returned error: %v", err)
	}
	if got != id {
		t.Errorf("id = %q, want %q", got, id)
	}
}

func TestRunWorkflowCLI(t *testing.T) {
	var requestBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/v1/workflows/workflow-1/workflow_runs") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &requestBody)
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
			"data": {
				"id": "run-1",
				"attributes": {
					"workflow_id": "workflow-1",
					"incident_id": "42",
					"status": "pending",
					"triggered_by": "api"
				}
			}
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	immediate := false
	checkConditions := true
	run, err := client.RunWorkflowCLI(context.Background(), "workflow-1", WorkflowRunOpts{
		IncidentID:      "incident-uuid",
		Immediate:       &immediate,
		CheckConditions: &checkConditions,
	})
	if err != nil {
		t.Fatalf("RunWorkflowCLI returned error: %v", err)
	}
	attributes := requestBody["data"].(map[string]interface{})["attributes"].(map[string]interface{})
	if attributes["incident_id"] != "incident-uuid" {
		t.Errorf("incident_id = %v, want incident-uuid", attributes["incident_id"])
	}
	if attributes["immediate"] != false {
		t.Errorf("immediate = %v, want false", attributes["immediate"])
	}
	if attributes["check_conditions"] != true {
		t.Errorf("check_conditions = %v, want true", attributes["check_conditions"])
	}
	if run.ID != "run-1" || run.Status != "pending" {
		t.Errorf("run = %+v, want run-1 pending", run)
	}
}
