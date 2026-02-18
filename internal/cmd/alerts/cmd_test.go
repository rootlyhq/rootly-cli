package alerts

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

func alertListResponse() string {
	return `{
		"data": [{
			"id": "alert-1",
			"attributes": {
				"short_id": "ALR-42",
				"summary": "High CPU usage",
				"status": "triggered",
				"source": "datadog",
				"created_at": "2025-06-15T10:00:00Z"
			}
		}],
		"meta": {"current_page": 1, "total_pages": 1, "total_count": 1}
	}`
}

func alertGetResponse() string {
	return `{
		"data": {
			"id": "alert-1",
			"attributes": {
				"short_id": "ALR-42",
				"summary": "High CPU usage",
				"status": "triggered",
				"source": "datadog",
				"description": "CPU at 95%",
				"url": "https://rootly.com/alerts/42",
				"created_at": "2025-06-15T10:00:00Z"
			}
		}
	}`
}

func alertCreateResponse() string {
	return `{
		"data": {
			"id": "alert-new",
			"attributes": {
				"short_id": "ALR-99",
				"summary": "New alert",
				"status": "triggered",
				"source": "cli",
				"created_at": "2025-06-15T12:00:00Z"
			}
		}
	}`
}

// --- runList tests ---

func TestRunListTable(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(alertListResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().Int("page", 1, "")
	cmd.Flags().Int("page-size", 25, "")
	cmd.Flags().String("sort", "-created_at", "")
	cmd.Flags().String("status", "", "")
	cmd.Flags().String("source", "", "")

	output := captureStdout(t, func() {
		err := runList(cmd, nil)
		if err != nil {
			t.Fatalf("runList error: %v", err)
		}
	})

	if !strings.Contains(output, "High CPU usage") {
		t.Errorf("expected 'High CPU usage' in output, got: %s", output)
	}
}

func TestRunListJSON(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(alertListResponse()))
	})

	viper.Set("format", "json")

	cmd := newTestCmd()
	cmd.Flags().Int("page", 1, "")
	cmd.Flags().Int("page-size", 25, "")
	cmd.Flags().String("sort", "-created_at", "")
	cmd.Flags().String("status", "", "")
	cmd.Flags().String("source", "", "")

	output := captureStdout(t, func() {
		err := runList(cmd, nil)
		if err != nil {
			t.Fatalf("runList error: %v", err)
		}
	})

	if !strings.Contains(output, "alert-1") {
		t.Errorf("expected 'alert-1' in output, got: %s", output)
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
	cmd.Flags().String("source", "", "")

	err := runList(cmd, nil)
	if err == nil {
		t.Fatal("expected error when no API token")
	}
	if !strings.Contains(err.Error(), "API token required") {
		t.Errorf("expected 'API token required' error, got: %v", err)
	}
}

// --- runGet tests ---

func TestRunGetTable(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(alertGetResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()

	output := captureStdout(t, func() {
		err := runGet(cmd, []string{"ALR-42"})
		if err != nil {
			t.Fatalf("runGet error: %v", err)
		}
	})

	if !strings.Contains(output, "High CPU usage") {
		t.Errorf("expected 'High CPU usage' in output, got: %s", output)
	}
}

func TestRunGetJSON(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(alertGetResponse()))
	})

	viper.Set("format", "json")

	cmd := newTestCmd()

	output := captureStdout(t, func() {
		err := runGet(cmd, []string{"ALR-42"})
		if err != nil {
			t.Fatalf("runGet error: %v", err)
		}
	})

	if !strings.Contains(output, "alert-1") {
		t.Errorf("expected 'alert-1' in output, got: %s", output)
	}
}

// --- runCreate tests ---

func TestRunCreateTable(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(alertCreateResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().String("summary", "New alert", "")
	cmd.Flags().String("description", "Test desc", "")
	cmd.Flags().String("source", "datadog", "")
	cmd.Flags().String("status", "triggered", "")
	cmd.Flags().String("external-url", "", "")

	output := captureStdout(t, func() {
		err := runCreate(cmd, nil)
		if err != nil {
			t.Fatalf("runCreate error: %v", err)
		}
	})

	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestRunCreateJSON(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(alertCreateResponse()))
	})

	viper.Set("format", "json")

	cmd := newTestCmd()
	cmd.Flags().String("summary", "New alert", "")
	cmd.Flags().String("description", "", "")
	cmd.Flags().String("source", "", "")
	cmd.Flags().String("status", "", "")
	cmd.Flags().String("external-url", "", "")

	output := captureStdout(t, func() {
		err := runCreate(cmd, nil)
		if err != nil {
			t.Fatalf("runCreate error: %v", err)
		}
	})

	if !strings.Contains(output, "alert-new") {
		t.Errorf("expected 'alert-new' in output, got: %s", output)
	}
}

// --- runUpdate tests ---

func TestRunUpdateTable(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(alertGetResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().String("summary", "", "")
	cmd.Flags().String("description", "", "")
	cmd.Flags().String("source", "", "")
	cmd.Flags().String("status", "", "")
	cmd.Flags().String("external-url", "", "")
	cmd.Flags().Set("status", "acknowledged")

	output := captureStdout(t, func() {
		err := runUpdate(cmd, []string{"ALR-42"})
		if err != nil {
			t.Fatalf("runUpdate error: %v", err)
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
	cmd.Flags().String("summary", "", "")
	cmd.Flags().String("description", "", "")
	cmd.Flags().String("source", "", "")
	cmd.Flags().String("status", "", "")
	cmd.Flags().String("external-url", "", "")

	err := runUpdate(cmd, []string{"ALR-42"})
	if err == nil {
		t.Fatal("expected error when no flags changed")
	}
	if !strings.Contains(err.Error(), "at least one field") {
		t.Errorf("expected 'at least one field' error, got: %v", err)
	}
}
