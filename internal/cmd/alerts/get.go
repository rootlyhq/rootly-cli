package alerts

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/printer"
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

	// For json/yaml: convert to map for clean output
	if format == "json" || format == "yaml" {
		data := alertToMap(alert)
		return p.PrintObj(data, os.Stdout)
	}

	// For table/markdown: build key-value rows
	headers := []string{"Field", "Value"}
	rows := alertDetailRows(alert)

	return p.PrintList(headers, rows, os.Stdout)
}

// alertToMap converts an Alert to a map for JSON/YAML output.
func alertToMap(alert *api.Alert) map[string]interface{} {
	data := map[string]interface{}{
		"id":      alert.ID,
		"summary": alert.Summary,
		"status":  alert.Status,
	}

	if alert.ShortID != "" {
		data["short_id"] = alert.ShortID
	}
	if alert.Description != "" {
		data["description"] = alert.Description
	}
	if alert.Source != "" {
		data["source"] = alert.Source
	}
	if alert.URL != "" {
		data["url"] = alert.URL
	}
	if alert.ExternalURL != "" {
		data["external_url"] = alert.ExternalURL
	}
	if alert.ExternalID != "" {
		data["external_id"] = alert.ExternalID
	}

	// Timestamps
	data["created_at"] = alert.CreatedAt.Format(time.RFC3339)
	if !alert.UpdatedAt.IsZero() {
		data["updated_at"] = alert.UpdatedAt.Format(time.RFC3339)
	}
	if alert.StartedAt != nil {
		data["started_at"] = alert.StartedAt.Format(time.RFC3339)
	}
	if alert.EndedAt != nil {
		data["ended_at"] = alert.EndedAt.Format(time.RFC3339)
	}

	// Additional fields
	if alert.Urgency != "" {
		data["urgency"] = alert.Urgency
	}
	if alert.Noise != "" {
		data["noise"] = alert.Noise
	}
	if alert.DeduplicationKey != "" {
		data["deduplication_key"] = alert.DeduplicationKey
	}
	if alert.IsGroupLeaderAlert {
		data["is_group_leader_alert"] = alert.IsGroupLeaderAlert
	}
	if alert.GroupLeaderAlertID != "" {
		data["group_leader_alert_id"] = alert.GroupLeaderAlertID
	}

	// Arrays (if len > 0)
	if len(alert.Services) > 0 {
		data["services"] = alert.Services
	}
	if len(alert.Environments) > 0 {
		data["environments"] = alert.Environments
	}
	if len(alert.Groups) > 0 {
		data["groups"] = alert.Groups
	}
	if len(alert.Responders) > 0 {
		data["responders"] = alert.Responders
	}

	// Labels (if len > 0)
	if len(alert.Labels) > 0 {
		data["labels"] = alert.Labels
	}

	// Notified Users (if len > 0)
	if len(alert.NotifiedUsers) > 0 {
		users := make([]map[string]string, len(alert.NotifiedUsers))
		for i, u := range alert.NotifiedUsers {
			users[i] = map[string]string{
				"name":  u.Name,
				"email": u.Email,
			}
		}
		data["notified_users"] = users
	}

	// Related Incidents (if len > 0)
	if len(alert.RelatedIncidents) > 0 {
		incidents := make([]map[string]string, len(alert.RelatedIncidents))
		for i, inc := range alert.RelatedIncidents {
			incidents[i] = map[string]string{
				"id":            inc.ID,
				"sequential_id": inc.SequentialID,
				"title":         inc.Title,
				"status":        inc.Status,
			}
		}
		data["related_incidents"] = incidents
	}

	// Raw data (if len > 0)
	if len(alert.Data) > 0 {
		data["data"] = alert.Data
	}

	return data
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
	addRow("Created", alert.CreatedAt.Format(time.RFC3339))
	addRow("Started", formatTimePtr(alert.StartedAt))
	addRow("Ended", formatTimePtr(alert.EndedAt))

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

// formatTimePtr formats a time pointer, returning "-" if nil.
func formatTimePtr(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format(time.RFC3339)
}
