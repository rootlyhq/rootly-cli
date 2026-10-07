package formfields

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/printer"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List custom form fields",
	Long:  "List custom form fields with their input types and option values.",
	Example: `  # List custom form fields
  rootly form-fields list

  # List a specific page
  rootly form-fields list --page=2 --page-size=50

  # Output as JSON
  rootly form-fields list --format=json`,
	RunE: runList,
}

func init() {
	listCmd.Flags().Int("page", 1, "Page number")
	listCmd.Flags().Int("page-size", 25, "Results per page (max 100)")
	FormFieldsCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, _ []string) error {
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}

	page, _ := cmd.Flags().GetInt("page")
	pageSize, _ := cmd.Flags().GetInt("page-size")
	result, err := apiClient.ListFormFieldsCLI(cmd.Context(), page, pageSize)
	if err != nil {
		return fmt.Errorf("failed to list form fields: %w", err)
	}

	format := viper.GetString("format")
	p, err := printer.NewPrinter(format)
	if err != nil {
		return err
	}
	if format == "json" || format == "yaml" {
		return p.PrintRawJSON(result.RawBody, os.Stdout)
	}

	headers := []string{"ID", "SLUG", "NAME", "KIND", "INPUT KIND", "VALUE KIND", "OPTIONS"}
	rows := make([][]string, 0, len(result.FormFields))
	for _, field := range result.FormFields {
		optionValues := make([]string, 0, len(field.Options))
		for _, option := range field.Options {
			optionValues = append(optionValues, option.Value)
		}
		rows = append(rows, []string{
			field.ID,
			field.Slug,
			field.Name,
			field.Kind,
			field.InputKind,
			field.ValueKind,
			strings.Join(optionValues, ", "),
		})
	}

	if err := p.PrintList(headers, rows, os.Stdout); err != nil {
		return fmt.Errorf("failed to print output: %w", err)
	}
	if result.Pagination.TotalPages > 1 {
		fmt.Fprintf(os.Stderr, "\nPage %d of %d (%d total form fields)\n",
			result.Pagination.CurrentPage,
			result.Pagination.TotalPages,
			result.Pagination.TotalCount)
	}
	return nil
}
