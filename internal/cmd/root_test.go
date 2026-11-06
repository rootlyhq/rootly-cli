package cmd

import (
	"testing"

	"github.com/rootlyhq/rootly-cli/internal/buildinfo"
)

func TestSetVersionInfoSetsUserAgent(t *testing.T) {
	previousVersion, previousCommit, previousDate := version, commit, date
	previousBuildVersion, previousRootVersion := buildinfo.Version, rootCmd.Version
	t.Cleanup(func() {
		SetVersionInfo(previousVersion, previousCommit, previousDate)
		buildinfo.Version, rootCmd.Version = previousBuildVersion, previousRootVersion
	})
	SetVersionInfo("1.2.3", "abc123", "today")
	if got := buildinfo.UserAgent(); got != "rootly-cli/1.2.3" {
		t.Fatalf("User-Agent = %q", got)
	}
	if rootCmd.Version != "1.2.3" {
		t.Fatalf("root command version = %q", rootCmd.Version)
	}
}
