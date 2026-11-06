package cmd

import "github.com/rootlyhq/rootly-cli/internal/cmd/mcp"

func init() {
	rootCmd.AddCommand(mcp.MCPCmd)
}
