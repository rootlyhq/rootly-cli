package oncall

import (
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/printer"
	"github.com/rootlyhq/rootly-cli/internal/timeformat"
)

var shiftsCmd = &cobra.Command{
	Use:   "shifts",
	Short: "List on-call shifts",
	Long:  "List on-call shifts for the current and upcoming time period.",
	Example: `  # View shifts for the next 7 days (default)
  rootly oncall shifts

  # View shifts for the next 14 days
  rootly oncall shifts --days=14

  # Filter by schedule name or ID
  rootly oncall shifts --schedule="Primary On-Call"

  # Output as JSON
  rootly oncall shifts --format=json`,
	RunE: runShifts,
}

func init() {
	shiftsCmd.Flags().Int("days", 7, "Number of days ahead to show shifts (default: 7)")
	shiftsCmd.Flags().String("schedule", "", "Filter by schedule name or ID")
	shiftsCmd.Flags().Int("page", 1, "Page number")
	shiftsCmd.Flags().Int("page-size", 25, "Results per page (max 100)")

	OncallCmd.AddCommand(shiftsCmd)
}

func runShifts(cmd *cobra.Command, args []string) error {
	// Get API client
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}

	// Read flags
	days, _ := cmd.Flags().GetInt("days")
	schedule, _ := cmd.Flags().GetString("schedule")
	page, _ := cmd.Flags().GetInt("page")
	pageSize, _ := cmd.Flags().GetInt("page-size")

	// Build filters map with time range
	filters := make(map[string]string)

	// Set time range: from now to N days ahead
	now := time.Now()
	endTime := now.AddDate(0, 0, days)

	// Use ISO 8601 format for API filters
	filters["starts_before"] = url.QueryEscape(endTime.Format(time.RFC3339))
	filters["ends_after"] = url.QueryEscape(now.Format(time.RFC3339))

	if schedule != "" {
		// Try filtering by schedule name first, API will handle it
		filters["schedule"] = url.QueryEscape(schedule)
	}

	// Call API
	result, err := apiClient.ListShiftsCLI(cmd.Context(), page, pageSize, filters)
	if err != nil {
		return fmt.Errorf("failed to list shifts: %w", err)
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
	headers := []string{"User", "Schedule", "Starts", "Ends", "Active"}
	rows := make([][]string, 0, len(result.Shifts))

	for _, shift := range result.Shifts {
		active := "No"
		if shift.IsActive {
			active = "Yes"
		}

		row := []string{
			shift.UserName,
			truncateString(shift.ScheduleName, 30),
			timeformat.FormatTime(shift.StartsAt),
			timeformat.FormatTime(shift.EndsAt),
			active,
		}
		rows = append(rows, row)
	}

	// Print list
	if err := p.PrintList(headers, rows, os.Stdout); err != nil {
		return fmt.Errorf("failed to print output: %w", err)
	}

	// Print pagination info to stderr if there are multiple pages
	if result.Pagination.TotalPages > 1 {
		fmt.Fprintf(os.Stderr, "\nPage %d of %d (%d total shifts)\n",
			result.Pagination.CurrentPage,
			result.Pagination.TotalPages,
			result.Pagination.TotalCount)
	}

	return nil
}
