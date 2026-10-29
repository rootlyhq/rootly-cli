package cmd

import "github.com/rootlyhq/rootly-cli/internal/cmd/agent"

func init() {
	rootCmd.AddCommand(agent.AgentCmd)
}
