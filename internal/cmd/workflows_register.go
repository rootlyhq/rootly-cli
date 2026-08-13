package cmd

import "github.com/rootlyhq/rootly-cli/internal/cmd/workflows"

func init() {
	rootCmd.AddCommand(workflows.WorkflowsCmd)
}
