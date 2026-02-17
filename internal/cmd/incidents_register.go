package cmd

import "github.com/rootlyhq/rootly-tui/internal/cmd/incidents"

func init() {
	rootCmd.AddCommand(incidents.IncidentsCmd)
}
