package views

import (
	"os"
	"strings"
	"testing"

	"github.com/rootlyhq/rootly-tui/internal/i18n"
)

func TestMain(m *testing.M) {
	// Set language to English for consistent test output
	i18n.SetLanguage(i18n.LangEnglish)
	os.Exit(m.Run())
}

func TestNewHelpModel(t *testing.T) {
	m := NewHelpModel()

	if m.Visible {
		t.Error("expected help to be hidden initially")
	}
}

func TestHelpModelToggle(t *testing.T) {
	m := NewHelpModel()

	m.Toggle()
	if !m.Visible {
		t.Error("expected help to be visible after toggle")
	}

	m.Toggle()
	if m.Visible {
		t.Error("expected help to be hidden after second toggle")
	}
}

func TestHelpModelShow(t *testing.T) {
	m := NewHelpModel()

	m.Show()
	if !m.Visible {
		t.Error("expected help to be visible after Show()")
	}
}

func TestHelpModelHide(t *testing.T) {
	m := NewHelpModel()
	m.Visible = true

	m.Hide()
	if m.Visible {
		t.Error("expected help to be hidden after Hide()")
	}
}

func TestHelpModelView(t *testing.T) {
	m := NewHelpModel()
	view := m.View()

	// Check for key sections
	expectedContent := []string{
		"Keyboard Shortcuts",
		"Navigation",
		"j / Down",
		"k / Up",
		"Tab",
		"Actions",
		"Refresh",
		"General",
		"Quit",
	}

	for _, expected := range expectedContent {
		if !strings.Contains(view, expected) {
			t.Errorf("expected help view to contain '%s'", expected)
		}
	}
}

func TestRenderHelpBar(t *testing.T) {
	bar := RenderHelpBar(80, false, false, false, 1, 10, 100)

	expectedItems := []string{
		"navigate",
		"switch",
		"refresh",
		"help",
		"quit",
	}

	for _, item := range expectedItems {
		if !strings.Contains(bar, item) {
			t.Errorf("expected help bar to contain '%s'", item)
		}
	}

	// 'open' should not be shown when hasSelection is false
	if strings.Contains(bar, "open") {
		t.Error("expected help bar to NOT contain 'open' when hasSelection is false")
	}

	// 'sort' should not be shown when isIncidentsTab is false
	if strings.Contains(bar, "sort") {
		t.Error("expected help bar to NOT contain 'sort' when isIncidentsTab is false")
	}
}

func TestRenderHelpBarWithSelection(t *testing.T) {
	bar := RenderHelpBar(80, true, false, false, 1, 10, 100)

	// 'open' should be shown when hasSelection is true
	if !strings.Contains(bar, "open") {
		t.Error("expected help bar to contain 'open' when hasSelection is true")
	}

	// 'refresh' should be shown when not loading
	if !strings.Contains(bar, "refresh") {
		t.Error("expected help bar to contain 'refresh' when not loading")
	}
}

func TestRenderHelpBarWhileLoading(t *testing.T) {
	bar := RenderHelpBar(80, true, true, false, 1, 10, 100)

	// 'refresh' should NOT be shown when loading
	if strings.Contains(bar, "refresh") {
		t.Error("expected help bar to NOT contain 'refresh' when loading")
	}

	// 'open' should NOT be shown when loading (even with selection)
	if strings.Contains(bar, "open") {
		t.Error("expected help bar to NOT contain 'open' when loading")
	}
}

func TestRenderHelpBarWithIncidentsTab(t *testing.T) {
	bar := RenderHelpBar(80, false, false, true, 1, 10, 100)

	// 'sort' should be shown when isIncidentsTab is true
	if !strings.Contains(bar, "sort") {
		t.Error("expected help bar to contain 'sort' when isIncidentsTab is true")
	}
}

func TestRenderHelpBarWithoutIncidentsTab(t *testing.T) {
	bar := RenderHelpBar(80, false, false, false, 1, 10, 100)

	// 'sort' should NOT be shown when isIncidentsTab is false
	if strings.Contains(bar, "sort") {
		t.Error("expected help bar to NOT contain 'sort' when isIncidentsTab is false")
	}
}
