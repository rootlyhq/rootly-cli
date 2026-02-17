package cmd

import "github.com/rootlyhq/rootly-cli/internal/cmd/incidents"

func init() {
	rootCmd.AddCommand(incidents.IncidentsCmd)
}
