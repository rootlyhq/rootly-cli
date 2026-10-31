package mcp

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	localmcp "github.com/rootlyhq/rootly-cli/internal/mcp"
)

var (
	registryURL   = localmcp.RegistryURL
	indexPath     = localmcp.IndexPath
	searchRefresh bool
	searchLimit   int
)

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search the MCP marketplace (official registry + curated list)",
	Long: `Search the official MCP registry (registry.modelcontextprotocol.io) merged with a
curated list; Rootly-integrated vendors (Datadog, Sentry, Linear, Notion) come first.
The index is cached in ~/.rootly-cli/mcp-index.json for 7 days (--refresh refetches).
Nothing is installed: each result shows the exact command or URL and the
'rootly mcp add' line that adds it, or add it from /mcp in 'rootly agent'.`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		errOut := cmd.ErrOrStderr()
		client := &http.Client{Timeout: 20 * time.Second}
		catalog, err := localmcp.LoadCatalog(cmd.Context(), client, registryURL, indexPath(), searchRefresh, func(entries, pages int) {
			_, _ = fmt.Fprintf(errOut, "\rIndexing MCP registry: %d servers (%d pages)", entries, pages)
		})
		_, _ = fmt.Fprint(errOut, "\r\033[K")
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "warning: %v\n", err)
		}
		results := localmcp.SearchMarket(catalog, strings.Join(args, " "), max(1, searchLimit))
		out := cmd.OutOrStdout()
		if len(results) == 0 {
			_, _ = fmt.Fprintln(out, "No MCP servers match.")
			return nil
		}
		for _, entry := range results {
			title := entry.Name
			if entry.Author != "" {
				title += " (" + entry.Author + ")"
			}
			_, _ = fmt.Fprintf(out, "%s\n  %s\n  runs: %s\n  add:  %s\n", title, oneLine(entry.Description), localmcp.InstallLabel(entry), localmcp.AddCommand(entry))
			if entry.EnvHint != "" {
				_, _ = fmt.Fprintf(out, "  needs: %s\n", entry.EnvHint)
			}
		}
		return nil
	},
}

func oneLine(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if runes := []rune(text); len(runes) > 160 {
		text = string(runes[:157]) + "..."
	}
	return text
}

func init() {
	searchCmd.Flags().BoolVar(&searchRefresh, "refresh", false, "Refetch the registry index")
	searchCmd.Flags().IntVar(&searchLimit, "limit", 12, "Maximum results")
	MCPCmd.AddCommand(searchCmd)
}
