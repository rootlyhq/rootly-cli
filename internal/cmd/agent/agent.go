package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/google/uuid"
	"github.com/mattn/go-isatty"
	rootly "github.com/rootlyhq/rootly-go"
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
		resumeValue, _ := cmd.Flags().GetString("resume")
		continueSession, _ := cmd.Flags().GetBool("continue")
		incidentValue, _ := cmd.Flags().GetString("incident")
		alertValue, _ := cmd.Flags().GetString("alert")
		outputFormat, _ := cmd.Flags().GetString("output-format")
		printMode := cmd.Flags().Changed("print") || !isTerminalReader(stdin)
		if cmd.Flags().Changed("output-format") && !printMode {
			return fmt.Errorf("--output-format is only valid with print or stdin mode")
		}
		if outputFormat != "text" && outputFormat != "json" && outputFormat != "stream-json" {
			return fmt.Errorf("invalid output format %q (expected text, json, or stream-json)", outputFormat)
		}
		var sessionID *uuid.UUID
		sessionTitle := ""
		if resumeValue != "" {
			id, parseErr := uuid.Parse(resumeValue)
			if parseErr != nil {
				return fmt.Errorf("invalid resume session UUID %q: %w", resumeValue, parseErr)
			}
			sessionID = &id
		}
		currentUser, err := preflightAI(cmd, client)
		if err != nil {
			return err
		}
		if continueSession {
			id, title, err := latestSession(cmd.Context(), client)
			if err != nil {
				return aiagent.SanitizeTerminalError(err)
			}
			sessionID = &id
			sessionTitle = title
		}
		var incident *api.Incident
		if incidentValue != "" {
			incident, err = client.GetIncidentByID(cmd.Context(), api.NormalizeIncidentID(incidentValue))
			if err != nil {
				return fmt.Errorf("incident not found: %w", aiagent.SanitizeTerminalError(err))
			}
		}
		var boundAlert *api.Alert
		if alertValue != "" {
			boundAlert, err = client.GetAlertByID(cmd.Context(), alertValue)
			if err != nil {
				return fmt.Errorf("alert not found: %w", aiagent.SanitizeTerminalError(err))
			}
		}
		if printMode {
			return aiagent.RunPrint(cmd.Context(), client, aiagent.PrintOptions{
				Prompt:       prompt,
				Stdin:        stdin,
				Stdout:       cmd.OutOrStdout(),
				Stderr:       cmd.ErrOrStderr(),
				NoColor:      viper.GetBool("no-color"),
				Quiet:        viper.GetBool("quiet"),
				APIHost:      client.APIHost(),
				SessionID:    sessionID,
				OutputFormat: outputFormat,
				Incident:     incident,
				Alert:        boundAlert,
				IsStdinTTY:   func() bool { return isTerminalReader(stdin) },
				IsStdoutTTY:  func() bool { return isTerminalWriter(cmd.OutOrStdout()) },
				IsStderrTTY:  func() bool { return isTerminalWriter(cmd.ErrOrStderr()) },
			})
		}

		noColor := viper.GetBool("no-color")
		model := aiagent.NewTUIModel(cmd.Context(), client, aiagent.TUIOptions{
			NoColor: noColor, Version: cmd.Root().Version, API: client, SessionID: sessionID, SessionTitle: sessionTitle,
			APIHost: client.APIHost(), CurrentUser: currentUser,
			Incident: incident, Alert: boundAlert,
		})
		options := []tea.ProgramOption{
			tea.WithInput(stdin),
			tea.WithOutput(cmd.OutOrStdout()),
			tea.WithContext(cmd.Context()),
		}
		if noColor {
			options = append(options, tea.WithColorProfile(colorprofile.Ascii))
		}
		noClear, _ := cmd.Flags().GetBool("no-clear")
		if err := clearTerminalForAgent(cmd.OutOrStdout(), !noClear, isTerminalWriter(cmd.OutOrStdout())); err != nil {
			return err
		}
		program := tea.NewProgram(model, options...)
		return runAgentTUI(cmd.OutOrStdout(), isTerminalWriter(cmd.OutOrStdout()), program.Run)
	},
}

func init() {
	AgentCmd.Flags().StringP("print", "p", "", "Send a prompt and print the response")
	AgentCmd.Flags().StringP("resume", "r", "", "Resume an AI chat session by UUID")
	AgentCmd.Flags().BoolP("continue", "c", false, "Continue the most recent AI chat session (incident/alert chats: use --resume <id>)")
	AgentCmd.Flags().String("incident", "", "Bind the chat to an incident")
	AgentCmd.Flags().String("alert", "", "Bind the chat to an alert")
	AgentCmd.Flags().String("output-format", "text", "Print output format: text, json, or stream-json")
	AgentCmd.Flags().Bool("no-clear", false, "Don't clear the terminal and scrollback on start")
	AgentCmd.MarkFlagsMutuallyExclusive("resume", "continue", "incident", "alert")
}

func latestSession(ctx context.Context, client *api.Client) (uuid.UUID, string, error) {
	limit, archived := 1, false
	response, err := client.ListAiChatSessionsWithResponse(ctx, &rootly.ListAiChatSessionsParams{Limit: &limit, Archived: &archived})
	if err != nil {
		return uuid.Nil, "", aiagent.SanitizeTerminalError(err)
	}
	if response == nil {
		return uuid.Nil, "", errors.New("empty response while listing AI chat sessions")
	}
	if response.ApplicationVndAPIJSON200 == nil {
		return uuid.Nil, "", aiagent.StatusResponseError(response.StatusCode(), response.Body)
	}
	sessions := response.ApplicationVndAPIJSON200.Sessions
	if len(sessions) == 0 {
		return uuid.Nil, "", errors.New("no previous AI chat session to continue (incident/alert-bound chats are resumed with --resume <id>)")
	}
	id, err := uuid.Parse(sessions[0].ID.String())
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("invalid session UUID from API: %w", err)
	}
	return id, sessions[0].Title, nil
}

func preflightAI(cmd *cobra.Command, client *api.Client) (*rootly.CurrentUserResponse, error) {
	response, err := client.GetCurrentUserWithResponse(cmd.Context())
	if err != nil {
		if viper.GetBool("debug") {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "AI preflight failed: %v\n", aiagent.SanitizeTerminalError(err))
		}
		return nil, nil
	}
	if response == nil {
		if viper.GetBool("debug") {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "AI preflight returned an empty response")
		}
		return nil, nil
	}
	if response.StatusCode() == 401 {
		return nil, aiagent.StatusResponseError(response.StatusCode(), response.Body)
	}
	if response.StatusCode() != 200 || response.ApplicationVndAPIJSON200 == nil {
		if response.StatusCode() == 403 {
			mapped := aiagent.StatusResponseError(response.StatusCode(), response.Body)
			if aiagent.IsAIChatDisabled(mapped) {
				return nil, mapped
			}
		}
		if viper.GetBool("debug") {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "AI preflight returned HTTP %d\n", response.StatusCode())
		}
		return nil, nil
	}
	current := response.ApplicationVndAPIJSON200
	if enabled, exists := current.Data.Attributes.FeatureFlags["ai-chat"]; exists && !enabled {
		return nil, aiagent.AIChatDisabledError()
	}
	return current, nil
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

func runAgentTUI(writer io.Writer, terminal bool, run func() (tea.Model, error)) error {
	finalModel, runErr := run()
	if terminal {
		if model, ok := finalModel.(*aiagent.TUIModel); ok {
			if _, err := io.WriteString(writer, model.Transcript()); err != nil && runErr == nil {
				return err
			}
		}
	}
	return runErr
}

func isTerminalReader(reader io.Reader) bool {
	file, ok := reader.(*os.File)
	return ok && isatty.IsTerminal(file.Fd())
}

func isTerminalWriter(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	return ok && isatty.IsTerminal(file.Fd())
}

func clearTerminalForAgent(writer io.Writer, enabled, terminal bool) error {
	if !enabled || !terminal {
		return nil
	}
	_, err := io.WriteString(writer, "\x1b[2J\x1b[3J\x1b[H")
	return err
}
