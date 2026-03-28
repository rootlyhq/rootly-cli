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
	Long: `Rootly CLI - manage incidents, alerts, and resources from the terminal.

Authenticate via environment variable:
  export ROOTLY_API_KEY=your-api-key
  rootly incidents list

Or use a config file at ~/.rootly-cli/config.yaml:
  api_key: your-api-key
  api_host: api.rootly.com`,
	Example: `  # List incidents
  rootly incidents list

  # Get incident details as JSON
  rootly incidents get INC-123 --format=json

  # List with filters
  rootly incidents list --status=started --severity=critical

  # Send a deployment pulse
  rootly pulse create "Deploy v1.2.3" --source=ci

  # Generate shell completions
  rootly completion bash`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Bind flags to viper (both local and inherited persistent flags)
		if err := viper.BindPFlags(cmd.Flags()); err != nil {
			return err
		}
		if err := viper.BindPFlags(cmd.InheritedFlags()); err != nil {
			return err
		}

		// Explicitly bind hyphenated flags to underscore viper keys
		if f := cmd.Flag("api-host"); f != nil && f.Changed {
			viper.Set("api_host", f.Value.String())
		}
		if f := cmd.Flag("api-key"); f != nil && f.Changed {
			viper.Set("api_key", f.Value.String())
		}

		// Configure config file
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath("$HOME/.rootly-cli")

		// Configure environment variable handling
		viper.SetEnvPrefix("ROOTLY")
		viper.AutomaticEnv()

		// Map specific environment variables to config keys
		_ = viper.BindEnv("api_key", "ROOTLY_API_KEY")
		_ = viper.BindEnv("api_host", "ROOTLY_API_HOST")
		_ = viper.BindEnv("debug", "ROOTLY_DEBUG")
		_ = viper.BindEnv("quiet", "ROOTLY_QUIET")

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
		if cmdName == "version" || cmdName == "completion" || cmdName == "help" || cmdName == "login" || cmdName == "logout" {
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
	rootCmd.PersistentFlags().StringP("api-key", "k", "", "Rootly API key (env: ROOTLY_API_KEY)")
	rootCmd.PersistentFlags().String("api-host", "api.rootly.com", "Rootly API host (env: ROOTLY_API_HOST)")
	rootCmd.PersistentFlags().String("format", "table", "Output format (table, json, yaml, markdown)")
	rootCmd.PersistentFlags().Bool("no-color", false, "Disable colored output")
	rootCmd.PersistentFlags().BoolP("debug", "d", false, "Enable debug output (env: ROOTLY_DEBUG)")
	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "Suppress non-essential output (env: ROOTLY_QUIET)")
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
