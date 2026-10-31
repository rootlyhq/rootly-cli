package auth

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/rootlyhq/rootly-cli/internal/oauth"
)

var LogoutCmd = &cobra.Command{
	Use:         "logout",
	Short:       "Clear stored OAuth2 tokens and client registration",
	Annotations: map[string]string{"skipAuth": "true"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := oauth.ClearTokens(); err != nil {
			return fmt.Errorf("failed to clear tokens: %w", err)
		}
		if err := oauth.ClearRegistration(); err != nil {
			return fmt.Errorf("failed to clear OAuth client registration: %w", err)
		}
		_, _ = fmt.Fprintf(cmd.OutOrStderr(), "Logged out. OAuth tokens and client registration cleared.\n")
		return nil
	},
}
