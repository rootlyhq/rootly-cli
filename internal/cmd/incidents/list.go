package incidents

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
	Short: "List incidents",
	Long:  "List Rootly incidents with optional filters and pagination.",
	Example: `  # List incidents (default: 25 per page, newest first)
  rootly incidents list

  # List with custom page size
  rootly incidents list --page-size=50

  # Filter by status
  rootly incidents list --status=started

  # Filter by severity
  rootly incidents list --severity=sev0

  # Sort by status
  rootly incidents list --sort=status

  # Page 2 of results
  rootly incidents list --page=2

  # Output as JSON (useful for piping to jq)
  rootly incidents list --format=json`,
	RunE: runList,
}

func init() {
	listCmd.Flags().Int("page", 1, "Page number")
	listCmd.Flags().Int("page-size", 25, "Results per page (max 100)")
	listCmd.Flags().String("sort", "-created_at", "Sort order (e.g., created_at, -created_at, status, -status, severity, title)")
	listCmd.Flags().String("status", "", "Filter by status (started, mitigated, resolved, closed, cancelled)")
	listCmd.Flags().String("severity", "", "Filter by severity slug/name")

	IncidentsCmd.AddCommand(listCmd)
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
	severity, _ := cmd.Flags().GetString("severity")

	// Build filters map
	filters := make(map[string]string)
	if status != "" {
		filters["status"] = status
	}
	if severity != "" {
		filters["severity"] = severity
	}

	// Call API
	result, err := apiClient.ListIncidentsCLI(cmd.Context(), page, pageSize, sort, filters)
	if err != nil {
		return fmt.Errorf("failed to list incidents: %w", err)
	}

	// Get format from viper
	format := viper.GetString("format")

	// Create printer
	p, err := printer.NewPrinter(format)
	if err != nil {
		return err
	}

	// Build headers and rows
	headers := []string{"ID", "Title", "Status", "Severity", "Started", "Duration"}
	rows := make([][]string, 0, len(result.Incidents))

	for _, inc := range result.Incidents {
		id := inc.SequentialID
		if id == "" {
			id = inc.ID
		}

		row := []string{
			id,
			truncateString(inc.Title, 60),
			inc.Status,
			inc.Severity,
			formatTime(inc.StartedAt, inc.CreatedAt),
			formatDuration(inc.Duration()),
		}
		rows = append(rows, row)
	}

	// Print list
	if err := p.PrintList(headers, rows, os.Stdout); err != nil {
		return fmt.Errorf("failed to print output: %w", err)
	}

	// Print pagination info to stderr if there are multiple pages
	if result.Pagination.TotalPages > 1 {
		fmt.Fprintf(os.Stderr, "\nPage %d of %d (%d total incidents)\n",
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

// formatDuration converts seconds to human-readable duration.
// Returns formats like "2h 30m", "45m", "3d 2h", or "-" if 0.
func formatDuration(seconds int64) string {
	if seconds == 0 {
		return "-"
	}

	d := time.Duration(seconds) * time.Second

	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		if hours > 0 {
			return fmt.Sprintf("%dd %dh", days, hours)
		}
		return fmt.Sprintf("%dd", days)
	}

	if hours > 0 {
		if minutes > 0 {
			return fmt.Sprintf("%dh %dm", hours, minutes)
		}
		return fmt.Sprintf("%dh", hours)
	}

	if minutes > 0 {
		return fmt.Sprintf("%dm", minutes)
	}

	return fmt.Sprintf("%ds", int(d.Seconds()))
}
