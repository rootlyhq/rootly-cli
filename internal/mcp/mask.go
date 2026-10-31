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

var secretArgName = regexp.MustCompile(`(?i)(token|secret|password|passwd|api[-_]?key|apikey|auth|credential|bearer)`)

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
			masked[index] = Mask
			hideNext = false
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
