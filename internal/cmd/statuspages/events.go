package statuspages

import "github.com/spf13/cobra"

var eventsCmd = &cobra.Command{
	Use:   "events",
	Short: "Manage incident status-page events",
}

func init() {
	StatusPagesCmd.AddCommand(eventsCmd)
}
