package builtin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFilesystemToolsConfinePathsAndDenySensitiveFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, "project")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(home, "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "outside-link")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	safeTarget := filepath.Join(root, "safe-config")
	if err := os.WriteFile(safeTarget, []byte("not sensitive"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(safeTarget, filepath.Join(root, ".env.local")); err != nil {
		t.Fatal(err)
	}
	registry := testRegistry(t, root, nil)
	readFile := mustLookup(t, registry, "local_read_file")
	for _, path := range []string{"../outside.txt", "outside-link", ".env.local"} {
		result := readFile.Run(context.Background(), map[string]any{"path": path})
		if !result.IsError {
			t.Errorf("reading %q unexpectedly succeeded: %q", path, result.Output)
		}
	}
	listing := mustLookup(t, registry, "local_list_dir").Run(context.Background(), map[string]any{})
	if strings.Contains(listing.Output, "outside-link") || strings.Contains(listing.Output, ".env.local") {
		t.Fatalf("directory listing returned a denied symlink: %s", listing.Output)
	}
	sensitiveRegistry := testRegistry(t, home, nil)
	sensitiveReadFile := mustLookup(t, sensitiveRegistry, "local_read_file")
	cases := []struct {
		name string
		path string
	}{
		{name: "ssh directory", path: ".ssh/config"},
		{name: "aws directory", path: ".aws/config"},
		{name: "gcloud directory", path: ".config/gcloud/credentials.db"},
		{name: "kube config", path: ".kube/config"},
		{name: "env file", path: ".env"},
		{name: "env variant", path: ".env.production"},
		{name: "pem file", path: "server.pem"},
		{name: "key file", path: "server.key"},
		{name: "id file", path: "id_service"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fullPath := filepath.Join(home, test.path)
			if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(fullPath, []byte("sensitive"), 0o600); err != nil {
				t.Fatal(err)
			}
			result := sensitiveReadFile.Run(context.Background(), map[string]any{"path": test.path})
			if !result.IsError || !strings.Contains(result.Output, "path is denied") {
				t.Errorf("sensitive path was not denied: %#v", result)
			}
		})
	}
}

func TestFilesystemTraversalSkipsGitAndSensitivePaths(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	for path, body := range map[string]string{
		"visible.txt":              "match",
		".git/hidden.txt":          "match",
		".aws/credentials":         "match",
		"nested/visible-match.txt": "match",
	} {
		fullPath := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	registry := testRegistry(t, root, nil)
	glob := mustLookup(t, registry, "local_glob").Run(context.Background(), map[string]any{"pattern": "**/*.txt"})
	if strings.Contains(glob.Output, ".git") || strings.Contains(glob.Output, ".aws") {
		t.Fatalf("glob returned a denied path: %s", glob.Output)
	}
	grep := mustLookup(t, registry, "local_grep").Run(context.Background(), map[string]any{
		"pattern":     "match",
		"output_mode": "content",
	})
	if strings.Contains(grep.Output, ".git") || strings.Contains(grep.Output, ".aws") {
		t.Fatalf("grep returned a denied path: %s", grep.Output)
	}
}

func TestFilesystemOutputIsCapped(t *testing.T) {
	root := t.TempDir()
	content := strings.Repeat("x", maxFSOutput+100)
	if err := os.WriteFile(filepath.Join(root, "large.txt"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result := mustLookup(t, testRegistry(t, root, nil), "local_read_file").Run(
		context.Background(),
		map[string]any{"path": "large.txt"},
	)
	if len([]rune(result.Output)) != maxFSOutput || !strings.Contains(result.Output, "[truncated]") {
		t.Fatalf("expected capped output, got %d characters", len([]rune(result.Output)))
	}
}

func TestBashIsApprovalGatedAndEnforcesTimeout(t *testing.T) {
	bash := mustLookup(t, testRegistry(t, t.TempDir(), nil), "local_bash")
	if !bash.NeedsApproval(map[string]any{"command": "true"}) {
		t.Fatal("bash must always require approval")
	}
	result := bash.Run(context.Background(), map[string]any{
		"command":         "exec sleep 5",
		"timeout_seconds": 1,
	})
	if !result.IsError || !strings.Contains(result.Output, "timed out") {
		t.Fatalf("expected bash timeout, got %#v", result)
	}
	capped := bash.Run(context.Background(), map[string]any{"command": "printf '%30000s' ''"})
	if len([]rune(capped.Output)) != maxShellOutput || !strings.Contains(capped.Output, "[truncated]") {
		t.Fatalf("expected bash output cap, got %d characters", len([]rune(capped.Output)))
	}
}

func TestBashRedactsSensitiveEnvironmentValues(t *testing.T) {
	const secret = "supersecretvalue123"
	t.Setenv("ROOTLY_TEST_TOKEN", secret)
	bash := mustLookup(t, testRegistry(t, t.TempDir(), nil), "local_bash")
	result := bash.Run(context.Background(), map[string]any{"command": `echo "$ROOTLY_TEST_TOKEN"`})
	if result.IsError {
		t.Fatalf("bash command failed: %#v", result)
	}
	if strings.Contains(result.Output, secret) || !strings.Contains(result.Output, "[REDACTED]") {
		t.Fatalf("sensitive environment value was not redacted: %q", result.Output)
	}
}

func TestRunCommandRedactsBeforeOutputTruncation(t *testing.T) {
	const secret = "supersecretvalue123"
	t.Setenv("ROOTLY_TEST_TOKEN", secret)
	result := runCommand(
		context.Background(),
		t.TempDir(),
		"bash",
		[]string{"-c", `printf '%19975s' ''; printf '%s' "$ROOTLY_TEST_TOKEN"`},
		defaultTimeout,
		maxShellOutput,
		"bash",
	)
	if strings.Contains(result.Output, secret[:5]) {
		t.Fatalf("output truncation exposed part of a sensitive value: %q", result.Output)
	}
}

func TestCLIDiscoveryAllowListApprovalAndTimeout(t *testing.T) {
	root := t.TempDir()
	binDir := t.TempDir()
	executable := filepath.Join(binDir, "gh")
	writeExecutable(t, executable, "#!/bin/sh\nprintf '%13000s' ''\n")
	lookPath := func(bin string) (string, error) {
		if bin == "gh" {
			return executable, nil
		}
		return "", errors.New("not found")
	}
	registry := testRegistry(t, root, lookPath)
	if _, ok := registry.Lookup("local_gh"); !ok {
		t.Fatal("gh should be advertised when installed on PATH")
	}
	if _, ok := registry.Lookup("local_aws"); ok {
		t.Fatal("aws should not be advertised when absent from PATH")
	}
	gh := mustLookup(t, registry, "local_gh")
	result := gh.Run(context.Background(), map[string]any{"args": []any{"version"}})
	if result.IsError || len([]rune(result.Output)) != maxCLIOutput || !strings.Contains(result.Output, "[truncated]") {
		t.Fatalf("expected capped CLI output, got %#v", result)
	}
	argsExecutable := filepath.Join(binDir, "args-gh")
	writeExecutable(t, argsExecutable, "#!/bin/sh\nprintf '%s' \"$1\"\n")
	argsTool := newCLITool(root, cliSpec{bin: "gh"}, func(string) (string, error) { return argsExecutable, nil })
	argument := "; touch " + filepath.Join(root, "injected")
	argsResult := argsTool.Run(context.Background(), map[string]any{"args": []any{argument}})
	if argsResult.IsError || !strings.Contains(argsResult.Output, argument) {
		t.Fatalf("CLI argument was not passed verbatim: %#v", argsResult)
	}
	if _, err := os.Stat(filepath.Join(root, "injected")); !os.IsNotExist(err) {
		t.Fatalf("CLI argument was interpreted by a shell: %v", err)
	}
	timeoutExecutable := filepath.Join(binDir, "slow-gh")
	writeExecutable(t, timeoutExecutable, "#!/bin/sh\nexec sleep 5\n")
	timeoutTool := newCLITool(root, cliSpec{bin: "gh"}, func(string) (string, error) { return timeoutExecutable, nil })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	timedOut := timeoutTool.Run(ctx, map[string]any{"args": []any{"version"}})
	if !timedOut.IsError || !strings.Contains(timedOut.Output, "timed out") {
		t.Fatalf("expected context timeout, got %#v", timedOut)
	}
	notInstalled := newCLITool(root, cliSpec{bin: "gh"}, func(string) (string, error) { return "", errors.New("missing") })
	if got := notInstalled.Run(context.Background(), map[string]any{"args": []any{}}); !got.IsError || !strings.Contains(got.Output, "not on PATH") {
		t.Fatalf("expected missing CLI rejection, got %#v", got)
	}
	if got := runCLI(context.Background(), root, "sh", nil, lookPath); !got.IsError || !strings.Contains(got.Output, "not allow-listed") {
		t.Fatalf("expected non-allow-listed CLI rejection, got %#v", got)
	}
}

func TestCLIApprovalHeuristics(t *testing.T) {
	cases := []struct {
		bin  string
		args []string
		want bool
	}{
		{bin: "aws", args: []string{"ec2", "describe-instances"}, want: false},
		{bin: "aws", args: []string{"ec2", "delete-vpc"}, want: true},
		{bin: "aws", args: []string{"secretsmanager", "get-secret-value"}, want: true},
		{bin: "aws", args: []string{"ssm", "get-parameter"}, want: true},
		{bin: "aws", args: []string{"ecr", "get-login-password"}, want: true},
		{bin: "aws", args: []string{"s3api", "get-object"}, want: true},
		{bin: "aws", args: []string{"kms", "decrypt"}, want: true},
		{bin: "aws", args: []string{"ec2", "describe-instances", "--with-decryption"}, want: true},
		{bin: "kubectl", args: []string{"get", "pods"}, want: false},
		{bin: "kubectl", args: []string{"delete", "pod", "api"}, want: true},
		{bin: "kubectl", args: []string{"get", "secrets"}, want: true},
		{bin: "kubectl", args: []string{"get", "pods,secrets"}, want: true},
		{bin: "kubectl", args: []string{"describe", "secret/x"}, want: true},
		{bin: "kubectl", args: []string{"get", "-f", "x.yaml"}, want: true},
		{bin: "kubectl", args: []string{"get", "--filename=x.yaml"}, want: true},
		{bin: "kubectl", args: []string{"get", "-k", "overlays/prod"}, want: true},
		{bin: "kubectl", args: []string{"get", "--kustomize=overlays/prod"}, want: true},
		{bin: "kubectl", args: []string{"get", "--raw=/api"}, want: true},
		{bin: "gh", args: []string{"pr", "view", "42"}, want: false},
		{bin: "gh", args: []string{"pr", "merge", "42"}, want: true},
		{bin: "terraform", args: []string{"plan"}, want: true},
		{bin: "terraform", args: []string{"apply"}, want: true},
		{bin: "terraform", args: []string{"show"}, want: false},
		{bin: "terraform", args: []string{"show", "-json"}, want: false},
		{bin: "redis-cli", args: []string{"get", "app:key"}, want: false},
		{bin: "redis-cli", args: []string{"del", "app:key"}, want: true},
		{bin: "gcloud", args: []string{"compute", "instances", "list"}, want: true},
		{bin: "jq", args: []string{"."}, want: true},
		{bin: "pup", args: []string{"a"}, want: true},
	}
	for _, test := range cases {
		t.Run(test.bin+"/"+strings.Join(test.args, "_"), func(t *testing.T) {
			if got := cliNeedsApproval(test.bin, test.args); got != test.want {
				t.Fatalf("cliNeedsApproval(%q, %v) = %t, want %t", test.bin, test.args, got, test.want)
			}
		})
	}
}

func testRegistry(t *testing.T, root string, lookPath LookPathFunc) *Registry {
	t.Helper()
	if lookPath == nil {
		lookPath = func(string) (string, error) { return "", errors.New("not installed") }
	}
	registry, err := NewRegistry(root, lookPath)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func mustLookup(t *testing.T, registry *Registry, name string) Tool {
	t.Helper()
	tool, ok := registry.Lookup(name)
	if !ok {
		t.Fatalf("tool %q was not registered", name)
	}
	return tool
}

func writeExecutable(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
		t.Fatal(err)
	}
}
