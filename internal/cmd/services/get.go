package services

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/printer"
	"github.com/rootlyhq/rootly-cli/internal/timeformat"
)

var getCmd = &cobra.Command{
	Use:   "get <service-id>",
	Short: "Get service details",
	Long:  "Get detailed information about a specific service by ID or slug.",
	Example: `  # Get service by slug
  rootly services get api-gateway

  # Get service by UUID
  rootly services get 01234567-89ab-cdef-0123-456789abcdef

  # Output as JSON
  rootly services get api-gateway --format=json

  # Output as YAML
  rootly services get api-gateway --format=yaml`,
	Args: cobra.ExactArgs(1),
	RunE: runGet,
}

func init() {
	ServicesCmd.AddCommand(getCmd)
}

func runGet(cmd *cobra.Command, args []string) error {
	// Get service ID from args
	id := args[0]

	// Get API client
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}

	// Call API
	service, err := apiClient.GetServiceByID(cmd.Context(), id)
	if err != nil {
		return fmt.Errorf("failed to get service: %w", err)
	}

	// Get format from viper
	format := viper.GetString("format")

	// Create printer
	p, err := printer.NewPrinter(format)
	if err != nil {
		return err
	}

	// For json/yaml: pass through raw API response
	if format == "json" || format == "yaml" {
		return p.PrintRawJSON(service.RawBody, os.Stdout)
	}

	// For table/markdown: build key-value rows
	headers := []string{"Field", "Value"}
	rows := serviceDetailRows(service)

	return p.PrintList(headers, rows, os.Stdout)
}

// serviceDetailRows converts a Service to table rows for display.
func serviceDetailRows(svc *api.Service) [][]string {
	rows := [][]string{
		{"ID", svc.ID},
		{"Name", svc.Name},
		{"Slug", svc.Slug},
	}

	if svc.Description != "" {
		rows = append(rows, []string{"Description", svc.Description})
	}
	if svc.Color != "" {
		rows = append(rows, []string{"Color", svc.Color})
	}
	if svc.EscalationPolicyID != "" {
		rows = append(rows, []string{"Escalation Policy", svc.EscalationPolicyID})
	}
	if svc.OwnerTeamName != "" {
		rows = append(rows, []string{"Owner Team", svc.OwnerTeamName})
	}
	if !svc.CreatedAt.IsZero() {
		rows = append(rows, []string{"Created", timeformat.FormatTime(svc.CreatedAt)})
	}
	if !svc.UpdatedAt.IsZero() {
		rows = append(rows, []string{"Updated", timeformat.FormatTime(svc.UpdatedAt)})
	}

	return rows
}
