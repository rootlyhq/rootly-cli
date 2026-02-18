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
