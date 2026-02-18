package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCreatePulseCLI_Success(t *testing.T) {
	var receivedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/pulses" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/vnd.api+json" {
			t.Errorf("unexpected content type: %s", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}

		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusCreated)
		resp := `{
			"data": {
				"id": "pulse-123",
				"attributes": {
					"summary": "Deploy v1.2.3",
					"source": "ci",
					"started_at": "2025-06-15T10:00:00Z",
					"ended_at": "2025-06-15T10:05:00Z"
				}
			}
		}`
		w.Write([]byte(resp))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	now := time.Now()
	opts := PulseOpts{
		Source:         "ci",
		ServiceIDs:     []string{"svc-1", "svc-2"},
		EnvironmentIDs: []string{"production"},
		Labels:         []KeyValue{{Key: "version", Value: "1.2.3"}},
		Refs:           []KeyValue{{Key: "commit", Value: "abc123"}},
		StartedAt:      &now,
		EndedAt:        &now,
	}

	pulse, err := client.CreatePulseCLI(context.Background(), "Deploy v1.2.3", opts)
	if err != nil {
		t.Fatalf("CreatePulseCLI returned error: %v", err)
	}

	if pulse.ID != "pulse-123" {
		t.Errorf("ID = %q, want %q", pulse.ID, "pulse-123")
	}
	if pulse.Summary != "Deploy v1.2.3" {
		t.Errorf("Summary = %q, want %q", pulse.Summary, "Deploy v1.2.3")
	}
	if pulse.Source != "ci" {
		t.Errorf("Source = %q, want %q", pulse.Source, "ci")
	}
	if pulse.RawBody == nil {
		t.Error("expected RawBody to be non-nil")
	}

	// Verify request body structure
	data := receivedBody["data"].(map[string]interface{})
	if data["type"] != "pulses" {
		t.Errorf("request type = %q, want %q", data["type"], "pulses")
	}
	attrs := data["attributes"].(map[string]interface{})
	if attrs["summary"] != "Deploy v1.2.3" {
		t.Errorf("request summary = %q, want %q", attrs["summary"], "Deploy v1.2.3")
	}
	if attrs["source"] != "ci" {
		t.Errorf("request source = %q, want %q", attrs["source"], "ci")
	}
}

func TestCreatePulseCLI_MinimalOpts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{
			"data": {
				"id": "pulse-456",
				"attributes": {
					"summary": "Simple pulse",
					"source": ""
				}
			}
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	pulse, err := client.CreatePulseCLI(context.Background(), "Simple pulse", PulseOpts{})
	if err != nil {
		t.Fatalf("CreatePulseCLI returned error: %v", err)
	}
	if pulse.ID != "pulse-456" {
		t.Errorf("ID = %q, want %q", pulse.ID, "pulse-456")
	}
}

func TestCreatePulseCLI_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"errors": [{"title": "Unauthorized"}]}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.CreatePulseCLI(context.Background(), "test", PulseOpts{})
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
	if err.Error() != "invalid API key" {
		t.Errorf("error = %q, want %q", err.Error(), "invalid API key")
	}
}

func TestCreatePulseCLI_Forbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"errors": [{"title": "Forbidden"}]}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.CreatePulseCLI(context.Background(), "test", PulseOpts{})
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
	if err.Error() != "access denied: API key lacks 'create pulses' permission" {
		t.Errorf("error = %q, want access denied message", err.Error())
	}
}

func TestCreatePulseCLI_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"errors": [{"title": "Server Error"}]}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.CreatePulseCLI(context.Background(), "test", PulseOpts{})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if err.Error() != "API returned status 500" {
		t.Errorf("error = %q, want API status message", err.Error())
	}
}

// --- JSON:API body construction tests (equivalent to old repo's TestConvertPulse) ---

func TestCreatePulseCLI_BodyMinimal(t *testing.T) {
	var receivedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"data":{"id":"p1","attributes":{"summary":"Hello"}}}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.CreatePulseCLI(context.Background(), "Hello", PulseOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := receivedBody["data"].(map[string]interface{})
	if data["type"] != "pulses" {
		t.Errorf("type = %q, want %q", data["type"], "pulses")
	}
	attrs := data["attributes"].(map[string]interface{})
	if attrs["summary"] != "Hello" {
		t.Errorf("summary = %q, want %q", attrs["summary"], "Hello")
	}
	// Minimal opts should not include optional fields
	if _, ok := attrs["service_ids"]; ok {
		t.Error("minimal pulse should not have service_ids")
	}
	if _, ok := attrs["environment_ids"]; ok {
		t.Error("minimal pulse should not have environment_ids")
	}
	if _, ok := attrs["labels"]; ok {
		t.Error("minimal pulse should not have labels")
	}
	if _, ok := attrs["refs"]; ok {
		t.Error("minimal pulse should not have refs")
	}
	if _, ok := attrs["started_at"]; ok {
		t.Error("minimal pulse should not have started_at")
	}
	if _, ok := attrs["ended_at"]; ok {
		t.Error("minimal pulse should not have ended_at")
	}
}

func TestCreatePulseCLI_BodyWithLabels(t *testing.T) {
	var receivedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"data":{"id":"p2","attributes":{"summary":"Deploy"}}}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.CreatePulseCLI(context.Background(), "Deploy", PulseOpts{
		Labels: []KeyValue{
			{Key: "platform", Value: "osx"},
			{Key: "exit_code", Value: "0"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	attrs := receivedBody["data"].(map[string]interface{})["attributes"].(map[string]interface{})
	labels, ok := attrs["labels"].(map[string]interface{})
	if !ok {
		t.Fatal("expected labels to be a map")
	}
	if labels["platform"] != "osx" {
		t.Errorf("labels[platform] = %q, want %q", labels["platform"], "osx")
	}
	if labels["exit_code"] != "0" {
		t.Errorf("labels[exit_code] = %q, want %q", labels["exit_code"], "0")
	}
}

func TestCreatePulseCLI_BodyWithTimestamps(t *testing.T) {
	var receivedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"data":{"id":"p3","attributes":{"summary":"Deploy"}}}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	startedAt := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	endedAt := time.Date(2025, 6, 15, 10, 5, 0, 0, time.UTC)
	_, err := client.CreatePulseCLI(context.Background(), "Deploy", PulseOpts{
		StartedAt: &startedAt,
		EndedAt:   &endedAt,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	attrs := receivedBody["data"].(map[string]interface{})["attributes"].(map[string]interface{})
	if attrs["started_at"] != "2025-06-15T10:00:00Z" {
		t.Errorf("started_at = %q, want %q", attrs["started_at"], "2025-06-15T10:00:00Z")
	}
	if attrs["ended_at"] != "2025-06-15T10:05:00Z" {
		t.Errorf("ended_at = %q, want %q", attrs["ended_at"], "2025-06-15T10:05:00Z")
	}
}

func TestCreatePulseCLI_BodyWithRefs(t *testing.T) {
	var receivedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"data":{"id":"p4","attributes":{"summary":"Deploy"}}}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.CreatePulseCLI(context.Background(), "Deploy", PulseOpts{
		Refs: []KeyValue{
			{Key: "sha", Value: "abc123"},
			{Key: "image", Value: "registry.rootly.com/app:v1"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	attrs := receivedBody["data"].(map[string]interface{})["attributes"].(map[string]interface{})
	refs, ok := attrs["refs"].([]interface{})
	if !ok {
		t.Fatal("expected refs to be an array")
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
	ref0 := refs[0].(map[string]interface{})
	if ref0["name"] != "sha" || ref0["value"] != "abc123" {
		t.Errorf("ref[0] = %v, want name=sha value=abc123", ref0)
	}
	ref1 := refs[1].(map[string]interface{})
	if ref1["name"] != "image" || ref1["value"] != "registry.rootly.com/app:v1" {
		t.Errorf("ref[1] = %v, want name=image value=registry.rootly.com/app:v1", ref1)
	}
}

func TestCreatePulseCLI_BodyFull(t *testing.T) {
	var receivedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"data":{"id":"p5","attributes":{"summary":"Full deploy","source":"k8s"}}}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	startedAt := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	endedAt := time.Date(2025, 1, 1, 0, 5, 0, 0, time.UTC)
	_, err := client.CreatePulseCLI(context.Background(), "Full deploy", PulseOpts{
		Source:         "k8s",
		ServiceIDs:     []string{"elasticsearch-prod"},
		EnvironmentIDs: []string{"production", "staging"},
		Labels: []KeyValue{
			{Key: "platform", Value: "osx"},
			{Key: "exit_code", Value: "1"},
		},
		Refs: []KeyValue{
			{Key: "sha", Value: "cd62148"},
			{Key: "image", Value: "registry.rootly.com/rootly/my-service:cd6214"},
		},
		StartedAt: &startedAt,
		EndedAt:   &endedAt,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	attrs := receivedBody["data"].(map[string]interface{})["attributes"].(map[string]interface{})

	if attrs["summary"] != "Full deploy" {
		t.Errorf("summary = %q, want %q", attrs["summary"], "Full deploy")
	}
	if attrs["source"] != "k8s" {
		t.Errorf("source = %q, want %q", attrs["source"], "k8s")
	}

	serviceIDs := attrs["service_ids"].([]interface{})
	if len(serviceIDs) != 1 || serviceIDs[0] != "elasticsearch-prod" {
		t.Errorf("service_ids = %v, want [elasticsearch-prod]", serviceIDs)
	}

	envIDs := attrs["environment_ids"].([]interface{})
	if len(envIDs) != 2 || envIDs[0] != "production" || envIDs[1] != "staging" {
		t.Errorf("environment_ids = %v, want [production, staging]", envIDs)
	}

	labels := attrs["labels"].(map[string]interface{})
	if labels["platform"] != "osx" || labels["exit_code"] != "1" {
		t.Errorf("labels = %v, want platform=osx exit_code=1", labels)
	}

	refs := attrs["refs"].([]interface{})
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}

	if attrs["started_at"] != "2025-01-01T00:00:00Z" {
		t.Errorf("started_at = %q, want %q", attrs["started_at"], "2025-01-01T00:00:00Z")
	}
	if attrs["ended_at"] != "2025-01-01T00:05:00Z" {
		t.Errorf("ended_at = %q, want %q", attrs["ended_at"], "2025-01-01T00:05:00Z")
	}
}

func TestCreatePulseCLI_WithServicesAndEnvs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{
			"data": {
				"id": "pulse-789",
				"attributes": {
					"summary": "Deploy",
					"source": "ci",
					"services": {
						"data": [
							{"attributes": {"name": "API Gateway"}}
						]
					},
					"environments": {
						"data": [
							{"attributes": {"name": "production"}}
						]
					}
				}
			}
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	pulse, err := client.CreatePulseCLI(context.Background(), "Deploy", PulseOpts{
		Source:         "ci",
		ServiceIDs:     []string{"api-gateway"},
		EnvironmentIDs: []string{"production"},
	})
	if err != nil {
		t.Fatalf("CreatePulseCLI returned error: %v", err)
	}
	if len(pulse.Services) != 1 || pulse.Services[0] != "API Gateway" {
		t.Errorf("Services = %v, want [API Gateway]", pulse.Services)
	}
	if len(pulse.Environments) != 1 || pulse.Environments[0] != "production" {
		t.Errorf("Environments = %v, want [production]", pulse.Environments)
	}
}
