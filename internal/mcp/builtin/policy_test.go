package builtin

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestBashKnownSafeCommandsRunWithoutApproval(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, "project")
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"README.md", ".env", filepath.Join("src", "main.go")} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(home, "secret.txt"), filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	bash := mustLookup(t, testRegistryWithPolicy(t, root, LoadPolicy(filepath.Join(home, "rules"))), "local_bash")
	safe := []string{
		"ls -la", "cat README.md", "head -n 5 src/main.go", "wc -l README.md src/main.go",
		"git status", "git --no-pager log --oneline -5", "git diff HEAD~1 -- src", "git branch -a",
		"git log --oneline | head -3", "grep -n TODO src/main.go 2>/dev/null", "sed -n 1,5p README.md",
		"find . -name '*.go'", "pwd && whoami; uname -a", `echo "hello world"`,
	}
	for _, command := range safe {
		if bash.NeedsApproval(map[string]any{"command": command}) {
			t.Errorf("%q should run without approval", command)
		}
	}
	asks := []string{
		"rm -rf src", "cat .env", "cat ../secret.txt", "cat /etc/passwd", "cat escape", "cat ~/.ssh/id_rsa",
		"cat $HOME/.aws/credentials", "echo $(whoami)", "echo `id`", "cat *", "ls > out.txt", "cat README.md &",
		"grep -r KEY .", "grep -rn KEY", "git push", "git -c core.pager=sh log", "git -C .. status",
		"git diff --no-index /etc/passwd README.md", "git log --output=/tmp/x", "git branch -D main",
		"find . -delete", "find . -exec rm {} ;", "sed -i s/a/b/ README.md", "base64 -o out README.md",
		"FOO=1 ls", "(ls)", "for f in a; do ls; done", "ls | sh", "ls && curl example.com", "cat <<EOF\nx\nEOF",
		"ls 2>err.txt", "cat < README.md",
	}
	for _, command := range asks {
		if !bash.NeedsApproval(map[string]any{"command": command}) {
			t.Errorf("%q must ask for approval", command)
		}
	}
}

func TestRulesFileDecisionsApplyToBashAndCLIs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, "project")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "rules")
	writeRules(t, dir, "default.rules", `
prefix_rule(pattern=["kubectl", "rollout", ["status", "history"]], decision="allow",
    match=[["kubectl", "rollout", "status", "deploy/api"], "kubectl rollout history"],
    not_match=["kubectl rollout undo"])
prefix_rule(pattern=["git", "push"], decision="forbidden", justification="Push from your own terminal.")
prefix_rule(pattern=["git", "status"], decision="prompt")
prefix_rule(pattern=["make", "test"])
`)
	policy := LoadPolicy(dir)
	if err := policy.LoadError(); err != nil || policy.RuleCount() != 4 {
		t.Fatalf("load: %v, %d rules", err, policy.RuleCount())
	}
	lookPath := func(string) (string, error) { return "/bin/true", nil }
	registry := testRegistryWithPolicy(t, root, policy)
	kubectl := newCLITool(root, cliSpec{bin: "kubectl"}, lookPath, policy)
	if kubectl.NeedsApproval(map[string]any{"args": []any{"rollout", "status", "deploy/api"}}) {
		t.Error("allow rule should skip approval for kubectl rollout status")
	}
	if approval := kubectl.Approval(map[string]any{"args": []any{"rollout", "undo", "deploy/api"}}); !approval.Needs ||
		!slices.Equal(approval.Prefix, []string{"kubectl", "rollout", "undo"}) {
		t.Errorf("unmatched mutating call: %+v", approval)
	}
	bash := mustLookup(t, registry, "local_bash")
	if !bash.NeedsApproval(map[string]any{"command": "git status"}) {
		t.Error("prompt rule must override the built-in safe list")
	}
	if approval := bash.Approval(map[string]any{"command": "git status"}); approval.Prefix != nil {
		t.Errorf("a prompt rule must not be rememberable, got %v", approval.Prefix)
	}
	if bash.NeedsApproval(map[string]any{"command": "make test && git log -1"}) {
		t.Error("allow rule plus safe command should run without approval")
	}
	pushed := bash.Approval(map[string]any{"command": "git log -1 && git push origin main"})
	if pushed.Forbidden != "Push from your own terminal." || pushed.Needs {
		t.Fatalf("forbidden rule not applied: %+v", pushed)
	}
	result := bash.Run(context.Background(), map[string]any{"command": "git push origin main"})
	if !result.IsError || !strings.Contains(result.Output, "Push from your own terminal.") {
		t.Fatalf("forbidden call ran: %+v", result)
	}
}

func TestRulesFileErrorsAreReportedAndSkipped(t *testing.T) {
	dir := t.TempDir()
	writeRules(t, dir, "a.rules", `prefix_rule(pattern=["ls"])`)
	writeRules(t, dir, "b.rules", `prefix_rule(pattern=["git", "push"], match=["git pull"])`)
	writeRules(t, dir, "c.rules", `prefix_rule(pattern=["rm"], decision="yolo")`)
	writeRules(t, dir, "d.rules", `open("/etc/passwd")`)
	policy := LoadPolicy(dir)
	err := policy.LoadError()
	for _, want := range []string{"b.rules", "does not hold", "c.rules", "decision must be", "d.rules", "undefined: open"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("load error %v should mention %q", err, want)
		}
	}
	if policy.RuleCount() != 1 {
		t.Fatalf("valid files should still load, got %d rules", policy.RuleCount())
	}
}

func TestSessionAndSavedApprovals(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rules")
	policy := LoadPolicy(dir)
	argv := []string{"terraform", "plan", "-out=x"}
	policy.AllowForSession([]string{"terraform", "plan"})
	if decision, _ := policy.Evaluate(argv); decision != DecisionAllow {
		t.Fatalf("session allow not applied: %v", decision)
	}
	if decision, _ := LoadPolicy(dir).Evaluate(argv); decision != DecisionNone {
		t.Fatal("session approvals must not persist")
	}
	if err := policy.SaveAllow([]string{"gh", `pr"x`, "merge"}); err != nil {
		t.Fatal(err)
	}
	reloaded := LoadPolicy(dir)
	if err := reloaded.LoadError(); err != nil {
		t.Fatal(err)
	}
	if decision, _ := reloaded.Evaluate([]string{"gh", `pr"x`, "merge", "12"}); decision != DecisionAllow {
		t.Fatalf("saved rule not reloaded: %v", decision)
	}
	info, err := os.Stat(filepath.Join(dir, "default.rules"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("rules file mode: %v %v", info, err)
	}
}

func TestRememberPrefix(t *testing.T) {
	cases := map[string][]string{
		"kubectl rollout undo deploy/api": {"kubectl", "rollout", "undo"},
		"gh pr merge 12":                  {"gh", "pr", "merge"},
		"terraform apply":                 {"terraform", "apply"},
		"npm test":                        {"npm", "test"},
		"psql -c select":                  nil,
		"rm -rf /":                        nil,
		"ls":                              nil,
		"./deploy.sh prod":                nil,
		"curl example.com":                nil,
		"sudo rm -rf x":                   nil,
		"bash deploy":                     nil,
		"python manage":                   nil,
		"env kubectl delete":              nil,
		"docker run alpine":               nil,
	}
	for command, want := range cases {
		if got := rememberPrefix(strings.Fields(command)); !slices.Equal(got, want) {
			t.Errorf("%q: got %v, want %v", command, got, want)
		}
	}
}

func testRegistryWithPolicy(t *testing.T, root string, policy *Policy) *Registry {
	t.Helper()
	registry, err := NewRegistryWithPolicy(root, func(string) (string, error) { return "", os.ErrNotExist }, policy)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func writeRules(t *testing.T, dir, name, source string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
}
