package buildinfo

import "testing"

func TestUserAgent(t *testing.T) {
	previous := Version
	t.Cleanup(func() { Version = previous })
	Version = "1.2.3"
	if got := UserAgent(); got != "rootly-cli/1.2.3" {
		t.Fatalf("UserAgent() = %q", got)
	}
}
