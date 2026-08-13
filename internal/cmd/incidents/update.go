package incidents

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/printer"
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
  rootly incidents update INC-123 --severity=sev1

  # Update attached services and incident types
  rootly incidents update INC-123 \
    --services=api-gateway,payments \
    --types=customer-impacting \
    --functionalities=checkout \
    --environments=production \
    --teams=platform`,
	Args: cobra.ExactArgs(1),
	RunE: runUpdate,
}

func init() {
	updateCmd.Flags().String("title", "", "Updated incident title")
	updateCmd.Flags().String("summary", "", "Updated summary")
	updateCmd.Flags().String("severity", "", "Updated severity ID")
	updateCmd.Flags().String("status", "", "Updated status (started, mitigated, resolved, closed, cancelled)")
	updateCmd.Flags().StringSlice("services", nil, "Updated service slugs/IDs, comma-separated")
	updateCmd.Flags().StringSlice("types", nil, "Updated incident type slugs/IDs, comma-separated")
	updateCmd.Flags().StringSlice("functionalities", nil, "Updated functionality slugs/IDs, comma-separated")
	updateCmd.Flags().StringSlice("environments", nil, "Updated environment slugs/IDs, comma-separated")
	updateCmd.Flags().StringSlice("teams", nil, "Updated team slugs/IDs, comma-separated")
	updateCmd.Flags().StringSlice("causes", nil, "Updated cause slugs/IDs, comma-separated")

	// Register with parent command
	IncidentsCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	displayID := args[0]
	incidentID := api.NormalizeIncidentID(displayID)

	// Get API client
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}

	// Build opts map using cmd.Flags().Changed() - ONLY include fields the user explicitly set
	opts := make(map[string]interface{})
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
	if cmd.Flags().Changed("services") {
		services, _ := cmd.Flags().GetStringSlice("services")
		opts["service_ids"] = services
	}
	if cmd.Flags().Changed("types") {
		incidentTypes, _ := cmd.Flags().GetStringSlice("types")
		opts["incident_type_ids"] = incidentTypes
	}
	if cmd.Flags().Changed("functionalities") {
		functionalities, _ := cmd.Flags().GetStringSlice("functionalities")
		opts["functionality_ids"] = functionalities
	}
	if cmd.Flags().Changed("environments") {
		environments, _ := cmd.Flags().GetStringSlice("environments")
		opts["environment_ids"] = environments
	}
	if cmd.Flags().Changed("teams") {
		teams, _ := cmd.Flags().GetStringSlice("teams")
		opts["group_ids"] = teams
	}
	if cmd.Flags().Changed("causes") {
		causes, _ := cmd.Flags().GetStringSlice("causes")
		opts["cause_ids"] = causes
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
	fmt.Fprintf(os.Stderr, "Updated incident %s\n", displayID)

	// Print incident to stdout using configured format
	format := viper.GetString("format")
	p, err := printer.NewPrinter(format)
	if err != nil {
		return err
	}

	if format == "json" || format == "yaml" {
		return p.PrintRawJSON(incident.RawBody, os.Stdout)
	}
	return p.PrintObj(incident, os.Stdout)
}
