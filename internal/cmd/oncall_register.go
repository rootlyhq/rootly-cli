package cmd

import (
	"github.com/rootlyhq/rootly-cli/internal/cmd/oncall"
)

func init() {
	rootCmd.AddCommand(oncall.OncallCmd)
}
