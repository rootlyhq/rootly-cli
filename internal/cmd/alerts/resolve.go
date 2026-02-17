package alerts

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var resolveCmd = &cobra.Command{
	Use:   "resolve <alert-id>",
	Short: "Resolve an alert",
	Long:  "Mark an alert as resolved with optional resolution message.\n\nThis calls the dedicated resolve action endpoint. Optionally resolves related incidents.",
	Example: `  # Basic resolve
  rootly alerts resolve ALR-123

  # Resolve with message
  rootly alerts resolve ALR-123 --message "Issue fixed by restarting service"

  # Resolve and also resolve related incidents
  rootly alerts resolve ALR-123 --resolve-incidents --message "All systems recovered"`,
	Args: cobra.ExactArgs(1),
	RunE: runResolve,
}

func init() {
	resolveCmd.Flags().String("message", "", "Resolution message")
	resolveCmd.Flags().Bool("resolve-incidents", false, "Also resolve related incidents")

	// Register with parent command
	AlertsCmd.AddCommand(resolveCmd)
}

func runResolve(cmd *cobra.Command, args []string) error {
	alertID := args[0]

	// Read flags
	message, _ := cmd.Flags().GetString("message")
	resolveIncidents, _ := cmd.Flags().GetBool("resolve-incidents")

	// Get API client
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}

	// Call API
	err = apiClient.ResolveAlertCLI(cmd.Context(), alertID, message, resolveIncidents)
	if err != nil {
		return fmt.Errorf("failed to resolve alert: %w", err)
	}

	// Print success message to stdout
	_, _ = fmt.Fprintf(os.Stdout, "Resolved alert %s\n", alertID)
	return nil
}
