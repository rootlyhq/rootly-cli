package alerts

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/printer"
	"github.com/rootlyhq/rootly-cli/internal/timeformat"
)

var getCmd = &cobra.Command{
	Use:   "get <alert-id>",
	Short: "Get alert details",
	Long:  "Get detailed information about a specific alert by ID.",
	Example: `  # Get alert by short ID
  rootly alerts get ALR-123

  # Get alert by UUID
  rootly alerts get 01234567-89ab-cdef-0123-456789abcdef

  # Output as JSON
  rootly alerts get ALR-123 --format=json

  # Output as YAML
  rootly alerts get ALR-123 --format=yaml`,
	Args: cobra.ExactArgs(1),
	RunE: runGet,
}

func init() {
	AlertsCmd.AddCommand(getCmd)
}

func runGet(cmd *cobra.Command, args []string) error {
	// Get alert ID from args
	id := args[0]

	// Get API client
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}

	// Call API
	alert, err := apiClient.GetAlertByID(cmd.Context(), id)
	if err != nil {
		return fmt.Errorf("failed to get alert: %w", err)
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
		return p.PrintRawJSON(alert.RawBody, os.Stdout)
	}

	// For table/markdown: build key-value rows
	headers := []string{"Field", "Value"}
	rows := alertDetailRows(alert)

	return p.PrintList(headers, rows, os.Stdout)
}

// alertDetailRows builds key-value rows for table/markdown output.
func alertDetailRows(alert *api.Alert) [][]string {
	rows := make([][]string, 0, 30)

	// Helper to add non-empty rows
	addRow := func(field, value string) {
		if value != "" && value != "-" {
			rows = append(rows, []string{field, value})
		}
	}

	// Basic info
	if alert.ShortID != "" {
		addRow("ID", alert.ShortID)
	} else {
		addRow("ID", alert.ID)
	}
	addRow("Summary", alert.Summary)
	addRow("Status", alert.Status)
	addRow("Source", alert.Source)
	addRow("Description", alert.Description)
	addRow("Urgency", alert.Urgency)
	addRow("URL", alert.URL)
	addRow("External URL", alert.ExternalURL)
	addRow("External ID", alert.ExternalID)

	// Timestamps
	addRow("Created", timeformat.FormatTime(alert.CreatedAt))
	addRow("Started", timeformat.FormatTimePtr(alert.StartedAt))
	addRow("Ended", timeformat.FormatTimePtr(alert.EndedAt))

	// Resources
	addRow("Services", strings.Join(alert.Services, ", "))
	addRow("Environments", strings.Join(alert.Environments, ", "))
	addRow("Groups", strings.Join(alert.Groups, ", "))
	addRow("Responders", strings.Join(alert.Responders, ", "))

	// Notified Users
	if len(alert.NotifiedUsers) > 0 {
		userNames := make([]string, len(alert.NotifiedUsers))
		for i, u := range alert.NotifiedUsers {
			userNames[i] = u.Name
		}
		addRow("Notified Users", strings.Join(userNames, ", "))
	}

	// Related Incidents
	if len(alert.RelatedIncidents) > 0 {
		incidentIDs := make([]string, len(alert.RelatedIncidents))
		for i, inc := range alert.RelatedIncidents {
			if inc.SequentialID != "" {
				incidentIDs[i] = inc.SequentialID
			} else {
				incidentIDs[i] = inc.ID
			}
		}
		addRow("Related Incidents", strings.Join(incidentIDs, ", "))
	}

	// Additional fields
	addRow("Noise", alert.Noise)
	addRow("Deduplication Key", alert.DeduplicationKey)

	return rows
}
