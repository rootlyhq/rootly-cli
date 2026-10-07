package incidents

import (
	"strings"
	"testing"

	"github.com/rootlyhq/rootly-cli/internal/api"
)

func TestResolveFormFieldSelections(t *testing.T) {
	fields := []api.FormField{
		{ID: "text-id", Slug: "description", InputKind: "text", ValueKind: "inherit"},
		{
			ID: "select-id", Slug: "priority", InputKind: "select", ValueKind: "inherit",
			Options: []api.FormFieldOption{{ID: "p1", Value: "P1"}, {ID: "p2", Value: "P2"}},
		},
		{
			ID: "multi-id", Slug: "regions", InputKind: "multi_select", ValueKind: "inherit",
			Options: []api.FormFieldOption{{ID: "us", Value: "US East"}, {ID: "eu", Value: "EU West"}},
		},
		{ID: "service-id", Slug: "service", InputKind: "select", ValueKind: "service"},
	}

	tests := []struct {
		name     string
		inputs   []string
		wantErr  string
		validate func(*testing.T, []map[string]interface{})
	}{
		{
			name:   "text preserves commas in value",
			inputs: []string{"description=Database, cache, and API"},
			validate: func(t *testing.T, selections []map[string]interface{}) {
				t.Helper()
				if len(selections) != 1 || selections[0]["form_field_id"] != "text-id" || selections[0]["value"] != "Database, cache, and API" {
					t.Errorf("selections = %#v", selections)
				}
			},
		},
		{
			name:   "select resolves option case insensitively",
			inputs: []string{"priority=p2"},
			validate: func(t *testing.T, selections []map[string]interface{}) {
				t.Helper()
				options := selections[0]["selected_option_ids"].([]string)
				if selections[0]["form_field_id"] != "select-id" || len(options) != 1 || options[0] != "p2" {
					t.Errorf("selections = %#v", selections)
				}
			},
		},
		{
			name:   "multi select accumulates repeated flags",
			inputs: []string{"regions=us east", "regions=EU WEST", "regions=US East"},
			validate: func(t *testing.T, selections []map[string]interface{}) {
				t.Helper()
				options := selections[0]["selected_option_ids"].([]string)
				if selections[0]["form_field_id"] != "multi-id" || len(options) != 2 || options[0] != "us" || options[1] != "eu" {
					t.Errorf("selections = %#v", selections)
				}
			},
		},
		{name: "unknown slug", inputs: []string{"missing=value"}, wantErr: "rootly form-fields list"},
		{name: "unknown option", inputs: []string{"priority=p3"}, wantErr: "valid options: P1, P2"},
		{name: "unsupported value kind", inputs: []string{"service=api"}, wantErr: `field "service" (select/service) is not supported by --field yet`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selections, err := resolveFormFieldSelections(fields, tt.inputs)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveFormFieldSelections returned error: %v", err)
			}
			tt.validate(t, selections)
		})
	}
}

func TestResolveFormFieldSelectionsRejectsMultipleSingleSelectValues(t *testing.T) {
	field := api.FormField{
		ID: "select-id", Slug: "priority", InputKind: "select", ValueKind: "inherit",
		Options: []api.FormFieldOption{{ID: "p1", Value: "P1"}, {ID: "p2", Value: "P2"}},
	}
	_, err := resolveFormFieldSelections([]api.FormField{field}, []string{"priority=P1", "priority=P2"})
	if err == nil || !strings.Contains(err.Error(), "single-select") {
		t.Fatalf("error = %v, want single-select validation", err)
	}
}
