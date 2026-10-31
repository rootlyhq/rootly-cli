package agent

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/rootlyhq/rootly-cli/internal/mcp"
)

func extrasManager() *fakeMCPManager {
	resources := []mcp.ResourceInfo{{URI: "file:///runbook.md", Name: "Runbook"}, {URI: "file:///big.log", Name: "Big log"}, {URI: "file:///gone.md", Name: "Gone"}}
	for index := range 10 {
		resources = append(resources, mcp.ResourceInfo{URI: fmt.Sprintf("file:///extra-%d.md", index), Name: "Extra"})
	}
	return &fakeMCPManager{
		views: []mcp.ServerView{
			{
				Server: mcp.Server{Name: "docs", Scope: mcp.ScopeUser}, Status: mcp.StatusConnected,
				Tools:     []mcp.ToolInfo{{Name: "search"}},
				Prompts:   []mcp.PromptInfo{{Name: "triage", Description: "Triage an\nalert", Arguments: []mcp.PromptArg{{Name: "service", Required: true}, {Name: "notes"}}}, {Name: "summary"}},
				Resources: resources,
			},
			{Server: mcp.Server{Name: "offline", Scope: mcp.ScopeUser}, Status: mcp.StatusError, Prompts: []mcp.PromptInfo{{Name: "hidden"}}},
		},
		resources: map[string]string{
			"docs:file:///runbook.md": "restart the api",
			"docs:file:///big.log":    strings.Repeat("x", mcp.ResourceByteCap+500),
		},
	}
}

func menuCommands(model *TUIModel) []string {
	var commands []string
	for _, item := range model.commandMenu.Items() {
		commands = append(commands, item.(slashCommandItem).command)
	}
	return commands
}

func TestSlashPaletteListsMCPPromptsFromConnectedServers(t *testing.T) {
	model := newMCPTestModel(t, extrasManager(), &fakeConnectorAPI{})
	model.composer.SetValue("/")
	model.refreshCommandMenu()
	model.composer.SetValue("/docs")
	model.refreshCommandMenu()
	got := menuCommands(model)
	if !model.commandMenuOpen || strings.Join(got, ",") != "/docs:triage,/docs:summary" {
		t.Fatalf("menu = %v (open %v)", got, model.commandMenuOpen)
	}
	model.composer.SetValue("/off")
	model.refreshCommandMenu()
	if model.commandMenuOpen {
		t.Fatalf("prompts from disconnected servers listed: %v", menuCommands(model))
	}
}

func TestPromptItemsLoseToBuiltinsAndSkills(t *testing.T) {
	model := newMCPTestModel(t, extrasManager(), &fakeConnectorAPI{})
	items := model.appendPromptItems(nil, "/")
	if len(items) != 2 {
		t.Fatalf("items = %v", items)
	}
	skill := slashCommandItem{command: "/docs:triage", description: "skill", skill: true}
	items = model.appendPromptItems([]list.Item{skill}, "/docs:t")
	if len(items) != 1 || items[0].(slashCommandItem) != skill {
		t.Fatalf("skill should win on collision, got %v", items)
	}
	if !isBuiltinSlashCommand("/prompts") || !isBuiltinSlashCommand("/resources") {
		t.Fatal("/prompts and /resources must be built-ins so skills cannot shadow them")
	}
}

func TestRunMCPPromptArgumentsAndUsage(t *testing.T) {
	model := newMCPTestModel(t, extrasManager(), &fakeConnectorAPI{})
	cmd := model.runSlashCommand("/docs:triage")
	runCmd(model, cmd)
	if !strings.Contains(model.Transcript(), "Usage: /docs:triage <service> [notes]") {
		t.Fatalf("transcript = %s", model.Transcript())
	}
	cmd = model.runSlashCommand("/docs:triage api notes=urgent")
	msg, ok := cmd().(mcpPromptMsg)
	if !ok || msg.err != nil || msg.text != "prompt triage map[notes:urgent service:api]" {
		t.Fatalf("prompt msg = %+v", msg)
	}
	model.Update(msg)
	if !strings.Contains(model.Transcript(), "prompt triage map[notes:urgent service:api]") {
		t.Fatalf("prompt not sent: %s", model.Transcript())
	}
	if _, missing := promptArguments(mcp.PromptInfo{Arguments: []mcp.PromptArg{{Name: "service", Required: true}, {Name: "notes"}}}, []string{"api", "very", "slow"}); missing != nil {
		t.Fatalf("missing = %v", missing)
	}
	args, _ := promptArguments(mcp.PromptInfo{Arguments: []mcp.PromptArg{{Name: "service"}, {Name: "notes"}}}, []string{"api", "very", "slow"})
	if args["service"] != "api" || args["notes"] != "very slow" {
		t.Fatalf("args = %v", args)
	}
}

func TestPromptsAndResourcesListings(t *testing.T) {
	model := newMCPTestModel(t, extrasManager(), &fakeConnectorAPI{})
	model.runSlashCommand("/prompts")
	model.runSlashCommand("/resources")
	transcript := model.Transcript()
	for _, want := range []string{"/docs:triage <service> [notes]  Triage an alert", "/docs:summary", "@docs:file:///runbook.md  Runbook"} {
		if !strings.Contains(transcript, want) {
			t.Fatalf("missing %q in:\n%s", want, transcript)
		}
	}
	if strings.Contains(transcript, "hidden") {
		t.Fatalf("listed prompt from a disconnected server:\n%s", transcript)
	}
	empty := newMCPTestModel(t, &fakeMCPManager{}, &fakeConnectorAPI{})
	empty.runSlashCommand("/prompts")
	if !strings.Contains(empty.Transcript(), "No MCP prompts") {
		t.Fatalf("transcript = %s", empty.Transcript())
	}
}

func TestMentionPaletteShowsTopEightAndTabInserts(t *testing.T) {
	model := newMCPTestModel(t, extrasManager(), &fakeConnectorAPI{})
	model.composer.SetValue("check @docs")
	model.refreshCommandMenu()
	if got := menuCommands(model); !model.commandMenuOpen || len(got) != maxMentionSuggestions {
		t.Fatalf("menu = %v", got)
	}
	model.composer.SetValue("check @run")
	model.refreshCommandMenu()
	if got := menuCommands(model); len(got) != 1 || got[0] != "@docs:file:///runbook.md" {
		t.Fatalf("menu = %v", got)
	}
	model.handleCommandMenuKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if got := model.composer.Value(); got != "check @docs:file:///runbook.md " {
		t.Fatalf("composer = %q", got)
	}
	model.composer.SetValue("mail me@")
	model.refreshCommandMenu()
	if model.commandMenuOpen && len(menuCommands(model)) > maxMentionSuggestions {
		t.Fatal("menu too long")
	}
}

func TestMentionedResourcesAttachedWithCapAndNotes(t *testing.T) {
	manager := extrasManager()
	model := newMCPTestModel(t, manager, &fakeConnectorAPI{})
	if model.sendWithResources("email ops@example.com and @docs:unknown") != nil {
		t.Fatal("unknown mentions should send as plain text")
	}
	cmd := model.sendWithResources("why? @docs:file:///runbook.md, @docs:file:///big.log @docs:file:///gone.md @docs:file:///runbook.md")
	msg, ok := cmd().(mentionsReadyMsg)
	if !ok {
		t.Fatal("expected mentionsReadyMsg")
	}
	if strings.Join(msg.attached, "|") != "@docs:file:///runbook.md|@docs:file:///big.log (first 32 KB)" {
		t.Fatalf("attached = %v", msg.attached)
	}
	if len(msg.notes) != 1 || !strings.Contains(msg.notes[0], "skipped @docs:file:///gone.md: no text content") {
		t.Fatalf("notes = %v", msg.notes)
	}
	if !strings.HasPrefix(msg.message, "<resource server=\"docs\" uri=\"file:///runbook.md\">\nrestart the api\n</resource>") ||
		!strings.HasSuffix(msg.message, "\n\n"+msg.echo) || strings.Count(msg.message, "<resource ") != 2 {
		t.Fatalf("message = %.200q", msg.message)
	}
	if strings.Count(msg.message, "x") > mcp.ResourceByteCap+10 {
		t.Fatal("resource cap not applied")
	}
	for _, limit := range manager.readLimits {
		if limit != mcp.ResourceByteCap {
			t.Fatalf("read limit = %d", limit)
		}
	}
	model.Update(msg)
	transcript := model.Transcript()
	if !strings.Contains(transcript, "attached: @docs:file:///runbook.md, @docs:file:///big.log (first 32 KB)") || !strings.Contains(transcript, "skipped @docs:file:///gone.md") {
		t.Fatalf("transcript = %s", transcript)
	}
	if strings.Contains(transcript, "restart the api") {
		t.Fatal("resource body should not be echoed into the transcript")
	}
}

func TestMCPRowCounts(t *testing.T) {
	view := mcp.ServerView{Status: mcp.StatusConnected, Tools: make([]mcp.ToolInfo, 3), Resources: make([]mcp.ResourceInfo, 1), Prompts: make([]mcp.PromptInfo, 2)}
	if got := mcpStatusLabel(view); got != "✔ 3 tools · 1 resource · 2 prompts" {
		t.Fatalf("label = %q", got)
	}
	if got := mcpStatusLabel(mcp.ServerView{Status: mcp.StatusConnected, Tools: make([]mcp.ToolInfo, 1)}); got != "✔ 1 tool" {
		t.Fatalf("label = %q", got)
	}
}
