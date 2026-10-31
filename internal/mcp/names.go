package mcp

import (
	"fmt"
	"strings"
)

const (
	ClientToolPrefix  = "local_"
	maxClientToolName = 60
	maxServerNamePart = 20
)

func sanitizeNamePart(value string) string {
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			builder.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(builder.String(), "_")
}

// ClientToolNamer builds unique `local_<server>_<tool>` names matching
// ^local_[a-z0-9_]{1,60}$ and remembers the original server/tool.
type ClientToolNamer struct {
	used map[string]bool
}

func NewClientToolNamer() *ClientToolNamer {
	return &ClientToolNamer{used: map[string]bool{}}
}

func (n *ClientToolNamer) Reserve(name string) {
	name = strings.TrimPrefix(name, ClientToolPrefix)
	if name != "" {
		n.used[name] = true
	}
}

func (n *ClientToolNamer) Name(server, tool string) string {
	serverPart := sanitizeNamePart(server)
	if len(serverPart) > maxServerNamePart {
		serverPart = strings.TrimRight(serverPart[:maxServerNamePart], "_")
	}
	if serverPart == "" {
		serverPart = "server"
	}
	toolPart := sanitizeNamePart(tool)
	if toolPart == "" {
		toolPart = "tool"
	}
	body := serverPart + "_" + toolPart
	if len(body) > maxClientToolName {
		body = strings.TrimRight(body[:maxClientToolName], "_")
	}
	candidate := body
	for suffix := 2; n.used[candidate]; suffix++ {
		tail := fmt.Sprintf("_%d", suffix)
		trimmed := body
		if len(trimmed)+len(tail) > maxClientToolName {
			trimmed = strings.TrimRight(trimmed[:maxClientToolName-len(tail)], "_")
		}
		candidate = trimmed + tail
	}
	n.used[candidate] = true
	return ClientToolPrefix + candidate
}
