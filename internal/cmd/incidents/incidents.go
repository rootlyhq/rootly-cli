package incidents

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/config"
	"github.com/rootlyhq/rootly-cli/internal/oauth"
)

// IncidentsCmd is the parent command for all incident operations
var IncidentsCmd = &cobra.Command{
	Use:     "incidents",
	Aliases: []string{"incident", "inc"},
	Short:   "Manage incidents",
	Long:    "Create, view, update, and delete Rootly incidents.",
	Example: `  # List incidents
  rootly incidents list

  # Get incident details
  rootly incidents get INC-123

  # Create a new incident
  rootly incidents create --title="Database outage"

  # Update an incident
  rootly incidents update INC-123 --status=mitigated

  # Delete an incident
  rootly incidents delete INC-123`,
}

// getAPIClient creates a stateless API client for CLI operations.
// Returns error if API token is not configured.
func getAPIClient() (*api.Client, error) {
	token := viper.GetString("api_key")
	if token == "" {
		if _, err := oauth.LoadTokens(); err != nil {
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
