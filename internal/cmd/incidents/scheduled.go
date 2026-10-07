package incidents

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func parseScheduledTimestamp(cmd *cobra.Command, flag string) (string, error) {
	value, _ := cmd.Flags().GetString(flag)
	if value == "" {
		return "", nil
	}

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return "", fmt.Errorf("invalid --%s value %q: expected an RFC3339 timestamp", flag, value)
	}
	return parsed.Format(time.RFC3339Nano), nil
}
