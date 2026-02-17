package alerts

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-tui/internal/printer"
)

var updateCmd = &cobra.Command{
	Use:   "update <alert-id>",
	Short: "Update an alert",
	Long:  "Update an existing Rootly alert's attributes.",
	Example: `  # Update status
  rootly alerts update ALR-123 --status=acknowledged

  # Update summary and description
  rootly alerts update ALR-123 \
    --summary="Updated: API latency spike" \
    --description="Root cause identified: database query optimization needed"

  # Update source
  rootly alerts update ALR-123 --source=pagerduty`,
	Args: cobra.ExactArgs(1),
	RunE: runUpdate,
}

func init() {
	updateCmd.Flags().String("summary", "", "Updated alert summary")
	updateCmd.Flags().String("description", "", "Updated description")
	updateCmd.Flags().String("source", "", "Updated source")
	updateCmd.Flags().String("status", "", "Updated status (triggered, acknowledged, resolved)")
	updateCmd.Flags().String("external-url", "", "Updated external URL")

	// Register with parent command
	AlertsCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	alertID := args[0]

	// Get API client
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}

	// Build opts map using cmd.Flags().Changed() - ONLY include fields the user explicitly set
	opts := make(map[string]string)
	if cmd.Flags().Changed("summary") {
		summary, _ := cmd.Flags().GetString("summary")
		opts["summary"] = summary
	}
	if cmd.Flags().Changed("description") {
		description, _ := cmd.Flags().GetString("description")
		opts["description"] = description
	}
	if cmd.Flags().Changed("source") {
		source, _ := cmd.Flags().GetString("source")
		opts["source"] = source
	}
	if cmd.Flags().Changed("status") {
		status, _ := cmd.Flags().GetString("status")
		opts["status"] = status
	}
	if cmd.Flags().Changed("external-url") {
		externalURL, _ := cmd.Flags().GetString("external-url")
		opts["external_url"] = externalURL
	}

	// If opts is empty (no flags changed), return error
	if len(opts) == 0 {
		return fmt.Errorf("at least one field must be specified for update")
	}

	// Call API
	alert, err := apiClient.UpdateAlertCLI(cmd.Context(), alertID, opts)
	if err != nil {
		return fmt.Errorf("failed to update alert: %w", err)
	}

	// Print success message to stderr
	fmt.Fprintf(os.Stderr, "Updated alert %s\n", alertID)

	// Print alert to stdout using configured format
	format := viper.GetString("format")
	p, err := printer.NewPrinter(format)
	if err != nil {
		return err
	}

	return p.PrintObj(alert, os.Stdout)
}
