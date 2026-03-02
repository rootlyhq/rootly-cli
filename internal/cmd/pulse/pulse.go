package pulse

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/config"
)

// PulseCmd is the parent command for pulse operations
var PulseCmd = &cobra.Command{
	Use:     "pulse",
	Aliases: []string{"pulses"},
	Short:   "Send deployment and event pulses",
	Long:    "Send deployment and event pulses to Rootly for tracking changes and correlating with incidents.",
	Example: `  # Send a deployment pulse
  rootly pulse create "Deploy v1.2.3"

  # Send with labels and services
  rootly pulse create "Deploy v1.2.3" -s api-gateway -e production -l "version=1.2.3"

  # Wrap a command and send pulse with timing
  rootly pulse run -- make deploy

  # Wrap with custom summary
  rootly pulse run --summary="Deploy to production" -- make deploy`,
}

// getAPIClient creates a stateless API client for CLI operations.
// Returns error if API key is not configured.
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
