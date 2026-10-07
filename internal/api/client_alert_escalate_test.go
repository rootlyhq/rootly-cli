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

func TestEscalateAlertCLIWithOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/alerts/alert-1/escalate" {
			t.Errorf("request = %s %s, want POST /v1/alerts/alert-1/escalate", r.Method, r.URL.Path)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request body: %v", err)
			return
		}
		data := body["data"].(map[string]interface{})
		if data["type"] != "alerts" {
			t.Errorf("data type = %v, want alerts", data["type"])
		}
		attributes := data["attributes"].(map[string]interface{})
		if attributes["escalation_policy_id"] != "policy-1" || attributes["escalation_policy_level"] != float64(2) {
			t.Errorf("attributes = %+v", attributes)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	policyID := "policy-1"
	level := 2
	if err := client.EscalateAlertCLI(context.Background(), "alert-1", &policyID, &level); err != nil {
		t.Fatalf("EscalateAlertCLI returned error: %v", err)
	}
}

func TestEscalateAlertCLIWithoutOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/alerts/alert-1/escalate" {
			t.Errorf("request = %s %s, want POST /v1/alerts/alert-1/escalate", r.Method, r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
		}
		var request struct {
			Data struct {
				Type       string                 `json:"type"`
				Attributes map[string]interface{} `json:"attributes"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if request.Data.Type != "alerts" || len(request.Data.Attributes) != 0 {
			t.Errorf("request = %+v, want alerts with empty attributes", request)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	if err := client.EscalateAlertCLI(context.Background(), "alert-1", nil, nil); err != nil {
		t.Fatalf("EscalateAlertCLI returned error: %v", err)
	}
}

func TestEscalateAlertCLIUsesAPIErrorTitle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"errors":[{"title":"Alert has no escalation policy. Provide escalation_policy_id."}]}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.EscalateAlertCLI(context.Background(), "alert-1", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "Alert has no escalation policy. Provide escalation_policy_id.") {
		t.Fatalf("error = %v, want API error title", err)
	}
}
