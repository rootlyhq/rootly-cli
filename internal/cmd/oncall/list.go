package oncall

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/printer"
	"github.com/rootlyhq/rootly-cli/internal/timeformat"
)

var listCmd = &cobra.Command{
	Use:   "schedules",
	Short: "List on-call schedules",
	Long:  "List Rootly on-call schedules with optional pagination.",
	Example: `  # List on-call schedules (default: 25 per page, newest first)
  rootly oncall schedules

  # List with custom page size
  rootly oncall schedules --page-size=50

  # Page 2 of results
  rootly oncall schedules --page=2

  # Output as JSON (useful for piping to jq)
  rootly oncall schedules --format=json`,
	RunE: runList,
}

func init() {
	listCmd.Flags().Int("page", 1, "Page number")
	listCmd.Flags().Int("page-size", 25, "Results per page (max 100)")

	OncallCmd.AddCommand(listCmd)
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

	// Build filters map (minimal for schedules)
	filters := make(map[string]string)

	// Call API
	result, err := apiClient.ListSchedulesCLI(cmd.Context(), page, pageSize, filters)
	if err != nil {
		return fmt.Errorf("failed to list schedules: %w", err)
	}

	// Get format from viper
	format := viper.GetString("format")

	// Create printer
	p, err := printer.NewPrinter(format)
	if err != nil {
		return err
	}

	// For json/yaml: pass through raw API response (includes meta/pagination)
	if format == "json" || format == "yaml" {
		return p.PrintRawJSON(result.RawBody, os.Stdout)
	}

	// Build headers and rows
	headers := []string{"ID", "Name", "Description", "Created"}
	rows := make([][]string, 0, len(result.Schedules))

	for _, schedule := range result.Schedules {
		row := []string{
			schedule.ID,
			truncateString(schedule.Name, 40),
			truncateString(schedule.Description, 50),
			timeformat.FormatTime(schedule.CreatedAt),
		}
		rows = append(rows, row)
	}

	// Print list
	if err := p.PrintList(headers, rows, os.Stdout); err != nil {
		return fmt.Errorf("failed to print output: %w", err)
	}

	// Print pagination info to stderr if there are multiple pages
	if result.Pagination.TotalPages > 1 {
		fmt.Fprintf(os.Stderr, "\nPage %d of %d (%d total schedules)\n",
			result.Pagination.CurrentPage,
			result.Pagination.TotalPages,
			result.Pagination.TotalCount)
	}

	return nil
}

// truncateString truncates a string to maxLen length, appending "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
