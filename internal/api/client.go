package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	neturl "net/url"
	"os"
	"regexp"
	"strings"
	"time"

	rootly "github.com/rootlyhq/rootly-go"

	"github.com/rootlyhq/rootly-cli/internal/config"
	"github.com/rootlyhq/rootly-cli/internal/oauth"
)

// Version is set by the main package to include in User-Agent
var Version = "dev"

type Client struct {
	client     *rootly.ClientWithResponses
	endpoint   string
	apiKey     string
	httpClient *http.Client
}

// debugTransport wraps an http.RoundTripper and dumps requests/responses to stderr.
type debugTransport struct {
	transport http.RoundTripper
}

func (dt *debugTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	safeReq := req.Clone(req.Context())
	if auth := safeReq.Header.Get("Authorization"); auth != "" {
		safeReq.Header.Set("Authorization", "Bearer [REDACTED]")
	}
	safeReq.Body = http.NoBody
	dump, _ := httputil.DumpRequestOut(safeReq, false)
	fmt.Fprintf(os.Stderr, "\n--- DEBUG REQUEST ---\n%s\n", dump)

	resp, err := dt.transport.RoundTrip(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "--- DEBUG ERROR ---\n%v\n", err)
		return resp, err
	}

	dump, _ = httputil.DumpResponse(resp, true)
	fmt.Fprintf(os.Stderr, "--- DEBUG RESPONSE ---\n%s\n", dump)

	return resp, err
}

type Incident struct {
	ID              string
	SequentialID    string
	Title           string
	Summary         string
	Status          string
	Severity        string
	Kind            string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	StartedAt       *time.Time
	DetectedAt      *time.Time
	AcknowledgedAt  *time.Time
	MitigatedAt     *time.Time
	ResolvedAt      *time.Time
	InTriageAt      *time.Time
	ClosedAt        *time.Time
	CancelledAt     *time.Time
	ScheduledFor    *time.Time
	ScheduledUntil  *time.Time
	Services        []string
	Environments    []string
	Teams           []string
	SlackChannelURL string
	JiraIssueURL    string
	// Detail fields (populated by GetIncident)
	URL              string
	ShortURL         string
	Causes           []string
	IncidentTypes    []string
	Functionalities  []string
	Roles            []IncidentRole
	CommanderName    string
	CommunicatorName string
	CreatedByName    string
	CreatedByEmail   string
	DetailLoaded     bool
	// Additional detail fields
	Source                      string
	Private                     bool
	MitigationMessage           string
	ResolutionMessage           string
	RetrospectiveProgressStatus string
	SlackChannelName            string
	SlackChannelArchived        bool
	Labels                      map[string]string
	StartedByName               string
	StartedByEmail              string
	MitigatedByName             string
	MitigatedByEmail            string
	ResolvedByName              string
	ResolvedByEmail             string
	// Integration links
	GoogleMeetingURL      string
	LinearIssueURL        string
	ZoomMeetingJoinURL    string
	GithubIssueURL        string
	GitlabIssueURL        string
	PagerdutyIncidentURL  string
	OpsgenieIncidentURL   string
	AsanaTaskURL          string
	TrelloCardURL         string
	ConfluencePageURL     string
	DatadogNotebookURL    string
	ServiceNowIncidentURL string
	FreshserviceTicketURL string
	// Raw API response body for JSON/YAML passthrough
	RawBody []byte
}

type IncidentRole struct {
	Name      string
	UserName  string
	UserEmail string
}

type Alert struct {
	ID           string
	ShortID      string
	Summary      string
	Description  string
	Status       string
	Source       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	StartedAt    *time.Time
	EndedAt      *time.Time
	ExternalURL  string
	Services     []string
	Environments []string
	Groups       []string
	Labels       map[string]string
	// Detail fields (populated by GetAlert)
	Responders   []string
	Urgency      string
	DetailLoaded bool
	// Additional detail fields
	URL                string // Rootly URL
	ExternalID         string // External system ID (e.g., Sentry issue ID)
	Noise              string // "not_noise", "noise", etc.
	IsGroupLeaderAlert bool
	GroupLeaderAlertID string
	NotifiedUsers      []AlertUser     // Users who were notified
	RelatedIncidents   []AlertIncident // Related incidents
	DeduplicationKey   string
	Data               map[string]interface{} // Raw alert payload from source
	// Raw API response body for JSON/YAML passthrough
	RawBody []byte
}

// AlertUser represents a user who was notified about an alert
type AlertUser struct {
	Name  string
	Email string
}

// AlertIncident represents an incident related to an alert
type AlertIncident struct {
	ID           string
	SequentialID string
	Title        string
	Status       string
}

// PaginationInfo contains pagination state
type PaginationInfo struct {
	CurrentPage int
	TotalPages  int
	TotalCount  int
	HasNext     bool
	HasPrev     bool
}

// IncidentsResult contains incidents and pagination info
type IncidentsResult struct {
	Incidents  []Incident
	Pagination PaginationInfo
	RawBody    []byte
}

// AlertsResult contains alerts and pagination info
type AlertsResult struct {
	Alerts     []Alert
	Pagination PaginationInfo
	RawBody    []byte
}

// Service represents a Rootly service
type Service struct {
	ID                 string
	Name               string
	Slug               string
	Description        string
	Color              string
	EscalationPolicyID string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	OwnerTeamName      string // Populated from included owner_group relationship
	DetailLoaded       bool
	// Raw API response body for JSON/YAML passthrough
	RawBody []byte
}

// ServicesResult contains services and pagination info
type ServicesResult struct {
	Services   []Service
	Pagination PaginationInfo
	RawBody    []byte
}

// Team represents a Rootly team
type Team struct {
	ID           string
	Name         string
	Slug         string
	Description  string
	Color        string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	UserCount    int      // Populated from included users relationship count
	Users        []string // User names from included users relationship, for detail view
	DetailLoaded bool
	// Raw API response body for JSON/YAML passthrough
	RawBody []byte
}

// TeamsResult contains teams and pagination info
type TeamsResult struct {
	Teams      []Team
	Pagination PaginationInfo
	RawBody    []byte
}

// KeyValue represents a key-value pair for pulse labels and refs
type KeyValue struct {
	Key   string
	Value string
}

// Pulse represents a Rootly pulse (deployment/event signal)
type Pulse struct {
	ID           string
	Summary      string
	Source       string
	StartedAt    *time.Time
	EndedAt      *time.Time
	Services     []string
	Environments []string
	Labels       []KeyValue
	Refs         []KeyValue
	// Raw API response body for JSON/YAML passthrough
	RawBody []byte
}

// PulseOpts contains optional parameters for creating a pulse
type PulseOpts struct {
	Source         string
	ServiceIDs     []string
	EnvironmentIDs []string
	Labels         []KeyValue
	Refs           []KeyValue
	StartedAt      *time.Time
	EndedAt        *time.Time
}

// incidentResponseData represents the structure of incident data from the API response
type incidentResponseData struct {
	ID         string `json:"id"`
	Attributes struct {
		SequentialID *int   `json:"sequential_id"`
		Title        string `json:"title"`
		Summary      string `json:"summary"`
		Status       string `json:"status"`
		Severity     *struct {
			Data *struct {
				Attributes *struct {
					Name string `json:"name"`
				} `json:"attributes"`
			} `json:"data"`
		} `json:"severity"`
		Kind            string  `json:"kind"`
		CreatedAt       string  `json:"created_at"`
		StartedAt       *string `json:"started_at"`
		DetectedAt      *string `json:"detected_at"`
		AcknowledgedAt  *string `json:"acknowledged_at"`
		MitigatedAt     *string `json:"mitigated_at"`
		ResolvedAt      *string `json:"resolved_at"`
		InTriageAt      *string `json:"in_triage_at"`
		ClosedAt        *string `json:"closed_at"`
		CancelledAt     *string `json:"cancelled_at"`
		ScheduledFor    *string `json:"scheduled_for"`
		ScheduledUntil  *string `json:"scheduled_until"`
		SlackChannelURL *string `json:"slack_channel_url"`
		JiraIssueURL    *string `json:"jira_issue_url"`
		Services        *struct {
			Data []struct {
				Attributes struct {
					Name string `json:"name"`
				} `json:"attributes"`
			} `json:"data"`
		} `json:"services"`
		Environments *struct {
			Data []struct {
				Attributes struct {
					Name string `json:"name"`
				} `json:"attributes"`
			} `json:"data"`
		} `json:"environments"`
		Groups *struct {
			Data []struct {
				Attributes struct {
					Name string `json:"name"`
				} `json:"attributes"`
			} `json:"data"`
		} `json:"groups"`
	} `json:"attributes"`
}

// parseIncidentData converts API response data to an Incident struct
func parseIncidentData(d incidentResponseData) Incident {
	incident := Incident{
		ID:      d.ID,
		Title:   strings.TrimSpace(d.Attributes.Title),
		Summary: strings.TrimSpace(d.Attributes.Summary),
		Status:  strings.TrimSpace(d.Attributes.Status),
		Kind:    d.Attributes.Kind,
	}

	if d.Attributes.SequentialID != nil {
		incident.SequentialID = fmt.Sprintf("INC-%d", *d.Attributes.SequentialID)
	}

	if d.Attributes.Severity != nil && d.Attributes.Severity.Data != nil && d.Attributes.Severity.Data.Attributes != nil {
		incident.Severity = d.Attributes.Severity.Data.Attributes.Name
	}

	if t, err := time.Parse(time.RFC3339, d.Attributes.CreatedAt); err == nil {
		incident.CreatedAt = t
	}
	incident.StartedAt = parseTimePtr(d.Attributes.StartedAt)
	incident.DetectedAt = parseTimePtr(d.Attributes.DetectedAt)
	incident.AcknowledgedAt = parseTimePtr(d.Attributes.AcknowledgedAt)
	incident.MitigatedAt = parseTimePtr(d.Attributes.MitigatedAt)
	incident.ResolvedAt = parseTimePtr(d.Attributes.ResolvedAt)
	incident.InTriageAt = parseTimePtr(d.Attributes.InTriageAt)
	incident.ClosedAt = parseTimePtr(d.Attributes.ClosedAt)
	incident.CancelledAt = parseTimePtr(d.Attributes.CancelledAt)
	incident.ScheduledFor = parseTimePtr(d.Attributes.ScheduledFor)
	incident.ScheduledUntil = parseTimePtr(d.Attributes.ScheduledUntil)

	if d.Attributes.SlackChannelURL != nil {
		incident.SlackChannelURL = *d.Attributes.SlackChannelURL
	}
	if d.Attributes.JiraIssueURL != nil {
		incident.JiraIssueURL = *d.Attributes.JiraIssueURL
	}

	if d.Attributes.Services != nil {
		for _, s := range d.Attributes.Services.Data {
			incident.Services = append(incident.Services, s.Attributes.Name)
		}
	}
	if d.Attributes.Environments != nil {
		for _, e := range d.Attributes.Environments.Data {
			incident.Environments = append(incident.Environments, e.Attributes.Name)
		}
	}
	if d.Attributes.Groups != nil {
		for _, g := range d.Attributes.Groups.Data {
			incident.Teams = append(incident.Teams, g.Attributes.Name)
		}
	}

	return incident
}

// labelEntry represents a single key-value label.
type labelEntry struct {
	Key   string      `json:"key"`
	Value interface{} `json:"value"`
}

// flexibleLabels handles the API returning labels as either an array of {key,value}
// objects or as a plain object/map (e.g. empty {} or {"key":"value"}).
type flexibleLabels []labelEntry

func (f *flexibleLabels) UnmarshalJSON(data []byte) error {
	// Try array first (normal case)
	var arr []labelEntry
	if err := json.Unmarshal(data, &arr); err == nil {
		*f = arr
		return nil
	}
	// Fall back to object/map
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	result := make(flexibleLabels, 0, len(m))
	for k, v := range m {
		result = append(result, labelEntry{Key: k, Value: v})
	}
	*f = result
	return nil
}

// incidentDetailResponse is the full JSON:API response for a single incident with includes.
type incidentDetailResponse struct {
	Data struct {
		ID         string                   `json:"id"`
		Attributes incidentDetailAttributes `json:"attributes"`
	} `json:"data"`
}

type incidentDetailAttributes struct {
	SequentialID *int   `json:"sequential_id"`
	Title        string `json:"title"`
	Summary      string `json:"summary"`
	Status       string `json:"status"`
	Kind         string `json:"kind"`
	Private      bool   `json:"private"`
	Severity     *struct {
		Data *struct {
			Attributes *struct {
				Name string `json:"name"`
			} `json:"attributes"`
		} `json:"data"`
	} `json:"severity"`
	// Timestamps
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
	StartedAt      *string `json:"started_at"`
	DetectedAt     *string `json:"detected_at"`
	AcknowledgedAt *string `json:"acknowledged_at"`
	MitigatedAt    *string `json:"mitigated_at"`
	ResolvedAt     *string `json:"resolved_at"`
	InTriageAt     *string `json:"in_triage_at"`
	ClosedAt       *string `json:"closed_at"`
	CancelledAt    *string `json:"cancelled_at"`
	ScheduledFor   *string `json:"scheduled_for"`
	ScheduledUntil *string `json:"scheduled_until"`
	// URLs
	URL      *string `json:"url"`
	ShortURL *string `json:"short_url"`
	// Detail fields
	Source                      *string `json:"source"`
	MitigationMessage           *string `json:"mitigation_message"`
	ResolutionMessage           *string `json:"resolution_message"`
	RetrospectiveProgressStatus *string `json:"retrospective_progress_status"`
	SlackChannelName            *string `json:"slack_channel_name"`
	// Labels
	Labels flexibleLabels `json:"labels"`
	// Integration links
	SlackChannelURL       *string `json:"slack_channel_url"`
	JiraIssueURL          *string `json:"jira_issue_url"`
	GoogleMeetingURL      *string `json:"google_meeting_url"`
	LinearIssueURL        *string `json:"linear_issue_url"`
	ZoomMeetingJoinURL    *string `json:"zoom_meeting_join_url"`
	GithubIssueURL        *string `json:"github_issue_url"`
	GitlabIssueURL        *string `json:"gitlab_issue_url"`
	PagerdutyIncidentURL  *string `json:"pagerduty_incident_url"`
	OpsgenieIncidentURL   *string `json:"opsgenie_incident_url"`
	AsanaTaskURL          *string `json:"asana_task_url"`
	TrelloCardURL         *string `json:"trello_card_url"`
	ConfluencePageURL     *string `json:"confluence_page_url"`
	DatadogNotebookURL    *string `json:"datadog_notebook_url"`
	ServiceNowIncidentURL *string `json:"service_now_incident_url"`
	FreshserviceTicketURL *string `json:"freshservice_ticket_url"`
	// Nested relationships (embedded in attributes)
	Commander *struct {
		Data *struct {
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		} `json:"data"`
	} `json:"commander"`
	Communicator *struct {
		Data *struct {
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		} `json:"data"`
	} `json:"communicator"`
	User *struct {
		Data *struct {
			Attributes struct {
				FullName string `json:"full_name"`
				Email    string `json:"email"`
			} `json:"attributes"`
		} `json:"data"`
	} `json:"user"`
	StartedBy *struct {
		Data *struct {
			Attributes struct {
				FullName string `json:"full_name"`
				Email    string `json:"email"`
			} `json:"attributes"`
		} `json:"data"`
	} `json:"started_by"`
	MitigatedBy *struct {
		Data *struct {
			Attributes struct {
				FullName string `json:"full_name"`
				Email    string `json:"email"`
			} `json:"attributes"`
		} `json:"data"`
	} `json:"mitigated_by"`
	ResolvedBy *struct {
		Data *struct {
			Attributes struct {
				FullName string `json:"full_name"`
				Email    string `json:"email"`
			} `json:"attributes"`
		} `json:"data"`
	} `json:"resolved_by"`
	// Collection relationships
	Roles []struct {
		Attributes struct {
			Name string `json:"name"`
			User *struct {
				Data *struct {
					Attributes struct {
						FullName string `json:"full_name"`
						Email    string `json:"email"`
					} `json:"attributes"`
				} `json:"data"`
			} `json:"user"`
		} `json:"attributes"`
	} `json:"roles"`
	Causes []struct {
		Attributes struct {
			Name string `json:"name"`
		} `json:"attributes"`
	} `json:"causes"`
	IncidentTypes []struct {
		Attributes struct {
			Name string `json:"name"`
		} `json:"attributes"`
	} `json:"incident_types"`
	Functionalities []struct {
		Attributes struct {
			Name string `json:"name"`
		} `json:"attributes"`
	} `json:"functionalities"`
	Services *struct {
		Data []struct {
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		} `json:"data"`
	} `json:"services"`
	Environments *struct {
		Data []struct {
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		} `json:"data"`
	} `json:"environments"`
	Groups *struct {
		Data []struct {
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		} `json:"data"`
	} `json:"groups"`
}

// parseIncidentDetailResponse converts the full detail API response into an Incident.
func parseIncidentDetailResponse(result incidentDetailResponse, rawBody []byte) *Incident {
	d := result.Data
	a := d.Attributes

	incident := &Incident{
		ID:           d.ID,
		Title:        strings.TrimSpace(a.Title),
		Summary:      strings.TrimSpace(a.Summary),
		Status:       strings.TrimSpace(a.Status),
		Kind:         a.Kind,
		Private:      a.Private,
		DetailLoaded: true,
		RawBody:      rawBody,
		Labels:       make(map[string]string),
	}

	if a.SequentialID != nil {
		incident.SequentialID = fmt.Sprintf("INC-%d", *a.SequentialID)
	}
	if a.Severity != nil && a.Severity.Data != nil && a.Severity.Data.Attributes != nil {
		incident.Severity = a.Severity.Data.Attributes.Name
	}

	// Timestamps
	if t, err := time.Parse(time.RFC3339, a.CreatedAt); err == nil {
		incident.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, a.UpdatedAt); err == nil {
		incident.UpdatedAt = t
	}
	incident.StartedAt = parseTimePtr(a.StartedAt)
	incident.DetectedAt = parseTimePtr(a.DetectedAt)
	incident.AcknowledgedAt = parseTimePtr(a.AcknowledgedAt)
	incident.MitigatedAt = parseTimePtr(a.MitigatedAt)
	incident.ResolvedAt = parseTimePtr(a.ResolvedAt)
	incident.InTriageAt = parseTimePtr(a.InTriageAt)
	incident.ClosedAt = parseTimePtr(a.ClosedAt)
	incident.CancelledAt = parseTimePtr(a.CancelledAt)
	incident.ScheduledFor = parseTimePtr(a.ScheduledFor)
	incident.ScheduledUntil = parseTimePtr(a.ScheduledUntil)

	// Scalar detail fields
	setStringFromPtr(&incident.URL, a.URL)
	setStringFromPtr(&incident.ShortURL, a.ShortURL)
	setStringFromPtr(&incident.Source, a.Source)
	setStringFromPtr(&incident.MitigationMessage, a.MitigationMessage)
	setStringFromPtr(&incident.ResolutionMessage, a.ResolutionMessage)
	setStringFromPtr(&incident.RetrospectiveProgressStatus, a.RetrospectiveProgressStatus)
	setStringFromPtr(&incident.SlackChannelName, a.SlackChannelName)

	// Labels
	for _, l := range a.Labels {
		incident.Labels[l.Key] = fmt.Sprintf("%v", l.Value)
	}

	parseIncidentDetailLinks(incident, &a)
	parseIncidentDetailRelationships(incident, &a)

	return incident
}

// setStringFromPtr sets dst to *src if src is non-nil.
func setStringFromPtr(dst, src *string) {
	if src != nil {
		*dst = *src
	}
}

// parseIncidentDetailLinks populates integration link fields from the response.
func parseIncidentDetailLinks(incident *Incident, a *incidentDetailAttributes) {
	setStringFromPtr(&incident.SlackChannelURL, a.SlackChannelURL)
	setStringFromPtr(&incident.JiraIssueURL, a.JiraIssueURL)
	setStringFromPtr(&incident.GoogleMeetingURL, a.GoogleMeetingURL)
	setStringFromPtr(&incident.LinearIssueURL, a.LinearIssueURL)
	setStringFromPtr(&incident.ZoomMeetingJoinURL, a.ZoomMeetingJoinURL)
	setStringFromPtr(&incident.GithubIssueURL, a.GithubIssueURL)
	setStringFromPtr(&incident.GitlabIssueURL, a.GitlabIssueURL)
	setStringFromPtr(&incident.PagerdutyIncidentURL, a.PagerdutyIncidentURL)
	setStringFromPtr(&incident.OpsgenieIncidentURL, a.OpsgenieIncidentURL)
	setStringFromPtr(&incident.AsanaTaskURL, a.AsanaTaskURL)
	setStringFromPtr(&incident.TrelloCardURL, a.TrelloCardURL)
	setStringFromPtr(&incident.ConfluencePageURL, a.ConfluencePageURL)
	setStringFromPtr(&incident.DatadogNotebookURL, a.DatadogNotebookURL)
	setStringFromPtr(&incident.ServiceNowIncidentURL, a.ServiceNowIncidentURL)
	setStringFromPtr(&incident.FreshserviceTicketURL, a.FreshserviceTicketURL)
}

// parseIncidentDetailRelationships populates relationship fields from the response.
func parseIncidentDetailRelationships(incident *Incident, a *incidentDetailAttributes) {
	// Commander / Communicator
	if a.Commander != nil && a.Commander.Data != nil {
		incident.CommanderName = a.Commander.Data.Attributes.Name
	}
	if a.Communicator != nil && a.Communicator.Data != nil {
		incident.CommunicatorName = a.Communicator.Data.Attributes.Name
	}

	// User (creator)
	if a.User != nil && a.User.Data != nil {
		incident.CreatedByName = a.User.Data.Attributes.FullName
		incident.CreatedByEmail = a.User.Data.Attributes.Email
	}

	// Started/Mitigated/Resolved by
	if a.StartedBy != nil && a.StartedBy.Data != nil {
		incident.StartedByName = a.StartedBy.Data.Attributes.FullName
		incident.StartedByEmail = a.StartedBy.Data.Attributes.Email
	}
	if a.MitigatedBy != nil && a.MitigatedBy.Data != nil {
		incident.MitigatedByName = a.MitigatedBy.Data.Attributes.FullName
		incident.MitigatedByEmail = a.MitigatedBy.Data.Attributes.Email
	}
	if a.ResolvedBy != nil && a.ResolvedBy.Data != nil {
		incident.ResolvedByName = a.ResolvedBy.Data.Attributes.FullName
		incident.ResolvedByEmail = a.ResolvedBy.Data.Attributes.Email
	}

	// Roles
	for _, r := range a.Roles {
		role := IncidentRole{Name: r.Attributes.Name}
		if r.Attributes.User != nil && r.Attributes.User.Data != nil {
			role.UserName = r.Attributes.User.Data.Attributes.FullName
			role.UserEmail = r.Attributes.User.Data.Attributes.Email
		}
		incident.Roles = append(incident.Roles, role)
	}

	// Causes, incident types, functionalities
	for _, c := range a.Causes {
		incident.Causes = append(incident.Causes, c.Attributes.Name)
	}
	for _, it := range a.IncidentTypes {
		incident.IncidentTypes = append(incident.IncidentTypes, it.Attributes.Name)
	}
	for _, f := range a.Functionalities {
		incident.Functionalities = append(incident.Functionalities, f.Attributes.Name)
	}

	// Services, environments, teams
	if a.Services != nil {
		for _, s := range a.Services.Data {
			incident.Services = append(incident.Services, s.Attributes.Name)
		}
	}
	if a.Environments != nil {
		for _, e := range a.Environments.Data {
			incident.Environments = append(incident.Environments, e.Attributes.Name)
		}
	}
	if a.Groups != nil {
		for _, g := range a.Groups.Data {
			incident.Teams = append(incident.Teams, g.Attributes.Name)
		}
	}
}

// NewClient creates a stateless API client for CLI usage.
func NewClient(cfg *config.Config) (*Client, error) {
	endpoint := cfg.Endpoint
	if endpoint != "" {
		endpoint = ensureScheme(endpoint)
	}

	// Determine auth: use OAuth tokens only when no API key is set
	useOAuth := false
	if cfg.APIKey == "" {
		if tokens, err := oauth.LoadTokens(); err == nil && tokens.AccessToken != "" {
			useOAuth = true
		}
	}

	// Build base transport
	var transport http.RoundTripper
	transport = http.DefaultTransport

	if cfg.Debug {
		transport = &debugTransport{transport: transport}
	}

	var httpClient *http.Client
	if useOAuth {
		authBaseURL := oauth.DeriveAuthBaseURL(cfg.Endpoint)
		clientID, scopes := oauth.LoadCachedRegistration()
		oauthCfg := oauth.NewConfig(authBaseURL, clientID, scopes)
		var err error
		httpClient, err = oauth.NewHTTPClient(oauthCfg, transport, "rootly-cli/"+Version)
		if err != nil {
			return nil, fmt.Errorf("failed to create OAuth client: %w", err)
		}
	} else {
		httpClient = &http.Client{Transport: transport}
	}

	var reqEditorFn rootly.RequestEditorFn
	if useOAuth {
		// OAuth transport handles Authorization header
		reqEditorFn = func(ctx context.Context, req *http.Request) error {
			req.Header.Set("Content-Type", "application/vnd.api+json")
			req.Header.Set("User-Agent", "rootly-cli/"+Version)
			return nil
		}
	} else {
		reqEditorFn = func(ctx context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
			req.Header.Set("Content-Type", "application/vnd.api+json")
			req.Header.Set("User-Agent", "rootly-cli/"+Version)
			return nil
		}
	}

	client, err := rootly.NewClientWithResponses(endpoint,
		rootly.WithHTTPClient(httpClient),
		rootly.WithRequestEditorFn(reqEditorFn),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create rootly client: %w", err)
	}

	return &Client{
		client:     client,
		endpoint:   endpoint,
		apiKey:     cfg.APIKey,
		httpClient: httpClient,
	}, nil
}

// ensureScheme adds a scheme if missing, using http for localhost/127.0.0.1.
// For localhost without an explicit path, it also appends /api since the
// Rails monolith serves the API under /api/v1 rather than /v1.
func ensureScheme(endpoint string) string {
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		return endpoint
	}
	if strings.HasPrefix(endpoint, "localhost") || strings.HasPrefix(endpoint, "127.0.0.1") {
		result := "http://" + endpoint
		// Auto-append /api for localhost if no path is present
		if !strings.Contains(endpoint, "/") {
			result += "/api"
		}
		return result
	}
	return "https://" + endpoint
}

func (c *Client) ValidateAPIKey(ctx context.Context) error {
	// Use /v1/users/me endpoint to validate the API key
	resp, err := c.client.GetCurrentUserWithResponse(ctx)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	if resp.StatusCode() == 401 {
		return fmt.Errorf("invalid API key")
	}
	if resp.StatusCode() != 200 {
		return fmt.Errorf("API returned status %d", resp.StatusCode())
	}
	return nil
}

func parseTimePtr(s *string) *time.Time {
	if s == nil || *s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		return nil
	}
	return &t
}

// parseTime parses a time string in RFC3339 format, returning zero time if parsing fails.
// toStr converts an interface{} (string or number) to a string representation.
func toStr(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// Duration returns the total incident duration in seconds
func (i *Incident) Duration() int64 {
	if i.CancelledAt != nil {
		return i.durationTillCancelled()
	}
	return i.ongoingDuration()
}

// ongoingDuration returns duration from start to resolution (or now if ongoing)
func (i *Incident) ongoingDuration() int64 {
	if i.StartedAt != nil {
		endTime := i.ResolvedAt
		if endTime == nil {
			now := time.Now()
			endTime = &now
		}
		return int64(endTime.Sub(*i.StartedAt).Seconds())
	}
	if i.InTriageAt != nil {
		return int64(time.Since(*i.InTriageAt).Seconds())
	}
	return 0
}

// durationTillCancelled returns duration from start to cancellation
func (i *Incident) durationTillCancelled() int64 {
	startTime := i.StartedAt
	if startTime == nil {
		startTime = i.InTriageAt
	}
	if startTime == nil || i.CancelledAt == nil {
		return 0
	}
	return int64(i.CancelledAt.Sub(*startTime).Seconds())
}

// ListIncidentsCLI lists incidents with configurable page size and filters for CLI usage.
// Does not use cache (stateless).
func (c *Client) ListIncidentsCLI(ctx context.Context, page, pageSize int, sort string, filters map[string]string) (*IncidentsResult, error) {
	// Cap pageSize at 100 (API limit); if 0, default to 25
	if pageSize == 0 {
		pageSize = 25
	}
	if pageSize > 100 {
		pageSize = 100
	}

	// Build URL with query parameters
	baseURL := c.endpoint

	url := fmt.Sprintf("%s/v1/incidents?page[number]=%d&page[size]=%d", baseURL, page, pageSize)
	if sort != "" {
		url += fmt.Sprintf("&sort=%s", sort)
	}

	// Add filters (e.g., filter[status]=started, filter[severity]=critical)
	for key, value := range filters {
		url += fmt.Sprintf("&filter[%s]=%s", key, neturl.QueryEscape(value))
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list incidents: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode == 401 {
		return nil, fmt.Errorf("invalid API token")
	}
	if httpResp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied: API key lacks 'read incidents' permission")
	}
	if httpResp.StatusCode == 404 {
		return nil, fmt.Errorf("resource not found")
	}
	if httpResp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d", httpResp.StatusCode)
	}

	var result struct {
		Data  []incidentResponseData `json:"data"`
		Links struct {
			Next *string `json:"next"`
			Prev *string `json:"prev"`
		} `json:"links"`
		Meta struct {
			CurrentPage int  `json:"current_page"`
			NextPage    *int `json:"next_page"`
			PrevPage    *int `json:"prev_page"`
			TotalCount  int  `json:"total_count"`
			TotalPages  int  `json:"total_pages"`
		} `json:"meta"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	incidents := make([]Incident, 0, len(result.Data))
	for _, d := range result.Data {
		incidents = append(incidents, parseIncidentData(d))
	}

	// Build result with pagination info from Meta
	hasNext := result.Meta.NextPage != nil && *result.Meta.NextPage > 0
	if !hasNext && result.Links.Next != nil && *result.Links.Next != "" {
		hasNext = true
	}
	hasPrev := result.Meta.PrevPage != nil && *result.Meta.PrevPage > 0
	if !hasPrev && result.Links.Prev != nil && *result.Links.Prev != "" {
		hasPrev = true
	}

	currentPage := result.Meta.CurrentPage
	if currentPage == 0 {
		currentPage = page
	}

	return &IncidentsResult{
		Incidents: incidents,
		Pagination: PaginationInfo{
			CurrentPage: currentPage,
			TotalPages:  result.Meta.TotalPages,
			TotalCount:  result.Meta.TotalCount,
			HasNext:     hasNext,
			HasPrev:     hasPrev,
		},
		RawBody: body,
	}, nil
}

var incidentSeqIDPattern = regexp.MustCompile(`(?i)^INC-(\d+)$`)

// NormalizeIncidentID strips the "INC-" prefix from sequential IDs so the
// bare number can be passed to the API (which accepts UUID, slug, or numeric sequential ID).
func NormalizeIncidentID(id string) string {
	if m := incidentSeqIDPattern.FindStringSubmatch(id); m != nil {
		return m[1]
	}
	return id
}

// GetIncidentByID fetches incident detail by ID without requiring updatedAt parameter.
// Does not use cache (stateless).
func (c *Client) GetIncidentByID(ctx context.Context, id string) (*Incident, error) {
	// Build URL
	baseURL := c.endpoint

	url := fmt.Sprintf("%s/v1/incidents/%s?include=roles,causes,incident_types,functionalities,services,environments,groups,user", baseURL, id)
	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch incident: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode == 401 {
		return nil, fmt.Errorf("invalid API token")
	}
	if httpResp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied: API key lacks 'read incidents' permission")
	}
	if httpResp.StatusCode == 404 {
		return nil, fmt.Errorf("incident not found: %s", id)
	}
	if httpResp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d", httpResp.StatusCode)
	}

	var result incidentDetailResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	incident := parseIncidentDetailResponse(result, body)
	return incident, nil
}

// CreateIncident creates a new incident using raw HTTP POST.
func (c *Client) CreateIncident(ctx context.Context, title string, opts map[string]interface{}) (*Incident, error) {
	// Build JSON:API request body
	requestBody := map[string]interface{}{
		"data": map[string]interface{}{
			"type": "incidents",
			"attributes": map[string]interface{}{
				"title": title,
			},
		},
	}

	// Add optional attributes
	attributes := requestBody["data"].(map[string]interface{})["attributes"].(map[string]interface{})
	if summary, ok := opts["summary"]; ok {
		attributes["summary"] = summary
	}
	if severityID, ok := opts["severity_id"]; ok {
		attributes["severity_id"] = severityID
	}
	if status, ok := opts["status"]; ok {
		attributes["status"] = status
	}
	if serviceIDs, ok := opts["service_ids"]; ok {
		attributes["service_ids"] = serviceIDs
	}
	if incidentTypeIDs, ok := opts["incident_type_ids"]; ok {
		attributes["incident_type_ids"] = incidentTypeIDs
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Build URL
	baseURL := c.endpoint

	url := fmt.Sprintf("%s/v1/incidents", baseURL)

	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create incident: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode == 401 {
		return nil, fmt.Errorf("invalid API token")
	}
	if httpResp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied: API key lacks 'create incidents' permission")
	}
	if httpResp.StatusCode != 201 && httpResp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d", httpResp.StatusCode)
	}

	var result struct {
		Data incidentResponseData `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	incident := parseIncidentData(result.Data)
	incident.RawBody = body
	return &incident, nil
}

// UpdateIncident updates an incident using raw HTTP PUT.
func (c *Client) UpdateIncident(ctx context.Context, id string, opts map[string]interface{}) (*Incident, error) {
	// Build JSON:API request body with only changed attributes
	attributes := make(map[string]interface{})
	if title, ok := opts["title"]; ok {
		attributes["title"] = title
	}
	if summary, ok := opts["summary"]; ok {
		attributes["summary"] = summary
	}
	if severityID, ok := opts["severity_id"]; ok {
		attributes["severity_id"] = severityID
	}
	if status, ok := opts["status"]; ok {
		attributes["status"] = status
	}
	if serviceIDs, ok := opts["service_ids"]; ok {
		attributes["service_ids"] = serviceIDs
	}
	if incidentTypeIDs, ok := opts["incident_type_ids"]; ok {
		attributes["incident_type_ids"] = incidentTypeIDs
	}

	requestBody := map[string]interface{}{
		"data": map[string]interface{}{
			"type":       "incidents",
			"id":         id,
			"attributes": attributes,
		},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Build URL
	baseURL := c.endpoint

	url := fmt.Sprintf("%s/v1/incidents/%s", baseURL, id)

	req, err := http.NewRequestWithContext(ctx, "PUT", url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to update incident: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode == 401 {
		return nil, fmt.Errorf("invalid API token")
	}
	if httpResp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied: API key lacks 'update incidents' permission")
	}
	if httpResp.StatusCode == 404 {
		return nil, fmt.Errorf("incident not found: %s", id)
	}
	if httpResp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d", httpResp.StatusCode)
	}

	var result struct {
		Data incidentResponseData `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	incident := parseIncidentData(result.Data)
	incident.RawBody = body
	return &incident, nil
}

// DeleteIncident deletes an incident using raw HTTP DELETE.
func (c *Client) DeleteIncident(ctx context.Context, id string) error {
	// Build URL
	baseURL := c.endpoint

	url := fmt.Sprintf("%s/v1/incidents/%s", baseURL, id)

	req, err := http.NewRequestWithContext(ctx, "DELETE", url, http.NoBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete incident: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	if httpResp.StatusCode == 401 {
		return fmt.Errorf("invalid API token")
	}
	if httpResp.StatusCode == 403 {
		return fmt.Errorf("access denied: API key lacks 'delete incidents' permission")
	}
	if httpResp.StatusCode == 404 {
		return fmt.Errorf("incident not found: %s", id)
	}
	if httpResp.StatusCode != 204 && httpResp.StatusCode != 200 {
		return fmt.Errorf("API returned status %d", httpResp.StatusCode)
	}

	return nil
}

// alertResponseData represents the structure of alert data from the API response
type alertResponseData struct {
	ID         string `json:"id"`
	Attributes struct {
		ShortID     string  `json:"short_id"`
		Summary     string  `json:"summary"`
		Description *string `json:"description"`
		Status      *string `json:"status"`
		Source      string  `json:"source"`
		ExternalURL *string `json:"external_url"`
		CreatedAt   string  `json:"created_at"`
		UpdatedAt   string  `json:"updated_at"`
		StartedAt   *string `json:"started_at"`
		EndedAt     *string `json:"ended_at"`
		Services    []struct {
			Name string `json:"name"`
		} `json:"services"`
		Environments []struct {
			Name string `json:"name"`
		} `json:"environments"`
		Groups []struct {
			Name string `json:"name"`
		} `json:"groups"`
		Labels flexibleLabels `json:"labels"`
	} `json:"attributes"`
}

// parseAlertData converts API response data to an Alert struct
func parseAlertData(d alertResponseData) Alert {
	alert := Alert{
		ID:      d.ID,
		ShortID: strings.TrimSpace(d.Attributes.ShortID),
		Summary: strings.TrimSpace(d.Attributes.Summary),
		Source:  strings.TrimSpace(d.Attributes.Source),
		Labels:  make(map[string]string),
	}

	if d.Attributes.Description != nil {
		alert.Description = strings.TrimSpace(*d.Attributes.Description)
	}
	if d.Attributes.Status != nil {
		alert.Status = strings.TrimSpace(*d.Attributes.Status)
	}
	if d.Attributes.ExternalURL != nil {
		alert.ExternalURL = *d.Attributes.ExternalURL
	}

	if t, err := time.Parse(time.RFC3339, d.Attributes.CreatedAt); err == nil {
		alert.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, d.Attributes.UpdatedAt); err == nil {
		alert.UpdatedAt = t
	}
	alert.StartedAt = parseTimePtr(d.Attributes.StartedAt)
	alert.EndedAt = parseTimePtr(d.Attributes.EndedAt)

	for _, s := range d.Attributes.Services {
		alert.Services = append(alert.Services, s.Name)
	}
	for _, e := range d.Attributes.Environments {
		alert.Environments = append(alert.Environments, e.Name)
	}
	for _, g := range d.Attributes.Groups {
		alert.Groups = append(alert.Groups, g.Name)
	}
	for _, l := range d.Attributes.Labels {
		alert.Labels[l.Key] = fmt.Sprintf("%v", l.Value)
	}

	return alert
}

// ListAlertsCLI lists alerts with configurable page size and filters for CLI usage.
// Does not use cache (stateless).
func (c *Client) ListAlertsCLI(ctx context.Context, page, pageSize int, sort string, filters map[string]string) (*AlertsResult, error) {
	// Cap pageSize at 100 (API limit); if 0, default to 25
	if pageSize == 0 {
		pageSize = 25
	}
	if pageSize > 100 {
		pageSize = 100
	}

	// Build URL with query parameters
	baseURL := c.endpoint

	url := fmt.Sprintf("%s/v1/alerts?page[number]=%d&page[size]=%d", baseURL, page, pageSize)
	if sort != "" {
		url += fmt.Sprintf("&sort=%s", sort)
	}

	// Add filters (e.g., filter[status]=triggered, filter[source]=sentry)
	for key, value := range filters {
		url += fmt.Sprintf("&filter[%s]=%s", key, neturl.QueryEscape(value))
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list alerts: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode == 401 {
		return nil, fmt.Errorf("invalid API token")
	}
	if httpResp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied: API key lacks 'read alerts' permission")
	}
	if httpResp.StatusCode == 404 {
		return nil, fmt.Errorf("resource not found")
	}
	if httpResp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d", httpResp.StatusCode)
	}

	var result struct {
		Data  []alertResponseData `json:"data"`
		Links struct {
			Next *string `json:"next"`
			Prev *string `json:"prev"`
		} `json:"links"`
		Meta struct {
			CurrentPage int  `json:"current_page"`
			NextPage    *int `json:"next_page"`
			PrevPage    *int `json:"prev_page"`
			TotalCount  int  `json:"total_count"`
			TotalPages  int  `json:"total_pages"`
		} `json:"meta"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	alerts := make([]Alert, 0, len(result.Data))
	for _, d := range result.Data {
		alerts = append(alerts, parseAlertData(d))
	}

	// Build result with pagination info from Meta
	hasNext := result.Meta.NextPage != nil && *result.Meta.NextPage > 0
	if !hasNext && result.Links.Next != nil && *result.Links.Next != "" {
		hasNext = true
	}
	hasPrev := result.Meta.PrevPage != nil && *result.Meta.PrevPage > 0
	if !hasPrev && result.Links.Prev != nil && *result.Links.Prev != "" {
		hasPrev = true
	}

	currentPage := result.Meta.CurrentPage
	if currentPage == 0 {
		currentPage = page
	}

	return &AlertsResult{
		Alerts: alerts,
		Pagination: PaginationInfo{
			CurrentPage: currentPage,
			TotalPages:  result.Meta.TotalPages,
			TotalCount:  result.Meta.TotalCount,
			HasNext:     hasNext,
			HasPrev:     hasPrev,
		},
		RawBody: body,
	}, nil
}

// GetAlertByID fetches alert detail by ID without requiring updatedAt parameter.
// Does not use cache (stateless).
func (c *Client) GetAlertByID(ctx context.Context, id string) (*Alert, error) {
	// Build URL
	baseURL := c.endpoint

	url := fmt.Sprintf("%s/v1/alerts/%s?include=services,environments,groups,responders,alert_urgency", baseURL, id)
	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch alert: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode == 401 {
		return nil, fmt.Errorf("invalid API token")
	}
	if httpResp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied: API key lacks 'read alerts' permission")
	}
	if httpResp.StatusCode == 404 {
		return nil, fmt.Errorf("alert not found: %s", id)
	}
	if httpResp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d", httpResp.StatusCode)
	}

	var result struct {
		Data struct {
			ID         string `json:"id"`
			Attributes struct {
				ShortID     *string        `json:"short_id"`
				Summary     string         `json:"summary"`
				Description *string        `json:"description"`
				Status      string         `json:"status"`
				Source      *string        `json:"source"`
				ExternalURL *string        `json:"external_url"`
				CreatedAt   string         `json:"created_at"`
				UpdatedAt   string         `json:"updated_at"`
				StartedAt   *string        `json:"started_at"`
				EndedAt     *string        `json:"ended_at"`
				Labels      flexibleLabels `json:"labels"`
				Services    []struct {
					Name string `json:"name"`
				} `json:"services"`
				Environments []struct {
					Name string `json:"name"`
				} `json:"environments"`
				Groups []struct {
					Name string `json:"name"`
				} `json:"groups"`
				Responders []struct {
					ID         interface{} `json:"id"`
					Attributes struct {
						User *struct {
							Data *struct {
								Attributes struct {
									Name string `json:"name"`
								} `json:"attributes"`
							} `json:"data"`
						} `json:"user"`
					} `json:"attributes"`
				} `json:"responders"`
				AlertUrgency *struct {
					Data *struct {
						Attributes struct {
							Name string `json:"name"`
						} `json:"attributes"`
					} `json:"data"`
				} `json:"alert_urgency"`
				URL                *string `json:"url"`
				ExternalID         *string `json:"external_id"`
				Noise              *string `json:"noise"`
				IsGroupLeaderAlert bool    `json:"is_group_leader_alert"`
				GroupLeaderAlertID *string `json:"group_leader_alert_id"`
				DeduplicationKey   *string `json:"deduplication_key"`
				NotifiedUsers      []struct {
					Name  string `json:"name"`
					Email string `json:"email"`
				} `json:"notified_users"`
				Incidents []struct {
					ID         string `json:"id"`
					Attributes struct {
						SequentialID *int   `json:"sequential_id"`
						Title        string `json:"title"`
						Status       string `json:"status"`
					} `json:"attributes"`
				} `json:"incidents"`
				Data map[string]interface{} `json:"data"`
			} `json:"attributes"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	d := result.Data
	alert := &Alert{
		ID:           d.ID,
		Summary:      strings.TrimSpace(d.Attributes.Summary),
		Status:       strings.TrimSpace(d.Attributes.Status),
		Labels:       make(map[string]string),
		DetailLoaded: true,
	}

	if d.Attributes.ShortID != nil {
		alert.ShortID = strings.TrimSpace(*d.Attributes.ShortID)
	}
	if d.Attributes.Source != nil {
		alert.Source = *d.Attributes.Source
	}
	if d.Attributes.Description != nil {
		alert.Description = strings.TrimSpace(*d.Attributes.Description)
	}
	if d.Attributes.ExternalURL != nil {
		alert.ExternalURL = *d.Attributes.ExternalURL
	}

	if t, err := time.Parse(time.RFC3339, d.Attributes.CreatedAt); err == nil {
		alert.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, d.Attributes.UpdatedAt); err == nil {
		alert.UpdatedAt = t
	}
	alert.StartedAt = parseTimePtr(d.Attributes.StartedAt)
	alert.EndedAt = parseTimePtr(d.Attributes.EndedAt)

	for _, l := range d.Attributes.Labels {
		alert.Labels[l.Key] = fmt.Sprintf("%v", l.Value)
	}

	for _, s := range d.Attributes.Services {
		alert.Services = append(alert.Services, s.Name)
	}
	for _, e := range d.Attributes.Environments {
		alert.Environments = append(alert.Environments, e.Name)
	}
	for _, g := range d.Attributes.Groups {
		alert.Groups = append(alert.Groups, g.Name)
	}

	for _, r := range d.Attributes.Responders {
		if r.Attributes.User != nil && r.Attributes.User.Data != nil {
			alert.Responders = append(alert.Responders, r.Attributes.User.Data.Attributes.Name)
		}
	}

	if d.Attributes.AlertUrgency != nil && d.Attributes.AlertUrgency.Data != nil {
		alert.Urgency = d.Attributes.AlertUrgency.Data.Attributes.Name
	}

	if d.Attributes.URL != nil {
		alert.URL = *d.Attributes.URL
	}
	if d.Attributes.ExternalID != nil {
		alert.ExternalID = *d.Attributes.ExternalID
	}
	if d.Attributes.Noise != nil {
		alert.Noise = *d.Attributes.Noise
	}
	alert.IsGroupLeaderAlert = d.Attributes.IsGroupLeaderAlert
	if d.Attributes.GroupLeaderAlertID != nil {
		alert.GroupLeaderAlertID = *d.Attributes.GroupLeaderAlertID
	}
	if d.Attributes.DeduplicationKey != nil {
		alert.DeduplicationKey = *d.Attributes.DeduplicationKey
	}
	if d.Attributes.Data != nil {
		alert.Data = d.Attributes.Data
	}

	for _, u := range d.Attributes.NotifiedUsers {
		alert.NotifiedUsers = append(alert.NotifiedUsers, AlertUser{
			Name:  u.Name,
			Email: u.Email,
		})
	}

	for _, inc := range d.Attributes.Incidents {
		seqID := ""
		if inc.Attributes.SequentialID != nil {
			seqID = fmt.Sprintf("INC-%d", *inc.Attributes.SequentialID)
		}
		alert.RelatedIncidents = append(alert.RelatedIncidents, AlertIncident{
			ID:           inc.ID,
			SequentialID: seqID,
			Title:        inc.Attributes.Title,
			Status:       inc.Attributes.Status,
		})
	}

	alert.RawBody = body
	return alert, nil
}

// CreateAlertCLI creates a new alert using raw HTTP POST.
func (c *Client) CreateAlertCLI(ctx context.Context, summary string, opts map[string]string) (*Alert, error) {
	// Build JSON:API request body
	requestBody := map[string]interface{}{
		"data": map[string]interface{}{
			"type": "alerts",
			"attributes": map[string]interface{}{
				"summary": summary,
			},
		},
	}

	// Add optional attributes
	attributes := requestBody["data"].(map[string]interface{})["attributes"].(map[string]interface{})
	if description, ok := opts["description"]; ok {
		attributes["description"] = description
	}
	if source, ok := opts["source"]; ok {
		attributes["source"] = source
	}
	if status, ok := opts["status"]; ok {
		attributes["status"] = status
	}
	if externalURL, ok := opts["external_url"]; ok {
		attributes["external_url"] = externalURL
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Build URL
	baseURL := c.endpoint

	url := fmt.Sprintf("%s/v1/alerts", baseURL)

	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create alert: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode == 401 {
		return nil, fmt.Errorf("invalid API token")
	}
	if httpResp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied: API key lacks 'create alerts' permission")
	}
	if httpResp.StatusCode != 201 && httpResp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d", httpResp.StatusCode)
	}

	var result struct {
		Data alertResponseData `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	alert := parseAlertData(result.Data)
	alert.RawBody = body
	return &alert, nil
}

// UpdateAlertCLI updates an alert using raw HTTP PUT.
func (c *Client) UpdateAlertCLI(ctx context.Context, id string, opts map[string]string) (*Alert, error) {
	// Build JSON:API request body with only changed attributes
	attributes := make(map[string]interface{})
	if summary, ok := opts["summary"]; ok {
		attributes["summary"] = summary
	}
	if description, ok := opts["description"]; ok {
		attributes["description"] = description
	}
	if source, ok := opts["source"]; ok {
		attributes["source"] = source
	}
	if status, ok := opts["status"]; ok {
		attributes["status"] = status
	}
	if externalURL, ok := opts["external_url"]; ok {
		attributes["external_url"] = externalURL
	}

	requestBody := map[string]interface{}{
		"data": map[string]interface{}{
			"type":       "alerts",
			"id":         id,
			"attributes": attributes,
		},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Build URL
	baseURL := c.endpoint

	url := fmt.Sprintf("%s/v1/alerts/%s", baseURL, id)

	req, err := http.NewRequestWithContext(ctx, "PUT", url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to update alert: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode == 401 {
		return nil, fmt.Errorf("invalid API token")
	}
	if httpResp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied: API key lacks 'update alerts' permission")
	}
	if httpResp.StatusCode == 404 {
		return nil, fmt.Errorf("alert not found: %s", id)
	}
	if httpResp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d", httpResp.StatusCode)
	}

	var result struct {
		Data alertResponseData `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	alert := parseAlertData(result.Data)
	alert.RawBody = body
	return &alert, nil
}

// AcknowledgeAlertCLI acknowledges an alert using raw HTTP POST.
func (c *Client) AcknowledgeAlertCLI(ctx context.Context, id string) error {
	// Build URL
	baseURL := c.endpoint

	url := fmt.Sprintf("%s/v1/alerts/%s/acknowledge", baseURL, id)

	req, err := http.NewRequestWithContext(ctx, "POST", url, http.NoBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to acknowledge alert: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	if httpResp.StatusCode == 401 {
		return fmt.Errorf("invalid API token")
	}
	if httpResp.StatusCode == 403 {
		return fmt.Errorf("access denied: API key lacks 'acknowledge alerts' permission")
	}
	if httpResp.StatusCode == 404 {
		return fmt.Errorf("alert not found: %s", id)
	}
	if httpResp.StatusCode != 200 {
		return fmt.Errorf("API returned status %d", httpResp.StatusCode)
	}

	return nil
}

// ResolveAlertCLI resolves an alert using raw HTTP POST.
func (c *Client) ResolveAlertCLI(ctx context.Context, id, resolutionMessage string, resolveIncidents bool) error {
	// Build URL
	baseURL := c.endpoint

	url := fmt.Sprintf("%s/v1/alerts/%s/resolve", baseURL, id)

	var reqBody io.Reader = http.NoBody

	// Build body only if resolutionMessage != "" or resolveIncidents is true
	if resolutionMessage != "" || resolveIncidents {
		attributes := make(map[string]interface{})
		if resolutionMessage != "" {
			attributes["resolution_message"] = resolutionMessage
		}
		if resolveIncidents {
			attributes["resolve_related_incidents"] = true
		}

		requestBody := map[string]interface{}{
			"data": map[string]interface{}{
				"type":       "resolve_alert",
				"attributes": attributes,
			},
		}

		bodyBytes, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = strings.NewReader(string(bodyBytes))
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to resolve alert: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	if httpResp.StatusCode == 401 {
		return fmt.Errorf("invalid API token")
	}
	if httpResp.StatusCode == 403 {
		return fmt.Errorf("access denied: API key lacks 'resolve alerts' permission")
	}
	if httpResp.StatusCode == 404 {
		return fmt.Errorf("alert not found: %s", id)
	}
	if httpResp.StatusCode != 200 {
		return fmt.Errorf("API returned status %d", httpResp.StatusCode)
	}

	return nil
}

// ListServicesCLI fetches services for CLI operations (stateless, no cache).
func (c *Client) ListServicesCLI(ctx context.Context, page, pageSize int, sort string, filters map[string]string) (*ServicesResult, error) {
	// Cap pageSize at 100 (API limit); if 0, default to 25
	if pageSize == 0 {
		pageSize = 25
	}
	if pageSize > 100 {
		pageSize = 100
	}

	// Build URL with query parameters
	baseURL := c.endpoint

	url := fmt.Sprintf("%s/v1/services?page[number]=%d&page[size]=%d", baseURL, page, pageSize)
	if sort != "" {
		url += fmt.Sprintf("&sort=%s", sort)
	}

	// Add filters (e.g., filter[name]=foo, filter[slug]=bar)
	for key, value := range filters {
		url += fmt.Sprintf("&filter[%s]=%s", key, neturl.QueryEscape(value))
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode == 401 {
		return nil, fmt.Errorf("invalid API token")
	}
	if httpResp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied: API key lacks 'read services' permission")
	}
	if httpResp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d", httpResp.StatusCode)
	}

	// Parse JSON:API response
	var response struct {
		Data []struct {
			ID         string `json:"id"`
			Attributes struct {
				Name               string  `json:"name"`
				Slug               string  `json:"slug"`
				Description        *string `json:"description"`
				Color              *string `json:"color"`
				EscalationPolicyID *string `json:"escalation_policy_id"`
				CreatedAt          string  `json:"created_at"`
				UpdatedAt          string  `json:"updated_at"`
			} `json:"attributes"`
		} `json:"data"`
		Meta struct {
			CurrentPage int `json:"current_page"`
			TotalPages  int `json:"total_pages"`
			TotalCount  int `json:"total_count"`
		} `json:"meta"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse services response: %w", err)
	}

	// Convert to Service structs
	services := make([]Service, 0, len(response.Data))
	for _, item := range response.Data {
		service := Service{
			ID:        item.ID,
			Name:      item.Attributes.Name,
			Slug:      item.Attributes.Slug,
			CreatedAt: parseTime(item.Attributes.CreatedAt),
			UpdatedAt: parseTime(item.Attributes.UpdatedAt),
		}
		if item.Attributes.Description != nil {
			service.Description = *item.Attributes.Description
		}
		if item.Attributes.Color != nil {
			service.Color = *item.Attributes.Color
		}
		if item.Attributes.EscalationPolicyID != nil {
			service.EscalationPolicyID = *item.Attributes.EscalationPolicyID
		}
		services = append(services, service)
	}

	// Build pagination info
	pagination := PaginationInfo{
		CurrentPage: response.Meta.CurrentPage,
		TotalPages:  response.Meta.TotalPages,
		TotalCount:  response.Meta.TotalCount,
		HasNext:     response.Meta.CurrentPage < response.Meta.TotalPages,
		HasPrev:     response.Meta.CurrentPage > 1,
	}

	return &ServicesResult{
		Services:   services,
		Pagination: pagination,
		RawBody:    body,
	}, nil
}

// GetServiceByID fetches a single service by ID with detailed information.
func (c *Client) GetServiceByID(ctx context.Context, id string) (*Service, error) {
	// Build URL
	baseURL := c.endpoint

	url := fmt.Sprintf("%s/v1/services/%s?include=owner_group", baseURL, id)
	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch service: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode == 401 {
		return nil, fmt.Errorf("invalid API token")
	}
	if httpResp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied: API key lacks 'read services' permission")
	}
	if httpResp.StatusCode == 404 {
		return nil, fmt.Errorf("service not found: %s", id)
	}
	if httpResp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d", httpResp.StatusCode)
	}

	// Parse JSON:API response
	var response struct {
		Data struct {
			ID         string `json:"id"`
			Attributes struct {
				Name               string  `json:"name"`
				Slug               string  `json:"slug"`
				Description        *string `json:"description"`
				Color              *string `json:"color"`
				EscalationPolicyID *string `json:"escalation_policy_id"`
				CreatedAt          string  `json:"created_at"`
				UpdatedAt          string  `json:"updated_at"`
			} `json:"attributes"`
			Relationships struct {
				OwnerGroup struct {
					Data *struct {
						ID string `json:"id"`
					} `json:"data"`
				} `json:"owner_group"`
			} `json:"relationships"`
		} `json:"data"`
		Included []struct {
			ID         string `json:"id"`
			Type       string `json:"type"`
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		} `json:"included"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse service response: %w", err)
	}

	service := &Service{
		ID:           response.Data.ID,
		Name:         response.Data.Attributes.Name,
		Slug:         response.Data.Attributes.Slug,
		CreatedAt:    parseTime(response.Data.Attributes.CreatedAt),
		UpdatedAt:    parseTime(response.Data.Attributes.UpdatedAt),
		DetailLoaded: true,
	}
	if response.Data.Attributes.Description != nil {
		service.Description = *response.Data.Attributes.Description
	}
	if response.Data.Attributes.Color != nil {
		service.Color = *response.Data.Attributes.Color
	}
	if response.Data.Attributes.EscalationPolicyID != nil {
		service.EscalationPolicyID = *response.Data.Attributes.EscalationPolicyID
	}

	// Parse owner_group from included relationships
	if response.Data.Relationships.OwnerGroup.Data != nil {
		ownerGroupID := response.Data.Relationships.OwnerGroup.Data.ID
		for _, inc := range response.Included {
			if inc.Type == "groups" && inc.ID == ownerGroupID {
				service.OwnerTeamName = inc.Attributes.Name
				break
			}
		}
	}

	service.RawBody = body
	return service, nil
}

// CreateService creates a new service with the given name and optional attributes.
func (c *Client) CreateService(ctx context.Context, name string, opts map[string]string) (*Service, error) {
	// Build JSON:API request body
	requestBody := map[string]interface{}{
		"data": map[string]interface{}{
			"type": "services",
			"attributes": map[string]interface{}{
				"name": name,
			},
		},
	}

	// Add optional attributes
	attributes := requestBody["data"].(map[string]interface{})["attributes"].(map[string]interface{})
	if description, ok := opts["description"]; ok {
		attributes["description"] = description
	}
	if color, ok := opts["color"]; ok {
		attributes["color"] = color
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Build URL
	baseURL := c.endpoint

	url := fmt.Sprintf("%s/v1/services", baseURL)

	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create service: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode == 401 {
		return nil, fmt.Errorf("invalid API token")
	}
	if httpResp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied: API key lacks 'create services' permission")
	}
	if httpResp.StatusCode != 201 && httpResp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d", httpResp.StatusCode)
	}

	// Parse response
	var response struct {
		Data struct {
			ID         string `json:"id"`
			Attributes struct {
				Name               string  `json:"name"`
				Slug               string  `json:"slug"`
				Description        *string `json:"description"`
				Color              *string `json:"color"`
				EscalationPolicyID *string `json:"escalation_policy_id"`
				CreatedAt          string  `json:"created_at"`
				UpdatedAt          string  `json:"updated_at"`
			} `json:"attributes"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse create service response: %w", err)
	}

	service := &Service{
		ID:        response.Data.ID,
		Name:      response.Data.Attributes.Name,
		Slug:      response.Data.Attributes.Slug,
		CreatedAt: parseTime(response.Data.Attributes.CreatedAt),
		UpdatedAt: parseTime(response.Data.Attributes.UpdatedAt),
	}
	if response.Data.Attributes.Description != nil {
		service.Description = *response.Data.Attributes.Description
	}
	if response.Data.Attributes.Color != nil {
		service.Color = *response.Data.Attributes.Color
	}
	if response.Data.Attributes.EscalationPolicyID != nil {
		service.EscalationPolicyID = *response.Data.Attributes.EscalationPolicyID
	}

	service.RawBody = body
	return service, nil
}

// UpdateService updates an existing service with the given attributes.
func (c *Client) UpdateService(ctx context.Context, id string, opts map[string]string) (*Service, error) {
	// Build JSON:API request body with only changed attributes
	attributes := make(map[string]interface{})
	if name, ok := opts["name"]; ok {
		attributes["name"] = name
	}
	if description, ok := opts["description"]; ok {
		attributes["description"] = description
	}
	if color, ok := opts["color"]; ok {
		attributes["color"] = color
	}
	if epID, ok := opts["escalation_policy_id"]; ok {
		if epID == "" {
			attributes["escalation_policy_id"] = nil
		} else {
			attributes["escalation_policy_id"] = epID
		}
	}

	requestBody := map[string]interface{}{
		"data": map[string]interface{}{
			"type":       "services",
			"id":         id,
			"attributes": attributes,
		},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Build URL
	baseURL := c.endpoint

	url := fmt.Sprintf("%s/v1/services/%s", baseURL, id)

	req, err := http.NewRequestWithContext(ctx, "PUT", url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to update service: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode == 401 {
		return nil, fmt.Errorf("invalid API token")
	}
	if httpResp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied: API key lacks 'update services' permission")
	}
	if httpResp.StatusCode == 404 {
		return nil, fmt.Errorf("service not found: %s", id)
	}
	if httpResp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d", httpResp.StatusCode)
	}

	// Parse response
	var response struct {
		Data struct {
			ID         string `json:"id"`
			Attributes struct {
				Name               string  `json:"name"`
				Slug               string  `json:"slug"`
				Description        *string `json:"description"`
				Color              *string `json:"color"`
				EscalationPolicyID *string `json:"escalation_policy_id"`
				CreatedAt          string  `json:"created_at"`
				UpdatedAt          string  `json:"updated_at"`
			} `json:"attributes"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse update service response: %w", err)
	}

	service := &Service{
		ID:        response.Data.ID,
		Name:      response.Data.Attributes.Name,
		Slug:      response.Data.Attributes.Slug,
		CreatedAt: parseTime(response.Data.Attributes.CreatedAt),
		UpdatedAt: parseTime(response.Data.Attributes.UpdatedAt),
	}
	if response.Data.Attributes.Description != nil {
		service.Description = *response.Data.Attributes.Description
	}
	if response.Data.Attributes.Color != nil {
		service.Color = *response.Data.Attributes.Color
	}
	if response.Data.Attributes.EscalationPolicyID != nil {
		service.EscalationPolicyID = *response.Data.Attributes.EscalationPolicyID
	}

	service.RawBody = body
	return service, nil
}

// DeleteService deletes a service by ID.
func (c *Client) DeleteService(ctx context.Context, id string) error {
	// Build URL
	baseURL := c.endpoint

	url := fmt.Sprintf("%s/v1/services/%s", baseURL, id)

	req, err := http.NewRequestWithContext(ctx, "DELETE", url, http.NoBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	if httpResp.StatusCode == 401 {
		return fmt.Errorf("invalid API token")
	}
	if httpResp.StatusCode == 403 {
		return fmt.Errorf("access denied: API key lacks 'delete services' permission")
	}
	if httpResp.StatusCode == 404 {
		return fmt.Errorf("service not found: %s", id)
	}
	if httpResp.StatusCode != 204 && httpResp.StatusCode != 200 {
		return fmt.Errorf("API returned status %d", httpResp.StatusCode)
	}

	return nil
}

// ListTeamsCLI lists teams with pagination, sorting, and filters (stateless for CLI).
func (c *Client) ListTeamsCLI(ctx context.Context, page, pageSize int, sort string, filters map[string]string) (*TeamsResult, error) {
	// Cap pageSize at 100 (API limit); if 0, default to 25
	if pageSize == 0 {
		pageSize = 25
	}
	if pageSize > 100 {
		pageSize = 100
	}

	// Build URL with query parameters
	baseURL := c.endpoint

	url := fmt.Sprintf("%s/v1/teams?page[number]=%d&page[size]=%d", baseURL, page, pageSize)
	if sort != "" {
		url += fmt.Sprintf("&sort=%s", sort)
	}

	// Add filters (e.g., filter[name]=foo)
	for key, value := range filters {
		url += fmt.Sprintf("&filter[%s]=%s", key, neturl.QueryEscape(value))
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list teams: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode == 401 {
		return nil, fmt.Errorf("invalid API token")
	}
	if httpResp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied: API key lacks 'read teams' permission")
	}
	if httpResp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d", httpResp.StatusCode)
	}

	// Parse JSON:API response
	var response struct {
		Data []struct {
			ID         string `json:"id"`
			Attributes struct {
				Name        string  `json:"name"`
				Slug        string  `json:"slug"`
				Description *string `json:"description"`
				Color       *string `json:"color"`
				CreatedAt   string  `json:"created_at"`
				UpdatedAt   string  `json:"updated_at"`
			} `json:"attributes"`
		} `json:"data"`
		Meta struct {
			CurrentPage int `json:"current_page"`
			TotalPages  int `json:"total_pages"`
			TotalCount  int `json:"total_count"`
		} `json:"meta"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse teams: %w", err)
	}

	// Convert to Team structs
	teams := make([]Team, 0, len(response.Data))
	for _, item := range response.Data {
		team := Team{
			ID:   item.ID,
			Name: item.Attributes.Name,
			Slug: item.Attributes.Slug,
		}
		if item.Attributes.Description != nil {
			team.Description = *item.Attributes.Description
		}
		if item.Attributes.Color != nil {
			team.Color = *item.Attributes.Color
		}
		if item.Attributes.CreatedAt != "" {
			team.CreatedAt = parseTime(item.Attributes.CreatedAt)
		}
		if item.Attributes.UpdatedAt != "" {
			team.UpdatedAt = parseTime(item.Attributes.UpdatedAt)
		}

		teams = append(teams, team)
	}

	result := &TeamsResult{
		Teams: teams,
		Pagination: PaginationInfo{
			CurrentPage: response.Meta.CurrentPage,
			TotalPages:  response.Meta.TotalPages,
			TotalCount:  response.Meta.TotalCount,
		},
		RawBody: body,
	}

	return result, nil
}

// GetTeamByID fetches a single team by ID or slug (stateless for CLI).
func (c *Client) GetTeamByID(ctx context.Context, id string) (*Team, error) {
	// Build URL
	baseURL := c.endpoint

	url := fmt.Sprintf("%s/v1/teams/%s?include=users", baseURL, id)
	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch team: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode == 401 {
		return nil, fmt.Errorf("invalid API token")
	}
	if httpResp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied: API key lacks 'read teams' permission")
	}
	if httpResp.StatusCode == 404 {
		return nil, fmt.Errorf("team not found: %s", id)
	}
	if httpResp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d", httpResp.StatusCode)
	}

	// Parse JSON:API response with included relationships
	var response struct {
		Data struct {
			ID         string `json:"id"`
			Attributes struct {
				Name        string  `json:"name"`
				Slug        string  `json:"slug"`
				Description *string `json:"description"`
				Color       *string `json:"color"`
				CreatedAt   string  `json:"created_at"`
				UpdatedAt   string  `json:"updated_at"`
			} `json:"attributes"`
		} `json:"data"`
		Included []struct {
			Type       string `json:"type"`
			ID         string `json:"id"`
			Attributes struct {
				FullName string `json:"full_name"`
				Email    string `json:"email"`
			} `json:"attributes"`
		} `json:"included"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse team: %w", err)
	}

	team := &Team{
		ID:           response.Data.ID,
		Name:         response.Data.Attributes.Name,
		Slug:         response.Data.Attributes.Slug,
		DetailLoaded: true,
	}

	if response.Data.Attributes.Description != nil {
		team.Description = *response.Data.Attributes.Description
	}
	if response.Data.Attributes.Color != nil {
		team.Color = *response.Data.Attributes.Color
	}
	if response.Data.Attributes.CreatedAt != "" {
		team.CreatedAt = parseTime(response.Data.Attributes.CreatedAt)
	}
	if response.Data.Attributes.UpdatedAt != "" {
		team.UpdatedAt = parseTime(response.Data.Attributes.UpdatedAt)
	}

	// Parse included users relationship
	users := make([]string, 0)
	for _, included := range response.Included {
		if included.Type == "users" {
			userName := included.Attributes.FullName
			if userName == "" {
				userName = included.Attributes.Email
			}
			users = append(users, userName)
		}
	}
	team.Users = users
	team.RawBody = body

	return team, nil
}

// CreateTeam creates a new team.
func (c *Client) CreateTeam(ctx context.Context, name string, opts map[string]string) (*Team, error) {
	// Build JSON:API request body
	body := map[string]interface{}{
		"data": map[string]interface{}{
			"type": "teams",
			"attributes": map[string]interface{}{
				"name": name,
			},
		},
	}

	// Add optional attributes
	attrs := body["data"].(map[string]interface{})["attributes"].(map[string]interface{})
	if description, ok := opts["description"]; ok {
		attrs["description"] = description
	}
	if color, ok := opts["color"]; ok {
		attrs["color"] = color
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Build URL
	baseURL := c.endpoint

	url := fmt.Sprintf("%s/v1/teams", baseURL)

	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create team: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode == 401 {
		return nil, fmt.Errorf("invalid API token")
	}
	if httpResp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied: API key lacks 'create teams' permission")
	}
	if httpResp.StatusCode != 201 && httpResp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d", httpResp.StatusCode)
	}

	// Parse response
	var response struct {
		Data struct {
			ID         string `json:"id"`
			Attributes struct {
				Name        string  `json:"name"`
				Slug        string  `json:"slug"`
				Description *string `json:"description"`
				Color       *string `json:"color"`
				CreatedAt   string  `json:"created_at"`
				UpdatedAt   string  `json:"updated_at"`
			} `json:"attributes"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	team := &Team{
		ID:   response.Data.ID,
		Name: response.Data.Attributes.Name,
		Slug: response.Data.Attributes.Slug,
	}
	if response.Data.Attributes.Description != nil {
		team.Description = *response.Data.Attributes.Description
	}
	if response.Data.Attributes.Color != nil {
		team.Color = *response.Data.Attributes.Color
	}
	if response.Data.Attributes.CreatedAt != "" {
		team.CreatedAt = parseTime(response.Data.Attributes.CreatedAt)
	}
	if response.Data.Attributes.UpdatedAt != "" {
		team.UpdatedAt = parseTime(response.Data.Attributes.UpdatedAt)
	}

	team.RawBody = respBody
	return team, nil
}

// UpdateTeam updates an existing team.
func (c *Client) UpdateTeam(ctx context.Context, id string, opts map[string]string) (*Team, error) {
	// Build JSON:API request body
	body := map[string]interface{}{
		"data": map[string]interface{}{
			"type":       "teams",
			"id":         id,
			"attributes": opts,
		},
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Build URL
	baseURL := c.endpoint

	url := fmt.Sprintf("%s/v1/teams/%s", baseURL, id)

	req, err := http.NewRequestWithContext(ctx, "PATCH", url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to update team: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode == 401 {
		return nil, fmt.Errorf("invalid API token")
	}
	if httpResp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied: API key lacks 'update teams' permission")
	}
	if httpResp.StatusCode == 404 {
		return nil, fmt.Errorf("team not found: %s", id)
	}
	if httpResp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d", httpResp.StatusCode)
	}

	// Parse response
	var response struct {
		Data struct {
			ID         string `json:"id"`
			Attributes struct {
				Name        string  `json:"name"`
				Slug        string  `json:"slug"`
				Description *string `json:"description"`
				Color       *string `json:"color"`
				CreatedAt   string  `json:"created_at"`
				UpdatedAt   string  `json:"updated_at"`
			} `json:"attributes"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	team := &Team{
		ID:   response.Data.ID,
		Name: response.Data.Attributes.Name,
		Slug: response.Data.Attributes.Slug,
	}
	if response.Data.Attributes.Description != nil {
		team.Description = *response.Data.Attributes.Description
	}
	if response.Data.Attributes.Color != nil {
		team.Color = *response.Data.Attributes.Color
	}
	if response.Data.Attributes.CreatedAt != "" {
		team.CreatedAt = parseTime(response.Data.Attributes.CreatedAt)
	}
	if response.Data.Attributes.UpdatedAt != "" {
		team.UpdatedAt = parseTime(response.Data.Attributes.UpdatedAt)
	}

	team.RawBody = respBody
	return team, nil
}

// DeleteTeam deletes a team.
func (c *Client) DeleteTeam(ctx context.Context, id string) error {
	// Build URL
	baseURL := c.endpoint

	url := fmt.Sprintf("%s/v1/teams/%s", baseURL, id)

	req, err := http.NewRequestWithContext(ctx, "DELETE", url, http.NoBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete team: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	if httpResp.StatusCode == 401 {
		return fmt.Errorf("invalid API token")
	}
	if httpResp.StatusCode == 403 {
		return fmt.Errorf("access denied: API key lacks 'delete teams' permission")
	}
	if httpResp.StatusCode == 404 {
		return fmt.Errorf("team not found: %s", id)
	}
	if httpResp.StatusCode != 204 && httpResp.StatusCode != 200 {
		return fmt.Errorf("API returned status %d", httpResp.StatusCode)
	}

	return nil
}

// ============================================================================
// On-Call Schedules & Shifts (Read-Only)
// ============================================================================

// Schedule represents an on-call schedule
type Schedule struct {
	ID            string
	Name          string
	Description   string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	OwnerUserName string // Populated from included owner_user relationship
}

// SchedulesResult contains schedules and pagination info
type SchedulesResult struct {
	Schedules  []Schedule
	Pagination PaginationInfo
	RawBody    []byte
}

// OnCallEntry represents a single on-call entry from the /v1/oncalls endpoint.
type OnCallEntry struct {
	ID                   string
	EscalationPolicyID   string
	EscalationPolicyName string
	EscalationLevel      int
	ScheduleID           string
	ScheduleName         string
	UserID               string
	UserName             string
	UserEmail            string
	StartsAt             time.Time
	EndsAt               time.Time
}

// OnCallsResult contains on-call entries and raw body for JSON passthrough.
type OnCallsResult struct {
	Entries []OnCallEntry
	RawBody []byte
}

// OnCallsParams configures the /v1/oncalls request.
type OnCallsParams struct {
	Include             string // comma-separated: user, schedule, escalation_policy
	Since               string // ISO-8601 start time
	Until               string // ISO-8601 end time
	Earliest            bool   // only first on-call user per escalation level
	TimeZone            string // e.g. America/New_York
	ScheduleIDs         string // filter by schedule ID(s), comma-separated
	ServiceIDs          string // filter by service ID(s), comma-separated
	EscalationPolicyIDs string // filter by escalation policy ID(s), comma-separated
	UserIDs             string // filter by user ID(s), comma-separated
	GroupIDs            string // filter by group/team ID(s), comma-separated
}

// ListSchedulesCLI lists on-call schedules (read-only).
// Follows ListServicesCLI pattern for consistency.
func (c *Client) ListSchedulesCLI(ctx context.Context, page, pageSize int, filters map[string]string) (*SchedulesResult, error) {
	// Cap pageSize at 100 (API limit); if 0, default to 25
	if pageSize == 0 {
		pageSize = 25
	}
	if pageSize > 100 {
		pageSize = 100
	}

	// Build URL with query parameters
	baseURL := c.endpoint

	url := fmt.Sprintf("%s/v1/schedules?page[number]=%d&page[size]=%d", baseURL, page, pageSize)

	// Add filters (e.g., filter[name]=foo)
	for key, value := range filters {
		url += fmt.Sprintf("&filter[%s]=%s", key, neturl.QueryEscape(value))
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list schedules: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode == 401 {
		return nil, fmt.Errorf("invalid API token")
	}
	if httpResp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied: API key lacks 'read on-call schedules' permission")
	}
	if httpResp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d", httpResp.StatusCode)
	}

	// Parse JSON:API response
	var response struct {
		Data []struct {
			ID         string `json:"id"`
			Attributes struct {
				Name        string  `json:"name"`
				Description *string `json:"description"`
				CreatedAt   string  `json:"created_at"`
				UpdatedAt   string  `json:"updated_at"`
			} `json:"attributes"`
		} `json:"data"`
		Meta struct {
			CurrentPage int `json:"current_page"`
			TotalPages  int `json:"total_pages"`
			TotalCount  int `json:"total_count"`
		} `json:"meta"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	schedules := make([]Schedule, 0, len(response.Data))
	for _, item := range response.Data {
		schedule := Schedule{
			ID:        item.ID,
			Name:      item.Attributes.Name,
			CreatedAt: parseTime(item.Attributes.CreatedAt),
			UpdatedAt: parseTime(item.Attributes.UpdatedAt),
		}
		if item.Attributes.Description != nil {
			schedule.Description = *item.Attributes.Description
		}
		schedules = append(schedules, schedule)
	}

	result := &SchedulesResult{
		Schedules: schedules,
		Pagination: PaginationInfo{
			CurrentPage: response.Meta.CurrentPage,
			TotalPages:  response.Meta.TotalPages,
			TotalCount:  response.Meta.TotalCount,
		},
		RawBody: body,
	}

	return result, nil
}

// ListOnCallsCLI lists on-call entries using the unified /v1/oncalls endpoint.
func (c *Client) ListOnCallsCLI(ctx context.Context, params OnCallsParams) (*OnCallsResult, error) {
	baseURL := c.endpoint

	url := fmt.Sprintf("%s/v1/oncalls?", baseURL)

	qp := make([]string, 0)
	if params.Include != "" {
		qp = append(qp, "include="+params.Include)
	}
	if params.Since != "" {
		qp = append(qp, "since="+params.Since)
	}
	if params.Until != "" {
		qp = append(qp, "until="+params.Until)
	}
	if params.Earliest {
		qp = append(qp, "earliest=true")
	}
	if params.TimeZone != "" {
		qp = append(qp, "time_zone="+params.TimeZone)
	}
	if params.ScheduleIDs != "" {
		qp = append(qp, "filter[schedule_ids]="+neturl.QueryEscape(params.ScheduleIDs))
	}
	if params.ServiceIDs != "" {
		qp = append(qp, "filter[service_ids]="+neturl.QueryEscape(params.ServiceIDs))
	}
	if params.EscalationPolicyIDs != "" {
		qp = append(qp, "filter[escalation_policy_ids]="+neturl.QueryEscape(params.EscalationPolicyIDs))
	}
	if params.UserIDs != "" {
		qp = append(qp, "filter[user_ids]="+neturl.QueryEscape(params.UserIDs))
	}
	if params.GroupIDs != "" {
		qp = append(qp, "filter[group_ids]="+neturl.QueryEscape(params.GroupIDs))
	}

	url += strings.Join(qp, "&")

	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list on-calls: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode == 401 {
		return nil, fmt.Errorf("invalid API token")
	}
	if httpResp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied: API key lacks 'read on-calls' permission")
	}
	if httpResp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d", httpResp.StatusCode)
	}

	// Parse JSON:API response
	var response struct {
		Data []struct {
			ID         string `json:"id"`
			Attributes struct {
				UserID               interface{} `json:"user_id"`
				ScheduleID           interface{} `json:"schedule_id"`
				ScheduleName         string      `json:"schedule_name"`
				EscalationPolicyID   interface{} `json:"escalation_policy_id"`
				EscalationPolicyName string      `json:"escalation_policy_name"`
				EscalationLevel      int         `json:"escalation_level"`
				StartsAt             string      `json:"starts_at"`
				EndsAt               string      `json:"ends_at"`
			} `json:"attributes"`
		} `json:"data"`
		Included []struct {
			ID         interface{} `json:"id"`
			Type       string      `json:"type"`
			Attributes struct {
				FullName string  `json:"full_name"`
				Name     string  `json:"name"`
				Email    *string `json:"email"`
			} `json:"attributes"`
		} `json:"included"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Build user lookup from included
	userMap := make(map[string]struct {
		Name  string
		Email string
	})
	for _, inc := range response.Included {
		if inc.Type != "users" {
			continue
		}
		name := inc.Attributes.FullName
		if name == "" {
			name = inc.Attributes.Name
		}
		email := ""
		if inc.Attributes.Email != nil {
			email = *inc.Attributes.Email
		}
		userMap[toStr(inc.ID)] = struct {
			Name  string
			Email string
		}{Name: name, Email: email}
	}

	entries := make([]OnCallEntry, 0, len(response.Data))
	for _, item := range response.Data {
		entry := OnCallEntry{
			ID:                   item.ID,
			EscalationPolicyID:   toStr(item.Attributes.EscalationPolicyID),
			EscalationPolicyName: item.Attributes.EscalationPolicyName,
			EscalationLevel:      item.Attributes.EscalationLevel,
			ScheduleID:           toStr(item.Attributes.ScheduleID),
			ScheduleName:         item.Attributes.ScheduleName,
			UserID:               toStr(item.Attributes.UserID),
			StartsAt:             parseTime(item.Attributes.StartsAt),
			EndsAt:               parseTime(item.Attributes.EndsAt),
		}

		if user, ok := userMap[toStr(item.Attributes.UserID)]; ok {
			entry.UserName = user.Name
			entry.UserEmail = user.Email
		}

		entries = append(entries, entry)
	}

	return &OnCallsResult{
		Entries: entries,
		RawBody: body,
	}, nil
}

// ResolveScheduleIDByName looks up a schedule by name and returns its ID.
// Uses filter[name] on the schedules endpoint for an exact match.
func (c *Client) ResolveScheduleIDByName(ctx context.Context, name string) (string, error) {
	result, err := c.ListSchedulesCLI(ctx, 1, 25, map[string]string{"name": name})
	if err != nil {
		return "", fmt.Errorf("failed to look up schedule %q: %w", name, err)
	}
	if len(result.Schedules) == 0 {
		return "", fmt.Errorf("no schedule found with name %q", name)
	}
	if len(result.Schedules) > 1 {
		return "", fmt.Errorf("multiple schedules match name %q; use --schedule-id to specify", name)
	}
	return result.Schedules[0].ID, nil
}

// ResolveServiceIDByName looks up a service by name and returns its ID.
func (c *Client) ResolveServiceIDByName(ctx context.Context, name string) (string, error) {
	result, err := c.ListServicesCLI(ctx, 1, 25, "", map[string]string{"name": name})
	if err != nil {
		return "", fmt.Errorf("failed to look up service %q: %w", name, err)
	}
	if len(result.Services) == 0 {
		return "", fmt.Errorf("no service found with name %q", name)
	}
	if len(result.Services) > 1 {
		return "", fmt.Errorf("multiple services match name %q; use --service-id to specify", name)
	}
	return result.Services[0].ID, nil
}

// ResolveTeamIDByName looks up a team (group) by name and returns its ID.
func (c *Client) ResolveTeamIDByName(ctx context.Context, name string) (string, error) {
	result, err := c.ListTeamsCLI(ctx, 1, 25, "", map[string]string{"name": name})
	if err != nil {
		return "", fmt.Errorf("failed to look up team %q: %w", name, err)
	}
	if len(result.Teams) == 0 {
		return "", fmt.Errorf("no team found with name %q", name)
	}
	if len(result.Teams) > 1 {
		return "", fmt.Errorf("multiple teams match name %q; use --team-id to specify", name)
	}
	return result.Teams[0].ID, nil
}

// ResolveUserID looks up a user by email or name and returns their ID.
// Tries filter[email] first (exact); falls back to filter[search] (fuzzy).
func (c *Client) ResolveUserID(ctx context.Context, query string) (string, error) {
	baseURL := c.endpoint
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "https://" + baseURL
	}

	filterKey := "search"
	if strings.Contains(query, "@") {
		filterKey = "email"
	}

	url := fmt.Sprintf("%s/v1/users?page[size]=25&filter[%s]=%s", baseURL, filterKey, neturl.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to look up user %q: %w", query, err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode != 200 {
		return "", fmt.Errorf("user lookup returned status %d", httpResp.StatusCode)
	}

	var response struct {
		Data []struct {
			ID         string `json:"id"`
			Attributes struct {
				Email    string  `json:"email"`
				FullName *string `json:"full_name"`
			} `json:"attributes"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if len(response.Data) == 0 {
		return "", fmt.Errorf("no user found matching %q", query)
	}
	if len(response.Data) > 1 && filterKey == "search" {
		return "", fmt.Errorf("multiple users match %q; use --user-id or an email address to be more specific", query)
	}
	return response.Data[0].ID, nil
}

// CreatePulseCLI creates a new pulse using raw HTTP POST.
func (c *Client) CreatePulseCLI(ctx context.Context, summary string, opts PulseOpts) (*Pulse, error) {
	// Build JSON:API request body
	attributes := map[string]interface{}{
		"summary": summary,
	}

	if opts.Source != "" {
		attributes["source"] = opts.Source
	}
	if len(opts.ServiceIDs) > 0 {
		attributes["service_ids"] = opts.ServiceIDs
	}
	if len(opts.EnvironmentIDs) > 0 {
		attributes["environment_ids"] = opts.EnvironmentIDs
	}
	if len(opts.Labels) > 0 {
		labels := make(map[string]string, len(opts.Labels))
		for _, kv := range opts.Labels {
			labels[kv.Key] = kv.Value
		}
		attributes["labels"] = labels
	}
	if len(opts.Refs) > 0 {
		refs := make([]map[string]string, 0, len(opts.Refs))
		for _, kv := range opts.Refs {
			refs = append(refs, map[string]string{"name": kv.Key, "value": kv.Value})
		}
		attributes["refs"] = refs
	}
	if opts.StartedAt != nil {
		attributes["started_at"] = opts.StartedAt.Format(time.RFC3339)
	}
	if opts.EndedAt != nil {
		attributes["ended_at"] = opts.EndedAt.Format(time.RFC3339)
	}

	requestBody := map[string]interface{}{
		"data": map[string]interface{}{
			"type":       "pulses",
			"attributes": attributes,
		},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Build URL
	baseURL := c.endpoint

	url := fmt.Sprintf("%s/v1/pulses", baseURL)

	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create pulse: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode == 401 {
		return nil, fmt.Errorf("invalid API key")
	}
	if httpResp.StatusCode == 403 {
		return nil, fmt.Errorf("access denied: API key lacks 'create pulses' permission")
	}
	if httpResp.StatusCode != 201 && httpResp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d", httpResp.StatusCode)
	}

	// Parse response
	var resp struct {
		Data struct {
			ID         string `json:"id"`
			Attributes struct {
				Summary   string  `json:"summary"`
				Source    string  `json:"source"`
				StartedAt *string `json:"started_at"`
				EndedAt   *string `json:"ended_at"`
				Services  *struct {
					Data []struct {
						Attributes struct {
							Name string `json:"name"`
						} `json:"attributes"`
					} `json:"data"`
				} `json:"services"`
				Environments *struct {
					Data []struct {
						Attributes struct {
							Name string `json:"name"`
						} `json:"attributes"`
					} `json:"data"`
				} `json:"environments"`
			} `json:"attributes"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	pulse := &Pulse{
		ID:      resp.Data.ID,
		Summary: resp.Data.Attributes.Summary,
		Source:  resp.Data.Attributes.Source,
		RawBody: body,
	}

	pulse.StartedAt = parseTimePtr(resp.Data.Attributes.StartedAt)
	pulse.EndedAt = parseTimePtr(resp.Data.Attributes.EndedAt)

	if resp.Data.Attributes.Services != nil {
		for _, s := range resp.Data.Attributes.Services.Data {
			pulse.Services = append(pulse.Services, s.Attributes.Name)
		}
	}
	if resp.Data.Attributes.Environments != nil {
		for _, e := range resp.Data.Attributes.Environments.Data {
			pulse.Environments = append(pulse.Environments, e.Attributes.Name)
		}
	}

	return pulse, nil
}
