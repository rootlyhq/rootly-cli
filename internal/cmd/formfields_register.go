package cmd

import "github.com/rootlyhq/rootly-cli/internal/cmd/formfields"

func init() {
	rootCmd.AddCommand(formfields.FormFieldsCmd)
}
