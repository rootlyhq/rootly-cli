package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func pageTargets() []NotificationTarget {
	return []NotificationTarget{
		{Type: NotificationTargetGroup, ID: "team-1"},
		{Type: NotificationTargetEscalationPolicy, ID: "ep-1"},
	}
}

func TestPageAlertTargetsCLICreate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/alerts" {
			t.Errorf("request = %s %s, want POST /v1/alerts", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/vnd.api+json" {
			t.Errorf("Content-Type = %q", got)
		}
		var body struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request body: %v", err)
			return
		}
		if _, ok := body.Data["id"]; ok {
			t.Errorf("create must not send data.id: %+v", body.Data)
		}
		attributes := body.Data["attributes"].(map[string]interface{})
		if attributes["summary"] != "Escalation" || attributes["description"] != "db down" {
			t.Errorf("attributes = %+v", attributes)
		}
		targets := attributes["notification_targets"].([]interface{})
		if len(targets) != 2 {
			t.Fatalf("targets = %+v", targets)
		}
		first := targets[0].(map[string]interface{})
		if first["type"] != "Group" || first["id"] != "team-1" {
			t.Errorf("first target = %+v", first)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"alert-new"}}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	id, err := client.PageAlertTargetsCLI(context.Background(), "", "Escalation", "db down", pageTargets())
	if err != nil {
		t.Fatalf("PageAlertTargetsCLI returned error: %v", err)
	}
	if id != "alert-new" {
		t.Errorf("alert id = %q, want alert-new", id)
	}
}

func TestPageAlertTargetsCLIUpsertsExistingAlert(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request body: %v", err)
			return
		}
		if body.Data["id"] != "alert-1" {
			t.Errorf("data.id = %v, want alert-1", body.Data["id"])
		}
		attributes := body.Data["attributes"].(map[string]interface{})
		if _, ok := attributes["description"]; ok {
			t.Errorf("empty description must be omitted: %+v", attributes)
		}
		_, _ = w.Write([]byte(`{"data":{"id":"alert-1"}}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	id, err := client.PageAlertTargetsCLI(context.Background(), "alert-1", "CPU", "", pageTargets()[:1])
	if err != nil {
		t.Fatalf("PageAlertTargetsCLI returned error: %v", err)
	}
	if id != "alert-1" {
		t.Errorf("alert id = %q", id)
	}
}

func TestPageAlertTargetsCLIErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"api title", http.StatusUnprocessableEntity, `{"errors":[{"title":"One or more notification_targets could not be found for this organization."}]}`, "notification_targets could not be found"},
		{"4xx without body", http.StatusForbidden, ``, "API returned status 403"},
		{"5xx", http.StatusBadGateway, ``, "API returned status 502"},
		{"bad json", http.StatusOK, `not json`, "failed to parse response"},
		{"missing id", http.StatusOK, `{"data":{}}`, "missing alert id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			client := newTestClient(t, server.URL)
			_, err := client.PageAlertTargetsCLI(context.Background(), "", "s", "", pageTargets())
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestPageAlertTargetsCLITransportError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	client := newTestClient(t, server.URL)
	server.Close()
	_, err := client.PageAlertTargetsCLI(context.Background(), "", "s", "", pageTargets())
	if err == nil || !strings.Contains(err.Error(), "failed to page team") {
		t.Fatalf("error = %v", err)
	}
}

func TestAttachAlertsToIncidentCLI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/incidents/inc-1/alerts" {
			t.Errorf("request = %s %s, want POST /v1/incidents/inc-1/alerts", r.Method, r.URL.Path)
		}
		var body struct {
			AlertIDs []string `json:"alert_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request body: %v", err)
			return
		}
		if len(body.AlertIDs) != 1 || body.AlertIDs[0] != "alert-1" {
			t.Errorf("alert_ids = %v", body.AlertIDs)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	if err := client.AttachAlertsToIncidentCLI(context.Background(), "inc-1", []string{"alert-1"}); err != nil {
		t.Fatalf("AttachAlertsToIncidentCLI returned error: %v", err)
	}
}

func TestAttachAlertsToIncidentCLIErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"api title", http.StatusNotFound, `{"errors":[{"title":"Incident not found"}]}`, "Incident not found"},
		{"5xx", http.StatusInternalServerError, ``, "API returned status 500"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			client := newTestClient(t, server.URL)
			err := client.AttachAlertsToIncidentCLI(context.Background(), "inc-1", []string{"a"})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestAttachAlertsToIncidentCLITransportError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	client := newTestClient(t, server.URL)
	server.Close()
	err := client.AttachAlertsToIncidentCLI(context.Background(), "inc-1", []string{"a"})
	if err == nil || !strings.Contains(err.Error(), "failed to attach alerts") {
		t.Fatalf("error = %v", err)
	}
}
