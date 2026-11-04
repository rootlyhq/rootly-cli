package alerts

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

const teamResponse = `{"data":{"id":"team-uuid","attributes":{"name":"Platform","slug":"platform"}}}`

func newTeamEscalateCmd(t *testing.T, flags map[string]string) *cobra.Command {
	t.Helper()
	cmd := newTestCmd()
	cmd.Flags().String("escalation-policy", "", "")
	cmd.Flags().Int("level", 0, "")
	cmd.Flags().String("team", "", "")
	for name, value := range flags {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	return cmd
}

func TestRunEscalateTeamPagesExistingAlert(t *testing.T) {
	var paged map[string]interface{}
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/alerts/ALR-42":
			_, _ = w.Write([]byte(alertGetResponse()))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/teams/platform":
			_, _ = w.Write([]byte(teamResponse))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/alerts":
			if err := json.NewDecoder(r.Body).Decode(&paged); err != nil {
				t.Errorf("failed to decode request body: %v", err)
			}
			_, _ = w.Write([]byte(`{"data":{"id":"alert-1"}}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	cmd := newTeamEscalateCmd(t, map[string]string{"team": "platform"})
	output := captureStdout(t, func() {
		if err := runEscalate(cmd, []string{"ALR-42"}); err != nil {
			t.Fatalf("runEscalate returned error: %v", err)
		}
	})
	if !strings.Contains(output, "Paged team Platform on alert ALR-42") {
		t.Errorf("output = %q", output)
	}
	data := paged["data"].(map[string]interface{})
	if data["id"] != "alert-1" {
		t.Errorf("data.id = %v, want alert-1", data["id"])
	}
	attributes := data["attributes"].(map[string]interface{})
	if attributes["summary"] != "High CPU usage" {
		t.Errorf("summary = %v", attributes["summary"])
	}
	targets := attributes["notification_targets"].([]interface{})
	if len(targets) != 1 || targets[0].(map[string]interface{})["id"] != "team-uuid" || targets[0].(map[string]interface{})["type"] != "Group" {
		t.Errorf("targets = %+v", targets)
	}
}

func TestRunEscalateTeamWithPolicyPagesBothTargets(t *testing.T) {
	var targets []interface{}
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/alerts/"):
			_, _ = w.Write([]byte(alertGetResponse()))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/teams/"):
			_, _ = w.Write([]byte(teamResponse))
		default:
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			targets = body["data"].(map[string]interface{})["attributes"].(map[string]interface{})["notification_targets"].([]interface{})
			_, _ = w.Write([]byte(`{"data":{"id":"alert-1"}}`))
		}
	})
	cmd := newTeamEscalateCmd(t, map[string]string{"team": "platform", "escalation-policy": "ep-1"})
	captureStdout(t, func() {
		if err := runEscalate(cmd, []string{"ALR-42"}); err != nil {
			t.Fatalf("runEscalate returned error: %v", err)
		}
	})
	if len(targets) != 2 || targets[1].(map[string]interface{})["type"] != "EscalationPolicy" || targets[1].(map[string]interface{})["id"] != "ep-1" {
		t.Errorf("targets = %+v", targets)
	}
}

func TestRunEscalateTeamRejectsEmptyTeam(t *testing.T) {
	setupTestServer(t, func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})
	cmd := newTeamEscalateCmd(t, map[string]string{"team": ""})
	err := runEscalate(cmd, []string{"ALR-42"})
	if err == nil || !strings.Contains(err.Error(), "--team must not be empty") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunEscalateTeamUnknownTeam(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/alerts/") {
			_, _ = w.Write([]byte(alertGetResponse()))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	cmd := newTeamEscalateCmd(t, map[string]string{"team": "nope"})
	err := runEscalate(cmd, []string{"ALR-42"})
	if err == nil || !strings.Contains(err.Error(), `unknown team "nope"`) {
		t.Fatalf("error = %v", err)
	}
}

func TestRunEscalateTeamUnknownAlert(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	cmd := newTeamEscalateCmd(t, map[string]string{"team": "platform"})
	err := runEscalate(cmd, []string{"ALR-404"})
	if err == nil || !strings.Contains(err.Error(), "alert not found") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunEscalateTeamSurfacesPagingError(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/alerts/"):
			_, _ = w.Write([]byte(alertGetResponse()))
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(teamResponse))
		default:
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"errors":[{"title":"Alerting is not enabled for this organization."}]}`))
		}
	})
	cmd := newTeamEscalateCmd(t, map[string]string{"team": "platform"})
	err := runEscalate(cmd, []string{"ALR-42"})
	if err == nil || !strings.Contains(err.Error(), "Alerting is not enabled for this organization.") {
		t.Fatalf("error = %v", err)
	}
}

func TestEscalateCmdTeamExcludesLevel(t *testing.T) {
	setupTestServer(t, func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})
	t.Cleanup(func() {
		for _, name := range []string{"team", "level"} {
			f := escalateCmd.Flags().Lookup(name)
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		}
	})
	if err := escalateCmd.ParseFlags([]string{"--team=platform", "--level=2"}); err != nil {
		t.Fatal(err)
	}
	err := escalateCmd.ValidateFlagGroups()
	if err == nil || !strings.Contains(err.Error(), "none of the others can be") {
		t.Fatalf("error = %v, want mutual exclusion", err)
	}
}

func TestEscalateHelpDocumentsTeam(t *testing.T) {
	if !strings.Contains(escalateCmd.Example, "--team=") || !strings.Contains(escalateCmd.Long, "--team") {
		t.Errorf("help does not document --team")
	}
}
