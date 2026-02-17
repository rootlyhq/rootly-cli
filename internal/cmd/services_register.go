package cmd

import "github.com/rootlyhq/rootly-tui/internal/cmd/services"

func init() {
	rootCmd.AddCommand(services.ServicesCmd)
}
