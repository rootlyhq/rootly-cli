package printer

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// --- Factory tests ---

func TestNewPrinterTable(t *testing.T) {
	p, err := NewPrinter("table")
	if err != nil {
		t.Fatalf("NewPrinter(\"table\") returned error: %v", err)
	}
	if _, ok := p.(*TablePrinter); !ok {
		t.Fatalf("expected *TablePrinter, got %T", p)
	}
}

func TestNewPrinterJSON(t *testing.T) {
	p, err := NewPrinter("json")
	if err != nil {
		t.Fatalf("NewPrinter(\"json\") returned error: %v", err)
	}
	if _, ok := p.(*JSONPrinter); !ok {
		t.Fatalf("expected *JSONPrinter, got %T", p)
	}
}

func TestNewPrinterYAML(t *testing.T) {
	p, err := NewPrinter("yaml")
	if err != nil {
		t.Fatalf("NewPrinter(\"yaml\") returned error: %v", err)
	}
	if _, ok := p.(*YAMLPrinter); !ok {
		t.Fatalf("expected *YAMLPrinter, got %T", p)
	}
}

func TestNewPrinterMarkdown(t *testing.T) {
	p, err := NewPrinter("markdown")
	if err != nil {
		t.Fatalf("NewPrinter(\"markdown\") returned error: %v", err)
	}
	if _, ok := p.(*MarkdownPrinter); !ok {
		t.Fatalf("expected *MarkdownPrinter, got %T", p)
	}
}

func TestNewPrinterDefault(t *testing.T) {
	p, err := NewPrinter("")
	if err != nil {
		t.Fatalf("NewPrinter(\"\") returned error: %v", err)
	}
	if _, ok := p.(*TablePrinter); !ok {
		t.Fatalf("expected *TablePrinter for empty format, got %T", p)
	}
}

func TestNewPrinterUnknown(t *testing.T) {
	_, err := NewPrinter("csv")
	if err == nil {
		t.Fatal("expected error for unknown format, got nil")
	}
	if !strings.Contains(err.Error(), "unknown output format") {
		t.Fatalf("expected 'unknown output format' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "csv") {
		t.Fatalf("expected format name 'csv' in error, got: %v", err)
	}
}

// --- JSON printer tests ---

func TestJSONPrinterPrintObj(t *testing.T) {
	p := NewJSONPrinter()
	obj := map[string]string{"name": "test-incident", "status": "active"}
	var buf bytes.Buffer

	err := p.PrintObj(obj, &buf)
	if err != nil {
		t.Fatalf("PrintObj returned error: %v", err)
	}

	output := buf.Bytes()
	if !json.Valid(output) {
		t.Fatalf("output is not valid JSON: %s", output)
	}

	var result map[string]string
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}
	if result["name"] != "test-incident" {
		t.Fatalf("expected name 'test-incident', got %q", result["name"])
	}
}

func TestJSONPrinterPrintObjNil(t *testing.T) {
	p := NewJSONPrinter()
	var buf bytes.Buffer

	err := p.PrintObj(nil, &buf)
	if err != nil {
		t.Fatalf("PrintObj(nil) returned error: %v", err)
	}

	output := strings.TrimSpace(buf.String())
	if output != "null" {
		t.Fatalf("expected 'null', got %q", output)
	}
}

func TestJSONPrinterPrintList(t *testing.T) {
	p := NewJSONPrinter()
	headers := []string{"ID", "Title", "Status"}
	rows := [][]string{
		{"1", "Server down", "active"},
		{"2", "DB slow", "resolved"},
	}
	var buf bytes.Buffer

	err := p.PrintList(headers, rows, &buf)
	if err != nil {
		t.Fatalf("PrintList returned error: %v", err)
	}

	output := buf.Bytes()
	if !json.Valid(output) {
		t.Fatalf("output is not valid JSON: %s", output)
	}

	var result []map[string]string
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result))
	}
	if result[0]["Title"] != "Server down" {
		t.Fatalf("expected Title 'Server down', got %q", result[0]["Title"])
	}
}

func TestJSONPrinterPrintListNil(t *testing.T) {
	p := NewJSONPrinter()
	var buf bytes.Buffer

	err := p.PrintList([]string{"ID"}, nil, &buf)
	if err != nil {
		t.Fatalf("PrintList(nil) returned error: %v", err)
	}

	output := strings.TrimSpace(buf.String())
	if output != "[]" {
		t.Fatalf("expected '[]', got %q", output)
	}
}

// --- YAML printer tests ---

func TestYAMLPrinterPrintObj(t *testing.T) {
	p := NewYAMLPrinter()
	obj := map[string]string{"name": "test-incident", "status": "active"}
	var buf bytes.Buffer

	err := p.PrintObj(obj, &buf)
	if err != nil {
		t.Fatalf("PrintObj returned error: %v", err)
	}

	var result map[string]string
	if err := yaml.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("output is not valid YAML: %v\nOutput: %s", err, buf.String())
	}
	if result["name"] != "test-incident" {
		t.Fatalf("expected name 'test-incident', got %q", result["name"])
	}
}

func TestYAMLPrinterPrintList(t *testing.T) {
	p := NewYAMLPrinter()
	headers := []string{"ID", "Title"}
	rows := [][]string{
		{"1", "Server down"},
		{"2", "DB slow"},
	}
	var buf bytes.Buffer

	err := p.PrintList(headers, rows, &buf)
	if err != nil {
		t.Fatalf("PrintList returned error: %v", err)
	}

	var result []map[string]string
	if err := yaml.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("output is not valid YAML: %v\nOutput: %s", err, buf.String())
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result))
	}
	if result[0]["Title"] != "Server down" {
		t.Fatalf("expected Title 'Server down', got %q", result[0]["Title"])
	}
}

func TestYAMLPrinterPrintListEmpty(t *testing.T) {
	p := NewYAMLPrinter()
	var buf bytes.Buffer

	err := p.PrintList([]string{"ID"}, nil, &buf)
	if err != nil {
		t.Fatalf("PrintList(nil) returned error: %v", err)
	}

	output := strings.TrimSpace(buf.String())
	if output != "[]" {
		t.Fatalf("expected '[]', got %q", output)
	}
}

// --- Table printer tests ---

func TestTablePrinterPrintList(t *testing.T) {
	p := NewTablePrinter()
	headers := []string{"ID", "Title", "Status"}
	rows := [][]string{
		{"1", "Server down", "active"},
		{"2", "DB slow", "resolved"},
	}
	var buf bytes.Buffer

	err := p.PrintList(headers, rows, &buf)
	if err != nil {
		t.Fatalf("PrintList returned error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Fatal("expected non-empty output")
	}
	// Table should contain the header and data values
	if !strings.Contains(output, "ID") {
		t.Fatal("expected output to contain header 'ID'")
	}
	if !strings.Contains(output, "Server down") {
		t.Fatal("expected output to contain 'Server down'")
	}
}

func TestTablePrinterPrintObj(t *testing.T) {
	p := NewTablePrinter()
	obj := map[string]string{"name": "test", "status": "active"}
	var buf bytes.Buffer

	err := p.PrintObj(obj, &buf)
	if err != nil {
		t.Fatalf("PrintObj returned error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Fatal("expected non-empty output")
	}
	// Key-value table should contain the field names and values
	if !strings.Contains(output, "Field") {
		t.Fatal("expected output to contain header 'Field'")
	}
	if !strings.Contains(output, "Value") {
		t.Fatal("expected output to contain header 'Value'")
	}
}

// --- Markdown printer tests ---

func TestMarkdownPrinterPrintList(t *testing.T) {
	p := NewMarkdownPrinter()
	headers := []string{"ID", "Title", "Status"}
	rows := [][]string{
		{"1", "Server down", "active"},
		{"2", "DB slow", "resolved"},
	}
	var buf bytes.Buffer

	err := p.PrintList(headers, rows, &buf)
	if err != nil {
		t.Fatalf("PrintList returned error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Fatal("expected non-empty output")
	}
	// Markdown tables use pipe characters
	if !strings.Contains(output, "|") {
		t.Fatal("expected output to contain '|' (Markdown table marker)")
	}
	// Should contain headers
	if !strings.Contains(output, "ID") {
		t.Fatal("expected output to contain header 'ID'")
	}
	// Should contain separator line with dashes
	if !strings.Contains(output, "---") {
		t.Fatal("expected output to contain '---' (Markdown separator)")
	}
}

func TestMarkdownPrinterPrintObj(t *testing.T) {
	p := NewMarkdownPrinter()
	obj := map[string]string{"name": "test", "status": "active"}
	var buf bytes.Buffer

	err := p.PrintObj(obj, &buf)
	if err != nil {
		t.Fatalf("PrintObj returned error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Fatal("expected non-empty output")
	}
	if !strings.Contains(output, "|") {
		t.Fatal("expected output to contain '|' (Markdown table marker)")
	}
	if !strings.Contains(output, "Field") {
		t.Fatal("expected output to contain header 'Field'")
	}
}

// --- PrintRawJSON tests ---

func TestJSONPrinterPrintRawJSON(t *testing.T) {
	p := NewJSONPrinter()
	raw := []byte(`{"data":{"id":"123","attributes":{"title":"Test"}}}`)
	var buf bytes.Buffer

	err := p.PrintRawJSON(raw, &buf)
	if err != nil {
		t.Fatalf("PrintRawJSON returned error: %v", err)
	}

	output := buf.Bytes()
	if !json.Valid(output) {
		t.Fatalf("output is not valid JSON: %s", output)
	}

	// Should be pretty-printed (contain newlines and indentation)
	if !strings.Contains(buf.String(), "\n") {
		t.Fatal("expected pretty-printed JSON with newlines")
	}
}

func TestJSONPrinterPrintRawJSONInvalid(t *testing.T) {
	p := NewJSONPrinter()
	var buf bytes.Buffer

	err := p.PrintRawJSON([]byte("not json"), &buf)
	if err == nil {
		t.Fatal("expected error for invalid JSON input")
	}
}

func TestYAMLPrinterPrintRawJSON(t *testing.T) {
	p := NewYAMLPrinter()
	raw := []byte(`{"data":{"id":"123","attributes":{"title":"Test Incident"}}}`)
	var buf bytes.Buffer

	err := p.PrintRawJSON(raw, &buf)
	if err != nil {
		t.Fatalf("PrintRawJSON returned error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Fatal("expected non-empty YAML output")
	}

	// Should contain the data as YAML
	var result map[string]interface{}
	if err := yaml.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("output is not valid YAML: %v", err)
	}

	if result["data"] == nil {
		t.Fatal("expected 'data' key in YAML output")
	}
}

func TestYAMLPrinterPrintRawJSONInvalid(t *testing.T) {
	p := NewYAMLPrinter()
	var buf bytes.Buffer

	err := p.PrintRawJSON([]byte("not json"), &buf)
	if err == nil {
		t.Fatal("expected error for invalid JSON input")
	}
}

func TestTablePrinterPrintRawJSON(t *testing.T) {
	p := NewTablePrinter()
	var buf bytes.Buffer

	err := p.PrintRawJSON([]byte(`{}`), &buf)
	if err == nil {
		t.Fatal("expected error for table format PrintRawJSON")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("expected 'not supported' error, got: %v", err)
	}
}

func TestMarkdownPrinterPrintRawJSON(t *testing.T) {
	p := NewMarkdownPrinter()
	var buf bytes.Buffer

	err := p.PrintRawJSON([]byte(`{}`), &buf)
	if err == nil {
		t.Fatal("expected error for markdown format PrintRawJSON")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("expected 'not supported' error, got: %v", err)
	}
}

// --- PrintObj edge case tests (nil values, empty strings, non-map objects) ---

func TestTablePrinterPrintObjWithNilAndEmptyValues(t *testing.T) {
	p := NewTablePrinter()
	// Object with nil and empty string values — those rows should be skipped
	obj := map[string]interface{}{
		"name":   "test",
		"status": "active",
		"empty":  "",
		"nilval": nil,
	}
	var buf bytes.Buffer

	err := p.PrintObj(obj, &buf)
	if err != nil {
		t.Fatalf("PrintObj returned error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "name") {
		t.Error("expected output to contain 'name'")
	}
	if !strings.Contains(output, "active") {
		t.Error("expected output to contain 'active'")
	}
	// nil and empty values should be skipped — "nilval" should not appear
	if strings.Contains(output, "nilval") {
		t.Error("expected nil value row to be skipped")
	}
}

func TestTablePrinterPrintObjNonMap(t *testing.T) {
	p := NewTablePrinter()
	// A plain string is not a map — unmarshal into map[string]interface{} will fail
	var buf bytes.Buffer

	err := p.PrintObj("just a string", &buf)
	if err != nil {
		t.Fatalf("PrintObj returned error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "just a string") {
		t.Errorf("expected output to contain 'just a string', got %q", output)
	}
}

func TestMarkdownPrinterPrintObjWithNilAndEmptyValues(t *testing.T) {
	p := NewMarkdownPrinter()
	obj := map[string]interface{}{
		"name":   "test",
		"status": "active",
		"empty":  "",
		"nilval": nil,
	}
	var buf bytes.Buffer

	err := p.PrintObj(obj, &buf)
	if err != nil {
		t.Fatalf("PrintObj returned error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "name") {
		t.Error("expected output to contain 'name'")
	}
	if strings.Contains(output, "nilval") {
		t.Error("expected nil value row to be skipped")
	}
}

func TestMarkdownPrinterPrintObjNonMap(t *testing.T) {
	p := NewMarkdownPrinter()
	var buf bytes.Buffer

	err := p.PrintObj("just a string", &buf)
	if err != nil {
		t.Fatalf("PrintObj returned error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "just a string") {
		t.Errorf("expected output to contain 'just a string', got %q", output)
	}
}

func TestTablePrinterPrintObjMarshalError(t *testing.T) {
	p := NewTablePrinter()
	var buf bytes.Buffer

	// Channels cannot be marshaled to JSON
	err := p.PrintObj(make(chan int), &buf)
	if err == nil {
		t.Fatal("expected error for unmarshalable object")
	}
	if !strings.Contains(err.Error(), "failed to marshal") {
		t.Errorf("expected 'failed to marshal' error, got: %v", err)
	}
}

func TestMarkdownPrinterPrintObjMarshalError(t *testing.T) {
	p := NewMarkdownPrinter()
	var buf bytes.Buffer

	err := p.PrintObj(make(chan int), &buf)
	if err == nil {
		t.Fatal("expected error for unmarshalable object")
	}
	if !strings.Contains(err.Error(), "failed to marshal") {
		t.Errorf("expected 'failed to marshal' error, got: %v", err)
	}
}

// --- PrintList edge case tests ---

func TestTablePrinterPrintListEmpty(t *testing.T) {
	p := NewTablePrinter()
	var buf bytes.Buffer

	err := p.PrintList([]string{"ID", "Name"}, nil, &buf)
	if err != nil {
		t.Fatalf("PrintList(nil rows) returned error: %v", err)
	}
	// Should still render the header
	output := buf.String()
	if !strings.Contains(output, "ID") {
		t.Error("expected output to contain header 'ID'")
	}
}

func TestMarkdownPrinterPrintListEmpty(t *testing.T) {
	p := NewMarkdownPrinter()
	var buf bytes.Buffer

	err := p.PrintList([]string{"ID", "Name"}, nil, &buf)
	if err != nil {
		t.Fatalf("PrintList(nil rows) returned error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "ID") {
		t.Error("expected output to contain header 'ID'")
	}
}

func TestJSONPrinterPrintListShortRow(t *testing.T) {
	p := NewJSONPrinter()
	headers := []string{"ID", "Title", "Status"}
	// Row has fewer values than headers — bounds check branch
	rows := [][]string{
		{"1", "Server down"},
	}
	var buf bytes.Buffer

	err := p.PrintList(headers, rows, &buf)
	if err != nil {
		t.Fatalf("PrintList returned error: %v", err)
	}

	var result []map[string]string
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result))
	}
	// "Status" key should not exist since the row was short
	if _, ok := result[0]["Status"]; ok {
		t.Error("expected 'Status' key to be absent for short row")
	}
}

func TestYAMLPrinterPrintListShortRow(t *testing.T) {
	p := NewYAMLPrinter()
	headers := []string{"ID", "Title", "Status"}
	rows := [][]string{
		{"1", "Server down"},
	}
	var buf bytes.Buffer

	err := p.PrintList(headers, rows, &buf)
	if err != nil {
		t.Fatalf("PrintList returned error: %v", err)
	}

	var result []map[string]string
	if err := yaml.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result))
	}
	if _, ok := result[0]["Status"]; ok {
		t.Error("expected 'Status' key to be absent for short row")
	}
}

// --- SupportedFormats test ---

func TestSupportedFormats(t *testing.T) {
	expected := map[string]bool{
		"table":    true,
		"json":     true,
		"yaml":     true,
		"markdown": true,
	}
	for _, f := range SupportedFormats {
		if !expected[f] {
			t.Fatalf("unexpected format in SupportedFormats: %q", f)
		}
	}
	if len(SupportedFormats) != len(expected) {
		t.Fatalf("expected %d formats, got %d", len(expected), len(SupportedFormats))
	}
}
