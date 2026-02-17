package incidents

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-tui/internal/printer"
)

var updateCmd = &cobra.Command{
	Use:   "update <incident-id>",
	Short: "Update an incident",
	Long:  "Update an existing Rootly incident's attributes.",
	Example: `  # Update status
  rootly incidents update INC-123 --status=mitigated

  # Update title and summary
  rootly incidents update INC-123 \
    --title="Updated: Database outage" \
    --summary="Root cause identified: connection pool exhaustion"

  # Update severity
  rootly incidents update INC-123 --severity=sev1`,
	Args: cobra.ExactArgs(1),
	RunE: runUpdate,
}

func init() {
	updateCmd.Flags().String("title", "", "Updated incident title")
	updateCmd.Flags().String("summary", "", "Updated summary")
	updateCmd.Flags().String("severity", "", "Updated severity ID")
	updateCmd.Flags().String("status", "", "Updated status (started, mitigated, resolved, closed, cancelled)")

	// Register with parent command
	IncidentsCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	incidentID := args[0]

	// Get API client
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}

	// Build opts map using cmd.Flags().Changed() - ONLY include fields the user explicitly set
	opts := make(map[string]string)
	if cmd.Flags().Changed("title") {
		title, _ := cmd.Flags().GetString("title")
		opts["title"] = title
	}
	if cmd.Flags().Changed("summary") {
		summary, _ := cmd.Flags().GetString("summary")
		opts["summary"] = summary
	}
	if cmd.Flags().Changed("severity") {
		severity, _ := cmd.Flags().GetString("severity")
		opts["severity_id"] = severity
	}
	if cmd.Flags().Changed("status") {
		status, _ := cmd.Flags().GetString("status")
		opts["status"] = status
	}

	// If opts is empty (no flags changed), return error
	if len(opts) == 0 {
		return fmt.Errorf("at least one field must be specified for update")
	}

	// Call API
	incident, err := apiClient.UpdateIncident(cmd.Context(), incidentID, opts)
	if err != nil {
		return fmt.Errorf("failed to update incident: %w", err)
	}

	// Print success message to stderr
	fmt.Fprintf(os.Stderr, "Updated incident %s\n", incidentID)

	// Print incident to stdout using configured format
	format := viper.GetString("format")
	p, err := printer.NewPrinter(format)
	if err != nil {
		return err
	}

	return p.PrintObj(incident, os.Stdout)
}
