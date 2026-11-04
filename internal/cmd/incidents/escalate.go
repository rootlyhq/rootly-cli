package incidents

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/rootlyhq/rootly-cli/internal/api"
)

var escalateCmd = &cobra.Command{
	Use:   "escalate <incident-id>",
	Short: "Page a team for an incident",
	Long: `Page a team for an incident.

Creates an alert that pages the team (and the escalation policy, if given) and links it to the incident.
The incident can be a UUID, slug, or sequential ID such as INC-123.`,
	Example: `  # Page a team
  rootly incidents escalate INC-123 --team=platform

  # Also page an escalation policy, with a note for the responders
  rootly incidents escalate INC-123 --team=platform --escalation-policy=<uuid> --note="Database is down"`,
	Args: cobra.ExactArgs(1),
	RunE: runEscalate,
}

func init() {
	escalateCmd.Flags().String("team", "", "Team slug or ID to page")
	escalateCmd.Flags().String("escalation-policy", "", "Escalation policy UUID to page as well")
	escalateCmd.Flags().String("note", "", "Note for the responders")
	_ = escalateCmd.MarkFlagRequired("team")
	IncidentsCmd.AddCommand(escalateCmd)
}

func runEscalate(cmd *cobra.Command, args []string) error {
	teamRef, _ := cmd.Flags().GetString("team")
	if teamRef == "" {
		return fmt.Errorf("--team must not be empty")
	}
	policyID, _ := cmd.Flags().GetString("escalation-policy")
	note, _ := cmd.Flags().GetString("note")

	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}
	incident, err := apiClient.GetIncidentByID(cmd.Context(), api.NormalizeIncidentID(args[0]))
	if err != nil {
		return err
	}
	team, err := apiClient.GetTeamByID(cmd.Context(), teamRef)
	if err != nil {
		return fmt.Errorf("unknown team %q: %w", teamRef, err)
	}
	targets := []api.NotificationTarget{{Type: api.NotificationTargetGroup, ID: team.ID}}
	if policyID != "" {
		targets = append(targets, api.NotificationTarget{Type: api.NotificationTargetEscalationPolicy, ID: policyID})
	}

	label := incident.ID
	if incident.SequentialID != "" {
		label = incident.SequentialID
	}
	summary := fmt.Sprintf("Escalation for %s: %s", label, incident.Title)
	alertID, err := apiClient.PageAlertTargetsCLI(cmd.Context(), "", summary, note, targets)
	if err != nil {
		return fmt.Errorf("failed to page team: %w", err)
	}
	if err := apiClient.AttachAlertsToIncidentCLI(cmd.Context(), incident.ID, []string{alertID}); err != nil {
		return fmt.Errorf("team %s was paged by alert %s but linking it to %s failed: %w", team.Name, alertID, label, err)
	}
	_, err = fmt.Fprintf(os.Stdout, "Paged team %s for incident %s (alert %s)\n", team.Name, label, alertID)
	return err
}
