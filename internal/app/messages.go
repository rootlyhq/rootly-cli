package app

import "github.com/rootlyhq/rootly-tui/internal/api"

// IncidentsLoadedMsg is sent when incidents are loaded from the API
type IncidentsLoadedMsg struct {
	Incidents  []api.Incident
	Pagination api.PaginationInfo
	Err        error
}

// AlertsLoadedMsg is sent when alerts are loaded from the API
type AlertsLoadedMsg struct {
	Alerts     []api.Alert
	Pagination api.PaginationInfo
	Err        error
}

// IncidentDetailLoadedMsg is sent when incident detail is fetched
type IncidentDetailLoadedMsg struct {
	Incident *api.Incident
	Index    int // Index in the incidents list to update
	Err      error
}

// AlertDetailLoadedMsg is sent when alert detail is fetched
type AlertDetailLoadedMsg struct {
	Alert *api.Alert
	Index int // Index in the alerts list to update
	Err   error
}

// ErrorMsg represents a generic error
type ErrorMsg struct {
	Err error
}
