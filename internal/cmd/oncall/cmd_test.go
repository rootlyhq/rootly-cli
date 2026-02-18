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

// shiftsResponse returns a shifts response with one active and one future shift.
// Uses dynamic times so IsActive is computed correctly (client-side check).
func shiftsResponse() string {
	now := time.Now()
	activeStart := now.Add(-1 * time.Hour).Format(time.RFC3339)
	activeEnd := now.Add(23 * time.Hour).Format(time.RFC3339)
	futureStart := now.Add(24 * time.Hour).Format(time.RFC3339)
	futureEnd := now.Add(48 * time.Hour).Format(time.RFC3339)

	return fmt.Sprintf(`{
		"data": [{
			"id": "shift-1",
			"attributes": {
				"starts_at": "%s",
				"ends_at": "%s"
			},
			"relationships": {
				"user": {"data": {"id": "user-1"}},
				"schedule": {"data": {"id": "sched-1"}}
			}
		}, {
			"id": "shift-2",
			"attributes": {
				"starts_at": "%s",
				"ends_at": "%s"
			},
			"relationships": {
				"user": {"data": {"id": "user-2"}},
				"schedule": {"data": {"id": "sched-1"}}
			}
		}],
		"included": [
			{"type": "users", "id": "user-1", "attributes": {"name": "Alice Smith"}},
			{"type": "users", "id": "user-2", "attributes": {"name": "Bob Jones"}},
			{"type": "on_call_schedules", "id": "sched-1", "attributes": {"name": "Primary On-Call"}}
		],
		"meta": {
			"current_page": 1,
			"total_pages": 1,
			"total_count": 2
		}
	}`, activeStart, activeEnd, futureStart, futureEnd)
}

func emptyShiftsResponse() string {
	return `{
		"data": [],
		"meta": {"current_page": 1, "total_pages": 1, "total_count": 0}
	}`
}

func setupTestServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	viper.Set("api_token", "test-token")
	viper.Set("endpoint", server.URL)
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
		if !strings.Contains(r.URL.Path, "/v1/on_call_schedules") {
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

	cmd := newTestCmd()
	cmd.Flags().Int("page", 1, "")
	cmd.Flags().Int("page-size", 25, "")

	err := runList(cmd, nil)
	if err == nil {
		t.Fatal("expected error when no API token")
	}
	if !strings.Contains(err.Error(), "API token required") {
		t.Errorf("expected 'API token required' error, got: %v", err)
	}
}

// --- runShifts tests ---

func TestRunShiftsTable(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/v1/shifts") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(shiftsResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().Int("days", 7, "")
	cmd.Flags().String("schedule", "", "")
	cmd.Flags().Int("page", 1, "")
	cmd.Flags().Int("page-size", 25, "")

	output := captureStdout(t, func() {
		err := runShifts(cmd, nil)
		if err != nil {
			t.Fatalf("runShifts returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Alice Smith") {
		t.Errorf("expected output to contain 'Alice Smith', got: %s", output)
	}
}

func TestRunShiftsJSON(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(shiftsResponse()))
	})

	viper.Set("format", "json")

	cmd := newTestCmd()
	cmd.Flags().Int("days", 7, "")
	cmd.Flags().String("schedule", "", "")
	cmd.Flags().Int("page", 1, "")
	cmd.Flags().Int("page-size", 25, "")

	output := captureStdout(t, func() {
		err := runShifts(cmd, nil)
		if err != nil {
			t.Fatalf("runShifts returned error: %v", err)
		}
	})

	if !strings.Contains(output, "shift-1") {
		t.Errorf("expected JSON output to contain 'shift-1', got: %s", output)
	}
}

func TestRunShiftsWithScheduleFilter(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.RawQuery
		if !strings.Contains(query, "schedule") {
			t.Errorf("expected schedule filter in query, got: %s", query)
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(shiftsResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().Int("days", 7, "")
	cmd.Flags().String("schedule", "Primary On-Call", "")
	cmd.Flags().Int("page", 1, "")
	cmd.Flags().Int("page-size", 25, "")

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
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(shiftsResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().String("schedule", "", "")

	output := captureStdout(t, func() {
		err := runWho(cmd, nil)
		if err != nil {
			t.Fatalf("runWho returned error: %v", err)
		}
	})

	// Only active shifts should appear in "who" output
	if !strings.Contains(output, "Alice Smith") {
		t.Errorf("expected active user 'Alice Smith' in output, got: %s", output)
	}
}

func TestRunWhoJSON(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(shiftsResponse()))
	})

	viper.Set("format", "json")

	cmd := newTestCmd()
	cmd.Flags().String("schedule", "", "")

	output := captureStdout(t, func() {
		err := runWho(cmd, nil)
		if err != nil {
			t.Fatalf("runWho returned error: %v", err)
		}
	})

	if !strings.Contains(output, "shift-1") {
		t.Errorf("expected JSON output to contain 'shift-1', got: %s", output)
	}
}

func TestRunWhoNoActiveShifts(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(emptyShiftsResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().String("schedule", "", "")

	// No error expected - just prints "no one is on-call" to stderr
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
		if !strings.Contains(query, "schedule") {
			t.Errorf("expected schedule filter in query, got: %s", query)
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(shiftsResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().String("schedule", "Primary On-Call", "")

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

	cmd := newTestCmd()
	cmd.Flags().Int("days", 7, "")
	cmd.Flags().String("schedule", "", "")
	cmd.Flags().Int("page", 1, "")
	cmd.Flags().Int("page-size", 25, "")

	err := runShifts(cmd, nil)
	if err == nil {
		t.Fatal("expected error when no API token")
	}
	if !strings.Contains(err.Error(), "API token required") {
		t.Errorf("expected 'API token required' error, got: %v", err)
	}
}

func TestRunWhoNoToken(t *testing.T) {
	viper.Reset()
	defer viper.Reset()

	cmd := newTestCmd()
	cmd.Flags().String("schedule", "", "")

	err := runWho(cmd, nil)
	if err == nil {
		t.Fatal("expected error when no API token")
	}
}
