package agent

import (
	"bytes"
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	goldmarktext "github.com/yuin/goldmark/text"
)

type markdownLinkReplacement struct {
	start int
	end   int
	text  string
}

func terminalMarkdown(markdown string, noColor bool) string {
	if noColor {
		return markdown
	}
	return preprocessMarkdownLinks(markdown)
}

func preprocessMarkdownLinks(markdown string) string {
	source := []byte(markdown)
	document := goldmark.New().Parser().Parse(goldmarktext.NewReader(source))
	replacements := make([]markdownLinkReplacement, 0)

	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		link, ok := node.(*ast.Link)
		if !ok || len(link.Destination) == 0 {
			return ast.WalkContinue, nil
		}
		label := markdownLinkLabel(link, source)
		destination := string(link.Destination)
		if label == "" || label == destination {
			return ast.WalkContinue, nil
		}
		labelStart, labelEnd, ok := markdownLinkTextRange(link, source)
		if !ok {
			return ast.WalkContinue, nil
		}
		start, closingBracket, end, ok := findInlineLinkRange(source, labelStart, labelEnd, destination)
		if !ok {
			return ast.WalkContinue, nil
		}
		replacement := ansi.SetHyperlink(destination) + string(source[start+1:closingBracket]) + ansi.ResetHyperlink()
		replacements = append(replacements, markdownLinkReplacement{start: start, end: end, text: replacement})
		return ast.WalkContinue, nil
	})

	if len(replacements) == 0 {
		return markdown
	}
	sort.Slice(replacements, func(i, j int) bool {
		return replacements[i].start > replacements[j].start
	})
	for _, replacement := range replacements {
		source = append(source[:replacement.start], append([]byte(replacement.text), source[replacement.end:]...)...)
	}
	return string(source)
}

func markdownLinkLabel(link ast.Node, source []byte) string {
	var label strings.Builder
	_ = ast.Walk(link, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if text, ok := node.(*ast.Text); ok {
			label.Write(text.Value(source))
		}
		return ast.WalkContinue, nil
	})
	return label.String()
}

func markdownLinkTextRange(link ast.Node, source []byte) (start, end int, found bool) {
	start, end = len(source), 0
	_ = ast.Walk(link, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		text, ok := node.(*ast.Text)
		if !ok || text.Segment.Len() == 0 {
			return ast.WalkContinue, nil
		}
		start = min(start, text.Segment.Start)
		end = max(end, text.Segment.Stop)
		found = true
		return ast.WalkContinue, nil
	})
	return start, end, found
}

func findInlineLinkRange(source []byte, labelStart, labelEnd int, destination string) (start, closingBracket, end int, found bool) {
	for start := labelStart - 1; start >= 0; start-- {
		if source[start] != '[' || escapedMarkdownCharacter(source, start) {
			continue
		}
		closingBracket := matchingMarkdownBracket(source, start)
		if closingBracket < labelEnd || closingBracket+1 >= len(source) || source[closingBracket+1] != '(' {
			continue
		}
		closingParen, ok := matchingMarkdownParen(source, closingBracket+1)
		if !ok {
			continue
		}
		rawDestination := markdownLinkDestination(source[closingBracket+2 : closingParen])
		if rawDestination == destination {
			return start, closingBracket, closingParen + 1, true
		}
	}
	return 0, 0, 0, false
}

func matchingMarkdownBracket(source []byte, start int) int {
	depth := 1
	for index := start + 1; index < len(source); index++ {
		if escapedMarkdownCharacter(source, index) {
			continue
		}
		switch source[index] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func matchingMarkdownParen(source []byte, start int) (int, bool) {
	depth := 1
	var quote byte
	angle := false
	for index := start + 1; index < len(source); index++ {
		character := source[index]
		if escapedMarkdownCharacter(source, index) {
			continue
		}
		if quote != 0 {
			if character == quote {
				quote = 0
			}
			continue
		}
		if character == '"' || character == '\'' {
			quote = character
			continue
		}
		if character == '<' {
			angle = true
			continue
		}
		if character == '>' {
			angle = false
			continue
		}
		if angle {
			continue
		}
		switch character {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return index, true
			}
		}
	}
	return 0, false
}

func markdownLinkDestination(raw []byte) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) > 0 && raw[0] == '<' {
		if end := bytes.IndexByte(raw, '>'); end >= 0 {
			raw = raw[1:end]
		}
	} else {
		depth := 0
		for index := 0; index < len(raw); index++ {
			if escapedMarkdownCharacter(raw, index) {
				continue
			}
			switch raw[index] {
			case '(':
				depth++
			case ')':
				if depth == 0 {
					raw = raw[:index]
					index = len(raw)
				} else {
					depth--
				}
			case ' ', '\t', '\r', '\n':
				if depth == 0 {
					raw = raw[:index]
					index = len(raw)
				}
			}
		}
	}
	var destination strings.Builder
	for index := 0; index < len(raw); index++ {
		if raw[index] == '\\' && index+1 < len(raw) {
			index++
		}
		destination.WriteByte(raw[index])
	}
	return destination.String()
}

func escapedMarkdownCharacter(source []byte, index int) bool {
	backslashes := 0
	for index--; index >= 0 && source[index] == '\\'; index-- {
		backslashes++
	}
	return backslashes%2 == 1
}
