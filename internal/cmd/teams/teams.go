package teams

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/config"
)

// TeamsCmd is the parent command for all team operations
var TeamsCmd = &cobra.Command{
	Use:     "teams",
	Aliases: []string{"team"},
	Short:   "Manage teams",
	Long:    "Create, view, update, and delete Rootly teams.",
	Example: `  # List teams
  rootly teams list

  # Get team details
  rootly teams get my-team-slug

  # Create a new team
  rootly teams create --name="Engineering"

  # Update a team
  rootly teams update my-team-slug --name="Updated Engineering"

  # Delete a team
  rootly teams delete my-team-slug`,
}

// getAPIClient creates a stateless API client for CLI operations.
// Returns error if API token is not configured.
func getAPIClient() (*api.Client, error) {
	token := viper.GetString("api_token")
	if token == "" {
		return nil, fmt.Errorf("API token required: set ROOTLY_API_TOKEN or add api_token to ~/.rootly-cli/config.yaml")
	}
	endpoint := viper.GetString("endpoint")
	if endpoint == "" {
		endpoint = config.DefaultEndpoint
	}
	cfg := &config.Config{
		APIKey:   token,
		Endpoint: endpoint,
	}
	return api.NewClientWithoutCache(cfg)
}
