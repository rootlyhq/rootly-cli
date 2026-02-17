package services

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/rootlyhq/rootly-cli/internal/printer"
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new service",
	Long:  "Create a new Rootly service with specified attributes.",
	Example: `  # Create with name only
  rootly services create --name="API Gateway"

  # Create with full details
  rootly services create \
    --name="Payment Service" \
    --description="Handles payment processing" \
    --color="#FF5733"

  # Create and output as JSON
  rootly services create --name="Auth Service" --format=json`,
	RunE: runCreate,
}

func init() {
	createCmd.Flags().String("name", "", "Service name (required)")
	createCmd.Flags().String("description", "", "Service description")
	createCmd.Flags().String("color", "", "Service color (hex format, e.g., #FF5733)")

	// Mark name as required
	_ = createCmd.MarkFlagRequired("name")

	// Register with parent command
	ServicesCmd.AddCommand(createCmd)
}

func runCreate(cmd *cobra.Command, args []string) error {
	// Get API client
	apiClient, err := getAPIClient()
	if err != nil {
		return err
	}

	// Read flags
	name, _ := cmd.Flags().GetString("name")
	description, _ := cmd.Flags().GetString("description")
	color, _ := cmd.Flags().GetString("color")

	// Validate color format if provided
	if color != "" && !strings.HasPrefix(color, "#") {
		return fmt.Errorf("color must be in hex format (e.g., #FF5733)")
	}

	// Build opts map - only add keys where the flag was provided
	opts := make(map[string]string)
	if description != "" {
		opts["description"] = description
	}
	if color != "" {
		opts["color"] = color
	}

	// Call API
	service, err := apiClient.CreateService(cmd.Context(), name, opts)
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}

	// Print success message to stderr
	serviceID := service.Slug
	if serviceID == "" {
		serviceID = service.ID
	}
	fmt.Fprintf(os.Stderr, "Created service %s\n", serviceID)

	// Print service to stdout using configured format
	format := viper.GetString("format")
	p, err := printer.NewPrinter(format)
	if err != nil {
		return err
	}

	return p.PrintObj(service, os.Stdout)
}
