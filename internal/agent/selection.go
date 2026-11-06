package agent

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

type selectionPoint struct{ row, col int }
type selectionRow struct {
	text         string
	continuation bool
}
type selectionNoticeTick int

func (m *TUIModel) clearSelectionKey(msg tea.KeyPressMsg) bool {
	return msg.String() == "esc" && m.selection.active && !m.blocksTranscriptMouseInput()
}

type transcriptSelection struct {
	active, dragging bool
	anchor, focus    selectionPoint
	rows             []selectionRow
	content          string
	width            int
	lastClick        time.Time
	lastCell         selectionPoint
	clicks           int
	notice           string
	generation       int
}

func (m *TUIModel) selectionRows() []selectionRow {
	s := &m.selection
	w := max(1, m.viewport.Width())
	if s.content == m.viewportContent && s.width == w && s.rows != nil {
		return s.rows
	}
	var rows []selectionRow
	for _, line := range strings.Split(m.viewportContent, "\n") {
		for i := 0; i < max(1, ansi.StringWidth(line)); i += w {
			rows = append(rows, selectionRow{ansi.Cut(line, i, i+w), i > 0})
		}
	}
	if s.active {
		start, end := s.bounds()
		valid := s.width == w && end.row < len(rows)
		for i := start.row; valid && i <= end.row; i++ {
			valid = i < len(s.rows) && ansi.Strip(s.rows[i].text) == ansi.Strip(rows[i].text)
		}
		if !valid {
			s.active, s.dragging = false, false
		}
	}
	s.rows, s.content, s.width = rows, m.viewportContent, w
	return rows
}

func (s *transcriptSelection) bounds() (start, end selectionPoint) {
	a, b := s.anchor, s.focus
	if a.row > b.row || (a.row == b.row && a.col > b.col) {
		return b, a
	}
	return a, b
}

func (s *transcriptSelection) columns(row int) (left, right int) {
	a, b := s.bounds()
	left, right = 0, s.width
	if row == a.row {
		left = a.col
	}
	if row == b.row {
		right = b.col + 1
	}
	return left, right
}

func (m *TUIModel) selectionText() string {
	rows := m.selectionRows()
	s := &m.selection
	if !s.active {
		return ""
	}
	a, b := s.bounds()
	var out strings.Builder
	for i := a.row; i <= b.row; i++ {
		if i > a.row && !rows[i].continuation {
			out.WriteByte('\n')
		}
		left, right := s.columns(i)
		out.WriteString(strings.TrimRight(ansi.Strip(ansi.Cut(rows[i].text, left, right)), " \t"))
	}
	return out.String()
}

func (m *TUIModel) copySelection() tea.Cmd {
	text := m.selectionText()
	s := &m.selection
	s.generation++
	generation := s.generation
	s.notice = fmt.Sprintf("copied %d chars", utf8.RuneCountInString(text))
	return tea.Batch(tea.SetClipboard(text), tea.SetPrimaryClipboard(text), tea.Tick(2*time.Second, func(time.Time) tea.Msg { return selectionNoticeTick(generation) }))
}

func (m *TUIModel) selectMouse(message tea.MouseMsg) tea.Cmd {
	rows := m.selectionRows()
	s := &m.selection
	mouse := message.Mouse()
	point := selectionPoint{min(len(rows)-1, m.viewport.YOffset()+max(0, min(mouse.Y, m.viewport.Height()-1))), max(0, min(mouse.X, s.width-1))}
	switch message.(type) {
	case tea.MouseClickMsg:
		if mouse.Button != tea.MouseLeft || mouse.X < 0 || mouse.X >= s.width || mouse.Y < 0 || mouse.Y >= m.viewport.Height() {
			return nil
		}
		if point == s.lastCell && m.now().Sub(s.lastClick) <= 400*time.Millisecond {
			s.clicks = s.clicks%3 + 1
		} else {
			s.clicks = 1
		}
		s.lastClick, s.lastCell = m.now(), point
		s.active, s.dragging, s.anchor, s.focus = true, true, point, point
		if s.clicks == 2 {
			m.selectWord(point)
			s.dragging = false
			return m.copySelection()
		}
		if s.clicks == 3 {
			for s.anchor.row > 0 && rows[s.anchor.row].continuation {
				s.anchor.row--
			}
			for s.focus.row+1 < len(rows) && rows[s.focus.row+1].continuation {
				s.focus.row++
			}
			s.anchor.col, s.focus.col, s.dragging = 0, s.width-1, false
			return m.copySelection()
		}
	case tea.MouseMotionMsg:
		if !s.dragging {
			return nil
		}
		if mouse.Y >= m.viewport.Height() {
			m.viewport.ScrollDown(1)
		}
		if mouse.Y <= 0 && m.viewport.YOffset() > 0 {
			m.viewport.ScrollUp(1)
		}
		point.row = min(len(rows)-1, m.viewport.YOffset()+max(0, min(mouse.Y, m.viewport.Height()-1)))
		s.focus = point
		m.updateNewOutputIndicator()
	case tea.MouseReleaseMsg:
		if !s.dragging {
			return nil
		}
		s.dragging = false
		if s.anchor == s.focus {
			s.active = false
			return nil
		}
		return m.copySelection()
	}
	return nil
}

func wordCharacter(text string) bool {
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("_-./:@~#%+=?&", r) {
			return true
		}
	}
	return false
}

func (m *TUIModel) selectWord(point selectionPoint) {
	s := &m.selection
	g := uniseg.NewGraphemes(ansi.Strip(s.rows[point.row].text))
	type cell struct {
		start, end int
		word       bool
	}
	var cells []cell
	col, hit := 0, -1
	for g.Next() {
		width := g.Width()
		if point.col >= col && point.col < col+width {
			hit = len(cells)
		}
		cells = append(cells, cell{col, col + width - 1, wordCharacter(g.Str())})
		col += width
	}
	if hit < 0 {
		return
	}
	left, right := hit, hit
	if cells[hit].word {
		for left > 0 && cells[left-1].word {
			left--
		}
		for right+1 < len(cells) && cells[right+1].word {
			right++
		}
	}
	s.anchor.col, s.focus.col = cells[left].start, cells[right].end
}

func (m *TUIModel) highlightSelection(view string) string {
	m.selectionRows()
	s := &m.selection
	if !s.active || m.blocksTranscriptMouseInput() {
		return view
	}
	a, b := s.bounds()
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		row := m.viewport.YOffset() + i
		if row < a.row || row > b.row {
			continue
		}
		left, right := s.columns(row)
		// SGR reverse works even with the terminal's ASCII color profile.
		middle := strings.ReplaceAll(ansi.Cut(line, left, right), "\x1b[m", "\x1b[0m")
		middle = strings.ReplaceAll(middle, "\x1b[0m", "\x1b[0m\x1b[7m")
		lines[i] = ansi.Cut(line, 0, left) + "\x1b[7m" + middle + "\x1b[27m" + ansi.Cut(line, right, s.width)
	}
	return strings.Join(lines, "\n")
}
