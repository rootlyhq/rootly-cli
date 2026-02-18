package teams

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

func teamListResponse() string {
	return `{
		"data": [{
			"id": "team-1",
			"attributes": {
				"name": "Platform",
				"slug": "platform",
				"description": "Platform engineering",
				"created_at": "2025-06-15T10:00:00Z"
			}
		}],
		"meta": {"current_page": 1, "total_pages": 1, "total_count": 1}
	}`
}

func teamGetResponse() string {
	return `{
		"data": {
			"id": "team-1",
			"attributes": {
				"name": "Platform",
				"slug": "platform",
				"description": "Platform engineering",
				"color": "#3498DB",
				"created_at": "2025-06-15T10:00:00Z",
				"updated_at": "2025-06-15T12:00:00Z"
			}
		}
	}`
}

func teamCreateResponse() string {
	return `{
		"data": {
			"id": "team-new",
			"attributes": {
				"name": "Security",
				"slug": "security",
				"description": "Security team",
				"created_at": "2025-06-15T12:00:00Z"
			}
		}
	}`
}

// --- runList tests ---

func TestRunListTable(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(teamListResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().Int("page", 1, "")
	cmd.Flags().Int("page-size", 25, "")
	cmd.Flags().String("sort", "-created_at", "")
	cmd.Flags().String("name", "", "")

	output := captureStdout(t, func() {
		err := runList(cmd, nil)
		if err != nil {
			t.Fatalf("runList error: %v", err)
		}
	})

	if !strings.Contains(output, "Platform") {
		t.Errorf("expected 'Platform' in output, got: %s", output)
	}
}

func TestRunListJSON(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(teamListResponse()))
	})

	viper.Set("format", "json")

	cmd := newTestCmd()
	cmd.Flags().Int("page", 1, "")
	cmd.Flags().Int("page-size", 25, "")
	cmd.Flags().String("sort", "-created_at", "")
	cmd.Flags().String("name", "", "")

	output := captureStdout(t, func() {
		err := runList(cmd, nil)
		if err != nil {
			t.Fatalf("runList error: %v", err)
		}
	})

	if !strings.Contains(output, "team-1") {
		t.Errorf("expected 'team-1' in output, got: %s", output)
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

	err := runList(cmd, nil)
	if err == nil {
		t.Fatal("expected error when no API token")
	}
	if !strings.Contains(err.Error(), "API key required") {
		t.Errorf("expected 'API key required' error, got: %v", err)
	}
}

// --- runGet tests ---

func TestRunGetTable(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(teamGetResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()

	output := captureStdout(t, func() {
		err := runGet(cmd, []string{"platform"})
		if err != nil {
			t.Fatalf("runGet error: %v", err)
		}
	})

	if !strings.Contains(output, "Platform") {
		t.Errorf("expected 'Platform' in output, got: %s", output)
	}
}

func TestRunGetJSON(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(teamGetResponse()))
	})

	viper.Set("format", "json")

	cmd := newTestCmd()

	output := captureStdout(t, func() {
		err := runGet(cmd, []string{"platform"})
		if err != nil {
			t.Fatalf("runGet error: %v", err)
		}
	})

	if !strings.Contains(output, "team-1") {
		t.Errorf("expected 'team-1' in output, got: %s", output)
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
		w.Write([]byte(teamCreateResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().String("name", "Security", "")
	cmd.Flags().String("description", "Security team", "")
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
		w.Write([]byte(teamGetResponse()))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().String("name", "", "")
	cmd.Flags().String("description", "", "")
	cmd.Flags().String("color", "", "")
	cmd.Flags().Set("name", "Updated Name")

	output := captureStdout(t, func() {
		err := runUpdate(cmd, []string{"platform"})
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

	err := runUpdate(cmd, []string{"platform"})
	if err == nil {
		t.Fatal("expected error when no flags changed")
	}
	if !strings.Contains(err.Error(), "at least one field") {
		t.Errorf("expected 'at least one field' error, got: %v", err)
	}
}

// --- confirmDelete tests ---

func TestConfirmDeleteSkip(t *testing.T) {
	err := confirmDelete("Delete?", true)
	if err != nil {
		t.Fatalf("expected nil error when skipConfirm=true, got: %v", err)
	}
}

func TestConfirmDeleteNonInteractive(t *testing.T) {
	err := confirmDelete("Delete?", false)
	if err == nil {
		t.Fatal("expected error in non-interactive mode")
	}
	if !strings.Contains(err.Error(), "cannot prompt in non-interactive mode") {
		t.Errorf("expected non-interactive error, got: %v", err)
	}
}

// --- runDelete tests ---

func TestRunDeleteWithYes(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	cmd := newTestCmd()
	cmd.Flags().BoolP("yes", "y", false, "")
	cmd.Flags().Set("yes", "true")

	output := captureStdout(t, func() {
		err := runDelete(cmd, []string{"engineering"})
		if err != nil {
			t.Fatalf("runDelete error: %v", err)
		}
	})

	if !strings.Contains(output, "Deleted team engineering") {
		t.Errorf("expected success message, got: %s", output)
	}
}

func TestRunDeleteNoConfirmNonInteractive(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	cmd := newTestCmd()
	cmd.Flags().BoolP("yes", "y", false, "")

	err := runDelete(cmd, []string{"engineering"})
	if err == nil {
		t.Fatal("expected error in non-interactive mode without --yes")
	}
	if !strings.Contains(err.Error(), "cannot prompt in non-interactive mode") {
		t.Errorf("expected non-interactive error, got: %v", err)
	}
}

func TestRunDeleteNoToken(t *testing.T) {
	viper.Reset()
	defer viper.Reset()

	cmd := newTestCmd()
	cmd.Flags().BoolP("yes", "y", false, "")
	cmd.Flags().Set("yes", "true")

	err := runDelete(cmd, []string{"engineering"})
	if err == nil {
		t.Fatal("expected error when no API token")
	}
	if !strings.Contains(err.Error(), "API key required") {
		t.Errorf("expected 'API key required' error, got: %v", err)
	}
}

func TestRunDeleteAPIError(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"errors": [{"title": "Not Found"}]}`))
	})

	cmd := newTestCmd()
	cmd.Flags().BoolP("yes", "y", false, "")
	cmd.Flags().Set("yes", "true")

	err := runDelete(cmd, []string{"team-999"})
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "failed to delete team") {
		t.Errorf("expected 'failed to delete team' error, got: %v", err)
	}
}
