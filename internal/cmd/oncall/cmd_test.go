package oncall

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	return cmd
}

func schedulesResponse() string {
	return `{
		"data": [{
			"id": "sched-1",
			"attributes": {
				"name": "Primary On-Call",
				"description": "Primary rotation",
				"created_at": "2025-01-01T00:00:00Z"
			}
		}, {
			"id": "sched-2",
			"attributes": {
				"name": "Secondary On-Call",
				"description": "Backup rotation",
				"created_at": "2025-02-01T00:00:00Z"
			}
		}],
		"meta": {
			"current_page": 1,
			"total_pages": 1,
			"total_count": 2
		}
	}`
}

// oncallsResponse returns a /v1/oncalls response with two on-call entries.
func oncallsResponse() string {
	now := time.Now()
	endsAt := now.Add(23 * time.Hour).Format(time.RFC3339)
	futureStart := now.Add(24 * time.Hour).Format(time.RFC3339)
	futureEnd := now.Add(48 * time.Hour).Format(time.RFC3339)

	return fmt.Sprintf(`{
		"data": [{
			"id": "oncall-1",
			"attributes": {
				"user_id": "user-1",
				"schedule_id": "sched-1",
				"schedule_name": "Primary On-Call",
				"escalation_policy_id": "ep-1",
				"escalation_policy_name": "Default Policy",
				"escalation_level": 1,
				"starts_at": "%s",
				"ends_at": "%s"
			}
		}, {
			"id": "oncall-2",
			"attributes": {
				"user_id": "user-2",
				"schedule_id": "sched-1",
				"schedule_name": "Primary On-Call",
				"escalation_policy_id": "ep-1",
				"escalation_policy_name": "Default Policy",
				"escalation_level": 2,
				"starts_at": "%s",
				"ends_at": "%s"
			}
		}],
		"included": [
			{"type": "users", "id": "user-1", "attributes": {"full_name": "Alice Smith", "email": "alice@example.com"}},
			{"type": "users", "id": "user-2", "attributes": {"full_name": "Bob Jones", "email": "bob@example.com"}}
		]
	}`, now.Add(-1*time.Hour).Format(time.RFC3339), endsAt, futureStart, futureEnd)
}

func emptyOncallsResponse() string {
	return `{
		"data": [],
		"included": []
	}`
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
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = origStdout

	buf := make([]byte, 65536)
	n, _ := r.Read(buf)
	r.Close()
	return string(buf[:n])
}

// --- runList tests ---

func TestRunListTable(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/v1/schedules") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(schedulesResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().Int("page", 1, "")
	cmd.Flags().Int("page-size", 25, "")

	output := captureStdout(t, func() {
		err := runList(cmd, nil)
		if err != nil {
			t.Fatalf("runList returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Primary On-Call") {
		t.Errorf("expected output to contain 'Primary On-Call', got: %s", output)
	}
}

func TestRunListJSON(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(schedulesResponse()))
	})

	viper.Set("format", "json")

	cmd := newTestCmd()
	cmd.Flags().Int("page", 1, "")
	cmd.Flags().Int("page-size", 25, "")

	output := captureStdout(t, func() {
		err := runList(cmd, nil)
		if err != nil {
			t.Fatalf("runList returned error: %v", err)
		}
	})

	if !strings.Contains(output, "sched-1") {
		t.Errorf("expected JSON output to contain 'sched-1', got: %s", output)
	}
}

func TestRunListPagination(t *testing.T) {
	resp := `{
		"data": [{"id": "sched-1", "attributes": {"name": "Test", "description": "", "created_at": "2025-01-01T00:00:00Z"}}],
		"meta": {"current_page": 1, "total_pages": 3, "total_count": 75}
	}`
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(resp))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().Int("page", 1, "")
	cmd.Flags().Int("page-size", 25, "")

	captureStdout(t, func() {
		err := runList(cmd, nil)
		if err != nil {
			t.Fatalf("runList returned error: %v", err)
		}
	})
}

func TestRunListNoToken(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	t.Setenv("HOME", t.TempDir())

	cmd := newTestCmd()
	cmd.Flags().Int("page", 1, "")
	cmd.Flags().Int("page-size", 25, "")

	err := runList(cmd, nil)
	if err == nil {
		t.Fatal("expected error when no API token")
	}
	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("expected 'authentication required' error, got: %v", err)
	}
}

// --- runShifts tests ---

func TestRunShiftsTable(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/v1/oncalls") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(oncallsResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().Int("days", 7, "")
	cmd.Flags().String("schedule-id", "", "")
	cmd.Flags().String("service-id", "", "")
	cmd.Flags().String("escalation-policy-id", "", "")
	cmd.Flags().String("user-id", "", "")
	cmd.Flags().String("time-zone", "", "")
	cmd.Flags().String("include", "user,schedule,escalation_policy", "")

	output := captureStdout(t, func() {
		err := runShifts(cmd, nil)
		if err != nil {
			t.Fatalf("runShifts returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Alice Smith") {
		t.Errorf("expected output to contain 'Alice Smith', got: %s", output)
	}
	if !strings.Contains(output, "alice@example.com") {
		t.Errorf("expected output to contain 'alice@example.com', got: %s", output)
	}
	if !strings.Contains(output, "Default Policy") {
		t.Errorf("expected output to contain 'Default Policy', got: %s", output)
	}
}

func TestRunShiftsEmptyResults(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(emptyOncallsResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().Int("days", 7, "")
	cmd.Flags().String("schedule-id", "", "")
	cmd.Flags().String("service-id", "", "")
	cmd.Flags().String("escalation-policy-id", "", "")
	cmd.Flags().String("user-id", "", "")
	cmd.Flags().String("time-zone", "", "")
	cmd.Flags().String("include", "user,schedule,escalation_policy", "")

	output := captureStdout(t, func() {
		err := runShifts(cmd, nil)
		if err != nil {
			t.Fatalf("runShifts returned error: %v", err)
		}
	})

	// Empty table should still have headers
	if strings.Contains(output, "Alice") {
		t.Errorf("expected no data rows, got: %s", output)
	}
}

func TestRunShiftsSinceUntilParams(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.RawQuery
		if !strings.Contains(query, "since=") {
			t.Errorf("expected 'since' param in query, got: %s", query)
		}
		if !strings.Contains(query, "until=") {
			t.Errorf("expected 'until' param in query, got: %s", query)
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(oncallsResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().Int("days", 14, "")
	cmd.Flags().String("schedule-id", "", "")
	cmd.Flags().String("service-id", "", "")
	cmd.Flags().String("escalation-policy-id", "", "")
	cmd.Flags().String("user-id", "", "")
	cmd.Flags().String("time-zone", "", "")
	cmd.Flags().String("include", "user,schedule,escalation_policy", "")

	captureStdout(t, func() {
		err := runShifts(cmd, nil)
		if err != nil {
			t.Fatalf("runShifts returned error: %v", err)
		}
	})
}

func TestRunShiftsJSON(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(oncallsResponse()))
	})

	viper.Set("format", "json")

	cmd := newTestCmd()
	cmd.Flags().Int("days", 7, "")
	cmd.Flags().String("schedule-id", "", "")
	cmd.Flags().String("service-id", "", "")
	cmd.Flags().String("escalation-policy-id", "", "")
	cmd.Flags().String("user-id", "", "")
	cmd.Flags().String("time-zone", "", "")
	cmd.Flags().String("include", "user,schedule,escalation_policy", "")

	output := captureStdout(t, func() {
		err := runShifts(cmd, nil)
		if err != nil {
			t.Fatalf("runShifts returned error: %v", err)
		}
	})

	if !strings.Contains(output, "oncall-1") {
		t.Errorf("expected JSON output to contain 'oncall-1', got: %s", output)
	}
}

func TestRunShiftsWithScheduleFilter(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.RawQuery
		if !strings.Contains(query, "filter[schedule_id]=sched-1") {
			t.Errorf("expected schedule_id filter in query, got: %s", query)
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(oncallsResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().Int("days", 7, "")
	cmd.Flags().String("schedule-id", "sched-1", "")
	cmd.Flags().String("service-id", "", "")
	cmd.Flags().String("escalation-policy-id", "", "")
	cmd.Flags().String("user-id", "", "")
	cmd.Flags().String("time-zone", "", "")
	cmd.Flags().String("include", "user,schedule,escalation_policy", "")

	captureStdout(t, func() {
		err := runShifts(cmd, nil)
		if err != nil {
			t.Fatalf("runShifts returned error: %v", err)
		}
	})
}

// --- runWho tests ---

func TestRunWhoTable(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/v1/oncalls") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		query := r.URL.RawQuery
		if !strings.Contains(query, "earliest=true") {
			t.Errorf("expected earliest=true in query, got: %s", query)
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(oncallsResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().String("schedule-id", "", "")
	cmd.Flags().String("service-id", "", "")
	cmd.Flags().String("escalation-policy-id", "", "")
	cmd.Flags().String("user-id", "", "")
	cmd.Flags().String("time-zone", "", "")
	cmd.Flags().Bool("earliest", true, "")
	cmd.Flags().String("include", "user,schedule,escalation_policy", "")

	output := captureStdout(t, func() {
		err := runWho(cmd, nil)
		if err != nil {
			t.Fatalf("runWho returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Alice Smith") {
		t.Errorf("expected 'Alice Smith' in output, got: %s", output)
	}
	if !strings.Contains(output, "alice@example.com") {
		t.Errorf("expected 'alice@example.com' in output, got: %s", output)
	}
	if !strings.Contains(output, "Default Policy") {
		t.Errorf("expected 'Default Policy' in output, got: %s", output)
	}
}

func TestRunWhoEarliestFalse(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.RawQuery
		if strings.Contains(query, "earliest=true") {
			t.Errorf("expected earliest=true to be absent, got: %s", query)
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(oncallsResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().String("schedule-id", "", "")
	cmd.Flags().String("service-id", "", "")
	cmd.Flags().String("escalation-policy-id", "", "")
	cmd.Flags().String("user-id", "", "")
	cmd.Flags().String("time-zone", "", "")
	cmd.Flags().Bool("earliest", false, "")
	cmd.Flags().String("include", "user,schedule,escalation_policy", "")

	captureStdout(t, func() {
		err := runWho(cmd, nil)
		if err != nil {
			t.Fatalf("runWho returned error: %v", err)
		}
	})
}

func TestRunWhoWithServiceFilter(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.RawQuery
		if !strings.Contains(query, "filter[service_id]=svc-1") {
			t.Errorf("expected service_id filter in query, got: %s", query)
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(oncallsResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().String("schedule-id", "", "")
	cmd.Flags().String("service-id", "svc-1", "")
	cmd.Flags().String("escalation-policy-id", "", "")
	cmd.Flags().String("user-id", "", "")
	cmd.Flags().String("time-zone", "", "")
	cmd.Flags().Bool("earliest", true, "")
	cmd.Flags().String("include", "user,schedule,escalation_policy", "")

	captureStdout(t, func() {
		err := runWho(cmd, nil)
		if err != nil {
			t.Fatalf("runWho returned error: %v", err)
		}
	})
}

func TestRunWhoJSON(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(oncallsResponse()))
	})

	viper.Set("format", "json")

	cmd := newTestCmd()
	cmd.Flags().String("schedule-id", "", "")
	cmd.Flags().String("service-id", "", "")
	cmd.Flags().String("escalation-policy-id", "", "")
	cmd.Flags().String("user-id", "", "")
	cmd.Flags().String("time-zone", "", "")
	cmd.Flags().Bool("earliest", true, "")
	cmd.Flags().String("include", "user,schedule,escalation_policy", "")

	output := captureStdout(t, func() {
		err := runWho(cmd, nil)
		if err != nil {
			t.Fatalf("runWho returned error: %v", err)
		}
	})

	if !strings.Contains(output, "oncall-1") {
		t.Errorf("expected JSON output to contain 'oncall-1', got: %s", output)
	}
}

func TestRunWhoNoActiveShifts(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(emptyOncallsResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().String("schedule-id", "", "")
	cmd.Flags().String("service-id", "", "")
	cmd.Flags().String("escalation-policy-id", "", "")
	cmd.Flags().String("user-id", "", "")
	cmd.Flags().String("time-zone", "", "")
	cmd.Flags().Bool("earliest", true, "")
	cmd.Flags().String("include", "user,schedule,escalation_policy", "")

	captureStdout(t, func() {
		err := runWho(cmd, nil)
		if err != nil {
			t.Fatalf("runWho returned error: %v", err)
		}
	})
}

func TestRunWhoWithScheduleFilter(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.RawQuery
		if !strings.Contains(query, "filter[schedule_id]=sched-1") {
			t.Errorf("expected schedule_id filter in query, got: %s", query)
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(oncallsResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().String("schedule-id", "sched-1", "")
	cmd.Flags().String("service-id", "", "")
	cmd.Flags().String("escalation-policy-id", "", "")
	cmd.Flags().String("user-id", "", "")
	cmd.Flags().String("time-zone", "", "")
	cmd.Flags().Bool("earliest", true, "")
	cmd.Flags().String("include", "user,schedule,escalation_policy", "")

	captureStdout(t, func() {
		err := runWho(cmd, nil)
		if err != nil {
			t.Fatalf("runWho returned error: %v", err)
		}
	})
}

func TestRunShiftsNoToken(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	t.Setenv("HOME", t.TempDir())

	cmd := newTestCmd()
	cmd.Flags().Int("days", 7, "")
	cmd.Flags().String("schedule-id", "", "")
	cmd.Flags().String("service-id", "", "")
	cmd.Flags().String("escalation-policy-id", "", "")
	cmd.Flags().String("user-id", "", "")
	cmd.Flags().String("time-zone", "", "")
	cmd.Flags().String("include", "user,schedule,escalation_policy", "")

	err := runShifts(cmd, nil)
	if err == nil {
		t.Fatal("expected error when no API token")
	}
	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("expected 'authentication required' error, got: %v", err)
	}
}

func TestRunWhoNoToken(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	t.Setenv("HOME", t.TempDir())

	cmd := newTestCmd()
	cmd.Flags().String("schedule-id", "", "")
	cmd.Flags().String("service-id", "", "")
	cmd.Flags().String("escalation-policy-id", "", "")
	cmd.Flags().String("user-id", "", "")
	cmd.Flags().String("time-zone", "", "")
	cmd.Flags().Bool("earliest", true, "")
	cmd.Flags().String("include", "user,schedule,escalation_policy", "")

	err := runWho(cmd, nil)
	if err == nil {
		t.Fatal("expected error when no API token")
	}
}
