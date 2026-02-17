package cmd

import "github.com/rootlyhq/rootly-tui/internal/cmd/teams"

func init() {
	rootCmd.AddCommand(teams.TeamsCmd)
}
