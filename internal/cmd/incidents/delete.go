package incidents

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "delete <incident-id>",
	Short: "Delete an incident",
	Long:  "Delete a Rootly incident. Requires confirmation unless --yes is set.",
	Example: `  # Delete with confirmation prompt
  rootly incidents delete INC-123

  # Delete without confirmation (for scripts)
  rootly incidents delete INC-123 --yes`,
	Args: cobra.ExactArgs(1),
	RunE: runDelete,
}

func init() {
	deleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	// Register with parent command
	IncidentsCmd.AddCommand(deleteCmd)
}

// confirmDelete prompts for confirmation before deleting.
// Returns nil if confirmed, error if declined or unable to prompt.
func confirmDelete(prompt string, skipConfirm bool) error {
	if skipConfirm {
		return nil
	}

	if !isatty.IsTerminal(os.Stdin.Fd()) {
		return fmt.Errorf("cannot prompt in non-interactive mode, use --yes flag")
	}

	fmt.Fprintf(os.Stderr, "%s [y/N]: ", prompt)

	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		response := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if response == "y" || response == "yes" {
			return nil
		}
	}

	return fmt.Errorf("aborted")
}

func runDelete(cmd *cobra.Command, args []string) error {
	incidentID := args[0]

	// Get API client
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}

	// Handle confirmation
	skipConfirm, _ := cmd.Flags().GetBool("yes")
	prompt := fmt.Sprintf("Delete incident %s? This cannot be undone", incidentID)
	if err := confirmDelete(prompt, skipConfirm); err != nil {
		if err.Error() == "aborted" {
			fmt.Fprintln(os.Stderr, "Aborted.")
			return nil // User declined = not an error, exit 0
		}
		return err
	}

	// Call API
	if err := apiClient.DeleteIncident(cmd.Context(), incidentID); err != nil {
		return fmt.Errorf("failed to delete incident: %w", err)
	}

	// Print success message to stdout
	_, _ = fmt.Fprintf(os.Stdout, "Deleted incident %s\n", incidentID)

	return nil
}
