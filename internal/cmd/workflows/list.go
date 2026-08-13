package workflows

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
	Short: "List workflows",
	Example: `  rootly workflows list
  rootly workflows list --name=retrospective
  rootly workflows list --slug=create-a-google-docs-retrospective --format=json`,
	RunE: runList,
}

func init() {
	listCmd.Flags().Int("page", 1, "Page number")
	listCmd.Flags().Int("page-size", 25, "Results per page (max 100)")
	listCmd.Flags().String("sort", "-created_at", "Sort order")
	listCmd.Flags().String("name", "", "Filter by workflow name")
	listCmd.Flags().String("slug", "", "Filter by workflow slug")
	WorkflowsCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}
	page, _ := cmd.Flags().GetInt("page")
	pageSize, _ := cmd.Flags().GetInt("page-size")
	sort, _ := cmd.Flags().GetString("sort")
	name, _ := cmd.Flags().GetString("name")
	slug, _ := cmd.Flags().GetString("slug")
	filters := make(map[string]string)
	if name != "" {
		filters["name"] = name
	}
	if slug != "" {
		filters["slug"] = slug
	}

	result, err := apiClient.ListWorkflowsCLI(cmd.Context(), page, pageSize, sort, filters)
	if err != nil {
		return fmt.Errorf("failed to list workflows: %w", err)
	}
	format := viper.GetString("format")
	p, err := printer.NewPrinter(format)
	if err != nil {
		return err
	}
	if format == "json" || format == "yaml" {
		return p.PrintRawJSON(result.RawBody, os.Stdout)
	}

	rows := make([][]string, 0, len(result.Workflows))
	for _, workflow := range result.Workflows {
		rows = append(rows, []string{
			workflow.ID,
			workflow.Name,
			workflow.Slug,
			strconv.FormatBool(workflow.Enabled),
			timeformat.FormatTime(workflow.CreatedAt),
		})
	}
	if err := p.PrintList([]string{"ID", "Name", "Slug", "Enabled", "Created"}, rows, os.Stdout); err != nil {
		return fmt.Errorf("failed to print output: %w", err)
	}
	if result.Pagination.TotalPages > 1 {
		fmt.Fprintf(os.Stderr, "\nPage %d of %d (%d total workflows)\n",
			result.Pagination.CurrentPage, result.Pagination.TotalPages, result.Pagination.TotalCount)
	}
	return nil
}
