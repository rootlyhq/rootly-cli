package mcp

import (
	"os"
	"regexp"
)

var envReference = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:-([^}]*))?\}`)

// ExpandEnv replaces ${VAR} and ${VAR:-default}; an unset ${VAR} becomes "".
func ExpandEnv(value string, lookup func(string) (string, bool)) string {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	return envReference.ReplaceAllStringFunc(value, func(match string) string {
		parts := envReference.FindStringSubmatch(match)
		if current, ok := lookup(parts[1]); ok && (current != "" || parts[2] == "") {
			return current
		}
		return parts[3]
	})
}

func (c ServerConfig) Expanded(lookup func(string) (string, bool)) ServerConfig {
	expanded := ServerConfig{
		Type:              c.Type,
		Command:           ExpandEnv(c.Command, lookup),
		URL:               ExpandEnv(c.URL, lookup),
		BearerTokenEnvVar: c.BearerTokenEnvVar,
	}
	for _, arg := range c.Args {
		expanded.Args = append(expanded.Args, ExpandEnv(arg, lookup))
	}
	if len(c.Env) > 0 {
		expanded.Env = make(map[string]string, len(c.Env))
		for key, value := range c.Env {
			expanded.Env[key] = ExpandEnv(value, lookup)
		}
	}
	if len(c.Headers) > 0 {
		expanded.Headers = make(map[string]string, len(c.Headers))
		for key, value := range c.Headers {
			expanded.Headers[key] = ExpandEnv(value, lookup)
		}
	}
	return expanded
}
