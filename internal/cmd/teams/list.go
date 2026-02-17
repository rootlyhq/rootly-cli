package teams

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/printer"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List teams",
	Long:  "List Rootly teams with optional filters and pagination.",
	Example: `  # List teams (default: 25 per page, newest first)
  rootly teams list

  # List with custom page size
  rootly teams list --page-size=50

  # Filter by name
  rootly teams list --name=engineering

  # Sort by name
  rootly teams list --sort=name

  # Page 2 of results
  rootly teams list --page=2

  # Output as JSON (useful for piping to jq)
  rootly teams list --format=json`,
	RunE: runList,
}

func init() {
	listCmd.Flags().Int("page", 1, "Page number")
	listCmd.Flags().Int("page-size", 25, "Results per page (max 100)")
	listCmd.Flags().String("sort", "-created_at", "Sort order (e.g., name, -name, created_at, -created_at)")
	listCmd.Flags().String("name", "", "Filter by name (partial match)")

	TeamsCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	// Get API client
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}

	// Read flags
	page, _ := cmd.Flags().GetInt("page")
	pageSize, _ := cmd.Flags().GetInt("page-size")
	sort, _ := cmd.Flags().GetString("sort")
	name, _ := cmd.Flags().GetString("name")

	// Build filters map
	filters := make(map[string]string)
	if name != "" {
		filters["name"] = name
	}

	// Call API
	result, err := apiClient.ListTeamsCLI(cmd.Context(), page, pageSize, sort, filters)
	if err != nil {
		return fmt.Errorf("failed to list teams: %w", err)
	}

	// Get format from viper
	format := viper.GetString("format")

	// Create printer
	p, err := printer.NewPrinter(format)
	if err != nil {
		return err
	}

	// Build headers and rows
	headers := []string{"ID", "Name", "Slug", "Description", "Created"}
	rows := make([][]string, 0, len(result.Teams))

	for _, team := range result.Teams {
		id := team.Slug
		if id == "" {
			id = team.ID
		}

		row := []string{
			id,
			truncateString(team.Name, 40),
			team.Slug,
			truncateString(team.Description, 50),
			formatTime(nil, team.CreatedAt),
		}
		rows = append(rows, row)
	}

	// Print list
	if err := p.PrintList(headers, rows, os.Stdout); err != nil {
		return fmt.Errorf("failed to print output: %w", err)
	}

	// Print pagination info to stderr if there are multiple pages
	if result.Pagination.TotalPages > 1 {
		fmt.Fprintf(os.Stderr, "\nPage %d of %d (%d total teams)\n",
			result.Pagination.CurrentPage,
			result.Pagination.TotalPages,
			result.Pagination.TotalCount)
	}

	return nil
}

// truncateString truncates a string to maxLen length, appending "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// formatTime formats a time pointer or falls back to a default time.
// Returns formatted time string in "2006-01-02 15:04" format.
func formatTime(primary *time.Time, fallback time.Time) string {
	t := fallback
	if primary != nil {
		t = *primary
	}
	return t.Format("2006-01-02 15:04")
}
