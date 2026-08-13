package cmd

import "github.com/rootlyhq/rootly-cli/internal/cmd/statuspages"

func init() {
	rootCmd.AddCommand(statuspages.StatusPagesCmd)
}
