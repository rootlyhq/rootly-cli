package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var versionCmd = &cobra.Command{
	Use:         "version",
	Short:       "Print version information",
	Long:        `Print the version, commit hash, and build date of the rootly CLI.`,
	Annotations: map[string]string{"skipAuth": "true"},
	RunE: func(cmd *cobra.Command, args []string) error {
		format := viper.GetString("format")

		if format == "json" {
			data := map[string]string{
				"version": version,
				"commit":  commit,
				"date":    date,
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			if err := enc.Encode(data); err != nil {
				return fmt.Errorf("error encoding JSON: %w", err)
			}
		} else {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "rootly version %s (commit: %s, built: %s)\n", version, commit, date)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
