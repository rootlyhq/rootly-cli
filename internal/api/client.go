package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	rootly "github.com/rootlyhq/rootly-go"

	"github.com/rootlyhq/rootly-tui/internal/config"
	"github.com/rootlyhq/rootly-tui/internal/debug"
)

// DefaultCacheTTL is the default cache duration
const DefaultCacheTTL = 5 * time.Minute

// Version is set by the main package to include in User-Agent
var Version = "dev"

type Client struct {
	client   *rootly.ClientWithResponses
	endpoint string
	apiKey   string
	cache    *PersistentCache
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
}

// AlertsResult contains alerts and pagination info
type AlertsResult struct {
	Alerts     []Alert
	Pagination PaginationInfo
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

func NewClient(cfg *config.Config) (*Client, error) {
	endpoint := cfg.Endpoint
	if endpoint != "" && !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}

	debug.Logger.Debug("Creating API client", "endpoint", endpoint)

	client, err := rootly.NewClientWithResponses(endpoint,
		rootly.WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
			req.Header.Set("Content-Type", "application/vnd.api+json")
			req.Header.Set("User-Agent", "rootly-tui/"+Version)
			debug.Logger.Debug("API request",
				"method", req.Method,
				"url", req.URL.String(),
			)
			return nil
		}),
	)
	if err != nil {
		debug.Logger.Error("Failed to create client", "error", err)
		return nil, fmt.Errorf("failed to create rootly client: %w", err)
	}

	cache, err := NewPersistentCache(DefaultCacheTTL)
	if err != nil {
		debug.Logger.Warn("Failed to create persistent cache, using in-memory", "error", err)
		// Fall back to in-memory cache
		return &Client{
			client:   client,
			endpoint: cfg.Endpoint,
			apiKey:   cfg.APIKey,
			cache:    nil,
		}, nil
	}

	return &Client{
		client:   client,
		endpoint: cfg.Endpoint,
		apiKey:   cfg.APIKey,
		cache:    cache,
	}, nil
}

// ClearCache clears all cached data
func (c *Client) ClearCache() {
	if c.cache != nil {
		c.cache.Clear()
		debug.Logger.Debug("Cache cleared")
	}
}

// Close closes the client and releases resources
func (c *Client) Close() error {
	if c.cache != nil {
		return c.cache.Close()
	}
	return nil
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

func (c *Client) ListIncidents(ctx context.Context, page int, sort string) (*IncidentsResult, error) {
	pageSize := 25

	// Build cache key with parameters including sort
	cacheKeyBuilder := NewCacheKey(CacheKeyPrefixIncidents).
		With("page", page).
		With("pageSize", pageSize)
	if sort != "" {
		cacheKeyBuilder = cacheKeyBuilder.With("sort", sort)
	}
	cacheKey := cacheKeyBuilder.Build()

	// Check cache first
	if c.cache != nil {
		var cached IncidentsResult
		if c.cache.GetTyped(cacheKey, &cached) {
			debug.Logger.Debug("Cache hit for incidents", "key", cacheKey)
			return &cached, nil
		}
	}

	// Build URL with query parameters
	baseURL := c.endpoint
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "https://" + baseURL
	}

	url := fmt.Sprintf("%s/v1/incidents?page[number]=%d&page[size]=%d", baseURL, page, pageSize)
	if sort != "" {
		url += fmt.Sprintf("&sort=%s", sort)
	}

	debug.Logger.Debug("Fetching incidents", "page", page, "pageSize", pageSize, "sort", sort, "cache", "miss", "key", cacheKey)

	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		debug.Logger.Error("Failed to list incidents", "error", err)
		return nil, fmt.Errorf("failed to list incidents: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	debug.Logger.Debug("Incidents response",
		"status", httpResp.StatusCode,
		"bodyLength", len(body),
	)
	debug.Logger.Debug("Incidents response body", "json", debug.PrettyJSON(body))

	if httpResp.StatusCode == 403 {
		debug.Logger.Error("API forbidden", "status", httpResp.StatusCode)
		return nil, fmt.Errorf("access denied: API key lacks 'read incidents' permission")
	}
	if httpResp.StatusCode != 200 {
		debug.Logger.Error("API error", "status", httpResp.StatusCode, "body", debug.PrettyJSON(body))
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
		debug.Logger.Error("Failed to parse incidents response",
			"error", err,
			"body", debug.PrettyJSON(body),
		)
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	debug.Logger.Debug("Parsed incidents", "count", len(result.Data))

	incidents := make([]Incident, 0, len(result.Data))
	for _, d := range result.Data {
		incidents = append(incidents, parseIncidentData(d))
	}

	// Build result with pagination info from Meta
	// Fall back to Links if Meta values are zero (API might not return meta)
	hasNext := result.Meta.NextPage != nil && *result.Meta.NextPage > 0
	if !hasNext && result.Links.Next != nil && *result.Links.Next != "" {
		hasNext = true
	}
	hasPrev := result.Meta.PrevPage != nil && *result.Meta.PrevPage > 0
	if !hasPrev && result.Links.Prev != nil && *result.Links.Prev != "" {
		hasPrev = true
	}

	// Use Meta values, fall back to page parameter if Meta.CurrentPage is 0
	currentPage := result.Meta.CurrentPage
	if currentPage == 0 {
		currentPage = page
	}

	incidentsResult := &IncidentsResult{
		Incidents: incidents,
		Pagination: PaginationInfo{
			CurrentPage: currentPage,
			TotalPages:  result.Meta.TotalPages,
			TotalCount:  result.Meta.TotalCount,
			HasNext:     hasNext,
			HasPrev:     hasPrev,
		},
	}

	// Store in cache
	if c.cache != nil {
		c.cache.Set(cacheKey, incidentsResult)
		debug.Logger.Debug("Cached incidents", "count", len(incidents), "key", cacheKey)
	}

	return incidentsResult, nil
}

func (c *Client) ListAlerts(ctx context.Context, page int) (*AlertsResult, error) {
	pageSize := 25

	// Build cache key with parameters
	cacheKey := NewCacheKey(CacheKeyPrefixAlerts).
		With("page", page).
		With("pageSize", pageSize).
		Build()

	// Check cache first
	if c.cache != nil {
		var cached AlertsResult
		if c.cache.GetTyped(cacheKey, &cached) {
			debug.Logger.Debug("Cache hit for alerts", "key", cacheKey)
			return &cached, nil
		}
	}

	params := &rootly.ListAlertsParams{
		PageNumber: &page,
		PageSize:   &pageSize,
	}

	debug.Logger.Debug("Fetching alerts", "pageSize", pageSize, "cache", "miss", "key", cacheKey)

	resp, err := c.client.ListAlertsWithResponse(ctx, params)
	if err != nil {
		debug.Logger.Error("Failed to list alerts", "error", err)
		return nil, fmt.Errorf("failed to list alerts: %w", err)
	}

	debug.Logger.Debug("Alerts response",
		"status", resp.StatusCode(),
		"bodyLength", len(resp.Body),
	)
	debug.Logger.Debug("Alerts response body", "json", debug.PrettyJSON(resp.Body))

	if resp.StatusCode() == 403 {
		debug.Logger.Error("API forbidden", "status", resp.StatusCode())
		return nil, fmt.Errorf("access denied: API key lacks 'read alerts' permission")
	}
	if resp.StatusCode() != 200 {
		debug.Logger.Error("API error", "status", resp.StatusCode(), "body", debug.PrettyJSON(resp.Body))
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode())
	}

	if resp.ApplicationvndApiJSON200 == nil {
		debug.Logger.Error("Failed to parse alerts response", "body", debug.PrettyJSON(resp.Body))
		return nil, fmt.Errorf("failed to parse response")
	}

	result := resp.ApplicationvndApiJSON200
	debug.Logger.Debug("Parsed alerts", "count", len(result.Data))

	alerts := make([]Alert, 0, len(result.Data))
	for _, d := range result.Data {
		alert := Alert{
			ID:      d.Id,
			ShortID: strings.TrimSpace(d.Attributes.ShortId),
			Summary: strings.TrimSpace(d.Attributes.Summary),
			Source:  string(d.Attributes.Source),
			Labels:  make(map[string]string),
		}

		if d.Attributes.Status != nil {
			alert.Status = strings.TrimSpace(string(*d.Attributes.Status))
		}

		if desc, err := d.Attributes.Description.Get(); err == nil {
			alert.Description = strings.TrimSpace(desc)
		}
		if extURL, err := d.Attributes.ExternalUrl.Get(); err == nil {
			alert.ExternalURL = extURL
		}

		if t, err := time.Parse(time.RFC3339, d.Attributes.CreatedAt); err == nil {
			alert.CreatedAt = t
		}
		if t, err := time.Parse(time.RFC3339, d.Attributes.UpdatedAt); err == nil {
			alert.UpdatedAt = t
		}
		if startedAt, err := d.Attributes.StartedAt.Get(); err == nil {
			alert.StartedAt = &startedAt
		}
		if endedAt, err := d.Attributes.EndedAt.Get(); err == nil {
			alert.EndedAt = &endedAt
		}

		if d.Attributes.Services != nil {
			for _, s := range *d.Attributes.Services {
				alert.Services = append(alert.Services, s.Name)
			}
		}
		if d.Attributes.Environments != nil {
			for _, e := range *d.Attributes.Environments {
				alert.Environments = append(alert.Environments, e.Name)
			}
		}
		if d.Attributes.Groups != nil {
			for _, g := range *d.Attributes.Groups {
				alert.Groups = append(alert.Groups, g.Name)
			}
		}
		if d.Attributes.Labels != nil {
			for _, l := range *d.Attributes.Labels {
				alert.Labels[l.Key] = alertLabelValueToString(l.Value)
			}
		}

		if data, err := d.Attributes.Data.Get(); err == nil {
			alert.Data = data
		}

		alerts = append(alerts, alert)
	}

	// Build result with pagination info from Meta
	// Fall back to Links if Meta values are zero (API might not return meta)
	hasNext := false
	hasPrev := false
	if nextPage, err := result.Meta.NextPage.Get(); err == nil && nextPage > 0 {
		hasNext = true
	} else if next, err := result.Links.Next.Get(); err == nil && next != "" {
		hasNext = true
	}
	if prevPage, err := result.Meta.PrevPage.Get(); err == nil && prevPage > 0 {
		hasPrev = true
	} else if prev, err := result.Links.Prev.Get(); err == nil && prev != "" {
		hasPrev = true
	}

	// Use Meta values, fall back to page parameter if Meta.CurrentPage is 0
	currentPage := result.Meta.CurrentPage
	if currentPage == 0 {
		currentPage = page
	}

	alertsResult := &AlertsResult{
		Alerts: alerts,
		Pagination: PaginationInfo{
			CurrentPage: currentPage,
			TotalPages:  result.Meta.TotalPages,
			TotalCount:  result.Meta.TotalCount,
			HasNext:     hasNext,
			HasPrev:     hasPrev,
		},
	}

	// Store in cache
	if c.cache != nil {
		c.cache.Set(cacheKey, alertsResult)
		debug.Logger.Debug("Cached alerts", "count", len(alerts), "key", cacheKey)
	}

	return alertsResult, nil
}

// alertLabelValueToString converts the SDK's union type to a string
func alertLabelValueToString(v rootly.Alert_Labels_Value) string {
	if s, err := v.AsAlertLabelsValue0(); err == nil {
		return s
	}
	if f, err := v.AsAlertLabelsValue1(); err == nil {
		return fmt.Sprintf("%v", f)
	}
	if b, err := v.AsAlertLabelsValue2(); err == nil {
		return fmt.Sprintf("%v", b)
	}
	return ""
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

// GetIncident fetches detailed incident data by ID
// updatedAt is used for cache invalidation - cache key includes it so changes invalidate the cache
//
//nolint:gocyclo // complexity from parsing deeply nested API response with many optional fields
func (c *Client) GetIncident(ctx context.Context, id string, updatedAt time.Time) (*Incident, error) {
	// Build cache key with updated_at for smart invalidation
	cacheKey := NewCacheKey(CacheKeyPrefixIncidentDetail).
		With("id", id).
		With("updated_at", updatedAt.UTC().Format(time.RFC3339)).
		Build()

	// Check cache first
	if c.cache != nil {
		var cached Incident
		if c.cache.GetTyped(cacheKey, &cached) {
			debug.Logger.Debug("Cache hit for incident detail", "key", cacheKey)
			return &cached, nil
		}
	}

	debug.Logger.Debug("Fetching incident detail", "id", id, "cache", "miss")

	// Build URL - endpoint may already have scheme
	baseURL := c.endpoint
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "https://" + baseURL
	}
	url := fmt.Sprintf("%s/v1/incidents/%s?include=roles,causes,incident_types,functionalities,services,environments,groups,user", baseURL, id)
	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		debug.Logger.Error("Failed to fetch incident", "error", err)
		return nil, fmt.Errorf("failed to fetch incident: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	debug.Logger.Debug("Incident detail response",
		"status", httpResp.StatusCode,
		"bodyLength", len(body),
	)
	debug.Logger.Debug("Incident detail response body", "json", debug.PrettyJSON(body))

	if httpResp.StatusCode == 403 {
		debug.Logger.Error("API forbidden", "status", httpResp.StatusCode)
		return nil, fmt.Errorf("access denied: API key lacks 'read incidents' permission")
	}
	if httpResp.StatusCode != 200 {
		debug.Logger.Error("API error", "status", httpResp.StatusCode, "body", debug.PrettyJSON(body))
		return nil, fmt.Errorf("API returned status %d", httpResp.StatusCode)
	}

	var result struct {
		Data struct {
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
				URL             *string `json:"url"`
				ShortURL        *string `json:"short_url"`
				CreatedAt       string  `json:"created_at"`
				UpdatedAt       string  `json:"updated_at"`
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
				Causes *struct {
					Data []struct {
						Attributes struct {
							Name string `json:"name"`
						} `json:"attributes"`
					} `json:"data"`
				} `json:"causes"`
				IncidentTypes *struct {
					Data []struct {
						Attributes struct {
							Name string `json:"name"`
						} `json:"attributes"`
					} `json:"data"`
				} `json:"incident_types"`
				Functionalities *struct {
					Data []struct {
						Attributes struct {
							Name string `json:"name"`
						} `json:"attributes"`
					} `json:"data"`
				} `json:"functionalities"`
				// User who created the incident
				User *struct {
					Data *struct {
						Attributes struct {
							Name  string `json:"name"`
							Email string `json:"email"`
						} `json:"attributes"`
					} `json:"data"`
				} `json:"user"`
				// Additional fields
				Source                      *string        `json:"source"`
				Private                     bool           `json:"private"`
				MitigationMessage           *string        `json:"mitigation_message"`
				ResolutionMessage           *string        `json:"resolution_message"`
				RetrospectiveProgressStatus *string        `json:"retrospective_progress_status"`
				SlackChannelName            *string        `json:"slack_channel_name"`
				SlackChannelArchived        bool           `json:"slack_channel_archived"`
				Labels                      map[string]any `json:"labels"`
				// Who performed actions
				StartedBy *struct {
					Data *struct {
						Attributes struct {
							Name  string `json:"name"`
							Email string `json:"email"`
						} `json:"attributes"`
					} `json:"data"`
				} `json:"started_by"`
				MitigatedBy *struct {
					Data *struct {
						Attributes struct {
							Name  string `json:"name"`
							Email string `json:"email"`
						} `json:"attributes"`
					} `json:"data"`
				} `json:"mitigated_by"`
				ResolvedBy *struct {
					Data *struct {
						Attributes struct {
							Name  string `json:"name"`
							Email string `json:"email"`
						} `json:"attributes"`
					} `json:"data"`
				} `json:"resolved_by"`
				// Integration links
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
			} `json:"attributes"`
		} `json:"data"`
		Included []struct {
			ID         string `json:"id"`
			Type       string `json:"type"`
			Attributes struct {
				// For groups, services, environments
				Name string `json:"name"`
				// For incident_role_assignments
				IncidentRole *struct {
					Data *struct {
						Attributes struct {
							Name string `json:"name"`
						} `json:"attributes"`
					} `json:"data"`
				} `json:"incident_role"`
				User *struct {
					Data *struct {
						Attributes struct {
							Name  string `json:"name"`
							Email string `json:"email"`
						} `json:"attributes"`
					} `json:"data"`
				} `json:"user"`
			} `json:"attributes"`
		} `json:"included"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		debug.Logger.Error("Failed to parse incident detail response",
			"error", err,
			"body", debug.PrettyJSON(body),
		)
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	d := result.Data
	incident := &Incident{
		ID:           d.ID,
		Title:        strings.TrimSpace(d.Attributes.Title),
		Summary:      strings.TrimSpace(d.Attributes.Summary),
		Status:       strings.TrimSpace(d.Attributes.Status),
		Kind:         d.Attributes.Kind,
		DetailLoaded: true,
	}

	if d.Attributes.SequentialID != nil {
		incident.SequentialID = fmt.Sprintf("INC-%d", *d.Attributes.SequentialID)
	}

	if d.Attributes.Severity != nil && d.Attributes.Severity.Data != nil && d.Attributes.Severity.Data.Attributes != nil {
		incident.Severity = d.Attributes.Severity.Data.Attributes.Name
	}

	if d.Attributes.URL != nil {
		incident.URL = *d.Attributes.URL
	}
	if d.Attributes.ShortURL != nil {
		incident.ShortURL = *d.Attributes.ShortURL
	}

	// Parse creator (user who created the incident)
	if d.Attributes.User != nil && d.Attributes.User.Data != nil {
		incident.CreatedByName = d.Attributes.User.Data.Attributes.Name
		incident.CreatedByEmail = d.Attributes.User.Data.Attributes.Email
	}

	if t, err := time.Parse(time.RFC3339, d.Attributes.CreatedAt); err == nil {
		incident.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, d.Attributes.UpdatedAt); err == nil {
		incident.UpdatedAt = t
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

	// Parse additional fields
	if d.Attributes.Source != nil {
		incident.Source = *d.Attributes.Source
	}
	incident.Private = d.Attributes.Private
	if d.Attributes.MitigationMessage != nil {
		incident.MitigationMessage = strings.TrimSpace(*d.Attributes.MitigationMessage)
	}
	if d.Attributes.ResolutionMessage != nil {
		incident.ResolutionMessage = strings.TrimSpace(*d.Attributes.ResolutionMessage)
	}
	if d.Attributes.RetrospectiveProgressStatus != nil {
		incident.RetrospectiveProgressStatus = *d.Attributes.RetrospectiveProgressStatus
	}
	if d.Attributes.SlackChannelName != nil {
		incident.SlackChannelName = *d.Attributes.SlackChannelName
	}
	incident.SlackChannelArchived = d.Attributes.SlackChannelArchived

	// Parse labels
	if d.Attributes.Labels != nil {
		incident.Labels = make(map[string]string)
		for k, v := range d.Attributes.Labels {
			incident.Labels[k] = fmt.Sprintf("%v", v)
		}
	}

	// Parse who performed actions
	if d.Attributes.StartedBy != nil && d.Attributes.StartedBy.Data != nil {
		incident.StartedByName = strings.TrimSpace(d.Attributes.StartedBy.Data.Attributes.Name)
		incident.StartedByEmail = strings.TrimSpace(d.Attributes.StartedBy.Data.Attributes.Email)
	}
	if d.Attributes.MitigatedBy != nil && d.Attributes.MitigatedBy.Data != nil {
		incident.MitigatedByName = strings.TrimSpace(d.Attributes.MitigatedBy.Data.Attributes.Name)
		incident.MitigatedByEmail = strings.TrimSpace(d.Attributes.MitigatedBy.Data.Attributes.Email)
	}
	if d.Attributes.ResolvedBy != nil && d.Attributes.ResolvedBy.Data != nil {
		incident.ResolvedByName = strings.TrimSpace(d.Attributes.ResolvedBy.Data.Attributes.Name)
		incident.ResolvedByEmail = strings.TrimSpace(d.Attributes.ResolvedBy.Data.Attributes.Email)
	}

	// Parse integration links
	if d.Attributes.GoogleMeetingURL != nil {
		incident.GoogleMeetingURL = *d.Attributes.GoogleMeetingURL
	}
	if d.Attributes.LinearIssueURL != nil {
		incident.LinearIssueURL = *d.Attributes.LinearIssueURL
	}
	if d.Attributes.ZoomMeetingJoinURL != nil {
		incident.ZoomMeetingJoinURL = *d.Attributes.ZoomMeetingJoinURL
	}
	if d.Attributes.GithubIssueURL != nil {
		incident.GithubIssueURL = *d.Attributes.GithubIssueURL
	}
	if d.Attributes.GitlabIssueURL != nil {
		incident.GitlabIssueURL = *d.Attributes.GitlabIssueURL
	}
	if d.Attributes.PagerdutyIncidentURL != nil {
		incident.PagerdutyIncidentURL = *d.Attributes.PagerdutyIncidentURL
	}
	if d.Attributes.OpsgenieIncidentURL != nil {
		incident.OpsgenieIncidentURL = *d.Attributes.OpsgenieIncidentURL
	}
	if d.Attributes.AsanaTaskURL != nil {
		incident.AsanaTaskURL = *d.Attributes.AsanaTaskURL
	}
	if d.Attributes.TrelloCardURL != nil {
		incident.TrelloCardURL = *d.Attributes.TrelloCardURL
	}
	if d.Attributes.ConfluencePageURL != nil {
		incident.ConfluencePageURL = *d.Attributes.ConfluencePageURL
	}
	if d.Attributes.DatadogNotebookURL != nil {
		incident.DatadogNotebookURL = *d.Attributes.DatadogNotebookURL
	}
	if d.Attributes.ServiceNowIncidentURL != nil {
		incident.ServiceNowIncidentURL = *d.Attributes.ServiceNowIncidentURL
	}
	if d.Attributes.FreshserviceTicketURL != nil {
		incident.FreshserviceTicketURL = *d.Attributes.FreshserviceTicketURL
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
	// Parse included array (JSON:API format) for roles, groups, services, environments
	for _, inc := range result.Included {
		switch inc.Type {
		case "incident_role_assignments":
			var roleName, userName, userEmail string
			if inc.Attributes.IncidentRole != nil && inc.Attributes.IncidentRole.Data != nil {
				roleName = inc.Attributes.IncidentRole.Data.Attributes.Name
			}
			if inc.Attributes.User != nil && inc.Attributes.User.Data != nil {
				userName = inc.Attributes.User.Data.Attributes.Name
				userEmail = inc.Attributes.User.Data.Attributes.Email
			}
			if roleName != "" {
				role := IncidentRole{Name: roleName, UserName: userName, UserEmail: userEmail}
				incident.Roles = append(incident.Roles, role)
				// Extract commander and communicator
				if strings.EqualFold(roleName, "commander") && userName != "" {
					incident.CommanderName = userName
				}
				if strings.Contains(strings.ToLower(roleName), "communications") && userName != "" {
					incident.CommunicatorName = userName
				}
			}
		case "groups":
			if inc.Attributes.Name != "" {
				incident.Teams = append(incident.Teams, inc.Attributes.Name)
			}
		case "services":
			if inc.Attributes.Name != "" {
				incident.Services = append(incident.Services, inc.Attributes.Name)
			}
		case "environments":
			if inc.Attributes.Name != "" {
				incident.Environments = append(incident.Environments, inc.Attributes.Name)
			}
		}
	}
	if d.Attributes.Causes != nil {
		for _, cause := range d.Attributes.Causes.Data {
			incident.Causes = append(incident.Causes, cause.Attributes.Name)
		}
	}
	if d.Attributes.IncidentTypes != nil {
		for _, it := range d.Attributes.IncidentTypes.Data {
			incident.IncidentTypes = append(incident.IncidentTypes, it.Attributes.Name)
		}
	}
	if d.Attributes.Functionalities != nil {
		for _, f := range d.Attributes.Functionalities.Data {
			incident.Functionalities = append(incident.Functionalities, f.Attributes.Name)
		}
	}

	// Store in cache
	if c.cache != nil {
		c.cache.Set(cacheKey, incident)
		debug.Logger.Debug("Cached incident detail", "id", incident.ID, "key", cacheKey)
	}

	debug.Logger.Debug("Parsed incident detail", "id", incident.ID, "title", incident.Title)
	return incident, nil
}

// GetAlert fetches detailed alert data by ID
// updatedAt is used for cache invalidation - cache key includes it so changes invalidate the cache
//
//nolint:gocyclo // Parsing API response requires many field assignments
func (c *Client) GetAlert(ctx context.Context, id string, updatedAt time.Time) (*Alert, error) {
	// Build cache key with updated_at for smart invalidation
	cacheKey := NewCacheKey(CacheKeyPrefixAlertDetail).
		With("id", id).
		With("updated_at", updatedAt.UTC().Format(time.RFC3339)).
		Build()

	// Check cache first
	if c.cache != nil {
		var cached Alert
		if c.cache.GetTyped(cacheKey, &cached) {
			debug.Logger.Debug("Cache hit for alert detail", "key", cacheKey)
			return &cached, nil
		}
	}

	debug.Logger.Debug("Fetching alert detail", "id", id, "cache", "miss")

	// Build URL - endpoint may already have scheme
	baseURL := c.endpoint
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "https://" + baseURL
	}
	url := fmt.Sprintf("%s/v1/alerts/%s?include=services,environments,groups,responders,alert_urgency", baseURL, id)
	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		debug.Logger.Error("Failed to fetch alert", "error", err)
		return nil, fmt.Errorf("failed to fetch alert: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	debug.Logger.Debug("Alert detail response",
		"status", httpResp.StatusCode,
		"bodyLength", len(body),
	)
	debug.Logger.Debug("Alert detail response body", "json", debug.PrettyJSON(body))

	if httpResp.StatusCode == 403 {
		debug.Logger.Error("API forbidden", "status", httpResp.StatusCode)
		return nil, fmt.Errorf("access denied: API key lacks 'read alerts' permission")
	}
	if httpResp.StatusCode != 200 {
		debug.Logger.Error("API error", "status", httpResp.StatusCode, "body", debug.PrettyJSON(body))
		return nil, fmt.Errorf("API returned status %d", httpResp.StatusCode)
	}

	var result struct {
		Data struct {
			ID         string `json:"id"`
			Attributes struct {
				ShortID     *string `json:"short_id"`
				Summary     string  `json:"summary"`
				Description *string `json:"description"`
				Status      string  `json:"status"`
				Source      *string `json:"source"`
				ExternalURL *string `json:"external_url"`
				CreatedAt   string  `json:"created_at"`
				UpdatedAt   string  `json:"updated_at"`
				StartedAt   *string `json:"started_at"`
				EndedAt     *string `json:"ended_at"`
				Labels      []struct {
					Key   string      `json:"key"`
					Value interface{} `json:"value"`
				} `json:"labels"`
				// Direct arrays (same format as list API)
				Services []struct {
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
				// Additional fields
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
		debug.Logger.Error("Failed to parse alert detail response",
			"error", err,
			"body", debug.PrettyJSON(body),
		)
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

	// Parse additional fields
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

	// Parse notified users
	for _, u := range d.Attributes.NotifiedUsers {
		alert.NotifiedUsers = append(alert.NotifiedUsers, AlertUser{
			Name:  u.Name,
			Email: u.Email,
		})
	}

	// Parse related incidents
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

	// Store in cache
	if c.cache != nil {
		c.cache.Set(cacheKey, alert)
		debug.Logger.Debug("Cached alert detail", "id", alert.ID, "key", cacheKey)
	}

	debug.Logger.Debug("Parsed alert detail", "id", alert.ID, "summary", alert.Summary)
	return alert, nil
}

// Duration calculation methods for Incident

// TimeToDetection returns time from started_at to detected_at in hours
func (i *Incident) TimeToDetection() float64 {
	if i.DetectedAt == nil || i.StartedAt == nil {
		return 0
	}
	return truncateToTwoDecimals(i.DetectedAt.Sub(*i.StartedAt).Hours())
}

// TimeToAcknowledge returns time from started_at to acknowledged_at in hours
func (i *Incident) TimeToAcknowledge() float64 {
	if i.AcknowledgedAt == nil || i.StartedAt == nil {
		return 0
	}
	return truncateToTwoDecimals(i.AcknowledgedAt.Sub(*i.StartedAt).Hours())
}

// TimeToMitigation returns time from started_at to mitigated_at in hours
func (i *Incident) TimeToMitigation() float64 {
	if i.MitigatedAt == nil || i.StartedAt == nil {
		return 0
	}
	return truncateToTwoDecimals(i.MitigatedAt.Sub(*i.StartedAt).Hours())
}

// TimeToResolution returns time from started_at to resolved_at in hours
func (i *Incident) TimeToResolution() float64 {
	if i.ResolvedAt == nil || i.StartedAt == nil {
		return 0
	}
	return truncateToTwoDecimals(i.ResolvedAt.Sub(*i.StartedAt).Hours())
}

// TimeToClose returns time from started_at to closed_at in hours
func (i *Incident) TimeToClose() float64 {
	if i.ClosedAt == nil || i.StartedAt == nil {
		return 0
	}
	return truncateToTwoDecimals(i.ClosedAt.Sub(*i.StartedAt).Hours())
}

// TimeToTriage returns time from in_triage_at to started_at in hours
func (i *Incident) TimeToTriage() float64 {
	if i.InTriageAt == nil {
		return 0
	}
	endTime := i.StartedAt
	if endTime == nil {
		now := time.Now()
		endTime = &now
	}
	return truncateToTwoDecimals(endTime.Sub(*i.InTriageAt).Hours())
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

// MaintenanceDuration returns scheduled maintenance duration in seconds
func (i *Incident) MaintenanceDuration() int64 {
	if i.ScheduledFor == nil || i.ScheduledUntil == nil {
		return 0
	}
	return int64(i.ScheduledUntil.Sub(*i.ScheduledFor).Seconds())
}

// InTriageDuration returns time spent in triage in seconds
func (i *Incident) InTriageDuration() int64 {
	if i.InTriageAt == nil {
		return 0
	}
	endTime := i.StartedAt
	if endTime == nil {
		now := time.Now()
		endTime = &now
	}
	return int64(endTime.Sub(*i.InTriageAt).Seconds())
}

func truncateToTwoDecimals(f float64) float64 {
	return float64(int(f*100)) / 100
}
