package cmd

import "github.com/rootlyhq/rootly-cli/internal/cmd/services"

func init() {
	rootCmd.AddCommand(services.ServicesCmd)
}
