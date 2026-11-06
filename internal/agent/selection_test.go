package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func selectionModel(text string, width, height int) *TUIModel {
	m := NewTUIModel(context.Background(), &fakeStreamer{}, TUIOptions{NoColor: true})
	m.viewport.SetWidth(width)
	m.viewport.SetHeight(height)
	m.viewportContent = text
	m.viewport.SetContent(text)
	m.now = func() time.Time { return time.Unix(100, 0) }
	return m
}

func click(m *TUIModel, x, y int) tea.Cmd {
	_, cmd := m.updateMouse(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	return cmd
}

func assertCopy(t *testing.T, cmd tea.Cmd, want string) {
	t.Helper()
	if cmd == nil {
		t.Fatal("missing clipboard command")
	}
	batch := cmd().(tea.BatchMsg)
	for i, kind := range []string{"tea.setClipboardMsg", "tea.setPrimaryClipboardMsg"} {
		msg := batch[i]()
		if fmt.Sprintf("%T", msg) != kind || fmt.Sprint(msg) != want {
			t.Fatalf("clipboard = %T %q, want %q", msg, msg, want)
		}
	}
}

func TestSelectionDrag(t *testing.T) {
	m := selectionModel("zero\n\x1b[31mone \u00a0\x1b[0m\ntwo  \nthree\nfour", 10, 2)
	m.viewport.SetYOffset(1)
	click(m, 0, 0)
	m.updateMouse(tea.MouseMotionMsg{X: 4, Y: 1, Button: tea.MouseLeft})
	_, cmd := m.updateMouse(tea.MouseReleaseMsg{Button: tea.MouseLeft})
	assertCopy(t, cmd, "one\ntwo")
	if !strings.Contains(m.footerContent(), "copied 7 chars") {
		t.Fatal(m.footerContent())
	}
	if !strings.Contains(m.highlightSelection(m.viewport.View()), "\x1b[7m") {
		t.Fatal("no highlight")
	}
	generation := m.selection.generation
	tick := cmd().(tea.BatchMsg)[2]()
	if tick != selectionNoticeTick(generation) {
		t.Fatalf("tick = %v", tick)
	}
	m.Update(selectionNoticeTick(generation - 1))
	if m.selection.notice == "" {
		t.Fatal("stale tick cleared notice")
	}
	m.Update(selectionNoticeTick(generation))
	if m.selection.notice != "" {
		t.Fatal("notice remained")
	}
	m.updateKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.selection.active {
		t.Fatal("escape did not clear")
	}
	click(m, 2, 1)
	m.updateMouse(tea.MouseReleaseMsg{Button: tea.MouseLeft})
	if m.selection.active {
		t.Fatal("plain click remained")
	}
}

func TestSelectionWrappedClicksAndChanges(t *testing.T) {
	m := selectionModel("hello world/path\nnext", 8, 4)
	click(m, 1, 0)
	assertCopy(t, click(m, 1, 0), "hello")
	assertCopy(t, click(m, 1, 0), "hello world/path")
	if got := m.selectionText(); got != "hello world/path" {
		t.Fatal(got)
	}
	click(m, 1, 0)
	if m.selection.clicks != 1 || !m.selection.dragging {
		t.Fatal("fourth click did not reset")
	}
	m.selection.anchor, m.selection.focus = selectionPoint{1, 6}, selectionPoint{0, 0}
	m.viewportContent += "\nappend"
	m.selectionRows()
	if !m.selection.active {
		t.Fatal("append cleared selection")
	}
	m.viewportContent = "changed"
	m.selectionRows()
	if m.selection.active {
		t.Fatal("changed content retained selection")
	}
	click(m, 0, 0)
	m.viewport.SetWidth(7)
	m.selectionRows()
	if m.selection.active {
		t.Fatal("resize retained selection")
	}
}

func TestSelectionWordCells(t *testing.T) {
	for _, tc := range []struct {
		text string
		x    int
		want string
	}{
		{"go https://host/a?b=2!", 8, "https://host/a?b=2"},
		{"a 世界!", 4, "世界"}, {"a ! b", 2, "!"}, {"a", 8, ""},
	} {
		m := selectionModel(tc.text, 30, 2)
		click(m, tc.x, 0)
		assertCopy(t, click(m, tc.x, 0), tc.want)
	}
}

func TestSelectionMouseGatesAndScroll(t *testing.T) {
	for _, gate := range []func(*TUIModel){
		func(m *TUIModel) { m.commandMenuOpen = true },
		func(m *TUIModel) { m.mcpPage.open = true },
		func(m *TUIModel) { m.state = tuiConfirming },
		func(m *TUIModel) { m.noMouse = true },
	} {
		m := selectionModel("one\ntwo\nthree\nfour", 10, 2)
		gate(m)
		click(m, 0, 0)
		if m.selection.active {
			t.Fatal("overlay accepted selection")
		}
	}
	m := selectionModel("one\ntwo\nthree\nfour", 10, 2)
	for _, p := range [][2]int{{0, 2}, {-1, 0}, {10, 0}, {0, -1}} {
		click(m, p[0], p[1])
	}
	if m.selection.active {
		t.Fatal("outside click accepted")
	}
	m.updateMouse(tea.MouseMotionMsg{})
	m.updateMouse(tea.MouseReleaseMsg{})
	click(m, 0, 0)
	m.updateMouse(tea.MouseMotionMsg{X: 1, Y: 3})
	if m.viewport.YOffset() != 1 {
		t.Fatal("no bottom auto-scroll")
	}
	m.updateMouse(tea.MouseMotionMsg{X: 1, Y: 0})
	if m.viewport.YOffset() != 0 {
		t.Fatal("no top auto-scroll")
	}
	m.updateMouse(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if m.viewport.YOffset() == 0 {
		t.Fatal("wheel down failed")
	}
	m.updateMouse(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if m.viewport.YOffset() != 0 {
		t.Fatal("wheel up failed")
	}
	if m.View().MouseMode != tea.MouseModeCellMotion {
		t.Fatal("default mouse mode")
	}
	m.noMouse = true
	if m.View().MouseMode != tea.MouseModeNone {
		t.Fatal("no-mouse mode")
	}
}
