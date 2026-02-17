package printer

import "io"

// Printer abstracts output formatting for CLI commands.
// Commands provide data; the printer handles formatting.
type Printer interface {
	// PrintObj prints a single object (for get/detail commands).
	PrintObj(obj interface{}, w io.Writer) error
	// PrintList prints a list of objects with headers (for list commands).
	PrintList(headers []string, rows [][]string, w io.Writer) error
}

// Column defines a column for table output.
type Column struct {
	Header string
	Field  string
}
