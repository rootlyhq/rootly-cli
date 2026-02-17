package teams

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/printer"
)

var updateCmd = &cobra.Command{
	Use:   "update <team-id>",
	Short: "Update a team",
	Long:  "Update an existing Rootly team's attributes.",
	Example: `  # Update name
  rootly teams update engineering --name="Engineering Team"

  # Update description and color
  rootly teams update engineering \
    --description="Updated: Core engineering team" \
    --color="#00FF00"

  # Update multiple fields
  rootly teams update engineering --name="New Name" --color="#FF0000"`,
	Args: cobra.ExactArgs(1),
	RunE: runUpdate,
}

func init() {
	updateCmd.Flags().String("name", "", "Updated team name")
	updateCmd.Flags().String("description", "", "Updated description")
	updateCmd.Flags().String("color", "", "Updated color (hex format, e.g., #FF5733)")

	// Register with parent command
	TeamsCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	teamID := args[0]

	// Get API client
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}

	// Build opts map using cmd.Flags().Changed() - ONLY include fields the user explicitly set
	opts := make(map[string]string)
	if cmd.Flags().Changed("name") {
		name, _ := cmd.Flags().GetString("name")
		opts["name"] = name
	}
	if cmd.Flags().Changed("description") {
		description, _ := cmd.Flags().GetString("description")
		opts["description"] = description
	}
	if cmd.Flags().Changed("color") {
		color, _ := cmd.Flags().GetString("color")
		// Validate color format
		if color != "" && !strings.HasPrefix(color, "#") {
			return fmt.Errorf("color must be in hex format (e.g., #FF5733)")
		}
		opts["color"] = color
	}

	// If opts is empty (no flags changed), return error
	if len(opts) == 0 {
		return fmt.Errorf("at least one field must be specified for update")
	}

	// Call API
	team, err := apiClient.UpdateTeam(cmd.Context(), teamID, opts)
	if err != nil {
		return fmt.Errorf("failed to update team: %w", err)
	}

	// Print success message to stderr
	fmt.Fprintf(os.Stderr, "Updated team %s\n", teamID)

	// Print team to stdout using configured format
	format := viper.GetString("format")
	p, err := printer.NewPrinter(format)
	if err != nil {
		return err
	}

	return p.PrintObj(team, os.Stdout)
}
