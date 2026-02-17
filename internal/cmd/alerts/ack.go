package alerts

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var ackCmd = &cobra.Command{
	Use:     "ack <alert-id>",
	Aliases: []string{"acknowledge"},
	Short:   "Acknowledge an alert",
	Long:    "Mark an alert as acknowledged. Changes status from triggered to acknowledged.\n\nThis calls the dedicated acknowledge action endpoint.",
	Example: `  # Acknowledge by short ID
  rootly alerts ack ALR-123

  # Acknowledge by UUID
  rootly alerts ack 01234567-89ab-cdef-0123-456789abcdef

  # Using full command name
  rootly alerts acknowledge ALR-123`,
	Args: cobra.ExactArgs(1),
	RunE: runAck,
}

func init() {
	// Register with parent command
	AlertsCmd.AddCommand(ackCmd)
}

func runAck(cmd *cobra.Command, args []string) error {
	alertID := args[0]

	// Get API client
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}

	// Call API
	err = apiClient.AcknowledgeAlertCLI(cmd.Context(), alertID)
	if err != nil {
		return fmt.Errorf("failed to acknowledge alert: %w", err)
	}

	// Print success message to stdout
	fmt.Fprintf(os.Stdout, "Acknowledged alert %s\n", alertID)
	return nil
}
