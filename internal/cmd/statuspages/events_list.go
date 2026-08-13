package statuspages

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/printer"
	"github.com/rootlyhq/rootly-cli/internal/timeformat"
)

var eventsListCmd = &cobra.Command{
	Use:     "list <incident-id>",
	Short:   "List status-page events for an incident",
	Example: `  rootly status-pages events list INC-123`,
	Args:    cobra.ExactArgs(1),
	RunE:    runEventsList,
}

func init() {
	eventsListCmd.Flags().Int("page", 1, "Page number")
	eventsListCmd.Flags().Int("page-size", 25, "Results per page (max 100)")
	eventsCmd.AddCommand(eventsListCmd)
}

func runEventsList(cmd *cobra.Command, args []string) error {
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}
	page, _ := cmd.Flags().GetInt("page")
	pageSize, _ := cmd.Flags().GetInt("page-size")
	incidentID := api.NormalizeIncidentID(args[0])
	result, err := apiClient.ListStatusPageEventsCLI(cmd.Context(), incidentID, page, pageSize)
	if err != nil {
		return fmt.Errorf("failed to list status-page events: %w", err)
	}
	format := viper.GetString("format")
	p, err := printer.NewPrinter(format)
	if err != nil {
		return err
	}
	if format == "json" || format == "yaml" {
		return p.PrintRawJSON(result.RawBody, os.Stdout)
	}
	rows := make([][]string, 0, len(result.Events))
	for _, event := range result.Events {
		rows = append(rows, []string{event.ID, event.StatusPageID, event.Status, event.Event, timeformat.FormatTime(event.UpdatedAt)})
	}
	if err := p.PrintList([]string{"ID", "Status Page", "Status", "Message", "Updated"}, rows, os.Stdout); err != nil {
		return fmt.Errorf("failed to print output: %w", err)
	}
	if result.Pagination.TotalPages > 1 {
		fmt.Fprintf(os.Stderr, "\nPage %d of %d (%d total events)\n", result.Pagination.CurrentPage, result.Pagination.TotalPages, result.Pagination.TotalCount)
	}
	return nil
}
