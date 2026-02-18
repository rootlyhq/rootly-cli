package incidents

import (
	"context"
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

// listResponse returns a minimal valid JSON:API list response for incidents.
func listResponse() string {
	return `{
		"data": [{
			"id": "inc-1",
			"attributes": {
				"title": "DB outage",
				"status": "started",
				"kind": "normal",
				"created_at": "2025-06-15T10:00:00Z",
				"severity": {
					"data": {
						"attributes": {"name": "sev0"}
					}
				},
				"services": {
					"data": [{
						"attributes": {"name": "api-gateway"}
					}]
				}
			}
		}],
		"meta": {
			"current_page": 1,
			"total_pages": 1,
			"total_count": 1
		}
	}`
}

// getResponse returns a minimal valid JSON:API detail response for an incident.
func getResponse() string {
	return `{
		"data": {
			"id": "inc-1",
			"attributes": {
				"sequential_id": 42,
				"title": "DB outage",
				"status": "started",
				"kind": "normal",
				"created_at": "2025-06-15T10:00:00Z"
			}
		}
	}`
}

// createResponse returns a minimal valid JSON:API create response.
func createResponse() string {
	return `{
		"data": {
			"id": "inc-new",
			"attributes": {
				"sequential_id": 99,
				"title": "New incident",
				"status": "started",
				"kind": "normal",
				"created_at": "2025-06-15T12:00:00Z"
			}
		}
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

// captureStdout redirects os.Stdout for the duration of fn, returning captured output.
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

func TestRunListTable(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/v1/incidents") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(listResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().Int("page", 1, "")
	cmd.Flags().Int("page-size", 25, "")
	cmd.Flags().String("sort", "-created_at", "")
	cmd.Flags().String("status", "", "")
	cmd.Flags().String("severity", "", "")

	output := captureStdout(t, func() {
		err := runList(cmd, nil)
		if err != nil {
			t.Fatalf("runList returned error: %v", err)
		}
	})

	if !strings.Contains(output, "DB outage") {
		t.Errorf("expected output to contain 'DB outage', got: %s", output)
	}
}

func TestRunListJSON(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(listResponse()))
	})

	viper.Set("format", "json")

	cmd := newTestCmd()
	cmd.Flags().Int("page", 1, "")
	cmd.Flags().Int("page-size", 25, "")
	cmd.Flags().String("sort", "-created_at", "")
	cmd.Flags().String("status", "", "")
	cmd.Flags().String("severity", "", "")

	output := captureStdout(t, func() {
		err := runList(cmd, nil)
		if err != nil {
			t.Fatalf("runList returned error: %v", err)
		}
	})

	if !strings.Contains(output, "inc-1") {
		t.Errorf("expected JSON output to contain 'inc-1', got: %s", output)
	}
}

func TestRunListWithFilters(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.RawQuery
		if !strings.Contains(query, "filter[status]=started") {
			t.Errorf("expected status filter in query, got: %s", query)
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(listResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().Int("page", 1, "")
	cmd.Flags().Int("page-size", 25, "")
	cmd.Flags().String("sort", "-created_at", "")
	cmd.Flags().String("status", "started", "")
	cmd.Flags().String("severity", "", "")

	captureStdout(t, func() {
		err := runList(cmd, nil)
		if err != nil {
			t.Fatalf("runList returned error: %v", err)
		}
	})
}

func TestRunListPagination(t *testing.T) {
	resp := `{
		"data": [{"id": "inc-1", "attributes": {"title": "Test", "status": "started", "kind": "normal", "created_at": "2025-01-01T00:00:00Z"}}],
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
	cmd.Flags().String("sort", "-created_at", "")
	cmd.Flags().String("status", "", "")
	cmd.Flags().String("severity", "", "")

	captureStdout(t, func() {
		err := runList(cmd, nil)
		if err != nil {
			t.Fatalf("runList returned error: %v", err)
		}
	})
	// Pagination message goes to stderr, which we don't capture here, but the code path is exercised
}

func TestRunGetTable(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/v1/incidents/") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(getResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()

	output := captureStdout(t, func() {
		err := runGet(cmd, []string{"INC-42"})
		if err != nil {
			t.Fatalf("runGet returned error: %v", err)
		}
	})

	if !strings.Contains(output, "DB outage") {
		t.Errorf("expected output to contain 'DB outage', got: %s", output)
	}
}

func TestRunGetJSON(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(getResponse()))
	})

	viper.Set("format", "json")

	cmd := newTestCmd()

	output := captureStdout(t, func() {
		err := runGet(cmd, []string{"INC-42"})
		if err != nil {
			t.Fatalf("runGet returned error: %v", err)
		}
	})

	if !strings.Contains(output, "inc-1") {
		t.Errorf("expected JSON output to contain 'inc-1', got: %s", output)
	}
}

func TestRunCreateTable(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(createResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().String("title", "New incident", "")
	cmd.Flags().String("summary", "Test summary", "")
	cmd.Flags().String("severity", "", "")
	cmd.Flags().String("status", "started", "")

	output := captureStdout(t, func() {
		err := runCreate(cmd, nil)
		if err != nil {
			t.Fatalf("runCreate returned error: %v", err)
		}
	})

	// Table PrintObj output should contain some content
	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestRunCreateJSON(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(createResponse()))
	})

	viper.Set("format", "json")

	cmd := newTestCmd()
	cmd.Flags().String("title", "New incident", "")
	cmd.Flags().String("summary", "", "")
	cmd.Flags().String("severity", "", "")
	cmd.Flags().String("status", "", "")

	output := captureStdout(t, func() {
		err := runCreate(cmd, nil)
		if err != nil {
			t.Fatalf("runCreate returned error: %v", err)
		}
	})

	if !strings.Contains(output, "inc-new") {
		t.Errorf("expected JSON output to contain 'inc-new', got: %s", output)
	}
}

func TestRunUpdateTable(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(getResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().String("title", "", "")
	cmd.Flags().String("summary", "", "")
	cmd.Flags().String("severity", "", "")
	cmd.Flags().String("status", "", "")

	// Simulate the user passing --status=mitigated
	cmd.Flags().Set("status", "mitigated")

	output := captureStdout(t, func() {
		err := runUpdate(cmd, []string{"INC-42"})
		if err != nil {
			t.Fatalf("runUpdate returned error: %v", err)
		}
	})

	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestRunUpdateNoFlags(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	cmd := newTestCmd()
	cmd.Flags().String("title", "", "")
	cmd.Flags().String("summary", "", "")
	cmd.Flags().String("severity", "", "")
	cmd.Flags().String("status", "", "")

	err := runUpdate(cmd, []string{"INC-42"})
	if err == nil {
		t.Fatal("expected error when no flags changed")
	}
	if !strings.Contains(err.Error(), "at least one field") {
		t.Errorf("expected 'at least one field' error, got: %v", err)
	}
}

func TestRunListNoToken(t *testing.T) {
	viper.Reset()
	defer viper.Reset()

	cmd := newTestCmd()
	cmd.Flags().Int("page", 1, "")
	cmd.Flags().Int("page-size", 25, "")
	cmd.Flags().String("sort", "-created_at", "")
	cmd.Flags().String("status", "", "")
	cmd.Flags().String("severity", "", "")

	err := runList(cmd, nil)
	if err == nil {
		t.Fatal("expected error when no API token")
	}
	if !strings.Contains(err.Error(), "API token required") {
		t.Errorf("expected 'API token required' error, got: %v", err)
	}
}

func TestRunGetAPIError(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"errors": [{"title": "Not Found"}]}`))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()

	err := runGet(cmd, []string{"INC-999"})
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "failed to get incident") {
		t.Errorf("expected 'failed to get incident' error, got: %v", err)
	}
}
