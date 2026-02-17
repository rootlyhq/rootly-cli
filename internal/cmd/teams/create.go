package teams

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/printer"
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new team",
	Long:  "Create a new Rootly team with specified attributes.",
	Example: `  # Create with name only
  rootly teams create --name="Engineering"

  # Create with full details
  rootly teams create \
    --name="Platform Team" \
    --description="Infrastructure and platform engineering" \
    --color="#FF5733"

  # Create and output as JSON
  rootly teams create --name="Security Team" --format=json`,
	RunE: runCreate,
}

func init() {
	createCmd.Flags().String("name", "", "Team name (required)")
	createCmd.Flags().String("description", "", "Team description")
	createCmd.Flags().String("color", "", "Team color (hex format, e.g., #FF5733)")

	// Mark name as required
	_ = createCmd.MarkFlagRequired("name")

	// Register with parent command
	TeamsCmd.AddCommand(createCmd)
}

func runCreate(cmd *cobra.Command, args []string) error {
	// Get API client
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}

	// Read flags
	name, _ := cmd.Flags().GetString("name")
	description, _ := cmd.Flags().GetString("description")
	color, _ := cmd.Flags().GetString("color")

	// Validate color format if provided
	if color != "" && !strings.HasPrefix(color, "#") {
		return fmt.Errorf("color must be in hex format (e.g., #FF5733)")
	}

	// Build opts map - only add keys where the flag was provided
	opts := make(map[string]string)
	if description != "" {
		opts["description"] = description
	}
	if color != "" {
		opts["color"] = color
	}

	// Call API
	team, err := apiClient.CreateTeam(cmd.Context(), name, opts)
	if err != nil {
		return fmt.Errorf("failed to create team: %w", err)
	}

	// Print success message to stderr
	teamID := team.Slug
	if teamID == "" {
		teamID = team.ID
	}
	fmt.Fprintf(os.Stderr, "Created team %s\n", teamID)

	// Print team to stdout using configured format
	format := viper.GetString("format")
	p, err := printer.NewPrinter(format)
	if err != nil {
		return err
	}

	return p.PrintObj(team, os.Stdout)
}
