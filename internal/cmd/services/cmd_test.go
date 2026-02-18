package services

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

func serviceListResponse() string {
	return `{
		"data": [{
			"id": "svc-1",
			"attributes": {
				"name": "API Gateway",
				"slug": "api-gateway",
				"description": "Main API entry point",
				"created_at": "2025-06-15T10:00:00Z"
			}
		}],
		"meta": {"current_page": 1, "total_pages": 1, "total_count": 1}
	}`
}

func serviceGetResponse() string {
	return `{
		"data": {
			"id": "svc-1",
			"attributes": {
				"name": "API Gateway",
				"slug": "api-gateway",
				"description": "Main API entry point",
				"color": "#FF5733",
				"created_at": "2025-06-15T10:00:00Z",
				"updated_at": "2025-06-15T12:00:00Z"
			}
		}
	}`
}

func serviceCreateResponse() string {
	return `{
		"data": {
			"id": "svc-new",
			"attributes": {
				"name": "Payment Service",
				"slug": "payment-service",
				"description": "Payments",
				"created_at": "2025-06-15T12:00:00Z"
			}
		}
	}`
}

// --- runList tests ---

func TestRunListTable(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(serviceListResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().Int("page", 1, "")
	cmd.Flags().Int("page-size", 25, "")
	cmd.Flags().String("sort", "-created_at", "")
	cmd.Flags().String("name", "", "")
	cmd.Flags().String("slug", "", "")

	output := captureStdout(t, func() {
		err := runList(cmd, nil)
		if err != nil {
			t.Fatalf("runList error: %v", err)
		}
	})

	if !strings.Contains(output, "API Gateway") {
		t.Errorf("expected 'API Gateway' in output, got: %s", output)
	}
}

func TestRunListJSON(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(serviceListResponse()))
	})

	viper.Set("format", "json")

	cmd := newTestCmd()
	cmd.Flags().Int("page", 1, "")
	cmd.Flags().Int("page-size", 25, "")
	cmd.Flags().String("sort", "-created_at", "")
	cmd.Flags().String("name", "", "")
	cmd.Flags().String("slug", "", "")

	output := captureStdout(t, func() {
		err := runList(cmd, nil)
		if err != nil {
			t.Fatalf("runList error: %v", err)
		}
	})

	if !strings.Contains(output, "svc-1") {
		t.Errorf("expected 'svc-1' in output, got: %s", output)
	}
}

func TestRunListNoToken(t *testing.T) {
	viper.Reset()
	defer viper.Reset()

	cmd := newTestCmd()
	cmd.Flags().Int("page", 1, "")
	cmd.Flags().Int("page-size", 25, "")
	cmd.Flags().String("sort", "-created_at", "")
	cmd.Flags().String("name", "", "")
	cmd.Flags().String("slug", "", "")

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
		w.Write([]byte(serviceGetResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()

	output := captureStdout(t, func() {
		err := runGet(cmd, []string{"api-gateway"})
		if err != nil {
			t.Fatalf("runGet error: %v", err)
		}
	})

	if !strings.Contains(output, "API Gateway") {
		t.Errorf("expected 'API Gateway' in output, got: %s", output)
	}
}

func TestRunGetJSON(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(serviceGetResponse()))
	})

	viper.Set("format", "json")

	cmd := newTestCmd()

	output := captureStdout(t, func() {
		err := runGet(cmd, []string{"api-gateway"})
		if err != nil {
			t.Fatalf("runGet error: %v", err)
		}
	})

	if !strings.Contains(output, "svc-1") {
		t.Errorf("expected 'svc-1' in output, got: %s", output)
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
		w.Write([]byte(serviceCreateResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().String("name", "Payment Service", "")
	cmd.Flags().String("description", "Payments", "")
	cmd.Flags().String("color", "#FF5733", "")

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

func TestRunCreateInvalidColor(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	cmd := newTestCmd()
	cmd.Flags().String("name", "Test", "")
	cmd.Flags().String("description", "", "")
	cmd.Flags().String("color", "invalid", "")

	err := runCreate(cmd, nil)
	if err == nil {
		t.Fatal("expected error for invalid color format")
	}
	if !strings.Contains(err.Error(), "hex format") {
		t.Errorf("expected 'hex format' error, got: %v", err)
	}
}

// --- runUpdate tests ---

func TestRunUpdateTable(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(serviceGetResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().String("name", "", "")
	cmd.Flags().String("description", "", "")
	cmd.Flags().String("color", "", "")
	cmd.Flags().Set("name", "Updated Name")

	output := captureStdout(t, func() {
		err := runUpdate(cmd, []string{"api-gateway"})
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
	cmd.Flags().String("name", "", "")
	cmd.Flags().String("description", "", "")
	cmd.Flags().String("color", "", "")

	err := runUpdate(cmd, []string{"api-gateway"})
	if err == nil {
		t.Fatal("expected error when no flags changed")
	}
	if !strings.Contains(err.Error(), "at least one field") {
		t.Errorf("expected 'at least one field' error, got: %v", err)
	}
}
