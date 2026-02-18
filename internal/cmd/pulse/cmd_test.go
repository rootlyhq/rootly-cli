package pulse

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/api"
)

func newTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	return cmd
}

func setupTestServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	viper.Set("api_key", "test-token")
	viper.Set("api_host", server.URL)
	t.Cleanup(viper.Reset)
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = origStdout

	buf := make([]byte, 65536)
	n, _ := r.Read(buf)
	r.Close()
	return string(buf[:n])
}

func pulseCreateResponse() string {
	return `{
		"data": {
			"id": "pulse-123",
			"attributes": {
				"summary": "Deploy v1.2.3",
				"source": "ci",
				"started_at": "2025-06-15T10:00:00Z",
				"ended_at": "2025-06-15T10:05:00Z"
			}
		}
	}`
}

// --- runCreate tests ---

func TestRunCreateWithArgs(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/v1/pulses") {
			t.Errorf("expected /v1/pulses path, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(pulseCreateResponse()))
	})

	viper.Set("format", "table")
	viper.Set("quiet", true)

	cmd := newTestCmd()
	cmd.Flags().StringP("labels", "l", "", "")
	cmd.Flags().StringP("services", "s", "", "")
	cmd.Flags().StringP("environments", "e", "", "")
	cmd.Flags().String("source", "cli", "")
	cmd.Flags().StringP("refs", "r", "", "")
	cmd.Flags().String("summary", "", "")

	output := captureStdout(t, func() {
		err := runCreate(cmd, []string{"Deploy", "v1.2.3"})
		if err != nil {
			t.Fatalf("runCreate error: %v", err)
		}
	})

	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestRunCreateWithSummaryFlag(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(pulseCreateResponse()))
	})

	viper.Set("format", "json")
	viper.Set("quiet", true)

	cmd := newTestCmd()
	cmd.Flags().StringP("labels", "l", "", "")
	cmd.Flags().StringP("services", "s", "", "")
	cmd.Flags().StringP("environments", "e", "", "")
	cmd.Flags().String("source", "cli", "")
	cmd.Flags().StringP("refs", "r", "", "")
	cmd.Flags().String("summary", "Deploy v1.2.3", "")
	cmd.Flags().Set("summary", "Deploy v1.2.3")

	output := captureStdout(t, func() {
		err := runCreate(cmd, nil)
		if err != nil {
			t.Fatalf("runCreate error: %v", err)
		}
	})

	if !strings.Contains(output, "pulse-123") {
		t.Errorf("expected 'pulse-123' in output, got: %s", output)
	}
}

func TestRunCreateWithLabelsAndServices(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(pulseCreateResponse()))
	})

	viper.Set("format", "table")
	viper.Set("quiet", true)

	cmd := newTestCmd()
	cmd.Flags().StringP("labels", "l", "", "")
	cmd.Flags().StringP("services", "s", "", "")
	cmd.Flags().StringP("environments", "e", "", "")
	cmd.Flags().String("source", "ci", "")
	cmd.Flags().StringP("refs", "r", "", "")
	cmd.Flags().String("summary", "", "")
	cmd.Flags().Set("labels", "version=1.2.3,team=backend")
	cmd.Flags().Set("services", "api-gateway,payments")
	cmd.Flags().Set("environments", "production")
	cmd.Flags().Set("source", "ci")

	output := captureStdout(t, func() {
		err := runCreate(cmd, []string{"Deploy v1.2.3"})
		if err != nil {
			t.Fatalf("runCreate error: %v", err)
		}
	})

	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestRunCreateNoSummary(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	cmd := newTestCmd()
	cmd.Flags().StringP("labels", "l", "", "")
	cmd.Flags().StringP("services", "s", "", "")
	cmd.Flags().StringP("environments", "e", "", "")
	cmd.Flags().String("source", "cli", "")
	cmd.Flags().StringP("refs", "r", "", "")
	cmd.Flags().String("summary", "", "")

	err := runCreate(cmd, nil)
	if err == nil {
		t.Fatal("expected error when no summary")
	}
	if !strings.Contains(err.Error(), "summary required") {
		t.Errorf("expected 'summary required' error, got: %v", err)
	}
}

func TestRunCreateNoToken(t *testing.T) {
	viper.Reset()
	defer viper.Reset()

	cmd := newTestCmd()
	cmd.Flags().StringP("labels", "l", "", "")
	cmd.Flags().StringP("services", "s", "", "")
	cmd.Flags().StringP("environments", "e", "", "")
	cmd.Flags().String("source", "cli", "")
	cmd.Flags().StringP("refs", "r", "", "")
	cmd.Flags().String("summary", "", "")

	err := runCreate(cmd, []string{"Deploy v1.2.3"})
	if err == nil {
		t.Fatal("expected error when no API key")
	}
	if !strings.Contains(err.Error(), "API key required") {
		t.Errorf("expected 'API key required' error, got: %v", err)
	}
}

func TestRunCreateInvalidLabels(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	cmd := newTestCmd()
	cmd.Flags().StringP("labels", "l", "", "")
	cmd.Flags().StringP("services", "s", "", "")
	cmd.Flags().StringP("environments", "e", "", "")
	cmd.Flags().String("source", "cli", "")
	cmd.Flags().StringP("refs", "r", "", "")
	cmd.Flags().String("summary", "", "")
	cmd.Flags().Set("labels", "no-equals-sign")

	err := runCreate(cmd, []string{"Deploy"})
	if err == nil {
		t.Fatal("expected error for invalid labels")
	}
	if !strings.Contains(err.Error(), "invalid --labels") {
		t.Errorf("expected 'invalid --labels' error, got: %v", err)
	}
}

func TestRunCreateAPIError(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"errors": [{"title": "Server Error"}]}`))
	})

	viper.Set("format", "table")

	cmd := newTestCmd()
	cmd.Flags().StringP("labels", "l", "", "")
	cmd.Flags().StringP("services", "s", "", "")
	cmd.Flags().StringP("environments", "e", "", "")
	cmd.Flags().String("source", "cli", "")
	cmd.Flags().StringP("refs", "r", "", "")
	cmd.Flags().String("summary", "", "")

	err := runCreate(cmd, []string{"Deploy"})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "failed to create pulse") {
		t.Errorf("expected 'failed to create pulse' error, got: %v", err)
	}
}

// --- parseKeyValuePairs tests ---

func TestParseKeyValuePairs(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []api.KeyValue
		wantErr bool
	}{
		{
			name:  "simple pair",
			input: "key=value",
			want:  []api.KeyValue{{Key: "key", Value: "value"}},
		},
		{
			name:  "multiple pairs",
			input: "key1=val1, key2=val2",
			want:  []api.KeyValue{{Key: "key1", Value: "val1"}, {Key: "key2", Value: "val2"}},
		},
		{
			name:  "spaces in key become underscores",
			input: "My Key=value",
			want:  []api.KeyValue{{Key: "my_key", Value: "value"}},
		},
		{
			name:  "key lowercased",
			input: "VERSION=1.2.3",
			want:  []api.KeyValue{{Key: "version", Value: "1.2.3"}},
		},
		{
			name:  "empty input",
			input: "",
			want:  nil,
		},
		{
			name:  "whitespace only",
			input: "  ",
			want:  nil,
		},
		{
			name:  "trailing comma",
			input: "key=value,",
			want:  []api.KeyValue{{Key: "key", Value: "value"}},
		},
		{
			name:    "missing equals",
			input:   "noequals",
			wantErr: true,
		},
		{
			name:    "empty value",
			input:   "key=",
			wantErr: true,
		},
		{
			name:    "empty key",
			input:   "=value",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseKeyValuePairs(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d pairs, want %d", len(got), len(tt.want))
			}
			for i, kv := range got {
				if kv.Key != tt.want[i].Key || kv.Value != tt.want[i].Value {
					t.Errorf("pair[%d] = {%q, %q}, want {%q, %q}", i, kv.Key, kv.Value, tt.want[i].Key, tt.want[i].Value)
				}
			}
		})
	}
}

// --- parseCommaSeparated tests ---

func TestParseCommaSeparated(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "single value",
			input: "api-gateway",
			want:  []string{"api-gateway"},
		},
		{
			name:  "multiple values",
			input: "api-gateway, payments, auth",
			want:  []string{"api-gateway", "payments", "auth"},
		},
		{
			name:  "empty input",
			input: "",
			want:  nil,
		},
		{
			name:  "whitespace only",
			input: "  ",
			want:  nil,
		},
		{
			name:  "trailing comma",
			input: "a,b,",
			want:  []string{"a", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCommaSeparated(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d items, want %d", len(got), len(tt.want))
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("item[%d] = %q, want %q", i, v, tt.want[i])
				}
			}
		})
	}
}

// --- resolveStringFlag tests ---

func TestResolveStringFlag(t *testing.T) {
	// Test default value
	cmd := newTestCmd()
	cmd.Flags().String("test-flag", "default", "")
	val := resolveStringFlag(cmd, "test-flag", "ROOTLY_TEST_NONEXISTENT_VAR", "fallback")
	if val != "fallback" {
		t.Errorf("expected 'fallback', got %q", val)
	}

	// Test flag explicitly set
	cmd2 := newTestCmd()
	cmd2.Flags().String("test-flag", "", "")
	cmd2.Flags().Set("test-flag", "explicit")
	val2 := resolveStringFlag(cmd2, "test-flag", "ROOTLY_TEST_NONEXISTENT_VAR", "default")
	if val2 != "explicit" {
		t.Errorf("expected 'explicit', got %q", val2)
	}
}

func TestResolveStringFlagEnvVar(t *testing.T) {
	envKey := "ROOTLY_TEST_RESOLVE_FLAG"
	original := os.Getenv(envKey)
	os.Setenv(envKey, "from-env")
	defer os.Setenv(envKey, original)

	cmd := newTestCmd()
	cmd.Flags().String("test-flag", "", "")
	val := resolveStringFlag(cmd, "test-flag", envKey, "default")
	if val != "from-env" {
		t.Errorf("expected 'from-env', got %q", val)
	}
}

// --- runProgram tests (equivalent to old repo's TestRunProgram) ---

func TestRunProgramSuccess(t *testing.T) {
	exitCode, err := runProgram(context.Background(), "echo", "hello", "world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestRunProgramNonZeroExit(t *testing.T) {
	exitCode, err := runProgram(context.Background(), "false")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
}

func TestRunProgramNotFound(t *testing.T) {
	exitCode, err := runProgram(context.Background(), "program_that_doesnt_exist_xyz")
	if err == nil {
		t.Fatal("expected error for nonexistent program")
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0 on start failure, got %d", exitCode)
	}
}

func TestRunProgramWithExitCode2(t *testing.T) {
	// sh -c "exit 2" returns exit code 2
	exitCode, err := runProgram(context.Background(), "sh", "-c", "exit 2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}
}

// --- runRun tests (additional coverage) ---

func TestRunRunNoArgs(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	cmd := newTestCmd()
	cmd.Flags().String("summary", "", "")
	cmd.Flags().StringP("labels", "l", "", "")
	cmd.Flags().StringP("services", "s", "", "")
	cmd.Flags().StringP("environments", "e", "", "")
	cmd.Flags().String("source", "cli", "")
	cmd.Flags().StringP("refs", "r", "", "")

	err := runRun(cmd, nil)
	if err == nil {
		t.Fatal("expected error when no args")
	}
	if !strings.Contains(err.Error(), "command required") {
		t.Errorf("expected 'command required' error, got: %v", err)
	}
}

func TestRunRunNoAPIKey(t *testing.T) {
	viper.Reset()
	defer viper.Reset()

	cmd := newTestCmd()
	cmd.Flags().String("summary", "", "")
	cmd.Flags().StringP("labels", "l", "", "")
	cmd.Flags().StringP("services", "s", "", "")
	cmd.Flags().StringP("environments", "e", "", "")
	cmd.Flags().String("source", "cli", "")
	cmd.Flags().StringP("refs", "r", "", "")

	err := runRun(cmd, []string{"echo", "hello"})
	if err == nil {
		t.Fatal("expected error when no API key")
	}
	if !strings.Contains(err.Error(), "API key required") {
		t.Errorf("expected 'API key required' error, got: %v", err)
	}
}

func TestRunRunInvalidLabels(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	cmd := newTestCmd()
	cmd.Flags().String("summary", "", "")
	cmd.Flags().StringP("labels", "l", "", "")
	cmd.Flags().StringP("services", "s", "", "")
	cmd.Flags().StringP("environments", "e", "", "")
	cmd.Flags().String("source", "cli", "")
	cmd.Flags().StringP("refs", "r", "", "")
	cmd.Flags().Set("labels", "no-equals")

	err := runRun(cmd, []string{"echo", "hello"})
	if err == nil {
		t.Fatal("expected error for invalid labels")
	}
	if !strings.Contains(err.Error(), "invalid --labels") {
		t.Errorf("expected 'invalid --labels' error, got: %v", err)
	}
}

func TestRunRunInvalidRefs(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	cmd := newTestCmd()
	cmd.Flags().String("summary", "", "")
	cmd.Flags().StringP("labels", "l", "", "")
	cmd.Flags().StringP("services", "s", "", "")
	cmd.Flags().StringP("environments", "e", "", "")
	cmd.Flags().String("source", "cli", "")
	cmd.Flags().StringP("refs", "r", "", "")
	cmd.Flags().Set("refs", "no-equals")

	err := runRun(cmd, []string{"echo", "hello"})
	if err == nil {
		t.Fatal("expected error for invalid refs")
	}
	if !strings.Contains(err.Error(), "invalid --refs") {
		t.Errorf("expected 'invalid --refs' error, got: %v", err)
	}
}

func TestRunCreateInvalidRefs(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	cmd := newTestCmd()
	cmd.Flags().StringP("labels", "l", "", "")
	cmd.Flags().StringP("services", "s", "", "")
	cmd.Flags().StringP("environments", "e", "", "")
	cmd.Flags().String("source", "cli", "")
	cmd.Flags().StringP("refs", "r", "", "")
	cmd.Flags().String("summary", "", "")
	cmd.Flags().Set("refs", "no-equals")

	err := runCreate(cmd, []string{"Deploy"})
	if err == nil {
		t.Fatal("expected error for invalid refs")
	}
	if !strings.Contains(err.Error(), "invalid --refs") {
		t.Errorf("expected 'invalid --refs' error, got: %v", err)
	}
}

func TestRunCreateYAMLFormat(t *testing.T) {
	setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(pulseCreateResponse()))
	})

	viper.Set("format", "yaml")
	viper.Set("quiet", true)

	cmd := newTestCmd()
	cmd.Flags().StringP("labels", "l", "", "")
	cmd.Flags().StringP("services", "s", "", "")
	cmd.Flags().StringP("environments", "e", "", "")
	cmd.Flags().String("source", "cli", "")
	cmd.Flags().StringP("refs", "r", "", "")
	cmd.Flags().String("summary", "", "")

	output := captureStdout(t, func() {
		err := runCreate(cmd, []string{"Deploy v1.2.3"})
		if err != nil {
			t.Fatalf("runCreate error: %v", err)
		}
	})

	if !strings.Contains(output, "pulse-123") {
		t.Errorf("expected 'pulse-123' in YAML output, got: %s", output)
	}
}
