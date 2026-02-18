package pulse

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/printer"
)

var createCmd = &cobra.Command{
	Use:   "create [flags] <summary>",
	Short: "Send a deployment or event pulse",
	Long:  "Send a deployment or event pulse to Rootly with optional labels, services, and environment metadata.",
	Example: `  # Simple pulse
  rootly pulse create "Deploy v1.2.3"

  # With labels and source
  rootly pulse create "Deploy v1.2.3" --source=ci --labels="version=1.2.3,team=backend"

  # With services and environments
  rootly pulse create "Deploy v1.2.3" -s api-gateway,payments -e production

  # With refs
  rootly pulse create "Deploy v1.2.3" -r "commit=abc123,pr=456"

  # Summary via flag instead of positional arg
  rootly pulse create --summary="Deploy v1.2.3"`,
	Args: cobra.ArbitraryArgs,
	RunE: runCreate,
}

func init() {
	createCmd.Flags().StringP("labels", "l", "", "Key=value pairs, comma-separated (env: ROOTLY_LABELS)")
	createCmd.Flags().StringP("services", "s", "", "Service slugs/IDs, comma-separated (env: ROOTLY_SERVICES)")
	createCmd.Flags().StringP("environments", "e", "", "Environment slugs/IDs, comma-separated (env: ROOTLY_ENVIRONMENTS)")
	createCmd.Flags().String("source", "cli", "Source identifier (env: ROOTLY_SOURCE)")
	createCmd.Flags().StringP("refs", "r", "", "Key=value ref pairs, comma-separated (env: ROOTLY_REFS)")
	createCmd.Flags().String("summary", "", "Summary (alternative to positional arg, env: ROOTLY_SUMMARY)")

	PulseCmd.AddCommand(createCmd)
}

// resolveStringFlag returns the flag value if explicitly set, then checks the env var, then returns the default.
func resolveStringFlag(cmd *cobra.Command, flagName, envVar, defaultVal string) string {
	if cmd.Flags().Changed(flagName) {
		val, _ := cmd.Flags().GetString(flagName)
		return val
	}
	if envVal := os.Getenv(envVar); envVal != "" {
		return envVal
	}
	return defaultVal
}

func runCreate(cmd *cobra.Command, args []string) error {
	// Resolve summary: positional args > --summary flag > ROOTLY_SUMMARY env var
	summary := strings.Join(args, " ")
	if summary == "" {
		summary = resolveStringFlag(cmd, "summary", "ROOTLY_SUMMARY", "")
	}
	if summary == "" {
		return fmt.Errorf("summary required: provide as positional argument or via --summary flag")
	}

	// Get API client
	apiClient, err := getAPIClient()
	if err != nil {
		return err
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

	now := time.Now()
	opts := api.PulseOpts{
		Source:         source,
		ServiceIDs:     services,
		EnvironmentIDs: environments,
		Labels:         labels,
		Refs:           refs,
		StartedAt:      &now,
		EndedAt:        &now,
	}

	pulse, err := apiClient.CreatePulseCLI(cmd.Context(), summary, opts)
	if err != nil {
		return fmt.Errorf("failed to create pulse: %w", err)
	}

	if !viper.GetBool("quiet") {
		fmt.Fprintf(os.Stderr, "Sent pulse %s\n", pulse.ID)
	}

	format := viper.GetString("format")
	p, err := printer.NewPrinter(format)
	if err != nil {
		return err
	}

	if format == "json" || format == "yaml" {
		return p.PrintRawJSON(pulse.RawBody, os.Stdout)
	}
	return p.PrintObj(pulse, os.Stdout)
}
