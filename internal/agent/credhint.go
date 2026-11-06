package agent

import (
	"regexp"

	"github.com/rootlyhq/rootly-cli/internal/mcp"
)

const localCredsHint = "Refresh local creds, e.g. `aws sso login`"

var (
	expiredLocalCredsPattern = regexp.MustCompile(`(?i)(sso session .*(expired|invalid)|token (has|is) expired|expiredtoken|expired ?token|credentials? (have|has) expired|aws sso login|unable to locate credentials)`)
	deniedCredsPattern       = regexp.MustCompile(`(?i)(\b401\b|\b403\b|unauthori[sz]ed|forbidden|invalid[_ ]token|access denied)`)
)

// looksLikeExpiredCreds reports whether a local tool result resembles stale local credentials.
func looksLikeExpiredCreds(result mcp.CallResult) bool {
	if result.Output == userDeclinedOutput {
		return false
	}
	if expiredLocalCredsPattern.MatchString(result.Output) {
		return true
	}
	return result.IsError && deniedCredsPattern.MatchString(result.Output)
}
