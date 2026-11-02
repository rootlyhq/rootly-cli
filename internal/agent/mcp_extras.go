package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/rootlyhq/rootly-cli/internal/mcp"
)

const (
	maxMentionSuggestions = 8
	maxPromptSuggestions  = 8
	maxAttachedResources  = 8
)

var resourceMentionPattern = regexp.MustCompile(`@([A-Za-z0-9_.-]+):(\S+)`)

type mcpPromptMsg struct {
	request int
	label   string
	text    string
	err     error
}

type mentionsReadyMsg struct {
	request  int
	echo     string
	message  string
	attached []string
	notes    []string
}

type resourceRef struct {
	server   string
	uri      string
	serverID string
}

func (r resourceRef) mention() string { return "@" + r.server + ":" + r.uri }

func (m *TUIModel) liveMCPViews() []mcp.ServerView {
	if m.mcp == nil {
		return nil
	}
	var views []mcp.ServerView
	for _, view := range m.mcp.Views() {
		if view.Status == mcp.StatusConnected && !view.SessionOff {
			views = append(views, view)
		}
	}
	return views
}

func oneLine(text string) string {
	return sanitizeTerminalText(strings.Join(strings.Fields(text), " "))
}

// appendPromptItems adds /server:prompt entries after built-ins and skills, which win on collision.
func (m *TUIModel) appendPromptItems(items []list.Item, value string) []list.Item {
	taken := map[string]bool{}
	for _, item := range items {
		if command, ok := item.(slashCommandItem); ok {
			taken[strings.ToLower(command.command)] = true
		}
	}
	lower := strings.ToLower(value)
	added := 0
	for _, view := range m.liveMCPViews() {
		for _, prompt := range view.Prompts {
			command := "/" + view.Server.Name + ":" + prompt.Name
			key := strings.ToLower(command)
			if taken[key] || isBuiltinSlashCommand(command) || !strings.HasPrefix(key, lower) {
				continue
			}
			description := oneLine(prompt.Description)
			if description == "" {
				description = "MCP prompt"
			}
			taken[key] = true
			items = append(items, slashCommandItem{command: command, description: description, insert: len(prompt.Arguments) > 0})
			added++
			if added == maxPromptSuggestions {
				return items
			}
		}
	}
	return items
}

func (m *TUIModel) findPrompt(server, name string) (prompt mcp.PromptInfo, serverID string, found bool) {
	for _, view := range m.liveMCPViews() {
		if view.Server.Name != server {
			continue
		}
		for _, candidate := range view.Prompts {
			if candidate.Name == name {
				return candidate, view.Server.ID(), true
			}
		}
	}
	return mcp.PromptInfo{}, "", false
}

// promptArguments maps key=value tokens by name and fills the rest positionally; extra words join the last argument.
func promptArguments(prompt mcp.PromptInfo, tokens []string) (arguments map[string]string, missing []string) {
	arguments = map[string]string{}
	known := map[string]bool{}
	for _, argument := range prompt.Arguments {
		known[argument.Name] = true
	}
	var positional []string
	for _, token := range tokens {
		if key, value, ok := strings.Cut(token, "="); ok && known[key] {
			arguments[key] = value
			continue
		}
		positional = append(positional, token)
	}
	var open []string
	for _, argument := range prompt.Arguments {
		if _, set := arguments[argument.Name]; !set {
			open = append(open, argument.Name)
		}
	}
	for index, name := range open {
		if index >= len(positional) {
			break
		}
		if index == len(open)-1 {
			arguments[name] = strings.Join(positional[index:], " ")
			break
		}
		arguments[name] = positional[index]
	}
	for _, argument := range prompt.Arguments {
		if argument.Required && arguments[argument.Name] == "" {
			missing = append(missing, argument.Name)
		}
	}
	return arguments, missing
}

func promptUsage(command string, prompt mcp.PromptInfo) string {
	parts := []string{command}
	for _, argument := range prompt.Arguments {
		if argument.Required {
			parts = append(parts, "<"+argument.Name+">")
		} else {
			parts = append(parts, "["+argument.Name+"]")
		}
	}
	return sanitizeTerminalText(strings.Join(parts, " "))
}

func (m *TUIModel) runMCPPrompt(command string) (tea.Cmd, bool) {
	parts := strings.Fields(command)
	if len(parts) == 0 || m.mcp == nil {
		return nil, false
	}
	server, name, ok := strings.Cut(strings.TrimPrefix(parts[0], "/"), ":")
	if !ok {
		return nil, false
	}
	prompt, serverID, found := m.findPrompt(server, name)
	if !found {
		return nil, false
	}
	arguments, missing := promptArguments(prompt, parts[1:])
	if len(missing) > 0 {
		return m.println("Usage: " + promptUsage(parts[0], prompt)), true
	}
	m.mentionRequest++
	manager, ctx, label, request := m.mcp, m.ctx, parts[0], m.mentionRequest
	m.status = "Loading prompt " + sanitizeTerminalText(label) + "…"
	return func() tea.Msg {
		text, err := manager.GetPrompt(ctx, serverID, name, arguments)
		return mcpPromptMsg{request: request, label: label, text: text, err: err}
	}, true
}

func (m *TUIModel) handleMCPPrompt(msg mcpPromptMsg) tea.Cmd {
	if msg.request != m.mentionRequest {
		return nil
	}
	m.status = ""
	if msg.err != nil {
		return m.println(faint(m.noColor, sanitizeTerminalText("Prompt "+msg.label+" failed: "+msg.err.Error())))
	}
	return m.submitPrompt(msg.text)
}

func (m *TUIModel) listMCPPrompts() tea.Cmd {
	var lines []string
	for _, view := range m.liveMCPViews() {
		for _, prompt := range view.Prompts {
			line := promptUsage("/"+view.Server.Name+":"+prompt.Name, prompt)
			if description := oneLine(prompt.Description); description != "" {
				line += "  " + faint(m.noColor, description)
			}
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return m.println("No MCP prompts from connected servers. Open /mcp to manage servers.")
	}
	return m.println("MCP prompts\n" + strings.Join(lines, "\n"))
}

func (m *TUIModel) listMCPResources() tea.Cmd {
	var lines []string
	for _, view := range m.liveMCPViews() {
		for _, resource := range view.Resources {
			line := sanitizeTerminalText(resourceRef{server: view.Server.Name, uri: resource.URI}.mention())
			if name := oneLine(resource.Name); name != "" {
				line += "  " + faint(m.noColor, name)
			}
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return m.println("No MCP resources from connected servers. Open /mcp to manage servers.")
	}
	return m.println("MCP resources (mention with @server:uri)\n" + strings.Join(lines, "\n"))
}

func lastToken(value string) string {
	if value == "" || strings.ContainsAny(value[len(value)-1:], " \t\r\n") {
		return ""
	}
	return value[strings.LastIndexAny(value, " \t\r\n")+1:]
}

// refreshMentionMenu opens the @server:uri palette for the word being typed.
func (m *TUIModel) refreshMentionMenu(value string) bool {
	token := lastToken(value)
	if !strings.HasPrefix(token, "@") || m.mcp == nil {
		return false
	}
	query := strings.ToLower(token[1:])
	var prefixed, contains []list.Item
	for _, view := range m.liveMCPViews() {
		for _, resource := range view.Resources {
			mention := resourceRef{server: view.Server.Name, uri: resource.URI}.mention()
			item := slashCommandItem{command: sanitizeTerminalText(mention), description: oneLine(resource.Name), mention: true}
			switch lower := strings.ToLower(mention[1:]); {
			case strings.HasPrefix(lower, query):
				prefixed = append(prefixed, item)
			case strings.Contains(lower, query):
				contains = append(contains, item)
			}
		}
	}
	items := make([]list.Item, 0, len(prefixed)+len(contains))
	items = append(items, prefixed...)
	items = append(items, contains...)
	if len(items) == 0 {
		return false
	}
	m.showCommandMenu(items[:min(len(items), maxMentionSuggestions)])
	return true
}

func (m *TUIModel) insertMention(mention string) {
	value := m.composer.Value()
	start := strings.LastIndexAny(value, " \t\r\n") + 1
	m.composer.SetValue(value[:start] + mention + " ")
	m.composer.CursorEnd()
}

func (m *TUIModel) resourceMentions(message string) []resourceRef {
	known := map[resourceRef]string{}
	for _, view := range m.liveMCPViews() {
		for _, resource := range view.Resources {
			ref := resourceRef{server: view.Server.Name, uri: resource.URI}
			if _, taken := known[ref]; !taken {
				known[ref] = view.Server.ID()
			}
		}
	}
	if len(known) == 0 {
		return nil
	}
	var refs []resourceRef
	seen := map[resourceRef]bool{}
	for _, match := range resourceMentionPattern.FindAllStringSubmatch(message, -1) {
		ref := resourceRef{server: match[1], uri: strings.TrimRight(match[2], ".,;:!?)")}
		if _, ok := known[ref]; !ok {
			ref.uri = match[2]
		}
		serverID, ok := known[ref]
		ref.serverID = serverID
		if ok && !seen[ref] && len(refs) < maxAttachedResources {
			seen[ref] = true
			refs = append(refs, ref)
		}
	}
	return refs
}

// sendWithResources reads mentioned resources before sending; nil when the message mentions none.
func (m *TUIModel) sendWithResources(message string) tea.Cmd {
	refs := m.resourceMentions(message)
	if len(refs) == 0 {
		return nil
	}
	m.mentionRequest++
	request, manager, ctx := m.mentionRequest, m.mcp, m.ctx
	m.status = fmt.Sprintf("Reading %d MCP resource(s)…", len(refs))
	return func() tea.Msg {
		return readMentionedResources(ctx, manager, request, message, refs)
	}
}

func readMentionedResources(ctx context.Context, manager MCPManager, request int, message string, refs []resourceRef) mentionsReadyMsg {
	result := mentionsReadyMsg{request: request, echo: message, message: message}
	var blocks []string
	for _, ref := range refs {
		text, truncated, err := manager.ReadResource(ctx, ref.serverID, ref.uri, mcp.ResourceByteCap)
		if err != nil {
			result.notes = append(result.notes, "skipped "+ref.mention()+": "+err.Error())
			continue
		}
		label := ref.mention()
		if truncated {
			label += fmt.Sprintf(" (first %d KB)", mcp.ResourceByteCap/1024)
		}
		result.attached = append(result.attached, label)
		blocks = append(blocks, fmt.Sprintf("<resource server=%q uri=%q>\n%s\n</resource>", ref.server, ref.uri, text))
	}
	if len(blocks) > 0 {
		result.message = strings.Join(blocks, "\n\n") + "\n\n" + message
	}
	return result
}

func (m *TUIModel) handleMentionsReady(msg mentionsReadyMsg) tea.Cmd {
	if msg.request != m.mentionRequest {
		return nil
	}
	m.status = ""
	var notes []string
	if len(msg.attached) > 0 {
		notes = append(notes, "attached: "+strings.Join(msg.attached, ", "))
	}
	notes = append(notes, msg.notes...)
	return m.sendPrompt(msg.echo, msg.message, sanitizeTerminalText(strings.Join(notes, "\n")))
}
