package statuspagewarn

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/config"
)

const mixedEvents = `{"data": [
	{"id": "ev-open", "attributes": {"event": "Investigating", "status": "investigating", "status_page_id": "page-1"}},
	{"id": "ev-resolved", "attributes": {"event": "Done", "status": "resolved", "status_page_id": "page-2"}},
	{"id": "ev-completed", "attributes": {"event": "Done", "status": "completed", "status_page_id": "page-3"}}
], "meta": {"current_page": 1, "total_pages": 1}}`

func newClient(t *testing.T, handler http.HandlerFunc) (*api.Client, *int32) {
	t.Helper()
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	t.Cleanup(viper.Reset)
	client, err := api.NewClient(&config.Config{APIKey: "test-token", Endpoint: server.URL})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client, &requests
}

func eventsHandler(body string, status int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

func TestIsResolved(t *testing.T) {
	for status, want := range map[string]bool{"resolved": true, "closed": true, "started": false, "mitigated": false, "cancelled": false, "": false} {
		if got := IsResolved(status); got != want {
			t.Errorf("IsResolved(%q) = %v, want %v", status, got, want)
		}
	}
}

func TestIncidentListsOpenEvents(t *testing.T) {
	client, _ := newClient(t, eventsHandler(mixedEvents, http.StatusOK))
	var buf bytes.Buffer
	Incident(context.Background(), client, "inc-1", "resolved", &buf)

	out := buf.String()
	for _, want := range []string{"inc-1", "1 status page event(s)", "status page page-1", "status investigating", "rootly status-pages events resolve ev-open"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"ev-resolved", "ev-completed"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("output lists closed event %q:\n%s", unwanted, out)
		}
	}
}

func TestIncidentFollowsPagination(t *testing.T) {
	client, requests := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page[number]") == "2" {
			_, _ = w.Write([]byte(`{"data": [{"id": "ev-page2", "attributes": {"status": "investigating", "status_page_id": "page-9"}}],
				"meta": {"current_page": 2, "total_pages": 2}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data": [{"id": "ev-old", "attributes": {"status": "resolved", "status_page_id": "page-1"}}],
			"meta": {"current_page": 1, "next_page": 2, "total_pages": 2}}`))
	})
	var buf bytes.Buffer
	Incident(context.Background(), client, "inc-1", "resolved", &buf)

	if *requests != 2 || !strings.Contains(buf.String(), "events resolve ev-page2") {
		t.Errorf("expected open event from page 2 after %d requests, got %q", *requests, buf.String())
	}
}

func TestIncidentFollowsManyPages(t *testing.T) {
	const last = 25
	client, requests := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(r.URL.Query().Get("page[number]"))
		status, next := "resolved", fmt.Sprintf(`"next_page": %d,`, n+1)
		if n == last {
			status, next = "investigating", ""
		}
		_, _ = fmt.Fprintf(w, `{"data": [{"id": "ev-%d", "attributes": {"status": %q, "status_page_id": "p"}}],
			"meta": {"current_page": %d, %s "total_pages": %d}}`, n, status, n, next, last)
	})
	var buf bytes.Buffer
	Incident(context.Background(), client, "inc-1", "resolved", &buf)
	if int(*requests) != last || !strings.Contains(buf.String(), "events resolve ev-25") {
		t.Errorf("expected open event from page %d, got %q after %d requests", last, buf.String(), *requests)
	}
}

func TestIncidentStopsWhenPageIgnored(t *testing.T) {
	client, requests := newClient(t, eventsHandler(`{"data": [{"id": "ev-open", "attributes": {"status": "investigating", "status_page_id": "p"}}], "meta": {"current_page": 1, "next_page": 2}}`, http.StatusOK))
	var buf bytes.Buffer
	Incident(context.Background(), client, "inc-1", "resolved", &buf)
	if *requests != 2 || strings.Count(buf.String(), "events resolve ev-open") != 1 {
		t.Errorf("expected 2 requests and the event listed once, got %d requests: %q", *requests, buf.String())
	}
}

func TestIncidentFollowsTotalPagesWithoutNextPage(t *testing.T) {
	client, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page[number]") == "2" {
			_, _ = w.Write([]byte(`{"data": [{"id": "ev-page2", "attributes": {"status": "investigating", "status_page_id": "p"}}], "meta": {"current_page": 2, "total_pages": 2}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data": [{"id": "a", "attributes": {"status": "resolved", "status_page_id": "p"}}], "meta": {"current_page": 1, "total_pages": 2}}`))
	})
	var buf bytes.Buffer
	Incident(context.Background(), client, "inc-1", "resolved", &buf)
	if !strings.Contains(buf.String(), "events resolve ev-page2") {
		t.Errorf("expected page 2 event, got %q", buf.String())
	}
}

func TestIncidentPrintsRequiredMessageFlag(t *testing.T) {
	client, _ := newClient(t, eventsHandler(mixedEvents, http.StatusOK))
	var buf bytes.Buffer
	Incident(context.Background(), client, "inc-1", "resolved", &buf)
	if !strings.Contains(buf.String(), `events resolve ev-open --message="<public resolution message>"`) {
		t.Errorf("command must include --message, got %q", buf.String())
	}
}

func TestIncidentSilentCases(t *testing.T) {
	cases := map[string]string{
		"no events": `{"data": [], "meta": {"current_page": 1, "total_pages": 1}}`,
		"all closed": `{"data": [
			{"id": "a", "attributes": {"status": "resolved", "status_page_id": "p"}},
			{"id": "b", "attributes": {"status": "completed", "status_page_id": "p"}}
		], "meta": {"current_page": 1, "total_pages": 1}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			client, _ := newClient(t, eventsHandler(body, http.StatusOK))
			var buf bytes.Buffer
			Incident(context.Background(), client, "inc-1", "closed", &buf)
			if buf.Len() != 0 {
				t.Errorf("expected no output, got %q", buf.String())
			}
		})
	}
}

func TestIncidentNotResolvedMakesNoRequest(t *testing.T) {
	client, requests := newClient(t, eventsHandler(mixedEvents, http.StatusOK))
	var buf bytes.Buffer
	for _, status := range []string{"started", "mitigated", ""} {
		Incident(context.Background(), client, "inc-1", status, &buf)
	}
	if buf.Len() != 0 || *requests != 0 {
		t.Errorf("expected no output and no requests, got %q and %d requests", buf.String(), *requests)
	}
}

func TestIncidentLookupFailureIsSilent(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusForbidden} {
		client, _ := newClient(t, eventsHandler(`{}`, status))
		var buf bytes.Buffer
		Incident(context.Background(), client, "inc-1", "resolved", &buf)
		if buf.Len() != 0 {
			t.Errorf("status %d: expected no output, got %q", status, buf.String())
		}
	}
}

func TestIncidentLookupFailureDebugNote(t *testing.T) {
	client, _ := newClient(t, eventsHandler(`{}`, http.StatusInternalServerError))
	viper.Set("debug", true)
	var buf bytes.Buffer
	Incident(context.Background(), client, "inc-1", "resolved", &buf)
	if !strings.Contains(buf.String(), "debug: status page check skipped") {
		t.Errorf("expected debug note, got %q", buf.String())
	}
}

func alertHandler(alertStatus int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/alerts/"):
			w.WriteHeader(alertStatus)
			_, _ = w.Write([]byte(`{"data": {"id": "alert-1", "attributes": {"short_id": "ALR-1", "incidents": [
				{"id": "inc-resolved", "attributes": {"status": "resolved"}},
				{"id": "inc-started", "attributes": {"status": "started"}}
			]}}}`))
		case strings.Contains(r.URL.Path, "/inc-resolved/status-page-events"):
			_, _ = w.Write([]byte(mixedEvents))
		default:
			http.Error(w, "unexpected request "+r.URL.Path, http.StatusTeapot)
		}
	}
}

func TestAlertIncidentsWarnsOnlyForResolved(t *testing.T) {
	client, _ := newClient(t, alertHandler(http.StatusOK))
	var buf bytes.Buffer
	AlertIncidents(context.Background(), client, "alert-1", &buf)

	out := buf.String()
	if !strings.Contains(out, "incident inc-resolved") || !strings.Contains(out, "events resolve ev-open") {
		t.Errorf("expected warning for resolved incident, got %q", out)
	}
	if strings.Contains(out, "inc-started") {
		t.Errorf("unexpected warning for started incident: %q", out)
	}
}

func TestAlertIncidentsLookupFailure(t *testing.T) {
	client, _ := newClient(t, alertHandler(http.StatusInternalServerError))
	var buf bytes.Buffer
	AlertIncidents(context.Background(), client, "alert-1", &buf)
	if buf.Len() != 0 {
		t.Errorf("expected no output, got %q", buf.String())
	}

	viper.Set("debug", true)
	AlertIncidents(context.Background(), client, "alert-1", &buf)
	if !strings.Contains(buf.String(), "debug: status page check skipped") {
		t.Errorf("expected debug note, got %q", buf.String())
	}
}
