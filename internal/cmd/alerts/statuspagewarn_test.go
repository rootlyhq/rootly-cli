package alerts

import (
	"net/http"
	"os"
	"strings"
	"testing"
)

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

func resolveWithStatusPageServer(t *testing.T, resolveIncidents bool) (stdout, stderr string, alertGets int) {
	t.Helper()
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/alerts/"):
			alertGets++
			_, _ = w.Write([]byte(`{"data": {"id": "alert-1", "attributes": {"short_id": "ALR-42", "incidents": [
				{"id": "inc-resolved", "attributes": {"status": "resolved"}},
				{"id": "inc-started", "attributes": {"status": "started"}}
			]}}}`))
		case strings.Contains(r.URL.Path, "/inc-resolved/status-page-events"):
			_, _ = w.Write([]byte(`{"data": [{"id": "ev-1", "attributes": {"status": "investigating", "status_page_id": "page-1"}}], "meta": {"current_page": 1, "total_pages": 1}}`))
		case strings.Contains(r.URL.Path, "/inc-started/status-page-events"):
			t.Errorf("events requested for an incident that is not resolved")
		default:
			w.WriteHeader(http.StatusOK)
		}
	})

	cmd := newTestCmd()
	cmd.Flags().String("message", "", "")
	cmd.Flags().Bool("resolve-incidents", false, "")
	if resolveIncidents {
		_ = cmd.Flags().Set("resolve-incidents", "true")
	}

	stderr = captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			if err := runResolve(cmd, []string{"ALR-42"}); err != nil {
				t.Fatalf("runResolve error: %v", err)
			}
		})
	})
	return stdout, stderr, alertGets
}

func TestRunResolveWithIncidentsWarnsAboutOpenStatusPageEvents(t *testing.T) {
	stdout, stderr, _ := resolveWithStatusPageServer(t, true)

	if !strings.Contains(stderr, "inc-resolved") || !strings.Contains(stderr, "rootly status-pages events resolve ev-1") {
		t.Errorf("expected warning for the resolved incident on stderr, got: %s", stderr)
	}
	if stdout != "Resolved alert ALR-42\n" {
		t.Errorf("stdout should only hold the success message, got: %q", stdout)
	}
}

func TestRunResolveWithoutIncidentsDoesNotCheckStatusPages(t *testing.T) {
	_, stderr, alertGets := resolveWithStatusPageServer(t, false)

	if stderr != "" || alertGets != 0 {
		t.Errorf("expected no warning and no alert lookup, got %q and %d lookups", stderr, alertGets)
	}
}
