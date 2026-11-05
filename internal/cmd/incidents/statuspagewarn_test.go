package incidents

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const resolvedIncidentBody = `{"data": {"id": "inc-1", "attributes": {"sequential_id": 42, "title": "DB outage", "status": "resolved", "kind": "normal", "created_at": "2025-06-15T10:00:00Z"}}}`

const openStatusPageEvents = `{"data": [{"id": "ev-1", "attributes": {"event": "Investigating", "status": "investigating", "status_page_id": "page-1"}}], "meta": {"current_page": 1, "total_pages": 1}}`

// captureStderr redirects os.Stderr for the duration of fn, returning captured output.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stderr = w

	fn()

	w.Close()
	os.Stderr = origStderr

	buf := make([]byte, 65536)
	n, _ := r.Read(buf)
	r.Close()
	return string(buf[:n])
}

// statusPageWarnServer serves the incident write and the status-page events lookup.
func statusPageWarnServer(t *testing.T, incidentBody string, eventsStatus int, eventsRequests *int) {
	t.Helper()
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		if strings.HasSuffix(r.URL.Path, "/status-page-events") {
			*eventsRequests++
			w.WriteHeader(eventsStatus)
			_, _ = w.Write([]byte(openStatusPageEvents))
			return
		}
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
		}
		_, _ = w.Write([]byte(incidentBody))
	})
}

func newStatusCmd(title, status string) *cobra.Command {
	cmd := newTestCmd()
	for _, name := range []string{"summary", "severity", "status", "kind", "scheduled-for", "scheduled-until"} {
		cmd.Flags().String(name, "", "")
	}
	cmd.Flags().String("title", title, "")
	cmd.Flags().StringArray("field", nil, "")
	for _, name := range []string{"services", "types", "functionalities", "environments", "teams", "causes"} {
		cmd.Flags().StringSlice(name, nil, "")
	}
	_ = cmd.Flags().Set("status", status)
	return cmd
}

func TestRunUpdateResolvedWarnsAboutOpenStatusPageEvents(t *testing.T) {
	var eventsRequests int
	statusPageWarnServer(t, resolvedIncidentBody, http.StatusOK, &eventsRequests)
	viper.Set("format", "json")

	var stdout string
	stderr := captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			if err := runUpdate(newStatusCmd("", "resolved"), []string{"INC-42"}); err != nil {
				t.Fatalf("runUpdate returned error: %v", err)
			}
		})
	})

	if !strings.Contains(stderr, "rootly status-pages events resolve ev-1") || !strings.Contains(stderr, "page-1") || !strings.Contains(stderr, "investigating") {
		t.Errorf("expected status page warning on stderr, got: %s", stderr)
	}
	if strings.Contains(stdout, "status-pages") || !strings.Contains(stdout, `"inc-1"`) {
		t.Errorf("stdout should hold only the incident JSON, got: %s", stdout)
	}
}

func TestRunUpdateNotResolvedDoesNotCheckStatusPages(t *testing.T) {
	var eventsRequests int
	statusPageWarnServer(t, getResponse(), http.StatusOK, &eventsRequests)
	viper.Set("format", "json")

	stderr := captureStderr(t, func() {
		captureStdout(t, func() {
			if err := runUpdate(newStatusCmd("", "mitigated"), []string{"INC-42"}); err != nil {
				t.Fatalf("runUpdate returned error: %v", err)
			}
		})
	})

	if strings.Contains(stderr, "status-pages") || eventsRequests != 0 {
		t.Errorf("expected no warning and no events request, got %q and %d requests", stderr, eventsRequests)
	}
}

func TestRunUpdateStatusPageLookupFailureIsIgnored(t *testing.T) {
	var eventsRequests int
	statusPageWarnServer(t, resolvedIncidentBody, http.StatusInternalServerError, &eventsRequests)
	viper.Set("format", "json")

	var stdout string
	stderr := captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			if err := runUpdate(newStatusCmd("", "resolved"), []string{"INC-42"}); err != nil {
				t.Fatalf("runUpdate must succeed when the events lookup fails: %v", err)
			}
		})
	})

	if eventsRequests != 1 || strings.Contains(stderr, "status-pages") {
		t.Errorf("expected one failed lookup and no warning, got %d requests and %q", eventsRequests, stderr)
	}
	if !strings.Contains(stdout, `"inc-1"`) {
		t.Errorf("expected incident JSON on stdout, got: %s", stdout)
	}
}

func TestRunCreateResolvedWarnsAboutOpenStatusPageEvents(t *testing.T) {
	var eventsRequests int
	statusPageWarnServer(t, strings.Replace(createResponse(), `"started"`, `"resolved"`, 1), http.StatusOK, &eventsRequests)
	viper.Set("format", "json")

	stderr := captureStderr(t, func() {
		captureStdout(t, func() {
			if err := runCreate(newStatusCmd("New incident", "resolved"), nil); err != nil {
				t.Fatalf("runCreate returned error: %v", err)
			}
		})
	})

	if !strings.Contains(stderr, "rootly status-pages events resolve ev-1") {
		t.Errorf("expected status page warning on stderr, got: %s", stderr)
	}
}
