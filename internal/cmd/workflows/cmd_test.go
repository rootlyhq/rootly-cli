package workflows

import (
	"context"
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

func TestRunList(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"data": [{"id": "workflow-1", "attributes": {
				"name": "Create retrospective", "slug": "retrospective", "enabled": true,
				"created_at": "2026-08-12T12:00:00Z", "updated_at": "2026-08-12T12:00:00Z"
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
	if !strings.Contains(output, "Create retrospective") {
		t.Errorf("expected workflow in output, got: %s", output)
	}
}

func TestRunWorkflowResolvesSlugAndNormalizesIncident(t *testing.T) {
	requestCount := 0
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{
				"data": [{"id": "workflow-1", "attributes": {"name": "Retrospective", "slug": "retrospective"}}],
				"meta": {"current_page": 1, "total_pages": 1, "total_count": 1}
			}`))
			return
		}
		if r.URL.Path != "/v1/workflows/workflow-1/workflow_runs" {
			t.Errorf("unexpected run path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"data": {"id": "run-1", "attributes": {
				"workflow_id": "workflow-1", "incident_id": "42", "status": "pending", "triggered_by": "api"
			}}
		}`))
	})
	viper.Set("format", "json")
	cmd := newTestCmd()
	cmd.Flags().String("incident", "INC-42", "")

	output := captureStdout(t, func() {
		if err := runWorkflow(cmd, []string{"retrospective"}); err != nil {
			t.Fatalf("runWorkflow returned error: %v", err)
		}
	})
	if requestCount != 2 {
		t.Errorf("request count = %d, want 2", requestCount)
	}
	if !strings.Contains(output, "run-1") {
		t.Errorf("expected run response, got: %s", output)
	}
}
