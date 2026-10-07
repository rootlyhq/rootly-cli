package alerts

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var escalateCmd = &cobra.Command{
	Use:     "escalate <alert-id>",
	Short:   "Escalate an alert",
	Example: "  rootly alerts escalate <alert-id> --escalation-policy=<uuid> --level=2",
	Args:    cobra.ExactArgs(1),
	RunE:    runEscalate,
}

func init() {
	escalateCmd.Flags().String("escalation-policy", "", "Escalation policy UUID")
	escalateCmd.Flags().Int("level", 0, "Escalation policy level")
	AlertsCmd.AddCommand(escalateCmd)
}

func runEscalate(cmd *cobra.Command, args []string) error {
	var policyID *string
	if cmd.Flags().Changed("escalation-policy") {
		value, _ := cmd.Flags().GetString("escalation-policy")
		policyID = &value
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
