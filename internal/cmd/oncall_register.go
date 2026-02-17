package cmd

import (
	"github.com/rootlyhq/rootly-tui/internal/cmd/oncall"
)

func init() {
	rootCmd.AddCommand(oncall.OncallCmd)
}
