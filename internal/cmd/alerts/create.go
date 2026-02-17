package alerts

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/printer"
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new alert",
	Long:  "Create a new Rootly alert with specified attributes.",
	Example: `  # Create with summary only
  rootly alerts create --summary="Database connection pool exhausted"

  # Create with full details
  rootly alerts create \
    --summary="API latency spike detected" \
    --description="p99 latency exceeded 2s threshold" \
    --source=datadog \
    --status=triggered

  # Create and output as JSON
  rootly alerts create --summary="Deployment failed" --format=json`,
	RunE: runCreate,
}

func init() {
	createCmd.Flags().String("summary", "", "Alert summary (required)")
	createCmd.Flags().String("description", "", "Alert description")
	createCmd.Flags().String("source", "", "Alert source (e.g., datadog, pagerduty, sentry)")
	createCmd.Flags().String("status", "", "Initial status (triggered, acknowledged, resolved)")
	createCmd.Flags().String("external-url", "", "External URL for the alert source")

	// Mark summary as required
	_ = createCmd.MarkFlagRequired("summary")

	// Register with parent command
	AlertsCmd.AddCommand(createCmd)
}

func runCreate(cmd *cobra.Command, args []string) error {
	// Get API client
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}

	// Read flags
	summary, _ := cmd.Flags().GetString("summary")
	description, _ := cmd.Flags().GetString("description")
	source, _ := cmd.Flags().GetString("source")
	status, _ := cmd.Flags().GetString("status")
	externalURL, _ := cmd.Flags().GetString("external-url")

	// Build opts map - only add keys where the flag was provided
	opts := make(map[string]string)
	if description != "" {
		opts["description"] = description
	}
	if source != "" {
		opts["source"] = source
	}
	if status != "" {
		opts["status"] = status
	}
	if externalURL != "" {
		opts["external_url"] = externalURL
	}

	// Call API
	alert, err := apiClient.CreateAlertCLI(cmd.Context(), summary, opts)
	if err != nil {
		return fmt.Errorf("failed to create alert: %w", err)
	}

	// Print success message to stderr (use ShortID if available, fallback to ID)
	alertID := alert.ShortID
	if alertID == "" {
		alertID = alert.ID
	}
	fmt.Fprintf(os.Stderr, "Created alert %s\n", alertID)

	// Print alert to stdout using configured format
	format := viper.GetString("format")
	p, err := printer.NewPrinter(format)
	if err != nil {
		return err
	}

	return p.PrintObj(alert, os.Stdout)
}
