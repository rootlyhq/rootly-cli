package cmd

import "github.com/rootlyhq/rootly-cli/internal/cmd/teams"

func init() {
	rootCmd.AddCommand(teams.TeamsCmd)
}
