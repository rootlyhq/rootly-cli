package teams

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/printer"
	"github.com/rootlyhq/rootly-cli/internal/timeformat"
)

var getCmd = &cobra.Command{
	Use:   "get <team-id>",
	Short: "Get team details",
	Long:  "Get detailed information about a specific team by ID or slug.",
	Example: `  # Get team by slug
  rootly teams get engineering

  # Get team by UUID
  rootly teams get 01234567-89ab-cdef-0123-456789abcdef

  # Output as JSON
  rootly teams get engineering --format=json

  # Output as YAML
  rootly teams get engineering --format=yaml`,
	Args: cobra.ExactArgs(1),
	RunE: runGet,
}

func init() {
	TeamsCmd.AddCommand(getCmd)
}

func runGet(cmd *cobra.Command, args []string) error {
	// Get team ID from args
	id := args[0]

	// Get API client
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}

	// Call API
	team, err := apiClient.GetTeamByID(cmd.Context(), id)
	if err != nil {
		return fmt.Errorf("failed to get team: %w", err)
	}

	// Get format from viper
	format := viper.GetString("format")

	// Create printer
	p, err := printer.NewPrinter(format)
	if err != nil {
		return err
	}

	// For json/yaml: pass through raw API response
	if format == "json" || format == "yaml" {
		return p.PrintRawJSON(team.RawBody, os.Stdout)
	}

	// For table/markdown: build key-value rows
	headers := []string{"Field", "Value"}
	rows := teamDetailRows(team)

	return p.PrintList(headers, rows, os.Stdout)
}

// teamDetailRows converts a Team to table rows for display.
func teamDetailRows(team *api.Team) [][]string {
	rows := [][]string{
		{"ID", team.ID},
		{"Name", team.Name},
		{"Slug", team.Slug},
	}

	if team.Description != "" {
		rows = append(rows, []string{"Description", team.Description})
	}
	if team.Color != "" {
		rows = append(rows, []string{"Color", team.Color})
	}
	if len(team.Users) > 0 {
		rows = append(rows, []string{"Users", strings.Join(team.Users, ", ")})
	}
	if !team.CreatedAt.IsZero() {
		rows = append(rows, []string{"Created", timeformat.FormatTime(team.CreatedAt)})
	}
	if !team.UpdatedAt.IsZero() {
		rows = append(rows, []string{"Updated", timeformat.FormatTime(team.UpdatedAt)})
	}

	return rows
}
