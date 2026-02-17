package alerts

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-tui/internal/printer"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List alerts",
	Long:  "List Rootly alerts with optional filters and pagination.",
	Example: `  # List alerts (default: 25 per page, newest first)
  rootly alerts list

  # List with custom page size
  rootly alerts list --page-size=50

  # Filter by status
  rootly alerts list --status=triggered

  # Filter by source
  rootly alerts list --source=datadog

  # Sort by status
  rootly alerts list --sort=status

  # Page 2 of results
  rootly alerts list --page=2

  # Output as JSON (useful for piping to jq)
  rootly alerts list --format=json`,
	RunE: runList,
}

func init() {
	listCmd.Flags().Int("page", 1, "Page number")
	listCmd.Flags().Int("page-size", 25, "Results per page (max 100)")
	listCmd.Flags().String("sort", "-created_at", "Sort order (e.g., created_at, -created_at, status, summary)")
	listCmd.Flags().String("status", "", "Filter by status (triggered, acknowledged, resolved)")
	listCmd.Flags().String("source", "", "Filter by source (e.g., datadog, pagerduty, sentry)")

	AlertsCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	// Get API client
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}

	// Read flags
	page, _ := cmd.Flags().GetInt("page")
	pageSize, _ := cmd.Flags().GetInt("page-size")
	sort, _ := cmd.Flags().GetString("sort")
	status, _ := cmd.Flags().GetString("status")
	source, _ := cmd.Flags().GetString("source")

	// Build filters map
	filters := make(map[string]string)
	if status != "" {
		filters["status"] = status
	}
	if source != "" {
		filters["source"] = source
	}

	// Call API
	result, err := apiClient.ListAlertsCLI(cmd.Context(), page, pageSize, sort, filters)
	if err != nil {
		return fmt.Errorf("failed to list alerts: %w", err)
	}

	// Get format from viper
	format := viper.GetString("format")

	// Create printer
	p, err := printer.NewPrinter(format)
	if err != nil {
		return err
	}

	// Build headers and rows
	headers := []string{"ID", "Summary", "Status", "Source", "Started"}
	rows := make([][]string, 0, len(result.Alerts))

	for _, alert := range result.Alerts {
		id := alert.ShortID
		if id == "" {
			id = alert.ID
		}

		row := []string{
			id,
			truncateString(alert.Summary, 60),
			alert.Status,
			alert.Source,
			formatTime(alert.StartedAt, alert.CreatedAt),
		}
		rows = append(rows, row)
	}

	// Print list
	if err := p.PrintList(headers, rows, os.Stdout); err != nil {
		return fmt.Errorf("failed to print output: %w", err)
	}

	// Print pagination info to stderr if there are multiple pages
	if result.Pagination.TotalPages > 1 {
		fmt.Fprintf(os.Stderr, "\nPage %d of %d (%d total alerts)\n",
			result.Pagination.CurrentPage,
			result.Pagination.TotalPages,
			result.Pagination.TotalCount)
	}

	return nil
}

// truncateString truncates a string to max length, appending "..." if truncated.
func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

// formatTime formats a time pointer or falls back to a default time.
// Returns formatted time string in "2006-01-02 15:04" format.
func formatTime(primary *time.Time, fallback time.Time) string {
	t := fallback
	if primary != nil {
		t = *primary
	}
	return t.Format("2006-01-02 15:04")
}
