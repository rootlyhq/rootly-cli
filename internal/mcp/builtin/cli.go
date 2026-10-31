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
	"strings"
	"time"
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

func newCLITool(root string, spec cliSpec, lookPath LookPathFunc) Tool {
	name := "local_" + strings.ReplaceAll(spec.bin, "-", "_")
	return localTool{
		name:        name,
		description: fmt.Sprintf("%s (%q) runs on this machine with its local credentials. Pass argument tokens via `args`.", spec.label, spec.bin),
		schema:      jsonSchema(`{"type":"object","properties":{"args":{"type":"array","items":{"type":"string"},"description":"CLI argument tokens, passed without a shell."}},"required":["args"]}`),
		readOnly:    false,
		needsApproval: func(args map[string]any) bool {
			values, ok := stringSliceArg(args, "args")
			return !ok || cliNeedsApproval(spec.bin, values)
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

func newBashTool(root string) Tool {
	return localTool{
		name:          "local_bash",
		description:   "Run a shell command on this machine with its local credentials. Always requires approval; output and runtime are capped.",
		schema:        jsonSchema(`{"type":"object","properties":{"command":{"type":"string"},"timeout_seconds":{"type":"integer","minimum":1,"maximum":600}},"required":["command"]}`),
		readOnly:      false,
		needsApproval: func(map[string]any) bool { return true },
		run: func(ctx context.Context, args map[string]any) Result {
			command, ok := stringArg(args, "command")
			if !ok || strings.TrimSpace(command) == "" {
				return errorResult("command is required")
			}
			timeoutSeconds := intArg(args, "timeout_seconds", int(defaultTimeout/time.Second))
			if timeoutSeconds < 1 {
				timeoutSeconds = int(defaultTimeout / time.Second)
			}
			timeout := min(time.Duration(timeoutSeconds)*time.Second, maxShellTimeout)
			return runCommand(ctx, root, "bash", []string{"-c", command}, timeout, maxShellOutput, "bash")
		},
	}
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
	cmd.Dir = root
	cmd.Env = os.Environ()
	stdout := &boundedBuffer{limit: outputLimit * 4}
	stderr := &boundedBuffer{limit: outputLimit * 4}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		exitCode = 1
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode = exitError.ExitCode()
		}
	}
	body := strings.TrimRight(stdout.String(), "\n")
	if stderrText := strings.TrimRight(stderr.String(), "\n"); stderrText != "" {
		if body != "" {
			body += "\n"
		}
		body += "[stderr]\n" + stderrText
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
	return Result{Output: capOutput(header, outputLimit), IsError: err != nil}
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
		return len(lower) < 2 || !containsCommand(lower[0], "pr", "issue") || !containsCommand(lower[1], "view", "list")
	case "kubectl":
		return len(lower) == 0 || !containsCommand(lower[0], "get", "describe", "logs")
	case "aws":
		if len(lower) < 2 {
			return true
		}
		action := lower[1]
		return !strings.HasPrefix(action, "describe-") && !strings.HasPrefix(action, "list-") && !strings.HasPrefix(action, "get-")
	case "terraform":
		return len(lower) == 0 || !containsCommand(lower[0], "plan", "show") || hasPlanOutputFlag(lower)
	case "kubectx":
		return len(lower) > 0
	case "jq", "pup":
		return false
	case "redis-cli":
		return len(lower) == 0 || !containsCommand(lower[0], "get", "mget", "scan", "type", "exists", "ttl", "pttl", "info", "dbsize")
	case "sentry-cli":
		return len(lower) < 2 || !containsCommand(lower[0], "issues", "releases") ||
			!containsCommand(lower[1], "list", "info", "show")
	default:
		return true
	}
}

func containsCommand(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func hasPlanOutputFlag(args []string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-out") {
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
