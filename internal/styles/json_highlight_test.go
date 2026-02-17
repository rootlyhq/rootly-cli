package styles

import "testing"

func TestHighlightJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "simple object",
			input: `{"key": "value"}`,
		},
		{
			name:  "numbers",
			input: `{"count": 42}`,
		},
		{
			name:  "booleans",
			input: `{"active": true, "disabled": false}`,
		},
		{
			name:  "null",
			input: `{"data": null}`,
		},
		{
			name:  "array",
			input: `{"items": [1, 2, 3]}`,
		},
		{
			name:  "nested",
			input: `{"outer": {"inner": "value"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HighlightJSON(tt.input)
			if result == "" {
				t.Error("Highlighted JSON should not be empty")
			}
		})
	}
}

func TestIsJSONDelimiter(t *testing.T) {
	tests := []struct {
		char     byte
		expected bool
	}{
		{',', true},
		{'}', true},
		{']', true},
		{' ', true},
		{'\n', true},
		{'\t', true},
		{'{', false},
		{'[', false},
		{'"', false},
		{'a', false},
		{'1', false},
	}

	for _, tt := range tests {
		result := isJSONDelimiter(tt.char)
		if result != tt.expected {
			t.Errorf("isJSONDelimiter(%q) = %v, expected %v", tt.char, result, tt.expected)
		}
	}
}
