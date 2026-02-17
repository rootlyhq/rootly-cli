package incidents

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
	// Get incident ID from args
	id := args[0]

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

	// For json/yaml: convert to map for clean output
	if format == "json" || format == "yaml" {
		data := incidentToMap(incident)
		return p.PrintObj(data, os.Stdout)
	}

	// For table/markdown: build key-value rows
	headers := []string{"Field", "Value"}
	rows := incidentDetailRows(incident)

	return p.PrintList(headers, rows, os.Stdout)
}

// incidentToMap converts an Incident to a map for JSON/YAML output.
//
//nolint:gocyclo
func incidentToMap(inc *api.Incident) map[string]interface{} {
	data := map[string]interface{}{
		"id":     inc.ID,
		"title":  inc.Title,
		"status": inc.Status,
	}

	if inc.SequentialID != "" {
		data["sequential_id"] = inc.SequentialID
	}
	if inc.Severity != "" {
		data["severity"] = inc.Severity
	}
	if inc.Summary != "" {
		data["summary"] = inc.Summary
	}
	if inc.Kind != "" {
		data["kind"] = inc.Kind
	}
	if inc.URL != "" {
		data["url"] = inc.URL
	}
	if inc.ShortURL != "" {
		data["short_url"] = inc.ShortURL
	}

	// Timestamps
	data["created_at"] = inc.CreatedAt.Format(time.RFC3339)
	if !inc.UpdatedAt.IsZero() {
		data["updated_at"] = inc.UpdatedAt.Format(time.RFC3339)
	}
	if inc.StartedAt != nil {
		data["started_at"] = inc.StartedAt.Format(time.RFC3339)
	}
	if inc.DetectedAt != nil {
		data["detected_at"] = inc.DetectedAt.Format(time.RFC3339)
	}
	if inc.AcknowledgedAt != nil {
		data["acknowledged_at"] = inc.AcknowledgedAt.Format(time.RFC3339)
	}
	if inc.MitigatedAt != nil {
		data["mitigated_at"] = inc.MitigatedAt.Format(time.RFC3339)
	}
	if inc.ResolvedAt != nil {
		data["resolved_at"] = inc.ResolvedAt.Format(time.RFC3339)
	}
	if inc.ClosedAt != nil {
		data["closed_at"] = inc.ClosedAt.Format(time.RFC3339)
	}
	if inc.InTriageAt != nil {
		data["in_triage_at"] = inc.InTriageAt.Format(time.RFC3339)
	}
	if inc.CancelledAt != nil {
		data["cancelled_at"] = inc.CancelledAt.Format(time.RFC3339)
	}
	if inc.ScheduledFor != nil {
		data["scheduled_for"] = inc.ScheduledFor.Format(time.RFC3339)
	}
	if inc.ScheduledUntil != nil {
		data["scheduled_until"] = inc.ScheduledUntil.Format(time.RFC3339)
	}

	// Duration
	if duration := inc.Duration(); duration > 0 {
		data["duration_seconds"] = duration
	}

	// People
	if inc.CommanderName != "" {
		data["commander"] = inc.CommanderName
	}
	if inc.CommunicatorName != "" {
		data["communicator"] = inc.CommunicatorName
	}
	if inc.CreatedByName != "" {
		data["created_by"] = map[string]string{
			"name":  inc.CreatedByName,
			"email": inc.CreatedByEmail,
		}
	}
	if inc.StartedByName != "" {
		data["started_by"] = map[string]string{
			"name":  inc.StartedByName,
			"email": inc.StartedByEmail,
		}
	}
	if inc.MitigatedByName != "" {
		data["mitigated_by"] = map[string]string{
			"name":  inc.MitigatedByName,
			"email": inc.MitigatedByEmail,
		}
	}
	if inc.ResolvedByName != "" {
		data["resolved_by"] = map[string]string{
			"name":  inc.ResolvedByName,
			"email": inc.ResolvedByEmail,
		}
	}

	// Resources
	if len(inc.Services) > 0 {
		data["services"] = inc.Services
	}
	if len(inc.Teams) > 0 {
		data["teams"] = inc.Teams
	}
	if len(inc.Environments) > 0 {
		data["environments"] = inc.Environments
	}
	if len(inc.Causes) > 0 {
		data["causes"] = inc.Causes
	}
	if len(inc.IncidentTypes) > 0 {
		data["incident_types"] = inc.IncidentTypes
	}
	if len(inc.Functionalities) > 0 {
		data["functionalities"] = inc.Functionalities
	}
	if len(inc.Roles) > 0 {
		roles := make([]map[string]string, len(inc.Roles))
		for i, r := range inc.Roles {
			roles[i] = map[string]string{
				"name":       r.Name,
				"user_name":  r.UserName,
				"user_email": r.UserEmail,
			}
		}
		data["roles"] = roles
	}

	// Links
	if inc.SlackChannelURL != "" {
		data["slack_channel_url"] = inc.SlackChannelURL
	}
	if inc.JiraIssueURL != "" {
		data["jira_issue_url"] = inc.JiraIssueURL
	}
	if inc.GoogleMeetingURL != "" {
		data["google_meeting_url"] = inc.GoogleMeetingURL
	}
	if inc.LinearIssueURL != "" {
		data["linear_issue_url"] = inc.LinearIssueURL
	}
	if inc.ZoomMeetingJoinURL != "" {
		data["zoom_meeting_url"] = inc.ZoomMeetingJoinURL
	}
	if inc.GithubIssueURL != "" {
		data["github_issue_url"] = inc.GithubIssueURL
	}
	if inc.GitlabIssueURL != "" {
		data["gitlab_issue_url"] = inc.GitlabIssueURL
	}
	if inc.PagerdutyIncidentURL != "" {
		data["pagerduty_incident_url"] = inc.PagerdutyIncidentURL
	}
	if inc.OpsgenieIncidentURL != "" {
		data["opsgenie_incident_url"] = inc.OpsgenieIncidentURL
	}
	if inc.AsanaTaskURL != "" {
		data["asana_task_url"] = inc.AsanaTaskURL
	}
	if inc.TrelloCardURL != "" {
		data["trello_card_url"] = inc.TrelloCardURL
	}
	if inc.ConfluencePageURL != "" {
		data["confluence_page_url"] = inc.ConfluencePageURL
	}
	if inc.DatadogNotebookURL != "" {
		data["datadog_notebook_url"] = inc.DatadogNotebookURL
	}
	if inc.ServiceNowIncidentURL != "" {
		data["servicenow_incident_url"] = inc.ServiceNowIncidentURL
	}
	if inc.FreshserviceTicketURL != "" {
		data["freshservice_ticket_url"] = inc.FreshserviceTicketURL
	}

	// Additional fields
	if inc.Source != "" {
		data["source"] = inc.Source
	}
	if inc.Private {
		data["private"] = inc.Private
	}
	if inc.MitigationMessage != "" {
		data["mitigation_message"] = inc.MitigationMessage
	}
	if inc.ResolutionMessage != "" {
		data["resolution_message"] = inc.ResolutionMessage
	}
	if inc.RetrospectiveProgressStatus != "" {
		data["retrospective_progress_status"] = inc.RetrospectiveProgressStatus
	}
	if inc.SlackChannelName != "" {
		data["slack_channel_name"] = inc.SlackChannelName
	}
	if inc.SlackChannelArchived {
		data["slack_channel_archived"] = inc.SlackChannelArchived
	}
	if len(inc.Labels) > 0 {
		data["labels"] = inc.Labels
	}

	return data
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
	addRow("Created", inc.CreatedAt.Format(time.RFC3339))
	addRow("Started", formatTimePtr(inc.StartedAt))
	addRow("Detected", formatTimePtr(inc.DetectedAt))
	addRow("Acknowledged", formatTimePtr(inc.AcknowledgedAt))
	addRow("Mitigated", formatTimePtr(inc.MitigatedAt))
	addRow("Resolved", formatTimePtr(inc.ResolvedAt))
	addRow("Closed", formatTimePtr(inc.ClosedAt))
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

	// Links
	addRow("Slack Channel", inc.SlackChannelURL)
	addRow("Jira Issue", inc.JiraIssueURL)
	addRow("Google Meet", inc.GoogleMeetingURL)
	addRow("Linear Issue", inc.LinearIssueURL)
	addRow("Zoom Meeting", inc.ZoomMeetingJoinURL)
	addRow("GitHub Issue", inc.GithubIssueURL)
	addRow("GitLab Issue", inc.GitlabIssueURL)
	addRow("PagerDuty", inc.PagerdutyIncidentURL)
	addRow("Opsgenie", inc.OpsgenieIncidentURL)

	// Additional fields
	addRow("Source", inc.Source)
	if inc.MitigationMessage != "" {
		addRow("Mitigation Message", inc.MitigationMessage)
	}
	if inc.ResolutionMessage != "" {
		addRow("Resolution Message", inc.ResolutionMessage)
	}
	addRow("Retrospective", inc.RetrospectiveProgressStatus)

	return rows
}

// formatTimePtr formats a time pointer, returning "-" if nil.
func formatTimePtr(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format(time.RFC3339)
}
