package cmd

import "github.com/rootlyhq/rootly-cli/internal/cmd/alerts"

func init() {
	rootCmd.AddCommand(alerts.AlertsCmd)
}
