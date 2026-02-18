package cmd

import "github.com/rootlyhq/rootly-cli/internal/cmd/pulse"

func init() {
	rootCmd.AddCommand(pulse.PulseCmd)
}
