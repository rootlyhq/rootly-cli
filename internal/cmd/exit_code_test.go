package cmd

import (
	"errors"
	"fmt"
	"testing"

	aiagent "github.com/rootlyhq/rootly-cli/internal/agent"
)

func TestExitCode(t *testing.T) {
	notAuthenticated := aiagent.PrintExitError(aiagent.NotAuthenticatedError("not authenticated"))
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "generic", err: errors.New("boom"), want: 1},
		{name: "print not authenticated", err: notAuthenticated, want: 2},
		{name: "wrapped", err: fmt.Errorf("agent: %w", notAuthenticated), want: 2},
		{name: "unauthenticated outside print mode", err: aiagent.NotAuthenticatedError("not authenticated"), want: 1},
	}
	for _, test := range tests {
		if got := exitCode(test.err); got != test.want {
			t.Errorf("%s: exitCode() = %d, want %d", test.name, got, test.want)
		}
	}
}
