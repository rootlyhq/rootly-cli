package services

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-tui/internal/printer"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List services",
	Long:  "List Rootly services with optional filters and pagination.",
	Example: `  # List services (default: 25 per page, newest first)
  rootly services list

  # List with custom page size
  rootly services list --page-size=50

  # Filter by name
  rootly services list --name=api

  # Filter by slug
  rootly services list --slug=api-gateway

  # Sort by name
  rootly services list --sort=name

  # Page 2 of results
  rootly services list --page=2

  # Output as JSON (useful for piping to jq)
  rootly services list --format=json`,
	RunE: runList,
}

func init() {
	listCmd.Flags().Int("page", 1, "Page number")
	listCmd.Flags().Int("page-size", 25, "Results per page (max 100)")
	listCmd.Flags().String("sort", "-created_at", "Sort order (e.g., name, -name, created_at, -created_at)")
	listCmd.Flags().String("name", "", "Filter by name (partial match)")
	listCmd.Flags().String("slug", "", "Filter by slug")

	ServicesCmd.AddCommand(listCmd)
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
	slug, _ := cmd.Flags().GetString("slug")

	// Build filters map
	filters := make(map[string]string)
	if name != "" {
		filters["name"] = name
	}
	if slug != "" {
		filters["slug"] = slug
	}

	// Call API
	result, err := apiClient.ListServicesCLI(cmd.Context(), page, pageSize, sort, filters)
	if err != nil {
		return fmt.Errorf("failed to list services: %w", err)
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
	rows := make([][]string, 0, len(result.Services))

	for _, svc := range result.Services {
		id := svc.Slug
		if id == "" {
			id = svc.ID
		}

		row := []string{
			id,
			truncateString(svc.Name, 40),
			svc.Slug,
			truncateString(svc.Description, 50),
			formatTime(nil, svc.CreatedAt),
		}
		rows = append(rows, row)
	}

	// Print list
	if err := p.PrintList(headers, rows, os.Stdout); err != nil {
		return fmt.Errorf("failed to print output: %w", err)
	}

	// Print pagination info to stderr if there are multiple pages
	if result.Pagination.TotalPages > 1 {
		fmt.Fprintf(os.Stderr, "\nPage %d of %d (%d total services)\n",
			result.Pagination.CurrentPage,
			result.Pagination.TotalPages,
			result.Pagination.TotalCount)
	}

	return nil
}

// truncateString truncates a string to max length, appending "..." if truncated.
func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
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
