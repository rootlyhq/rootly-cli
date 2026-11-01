package agent

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/rootlyhq/rootly-cli/internal/mcp"
)

func stubMarket(t *testing.T) *int {
	t.Helper()
	calls := 0
	previous := loadMarketCatalog
	loadMarketCatalog = func(context.Context, bool) ([]mcp.MarketEntry, error) {
		calls++
		return mcp.Catalog([]mcp.MarketEntry{{Name: "pager-mcp", Description: "Paging tool", Author: "acme", Config: mcp.ServerConfig{Command: "npx", Args: []string{"-y", "@acme/pager-mcp"}}}}), nil
	}
	t.Cleanup(func() { loadMarketCatalog = previous })
	return &calls
}

func typeKeys(model *TUIModel, text string) {
	for _, char := range text {
		model.Update(tea.KeyPressMsg{Code: char, Text: string(char)})
	}
}

func TestMarketplaceSearchPreviewAndAddToUserScopeOff(t *testing.T) {
	calls := stubMarket(t)
	manager := &fakeMCPManager{views: testViews(), changes: make(chan struct{})}
	model := newMCPTestModel(t, manager, &fakeConnectorAPI{})
	model.mcpPage.open = true
	_, cmd := model.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	runCmd(model, cmd)
	if *calls != 1 || model.mcpPage.market == nil {
		t.Fatalf("marketplace did not open: calls=%d", *calls)
	}
	view := model.mcpPageView()
	if !strings.Contains(view, "MCP marketplace") || strings.Index(view, "datadog ★") > strings.Index(view, "github") {
		t.Fatalf("Rootly vendors should be listed first:\n%s", view)
	}
	typeKeys(model, "pager")
	if got := model.mcpPage.market.results; len(got) == 0 || got[0].Name != "pager-mcp" {
		t.Fatalf("results = %+v", got)
	}
	model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	preview := model.mcpPageView()
	for _, want := range []string{"Add pager-mcp to your user MCPs?", "Runs: npx -y @acme/pager-mcp", "Writes to: ~/.rootly-cli/mcp.json (user scope)", "nothing runs until you turn it on"} {
		if !strings.Contains(preview, want) {
			t.Fatalf("preview missing %q:\n%s", want, preview)
		}
	}
	if len(manager.added) != 0 {
		t.Fatal("nothing may be added before y")
	}
	model.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if len(manager.added) != 1 || manager.added[0].Name != "pager-mcp" || model.mcpPage.market != nil {
		t.Fatalf("added = %+v market=%v", manager.added, model.mcpPage.market)
	}
	if !strings.Contains(model.mcpPage.message, "Added pager-mcp") || !strings.Contains(model.mcpPage.message, "(off)") {
		t.Fatalf("message = %q", model.mcpPage.message)
	}
	if row, ok := model.selectedMCPRow(); !ok || row.view.Server.Name != "pager-mcp" || row.view.Status != mcp.StatusOff {
		t.Fatalf("cursor should land on the new server, switched off: %+v", row)
	}
}

func TestMarketplaceDeclineAndDuplicate(t *testing.T) {
	stubMarket(t)
	manager := &fakeMCPManager{views: testViews(), changes: make(chan struct{})}
	model := newMCPTestModel(t, manager, &fakeConnectorAPI{})
	model.mcpPage.open = true
	_, cmd := model.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	runCmd(model, cmd)
	model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if model.mcpPage.market.preview != nil || len(manager.added) != 0 {
		t.Fatal("n should go back without adding")
	}
	typeKeys(model, "sentry")
	model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if len(manager.added) != 0 || !strings.Contains(model.mcpPage.market.err, "already exists") {
		t.Fatalf("duplicate should be refused: added=%v err=%q", manager.added, model.mcpPage.market.err)
	}
	model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.mcpPage.market != nil || !model.mcpPage.open {
		t.Fatal("Esc should return to the /mcp list")
	}
}

func TestMarketplaceIgnoresLoadsFromEarlierVisits(t *testing.T) {
	stubMarket(t)
	manager := &fakeMCPManager{views: testViews(), changes: make(chan struct{})}
	model := newMCPTestModel(t, manager, &fakeConnectorAPI{})
	model.mcpPage.open = true
	model.openMarket()
	first := model.mcpPage.market.request
	model.mcpPage.market = nil
	model.openMarket()
	if model.mcpPage.market.request == first {
		t.Fatal("a new marketplace visit must not reuse the previous request id")
	}
	model.handleMarketLoaded(marketLoadedMsg{request: first, catalog: []mcp.MarketEntry{{Name: "stale"}}})
	if model.mcpPage.market.catalog != nil || !model.mcpPage.market.loading {
		t.Fatalf("stale load populated the new visit: %+v", model.mcpPage.market)
	}
}

func TestMarketplacePreviewIgnoresEnter(t *testing.T) {
	stubMarket(t)
	manager := &fakeMCPManager{views: testViews(), changes: make(chan struct{})}
	model := newMCPTestModel(t, manager, &fakeConnectorAPI{})
	model.mcpPage.open = true
	_, cmd := model.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	runCmd(model, cmd)
	typeKeys(model, "pager")
	model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(manager.added) != 0 || model.mcpPage.market == nil || model.mcpPage.market.preview == nil {
		t.Fatalf("Enter in the preview must not add: added=%+v", manager.added)
	}
}
