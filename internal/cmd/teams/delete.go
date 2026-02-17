package teams

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "delete <team-id>",
	Short: "Delete a team",
	Long:  "Delete a Rootly team. Requires confirmation unless --yes is set.",
	Example: `  # Delete with confirmation prompt
  rootly teams delete engineering

  # Delete without confirmation (for scripts)
  rootly teams delete engineering --yes`,
	Args: cobra.ExactArgs(1),
	RunE: runDelete,
}

func init() {
	deleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")

	// Register with parent command
	TeamsCmd.AddCommand(deleteCmd)
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
	teamID := args[0]

	// Get API client
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}

	// Handle confirmation
	skipConfirm, _ := cmd.Flags().GetBool("yes")
	prompt := fmt.Sprintf("Delete team %s? This cannot be undone", teamID)
	if err := confirmDelete(prompt, skipConfirm); err != nil {
		if err.Error() == "aborted" {
			fmt.Fprintln(os.Stderr, "Aborted.")
			return nil // User declined = not an error, exit 0
		}
		return err
	}

	// Call API
	if err := apiClient.DeleteTeam(cmd.Context(), teamID); err != nil {
		return fmt.Errorf("failed to delete team: %w", err)
	}

	// Print success message to stdout
	fmt.Fprintf(os.Stdout, "Deleted team %s\n", teamID)

	return nil
}
