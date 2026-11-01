package agent

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/rootlyhq/rootly-cli/internal/mcp"
)

const marketResultLimit = 50

var loadMarketCatalog = func(ctx context.Context, refresh bool) ([]mcp.MarketEntry, error) {
	return mcp.LoadCatalog(ctx, nil, mcp.RegistryURL, mcp.IndexPath(), refresh, nil)
}

type marketState struct {
	query   string
	loading bool
	err     string
	catalog []mcp.MarketEntry
	results []mcp.MarketEntry
	cursor  int
	offset  int
	preview *mcp.MarketEntry
	request int
}

type marketLoadedMsg struct {
	request int
	catalog []mcp.MarketEntry
	err     error
}

func (m *TUIModel) openMarket() tea.Cmd {
	if m.mcp == nil {
		m.mcpPage.message = "Local MCP is unavailable in this session"
		return nil
	}
	m.mcpPage.market = &marketState{}
	return m.loadMarket(false)
}

func (m *TUIModel) loadMarket(refresh bool) tea.Cmd {
	market := m.mcpPage.market
	m.marketRequest++
	market.request = m.marketRequest
	market.loading, market.err = true, ""
	request, ctx := market.request, m.ctx
	return func() tea.Msg {
		catalog, err := loadMarketCatalog(ctx, refresh)
		return marketLoadedMsg{request: request, catalog: catalog, err: err}
	}
}

func (m *TUIModel) handleMarketLoaded(msg marketLoadedMsg) {
	market := m.mcpPage.market
	if market == nil || msg.request != market.request {
		return
	}
	market.loading, market.catalog = false, msg.catalog
	if msg.err != nil {
		market.err = sanitizeTerminalText(msg.err.Error())
	}
	m.searchMarket()
}

func (m *TUIModel) searchMarket() {
	market := m.mcpPage.market
	market.results = mcp.SearchMarket(market.catalog, market.query, marketResultLimit)
	market.cursor, market.offset = 0, 0
}

func (m *TUIModel) handleMarketKey(msg tea.KeyPressMsg) tea.Cmd {
	market := m.mcpPage.market
	key := msg.String()
	if market.preview != nil {
		switch key {
		case "y", "enter":
			m.addFromMarket(*market.preview)
		case "n", "esc", "backspace":
			market.preview = nil
		}
		return nil
	}
	switch key {
	case "esc", "ctrl+c":
		m.mcpPage.market = nil
	case "up":
		market.cursor = max(0, market.cursor-1)
	case "down":
		market.cursor = min(max(0, len(market.results)-1), market.cursor+1)
	case "enter":
		if market.cursor < len(market.results) {
			entry := market.results[market.cursor]
			market.preview = &entry
		}
	case "ctrl+r":
		return m.loadMarket(true)
	case "backspace":
		if market.query != "" {
			runes := []rune(market.query)
			market.query = string(runes[:len(runes)-1])
			m.searchMarket()
		}
	default:
		if msg.Text != "" {
			market.query += msg.Text
			m.searchMarket()
		}
	}
	return nil
}

func (m *TUIModel) addFromMarket(entry mcp.MarketEntry) {
	server, err := m.mcp.AddFromMarket(entry)
	if err != nil {
		m.mcpPage.market.preview = nil
		m.mcpPage.market.err = sanitizeTerminalText(err.Error())
		return
	}
	m.mcpPage.market = nil
	m.mcpPage.message = fmt.Sprintf("Added %s to ~/.rootly-cli/mcp.json (off). Select it and press space to turn it on", sanitizeTerminalText(server.Name))
	m.refreshMCPViews()
	for index, row := range m.mcpRows() {
		if row.kind == mcpRowServer && row.view.Server.ID() == server.ID() {
			m.mcpPage.cursor = index
		}
	}
}

func (m *TUIModel) marketView() string {
	market := m.mcpPage.market
	width := max(20, m.width-1)
	bold := lipgloss.NewStyle().Bold(true)
	if market.preview != nil {
		return m.marketPreviewView(*market.preview, width, bold)
	}
	lines := []string{m.mcpStyle(bold, "MCP marketplace"), faint(m.noColor, "Official MCP registry + curated list · Rootly-integrated vendors first"), "Search: " + sanitizeTerminalText(market.query) + "▌"}
	switch {
	case market.loading && len(market.catalog) == 0:
		lines = append(lines, "", faint(m.noColor, "Indexing the MCP registry (cached in ~/.rootly-cli/mcp-index.json)…"))
	case market.err != "":
		lines = append(lines, faint(m.noColor, "✘ "+market.err))
	default:
		lines = append(lines, "")
	}
	height := max(1, m.mcpListHeight()-2)
	if market.cursor < market.offset {
		market.offset = market.cursor
	}
	if market.cursor >= market.offset+height {
		market.offset = market.cursor - height + 1
	}
	end := min(len(market.results), market.offset+height)
	for index := market.offset; index < end; index++ {
		lines = append(lines, ansi.Truncate(m.renderMarketRow(market.results[index], index == market.cursor), width, "…"))
	}
	if len(market.results) == 0 && !market.loading {
		lines = append(lines, faint(m.noColor, "No MCP servers match"))
	}
	for len(lines) < height+4 {
		lines = append(lines, "")
	}
	lines = append(lines, "", faint(m.noColor, ansi.Truncate("type to search · ↑/↓ move · Enter preview · ctrl+r refresh index · Esc back", width, "…")))
	return strings.Join(lines, "\n")
}

func (m *TUIModel) renderMarketRow(entry mcp.MarketEntry, selected bool) string {
	marker := "  "
	if selected {
		marker = "› "
	}
	name := sanitizeTerminalText(entry.Name)
	if entry.Rootly {
		name += " ★"
	}
	return marker + fmt.Sprintf("%-24s ", name) + faint(m.noColor, sanitizeTerminalText(strings.Join(strings.Fields(entry.Description), " ")))
}

func (m *TUIModel) marketPreviewView(entry mcp.MarketEntry, width int, bold lipgloss.Style) string {
	lines := []string{m.mcpStyle(bold, "Add "+sanitizeTerminalText(entry.Name)+" to your user MCPs?"), ""}
	if entry.Description != "" {
		lines = append(lines, sanitizeTerminalText(strings.Join(strings.Fields(entry.Description), " ")), "")
	}
	verb := "Runs"
	if entry.Config.URL != "" {
		verb = "Connects to"
	}
	lines = append(lines,
		verb+": "+sanitizeTerminalText(mcp.InstallLabel(entry)),
		"Writes to: ~/.rootly-cli/mcp.json (user scope)",
	)
	if entry.Author != "" {
		lines = append(lines, "Publisher: "+sanitizeTerminalText(entry.Author))
	}
	if entry.EnvHint != "" {
		lines = append(lines, "Needs: "+sanitizeTerminalText(entry.EnvHint))
	}
	lines = append(lines, "", "It is added switched off: nothing runs until you turn it on in /mcp.", "", faint(m.noColor, "y add · n back"))
	for index, line := range lines {
		lines[index] = ansi.Truncate(line, width, "…")
	}
	return strings.Join(lines, "\n")
}
