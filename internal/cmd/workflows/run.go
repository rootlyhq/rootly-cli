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
  rootly workflows run <workflow-id> --incident=INC-123 --format=json`,
	Args: cobra.ExactArgs(1),
	RunE: runWorkflow,
}

func init() {
	runCmd.Flags().String("incident", "", "Incident ID (required)")
	_ = runCmd.MarkFlagRequired("incident")
	WorkflowsCmd.AddCommand(runCmd)
}

func runWorkflow(cmd *cobra.Command, args []string) error {
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}
	workflowRef := args[0]
	incidentID, _ := cmd.Flags().GetString("incident")
	incidentID = api.NormalizeIncidentID(incidentID)

	workflowID, err := apiClient.ResolveWorkflowID(cmd.Context(), workflowRef)
	if err != nil {
		return fmt.Errorf("failed to resolve workflow: %w", err)
	}
	run, err := apiClient.RunWorkflowCLI(cmd.Context(), workflowID, incidentID)
	if err != nil {
		return fmt.Errorf("failed to run workflow: %w", err)
	}
	if !viper.GetBool("quiet") {
		fmt.Fprintf(os.Stderr, "Started workflow %s for incident %s\n", workflowRef, incidentID)
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
