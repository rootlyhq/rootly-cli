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
	cmd.Flags().String("search", "", "")
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
	cmd.Flags().Bool("notify-subscribers", false, "")

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
	cmd.Flags().Bool("notify-subscribers", true, "")

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

func TestValidateEventStatus(t *testing.T) {
	if err := validateEventStatus("monitoring"); err != nil {
		t.Fatalf("monitoring should be valid: %v", err)
	}
	if err := validateEventStatus("unknown"); err == nil {
		t.Fatal("unknown status should be rejected")
	}
}
