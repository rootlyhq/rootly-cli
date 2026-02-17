package printer

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/jedib0t/go-pretty/v6/table"
)

// MarkdownPrinter renders output as Markdown tables using go-pretty's
// built-in RenderMarkdown format.
type MarkdownPrinter struct{}

// NewMarkdownPrinter creates a new MarkdownPrinter.
func NewMarkdownPrinter() *MarkdownPrinter {
	return &MarkdownPrinter{}
}

// PrintList renders a list as a Markdown table.
func (p *MarkdownPrinter) PrintList(headers []string, rows [][]string, w io.Writer) error {
	tw := table.NewWriter()
	tw.SetOutputMirror(w)

	// Convert headers to table.Row
	headerRow := make(table.Row, len(headers))
	for i, h := range headers {
		headerRow[i] = h
	}
	tw.AppendHeader(headerRow)

	// Append data rows
	for _, row := range rows {
		tableRow := make(table.Row, len(row))
		for i, cell := range row {
			tableRow[i] = cell
		}
		tw.AppendRow(tableRow)
	}

	tw.RenderMarkdown()
	return nil
}

// PrintRawJSON is not supported for markdown format.
func (p *MarkdownPrinter) PrintRawJSON(_ []byte, _ io.Writer) error {
	return fmt.Errorf("raw JSON passthrough is not supported for markdown format")
}

// PrintObj renders a single object as a Markdown key-value table.
func (p *MarkdownPrinter) PrintObj(obj interface{}, w io.Writer) error {
	// Marshal to JSON and back to get a map
	data, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("failed to marshal object: %w", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		// Not a map - print as plain text
		_, _ = fmt.Fprintf(w, "%v\n", obj)
		return nil
	}

	tw := table.NewWriter()
	tw.SetOutputMirror(w)
	tw.AppendHeader(table.Row{"Field", "Value"})

	for k, v := range m {
		if v == nil {
			continue
		}
		strVal := fmt.Sprintf("%v", v)
		if strVal == "" {
			continue
		}
		tw.AppendRow(table.Row{k, strVal})
	}

	tw.RenderMarkdown()
	return nil
}
