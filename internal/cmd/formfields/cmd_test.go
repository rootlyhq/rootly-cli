package formfields

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
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = writer
	fn()
	_ = writer.Close()
	os.Stdout = original
	output, _ := io.ReadAll(reader)
	_ = reader.Close()
	return string(output)
}

func TestRunListTable(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/form_fields" || r.URL.Query().Get("include") != "options" {
			t.Errorf("unexpected request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(formFieldsResponse()))
	})
	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().Int("page", 1, "")
	cmd.Flags().Int("page-size", 25, "")
	output := captureStdout(t, func() {
		if err := runList(cmd, nil); err != nil {
			t.Fatalf("runList returned error: %v", err)
		}
	})
	if !strings.Contains(output, "Risk") || !strings.Contains(output, "Low, High") {
		t.Errorf("form field output = %q, want name and options", output)
	}
}

func TestRunListJSON(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(formFieldsResponse()))
	})
	viper.Set("format", "json")
	cmd := newTestCmd()
	cmd.Flags().Int("page", 1, "")
	cmd.Flags().Int("page-size", 25, "")
	output := captureStdout(t, func() {
		if err := runList(cmd, nil); err != nil {
			t.Fatalf("runList returned error: %v", err)
		}
	})
	if !strings.Contains(output, `"form_field_options"`) {
		t.Errorf("JSON output = %q, want included options", output)
	}
}

func formFieldsResponse() string {
	return `{
		"data": [{
			"id": "field-1",
			"attributes": {
				"slug": "risk",
				"name": "Risk",
				"kind": "custom_field",
				"input_kind": "select",
				"value_kind": "inherit",
				"enabled": true
			}
		}],
		"included": [{
			"id": "option-low",
			"type": "form_field_options",
			"attributes": {"form_field_id": "field-1", "value": "Low"}
		}, {
			"id": "option-high",
			"type": "form_field_options",
			"attributes": {"form_field_id": "field-1", "value": "High"}
		}],
		"meta": {"current_page": 1, "total_pages": 1, "total_count": 1}
	}`
}
