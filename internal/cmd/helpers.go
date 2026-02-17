package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

// ErrAborted is returned when the user declines a confirmation prompt.
var ErrAborted = errors.New("aborted")

// ConfirmAction prompts the user for confirmation before destructive operations.
// Returns nil if confirmed, ErrAborted if declined.
// If stdin is not a TTY and skipConfirm is false, returns an error telling
// the user to use --yes flag.
func ConfirmAction(prompt string, skipConfirm bool) error {
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

	return ErrAborted
}

// PrintDryRun outputs a dry-run preview message to stderr.
// Commands should call this and return early when --dry-run is set.
func PrintDryRun(action string, details map[string]string) {
	fmt.Fprintln(os.Stderr, "DRY RUN: no changes will be made")
	fmt.Fprintf(os.Stderr, "Would %s:\n", action)

	// Sort keys for consistent output
	keys := make([]string, 0, len(details))
	for k := range details {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		fmt.Fprintf(os.Stderr, "  %s: %s\n", k, details[k])
	}
}

// FormatAPIError wraps API errors with user-friendly context.
func FormatAPIError(action string, err error) error {
	return fmt.Errorf("failed to %s: %w", action, err)
}

// AddConfirmFlag adds --yes/-y flag to a command for skipping confirmation prompts.
func AddConfirmFlag(cmd *cobra.Command) {
	cmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
}

// AddDryRunFlag adds --dry-run flag to a command for previewing changes.
func AddDryRunFlag(cmd *cobra.Command) {
	cmd.Flags().Bool("dry-run", false, "Preview changes without applying")
}
