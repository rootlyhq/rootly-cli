package statuspages

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/printer"
)

var validEventStatuses = map[string]bool{
	"completed": true, "identified": true, "in_progress": true,
	"investigating": true, "monitoring": true, "resolved": true, "scheduled": true,
}

var eventsCreateCmd = &cobra.Command{
	Use:     "create <incident-id>",
	Short:   "Create a status-page event for an incident",
	Example: `  rootly status-pages events create INC-123 --status-page=<id> --status=investigating --message="We are investigating."`,
	Args:    cobra.ExactArgs(1),
	RunE:    runEventsCreate,
}

var eventsUpdateCmd = &cobra.Command{
	Use:     "update <event-id>",
	Short:   "Update a status-page event",
	Example: `  rootly status-pages events update <event-id> --status=monitoring --message="A fix has been applied." --notify-subscribers`,
	Args:    cobra.ExactArgs(1),
	RunE:    runEventsUpdate,
}

var eventsResolveCmd = &cobra.Command{
	Use:     "resolve <event-id>",
	Short:   "Resolve a status-page event",
	Example: `  rootly status-pages events resolve <event-id> --message="The incident has been resolved." --notify-subscribers`,
	Args:    cobra.ExactArgs(1),
	RunE:    runEventsResolve,
}

func init() {
	eventsCreateCmd.Flags().String("status-page", "", "Status page ID (required)")
	eventsCreateCmd.Flags().String("status", "", "Event status (required)")
	eventsCreateCmd.Flags().String("message", "", "Public update message (required)")
	eventsCreateCmd.Flags().Bool("notify-subscribers", false, "Notify status-page subscribers")
	_ = eventsCreateCmd.MarkFlagRequired("status-page")
	_ = eventsCreateCmd.MarkFlagRequired("status")
	_ = eventsCreateCmd.MarkFlagRequired("message")

	eventsUpdateCmd.Flags().String("status", "", "Updated event status")
	eventsUpdateCmd.Flags().String("message", "", "Updated public message")
	eventsUpdateCmd.Flags().Bool("notify-subscribers", false, "Notify status-page subscribers")

	eventsResolveCmd.Flags().String("message", "", "Resolution message (required)")
	eventsResolveCmd.Flags().Bool("notify-subscribers", false, "Notify status-page subscribers")
	_ = eventsResolveCmd.MarkFlagRequired("message")

	eventsCmd.AddCommand(eventsCreateCmd, eventsUpdateCmd, eventsResolveCmd)
}

func validateEventStatus(status string) error {
	if !validEventStatuses[status] {
		return fmt.Errorf("invalid status %q: expected completed, identified, in_progress, investigating, monitoring, resolved, or scheduled", status)
	}
	return nil
}

func runEventsCreate(cmd *cobra.Command, args []string) error {
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}
	statusPageID, _ := cmd.Flags().GetString("status-page")
	status, _ := cmd.Flags().GetString("status")
	message, _ := cmd.Flags().GetString("message")
	notify, _ := cmd.Flags().GetBool("notify-subscribers")
	if err := validateEventStatus(status); err != nil {
		return err
	}
	event, err := apiClient.CreateStatusPageEventCLI(cmd.Context(), api.NormalizeIncidentID(args[0]), statusPageID, status, message, notify)
	if err != nil {
		return fmt.Errorf("failed to create status-page event: %w", err)
	}
	return printSavedEvent(event, "Created")
}

func runEventsUpdate(cmd *cobra.Command, args []string) error {
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}
	opts := api.StatusPageEventOpts{}
	if cmd.Flags().Changed("status") {
		status, _ := cmd.Flags().GetString("status")
		if err := validateEventStatus(status); err != nil {
			return err
		}
		opts.Status = &status
	}
	if cmd.Flags().Changed("message") {
		message, _ := cmd.Flags().GetString("message")
		opts.Message = &message
	}
	if cmd.Flags().Changed("notify-subscribers") {
		notify, _ := cmd.Flags().GetBool("notify-subscribers")
		opts.NotifySubscribers = &notify
	}
	if opts.Status == nil && opts.Message == nil && opts.NotifySubscribers == nil {
		return fmt.Errorf("at least one field must be specified for update")
	}
	event, err := apiClient.UpdateStatusPageEventCLI(cmd.Context(), args[0], opts)
	if err != nil {
		return fmt.Errorf("failed to update status-page event: %w", err)
	}
	return printSavedEvent(event, "Updated")
}

func runEventsResolve(cmd *cobra.Command, args []string) error {
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}
	status := "resolved"
	message, _ := cmd.Flags().GetString("message")
	notify, _ := cmd.Flags().GetBool("notify-subscribers")
	event, err := apiClient.UpdateStatusPageEventCLI(cmd.Context(), args[0], api.StatusPageEventOpts{
		Status: &status, Message: &message, NotifySubscribers: &notify,
	})
	if err != nil {
		return fmt.Errorf("failed to resolve status-page event: %w", err)
	}
	return printSavedEvent(event, "Resolved")
}

func printSavedEvent(event *api.StatusPageEvent, verb string) error {
	if !viper.GetBool("quiet") {
		fmt.Fprintf(os.Stderr, "%s status-page event %s\n", verb, event.ID)
	}
	format := viper.GetString("format")
	p, err := printer.NewPrinter(format)
	if err != nil {
		return err
	}
	if format == "json" || format == "yaml" {
		return p.PrintRawJSON(event.RawBody, os.Stdout)
	}
	return p.PrintObj(event, os.Stdout)
}
