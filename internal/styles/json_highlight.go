// Package styles provides lipgloss styles for the TUI.
package styles

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// JSON syntax highlighting styles.
var (
	jsonKeyStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#88C0D0")) // Light blue for keys.
	jsonStringStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#A3BE8C")) // Green for strings.
	jsonNumberStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#B48EAD")) // Purple for numbers.
	jsonBoolStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#EBCB8B")) // Yellow for booleans.
	jsonNullStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#BF616A")) // Red for null.
	jsonPunctStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#D8DEE9")) // Light gray for punctuation.
)

// HighlightJSON applies syntax highlighting to JSON string.
// It parses the JSON character by character and applies appropriate
// colors for keys, strings, numbers, booleans, null, and punctuation.
func HighlightJSON(jsonStr string) string {
	var result strings.Builder
	inString := false
	isKey := true
	var currentToken strings.Builder

	for i := 0; i < len(jsonStr); i++ {
		c := jsonStr[i]

		if c == '"' {
			if inString {
				// End of string.
				currentToken.WriteByte(c)
				token := currentToken.String()
				if isKey {
					result.WriteString(jsonKeyStyle.Render(token))
				} else {
					result.WriteString(jsonStringStyle.Render(token))
				}
				currentToken.Reset()
				inString = false
			} else {
				// Start of string.
				inString = true
				currentToken.WriteByte(c)
			}
			continue
		}

		if inString {
			currentToken.WriteByte(c)
			continue
		}

		// Outside of string.
		switch c {
		case ':':
			result.WriteString(jsonPunctStyle.Render(string(c)))
			isKey = false
		case ',':
			result.WriteString(jsonPunctStyle.Render(string(c)))
			isKey = true
		case '{', '}', '[', ']':
			result.WriteString(jsonPunctStyle.Render(string(c)))
			if c == '{' || c == '[' {
				isKey = (c == '{')
			}
		case ' ', '\n', '\t':
			result.WriteByte(c)
		default:
			// Collect non-string tokens (numbers, booleans, null).
			start := i
			for i < len(jsonStr) && !isJSONDelimiter(jsonStr[i]) {
				i++
			}
			token := jsonStr[start:i]
			i-- // Adjust for loop increment.

			// Determine token type.
			switch token {
			case "true", "false":
				result.WriteString(jsonBoolStyle.Render(token))
			case "null":
				result.WriteString(jsonNullStyle.Render(token))
			default:
				// Assume number.
				result.WriteString(jsonNumberStyle.Render(token))
			}
		}
	}

	return result.String()
}

// isJSONDelimiter checks if a character is a JSON delimiter.
func isJSONDelimiter(c byte) bool {
	return c == ',' || c == '}' || c == ']' || c == ' ' || c == '\n' || c == '\t'
}
