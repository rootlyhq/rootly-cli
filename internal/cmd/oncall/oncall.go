package oncall

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/config"
)

// OncallCmd is the parent command for all on-call operations
var OncallCmd = &cobra.Command{
	Use:     "oncall",
	Aliases: []string{"on-call"},
	Short:   "Query on-call schedules and shifts",
	Long: `Query on-call schedules, view shifts, and see who is currently on-call.

Note: Schedules are managed in the Rootly UI. This command provides read-only access.`,
	Example: `  # List on-call schedules
  rootly oncall list

  # View upcoming shifts (next 7 days)
  rootly oncall shifts

  # View shifts for next 14 days
  rootly oncall shifts --days=14

  # See who is on-call right now
  rootly oncall who

  # Filter by schedule or service
  rootly oncall who --schedule-id=sched-123
  rootly oncall shifts --service-id=svc-456`,
}

// getAPIClient creates a stateless API client for CLI operations.
// Returns error if API token is not configured.
func getAPIClient() (*api.Client, error) {
	token := viper.GetString("api_key")
	if token == "" {
		return nil, fmt.Errorf("API key required: set ROOTLY_API_KEY or add api_key to ~/.rootly-cli/config.yaml")
	}
	endpoint := viper.GetString("api_host")
	if endpoint == "" {
		endpoint = config.DefaultEndpoint
	}
	cfg := &config.Config{
		APIKey:   token,
		Endpoint: endpoint,
		Debug:    viper.GetBool("debug"),
	}
	return api.NewClient(cfg)
}
