package incidents

import (
	"context"
	"encoding/json"
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

	viper.Set("api_key", "test-token")
	viper.Set("api_host", server.URL)
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

func TestRunCreateScheduledMaintenance(t *testing.T) {
	var attributes map[string]interface{}
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/incidents" {
			t.Errorf("request = %s %s, want POST /v1/incidents", r.Method, r.URL.Path)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request: %v", err)
			return
		}
		attributes = body["data"].(map[string]interface{})["attributes"].(map[string]interface{})
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(createResponse()))
	})
	viper.Set("format", "json")

	cmd := newTestCmd()
	cmd.Flags().String("title", "", "")
	cmd.Flags().String("kind", "", "")
	cmd.Flags().String("scheduled-for", "", "")
	cmd.Flags().String("scheduled-until", "", "")
	for flag, value := range map[string]string{
		"title":           "Maintenance",
		"kind":            "scheduled",
		"scheduled-for":   "2025-06-20T10:00:00+02:00",
		"scheduled-until": "2025-06-20T12:00:00+02:00",
	} {
		if err := cmd.Flags().Set(flag, value); err != nil {
			t.Fatal(err)
		}
	}

	captureStdout(t, func() {
		if err := runCreate(cmd, nil); err != nil {
			t.Fatalf("runCreate returned error: %v", err)
		}
	})

	if attributes["kind"] != "scheduled" || attributes["scheduled_for"] != "2025-06-20T10:00:00+02:00" || attributes["scheduled_until"] != "2025-06-20T12:00:00+02:00" {
		t.Errorf("scheduled attributes = %#v", attributes)
	}
}

func TestRunUpdateScheduledMaintenance(t *testing.T) {
	var attributes map[string]interface{}
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/v1/incidents/42" {
			t.Errorf("request = %s %s, want PUT /v1/incidents/42", r.Method, r.URL.Path)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request: %v", err)
			return
		}
		attributes = body["data"].(map[string]interface{})["attributes"].(map[string]interface{})
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(getResponse()))
	})
	viper.Set("format", "json")

	cmd := newTestCmd()
	cmd.Flags().String("kind", "", "")
	cmd.Flags().String("scheduled-until", "", "")
	if err := cmd.Flags().Set("kind", "scheduled"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("scheduled-until", "2025-06-20T12:00:00+02:00"); err != nil {
		t.Fatal(err)
	}

	captureStdout(t, func() {
		if err := runUpdate(cmd, []string{"INC-42"}); err != nil {
			t.Fatalf("runUpdate returned error: %v", err)
		}
	})

	if attributes["kind"] != "scheduled" || attributes["scheduled_until"] != "2025-06-20T12:00:00+02:00" {
		t.Errorf("scheduled attributes = %#v", attributes)
	}
}

func TestRunCreateWithFormFields(t *testing.T) {
	var selections []interface{}
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/form_fields":
			if r.URL.Query().Get("include") != "options" {
				t.Errorf("form-fields include = %q, want options", r.URL.Query().Get("include"))
			}
			_, _ = w.Write([]byte(incidentFormFieldsResponse()))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/incidents":
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("failed to decode create request: %v", err)
				return
			}
			attributes := body["data"].(map[string]interface{})["attributes"].(map[string]interface{})
			selections = attributes["form_field_selections"].([]interface{})
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(createResponse()))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	viper.Set("format", "json")

	cmd := newTestCmd()
	cmd.Flags().String("title", "", "")
	cmd.Flags().StringArray("field", nil, "")
	if err := cmd.Flags().Set("title", "Custom fields"); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"description=Cache, database", "priority=p2"} {
		if err := cmd.Flags().Set("field", value); err != nil {
			t.Fatal(err)
		}
	}

	captureStdout(t, func() {
		if err := runCreate(cmd, nil); err != nil {
			t.Fatalf("runCreate returned error: %v", err)
		}
	})

	if len(selections) != 2 {
		t.Fatalf("form field selections = %#v, want 2 selections", selections)
	}
	textSelection := selections[0].(map[string]interface{})
	if textSelection["form_field_id"] != "text-id" || textSelection["value"] != "Cache, database" {
		t.Errorf("text selection = %#v", textSelection)
	}
	optionSelection := selections[1].(map[string]interface{})
	optionIDs := optionSelection["selected_option_ids"].([]interface{})
	if optionSelection["form_field_id"] != "select-id" || len(optionIDs) != 1 || optionIDs[0] != "p2-id" {
		t.Errorf("option selection = %#v", optionSelection)
	}
}

func TestRunUpdateWithFormField(t *testing.T) {
	var selection map[string]interface{}
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/form_fields":
			_, _ = w.Write([]byte(incidentFormFieldsResponse()))
		case r.Method == http.MethodPut && r.URL.Path == "/v1/incidents/42":
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("failed to decode update request: %v", err)
				return
			}
			attributes := body["data"].(map[string]interface{})["attributes"].(map[string]interface{})
			selection = attributes["form_field_selections"].([]interface{})[0].(map[string]interface{})
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(getResponse()))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	viper.Set("format", "json")

	cmd := newTestCmd()
	cmd.Flags().StringArray("field", nil, "")
	if err := cmd.Flags().Set("field", "description=Updated value"); err != nil {
		t.Fatal(err)
	}
	captureStdout(t, func() {
		if err := runUpdate(cmd, []string{"INC-42"}); err != nil {
			t.Fatalf("runUpdate returned error: %v", err)
		}
	})
	if selection["form_field_id"] != "text-id" || selection["value"] != "Updated value" {
		t.Errorf("form field selection = %#v", selection)
	}
}

func TestRunCreateUnknownFormFieldDoesNotCreateIncident(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/form_fields" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(incidentFormFieldsResponse()))
	})
	cmd := newTestCmd()
	cmd.Flags().String("title", "Unknown field", "")
	cmd.Flags().StringArray("field", nil, "")
	if err := cmd.Flags().Set("field", "missing=value"); err != nil {
		t.Fatal(err)
	}
	err := runCreate(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "rootly form-fields list") {
		t.Fatalf("error = %v, want form-fields suggestion", err)
	}
}

func incidentFormFieldsResponse() string {
	return `{
		"data": [{
			"id": "text-id",
			"attributes": {"slug": "description", "name": "Description", "kind": "custom_field", "input_kind": "text", "value_kind": "inherit", "enabled": true}
		}, {
			"id": "select-id",
			"attributes": {"slug": "priority", "name": "Priority", "kind": "custom_field", "input_kind": "select", "value_kind": "inherit", "enabled": true}
		}],
		"included": [{
			"id": "p1-id", "type": "form_field_options", "attributes": {"form_field_id": "select-id", "value": "P1"}
		}, {
			"id": "p2-id", "type": "form_field_options", "attributes": {"form_field_id": "select-id", "value": "P2"}
		}],
		"meta": {"current_page": 1, "total_pages": 1, "total_count": 2}
	}`
}

func TestParseScheduledTimestamp(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "fractional seconds",
			input: "2025-06-20T10:00:00.500Z",
			want:  "2025-06-20T10:00:00.5Z",
		},
		{
			name:  "whole seconds",
			input: "2025-06-20T10:00:00Z",
			want:  "2025-06-20T10:00:00Z",
		},
		{
			name:  "offset",
			input: "2025-06-20T10:00:00+02:00",
			want:  "2025-06-20T10:00:00+02:00",
		},
		{
			name:    "invalid timestamp",
			input:   "not-a-timestamp",
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := newTestCmd()
			cmd.Flags().String("scheduled-for", "", "")
			if err := cmd.Flags().Set("scheduled-for", test.input); err != nil {
				t.Fatal(err)
			}

			got, err := parseScheduledTimestamp(cmd, "scheduled-for")
			if test.wantErr {
				if err == nil || !strings.Contains(err.Error(), "expected an RFC3339 timestamp") {
					t.Errorf("parseScheduledTimestamp error = %v, want clear RFC3339 error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseScheduledTimestamp returned error: %v", err)
			}
			if got != test.want {
				t.Errorf("scheduled timestamp = %q, want %q", got, test.want)
			}
		})
	}
}

func TestIncidentKindHelpIsConsistent(t *testing.T) {
	want := "Incident kind: normal, test, scheduled, backfilled, example"
	for _, command := range []*cobra.Command{createCmd, updateCmd} {
		flag := command.Flags().Lookup("kind")
		if flag == nil || flag.Usage != want {
			t.Errorf("%s --kind help = %q, want %q", command.Name(), flag.Usage, want)
		}
	}
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
	cmd.Flags().StringSlice("services", []string{"api-gateway", "payments"}, "")
	cmd.Flags().StringSlice("types", []string{"customer-impacting"}, "")
	cmd.Flags().StringSlice("functionalities", []string{"checkout"}, "")
	cmd.Flags().StringSlice("environments", []string{"production"}, "")
	cmd.Flags().StringSlice("teams", []string{"platform"}, "")
	cmd.Flags().StringSlice("causes", []string{"deployment"}, "")

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
	cmd.Flags().StringSlice("services", nil, "")
	cmd.Flags().StringSlice("types", nil, "")
	cmd.Flags().StringSlice("functionalities", nil, "")
	cmd.Flags().StringSlice("environments", nil, "")
	cmd.Flags().StringSlice("teams", nil, "")
	cmd.Flags().StringSlice("causes", nil, "")

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
	cmd.Flags().StringSlice("services", nil, "")
	cmd.Flags().StringSlice("types", nil, "")
	cmd.Flags().StringSlice("functionalities", nil, "")
	cmd.Flags().StringSlice("environments", nil, "")
	cmd.Flags().StringSlice("teams", nil, "")
	cmd.Flags().StringSlice("causes", nil, "")

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
	cmd.Flags().StringSlice("services", nil, "")
	cmd.Flags().StringSlice("types", nil, "")
	cmd.Flags().StringSlice("functionalities", nil, "")
	cmd.Flags().StringSlice("environments", nil, "")
	cmd.Flags().StringSlice("teams", nil, "")
	cmd.Flags().StringSlice("causes", nil, "")

	err := runUpdate(cmd, []string{"INC-42"})
	if err == nil {
		t.Fatal("expected error when no flags changed")
	}
	if !strings.Contains(err.Error(), "at least one field") {
		t.Errorf("expected 'at least one field' error, got: %v", err)
	}
}

func TestRunUpdateCanClearAssociations(t *testing.T) {
	var attributes map[string]interface{}
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var requestBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		attributes = requestBody["data"].(map[string]interface{})["attributes"].(map[string]interface{})
		w.Header().Set("Content-Type", "application/vnd.api+json")
		_, _ = w.Write([]byte(getResponse()))
	})
	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().String("title", "", "")
	cmd.Flags().String("summary", "", "")
	cmd.Flags().String("severity", "", "")
	cmd.Flags().String("status", "", "")
	cmd.Flags().StringSlice("services", nil, "")
	cmd.Flags().StringSlice("types", nil, "")
	cmd.Flags().StringSlice("functionalities", nil, "")
	cmd.Flags().StringSlice("environments", nil, "")
	cmd.Flags().StringSlice("teams", nil, "")
	cmd.Flags().StringSlice("causes", nil, "")
	if err := cmd.Flags().Set("services", ""); err != nil {
		t.Fatalf("failed to set empty services: %v", err)
	}

	captureStdout(t, func() {
		if err := runUpdate(cmd, []string{"INC-42"}); err != nil {
			t.Fatalf("runUpdate returned error: %v", err)
		}
	})
	services, ok := attributes["service_ids"].([]interface{})
	if !ok || len(services) != 0 {
		t.Errorf("service_ids = %#v, want an explicitly empty array", attributes["service_ids"])
	}
	if _, ok := attributes["incident_type_ids"]; ok {
		t.Error("incident_type_ids should be absent when --types is omitted")
	}
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
	cmd.Flags().String("sort", "-created_at", "")
	cmd.Flags().String("status", "", "")
	cmd.Flags().String("severity", "", "")

	err := runList(cmd, nil)
	if err == nil {
		t.Fatal("expected error when no API token")
	}
	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("expected 'authentication required' error, got: %v", err)
	}
}

func TestRunGetNormalizesSequentialID(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/incidents/42" {
			t.Errorf("expected path /v1/incidents/42, got %s", r.URL.Path)
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

func TestRunGetAcceptsUUID(t *testing.T) {
	uuid := "e5923856-6fe8-4a2c-b0eb-cb783e811d06"
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/incidents/"+uuid {
			t.Errorf("expected UUID in path, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(getResponse()))
	})

	viper.Set("format", "table")
	cmd := newTestCmd()

	captureStdout(t, func() {
		err := runGet(cmd, []string{uuid})
		if err != nil {
			t.Fatalf("runGet returned error: %v", err)
		}
	})
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
		err := runDelete(cmd, []string{"INC-42"})
		if err != nil {
			t.Fatalf("runDelete error: %v", err)
		}
	})

	if !strings.Contains(output, "Deleted incident INC-42") {
		t.Errorf("expected success message, got: %s", output)
	}
}

func TestRunDeleteNoConfirmNonInteractive(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	cmd := newTestCmd()
	cmd.Flags().BoolP("yes", "y", false, "")

	err := runDelete(cmd, []string{"INC-42"})
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
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)

	cmd := newTestCmd()
	cmd.Flags().BoolP("yes", "y", false, "")
	cmd.Flags().Set("yes", "true")

	err := runDelete(cmd, []string{"INC-42"})
	if err == nil {
		t.Fatal("expected error when no API token")
	}
	if !strings.Contains(err.Error(), "authentication required") {
		t.Errorf("expected 'authentication required' error, got: %v", err)
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

	err := runDelete(cmd, []string{"INC-999"})
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "failed to delete incident") {
		t.Errorf("expected 'failed to delete incident' error, got: %v", err)
	}
}
