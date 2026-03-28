package services

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/config"
	"github.com/rootlyhq/rootly-cli/internal/oauth"
)

// ServicesCmd is the parent command for all service operations
var ServicesCmd = &cobra.Command{
	Use:     "services",
	Aliases: []string{"service", "svc"},
	Short:   "Manage services",
	Long:    "Create, view, update, and delete Rootly services.",
	Example: `  # List services
  rootly services list

  # Get service details
  rootly services get my-service-slug

  # Create a new service
  rootly services create --name="API Gateway"

  # Update a service
  rootly services update my-service-slug --name="Updated API Gateway"

  # Delete a service
  rootly services delete my-service-slug`,
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
