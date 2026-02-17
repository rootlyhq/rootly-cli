package services

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/config"
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
