package cmd

import "github.com/rootlyhq/rootly-tui/internal/cmd/alerts"

func init() {
	rootCmd.AddCommand(alerts.AlertsCmd)
}
