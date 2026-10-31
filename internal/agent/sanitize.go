package agent

import (
	"strings"
	"unicode/utf8"
)

func sanitizeTerminalText(text string) string {
	var sanitized strings.Builder
	sanitized.Grow(len(text))
	for index := 0; index < len(text); {
		character := rune(text[index])
		size := 1
		if character >= 0x80 {
			character, size = utf8.DecodeRuneInString(text[index:])
		}
		if character == utf8.RuneError && size == 1 && text[index] >= 0x80 && text[index] <= 0x9f {
			index += size
			continue
		}
		if character == 0x1b {
			index += size
			index = skipEscapeSequence(text, index)
			continue
		}
		if character == '\n' || character == '\t' {
			sanitized.WriteRune(character)
			index += size
			continue
		}
		if character <= 0x1f || character >= 0x7f && character <= 0x9f {
			index += size
			continue
		}
		switch character {
		case 0x061c, 0x200e, 0x200f:
			index += size
			continue
		}
		if character >= 0x202a && character <= 0x202e || character >= 0x2066 && character <= 0x2069 {
			index += size
			continue
		}
		sanitized.WriteString(text[index : index+size])
		index += size
	}
	return sanitized.String()
}

func skipEscapeSequence(text string, index int) int {
	if index >= len(text) {
		return index
	}
	switch text[index] {
	case '[':
		return skipControlSequence(text, index+1)
	case ']', 'P', '^', '_', 'X':
		return skipStringControl(text, index+1)
	default:
		return index
	}
}

func skipControlSequence(text string, index int) int {
	for index < len(text) {
		character := text[index]
		index++
		if character >= 0x40 && character <= 0x7e {
			return index
		}
	}
	return index
}

func skipStringControl(text string, index int) int {
	for index < len(text) {
		if strings.HasPrefix(text[index:], "\u009c") {
			return index + len("\u009c")
		}
		switch text[index] {
		case '\a':
			return index + 1
		case 0x1b:
			if index+1 < len(text) && text[index+1] == '\\' {
				return index + 2
			}
		}
		index++
	}
	return index
}

type sanitizedTerminalError struct {
	message string
	cause   error
}

func (err sanitizedTerminalError) Error() string {
	return err.message
}

func (err sanitizedTerminalError) Unwrap() error {
	return err.cause
}

func sanitizeTerminalError(err error) error {
	if err == nil {
		return nil
	}
	original := err.Error()
	message := sanitizeTerminalText(original)
	if message == original {
		return err
	}
	return sanitizedTerminalError{message: message, cause: err}
}

// SanitizeTerminalError removes terminal controls while preserving the cause.
func SanitizeTerminalError(err error) error {
	return sanitizeTerminalError(err)
}
