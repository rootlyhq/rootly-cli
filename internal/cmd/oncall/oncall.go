package oncall

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/config"
	"github.com/rootlyhq/rootly-cli/internal/oauth"
)

// OncallCmd is the parent command for all on-call operations
var OncallCmd = &cobra.Command{
	Use:     "oncall",
	Aliases: []string{"on-call"},
	Short:   "Query on-call schedules and shifts",
	Long: `Query on-call schedules, view shifts, and see who is currently on-call.

Note: Schedules are managed in the Rootly UI. This command provides read-only access.`,
	Example: `  # List on-call schedules
  rootly oncall schedules

  # View upcoming shifts (next 7 days)
  rootly oncall shifts

  # View shifts for next 14 days
  rootly oncall shifts --days=14

  # See who is on-call right now
  rootly oncall who

  # Filter by name or ID
  rootly oncall who --schedule="Primary On-Call"
  rootly oncall shifts --service="API Gateway"
  rootly oncall who --user="alice@example.com"
  rootly oncall shifts --schedule-id=sched-123`,
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

// resolveScheduleID returns the schedule ID from either --schedule-id or --schedule (name lookup).
func resolveScheduleID(ctx context.Context, client *api.Client, cmd *cobra.Command) (string, error) {
	id, _ := cmd.Flags().GetString("schedule-id")
	name, _ := cmd.Flags().GetString("schedule")
	if id != "" && name != "" {
		return "", fmt.Errorf("specify either --schedule-id or --schedule, not both")
	}
	if name != "" {
		return client.ResolveScheduleIDByName(ctx, name)
	}
	return id, nil
}

// resolveServiceID returns the service ID from either --service-id or --service (name lookup).
func resolveServiceID(ctx context.Context, client *api.Client, cmd *cobra.Command) (string, error) {
	id, _ := cmd.Flags().GetString("service-id")
	name, _ := cmd.Flags().GetString("service")
	if id != "" && name != "" {
		return "", fmt.Errorf("specify either --service-id or --service, not both")
	}
	if name != "" {
		return client.ResolveServiceIDByName(ctx, name)
	}
	return id, nil
}

// resolveUserID returns the user ID from either --user-id or --user (name/email lookup).
func resolveUserID(ctx context.Context, client *api.Client, cmd *cobra.Command) (string, error) {
	id, _ := cmd.Flags().GetString("user-id")
	query, _ := cmd.Flags().GetString("user")
	if id != "" && query != "" {
		return "", fmt.Errorf("specify either --user-id or --user, not both")
	}
	if query != "" {
		return client.ResolveUserID(ctx, query)
	}
	return id, nil
}

// resolveTeamID returns the team/group ID from either --team-id or --team (name lookup).
func resolveTeamID(ctx context.Context, client *api.Client, cmd *cobra.Command) (string, error) {
	id, _ := cmd.Flags().GetString("team-id")
	name, _ := cmd.Flags().GetString("team")
	if id != "" && name != "" {
		return "", fmt.Errorf("specify either --team-id or --team, not both")
	}
	if name != "" {
		return client.ResolveTeamIDByName(ctx, name)
	}
	return id, nil
}
