package alerts

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/config"
	"github.com/rootlyhq/rootly-cli/internal/oauth"
)

// AlertsCmd is the parent command for all alert operations
var AlertsCmd = &cobra.Command{
	Use:     "alerts",
	Aliases: []string{"alert", "alr"},
	Short:   "Manage alerts",
	Long:    "Create, view, update, acknowledge, and resolve Rootly alerts.\n\nNote: Alerts cannot be deleted. Use 'rootly alerts resolve' to close an alert.",
	Example: `  # List alerts
  rootly alerts list

  # Get alert details
  rootly alerts get ALR-123

  # Create a new alert
  rootly alerts create --summary="Database connection pool exhausted"

  # Update an alert
  rootly alerts update ALR-123 --status=acknowledged

  # Acknowledge an alert
  rootly alerts acknowledge ALR-123

  # Resolve an alert
  rootly alerts resolve ALR-123 --message="Issue fixed by restarting service"`,
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
