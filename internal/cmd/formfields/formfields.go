package formfields

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/config"
	"github.com/rootlyhq/rootly-cli/internal/oauth"
)

var FormFieldsCmd = &cobra.Command{
	Use:     "form-fields",
	Aliases: []string{"form-field"},
	Short:   "List custom form fields",
	Long:    "List custom form fields and their available options.",
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
	return api.NewClient(&config.Config{
		APIKey:   token,
		Endpoint: endpoint,
		Debug:    viper.GetBool("debug"),
	})
}
