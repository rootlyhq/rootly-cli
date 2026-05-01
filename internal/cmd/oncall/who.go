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

var whoCmd = &cobra.Command{
	Use:   "who",
	Short: "Show who is on-call right now",
	Long:  "Display the users who are currently on-call across all schedules or a specific schedule.",
	Example: `  # See who is on-call right now
  rootly oncall who

  # Filter by schedule name or ID
  rootly oncall who --schedule="Primary On-Call"
  rootly oncall who --schedule-id=sched-123

  # Filter by service, team, or user
  rootly oncall who --service="API Gateway"
  rootly oncall who --team="Platform Engineering"
  rootly oncall who --user="alice@example.com"

  # Output as JSON
  rootly oncall who --format=json`,
	RunE: runWho,
}

func init() {
	whoCmd.Flags().String("schedule-id", "", "Filter by schedule ID")
	whoCmd.Flags().String("schedule", "", "Filter by schedule name (looked up automatically)")
	whoCmd.Flags().String("service-id", "", "Filter by service ID")
	whoCmd.Flags().String("service", "", "Filter by service name (looked up automatically)")
	whoCmd.Flags().String("escalation-policy-id", "", "Filter by escalation policy ID")
	whoCmd.Flags().String("user-id", "", "Filter by user ID")
	whoCmd.Flags().String("user", "", "Filter by user name or email (looked up automatically)")
	whoCmd.Flags().String("team-id", "", "Filter by team ID")
	whoCmd.Flags().String("team", "", "Filter by team name (looked up automatically)")
	whoCmd.Flags().String("time-zone", "", "Time zone (e.g. America/New_York)")
	whoCmd.Flags().Bool("earliest", true, "Only show first on-call user per escalation level")
	whoCmd.Flags().String("include", "user,schedule,escalation_policy", "Included resources (comma-separated)")

	OncallCmd.AddCommand(whoCmd)
}

func runWho(cmd *cobra.Command, args []string) error {
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	now := time.Now().UTC().Format(time.RFC3339)
	earliest, _ := cmd.Flags().GetBool("earliest")
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

	params := api.OnCallsParams{
		Include:             include,
		Since:               now,
		Until:               now,
		Earliest:            earliest,
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
	if format == "json" || format == "yaml" {
		p, err := printer.NewPrinter(format)
		if err != nil {
			return err
		}
		return p.PrintRawJSON(result.RawBody, os.Stdout)
	}

	if len(result.Entries) == 0 {
		fmt.Fprintln(os.Stderr, "No one is currently on-call")
		return nil
	}

	p, err := printer.NewPrinter(format)
	if err != nil {
		return err
	}

	headers := []string{"User", "Email", "Schedule", "Escalation Policy", "Level", "Ends"}
	rows := make([][]string, 0, len(result.Entries))

	for _, entry := range result.Entries {
		row := []string{
			entry.UserName,
			entry.UserEmail,
			truncateString(entry.ScheduleName, 30),
			truncateString(entry.EscalationPolicyName, 30),
			strconv.Itoa(entry.EscalationLevel),
			timeformat.FormatTime(entry.EndsAt),
		}
		rows = append(rows, row)
	}

	if err := p.PrintList(headers, rows, os.Stdout); err != nil {
		return fmt.Errorf("failed to print output: %w", err)
	}

	return nil
}
