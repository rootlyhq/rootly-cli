package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxCLIOutput    = 12_000
	maxShellOutput  = 20_000
	defaultTimeout  = 30 * time.Second
	maxShellTimeout = 600 * time.Second
)

type cliSpec struct {
	bin         string
	label       string
	description string
}

type boundedBuffer struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

var cliRegistry = []cliSpec{
	{bin: "gh", label: "GitHub CLI", description: "Query and manage GitHub pull requests, issues, checks, runs, releases, and repositories."},
	{bin: "kubectl", label: "kubectl", description: "Inspect and manage Kubernetes resources, logs, events, and workloads."},
	{bin: "kubectx", label: "kubectx", description: "Show or switch Kubernetes contexts."},
	{bin: "aws", label: "AWS CLI", description: "Query and manage AWS resources using the configured local credentials."},
	{bin: "gcloud", label: "gcloud", description: "Query and manage Google Cloud resources using the configured local credentials."},
	{bin: "datadog", label: "Datadog CLI", description: "Query and manage Datadog monitors, metrics, events, and logs."},
	{bin: "dog", label: "Datadog dogshell", description: "Run Datadog dogshell commands."},
	{bin: "pup", label: "pup", description: "Parse and filter HTML from stdin using CSS selectors."},
	{bin: "jq", label: "jq", description: "Filter and transform JSON from stdin."},
	{bin: "psql", label: "psql", description: "Run SQL against the configured PostgreSQL server."},
	{bin: "redis-cli", label: "redis-cli", description: "Query Redis using the configured local credentials."},
	{bin: "terraform", label: "Terraform", description: "Inspect Terraform plans and state; mutating actions require approval."},
	{bin: "sentry-cli", label: "Sentry CLI", description: "Query and manage Sentry releases and issues."},
}

var sensitiveEnvironmentName = regexp.MustCompile(`(?i)(TOKEN|SECRET|PASSWORD|PASSWD|KEY|CREDENTIAL|AUTH|SESSION|COOKIE)`)

func newCLITool(root string, spec cliSpec, lookPath LookPathFunc, policy *Policy) Tool {
	name := "local_" + strings.ReplaceAll(spec.bin, "-", "_")
	return localTool{
		name:        name,
		description: fmt.Sprintf("%s (%q) runs on this machine with its local credentials. Pass argument tokens via `args`.", spec.label, spec.bin),
		schema:      jsonSchema(`{"type":"object","properties":{"args":{"type":"array","items":{"type":"string"},"description":"CLI argument tokens, passed without a shell."}},"required":["args"]}`),
		readOnly:    false,
		approve: func(args map[string]any) Approval {
			values, ok := stringSliceArg(args, "args")
			if !ok {
				return Approval{Needs: true}
			}
			argv := append([]string{spec.bin}, values...)
			builtinNeeds := cliNeedsApproval(spec.bin, values) || (spec.bin == "kubectl" && kubectlKubeconfigInsideRoot(root))
			return commandsApproval(policy, [][]string{argv}, func([]string) bool { return !builtinNeeds })
		},
		run: func(ctx context.Context, args map[string]any) Result {
			values, ok := stringSliceArg(args, "args")
			if !ok {
				return errorResult("args must be an array of strings")
			}
			return runCLI(ctx, root, spec.bin, values, lookPath)
		},
	}
}

func newBashTool(root string, policy *Policy) Tool {
	return localTool{
		name: "local_bash",
		description: "Run a shell command on this machine with its local credentials. Read-only commands (ls, cat, grep, head, git status/log/diff/show) " +
			"on files in the working directory run without approval; anything else asks first. Output and runtime are capped.",
		schema:   jsonSchema(`{"type":"object","properties":{"command":{"type":"string"},"timeout_seconds":{"type":"integer","minimum":1,"maximum":600}},"required":["command"]}`),
		readOnly: false,
		approve: func(args map[string]any) Approval {
			command, _ := stringArg(args, "command")
			commands, ok := shellCommands(command)
			if !ok {
				return Approval{Needs: true}
			}
			return commandsApproval(policy, commands, func(argv []string) bool { return isKnownSafeCommand(argv, root) })
		},
		run: func(ctx context.Context, args map[string]any) Result {
			command, ok := stringArg(args, "command")
			if !ok || strings.TrimSpace(command) == "" {
				return errorResult("command is required")
			}
			return runCommand(ctx, root, "bash", []string{"-c", hardenSafeGit(command, root)}, shellTimeout(args), maxShellOutput, "bash")
		},
	}
}

// commandsApproval combines user rules with the built-in read-only check:
// any forbidden command blocks the call; it runs unasked only when every
// command is allowed by a rule or known safe.
func commandsApproval(policy *Policy, commands [][]string, safe func([]string) bool) Approval {
	approval := Approval{}
	ruleAsks := false
	for _, argv := range commands {
		decision, justification := policy.Evaluate(argv)
		switch decision {
		case DecisionForbidden:
			if justification == "" {
				justification = strings.Join(argv, " ")
			}
			return Approval{Forbidden: justification}
		case DecisionPrompt:
			ruleAsks = true
			approval.Needs = true
		case DecisionNone:
			if !safe(argv) {
				approval.Needs = true
			}
		}
	}
	if approval.Needs && !ruleAsks && len(commands) == 1 {
		approval.Prefix = rememberPrefix(commands[0])
	}
	return approval
}

// rememberPrefix is the command plus up to two leading subcommands, e.g.
// ["kubectl", "rollout", "undo"]. Commands that start with a flag or a path
// get no prefix, so they can only be approved once, as do wrappers and
// interpreters whose prefix would cover arbitrary commands.
func rememberPrefix(argv []string) []string {
	if len(argv) < 2 || strings.Contains(argv[0], "/") || unrememberableCommands[argv[0]] {
		return nil
	}
	prefix := []string{argv[0]}
	for _, token := range argv[1:min(len(argv), 3)] {
		if token == "" || strings.HasPrefix(token, "-") || strings.ContainsAny(token, "/=.:@ ") {
			break
		}
		prefix = append(prefix, token)
	}
	if len(prefix) < 2 {
		return nil
	}
	return prefix
}

var unrememberableCommands = map[string]bool{
	"sudo": true, "su": true, "doas": true, "env": true, "xargs": true, "eval": true, "exec": true, "command": true,
	"builtin": true, "nohup": true, "nice": true, "time": true, "timeout": true, "watch": true, "ssh": true,
	"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true, "fish": true, "source": true,
	"python": true, "python3": true, "node": true, "ruby": true, "perl": true, "php": true, "deno": true, "bun": true,
	"npx": true, "uvx": true, "docker": true,
}

func shellTimeout(args map[string]any) time.Duration {
	maxSeconds := int64(maxShellTimeout / time.Second)
	seconds := int64(defaultTimeout / time.Second)
	switch value := args["timeout_seconds"].(type) {
	case int:
		seconds = int64(value)
	case int64:
		seconds = value
	case float64:
		switch {
		case value < 1:
			seconds = 1
		case value > float64(maxSeconds):
			seconds = maxSeconds
		default:
			seconds = int64(value)
		}
	}
	seconds = min(max(seconds, int64(1)), maxSeconds)
	return time.Duration(seconds) * time.Second
}

func runCLI(ctx context.Context, root, bin string, args []string, lookPath LookPathFunc) Result {
	if !isAllowedCLI(bin) {
		return errorResult("CLI not allow-listed: %s", bin)
	}
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	path, err := lookPath(bin)
	if err != nil {
		return errorResult("CLI not on PATH: %s", bin)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return errorResult("resolve CLI: %v", err)
	}
	return runCommand(ctx, root, absolute, args, defaultTimeout, maxCLIOutput, bin)
}

func runCommand(ctx context.Context, root, command string, args []string, timeout time.Duration, outputLimit int, displayName string) Result {
	commandContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(commandContext, command, args...)
	cmd.WaitDelay = 2 * time.Second
	cmd.Dir = root
	environment := os.Environ()
	cmd.Env = environment
	secrets := sensitiveEnvironmentValues(environment)
	bufferLimit := outputLimit * 4
	for _, secret := range secrets {
		bufferLimit = max(bufferLimit, outputLimit*4+len(secret))
	}
	stdout := &boundedBuffer{limit: bufferLimit}
	stderr := &boundedBuffer{limit: bufferLimit}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	exitCode := 0
	isError := err != nil
	backgroundOutputDetached := errors.Is(err, exec.ErrWaitDelay) && commandContext.Err() == nil
	if err != nil {
		exitCode = 1
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode = exitError.ExitCode()
		}
	}
	if backgroundOutputDetached {
		exitCode = cmd.ProcessState.ExitCode()
		isError = exitCode != 0
	}
	body := strings.TrimRight(stdout.String(), "\n")
	if stderrText := strings.TrimRight(stderr.String(), "\n"); stderrText != "" {
		if body != "" {
			body += "\n"
		}
		body += "[stderr]\n" + stderrText
	}
	if backgroundOutputDetached {
		if body != "" {
			body += "\n"
		}
		body += "[background process still running; output detached]"
	}
	header := fmt.Sprintf("$ %s\n(exit %d)", displayName, exitCode)
	if commandContext.Err() != nil {
		header += "\n[command timed out]"
	}
	if stdout.truncated || stderr.truncated {
		body += "\n[command output truncated]"
	}
	if body != "" {
		header += "\n\n" + body
	}
	return Result{Output: capOutput(redactEnvironmentValues(header, secrets), outputLimit), IsError: isError}
}

func sensitiveEnvironmentValues(environment []string) []string {
	secrets := make([]string, 0)
	for _, entry := range environment {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || !sensitiveEnvironmentName.MatchString(name) || utf8.RuneCountInString(value) < 4 {
			continue
		}
		switch strings.ToLower(value) {
		case "true", "false", "yes", "no", "none", "null":
			continue
		}
		secrets = append(secrets, value)
	}
	return secrets
}

func redactEnvironmentValues(output string, secrets []string) string {
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	for _, secret := range secrets {
		output = strings.ReplaceAll(output, secret, "[REDACTED]")
	}
	return output
}

func (b *boundedBuffer) Write(data []byte) (int, error) {
	remaining := b.limit - b.buffer.Len()
	if remaining > 0 {
		_, _ = b.buffer.Write(data[:min(len(data), remaining)])
	}
	if len(data) > remaining {
		b.truncated = true
	}
	return len(data), nil
}

func (b *boundedBuffer) String() string {
	return b.buffer.String()
}

func isAllowedCLI(bin string) bool {
	for _, spec := range cliRegistry {
		if spec.bin == bin {
			return true
		}
	}
	return false
}

func cliNeedsApproval(bin string, args []string) bool {
	lower := make([]string, len(args))
	for index := range args {
		lower[index] = strings.ToLower(args[index])
	}
	switch bin {
	case "gh":
		return ghNeedsApproval(lower)
	case "kubectl":
		return kubectlNeedsApproval(lower)
	case "aws":
		return awsNeedsApproval(lower)
	case "terraform":
		return len(lower) == 0 || lower[0] != "show"
	case "kubectx":
		return len(lower) > 0
	case "jq", "pup":
		return true
	case "redis-cli":
		return len(lower) == 0 || !containsCommand(lower[0], "get", "mget", "scan", "type", "exists", "ttl", "pttl", "info", "dbsize")
	case "sentry-cli":
		return len(lower) < 2 || !containsCommand(lower[0], "issues", "releases") ||
			!containsCommand(lower[1], "list", "info", "show")
	default:
		return true
	}
}

func ghNeedsApproval(args []string) bool {
	if len(args) < 2 || !containsCommand(args[0], "pr", "issue") || !containsCommand(args[1], "view", "list") {
		return true
	}
	for _, arg := range args {
		if arg == "--web" || arg == "-w" || strings.HasPrefix(arg, "--web=") {
			return true
		}
	}
	return false
}

func awsNeedsApproval(args []string) bool {
	_, action, ok := awsServiceOperation(args)
	if !ok {
		return true
	}
	for _, sensitive := range []string{"secret", "password", "token", "credential", "login", "key", "decrypt"} {
		if strings.Contains(action, sensitive) {
			return true
		}
	}
	if containsCommand(action, "get-object", "get-parameter", "get-parameters", "get-parameters-by-path") {
		return true
	}
	for _, arg := range args {
		if arg == "--with-decryption" {
			return true
		}
	}
	return !strings.HasPrefix(action, "describe-") &&
		!strings.HasPrefix(action, "list-") &&
		!strings.HasPrefix(action, "get-")
}

func awsServiceOperation(args []string) (service, operation string, ok bool) {
	positionals := make([]string, 0, 2)
	for index := 0; index < len(args) && len(positionals) < 2; index++ {
		arg := args[index]
		if !strings.HasPrefix(arg, "-") {
			positionals = append(positionals, arg)
			continue
		}
		option, _, hasValue := strings.Cut(arg, "=")
		if hasValue && awsValueOption(option) {
			continue
		}
		if awsBooleanOption(arg) {
			continue
		}
		if awsValueOption(arg) {
			if index+1 >= len(args) {
				return "", "", false
			}
			index++
			continue
		}
		return "", "", false
	}
	if len(positionals) < 2 {
		return "", "", false
	}
	return positionals[0], positionals[1], true
}

func awsValueOption(option string) bool {
	switch option {
	case "--profile", "--region", "--output", "--endpoint-url", "--query",
		"--cli-read-timeout", "--cli-connect-timeout", "--ca-bundle", "--color",
		"--cli-binary-format":
		return true
	default:
		return false
	}
}

func awsBooleanOption(option string) bool {
	switch option {
	case "--debug", "--no-verify-ssl", "--no-paginate", "--no-sign-request", "--no-cli-pager":
		return true
	default:
		return false
	}
}

func kubectlNeedsApproval(args []string) bool {
	for _, arg := range args {
		if arg == "--kubeconfig" || strings.HasPrefix(arg, "--kubeconfig=") {
			return true
		}
	}
	if len(args) == 0 || !containsCommand(args[0], "get", "describe", "logs") {
		return true
	}
	for _, arg := range args {
		if strings.Contains(arg, "secret") ||
			strings.HasPrefix(arg, "-f") ||
			strings.HasPrefix(arg, "--filename") ||
			strings.HasPrefix(arg, "-k") ||
			strings.HasPrefix(arg, "--kustomize") ||
			strings.HasPrefix(arg, "--raw") {
			return true
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		for _, part := range strings.FieldsFunc(strings.ToLower(arg), func(r rune) bool {
			return r == ',' || r == '/'
		}) {
			if part == "se" || part == "secret" || part == "secrets" ||
				strings.HasSuffix(part, ".secrets") ||
				strings.HasPrefix(part, "secrets.") ||
				strings.HasPrefix(part, "secret.") {
				return true
			}
		}
	}
	return false
}

func kubectlKubeconfigInsideRoot(root string) bool {
	rootPath, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	rootPath, err = filepath.EvalSymlinks(rootPath)
	if err != nil {
		return false
	}
	for _, configPath := range filepath.SplitList(os.Getenv("KUBECONFIG")) {
		if configPath == "" {
			continue
		}
		if !filepath.IsAbs(configPath) {
			configPath = filepath.Join(rootPath, configPath)
		}
		configPath, err = filepath.Abs(configPath)
		if err != nil {
			continue
		}
		configPath, err = filepath.EvalSymlinks(configPath)
		if err == nil && isWithin(rootPath, configPath) {
			return true
		}
	}
	return false
}

func containsCommand(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func stringSliceArg(args map[string]any, key string) ([]string, bool) {
	value, ok := args[key]
	if !ok {
		return nil, false
	}
	switch values := value.(type) {
	case []string:
		return append([]string(nil), values...), true
	case []any:
		result := make([]string, len(values))
		for index, value := range values {
			text, ok := value.(string)
			if !ok || strings.ContainsRune(text, '\x00') {
				return nil, false
			}
			result[index] = text
		}
		return result, true
	default:
		return nil, false
	}
}

func jsonSchema(value string) json.RawMessage {
	return json.RawMessage(value)
}
