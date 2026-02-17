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
