package pulse

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/printer"
)

var runCmd = &cobra.Command{
	Use:   "run [flags] -- <command> [args...]",
	Short: "Run a command and send a pulse with timing",
	Long: `Run a command and automatically send a pulse to Rootly with start/end timestamps
and the command's exit status as a label.`,
	Example: `  # Run a command and send pulse
  rootly pulse run -- echo "hello world"

  # With custom summary
  rootly pulse run --summary="Build project" -- make build

  # With labels and services
  rootly pulse run -s api-gateway -l "env=prod" -- make deploy

  # Failed commands include exit_status label
  rootly pulse run -- false`,
	Args:               cobra.MinimumNArgs(1),
	RunE:               runRun,
	DisableFlagParsing: false,
}

func init() {
	runCmd.Flags().String("summary", "", "Pulse summary (defaults to command string, env: ROOTLY_SUMMARY)")
	runCmd.Flags().StringP("labels", "l", "", "Key=value pairs, comma-separated (env: ROOTLY_LABELS)")
	runCmd.Flags().StringP("services", "s", "", "Service slugs/IDs, comma-separated (env: ROOTLY_SERVICES)")
	runCmd.Flags().StringP("environments", "e", "", "Environment slugs/IDs, comma-separated (env: ROOTLY_ENVIRONMENTS)")
	runCmd.Flags().String("source", "cli", "Source identifier (env: ROOTLY_SOURCE)")
	runCmd.Flags().StringP("refs", "r", "", "Key=value ref pairs, comma-separated (env: ROOTLY_REFS)")

	PulseCmd.AddCommand(runCmd)
}

func runRun(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("command required: rootly pulse run -- <command> [args...]")
	}

	// Get API client early so we fail fast on missing config
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}

	// Build summary
	summary := resolveStringFlag(cmd, "summary", "ROOTLY_SUMMARY", "")
	if summary == "" {
		summary = strings.Join(args, " ")
	}

	// Resolve flags with env var fallback
	labelsStr := resolveStringFlag(cmd, "labels", "ROOTLY_LABELS", "")
	servicesStr := resolveStringFlag(cmd, "services", "ROOTLY_SERVICES", "")
	environmentsStr := resolveStringFlag(cmd, "environments", "ROOTLY_ENVIRONMENTS", "")
	source := resolveStringFlag(cmd, "source", "ROOTLY_SOURCE", "cli")
	refsStr := resolveStringFlag(cmd, "refs", "ROOTLY_REFS", "")

	// Parse labels
	labels, err := parseKeyValuePairs(labelsStr)
	if err != nil {
		return fmt.Errorf("invalid --labels: %w", err)
	}

	// Parse refs
	refs, err := parseKeyValuePairs(refsStr)
	if err != nil {
		return fmt.Errorf("invalid --refs: %w", err)
	}

	// Parse services and environments
	services := parseCommaSeparated(servicesStr)
	environments := parseCommaSeparated(environmentsStr)

	// Run the command
	startedAt := time.Now()

	childCmd := exec.CommandContext(cmd.Context(), args[0], args[1:]...)
	childCmd.Stdin = os.Stdin
	childCmd.Stdout = os.Stdout
	childCmd.Stderr = os.Stderr

	cmdErr := childCmd.Run()
	endedAt := time.Now()

	// Determine exit code
	exitCode := 0
	if cmdErr != nil {
		if exitErr, ok := cmdErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			// Command failed to start
			exitCode = 1
		}
	}

	// Append exit_status label
	labels = append(labels, api.KeyValue{Key: "exit_status", Value: fmt.Sprintf("%d", exitCode)})

	opts := api.PulseOpts{
		Source:         source,
		ServiceIDs:     services,
		EnvironmentIDs: environments,
		Labels:         labels,
		Refs:           refs,
		StartedAt:      &startedAt,
		EndedAt:        &endedAt,
	}

	pulse, pulseErr := apiClient.CreatePulseCLI(cmd.Context(), summary, opts)
	if pulseErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to send pulse: %v\n", pulseErr)
		// Still exit with command's exit code
		os.Exit(exitCode)
	}

	if !viper.GetBool("quiet") {
		fmt.Fprintf(os.Stderr, "Sent pulse %s\n", pulse.ID)
	}

	format := viper.GetString("format")
	p, printerErr := printer.NewPrinter(format)
	if printerErr == nil {
		if format == "json" || format == "yaml" {
			_ = p.PrintRawJSON(pulse.RawBody, os.Stdout)
		} else {
			_ = p.PrintObj(pulse, os.Stdout)
		}
	}

	if exitCode != 0 {
		os.Exit(exitCode)
	}

	return nil
}
