package statuspages

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/printer"
	"github.com/rootlyhq/rootly-cli/internal/timeformat"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List status pages",
	Example: `  rootly status-pages list
  rootly status-pages list --slug=public-status --format=json`,
	RunE: runList,
}

func init() {
	listCmd.Flags().Int("page", 1, "Page number")
	listCmd.Flags().Int("page-size", 25, "Results per page (max 100)")
	listCmd.Flags().String("sort", "-created_at", "Sort order")
	listCmd.Flags().String("search", "", "Filter by title or description")
	listCmd.Flags().String("slug", "", "Filter by slug")
	StatusPagesCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}
	page, _ := cmd.Flags().GetInt("page")
	pageSize, _ := cmd.Flags().GetInt("page-size")
	sort, _ := cmd.Flags().GetString("sort")
	search, _ := cmd.Flags().GetString("search")
	slug, _ := cmd.Flags().GetString("slug")
	filters := make(map[string]string)
	if search != "" {
		filters["search"] = search
	}
	if slug != "" {
		filters["slug"] = slug
	}
	result, err := apiClient.ListStatusPagesCLI(cmd.Context(), page, pageSize, sort, filters)
	if err != nil {
		return fmt.Errorf("failed to list status pages: %w", err)
	}
	format := viper.GetString("format")
	p, err := printer.NewPrinter(format)
	if err != nil {
		return err
	}
	if format == "json" || format == "yaml" {
		return p.PrintRawJSON(result.RawBody, os.Stdout)
	}
	rows := make([][]string, 0, len(result.StatusPages))
	for _, page := range result.StatusPages {
		rows = append(rows, []string{page.ID, page.Title, page.Slug, strconv.FormatBool(page.Public), strconv.FormatBool(page.Enabled), timeformat.FormatTime(page.CreatedAt)})
	}
	if err := p.PrintList([]string{"ID", "Title", "Slug", "Public", "Enabled", "Created"}, rows, os.Stdout); err != nil {
		return fmt.Errorf("failed to print output: %w", err)
	}
	if result.Pagination.TotalPages > 1 {
		fmt.Fprintf(os.Stderr, "\nPage %d of %d (%d total status pages)\n", result.Pagination.CurrentPage, result.Pagination.TotalPages, result.Pagination.TotalCount)
	}
	return nil
}
