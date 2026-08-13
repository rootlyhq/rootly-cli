package statuspages

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/config"
	"github.com/rootlyhq/rootly-cli/internal/oauth"
)

// StatusPagesCmd is the parent command for status-page operations.
var StatusPagesCmd = &cobra.Command{
	Use:     "status-pages",
	Aliases: []string{"status-page"},
	Short:   "Manage status-page incident updates",
	Long:    "List Rootly status pages and manage their incident events.",
}

func getAPIClient() (*api.Client, error) {
	token := viper.GetString("api_key")
	if token == "" && !oauth.HasTokens() {
		return nil, fmt.Errorf("authentication required: run 'rootly login' or set ROOTLY_API_KEY")
	}
	endpoint := viper.GetString("api_host")
	if endpoint == "" {
		endpoint = config.DefaultEndpoint
	}
	return api.NewClient(&config.Config{APIKey: token, Endpoint: endpoint, Debug: viper.GetBool("debug")})
}
