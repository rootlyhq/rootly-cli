// Package statuspagewarn warns, best-effort and on stderr only, when an incident
// is resolved from the CLI while public status-page events are still open.
// It never resolves anything and never returns an error.
package statuspagewarn

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/api"
)

const eventsPageSize = 100

// IsResolved reports whether an incident status counts as resolved.
func IsResolved(status string) bool {
	return status == "resolved" || status == "closed"
}

func isOpen(status string) bool {
	return status != "resolved" && status != "completed"
}

// Incident writes a warning to w for each open status-page event of the incident.
// It does nothing unless status is resolved or closed, and ignores lookup errors.
func Incident(ctx context.Context, client *api.Client, incidentID, status string, w io.Writer) {
	if !IsResolved(status) {
		return
	}
	var open []api.StatusPageEvent
	for page := 1; ; page++ {
		result, err := client.ListStatusPageEventsCLI(ctx, incidentID, page, eventsPageSize)
		if err != nil {
			debugf(w, "status page check skipped: %v", err)
			break
		}
		// An API that ignores the page number returns the same events again: drop them and stop.
		if result.Pagination.CurrentPage != page {
			break
		}
		for _, event := range result.Events {
			if isOpen(event.Status) {
				open = append(open, event)
			}
		}
		hasNext := result.Pagination.HasNext || result.Pagination.CurrentPage < result.Pagination.TotalPages
		if !hasNext || len(result.Events) == 0 {
			break
		}
	}
	if len(open) == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "Warning: incident %s is resolved but %d status page event(s) are still open:\n", incidentID, len(open))
	for _, event := range open {
		_, _ = fmt.Fprintf(w, "  status page %s, status %s: rootly status-pages events resolve %s --message=\"<public resolution message>\"\n", event.StatusPageID, event.Status, event.ID)
	}
	_, _ = fmt.Fprintln(w, "  --message is the public text shown on the status page; resolving may notify subscribers.")
}

// AlertIncidents runs Incident for each resolved or closed incident related to the alert.
func AlertIncidents(ctx context.Context, client *api.Client, alertID string, w io.Writer) {
	alert, err := client.GetAlertByID(ctx, alertID)
	if err != nil {
		debugf(w, "status page check skipped: %v", err)
		return
	}
	for _, incident := range alert.RelatedIncidents {
		Incident(ctx, client, incident.ID, incident.Status, w)
	}
}

func debugf(w io.Writer, format string, args ...interface{}) {
	if viper.GetBool("debug") {
		_, _ = fmt.Fprintf(w, "debug: "+format+"\n", args...)
	}
}
