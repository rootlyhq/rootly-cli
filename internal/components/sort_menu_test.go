package components

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

func TestNewSortMenu(t *testing.T) {
	options := []SortOption{
		{Label: "Option 1", Description: "Description 1", Value: 1},
		{Label: "Option 2", Description: "Description 2", Value: 2},
	}

	menu := NewSortMenu(options)

	if menu.visible {
		t.Error("expected menu to be hidden initially")
	}

	if menu.cursor != 0 {
		t.Errorf("expected cursor to be 0, got %d", menu.cursor)
	}

	if len(menu.options) != 2 {
		t.Errorf("expected 2 options, got %d", len(menu.options))
	}
}

func TestSortMenuToggle(t *testing.T) {
	options := []SortOption{
		{Label: "Option 1", Description: "Description 1", Value: 1},
	}
	menu := NewSortMenu(options)

	// Toggle to show
	menu.Toggle()
	if !menu.visible {
		t.Error("expected menu to be visible after toggle")
	}
	if menu.cursor != 0 {
		t.Errorf("expected cursor to be reset to 0, got %d", menu.cursor)
	}

	// Move cursor
	menu.cursor = 5

	// Toggle to hide
	menu.Toggle()
	if menu.visible {
		t.Error("expected menu to be hidden after second toggle")
	}

	// Toggle again should reset cursor
	menu.Toggle()
	if menu.cursor != 0 {
		t.Errorf("expected cursor to be reset to 0 on show, got %d", menu.cursor)
	}
}

func TestSortMenuIsVisible(t *testing.T) {
	options := []SortOption{
		{Label: "Option 1", Description: "Description 1", Value: 1},
	}
	menu := NewSortMenu(options)

	if menu.IsVisible() {
		t.Error("expected IsVisible to return false initially")
	}

	menu.Toggle()

	if !menu.IsVisible() {
		t.Error("expected IsVisible to return true after toggle")
	}
}

func TestSortMenuClose(t *testing.T) {
	options := []SortOption{
		{Label: "Option 1", Description: "Description 1", Value: 1},
	}
	menu := NewSortMenu(options)

	menu.Toggle()
	menu.Close()

	if menu.visible {
		t.Error("expected menu to be hidden after Close")
	}
}

func TestSortMenuHandleKeyDown(t *testing.T) {
	options := []SortOption{
		{Label: "Option 1", Description: "Description 1", Value: 1},
		{Label: "Option 2", Description: "Description 2", Value: 2},
		{Label: "Option 3", Description: "Description 3", Value: 3},
	}
	menu := NewSortMenu(options)
	menu.Toggle()

	// Move down
	selected, shouldApply := menu.HandleKey("down")
	if selected != nil {
		t.Error("expected no selection when moving down")
	}
	if shouldApply {
		t.Error("expected shouldApply to be false")
	}
	if menu.cursor != 1 {
		t.Errorf("expected cursor to be 1, got %d", menu.cursor)
	}

	// Move down again with 'j'
	menu.HandleKey("j")
	if menu.cursor != 2 {
		t.Errorf("expected cursor to be 2, got %d", menu.cursor)
	}

	// Try to move past last option
	menu.HandleKey("down")
	if menu.cursor != 2 {
		t.Errorf("expected cursor to stay at 2, got %d", menu.cursor)
	}
}

func TestSortMenuHandleKeyUp(t *testing.T) {
	options := []SortOption{
		{Label: "Option 1", Description: "Description 1", Value: 1},
		{Label: "Option 2", Description: "Description 2", Value: 2},
		{Label: "Option 3", Description: "Description 3", Value: 3},
	}
	menu := NewSortMenu(options)
	menu.Toggle()
	menu.cursor = 2

	// Move up
	selected, shouldApply := menu.HandleKey("up")
	if selected != nil {
		t.Error("expected no selection when moving up")
	}
	if shouldApply {
		t.Error("expected shouldApply to be false")
	}
	if menu.cursor != 1 {
		t.Errorf("expected cursor to be 1, got %d", menu.cursor)
	}

	// Move up again with 'k'
	menu.HandleKey("k")
	if menu.cursor != 0 {
		t.Errorf("expected cursor to be 0, got %d", menu.cursor)
	}

	// Try to move past first option
	menu.HandleKey("up")
	if menu.cursor != 0 {
		t.Errorf("expected cursor to stay at 0, got %d", menu.cursor)
	}
}

func TestSortMenuHandleKeyEnter(t *testing.T) {
	options := []SortOption{
		{Label: "Option 1", Description: "Description 1", Value: "field1"},
		{Label: "Option 2", Description: "Description 2", Value: "field2"},
	}
	menu := NewSortMenu(options)
	menu.Toggle()
	menu.cursor = 1

	selected, shouldApply := menu.HandleKey("enter")

	if !shouldApply {
		t.Error("expected shouldApply to be true")
	}

	if selected != "field2" {
		t.Errorf("expected selected to be 'field2', got %v", selected)
	}

	if menu.visible {
		t.Error("expected menu to be hidden after enter")
	}
}

func TestSortMenuHandleKeyEscape(t *testing.T) {
	options := []SortOption{
		{Label: "Option 1", Description: "Description 1", Value: 1},
	}
	menu := NewSortMenu(options)
	menu.Toggle()

	selected, shouldApply := menu.HandleKey("esc")

	if shouldApply {
		t.Error("expected shouldApply to be false")
	}

	if selected != nil {
		t.Errorf("expected selected to be nil, got %v", selected)
	}

	if menu.visible {
		t.Error("expected menu to be hidden after escape")
	}
}

func TestSortMenuHandleKeyQ(t *testing.T) {
	options := []SortOption{
		{Label: "Option 1", Description: "Description 1", Value: 1},
	}
	menu := NewSortMenu(options)
	menu.Toggle()

	selected, shouldApply := menu.HandleKey("q")

	if shouldApply {
		t.Error("expected shouldApply to be false")
	}

	if selected != nil {
		t.Errorf("expected selected to be nil, got %v", selected)
	}

	if menu.visible {
		t.Error("expected menu to be hidden after 'q'")
	}
}

func TestSortMenuHandleKeyUnknown(t *testing.T) {
	options := []SortOption{
		{Label: "Option 1", Description: "Description 1", Value: 1},
	}
	menu := NewSortMenu(options)
	menu.Toggle()
	initialCursor := menu.cursor

	selected, shouldApply := menu.HandleKey("x")

	if shouldApply {
		t.Error("expected shouldApply to be false for unknown key")
	}

	if selected != nil {
		t.Errorf("expected selected to be nil, got %v", selected)
	}

	if menu.cursor != initialCursor {
		t.Errorf("expected cursor to remain at %d, got %d", initialCursor, menu.cursor)
	}
}

func TestSortMenuRenderHidden(t *testing.T) {
	options := []SortOption{
		{Label: "Option 1", Description: "Description 1", Value: 1},
	}
	menu := NewSortMenu(options)

	output := menu.Render(nil, SortDesc)

	if output != "" {
		t.Error("expected empty string when menu is hidden")
	}
}

func TestSortMenuRenderVisible(t *testing.T) {
	options := []SortOption{
		{Label: "Option 1", Description: "Description 1", Value: 1},
		{Label: "Option 2", Description: "Description 2", Value: 2},
	}
	menu := NewSortMenu(options)
	menu.Toggle()

	output := menu.Render(nil, SortDesc)

	// Check for key elements
	expectedContent := []string{
		"Option 1",
		"Option 2",
		"Description 1",
	}

	for _, expected := range expectedContent {
		if !strings.Contains(output, expected) {
			t.Errorf("expected output to contain '%s'", expected)
		}
	}
}

func TestSortMenuRenderWithCursor(t *testing.T) {
	options := []SortOption{
		{Label: "Option 1", Description: "Description 1", Value: 1},
		{Label: "Option 2", Description: "Description 2", Value: 2},
	}
	menu := NewSortMenu(options)
	menu.Toggle()
	menu.cursor = 1

	output := menu.Render(nil, SortDesc)

	// Should show description for cursor position
	if !strings.Contains(output, "Description 2") {
		t.Error("expected output to contain description for cursor position")
	}

	// Should show cursor indicator
	if !strings.Contains(output, "▶") {
		t.Error("expected output to contain cursor indicator")
	}
}

func TestSortMenuRenderWithSortIndicator(t *testing.T) {
	options := []SortOption{
		{Label: "Option 1", Description: "Description 1", Value: 1},
		{Label: "Option 2", Description: "Description 2", Value: 2},
	}
	menu := NewSortMenu(options)
	menu.Toggle()

	// Render with descending sort on option 1
	output := menu.Render(1, SortDesc)
	if !strings.Contains(output, "Newest First") {
		t.Error("expected output to contain descending indicator")
	}

	// Render with ascending sort on option 2
	output = menu.Render(2, SortAsc)
	if !strings.Contains(output, "Oldest First") {
		t.Error("expected output to contain ascending indicator")
	}
}

func TestSortMenuRenderNoSortIndicator(t *testing.T) {
	options := []SortOption{
		{Label: "Option 1", Description: "Description 1", Value: 1},
		{Label: "Option 2", Description: "Description 2", Value: 2},
	}
	menu := NewSortMenu(options)
	menu.Toggle()

	// Render with sort on different field (not in options)
	output := menu.Render(3, SortDesc)

	// Check that sort indicators are not present
	if strings.Contains(output, "Newest First") || strings.Contains(output, "Oldest First") {
		t.Errorf("expected no sort indicators when current sort field doesn't match options")
	}
}

func TestSortMenuWithEmptyOptions(t *testing.T) {
	menu := NewSortMenu([]SortOption{})
	menu.Toggle()

	// Should handle navigation gracefully
	menu.HandleKey("down")
	menu.HandleKey("up")

	output := menu.Render(nil, SortDesc)
	if output == "" {
		t.Error("expected output even with empty options")
	}
}

func TestSortMenuCursorReset(t *testing.T) {
	options := []SortOption{
		{Label: "Option 1", Description: "Description 1", Value: 1},
		{Label: "Option 2", Description: "Description 2", Value: 2},
	}
	menu := NewSortMenu(options)

	// Move cursor
	menu.Toggle()
	menu.HandleKey("down")
	if menu.cursor != 1 {
		t.Errorf("expected cursor to be 1, got %d", menu.cursor)
	}

	// Hide and show again - cursor should reset
	menu.Toggle()
	menu.Toggle()

	if menu.cursor != 0 {
		t.Errorf("expected cursor to be reset to 0, got %d", menu.cursor)
	}
}

func TestSortOptionStruct(t *testing.T) {
	opt := SortOption{
		Label:       "Test Label",
		Description: "Test Description",
		Value:       42,
	}

	if opt.Label != "Test Label" {
		t.Errorf("expected Label to be 'Test Label', got '%s'", opt.Label)
	}

	if opt.Description != "Test Description" {
		t.Errorf("expected Description to be 'Test Description', got '%s'", opt.Description)
	}

	if opt.Value != 42 {
		t.Errorf("expected Value to be 42, got %v", opt.Value)
	}
}
