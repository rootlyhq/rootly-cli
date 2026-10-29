package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestAgentAliasHelpAndHiddenRootHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)
	defer func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs([]string{})
	}()

	rootCmd.SetArgs([]string{"chat", "--help"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rootly chat --help returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "rootly agent") {
		t.Errorf("chat help does not show agent command: %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	rootCmd.SetArgs([]string{"--help"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rootly --help returned error: %v", err)
	}
	if strings.Contains(stdout.String(), "rootly agent") || strings.Contains(stdout.String(), "rootly chat") {
		t.Errorf("root help lists hidden agent command: %q", stdout.String())
	}
}
