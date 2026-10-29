package agent

import (
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	aiagent "github.com/rootlyhq/rootly-cli/internal/agent"
	"github.com/rootlyhq/rootly-cli/internal/api"
	"github.com/rootlyhq/rootly-cli/internal/config"
	"github.com/rootlyhq/rootly-cli/internal/oauth"
)

var AgentCmd = &cobra.Command{
	Use:     "agent",
	Aliases: []string{"chat"},
	Short:   "Chat with Rootly AI",
	Hidden:  true,
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := getAPIClient()
		if err != nil {
			return err
		}

		prompt, _ := cmd.Flags().GetString("print")
		stdin := cmd.InOrStdin()
		if cmd.Flags().Changed("print") || !isTerminalReader(stdin) {
			return aiagent.RunPrint(cmd.Context(), client, aiagent.PrintOptions{
				Prompt:      prompt,
				Stdin:       stdin,
				Stdout:      cmd.OutOrStdout(),
				Stderr:      cmd.ErrOrStderr(),
				NoColor:     viper.GetBool("no-color"),
				Quiet:       viper.GetBool("quiet"),
				IsStdinTTY:  func() bool { return isTerminalReader(stdin) },
				IsStdoutTTY: func() bool { return isTerminalWriter(cmd.OutOrStdout()) },
				IsStderrTTY: func() bool { return isTerminalWriter(cmd.ErrOrStderr()) },
			})
		}

		model := aiagent.NewTUIModel(cmd.Context(), client)
		program := tea.NewProgram(model,
			tea.WithInput(stdin),
			tea.WithOutput(cmd.OutOrStdout()),
			tea.WithContext(cmd.Context()),
		)
		_, err = program.Run()
		return err
	},
}

func init() {
	AgentCmd.Flags().StringP("print", "p", "", "Send a prompt and print the response")
}

func getAPIClient() (*api.Client, error) {
	token := viper.GetString("api_key")
	if token == "" && !oauth.HasTokens() {
		return nil, fmt.Errorf("authentication required: run 'rootly login' or set ROOTLY_API_KEY")
	}
	endpoint := viper.GetString("api_host")
	if endpoint == "" {
		endpoint = config.DefaultEndpoint
	}
	return api.NewClient(&config.Config{
		APIKey:   token,
		Endpoint: endpoint,
		Debug:    viper.GetBool("debug"),
	})
}

func isTerminalReader(reader io.Reader) bool {
	file, ok := reader.(*os.File)
	return ok && isatty.IsTerminal(file.Fd())
}

func isTerminalWriter(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	return ok && isatty.IsTerminal(file.Fd())
}
