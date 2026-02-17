package views

import (
	"strings"
	"testing"
)

func TestNewAboutModel(t *testing.T) {
	m := NewAboutModel("1.0.0")

	if m.Visible {
		t.Error("expected about to be hidden initially")
	}

	if m.version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", m.version)
	}
}

func TestAboutModelToggle(t *testing.T) {
	m := NewAboutModel("1.0.0")

	m.Toggle()
	if !m.Visible {
		t.Error("expected about to be visible after toggle")
	}

	m.Toggle()
	if m.Visible {
		t.Error("expected about to be hidden after second toggle")
	}
}

func TestAboutModelShowHide(t *testing.T) {
	m := NewAboutModel("1.0.0")

	m.Show()
	if !m.Visible {
		t.Error("expected about to be visible after Show()")
	}

	m.Hide()
	if m.Visible {
		t.Error("expected about to be hidden after Hide()")
	}
}

func TestAboutModelView(t *testing.T) {
	m := NewAboutModel("2.1.0")

	view := m.View()

	// Should contain the version
	if !strings.Contains(view, "2.1.0") {
		t.Error("expected view to contain version")
	}

	// Should contain app name
	if !strings.Contains(view, "Rootly TUI") {
		t.Error("expected view to contain 'Rootly TUI'")
	}

	// Should contain GitHub link
	if !strings.Contains(view, "github.com") {
		t.Error("expected view to contain GitHub link")
	}

	// Should contain docs link
	if !strings.Contains(view, "rootly.com") {
		t.Error("expected view to contain docs link")
	}
}

func TestAboutModelViewContainsSystemInfo(t *testing.T) {
	m := NewAboutModel("1.0.0")

	view := m.View()

	// Should contain Go version info
	if !strings.Contains(view, "go") {
		t.Error("expected view to contain Go version info")
	}
}

func TestRenderAboutLine(t *testing.T) {
	result := renderAboutLine("Label", "Value")

	if result == "" {
		t.Error("expected non-empty result")
	}

	if !strings.Contains(result, "Label") {
		t.Error("expected result to contain label")
	}

	if !strings.Contains(result, "Value") {
		t.Error("expected result to contain value")
	}
}
