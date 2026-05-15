package teams

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/config"
	"github.com/rootlyhq/rootly-cli/internal/oauth"
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
	token := viper.GetString("api_key")
	if token == "" {
		if !oauth.HasTokens() {
			return nil, fmt.Errorf("authentication required: run 'rootly login' or set ROOTLY_API_KEY")
		}
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
