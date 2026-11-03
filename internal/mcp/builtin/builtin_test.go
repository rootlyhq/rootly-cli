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

func TestSensitivePathComponentsAreDeniedForReadAndGlob(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{".env.private/creds.json", "keys.pem/x"} {
		fullPath := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte("sensitive"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	registry := testRegistry(t, root, nil)
	readFile := mustLookup(t, registry, "local_read_file")
	for _, path := range []string{".env.private/creds.json", "keys.pem/x"} {
		result := readFile.Run(context.Background(), map[string]any{"path": path})
		if !result.IsError || !strings.Contains(result.Output, "path is denied") {
			t.Errorf("read_file did not deny %q: %#v", path, result)
		}
	}
	glob := mustLookup(t, registry, "local_glob")
	result := glob.Run(context.Background(), map[string]any{"pattern": "**/*"})
	if strings.Contains(result.Output, ".env.private") || strings.Contains(result.Output, "keys.pem") {
		t.Fatalf("glob returned children of denied directories: %q", result.Output)
	}
	for _, path := range []string{".env.private", "keys.pem"} {
		result := glob.Run(context.Background(), map[string]any{"pattern": "**/*", "path": path})
		if !result.IsError || !strings.Contains(result.Output, "path is denied") {
			t.Errorf("glob did not deny directory %q: %#v", path, result)
		}
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

func TestFilesystemSearchUsesSymlinkPath(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "real.txt"), []byte("needle\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.txt", filepath.Join(root, "alias.txt")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	registry := testRegistry(t, root, nil)
	glob := mustLookup(t, registry, "local_glob").Run(context.Background(), map[string]any{"pattern": "alias*"})
	if glob.Output != "alias.txt" {
		t.Fatalf("glob output = %q, want alias.txt", glob.Output)
	}
	grep := mustLookup(t, registry, "local_grep").Run(context.Background(), map[string]any{
		"pattern":     "needle",
		"output_mode": "content",
		"glob":        "alias.txt",
	})
	if grep.Output != "alias.txt:1:needle" {
		t.Fatalf("grep output = %q, want alias.txt:1:needle", grep.Output)
	}
	listing := mustLookup(t, registry, "local_list_dir").Run(context.Background(), map[string]any{})
	if !strings.Contains(listing.Output, "alias.txt") {
		t.Fatalf("directory listing omitted in-root symlink: %q", listing.Output)
	}
}

func TestOpenRootFileRejectsSymlinkToOutsideParent(t *testing.T) {
	rootPath := t.TempDir()
	outsidePath := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsidePath, "secret.txt"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsidePath, filepath.Join(rootPath, "parent")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	file, err := openRootFile(root, filepath.Join("parent", "secret.txt"))
	if err == nil {
		_ = file.Close()
		t.Fatal("rooted open succeeded through a symlink to an outside parent")
	}
}

func TestGrepCapsMatchesWithinOneFile(t *testing.T) {
	root := t.TempDir()
	content := strings.Repeat("match\n", maxSearchItems+100)
	if err := os.WriteFile(filepath.Join(root, "many.txt"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result := mustLookup(t, testRegistry(t, root, nil), "local_grep").Run(context.Background(), map[string]any{
		"pattern":     "match",
		"output_mode": "content",
	})
	matches := strings.Split(result.Output, "\n")
	if len(matches) != maxSearchItems {
		t.Fatalf("got %d matches, want %d", len(matches), maxSearchItems)
	}
	if !strings.HasSuffix(matches[maxSearchItems-1], ":200:match") || strings.Contains(result.Output, ":201:match") {
		t.Fatalf("grep did not stop at the 200th match: %q", matches[maxSearchItems-1])
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

func TestShellTimeoutClampsSeconds(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  time.Duration
	}{
		{name: "too large", value: int64(1 << 62), want: maxShellTimeout},
		{name: "below minimum", value: int64(0), want: time.Second},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := shellTimeout(map[string]any{"timeout_seconds": test.value}); got != test.want {
				t.Fatalf("shellTimeout() = %s, want %s", got, test.want)
			}
		})
	}
}

func TestRunCommandWaitDelayForBackgroundedChild(t *testing.T) {
	started := time.Now()
	result := runCommand(
		context.Background(),
		t.TempDir(),
		"bash",
		[]string{"-c", "sleep 30 & echo hi"},
		10*time.Second,
		maxShellOutput,
		"bash",
	)
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("command took %s with a backgrounded child", elapsed)
	}
	if result.IsError {
		t.Fatalf("successful shell with a backgrounded child was marked as an error: %q", result.Output)
	}
	if !strings.Contains(result.Output, "(exit 0)") {
		t.Fatalf("backgrounded command output did not report exit 0: %q", result.Output)
	}
	if !strings.Contains(result.Output, "hi") {
		t.Fatalf("backgrounded command output = %q, want it to include hi", result.Output)
	}
	if !strings.Contains(result.Output, "[background process still running; output detached]") {
		t.Fatalf("backgrounded command output omitted detached-output note: %q", result.Output)
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

func TestBashRedactsShortSensitiveEnvironmentValues(t *testing.T) {
	t.Setenv("DB_PASSWORD", "abc1")
	t.Setenv("FEATURE_TOKEN_ENABLED", "true")
	bash := mustLookup(t, testRegistry(t, t.TempDir(), nil), "local_bash")
	result := bash.Run(context.Background(), map[string]any{
		"command": `printf '%s\n' "$DB_PASSWORD" "$FEATURE_TOKEN_ENABLED"`,
	})
	if result.IsError {
		t.Fatalf("bash command failed: %#v", result)
	}
	if strings.Contains(result.Output, "abc1") || !strings.Contains(result.Output, "[REDACTED]") {
		t.Fatalf("short sensitive environment value was not redacted: %q", result.Output)
	}
	if !strings.Contains(result.Output, "true") {
		t.Fatalf("common boolean value was redacted: %q", result.Output)
	}
}

func TestSensitiveEnvironmentValuesSkipsCommonFlags(t *testing.T) {
	for _, value := range []string{"true", "FALSE", "yes", "NO", "none", "NULL"} {
		t.Run(value, func(t *testing.T) {
			secrets := sensitiveEnvironmentValues([]string{"FEATURE_TOKEN_ENABLED=" + value})
			if len(secrets) != 0 {
				t.Fatalf("common flag value was marked sensitive: %#v", secrets)
			}
		})
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
		{bin: "aws", args: []string{"--profile", "get-prod", "ec2", "terminate-instances"}, want: true},
		{bin: "aws", args: []string{"--region", "us-east-1", "ec2", "describe-instances"}, want: false},
		{bin: "aws", args: []string{"--weird", "x", "ec2", "describe-instances"}, want: true},
		{bin: "aws", args: []string{"--profile"}, want: true},
		{bin: "aws", args: []string{"ec2", "delete-vpc"}, want: true},
		{bin: "aws", args: []string{"secretsmanager", "get-secret-value"}, want: true},
		{bin: "aws", args: []string{"ssm", "get-parameter"}, want: true},
		{bin: "aws", args: []string{"ecr", "get-login-password"}, want: true},
		{bin: "aws", args: []string{"s3api", "get-object"}, want: true},
		{bin: "aws", args: []string{"kms", "decrypt"}, want: true},
		{bin: "aws", args: []string{"ec2", "describe-instances", "--with-decryption"}, want: true},
		{bin: "kubectl", args: []string{"get", "pods"}, want: false},
		{bin: "kubectl", args: []string{"get", "pods", "--kubeconfig"}, want: true},
		{bin: "kubectl", args: []string{"get", "pods", "--kubeconfig=/tmp/config"}, want: true},
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
		{bin: "gh", args: []string{"pr", "view", "42", "--web"}, want: true},
		{bin: "gh", args: []string{"pr", "view", "42", "-w"}, want: true},
		{bin: "gh", args: []string{"issue", "list", "--web=1"}, want: true},
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

func TestKubectlKubeconfigInsideRootNeedsApproval(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config")
	if err := os.WriteFile(configPath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	outsidePath := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(outsidePath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", outsidePath+string(os.PathListSeparator)+configPath)
	kubectl := newCLITool(root, cliSpec{bin: "kubectl"}, func(string) (string, error) { return "", errors.New("unused") })
	args := map[string]any{"args": []any{"get", "pods"}}
	if !kubectl.NeedsApproval(args) {
		t.Fatal("KUBECONFIG entry inside the working directory must require approval")
	}
	t.Setenv("KUBECONFIG", "")
	if kubectl.NeedsApproval(args) {
		t.Fatal("plain kubectl get pods should not require approval")
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
