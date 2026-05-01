package oncall

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/api"
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
  rootly oncall shifts --schedule-id=sched-123

  # Filter by service, team, or user
  rootly oncall shifts --service="API Gateway"
  rootly oncall shifts --team="Platform Engineering"
  rootly oncall shifts --user="alice@example.com"

  # Output as JSON
  rootly oncall shifts --format=json`,
	RunE: runShifts,
}

func init() {
	shiftsCmd.Flags().Int("days", 7, "Number of days ahead to show shifts (default: 7)")
	shiftsCmd.Flags().String("schedule-id", "", "Filter by schedule ID")
	shiftsCmd.Flags().String("schedule", "", "Filter by schedule name (looked up automatically)")
	shiftsCmd.Flags().String("service-id", "", "Filter by service ID")
	shiftsCmd.Flags().String("service", "", "Filter by service name (looked up automatically)")
	shiftsCmd.Flags().String("escalation-policy-id", "", "Filter by escalation policy ID")
	shiftsCmd.Flags().String("user-id", "", "Filter by user ID")
	shiftsCmd.Flags().String("user", "", "Filter by user name or email (looked up automatically)")
	shiftsCmd.Flags().String("team-id", "", "Filter by team ID")
	shiftsCmd.Flags().String("team", "", "Filter by team name (looked up automatically)")
	shiftsCmd.Flags().String("time-zone", "", "Time zone (e.g. America/New_York)")
	shiftsCmd.Flags().String("include", "user,schedule,escalation_policy", "Included resources (comma-separated)")

	OncallCmd.AddCommand(shiftsCmd)
}

func runShifts(cmd *cobra.Command, args []string) error {
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	days, _ := cmd.Flags().GetInt("days")
	include, _ := cmd.Flags().GetString("include")
	escalationPolicyID, _ := cmd.Flags().GetString("escalation-policy-id")
	timeZone, _ := cmd.Flags().GetString("time-zone")

	scheduleID, err := resolveScheduleID(ctx, apiClient, cmd)
	if err != nil {
		return err
	}
	serviceID, err := resolveServiceID(ctx, apiClient, cmd)
	if err != nil {
		return err
	}
	userID, err := resolveUserID(ctx, apiClient, cmd)
	if err != nil {
		return err
	}
	teamID, err := resolveTeamID(ctx, apiClient, cmd)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	until := now.AddDate(0, 0, days)

	params := api.OnCallsParams{
		Include:             include,
		Since:               now.Format(time.RFC3339),
		Until:               until.Format(time.RFC3339),
		TimeZone:            timeZone,
		ScheduleIDs:         scheduleID,
		ServiceIDs:          serviceID,
		EscalationPolicyIDs: escalationPolicyID,
		UserIDs:             userID,
		GroupIDs:            teamID,
	}

	result, err := apiClient.ListOnCallsCLI(cmd.Context(), params)
	if err != nil {
		return fmt.Errorf("failed to list on-calls: %w", err)
	}

	format := viper.GetString("format")

	p, err := printer.NewPrinter(format)
	if err != nil {
		return err
	}

	if format == "json" || format == "yaml" {
		return p.PrintRawJSON(result.RawBody, os.Stdout)
	}

	headers := []string{"User", "Email", "Schedule", "Escalation Policy", "Level", "Starts", "Ends"}
	rows := make([][]string, 0, len(result.Entries))

	for _, entry := range result.Entries {
		row := []string{
			entry.UserName,
			entry.UserEmail,
			truncateString(entry.ScheduleName, 30),
			truncateString(entry.EscalationPolicyName, 30),
			strconv.Itoa(entry.EscalationLevel),
			timeformat.FormatTime(entry.StartsAt),
			timeformat.FormatTime(entry.EndsAt),
		}
		rows = append(rows, row)
	}

	if err := p.PrintList(headers, rows, os.Stdout); err != nil {
		return fmt.Errorf("failed to print output: %w", err)
	}

	return nil
}
