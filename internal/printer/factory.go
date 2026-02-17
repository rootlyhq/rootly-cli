package printer

import (
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
)

// SupportedFormats lists all valid output format names.
var SupportedFormats = []string{"table", "json", "yaml", "markdown"}

// NewPrinter creates a Printer for the given format string.
// If format is empty, it defaults to "table" (consistent with kubectl/gh behavior).
func NewPrinter(format string) (Printer, error) {
	if format == "" {
		format = "table"
	}

	switch format {
	case "table":
		return NewTablePrinter(), nil
	case "json":
		return NewJSONPrinter(), nil
	case "yaml":
		return NewYAMLPrinter(), nil
	case "markdown":
		return NewMarkdownPrinter(), nil
	default:
		return nil, fmt.Errorf("unknown output format %q, must be one of: table, json, yaml, markdown", format)
	}
}

// IsTTY returns true if stdout is a terminal (not piped or redirected).
func IsTTY() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
}

// IsColorEnabled returns true if colored output should be used.
// Returns false if the NO_COLOR env var is set (any non-empty value)
// or if stdout is not a TTY.
func IsColorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return IsTTY()
}
