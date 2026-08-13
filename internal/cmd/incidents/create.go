package incidents

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/printer"
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
    --services=api-gateway,payments \
    --types=customer-impacting \
    --functionalities=checkout \
    --environments=production \
    --teams=platform \
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
	createCmd.Flags().StringSlice("services", nil, "Service slugs/IDs, comma-separated")
	createCmd.Flags().StringSlice("types", nil, "Incident type slugs/IDs, comma-separated")
	createCmd.Flags().StringSlice("functionalities", nil, "Functionality slugs/IDs, comma-separated")
	createCmd.Flags().StringSlice("environments", nil, "Environment slugs/IDs, comma-separated")
	createCmd.Flags().StringSlice("teams", nil, "Team slugs/IDs, comma-separated")
	createCmd.Flags().StringSlice("causes", nil, "Cause slugs/IDs, comma-separated")

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
	services, _ := cmd.Flags().GetStringSlice("services")
	incidentTypes, _ := cmd.Flags().GetStringSlice("types")
	functionalities, _ := cmd.Flags().GetStringSlice("functionalities")
	environments, _ := cmd.Flags().GetStringSlice("environments")
	teams, _ := cmd.Flags().GetStringSlice("teams")
	causes, _ := cmd.Flags().GetStringSlice("causes")

	// Build opts map - only add keys where the flag was provided
	opts := make(map[string]interface{})
	if summary != "" {
		opts["summary"] = summary
	}
	if severity != "" {
		opts["severity_id"] = severity
	}
	if status != "" {
		opts["status"] = status
	}
	if len(services) > 0 {
		opts["service_ids"] = services
	}
	if len(incidentTypes) > 0 {
		opts["incident_type_ids"] = incidentTypes
	}
	if len(functionalities) > 0 {
		opts["functionality_ids"] = functionalities
	}
	if len(environments) > 0 {
		opts["environment_ids"] = environments
	}
	if len(teams) > 0 {
		opts["group_ids"] = teams
	}
	if len(causes) > 0 {
		opts["cause_ids"] = causes
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

	if format == "json" || format == "yaml" {
		return p.PrintRawJSON(incident.RawBody, os.Stdout)
	}
	return p.PrintObj(incident, os.Stdout)
}
