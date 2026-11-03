package builtin

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	initTestRepo(t, root, "README.md", filepath.Join("src", "main.go"))
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
		"ls 2>err.txt", "cat < README.md", `cat '\etc\passwd'`,
		"git show HEAD:.env", "git show HEAD:README.md", "git show :README.md", "git cat-file -p HEAD:README.md",
		"git log -p -- .env", "git diff -- .env", "git diff .env", "git log -L1,2:README.md", "git log --full-diff -p -- src",
		"git log --show-signature", "git log src -n 5",
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
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("rules file mode: %v", info.Mode())
	}
}

func TestSaveAllowAfterFileWithoutTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	writeRules(t, dir, "default.rules", `prefix_rule(pattern=["make", "test"], decision="allow")`)
	if err := LoadPolicy(dir).SaveAllow([]string{"kubectl", "rollout"}); err != nil {
		t.Fatal(err)
	}
	reloaded := LoadPolicy(dir)
	if err := reloaded.LoadError(); err != nil || reloaded.RuleCount() != 2 {
		t.Fatalf("appended rule broke the file: %v, %d rules", err, reloaded.RuleCount())
	}
}

// hermeticGit points git at empty user and system config so runner-wide
// settings (for example a git-lfs filter) can't change what the tests see.
func hermeticGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	empty := t.TempDir()
	global := filepath.Join(empty, "gitconfig")
	if err := os.WriteFile(global, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", empty)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", global)
}

func runGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", append([]string{"-c", "user.email=e2e@example.com", "-c", "user.name=e2e"}, args...)...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// initTestRepo commits every file under root twice, so HEAD~1 exists.
func initTestRepo(t *testing.T, root string, files ...string) {
	t.Helper()
	hermeticGit(t)
	runGit(t, root, "init", "-q", "-b", "main")
	runGit(t, root, append([]string{"add", "--"}, files...)...)
	runGit(t, root, "commit", "-qm", "one")
	if err := os.WriteFile(filepath.Join(root, files[0]), []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "commit", "-qam", "two")
}

func TestHardenSafeGitRewritesAutoApprovedReads(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "a.go"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	initTestRepo(t, root, filepath.Join("src", "a.go"))
	config := "git " + gitHardeningConfig
	cases := map[string]string{
		"git status":                      config + " status",
		"git diff":                        config + " diff " + gitDiffHardening + " -- " + gitDeniedPathspecs,
		"git diff HEAD~1 src":             config + " diff " + gitDiffHardening + " HEAD~1 -- src " + gitDeniedPathspecs,
		"git diff HEAD~1 -- src":          config + " diff " + gitDiffHardening + " HEAD~1 -- src " + gitDeniedPathspecs,
		"git --no-pager show --stat HEAD": config + " --no-pager show " + gitDiffHardening + " --stat HEAD -- " + gitDeniedPathspecs,
		"git log --oneline | head -3":     config + " log " + gitDiffHardening + " --oneline -- " + gitDeniedPathspecs + " | head -3",
		"git log --follow -p -- src/a.go": config + " log " + gitDiffHardening + " --follow -p -- src/a.go",
		"git branch -a && git log -1":     config + " branch -a && " + config + " log " + gitDiffHardening + " -1 -- " + gitDeniedPathspecs,
		"git push":                        "git push",
		"rm -rf src; git diff":            "rm -rf src; " + config + " diff " + gitDiffHardening + " -- " + gitDeniedPathspecs,
		"echo $(git diff)":                "echo $(" + config + " diff " + gitDiffHardening + " -- " + gitDeniedPathspecs + ")",
	}
	for script, want := range cases {
		if got := hardenSafeGit(script, root); got != want {
			t.Errorf("hardenSafeGit(%q)\n got %q\nwant %q", script, got, want)
		}
	}
}

func TestGitDecidesWhichArgumentsAreRevisionsAndWhichArePaths(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "repo")
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"README.md", "gone.go", filepath.Join("src", "main.go")} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	initTestRepo(t, root, "README.md", "gone.go", filepath.Join("src", "main.go"))
	runGit(t, root, "branch", "src")
	if err := os.Remove(filepath.Join(root, "gone.go")); err != nil {
		t.Fatal(err)
	}
	readmeBlob := runGit(t, root, "rev-parse", "HEAD:README.md")
	bash := mustLookup(t, testRegistryWithPolicy(t, root, LoadPolicy(filepath.Join(home, "rules"))), "local_bash")
	config := "git " + gitHardeningConfig
	rewrites := map[string]string{
		"git diff gone.go":             config + " diff " + gitDiffHardening + " -- gone.go " + gitDeniedPathspecs,
		"git log HEAD~1..HEAD":         config + " log " + gitDiffHardening + " HEAD~1..HEAD -- " + gitDeniedPathspecs,
		"git diff HEAD~1...HEAD":       config + " diff " + gitDiffHardening + " HEAD~1...HEAD -- " + gitDeniedPathspecs,
		"git diff src --":              config + " diff " + gitDiffHardening + " src -- " + gitDeniedPathspecs,
		"git diff -- src":              config + " diff " + gitDiffHardening + " -- src " + gitDeniedPathspecs,
		"git log -n 3 src/main.go":     config + " log " + gitDiffHardening + " -n 3 -- src/main.go " + gitDeniedPathspecs,
		"git log --oneline main..HEAD": config + " log " + gitDiffHardening + " --oneline main..HEAD -- " + gitDeniedPathspecs,
	}
	for command, want := range rewrites {
		if bash.NeedsApproval(map[string]any{"command": command}) {
			t.Errorf("%q should run without approval", command)
			continue
		}
		if got := hardenSafeGit(command, root); got != want {
			t.Errorf("hardenSafeGit(%q)\n got %q\nwant %q", command, got, want)
		}
	}
	for _, command := range []string{"git diff gone.go", "git log HEAD~1..HEAD", "git diff src --"} {
		if result := bash.Run(context.Background(), map[string]any{"command": command}); result.IsError {
			t.Errorf("%q failed after hardening: %s", command, result.Output)
		}
	}
	for _, command := range []string{
		"git diff src", "git log src",
		"git show " + readmeBlob, "git show " + readmeBlob[:10], "git diff " + readmeBlob + ".." + readmeBlob,
		"git show HEAD^{tree}",
	} {
		if !bash.NeedsApproval(map[string]any{"command": command}) {
			t.Errorf("%q is ambiguous or names a blob/tree and must ask", command)
		}
	}
}

func TestAutoApprovedGitReadsCannotRunRepoProgramsOrLeakDeniedFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	if runtime.GOOS == "windows" {
		t.Skip("drivers below use POSIX shell commands")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-c", "user.email=e2e@example.com", "-c", "user.name=e2e"}, args...)...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	marker := filepath.Join(home, "pwned")
	git("init", "-q")
	write("README.md", "one\n")
	write(".env", "TOKEN=first\n")
	write(".gitattributes", "*.md diff=evil\n")
	git("add", "README.md", ".env", ".gitattributes")
	git("commit", "-qm", "init")
	write("README.md", "one\ntwo\n")
	write(".env", "TOKEN=leaked-secret\n")
	git("commit", "-qam", "two")
	write("README.md", "one\ntwo\nthree\n")
	git("config", "diff.evil.textconv", "touch "+marker+"; cat")
	git("config", "diff.external", "touch "+marker+";")
	git("config", "core.fsmonitor", "touch "+marker+"; true")
	bash := mustLookup(t, testRegistryWithPolicy(t, root, LoadPolicy(filepath.Join(home, "rules"))), "local_bash")
	for _, command := range []string{"git diff", "git show HEAD", "git log -p -2", "git status"} {
		args := map[string]any{"command": command}
		if bash.NeedsApproval(args) {
			t.Fatalf("%q should run without approval", command)
		}
		result := bash.Run(context.Background(), args)
		if result.IsError || strings.Contains(result.Output, "leaked-secret") || strings.Contains(result.Output, "TOKEN=") {
			t.Fatalf("%q: %+v", command, result)
		}
		if _, err := os.Stat(marker); err == nil {
			t.Fatalf("%q ran a repo-configured program", command)
		}
	}
	gitOut := func(args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = root
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
		return strings.TrimSpace(string(out))
	}
	envBlob, oldEnvBlob := gitOut("rev-parse", "HEAD:.env"), gitOut("rev-parse", "HEAD~1:.env")
	git("tag", "envblob", envBlob)
	for _, command := range []string{
		"git show " + envBlob, "git show " + envBlob[:12], "git diff " + oldEnvBlob + " " + envBlob,
		"git diff " + oldEnvBlob + ".." + envBlob, "git show HEAD^{tree}", "git show envblob", "git log -p " + envBlob,
	} {
		if !bash.NeedsApproval(map[string]any{"command": command}) {
			t.Errorf("%q names a blob or tree and must ask", command)
		}
	}
	for _, command := range []string{"git show HEAD~1", "git diff HEAD~1..HEAD", "git log -n 2 -p", "git show v9.9.9"} {
		if bash.NeedsApproval(map[string]any{"command": command}) {
			t.Errorf("%q names commits only and should not ask", command)
		}
	}
	git("config", "filter.evil.clean", "touch "+marker+"; cat")
	write(".gitattributes", "*.md diff=evil filter=evil\n")
	for _, command := range []string{"git diff", "git status"} {
		if !bash.NeedsApproval(map[string]any{"command": command}) {
			t.Errorf("%q must ask when the repo config defines a clean filter", command)
		}
	}
	if bash.NeedsApproval(map[string]any{"command": "git log -1"}) {
		t.Error("git log does not run filters and should not ask")
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

func TestSymlinkedWorkspaceRootIsComparedResolved(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.MkdirAll(filepath.Join(target, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"README.md": "x\n", ".env": "TOKEN=x\n", filepath.Join("src", "a.go"): "x\n", filepath.Join("..", "outside.txt"): "x\n",
	} {
		if err := os.WriteFile(filepath.Join(target, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(target, alias); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	if err := os.Symlink(filepath.Join(base, "outside.txt"), filepath.Join(target, "escape")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	initTestRepo(t, target, "README.md", filepath.Join("src", "a.go"))
	for _, root := range []string{alias, target} {
		inside := []string{"README.md", filepath.Join("src", "a.go"), "missing.txt", filepath.Join(alias, "README.md"), filepath.Join(target, "README.md")}
		for _, candidate := range inside {
			if !safeReadPath(root, candidate) {
				t.Errorf("root %s: %q is inside the workspace", root, candidate)
			}
		}
		outside := []string{
			"escape", ".env", filepath.Join("..", "outside.txt"), filepath.Join(base, "outside.txt"),
			filepath.Join(alias, ".env"), filepath.Join(alias, "escape"), filepath.Join(alias, "..", "outside.txt"),
		}
		for _, candidate := range outside {
			if safeReadPath(root, candidate) {
				t.Errorf("root %s: %q must stay outside the workspace", root, candidate)
			}
		}
		config := "git " + gitHardeningConfig
		want := config + " log " + gitDiffHardening + " --follow -p -- src/a.go"
		if got := hardenSafeGit("git log --follow -p -- src/a.go", root); got != want {
			t.Errorf("root %s: hardenSafeGit\n got %q\nwant %q", root, got, want)
		}
		if !isKnownSafeCommand([]string{"cat", "README.md"}, root) || isKnownSafeCommand([]string{"cat", "escape"}, root) {
			t.Errorf("root %s: cat classification changed through the symlinked root", root)
		}
	}
}
