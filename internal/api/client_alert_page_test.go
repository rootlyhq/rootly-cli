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
		attributes := body.Data["attributes"].(map[string]interface{})
		if attributes["id"] != "alert-1" {
			t.Errorf("attributes.id = %v, want alert-1 (the server reads the upsert id from attributes)", attributes["id"])
		}
		if _, ok := body.Data["id"]; ok {
			t.Errorf("data.id is ignored by the server and must not be relied on: %+v", body.Data)
		}
		if _, ok := attributes["summary"]; ok {
			t.Errorf("existing alert must not be sent a summary: %+v", attributes)
		}
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

// attachServer mimics the replace-the-set endpoint. dropWrites makes that many
// updates vanish, as if a concurrent writer replaced the set right after them.
type attachFake struct {
	server *httptest.Server
	linked []string
	posts  int
}

func newAttachFake(t *testing.T, linked []string, dropWrites int) *attachFake {
	t.Helper()
	fake := &attachFake{linked: linked}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/incidents/inc-1/alerts" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Method == http.MethodGet {
			items := make([]string, len(fake.linked))
			for i, id := range fake.linked {
				items[i] = `{"id":"` + id + `"}`
			}
			_, _ = w.Write([]byte(`{"data":[` + strings.Join(items, ",") + `]}`))
			return
		}
		var posted struct {
			Data struct {
				Type       string `json:"type"`
				Attributes struct {
					AlertIDs []string `json:"alert_ids"`
				} `json:"attributes"`
			} `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if posted.Data.Type != "alerts" {
			t.Errorf("data.type = %q, want alerts", posted.Data.Type)
		}
		fake.posts++
		if fake.posts > dropWrites {
			fake.linked = posted.Data.Attributes.AlertIDs
		}
		w.WriteHeader(http.StatusOK)
	}))
	fake.server = server
	t.Cleanup(server.Close)
	return fake
}

func TestAttachAlertsToIncidentCLI(t *testing.T) {
	fake := newAttachFake(t, []string{"alert-old", "alert-1"}, 0)
	if err := newTestClient(t, fake.server.URL).AttachAlertsToIncidentCLI(context.Background(), "inc-1", []string{"alert-1", "alert-2"}); err != nil {
		t.Fatalf("AttachAlertsToIncidentCLI returned error: %v", err)
	}
	got := strings.Join(fake.linked, ",")
	if got != "alert-old,alert-1,alert-2" {
		t.Errorf("data.attributes.alert_ids = %s, want existing alerts kept, deduplicated, new appended", got)
	}
	if fake.posts != 1 {
		t.Errorf("posts = %d, want 1 when the link is confirmed", fake.posts)
	}
}

func TestAttachAlertsToIncidentCLIRetriesWhenLinkIsLost(t *testing.T) {
	fake := newAttachFake(t, []string{"alert-old"}, 1)
	if err := newTestClient(t, fake.server.URL).AttachAlertsToIncidentCLI(context.Background(), "inc-1", []string{"alert-new"}); err != nil {
		t.Fatalf("AttachAlertsToIncidentCLI returned error: %v", err)
	}
	if fake.posts != 2 {
		t.Errorf("posts = %d, want 2 (one retry)", fake.posts)
	}
	if got := strings.Join(fake.linked, ","); got != "alert-old,alert-new" {
		t.Errorf("linked = %s", got)
	}
}

func TestAttachAlertsToIncidentCLIFailsWhenLinkKeepsGettingLost(t *testing.T) {
	fake := newAttachFake(t, []string{"alert-old"}, 10)
	err := newTestClient(t, fake.server.URL).AttachAlertsToIncidentCLI(context.Background(), "inc-1", []string{"alert-new"})
	if err == nil || !strings.Contains(err.Error(), "alert-new are not linked") {
		t.Fatalf("error = %v", err)
	}
	if fake.posts != 2 {
		t.Errorf("posts = %d, want 2", fake.posts)
	}
}

func TestListIncidentAlertIDsCLIPaginates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page[number]") == "1" {
			items := make([]string, 100)
			for i := range items {
				items[i] = `{"id":"a` + strings.Repeat("x", i) + `"}`
			}
			_, _ = w.Write([]byte(`{"data":[` + strings.Join(items, ",") + `]}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"last"}]}`))
	}))
	defer server.Close()
	ids, err := newTestClient(t, server.URL).listIncidentAlertIDsCLI(context.Background(), "inc-1")
	if err != nil || len(ids) != 101 || ids[100] != "last" {
		t.Fatalf("ids = %d, err = %v", len(ids), err)
	}
}

func TestListIncidentAlertIDsCLIBadJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`nope`))
	}))
	defer server.Close()
	_, err := newTestClient(t, server.URL).listIncidentAlertIDsCLI(context.Background(), "inc-1")
	if err == nil || !strings.Contains(err.Error(), "failed to parse response") {
		t.Fatalf("error = %v", err)
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
	// The same status on the initial listing and on the update both fail.
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
	if err == nil || !strings.Contains(err.Error(), "failed to list incident alerts") {
		t.Fatalf("error = %v", err)
	}
}
