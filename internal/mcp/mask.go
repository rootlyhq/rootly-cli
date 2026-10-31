package mcp

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
)

const Mask = "*****"

// MaskedConfig hides env/header values and URL credentials for display.
func (c ServerConfig) Masked() ServerConfig {
	masked := c
	masked.Args = maskArgs(c.Args)
	masked.URL = maskURL(c.URL)
	if len(c.Env) > 0 {
		masked.Env = make(map[string]string, len(c.Env))
		for key := range c.Env {
			masked.Env[key] = Mask
		}
	}
	if len(c.Headers) > 0 {
		masked.Headers = make(map[string]string, len(c.Headers))
		for key := range c.Headers {
			masked.Headers[key] = Mask
		}
	}
	return masked
}

func maskURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return raw
	}
	if parsed.User != nil {
		parsed.User = url.User(Mask)
	}
	if parsed.RawQuery != "" {
		query := parsed.Query()
		for key := range query {
			query.Set(key, Mask)
		}
		parsed.RawQuery = query.Encode()
	}
	return strings.ReplaceAll(parsed.String(), url.QueryEscape(Mask), Mask)
}

// Origin is a one-line, secret-free description of where a server runs.
func (c ServerConfig) Origin() string {
	if c.Transport() == TransportStdio {
		return strings.TrimSpace(c.Command + " " + strings.Join(maskArgs(c.Args), " "))
	}
	return maskURL(c.URL)
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// MaskedPairs renders KEY=***** pairs in key order.
func MaskedPairs(values map[string]string, separator string) []string {
	pairs := make([]string, 0, len(values))
	for _, key := range sortedKeys(values) {
		pairs = append(pairs, key+separator+Mask)
	}
	return pairs
}

var secretArgName = regexp.MustCompile(`(?i)(token|secret|password|passwd|api[-_]?key|apikey|auth|credential|bearer|access[-_]?key|private[-_]?key)`)
var secretCommandArgument = regexp.MustCompile(`(?i)(--?[\w-]*(?:token|secret|password|passwd|api[-_]?key|apikey|auth|credential|access[-_]?key|private[-_]?key)[\w-]*(?:=|\s+)|\b[A-Z_]*(?:TOKEN|SECRET|PASSWORD|PASSWD|API_KEY|ACCESS_KEY|PRIVATE_KEY)[A-Z_]*=)("[^"]*"|'[^']*'|[^\s;&|]+)`)
var secretEnvironmentArgument = regexp.MustCompile(`(?i)^[A-Z_]*(?:TOKEN|SECRET|PASSWORD|PASSWD|API_KEY|ACCESS_KEY|PRIVATE_KEY)[A-Z_]*=`)
var bearerCommandValue = regexp.MustCompile(`(?i)\bBearer\s+[^\s"';&|]+`)

// maskArgs hides values of secret-looking flags (--token=x, --api-key x)
// and inline Bearer tokens.
func maskArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}
	masked := make([]string, len(args))
	hideNext := false
	for index, arg := range args {
		switch {
		case hideNext:
			if containsShellCode(arg) {
				masked[index] = arg
			} else {
				masked[index] = Mask
			}
			hideNext = false
		case containsShellCode(arg):
			masked[index] = arg
		case secretEnvironmentArgument.MatchString(arg):
			name, _, _ := strings.Cut(arg, "=")
			masked[index] = name + "=" + Mask
		case strings.HasPrefix(arg, "-") && strings.Contains(arg, "="):
			name, _, _ := strings.Cut(arg, "=")
			if secretArgName.MatchString(name) {
				masked[index] = name + "=" + Mask
			} else {
				masked[index] = arg
			}
		case strings.HasPrefix(arg, "-") && secretArgName.MatchString(arg):
			masked[index] = arg
			hideNext = true
		case strings.HasPrefix(strings.ToLower(arg), "bearer "):
			masked[index] = arg[:len("bearer ")] + Mask
		default:
			masked[index] = arg
		}
	}
	return masked
}

func MaskCommandArgs(args []string) []string {
	return maskArgs(args)
}

func MaskCommand(command string) string {
	command = secretCommandArgument.ReplaceAllStringFunc(command, func(argument string) string {
		parts := secretCommandArgument.FindStringSubmatch(argument)
		if containsShellCode(parts[2]) {
			return argument
		}
		return parts[1] + Mask
	})
	return bearerCommandValue.ReplaceAllStringFunc(command, func(argument string) string {
		if containsShellCode(argument) {
			return argument
		}
		return "Bearer " + Mask
	})
}

func MaskToolArguments(args map[string]any) map[string]any {
	if args == nil {
		return nil
	}
	masked := make(map[string]any, len(args))
	for key, value := range args {
		masked[key] = maskToolArgument(key, value)
	}
	return masked
}

func maskToolArgument(key string, value any) any {
	if text, ok := value.(string); ok && containsShellCode(text) {
		return text
	}
	if secretArgName.MatchString(key) {
		return Mask
	}
	switch item := value.(type) {
	case map[string]any:
		return MaskToolArguments(item)
	case []string:
		if key == "args" {
			return maskArgs(item)
		}
		masked := make([]string, len(item))
		for index, text := range item {
			masked[index] = MaskCommand(text)
		}
		return masked
	case []any:
		if key == "args" {
			args := make([]string, len(item))
			for index, argument := range item {
				if text, ok := argument.(string); ok {
					args[index] = text
					continue
				}
				args[index] = Mask
			}
			return maskArgs(args)
		}
		masked := make([]any, len(item))
		for index, nested := range item {
			masked[index] = maskToolArgument("", nested)
		}
		return masked
	case string:
		return MaskCommand(item)
	default:
		return value
	}
}

func containsShellCode(value string) bool {
	return strings.Contains(value, "$(") || strings.ContainsRune(value, '`') || strings.Contains(value, "${")
}
