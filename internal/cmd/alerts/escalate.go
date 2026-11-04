package alerts

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/rootlyhq/rootly-cli/internal/api"
)

var escalateCmd = &cobra.Command{
	Use:   "escalate <alert-id>",
	Short: "Escalate an alert",
	Long: `Escalate an alert.

With --escalation-policy and/or --level, the alert is escalated through its escalation policy.
With --team, the team (slug or ID) is paged on the existing alert.`,
	Example: `  rootly alerts escalate <alert-id> --escalation-policy=<uuid> --level=2
  rootly alerts escalate <alert-id> --team=platform`,
	Args: cobra.ExactArgs(1),
	RunE: runEscalate,
}

func init() {
	escalateCmd.Flags().String("escalation-policy", "", "Escalation policy UUID")
	escalateCmd.Flags().Int("level", 0, "Escalation policy level")
	escalateCmd.Flags().String("team", "", "Team slug or ID to page")
	escalateCmd.MarkFlagsMutuallyExclusive("team", "level")
	AlertsCmd.AddCommand(escalateCmd)
}

func runEscalate(cmd *cobra.Command, args []string) error {
	var policyID *string
	if cmd.Flags().Changed("escalation-policy") {
		value, _ := cmd.Flags().GetString("escalation-policy")
		policyID = &value
	}
	if cmd.Flags().Changed("team") {
		team, _ := cmd.Flags().GetString("team")
		return runPageTeam(cmd, args[0], team, policyID)
	}
	var level *int
	if cmd.Flags().Changed("level") {
		value, _ := cmd.Flags().GetInt("level")
		if value < 1 {
			return fmt.Errorf("invalid --level %d: must be at least 1", value)
		}
		level = &value
	}

	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}
	if err := apiClient.EscalateAlertCLI(cmd.Context(), args[0], policyID, level); err != nil {
		return fmt.Errorf("failed to escalate alert: %w", err)
	}
	if _, err := fmt.Fprintf(os.Stdout, "Escalated alert %s\n", args[0]); err != nil {
		return err
	}
	return nil
}

// runPageTeam pages a team (and optionally an escalation policy) on an existing alert.
func runPageTeam(cmd *cobra.Command, alertID, teamRef string, policyID *string) error {
	if teamRef == "" {
		return fmt.Errorf("--team must not be empty")
	}
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}
	alert, err := apiClient.GetAlertByID(cmd.Context(), alertID)
	if err != nil {
		return fmt.Errorf("failed to escalate alert: %w", err)
	}
	team, err := apiClient.GetTeamByID(cmd.Context(), teamRef)
	if err != nil {
		return fmt.Errorf("unknown team %q: %w", teamRef, err)
	}
	targets := []api.NotificationTarget{{Type: api.NotificationTargetGroup, ID: team.ID}}
	if policyID != nil {
		targets = append(targets, api.NotificationTarget{Type: api.NotificationTargetEscalationPolicy, ID: *policyID})
	}
	if _, err := apiClient.PageAlertTargetsCLI(cmd.Context(), alert.ID, alert.Summary, "", targets); err != nil {
		return fmt.Errorf("failed to page team: %w", err)
	}
	_, err = fmt.Fprintf(os.Stdout, "Paged team %s on alert %s\n", team.Name, alertID)
	return err
}
