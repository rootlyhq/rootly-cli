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

// addShiftsFlags registers all flags that runShifts reads.
func addShiftsFlags(cmd *cobra.Command, overrides map[string]string) {
	defaults := map[string]string{
		"schedule-id":          "",
		"schedule":             "",
		"service-id":           "",
		"service":              "",
		"escalation-policy-id": "",
		"user-id":              "",
		"user":                 "",
		"team-id":              "",
		"team":                 "",
		"time-zone":            "",
		"include":              "user,schedule,escalation_policy",
	}
	for k, v := range overrides {
		defaults[k] = v
	}
	cmd.Flags().Int("days", 7, "")
	for k, v := range defaults {
		cmd.Flags().String(k, v, "")
	}
}

// addWhoFlags registers all flags that runWho reads.
func addWhoFlags(cmd *cobra.Command, overrides map[string]string) {
	defaults := map[string]string{
		"schedule-id":          "",
		"schedule":             "",
		"service-id":           "",
		"service":              "",
		"escalation-policy-id": "",
		"user-id":              "",
		"user":                 "",
		"team-id":              "",
		"team":                 "",
		"time-zone":            "",
		"include":              "user,schedule,escalation_policy",
	}
	for k, v := range overrides {
		defaults[k] = v
	}
	cmd.Flags().Bool("earliest", true, "")
	for k, v := range defaults {
		cmd.Flags().String(k, v, "")
	}
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

func singleScheduleResponse() string {
	return `{
		"data": [{
			"id": "sched-1",
			"attributes": {
				"name": "Primary On-Call",
				"description": "Primary rotation",
				"created_at": "2025-01-01T00:00:00Z"
			}
		}],
		"meta": {"current_page": 1, "total_pages": 1, "total_count": 1}
	}`
}

func singleServiceResponse() string {
	return `{
		"data": [{
			"id": "svc-42",
			"attributes": {
				"name": "API Gateway",
				"slug": "api-gateway",
				"created_at": "2025-01-01T00:00:00Z",
				"updated_at": "2025-01-01T00:00:00Z"
			}
		}],
		"meta": {"current_page": 1, "total_pages": 1, "total_count": 1}
	}`
}

func singleUserResponse() string {
	return `{
		"data": [{
			"id": "user-99",
			"attributes": {
				"email": "alice@example.com",
				"full_name": "Alice Smith",
				"created_at": "2025-01-01T00:00:00Z",
				"updated_at": "2025-01-01T00:00:00Z"
			}
		}],
		"meta": {"current_page": 1, "total_pages": 1, "total_count": 1}
	}`
}

func singleTeamResponse() string {
	return `{
		"data": [{
			"id": "team-7",
			"attributes": {
				"name": "Platform Engineering",
				"slug": "platform-engineering",
				"created_at": "2025-01-01T00:00:00Z",
				"updated_at": "2025-01-01T00:00:00Z"
			}
		}],
		"meta": {"current_page": 1, "total_pages": 1, "total_count": 1}
	}`
}

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

func emptyListResponse() string {
	return `{
		"data": [],
		"meta": {"current_page": 1, "total_pages": 0, "total_count": 0}
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
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)

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
	addShiftsFlags(cmd, nil)

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
	addShiftsFlags(cmd, nil)

	output := captureStdout(t, func() {
		err := runShifts(cmd, nil)
		if err != nil {
			t.Fatalf("runShifts returned error: %v", err)
		}
	})

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
	addShiftsFlags(cmd, nil)

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
	addShiftsFlags(cmd, nil)

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

func TestRunShiftsWithScheduleIDFilter(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.RawQuery
		if !strings.Contains(query, "filter[schedule_ids]=sched-1") {
			t.Errorf("expected schedule_ids filter in query, got: %s", query)
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(oncallsResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	addShiftsFlags(cmd, map[string]string{"schedule-id": "sched-1"})

	captureStdout(t, func() {
		err := runShifts(cmd, nil)
		if err != nil {
			t.Fatalf("runShifts returned error: %v", err)
		}
	})
}

func TestRunShiftsWithScheduleNameFilter(t *testing.T) {
	var oncallsRequested bool
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		if strings.Contains(r.URL.Path, "/v1/schedules") {
			if !strings.Contains(r.URL.RawQuery, "filter[name]=Primary+On-Call") &&
				!strings.Contains(r.URL.RawQuery, "filter[name]=Primary%20On-Call") {
				t.Errorf("expected schedule name filter, got: %s", r.URL.RawQuery)
			}
			w.Write([]byte(singleScheduleResponse()))
			return
		}
		if strings.Contains(r.URL.Path, "/v1/oncalls") {
			oncallsRequested = true
			if !strings.Contains(r.URL.RawQuery, "filter[schedule_ids]=sched-1") {
				t.Errorf("expected resolved schedule ID in oncalls query, got: %s", r.URL.RawQuery)
			}
			w.Write([]byte(oncallsResponse()))
			return
		}
		t.Errorf("unexpected path: %s", r.URL.Path)
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	addShiftsFlags(cmd, map[string]string{"schedule": "Primary On-Call"})

	captureStdout(t, func() {
		err := runShifts(cmd, nil)
		if err != nil {
			t.Fatalf("runShifts returned error: %v", err)
		}
	})

	if !oncallsRequested {
		t.Error("expected /v1/oncalls to be called after schedule lookup")
	}
}

func TestRunShiftsWithServiceNameFilter(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		if strings.Contains(r.URL.Path, "/v1/services") {
			w.Write([]byte(singleServiceResponse()))
			return
		}
		if strings.Contains(r.URL.Path, "/v1/oncalls") {
			if !strings.Contains(r.URL.RawQuery, "filter[service_ids]=svc-42") {
				t.Errorf("expected resolved service ID, got: %s", r.URL.RawQuery)
			}
			w.Write([]byte(oncallsResponse()))
			return
		}
		t.Errorf("unexpected path: %s", r.URL.Path)
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	addShiftsFlags(cmd, map[string]string{"service": "API Gateway"})

	captureStdout(t, func() {
		err := runShifts(cmd, nil)
		if err != nil {
			t.Fatalf("runShifts returned error: %v", err)
		}
	})
}

func TestRunShiftsWithUserEmailFilter(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		if strings.Contains(r.URL.Path, "/v1/users") {
			if !strings.Contains(r.URL.RawQuery, "filter[email]=alice") {
				t.Errorf("expected email filter for user lookup, got: %s", r.URL.RawQuery)
			}
			w.Write([]byte(singleUserResponse()))
			return
		}
		if strings.Contains(r.URL.Path, "/v1/oncalls") {
			if !strings.Contains(r.URL.RawQuery, "filter[user_ids]=user-99") {
				t.Errorf("expected resolved user ID, got: %s", r.URL.RawQuery)
			}
			w.Write([]byte(oncallsResponse()))
			return
		}
		t.Errorf("unexpected path: %s", r.URL.Path)
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	addShiftsFlags(cmd, map[string]string{"user": "alice@example.com"})

	captureStdout(t, func() {
		err := runShifts(cmd, nil)
		if err != nil {
			t.Fatalf("runShifts returned error: %v", err)
		}
	})
}

func TestRunShiftsWithTeamNameFilter(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		if strings.Contains(r.URL.Path, "/v1/teams") {
			w.Write([]byte(singleTeamResponse()))
			return
		}
		if strings.Contains(r.URL.Path, "/v1/oncalls") {
			if !strings.Contains(r.URL.RawQuery, "filter[group_ids]=team-7") {
				t.Errorf("expected resolved team/group ID, got: %s", r.URL.RawQuery)
			}
			w.Write([]byte(oncallsResponse()))
			return
		}
		t.Errorf("unexpected path: %s", r.URL.Path)
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	addShiftsFlags(cmd, map[string]string{"team": "Platform Engineering"})

	captureStdout(t, func() {
		err := runShifts(cmd, nil)
		if err != nil {
			t.Fatalf("runShifts returned error: %v", err)
		}
	})
}

func TestRunShiftsMutualExclusion(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(oncallsResponse()))
	})

	cmd := newTestCmd()
	addShiftsFlags(cmd, map[string]string{
		"schedule-id": "sched-1",
		"schedule":    "Primary On-Call",
	})

	err := runShifts(cmd, nil)
	if err == nil {
		t.Fatal("expected error when both --schedule-id and --schedule are set")
	}
	if !strings.Contains(err.Error(), "not both") {
		t.Errorf("expected mutual exclusion error, got: %v", err)
	}
}

func TestRunShiftsScheduleNotFound(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(emptyListResponse()))
	})

	cmd := newTestCmd()
	addShiftsFlags(cmd, map[string]string{"schedule": "Nonexistent"})

	err := runShifts(cmd, nil)
	if err == nil {
		t.Fatal("expected error when schedule not found")
	}
	if !strings.Contains(err.Error(), "no schedule found") {
		t.Errorf("expected 'no schedule found' error, got: %v", err)
	}
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
	addWhoFlags(cmd, nil)

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
	addWhoFlags(cmd, nil)
	cmd.Flags().Set("earliest", "false")

	captureStdout(t, func() {
		err := runWho(cmd, nil)
		if err != nil {
			t.Fatalf("runWho returned error: %v", err)
		}
	})
}

func TestRunWhoWithServiceIDFilter(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.RawQuery
		if !strings.Contains(query, "filter[service_ids]=svc-1") {
			t.Errorf("expected service_ids filter in query, got: %s", query)
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(oncallsResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	addWhoFlags(cmd, map[string]string{"service-id": "svc-1"})

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
	addWhoFlags(cmd, nil)

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
	addWhoFlags(cmd, nil)

	captureStdout(t, func() {
		err := runWho(cmd, nil)
		if err != nil {
			t.Fatalf("runWho returned error: %v", err)
		}
	})
}

func TestRunWhoWithScheduleIDFilter(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.RawQuery
		if !strings.Contains(query, "filter[schedule_ids]=sched-1") {
			t.Errorf("expected schedule_ids filter in query, got: %s", query)
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(oncallsResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	addWhoFlags(cmd, map[string]string{"schedule-id": "sched-1"})

	captureStdout(t, func() {
		err := runWho(cmd, nil)
		if err != nil {
			t.Fatalf("runWho returned error: %v", err)
		}
	})
}

func TestRunWhoWithScheduleNameFilter(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		if strings.Contains(r.URL.Path, "/v1/schedules") {
			w.Write([]byte(singleScheduleResponse()))
			return
		}
		if strings.Contains(r.URL.Path, "/v1/oncalls") {
			if !strings.Contains(r.URL.RawQuery, "filter[schedule_ids]=sched-1") {
				t.Errorf("expected resolved schedule ID, got: %s", r.URL.RawQuery)
			}
			w.Write([]byte(oncallsResponse()))
			return
		}
		t.Errorf("unexpected path: %s", r.URL.Path)
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	addWhoFlags(cmd, map[string]string{"schedule": "Primary On-Call"})

	captureStdout(t, func() {
		err := runWho(cmd, nil)
		if err != nil {
			t.Fatalf("runWho returned error: %v", err)
		}
	})
}

func TestRunWhoWithUserSearchFilter(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		if strings.Contains(r.URL.Path, "/v1/users") {
			if !strings.Contains(r.URL.RawQuery, "filter[search]=Alice") {
				t.Errorf("expected search filter for user lookup, got: %s", r.URL.RawQuery)
			}
			w.Write([]byte(singleUserResponse()))
			return
		}
		if strings.Contains(r.URL.Path, "/v1/oncalls") {
			if !strings.Contains(r.URL.RawQuery, "filter[user_ids]=user-99") {
				t.Errorf("expected resolved user ID, got: %s", r.URL.RawQuery)
			}
			w.Write([]byte(oncallsResponse()))
			return
		}
		t.Errorf("unexpected path: %s", r.URL.Path)
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	addWhoFlags(cmd, map[string]string{"user": "Alice"})

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
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)

	cmd := newTestCmd()
	addShiftsFlags(cmd, nil)

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
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)

	cmd := newTestCmd()
	addWhoFlags(cmd, nil)

	err := runWho(cmd, nil)
	if err == nil {
		t.Fatal("expected error when no API token")
	}
}
