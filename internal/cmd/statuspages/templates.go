package statuspages

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/printer"
)

var templatesCmd = &cobra.Command{
	Use:   "templates",
	Short: "Manage status-page templates",
}

var templatesListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List templates for a status page",
	Example: "  rootly status-pages templates list --status-page=<id>",
	RunE:    runTemplatesList,
}

func init() {
	templatesListCmd.Flags().String("status-page", "", "Status page ID (required)")
	templatesListCmd.Flags().Int("page", 1, "Page number")
	templatesListCmd.Flags().Int("page-size", 25, "Results per page (max 100)")
	_ = templatesListCmd.MarkFlagRequired("status-page")
	templatesCmd.AddCommand(templatesListCmd)
	StatusPagesCmd.AddCommand(templatesCmd)
}

func runTemplatesList(cmd *cobra.Command, _ []string) error {
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}
	statusPageID, _ := cmd.Flags().GetString("status-page")
	page, _ := cmd.Flags().GetInt("page")
	pageSize, _ := cmd.Flags().GetInt("page-size")
	result, err := apiClient.ListStatusPageTemplatesCLI(cmd.Context(), statusPageID, page, pageSize)
	if err != nil {
		return fmt.Errorf("failed to list status-page templates: %w", err)
	}

	format := viper.GetString("format")
	p, err := printer.NewPrinter(format)
	if err != nil {
		return err
	}
	if format == "json" || format == "yaml" {
		return p.PrintRawJSON(result.RawBody, os.Stdout)
	}

	rows := make([][]string, 0, len(result.Templates))
	for _, template := range result.Templates {
		rows = append(rows, []string{
			template.ID,
			template.Title,
			template.Kind,
			template.UpdateStatus,
			strconv.FormatBool(template.ShouldNotifySubscribers),
			strconv.FormatBool(template.Enabled),
		})
	}
	if err := p.PrintList([]string{"ID", "TITLE", "KIND", "UPDATE STATUS", "NOTIFY", "ENABLED"}, rows, os.Stdout); err != nil {
		return fmt.Errorf("failed to print output: %w", err)
	}
	if result.Pagination.TotalPages > 1 {
		fmt.Fprintf(os.Stderr, "\nPage %d of %d (%d total status-page templates)\n",
			result.Pagination.CurrentPage,
			result.Pagination.TotalPages,
			result.Pagination.TotalCount)
	}
	return nil
}
