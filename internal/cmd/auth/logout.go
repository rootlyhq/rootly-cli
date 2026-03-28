package auth

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/rootlyhq/rootly-cli/internal/oauth"
)

var LogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Clear stored OAuth2 tokens",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := oauth.ClearTokens(); err != nil {
			return fmt.Errorf("failed to clear tokens: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStderr(), "Logged out. OAuth tokens cleared.\n")
		return nil
	},
}
