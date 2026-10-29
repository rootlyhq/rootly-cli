package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	rootly "github.com/rootlyhq/rootly-go"
)

type confirmationPreview struct {
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	ContextText string `json:"context_text"`
	Fields      []struct {
		Label    string `json:"label"`
		Value    any    `json:"value"`
		Inferred bool   `json:"inferred"`
	} `json:"fields"`
	Destructive    bool            `json:"destructive"`
	InferredFields json.RawMessage `json:"inferred_fields"`
}

func renderConfirmationCard(confirmation *rootly.AiChatStreamConfirmationRequired, width int, noColor bool, now time.Time) string {
	if confirmation == nil {
		return ""
	}

	lines := []string{sanitizeTerminalText(confirmation.Question)}
	preview, validPreview := parseConfirmationPreview(confirmation.Preview)
	if validPreview {
		if preview.Title != "" {
			lines = append(lines, sanitizeTerminalText(preview.Title))
		}
		if preview.Summary != "" {
			lines = append(lines, sanitizeTerminalText(preview.Summary))
		}
		if preview.ContextText != "" {
			lines = append(lines, sanitizeTerminalText(preview.ContextText))
		}
		inferredFields := confirmationInferredFields(preview.InferredFields)
		for _, field := range preview.Fields {
			label := sanitizeTerminalText(field.Label)
			value := sanitizeTerminalText(confirmationValue(field.Value))
			row := fmt.Sprintf("%s: %s", label, value)
			if field.Inferred || inferredFields[label] {
				if noColor {
					row += " (inferred)"
				} else {
					row += lipgloss.NewStyle().Faint(true).Render(" (inferred)")
				}
			}
			lines = append(lines, row)
		}
		missing := confirmationMissingFields(confirmation.MissingFields)
		for index := range missing {
			missing[index] = sanitizeTerminalText(missing[index])
		}
		if len(missing) > 0 {
			lines = append(lines, "Missing: "+strings.Join(missing, ", "))
		}
		if preview.Destructive {
			tag := "DESTRUCTIVE"
			if !noColor {
				tag = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true).Render(tag)
			}
			lines = append(lines, tag)
		}
	} else if len(confirmation.Preview) > 0 {
		lines = append(lines, strings.Split(sanitizedConfirmationPreview(confirmation.Preview), "\n")...)
	}

	footer := "y approve · n/Esc cancel"
	if remaining, ok := confirmationRemaining(confirmation.ExpiresAt, now); ok {
		footer += " · expires in " + remaining
	}
	if !noColor {
		lines[0] = lipgloss.NewStyle().Bold(true).Render(lines[0])
		footer = lipgloss.NewStyle().Faint(true).Render(footer)
	}
	lines = append(lines, footer)

	content := strings.Join(lines, "\n")
	if noColor {
		return indent(content, "  ")
	}

	cardWidth := min(max(width, 1), 80)
	innerWidth := max(1, cardWidth-4)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Width(innerWidth).
		MaxWidth(innerWidth).
		Render(content)
}

func parseConfirmationPreview(raw json.RawMessage) (confirmationPreview, bool) {
	var preview confirmationPreview
	if len(raw) == 0 || json.Unmarshal(raw, &preview) != nil || len(preview.Fields) == 0 {
		return confirmationPreview{}, false
	}
	return preview, true
}

func confirmationValue(value any) string {
	switch value := value.(type) {
	case nil:
		return ""
	case string:
		return value
	case map[string]any, []any:
		encoded, err := json.Marshal(sanitizeConfirmationJSONValue(value))
		if err == nil {
			return string(encoded)
		}
	}
	return fmt.Sprint(value)
}

func sanitizedConfirmationPreview(raw []byte) string {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return sanitizeTerminalText(string(raw))
	}
	encoded, err := json.Marshal(sanitizeConfirmationJSONValue(value))
	if err != nil {
		return sanitizeTerminalText(string(raw))
	}
	var indented bytes.Buffer
	if err := json.Indent(&indented, encoded, "", "  "); err != nil {
		return sanitizeTerminalText(string(encoded))
	}
	return indented.String()
}

func sanitizeConfirmationJSONValue(value any) any {
	switch value := value.(type) {
	case string:
		return sanitizeTerminalText(value)
	case []any:
		sanitized := make([]any, len(value))
		for index, item := range value {
			sanitized[index] = sanitizeConfirmationJSONValue(item)
		}
		return sanitized
	case map[string]any:
		sanitized := make(map[string]any, len(value))
		for key, item := range value {
			sanitized[sanitizeTerminalText(key)] = sanitizeConfirmationJSONValue(item)
		}
		return sanitized
	default:
		return value
	}
}

func confirmationInferredFields(raw json.RawMessage) map[string]bool {
	inferred := make(map[string]bool)
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) != nil {
		return inferred
	}
	for _, value := range values {
		var name string
		if json.Unmarshal(value, &name) == nil {
			inferred[sanitizeTerminalText(name)] = true
			continue
		}
		var field map[string]any
		if json.Unmarshal(value, &field) == nil {
			if name, ok := field["label"].(string); ok {
				inferred[sanitizeTerminalText(name)] = true
			} else if name, ok := field["name"].(string); ok {
				inferred[sanitizeTerminalText(name)] = true
			}
		}
	}
	return inferred
}

func confirmationMissingFields(raw json.RawMessage) []string {
	var fields []map[string]any
	if json.Unmarshal(raw, &fields) != nil {
		return nil
	}
	missing := make([]string, 0, len(fields))
	for _, field := range fields {
		if label, ok := field["label"].(string); ok && label != "" {
			missing = append(missing, label)
		} else if name, ok := field["name"].(string); ok && name != "" {
			missing = append(missing, name)
		}
	}
	return missing
}

func confirmationRemaining(expiresAt string, now time.Time) (string, bool) {
	if expiresAt == "" {
		return "", false
	}
	expiration, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return "", false
	}
	remaining := expiration.Sub(now)
	if remaining >= time.Minute {
		return fmt.Sprintf("%dm", int((remaining+time.Minute-1)/time.Minute)), true
	}
	if remaining <= 0 {
		return "0s", true
	}
	return fmt.Sprintf("%ds", int((remaining+time.Second-1)/time.Second)), true
}
