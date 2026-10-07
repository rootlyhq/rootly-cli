package statuspages

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	return cmd
}

func setupTestServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	viper.Set("api_key", "test-token")
	viper.Set("api_host", server.URL)
	t.Cleanup(viper.Reset)
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	original := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = original
	output, _ := io.ReadAll(r)
	_ = r.Close()
	return string(output)
}

func statusPageEventResponse() string {
	return `{
		"data": {"id": "event-1", "attributes": {
			"event": "We are investigating.", "status": "investigating", "status_page_id": "page-1",
			"notify_subscribers": true, "started_at": "2026-08-12T12:00:00Z",
			"created_at": "2026-08-12T12:00:00Z", "updated_at": "2026-08-12T12:00:00Z"
		}}
	}`
}

func TestRunList(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"data": [{"id": "page-1", "attributes": {
				"title": "Public Status", "slug": "public-status", "enabled": true, "public": true,
				"created_at": "2026-08-12T12:00:00Z"
			}}],
			"meta": {"current_page": 1, "total_pages": 1, "total_count": 1}
		}`))
	})
	viper.Set("format", "table")
	cmd := newTestCmd()
	cmd.Flags().Int("page", 1, "")
	cmd.Flags().Int("page-size", 25, "")
	cmd.Flags().String("sort", "-created_at", "")
	cmd.Flags().String("name", "", "")
	cmd.Flags().String("slug", "", "")

	output := captureStdout(t, func() {
		if err := runList(cmd, nil); err != nil {
			t.Fatalf("runList returned error: %v", err)
		}
	})
	if !strings.Contains(output, "Public Status") {
		t.Errorf("expected status page in output, got: %s", output)
	}
}

func TestRunTemplatesList(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/status-pages/page-1/templates" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("page[number]") != "1" || r.URL.Query().Get("page[size]") != "25" {
			t.Errorf("pagination query = %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{
			"data": [{
				"id": "template-1",
				"attributes": {
					"title": "Maintenance update",
					"kind": "maintenance",
					"update_status": "scheduled",
					"should_notify_subscribers": true,
					"enabled": true
				}
			}],
			"meta": {"current_page": 1, "total_pages": 1, "total_count": 1}
		}`))
	})
	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().String("status-page", "page-1", "")
	cmd.Flags().Int("page", 1, "")
	cmd.Flags().Int("page-size", 25, "")
	output := captureStdout(t, func() {
		if err := runTemplatesList(cmd, nil); err != nil {
			t.Fatalf("runTemplatesList returned error: %v", err)
		}
	})
	for _, expected := range []string{"Maintenance update", "maintenance", "scheduled", "true"} {
		if !strings.Contains(output, expected) {
			t.Errorf("template output %q does not contain %q", output, expected)
		}
	}
}

func TestRunTemplatesListJSON(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"template-1","attributes":{"title":"Maintenance update"}}]}`))
	})
	viper.Set("format", "json")
	cmd := newTestCmd()
	cmd.Flags().String("status-page", "page-1", "")
	cmd.Flags().Int("page", 1, "")
	cmd.Flags().Int("page-size", 25, "")
	output := captureStdout(t, func() {
		if err := runTemplatesList(cmd, nil); err != nil {
			t.Fatalf("runTemplatesList returned error: %v", err)
		}
	})
	if !strings.Contains(output, "template-1") {
		t.Errorf("JSON output = %q, want template-1", output)
	}
}

func TestRunEventsListNormalizesIncidentID(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/incidents/42/status-page-events" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"data": [{"id": "event-1", "attributes": {
				"event": "Monitoring", "status": "monitoring", "status_page_id": "page-1",
				"started_at": "2026-08-12T12:00:00Z", "created_at": "2026-08-12T12:00:00Z", "updated_at": "2026-08-12T12:30:00Z"
			}}],
			"meta": {"current_page": 1, "total_pages": 1, "total_count": 1}
		}`))
	})
	viper.Set("format", "table")
	cmd := newTestCmd()
	cmd.Flags().Int("page", 1, "")
	cmd.Flags().Int("page-size", 25, "")

	output := captureStdout(t, func() {
		if err := runEventsList(cmd, []string{"INC-42"}); err != nil {
			t.Fatalf("runEventsList returned error: %v", err)
		}
	})
	if !strings.Contains(output, "Monitoring") {
		t.Errorf("expected event in output, got: %s", output)
	}
}

func TestRunEventsCreate(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/incidents/42/status-page-events" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(statusPageEventResponse()))
	})
	viper.Set("format", "json")
	cmd := newTestCmd()
	cmd.Flags().String("status-page", "page-1", "")
	cmd.Flags().String("status", "investigating", "")
	cmd.Flags().String("message", "We are investigating.", "")
	cmd.Flags().Bool("notify-subscribers", true, "")
	cmd.Flags().String("started-at", "", "")

	output := captureStdout(t, func() {
		if err := runEventsCreate(cmd, []string{"INC-42"}); err != nil {
			t.Fatalf("runEventsCreate returned error: %v", err)
		}
	})
	if !strings.Contains(output, "event-1") {
		t.Errorf("expected event response, got: %s", output)
	}
}

func TestRunEventsUpdateRequiresChange(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called")
	})
	cmd := newTestCmd()
	cmd.Flags().String("status", "", "")
	cmd.Flags().String("message", "", "")
	cmd.Flags().String("started-at", "", "")

	err := runEventsUpdate(cmd, []string{"event-1"})
	if err == nil || !strings.Contains(err.Error(), "at least one field") {
		t.Fatalf("error = %v, want at least one field", err)
	}
}

func TestRunEventsResolveSetsResolvedStatus(t *testing.T) {
	var requestBody map[string]interface{}
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &requestBody)
		_, _ = w.Write([]byte(statusPageEventResponse()))
	})
	viper.Set("format", "json")
	cmd := newTestCmd()
	cmd.Flags().String("message", "Resolved.", "")
	cmd.Flags().String("status", "resolved", "")

	captureStdout(t, func() {
		if err := runEventsResolve(cmd, []string{"event-1"}); err != nil {
			t.Fatalf("runEventsResolve returned error: %v", err)
		}
	})
	attributes := requestBody["data"].(map[string]interface{})["attributes"].(map[string]interface{})
	if attributes["status"] != "resolved" || attributes["event"] != "Resolved." {
		t.Errorf("unexpected resolve attributes: %+v", attributes)
	}
}

func TestRunEventsResolveAllowsCompletedStatus(t *testing.T) {
	var requestBody map[string]interface{}
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &requestBody)
		_, _ = w.Write([]byte(statusPageEventResponse()))
	})
	viper.Set("format", "json")
	cmd := newTestCmd()
	cmd.Flags().String("message", "Maintenance completed.", "")
	cmd.Flags().String("status", "completed", "")

	captureStdout(t, func() {
		if err := runEventsResolve(cmd, []string{"event-1"}); err != nil {
			t.Fatalf("runEventsResolve returned error: %v", err)
		}
	})
	attributes := requestBody["data"].(map[string]interface{})["attributes"].(map[string]interface{})
	if attributes["status"] != "completed" {
		t.Errorf("resolve status = %v, want completed", attributes["status"])
	}
}

func TestRunEventsResolveRejectsOtherStatuses(t *testing.T) {
	cmd := newTestCmd()
	cmd.Flags().String("message", "Done.", "")
	cmd.Flags().String("status", "monitoring", "")
	err := runEventsResolve(cmd, []string{"event-1"})
	if err == nil || !strings.Contains(err.Error(), "must be resolved or completed") {
		t.Fatalf("error = %v, want status validation", err)
	}
}

func TestEventsStatusFlagHelpIncludesMaintenanceStatuses(t *testing.T) {
	for _, command := range []*cobra.Command{eventsCreateCmd, eventsUpdateCmd} {
		flag := command.Flags().Lookup("status")
		if flag == nil {
			t.Fatalf("%s has no --status flag", command.Name())
		}
		if !strings.Contains(flag.Usage, "scheduled, in_progress, completed") || !strings.Contains(flag.Usage, "investigating, identified, monitoring, resolved") {
			t.Errorf("%s --status help = %q", command.Name(), flag.Usage)
		}
	}
	resolveStatus := eventsResolveCmd.Flags().Lookup("status")
	if resolveStatus == nil || resolveStatus.DefValue != "resolved" || resolveStatus.Usage != "use completed for scheduled maintenance" {
		t.Errorf("resolve --status flag = %+v", resolveStatus)
	}
}

func TestParseStartedAtRejectsInvalidTimestamp(t *testing.T) {
	cmd := newTestCmd()
	cmd.Flags().String("started-at", "tomorrow", "")
	if _, err := parseStartedAt(cmd); err == nil || !strings.Contains(err.Error(), "RFC3339") {
		t.Fatalf("error = %v, want RFC3339 validation error", err)
	}
}
