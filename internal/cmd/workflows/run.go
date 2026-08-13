package workflows

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/printer"
)

var runCmd = &cobra.Command{
	Use:   "run <workflow-id-or-slug>",
	Short: "Run a workflow for an incident",
	Example: `  rootly workflows run create-a-google-docs-retrospective --incident=INC-123
  rootly workflows run <workflow-id> --incident=INC-123 --check-conditions
  rootly workflows run <workflow-id> --incident=INC-123 --immediate=false --format=json`,
	Args: cobra.ExactArgs(1),
	RunE: runWorkflow,
}

func init() {
	runCmd.Flags().String("incident", "", "Incident ID, such as INC-123 or a UUID (required)")
	runCmd.Flags().Bool("immediate", true, "Run immediately instead of respecting the workflow wait time")
	runCmd.Flags().Bool("check-conditions", false, "Only run when the workflow conditions match the incident")
	_ = runCmd.MarkFlagRequired("incident")
	WorkflowsCmd.AddCommand(runCmd)
}

func runWorkflow(cmd *cobra.Command, args []string) error {
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}
	workflowRef := args[0]
	incidentRef, _ := cmd.Flags().GetString("incident")
	incidentID := api.NormalizeIncidentID(incidentRef)
	incident, err := apiClient.GetIncidentByID(cmd.Context(), incidentID)
	if err != nil {
		return fmt.Errorf("failed to resolve incident %s: %w", incidentID, err)
	}

	workflowID, err := apiClient.ResolveWorkflowID(cmd.Context(), workflowRef)
	if err != nil {
		return fmt.Errorf("failed to resolve workflow: %w", err)
	}
	immediate, _ := cmd.Flags().GetBool("immediate")
	opts := api.WorkflowRunOpts{IncidentID: incident.ID, Immediate: &immediate}
	if cmd.Flags().Changed("check-conditions") {
		checkConditions, _ := cmd.Flags().GetBool("check-conditions")
		opts.CheckConditions = &checkConditions
	}
	run, err := apiClient.RunWorkflowCLI(cmd.Context(), workflowID, opts)
	if err != nil {
		return fmt.Errorf("failed to run workflow: %w", err)
	}
	if !viper.GetBool("quiet") {
		fmt.Fprintf(os.Stderr, "Started workflow %s for incident %s\n", workflowRef, incidentRef)
	}

	format := viper.GetString("format")
	p, err := printer.NewPrinter(format)
	if err != nil {
		return err
	}
	if format == "json" || format == "yaml" {
		return p.PrintRawJSON(run.RawBody, os.Stdout)
	}
	return p.PrintObj(run, os.Stdout)
}
