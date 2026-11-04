package incidents

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

const escalateTeamResponse = `{"data":{"id":"team-uuid","attributes":{"name":"Platform","slug":"platform"}}}`

func newEscalateCmd(t *testing.T, flags map[string]string) *cobra.Command {
	t.Helper()
	cmd := newTestCmd()
	cmd.Flags().String("team", "", "")
	cmd.Flags().String("escalation-policy", "", "")
	cmd.Flags().String("note", "", "")
	for name, value := range flags {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	return cmd
}

type escalateRecorder struct {
	paged    map[string]interface{}
	attached map[string]interface{}
	paths    []string
}

func (rec *escalateRecorder) handler(t *testing.T, pageStatus, attachStatus int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rec.paths = append(rec.paths, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/incidents/"):
			_, _ = w.Write([]byte(getResponse()))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/teams/platform":
			_, _ = w.Write([]byte(escalateTeamResponse))
		case r.Method == http.MethodGet:
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/alerts":
			_ = json.NewDecoder(r.Body).Decode(&rec.paged)
			if pageStatus != http.StatusOK {
				w.WriteHeader(pageStatus)
				_, _ = w.Write([]byte(`{"errors":[{"title":"One or more notification_targets could not be found for this organization."}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":{"id":"alert-new"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/incidents/inc-1/alerts":
			_ = json.NewDecoder(r.Body).Decode(&rec.attached)
			w.WriteHeader(attachStatus)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}
}

func TestRunEscalatePagesTeamAndLinksIncident(t *testing.T) {
	rec := &escalateRecorder{}
	setupTestServer(t, rec.handler(t, http.StatusOK, http.StatusOK))
	cmd := newEscalateCmd(t, map[string]string{"team": "platform"})
	output := captureStdout(t, func() {
		if err := runEscalate(cmd, []string{"INC-42"}); err != nil {
			t.Fatalf("runEscalate returned error: %v", err)
		}
	})
	if rec.paths[0] != "GET /v1/incidents/42" {
		t.Errorf("first request = %q, want INC-42 normalized to 42", rec.paths[0])
	}
	data := rec.paged["data"].(map[string]interface{})
	if _, ok := data["id"]; ok {
		t.Errorf("a new alert must not send data.id: %+v", data)
	}
	attributes := data["attributes"].(map[string]interface{})
	if !strings.Contains(attributes["summary"].(string), "INC-42") || !strings.Contains(attributes["summary"].(string), "DB outage") {
		t.Errorf("summary = %v", attributes["summary"])
	}
	if _, ok := attributes["description"]; ok {
		t.Errorf("description must be omitted without --note: %+v", attributes)
	}
	targets := attributes["notification_targets"].([]interface{})
	if len(targets) != 1 || targets[0].(map[string]interface{})["type"] != "Group" || targets[0].(map[string]interface{})["id"] != "team-uuid" {
		t.Errorf("targets = %+v", targets)
	}
	ids := rec.attached["alert_ids"].([]interface{})
	if len(ids) != 1 || ids[0] != "alert-new" {
		t.Errorf("alert_ids = %v", ids)
	}
	if !strings.Contains(output, "Paged team Platform for incident INC-42 (alert alert-new)") {
		t.Errorf("output = %q", output)
	}
}

func TestRunEscalateWithPolicyAndNote(t *testing.T) {
	rec := &escalateRecorder{}
	setupTestServer(t, rec.handler(t, http.StatusOK, http.StatusOK))
	cmd := newEscalateCmd(t, map[string]string{"team": "platform", "escalation-policy": "ep-1", "note": "db down"})
	captureStdout(t, func() {
		if err := runEscalate(cmd, []string{"inc-1"}); err != nil {
			t.Fatalf("runEscalate returned error: %v", err)
		}
	})
	attributes := rec.paged["data"].(map[string]interface{})["attributes"].(map[string]interface{})
	if attributes["description"] != "db down" {
		t.Errorf("description = %v", attributes["description"])
	}
	targets := attributes["notification_targets"].([]interface{})
	if len(targets) != 2 || targets[1].(map[string]interface{})["type"] != "EscalationPolicy" || targets[1].(map[string]interface{})["id"] != "ep-1" {
		t.Errorf("targets = %+v", targets)
	}
}

func TestRunEscalateRejectsEmptyTeam(t *testing.T) {
	setupTestServer(t, func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})
	err := runEscalate(newEscalateCmd(t, nil), []string{"INC-42"})
	if err == nil || !strings.Contains(err.Error(), "--team must not be empty") {
		t.Fatalf("error = %v", err)
	}
}

func TestEscalateCmdRequiresTeam(t *testing.T) {
	setupTestServer(t, func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})
	err := escalateCmd.ValidateRequiredFlags()
	if err == nil || !strings.Contains(err.Error(), `required flag(s) "team" not set`) {
		t.Fatalf("error = %v", err)
	}
}

func TestRunEscalateUnknownIncident(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	err := runEscalate(newEscalateCmd(t, map[string]string{"team": "platform"}), []string{"INC-404"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunEscalateUnknownTeam(t *testing.T) {
	rec := &escalateRecorder{}
	setupTestServer(t, rec.handler(t, http.StatusOK, http.StatusOK))
	err := runEscalate(newEscalateCmd(t, map[string]string{"team": "nope"}), []string{"INC-42"})
	if err == nil || !strings.Contains(err.Error(), `unknown team "nope"`) {
		t.Fatalf("error = %v", err)
	}
}

func TestRunEscalateSurfacesPagingError(t *testing.T) {
	rec := &escalateRecorder{}
	setupTestServer(t, rec.handler(t, http.StatusUnprocessableEntity, http.StatusOK))
	err := runEscalate(newEscalateCmd(t, map[string]string{"team": "platform"}), []string{"INC-42"})
	if err == nil || !strings.Contains(err.Error(), "notification_targets could not be found") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunEscalateLinkFailureNamesCreatedAlert(t *testing.T) {
	rec := &escalateRecorder{}
	setupTestServer(t, rec.handler(t, http.StatusOK, http.StatusInternalServerError))
	err := runEscalate(newEscalateCmd(t, map[string]string{"team": "platform"}), []string{"INC-42"})
	if err == nil || !strings.Contains(err.Error(), "was paged by alert alert-new") {
		t.Fatalf("error = %v", err)
	}
}

func TestEscalateHelpDocumentsFlags(t *testing.T) {
	for _, want := range []string{"--team=", "--escalation-policy=", "--note=", "INC-123"} {
		if !strings.Contains(escalateCmd.Example, want) {
			t.Errorf("example does not mention %s", want)
		}
	}
}
