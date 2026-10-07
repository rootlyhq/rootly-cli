package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListFormFieldsCLI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/form_fields" {
			t.Errorf("request = %s %s, want GET /v1/form_fields", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("page[number]") != "2" || r.URL.Query().Get("page[size]") != "10" {
			t.Errorf("pagination query = %s", r.URL.RawQuery)
		}
		if r.URL.Query().Get("include") != "options" {
			t.Errorf("include = %q, want options", r.URL.Query().Get("include"))
		}
		_, _ = w.Write([]byte(`{
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
				"id": "option-1",
				"type": "form_field_options",
				"attributes": {"form_field_id": "field-1", "value": "Low"}
			}, {
				"id": "option-ignored",
				"type": "form_fields",
				"attributes": {"form_field_id": "field-1", "value": "Ignored"}
			}],
			"meta": {"current_page": 2, "total_pages": 3, "total_count": 25}
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.ListFormFieldsCLI(context.Background(), 2, 10)
	if err != nil {
		t.Fatalf("ListFormFieldsCLI returned error: %v", err)
	}
	if len(result.FormFields) != 1 {
		t.Fatalf("form fields = %d, want 1", len(result.FormFields))
	}
	field := result.FormFields[0]
	if field.ID != "field-1" || field.Slug != "risk" || field.Name != "Risk" || field.Kind != "custom_field" || field.InputKind != "select" || field.ValueKind != "inherit" || !field.Enabled {
		t.Errorf("form field = %+v", field)
	}
	if len(field.Options) != 1 || field.Options[0] != (FormFieldOption{ID: "option-1", Value: "Low"}) {
		t.Errorf("options = %+v, want option-1 Low", field.Options)
	}
	if result.Pagination.CurrentPage != 2 || result.Pagination.TotalPages != 3 || !result.Pagination.HasNext {
		t.Errorf("pagination = %+v", result.Pagination)
	}
}

func TestListAllFormFieldsCLIPaginates(t *testing.T) {
	var receivedPages []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPages = append(receivedPages, r.URL.Query().Get("page[number]"))
		if r.URL.Query().Get("include") != "options" {
			t.Errorf("include = %q, want options", r.URL.Query().Get("include"))
		}
		page := r.URL.Query().Get("page[number]")
		if page == "1" {
			_, _ = w.Write([]byte(`{
				"data": [{"id": "field-1", "attributes": {"slug": "first", "name": "First", "kind": "custom_field", "input_kind": "text", "value_kind": "inherit"}}],
				"meta": {"current_page": 1, "total_pages": 2, "total_count": 2}
			}`))
			return
		}
		_, _ = w.Write([]byte(`{
			"data": [{"id": "field-2", "attributes": {"slug": "second", "name": "Second", "kind": "custom_field", "input_kind": "text", "value_kind": "inherit"}}],
			"meta": {"current_page": 2, "total_pages": 2, "total_count": 2}
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	fields, err := client.ListAllFormFieldsCLI(context.Background())
	if err != nil {
		t.Fatalf("ListAllFormFieldsCLI returned error: %v", err)
	}
	if len(fields) != 2 || fields[0].ID != "field-1" || fields[1].ID != "field-2" {
		t.Errorf("fields = %+v, want both pages", fields)
	}
	if len(receivedPages) != 2 || receivedPages[0] != "1" || receivedPages[1] != "2" {
		t.Errorf("received pages = %v, want [1 2]", receivedPages)
	}
}
