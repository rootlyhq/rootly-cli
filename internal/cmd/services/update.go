package services

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/printer"
)

var updateCmd = &cobra.Command{
	Use:   "update <service-id>",
	Short: "Update a service",
	Long:  "Update an existing Rootly service's attributes.",
	Example: `  # Update name
  rootly services update api-gateway --name="API Gateway v2"

  # Update description and color
  rootly services update api-gateway \
    --description="Updated: Main API gateway" \
    --color="#00FF00"

  # Update multiple fields
  rootly services update api-gateway --name="New Name" --color="#FF0000"`,
	Args: cobra.ExactArgs(1),
	RunE: runUpdate,
}

func init() {
	updateCmd.Flags().String("name", "", "Updated service name")
	updateCmd.Flags().String("description", "", "Updated description")
	updateCmd.Flags().String("color", "", "Updated color (hex format, e.g., #FF5733)")

	// Register with parent command
	ServicesCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	serviceID := args[0]

	// Get API client
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}

	// Build opts map using cmd.Flags().Changed() - ONLY include fields the user explicitly set
	opts := make(map[string]string)
	if cmd.Flags().Changed("name") {
		name, _ := cmd.Flags().GetString("name")
		opts["name"] = name
	}
	if cmd.Flags().Changed("description") {
		description, _ := cmd.Flags().GetString("description")
		opts["description"] = description
	}
	if cmd.Flags().Changed("color") {
		color, _ := cmd.Flags().GetString("color")
		// Validate color format
		if color != "" && !strings.HasPrefix(color, "#") {
			return fmt.Errorf("color must be in hex format (e.g., #FF5733)")
		}
		opts["color"] = color
	}

	// If opts is empty (no flags changed), return error
	if len(opts) == 0 {
		return fmt.Errorf("at least one field must be specified for update")
	}

	// Call API
	service, err := apiClient.UpdateService(cmd.Context(), serviceID, opts)
	if err != nil {
		return fmt.Errorf("failed to update service: %w", err)
	}

	// Print success message to stderr
	fmt.Fprintf(os.Stderr, "Updated service %s\n", serviceID)

	// Print service to stdout using configured format
	format := viper.GetString("format")
	p, err := printer.NewPrinter(format)
	if err != nil {
		return err
	}

	return p.PrintObj(service, os.Stdout)
}
