package api

import (
	"testing"
	"time"
)

func TestParseTime(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantZero bool
	}{
		{"valid RFC3339", "2025-06-15T10:30:00Z", false},
		{"valid with offset", "2025-06-15T10:30:00-07:00", false},
		{"empty string", "", true},
		{"invalid format", "June 15 2025", true},
		{"garbage", "not-a-date", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTime(tt.input)
			if tt.wantZero && !result.IsZero() {
				t.Errorf("parseTime(%q) = %v, want zero time", tt.input, result)
			}
			if !tt.wantZero && result.IsZero() {
				t.Errorf("parseTime(%q) = zero time, want non-zero", tt.input)
			}
		})
	}
}

func TestIncidentDuration(t *testing.T) {
	now := time.Now()
	hourAgo := now.Add(-1 * time.Hour)
	twoHoursAgo := now.Add(-2 * time.Hour)

	tests := []struct {
		name     string
		incident Incident
		wantMin  int64
		wantMax  int64
	}{
		{
			name: "resolved incident",
			incident: Incident{
				StartedAt:  &twoHoursAgo,
				ResolvedAt: &hourAgo,
			},
			wantMin: 3599,
			wantMax: 3601,
		},
		{
			name: "ongoing incident",
			incident: Incident{
				StartedAt: &hourAgo,
			},
			wantMin: 3598,
			wantMax: 3602,
		},
		{
			name: "cancelled incident",
			incident: Incident{
				StartedAt:   &twoHoursAgo,
				CancelledAt: &hourAgo,
			},
			wantMin: 3599,
			wantMax: 3601,
		},
		{
			name: "cancelled without start, with triage",
			incident: Incident{
				InTriageAt:  &twoHoursAgo,
				CancelledAt: &hourAgo,
			},
			wantMin: 3599,
			wantMax: 3601,
		},
		{
			name:     "no times set",
			incident: Incident{},
			wantMin:  0,
			wantMax:  0,
		},
		{
			name: "in triage, ongoing",
			incident: Incident{
				InTriageAt: &hourAgo,
			},
			wantMin: 3598,
			wantMax: 3602,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.incident.Duration()
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("Duration() = %d, want between %d and %d", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestParseIncidentData(t *testing.T) {
	seqID := 42
	startedAt := "2025-06-15T10:00:00Z"
	slackURL := "https://slack.com/channel"
	jiraURL := "https://jira.com/issue/123"

	d := incidentResponseData{
		ID: "inc-abc",
	}
	d.Attributes.SequentialID = &seqID
	d.Attributes.Title = " Database Outage "
	d.Attributes.Summary = " Major DB failure "
	d.Attributes.Status = " started "
	d.Attributes.Kind = "normal"
	d.Attributes.CreatedAt = "2025-06-15T09:00:00Z"
	d.Attributes.StartedAt = &startedAt
	d.Attributes.SlackChannelURL = &slackURL
	d.Attributes.JiraIssueURL = &jiraURL

	inc := parseIncidentData(d)

	if inc.ID != "inc-abc" {
		t.Errorf("ID = %q, want %q", inc.ID, "inc-abc")
	}
	if inc.SequentialID != "INC-42" {
		t.Errorf("SequentialID = %q, want %q", inc.SequentialID, "INC-42")
	}
	if inc.Title != "Database Outage" {
		t.Errorf("Title = %q, want %q", inc.Title, "Database Outage")
	}
	if inc.Summary != "Major DB failure" {
		t.Errorf("Summary = %q, want %q", inc.Summary, "Major DB failure")
	}
	if inc.Status != "started" {
		t.Errorf("Status = %q, want %q", inc.Status, "started")
	}
	if inc.Kind != "normal" {
		t.Errorf("Kind = %q, want %q", inc.Kind, "normal")
	}
	if inc.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if inc.StartedAt == nil {
		t.Error("StartedAt should not be nil")
	}
	if inc.SlackChannelURL != slackURL {
		t.Errorf("SlackChannelURL = %q, want %q", inc.SlackChannelURL, slackURL)
	}
	if inc.JiraIssueURL != jiraURL {
		t.Errorf("JiraIssueURL = %q, want %q", inc.JiraIssueURL, jiraURL)
	}
}

func TestParseIncidentDataWithSeverity(t *testing.T) {
	d := incidentResponseData{
		ID: "inc-sev",
	}
	d.Attributes.Title = "Sev Test"
	d.Attributes.Status = "started"
	d.Attributes.CreatedAt = "2025-01-01T00:00:00Z"
	d.Attributes.Severity = &struct {
		Data *struct {
			Attributes *struct {
				Name string `json:"name"`
			} `json:"attributes"`
		} `json:"data"`
	}{
		Data: &struct {
			Attributes *struct {
				Name string `json:"name"`
			} `json:"attributes"`
		}{
			Attributes: &struct {
				Name string `json:"name"`
			}{Name: "sev0"},
		},
	}

	inc := parseIncidentData(d)

	if inc.Severity != "sev0" {
		t.Errorf("Severity = %q, want %q", inc.Severity, "sev0")
	}
}

func TestParseIncidentDataWithServices(t *testing.T) {
	d := incidentResponseData{
		ID: "inc-svc",
	}
	d.Attributes.Title = "Service Test"
	d.Attributes.Status = "started"
	d.Attributes.CreatedAt = "2025-01-01T00:00:00Z"
	d.Attributes.Services = &struct {
		Data []struct {
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		} `json:"data"`
	}{
		Data: []struct {
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		}{
			{Attributes: struct {
				Name string `json:"name"`
			}{Name: "api-gateway"}},
			{Attributes: struct {
				Name string `json:"name"`
			}{Name: "auth-service"}},
		},
	}

	inc := parseIncidentData(d)

	if len(inc.Services) != 2 {
		t.Fatalf("Services count = %d, want 2", len(inc.Services))
	}
	if inc.Services[0] != "api-gateway" {
		t.Errorf("Services[0] = %q, want %q", inc.Services[0], "api-gateway")
	}
	if inc.Services[1] != "auth-service" {
		t.Errorf("Services[1] = %q, want %q", inc.Services[1], "auth-service")
	}
}

func TestParseIncidentDataMinimal(t *testing.T) {
	d := incidentResponseData{
		ID: "inc-min",
	}
	d.Attributes.Title = "Minimal"
	d.Attributes.Status = "started"
	d.Attributes.CreatedAt = "2025-01-01T00:00:00Z"

	inc := parseIncidentData(d)

	if inc.ID != "inc-min" {
		t.Errorf("ID = %q, want %q", inc.ID, "inc-min")
	}
	if inc.SequentialID != "" {
		t.Errorf("SequentialID = %q, want empty", inc.SequentialID)
	}
	if inc.Severity != "" {
		t.Errorf("Severity = %q, want empty", inc.Severity)
	}
	if len(inc.Services) != 0 {
		t.Errorf("Services count = %d, want 0", len(inc.Services))
	}
	if inc.SlackChannelURL != "" {
		t.Errorf("SlackChannelURL = %q, want empty", inc.SlackChannelURL)
	}
}

func TestParseIncidentDataWithEnvironmentsAndGroups(t *testing.T) {
	d := incidentResponseData{
		ID: "inc-env",
	}
	d.Attributes.Title = "Env Test"
	d.Attributes.Status = "started"
	d.Attributes.CreatedAt = "2025-01-01T00:00:00Z"

	slackURL := "https://slack.com/channel"
	jiraURL := "https://jira.com/PROJ-1"
	d.Attributes.SlackChannelURL = &slackURL
	d.Attributes.JiraIssueURL = &jiraURL

	d.Attributes.Environments = &struct {
		Data []struct {
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		} `json:"data"`
	}{
		Data: []struct {
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		}{
			{Attributes: struct {
				Name string `json:"name"`
			}{Name: "production"}},
		},
	}

	d.Attributes.Groups = &struct {
		Data []struct {
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		} `json:"data"`
	}{
		Data: []struct {
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		}{
			{Attributes: struct {
				Name string `json:"name"`
			}{Name: "Platform"}},
		},
	}

	inc := parseIncidentData(d)

	if len(inc.Environments) != 1 || inc.Environments[0] != "production" {
		t.Errorf("Environments = %v, want [production]", inc.Environments)
	}
	if len(inc.Teams) != 1 || inc.Teams[0] != "Platform" {
		t.Errorf("Teams = %v, want [Platform]", inc.Teams)
	}
	if inc.SlackChannelURL != "https://slack.com/channel" {
		t.Errorf("SlackChannelURL = %q", inc.SlackChannelURL)
	}
	if inc.JiraIssueURL != "https://jira.com/PROJ-1" {
		t.Errorf("JiraIssueURL = %q", inc.JiraIssueURL)
	}
}

func TestDurationTillCancelled(t *testing.T) {
	started := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	cancelled := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	inTriage := time.Date(2025, 6, 15, 9, 50, 0, 0, time.UTC)

	tests := []struct {
		name     string
		incident Incident
		wantSecs int64
	}{
		{"from started", Incident{StartedAt: &started, CancelledAt: &cancelled}, 1800},
		{"from triage", Incident{InTriageAt: &inTriage, CancelledAt: &cancelled}, 2400},
		{"no start", Incident{CancelledAt: &cancelled}, 0},
		{"no cancel", Incident{StartedAt: &started}, 0},
		{"both nil", Incident{}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.incident.durationTillCancelled()
			if got != tt.wantSecs {
				t.Errorf("durationTillCancelled() = %d, want %d", got, tt.wantSecs)
			}
		})
	}
}

func TestSetStringFromPtr(t *testing.T) {
	tests := []struct {
		name string
		src  *string
		want string
	}{
		{"nil src", nil, "initial"},
		{"non-nil src", strPtr("updated"), "updated"},
		{"empty src", strPtr(""), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dst := "initial"
			setStringFromPtr(&dst, tt.src)
			if dst != tt.want {
				t.Errorf("dst = %q, want %q", dst, tt.want)
			}
		})
	}
}
