package incidents

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
	Use:   "get <incident-id>",
	Short: "Get incident details",
	Long:  "Get detailed information about a specific incident by ID.",
	Example: `  # Get incident by sequential ID
  rootly incidents get INC-123

  # Get incident by UUID
  rootly incidents get 01234567-89ab-cdef-0123-456789abcdef

  # Output as JSON
  rootly incidents get INC-123 --format=json

  # Output as YAML
  rootly incidents get INC-123 --format=yaml`,
	Args: cobra.ExactArgs(1),
	RunE: runGet,
}

func init() {
	IncidentsCmd.AddCommand(getCmd)
}

func runGet(cmd *cobra.Command, args []string) error {
	id := api.NormalizeIncidentID(args[0])

	// Get API client
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}

	// Call API
	incident, err := apiClient.GetIncidentByID(cmd.Context(), id)
	if err != nil {
		return fmt.Errorf("failed to get incident: %w", err)
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
		return p.PrintRawJSON(incident.RawBody, os.Stdout)
	}

	// For table/markdown: build key-value rows
	headers := []string{"Field", "Value"}
	rows := incidentDetailRows(incident)

	return p.PrintList(headers, rows, os.Stdout)
}

// incidentDetailRows builds key-value rows for table/markdown output.
func incidentDetailRows(inc *api.Incident) [][]string {
	rows := make([][]string, 0, 30)

	// Helper to add non-empty rows
	addRow := func(field, value string) {
		if value != "" && value != "-" {
			rows = append(rows, []string{field, value})
		}
	}

	// Basic info
	if inc.SequentialID != "" {
		addRow("ID", inc.SequentialID)
	} else {
		addRow("ID", inc.ID)
	}
	addRow("Title", inc.Title)
	addRow("Status", inc.Status)
	addRow("Severity", inc.Severity)
	addRow("Summary", inc.Summary)
	addRow("Kind", inc.Kind)
	addRow("URL", inc.URL)

	// Timestamps
	addRow("Created", timeformat.FormatTime(inc.CreatedAt))
	addRow("Updated", timeformat.FormatTime(inc.UpdatedAt))
	addRow("Started", timeformat.FormatTimePtr(inc.StartedAt))
	addRow("Detected", timeformat.FormatTimePtr(inc.DetectedAt))
	addRow("Acknowledged", timeformat.FormatTimePtr(inc.AcknowledgedAt))
	addRow("Mitigated", timeformat.FormatTimePtr(inc.MitigatedAt))
	addRow("Resolved", timeformat.FormatTimePtr(inc.ResolvedAt))
	addRow("Closed", timeformat.FormatTimePtr(inc.ClosedAt))
	addRow("Duration", formatDuration(inc.Duration()))

	// People
	addRow("Commander", inc.CommanderName)
	addRow("Communicator", inc.CommunicatorName)
	addRow("Created By", inc.CreatedByName)
	if inc.StartedByName != "" {
		addRow("Started By", inc.StartedByName)
	}
	if inc.MitigatedByName != "" {
		addRow("Mitigated By", inc.MitigatedByName)
	}
	if inc.ResolvedByName != "" {
		addRow("Resolved By", inc.ResolvedByName)
	}

	// Resources
	addRow("Services", strings.Join(inc.Services, ", "))
	addRow("Teams", strings.Join(inc.Teams, ", "))
	addRow("Environments", strings.Join(inc.Environments, ", "))
	addRow("Causes", strings.Join(inc.Causes, ", "))
	addRow("Incident Types", strings.Join(inc.IncidentTypes, ", "))
	addRow("Functionalities", strings.Join(inc.Functionalities, ", "))

	// Labels
	if len(inc.Labels) > 0 {
		labelParts := make([]string, 0, len(inc.Labels))
		for k, v := range inc.Labels {
			labelParts = append(labelParts, k+"="+v)
		}
		addRow("Labels", strings.Join(labelParts, ", "))
	}

	// Links
	addRow("Slack Channel", inc.SlackChannelURL)
	if inc.SlackChannelName != "" {
		addRow("Slack Channel Name", inc.SlackChannelName)
	}
	addRow("Jira Issue", inc.JiraIssueURL)
	addRow("Google Meet", inc.GoogleMeetingURL)
	addRow("Linear Issue", inc.LinearIssueURL)
	addRow("Zoom Meeting", inc.ZoomMeetingJoinURL)
	addRow("GitHub Issue", inc.GithubIssueURL)
	addRow("GitLab Issue", inc.GitlabIssueURL)
	addRow("PagerDuty", inc.PagerdutyIncidentURL)
	addRow("Opsgenie", inc.OpsgenieIncidentURL)
	addRow("Asana Task", inc.AsanaTaskURL)
	addRow("Trello Card", inc.TrelloCardURL)
	addRow("Confluence Page", inc.ConfluencePageURL)
	addRow("Datadog Notebook", inc.DatadogNotebookURL)
	addRow("ServiceNow", inc.ServiceNowIncidentURL)
	addRow("Freshservice", inc.FreshserviceTicketURL)

	// Additional fields
	addRow("Source", inc.Source)
	if inc.Private {
		addRow("Private", "true")
	}
	if inc.MitigationMessage != "" {
		addRow("Mitigation Message", inc.MitigationMessage)
	}
	if inc.ResolutionMessage != "" {
		addRow("Resolution Message", inc.ResolutionMessage)
	}
	addRow("Retrospective", inc.RetrospectiveProgressStatus)

	return rows
}
