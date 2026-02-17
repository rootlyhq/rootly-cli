package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "rootly",
	Short: "Rootly CLI - manage incidents and resources from the terminal",
	Long: `Rootly CLI provides command-line access to Rootly incidents, alerts,
services, and other resources. Perfect for terminal workflows, automation,
and LLM agent integration.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Bind flags to viper
		if err := viper.BindPFlags(cmd.Flags()); err != nil {
			return err
		}

		// Configure config file
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath("$HOME/.rootly-cli")

		// Configure environment variable handling
		viper.SetEnvPrefix("ROOTLY")
		viper.AutomaticEnv()

		// Map specific environment variables to config keys
		viper.BindEnv("api_token", "ROOTLY_API_TOKEN")
		viper.BindEnv("endpoint", "ROOTLY_API_ENDPOINT")

		// Read config file (ignore if not found)
		if err := viper.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				// Config file found but error reading - report it
				return fmt.Errorf("error reading config file: %w", err)
			}
			// Config file not found is OK - we'll use flags/env vars
		}

		// Skip auth validation for commands that don't need it
		cmdName := cmd.Name()
		if cmdName == "version" || cmdName == "completion" || cmdName == "help" {
			return nil
		}

		// TODO: Add auth validation for other commands when needed
		// For now, we'll let individual commands handle their own validation

		return nil
	},
}

func init() {
	// Disable command sorting for predictable help output
	cobra.EnableCommandSorting = false

	// Disable default completion command (we'll add custom completion in plan 03)
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	// Add persistent flags
	rootCmd.PersistentFlags().String("api-token", "", "Rootly API token (env: ROOTLY_API_TOKEN)")
	rootCmd.PersistentFlags().String("endpoint", "api.rootly.com", "Rootly API endpoint")
	rootCmd.PersistentFlags().String("format", "table", "Output format (table, json, yaml, markdown)")
	rootCmd.PersistentFlags().Bool("no-color", false, "Disable colored output")
}

// SetVersionInfo sets the version information from main package ldflags
func SetVersionInfo(v, c, d string) {
	version = v
	commit = c
	date = d
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
