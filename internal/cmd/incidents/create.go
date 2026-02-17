package incidents

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-tui/internal/printer"
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new incident",
	Long:  "Create a new Rootly incident with specified attributes.",
	Example: `  # Create with title only
  rootly incidents create --title="Database outage"

  # Create with full details
  rootly incidents create \
    --title="API degradation" \
    --summary="Response times elevated above 2s p99" \
    --severity=sev0 \
    --status=started

  # Create and output as JSON
  rootly incidents create --title="Deployment issue" --format=json`,
	RunE: runCreate,
}

func init() {
	createCmd.Flags().String("title", "", "Incident title (required)")
	createCmd.Flags().String("summary", "", "Incident summary/description")
	createCmd.Flags().String("severity", "", "Severity ID")
	createCmd.Flags().String("status", "", "Initial status (started, mitigated, resolved)")

	// Mark title as required
	_ = createCmd.MarkFlagRequired("title")

	// Register with parent command
	IncidentsCmd.AddCommand(createCmd)
}

func runCreate(cmd *cobra.Command, args []string) error {
	// Get API client
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}

	// Read flags
	title, _ := cmd.Flags().GetString("title")
	summary, _ := cmd.Flags().GetString("summary")
	severity, _ := cmd.Flags().GetString("severity")
	status, _ := cmd.Flags().GetString("status")

	// Build opts map - only add keys where the flag was provided
	opts := make(map[string]string)
	if summary != "" {
		opts["summary"] = summary
	}
	if severity != "" {
		opts["severity_id"] = severity
	}
	if status != "" {
		opts["status"] = status
	}

	// Call API
	incident, err := apiClient.CreateIncident(cmd.Context(), title, opts)
	if err != nil {
		return fmt.Errorf("failed to create incident: %w", err)
	}

	// Print success message to stderr
	fmt.Fprintf(os.Stderr, "Created incident %s\n", incident.SequentialID)

	// Print incident to stdout using configured format
	format := viper.GetString("format")
	p, err := printer.NewPrinter(format)
	if err != nil {
		return err
	}

	return p.PrintObj(incident, os.Stdout)
}
