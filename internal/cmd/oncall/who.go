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

var whoCmd = &cobra.Command{
	Use:   "who",
	Short: "Show who is on-call right now",
	Long:  "Display the users who are currently on-call across all schedules or a specific schedule.",
	Example: `  # See who is on-call right now
  rootly oncall who

  # Filter by schedule
  rootly oncall who --schedule="Primary On-Call"

  # Output as JSON
  rootly oncall who --format=json`,
	RunE: runWho,
}

func init() {
	whoCmd.Flags().String("schedule", "", "Filter by schedule name or ID")

	OncallCmd.AddCommand(whoCmd)
}

func runWho(cmd *cobra.Command, args []string) error {
	// Get API client
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}

	// Read flags
	schedule, _ := cmd.Flags().GetString("schedule")

	// Build filters map to get shifts active RIGHT NOW
	filters := make(map[string]string)

	// Set time range to capture current shifts
	now := time.Now()
	filters["from"] = url.QueryEscape(now.Format(time.RFC3339))
	filters["to"] = url.QueryEscape(now.Format(time.RFC3339))

	if schedule != "" {
		filters["schedule_ids[]"] = url.QueryEscape(schedule)
	}

	// Call API with large page size to get all current shifts
	result, err := apiClient.ListShiftsCLI(cmd.Context(), 1, 100, filters)
	if err != nil {
		return fmt.Errorf("failed to list shifts: %w", err)
	}

	// For json/yaml: pass through raw API response (unfiltered)
	format := viper.GetString("format")
	if format == "json" || format == "yaml" {
		p, err := printer.NewPrinter(format)
		if err != nil {
			return err
		}
		return p.PrintRawJSON(result.RawBody, os.Stdout)
	}

	// Filter to only active shifts (client-side verification)
	activeShifts := make([]struct {
		UserName     string
		ScheduleName string
		StartsAt     time.Time
		EndsAt       time.Time
	}, 0)

	for _, shift := range result.Shifts {
		if shift.IsActive {
			activeShifts = append(activeShifts, struct {
				UserName     string
				ScheduleName string
				StartsAt     time.Time
				EndsAt       time.Time
			}{
				UserName:     shift.UserName,
				ScheduleName: shift.ScheduleName,
				StartsAt:     shift.StartsAt,
				EndsAt:       shift.EndsAt,
			})
		}
	}

	// If no active shifts, print message and exit
	if len(activeShifts) == 0 {
		fmt.Fprintln(os.Stderr, "No one is currently on-call")
		return nil
	}

	// Create printer for table/markdown output
	p, err := printer.NewPrinter(format)
	if err != nil {
		return err
	}

	// Build headers and rows
	headers := []string{"User", "Schedule", "Started", "Ends"}
	rows := make([][]string, 0, len(activeShifts))

	for _, shift := range activeShifts {
		row := []string{
			shift.UserName,
			truncateString(shift.ScheduleName, 30),
			timeformat.FormatTime(shift.StartsAt),
			timeformat.FormatTime(shift.EndsAt),
		}
		rows = append(rows, row)
	}

	// Print list
	if err := p.PrintList(headers, rows, os.Stdout); err != nil {
		return fmt.Errorf("failed to print output: %w", err)
	}

	return nil
}
