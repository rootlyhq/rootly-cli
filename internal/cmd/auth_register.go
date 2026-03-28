package cmd

import "github.com/rootlyhq/rootly-cli/internal/cmd/auth"

func init() {
	rootCmd.AddCommand(auth.LoginCmd)
	rootCmd.AddCommand(auth.LogoutCmd)
}
