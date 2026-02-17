package printer

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

// TablePrinter renders output as formatted ASCII/Unicode tables.
type TablePrinter struct{}

// NewTablePrinter creates a new TablePrinter.
func NewTablePrinter() *TablePrinter {
	return &TablePrinter{}
}

// PrintList renders a list as a table with headers.
func (p *TablePrinter) PrintList(headers []string, rows [][]string, w io.Writer) error {
	tw := table.NewWriter()
	tw.SetOutputMirror(w)

	if IsColorEnabled() {
		tw.SetStyle(table.StyleRounded)
	} else {
		tw.SetStyle(table.StyleDefault)
	}
	tw.Style().Format.Header = text.FormatDefault

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

	// Set max column width to prevent terminal overflow
	configs := make([]table.ColumnConfig, len(headers))
	for i := range headers {
		configs[i] = table.ColumnConfig{
			Number:   i + 1,
			WidthMax: 80,
		}
	}
	tw.SetColumnConfigs(configs)

	tw.Render()
	return nil
}

// PrintRawJSON is not supported for table format.
func (p *TablePrinter) PrintRawJSON(_ []byte, _ io.Writer) error {
	return fmt.Errorf("raw JSON passthrough is not supported for table format")
}

// PrintObj renders a single object as a key-value table.
// The object is marshaled to a map via JSON round-trip.
func (p *TablePrinter) PrintObj(obj interface{}, w io.Writer) error {
	// Marshal to JSON and back to get a map
	data, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("failed to marshal object: %w", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		// If it's not a map (e.g., a string or number), just print the value
		_, _ = fmt.Fprintf(w, "%v\n", obj)
		return nil
	}

	tw := table.NewWriter()
	tw.SetOutputMirror(w)

	if IsColorEnabled() {
		tw.SetStyle(table.StyleRounded)
	} else {
		tw.SetStyle(table.StyleDefault)
	}
	tw.Style().Format.Header = text.FormatDefault

	tw.AppendHeader(table.Row{"Field", "Value"})

	// Set max width for value column
	tw.SetColumnConfigs([]table.ColumnConfig{
		{Number: 1, WidthMax: 30},
		{Number: 2, WidthMax: 80},
	})

	for k, v := range m {
		// Skip empty values
		if v == nil {
			continue
		}
		strVal := fmt.Sprintf("%v", v)
		if strVal == "" {
			continue
		}
		tw.AppendRow(table.Row{k, strVal})
	}

	tw.Render()
	return nil
}
