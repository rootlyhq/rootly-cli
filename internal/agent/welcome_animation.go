package agent

import (
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	welcomeDrawSpeed           = 1.2
	welcomeBannerFrameInterval = 45 * time.Millisecond
	welcomeGlimmerCenter       = 3
)

var (
	welcomeCrest   = []string{"#FFFFFF", "#E0DAFB", "#C9BEF7", "#9D86F0", "#7C5CE6"}
	welcomeGlimmer = []string{
		"#9D86F0", "#C9BEF7", "#E0DAFB", "#FFFFFF", "#E0DAFB", "#C9BEF7", "#9D86F0",
	}
	welcomeDrawFrames  = int(math.Ceil((welcomeDrawMaxDistance() + float64(len(welcomeCrest))) / welcomeDrawSpeed))
	welcomeSettleFrame = welcomeDrawFrames + welcomeGlimmerFrames() + 4
)

type welcomeBannerTickMsg struct{}

func (m *TUIModel) welcomeBannerTick() tea.Cmd {
	return tea.Tick(welcomeBannerFrameInterval, func(time.Time) tea.Msg {
		return welcomeBannerTickMsg{}
	})
}

func welcomeDrawMaxDistance() float64 {
	maxDistance := 0.0
	for row, line := range welcomeLogoLines {
		for column, character := range []rune(line) {
			if character == ' ' {
				continue
			}
			if distance := welcomeDrawDistance(row, column); distance > maxDistance {
				maxDistance = distance
			}
		}
	}
	return maxDistance
}

func welcomeDrawDistance(_, column int) float64 {
	return float64(column)
}

func welcomeCellColor(frame, row, column int) string {
	if frame >= welcomeSettleFrame {
		return "#7C5CE6"
	}
	if frame > welcomeDrawFrames && frame <= welcomeDrawFrames+welcomeGlimmerFrames() {
		phase := (frame - welcomeDrawFrames - 1) * 2
		index := column - phase + welcomeGlimmerCenter
		if index >= 0 && index < len(welcomeGlimmer) {
			return welcomeGlimmer[index]
		}
		return "#7C5CE6"
	}
	if frame > welcomeDrawFrames {
		return "#7C5CE6"
	}
	edge := float64(frame)*welcomeDrawSpeed - welcomeDrawDistance(row, column)
	if edge < 0 {
		return ""
	}
	return welcomeCrest[min(int(math.Floor(edge)), len(welcomeCrest)-1)]
}

func welcomeGlimmerPeriod() int {
	return welcomeLogoWidth + welcomeLogoHeight + 16
}

func welcomeGlimmerFrames() int {
	return (welcomeGlimmerPeriod() + 1) / 2
}

func renderPulseFrame(frame int) string {
	lines := make([]string, len(welcomeLogoLines))
	for row, line := range welcomeLogoLines {
		var rendered strings.Builder
		for column, character := range []rune(line) {
			if character == ' ' {
				rendered.WriteRune(character)
				continue
			}
			color := welcomeCellColor(frame, row, column)
			if color == "" {
				rendered.WriteRune(' ')
				continue
			}
			rendered.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(string(character)))
		}
		lines[row] = rendered.String()
	}
	return strings.Join(lines, "\n")
}
