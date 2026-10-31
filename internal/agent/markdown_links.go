package agent

import (
	"bytes"
	"net/url"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	goldmarktext "github.com/yuin/goldmark/text"
)

type markdownLink struct {
	destination string
	host        string
	hyperlink   bool
}

type markdownLinkReplacement struct {
	start int
	end   int
	text  string
	link  markdownLink
}

func terminalMarkdown(markdown string, noColor bool) string {
	markdown = sanitizeTerminalText(markdown)
	if noColor {
		return markdown
	}
	return preprocessMarkdownLinks(markdown)
}

func preprocessMarkdownLinks(markdown string) string {
	processed, _ := preprocessMarkdownLinksWithURLs(markdown, "")
	return processed
}

func preprocessMarkdownLinksWithURLs(markdown, apiHost string) (processed string, links []markdownLink) {
	markdown = sanitizeTerminalText(markdown)
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
		replacement := "\uE000" + string(source[start+1:closingBracket]) + "\uE001"
		safeLink := newMarkdownLink(destination, apiHost)
		replacements = append(replacements, markdownLinkReplacement{
			start: start,
			end:   end,
			text:  replacement,
			link:  safeLink,
		})
		return ast.WalkContinue, nil
	})

	if len(replacements) == 0 {
		return markdown, nil
	}
	sort.Slice(replacements, func(i, j int) bool {
		return replacements[i].start < replacements[j].start
	})
	links = make([]markdownLink, len(replacements))
	for index, replacement := range replacements {
		links[index] = replacement.link
	}
	for index := len(replacements) - 1; index >= 0; index-- {
		replacement := replacements[index]
		source = append(source[:replacement.start], append([]byte(replacement.text), source[replacement.end:]...)...)
	}
	return string(source), links
}

func rewriteDashboardLinks(markdown, webBase, sessionID string) string {
	if !strings.Contains(strings.ToLower(markdown), "#ai-add-to-dashboard") {
		return markdown
	}
	destination := strings.TrimRight(webBase, "/") + "/account"
	if sessionID != "" {
		destination += "?chat=" + url.QueryEscape(sessionID)
	}

	source := []byte(markdown)
	document := goldmark.New().Parser().Parse(goldmarktext.NewReader(source))
	replacements := make([]markdownLinkReplacement, 0)
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		link, ok := node.(*ast.Link)
		if !ok {
			return ast.WalkContinue, nil
		}
		href := strings.ToLower(string(link.Destination))
		if !strings.HasPrefix(href, "#ai-add-to-dashboard:") &&
			!strings.HasPrefix(href, "#ai-add-to-dashboard%3a") {
			return ast.WalkContinue, nil
		}
		labelStart, labelEnd, ok := markdownLinkTextRange(link, source)
		if !ok {
			return ast.WalkContinue, nil
		}
		start, _, end, ok := findInlineLinkRange(source, labelStart, labelEnd, string(link.Destination))
		if !ok {
			return ast.WalkContinue, nil
		}
		replacements = append(replacements, markdownLinkReplacement{
			start: start,
			end:   end,
			text:  "[Add to dashboard: open in web](" + destination + ")",
		})
		return ast.WalkContinue, nil
	})
	for index := len(replacements) - 1; index >= 0; index-- {
		replacement := replacements[index]
		source = append(source[:replacement.start], append([]byte(replacement.text), source[replacement.end:]...)...)
	}
	return string(source)
}

func newMarkdownLink(destination, apiHost string) markdownLink {
	link := markdownLink{destination: sanitizeTerminalText(destination)}
	parsed, err := url.Parse(link.destination)
	if err != nil {
		return link
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return link
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "" {
		return link
	}
	link.hyperlink = true
	domain := rootlyAppDomain(apiHost)
	if domain != "" && (host == domain || strings.HasSuffix(host, "."+domain)) {
		return link
	}
	link.host = host
	return link
}

func rootlyAppDomain(apiHost string) string {
	apiHost = strings.TrimSpace(sanitizeTerminalText(apiHost))
	if apiHost == "" {
		return ""
	}
	if !strings.Contains(apiHost, "://") {
		apiHost = "//" + apiHost
	}
	parsed, err := url.Parse(apiHost)
	if err != nil {
		return ""
	}
	domain := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	return strings.TrimPrefix(domain, "api.")
}

func restoreMarkdownLinks(rendered string, links []markdownLink) string {
	const (
		startMarker = "\uE000"
		endMarker   = "\uE001"
	)
	var output strings.Builder
	linkIndex := 0
	currentLink := -1
	for rendered != "" {
		startIndex := strings.Index(rendered, startMarker)
		endIndex := strings.Index(rendered, endMarker)
		if startIndex < 0 && endIndex < 0 {
			break
		}
		markerIndex := startIndex
		marker := startMarker
		if markerIndex < 0 || (endIndex >= 0 && endIndex < markerIndex) {
			markerIndex = endIndex
			marker = endMarker
		}
		output.WriteString(rendered[:markerIndex])
		if marker == startMarker {
			currentLink = linkIndex
			if currentLink < len(links) && links[currentLink].hyperlink {
				output.WriteString(ansi.SetHyperlink(links[currentLink].destination))
			}
			linkIndex++
		} else {
			if currentLink >= 0 && currentLink < len(links) {
				link := links[currentLink]
				if link.hyperlink {
					output.WriteString(ansi.ResetHyperlink())
					if link.host != "" {
						output.WriteString(lipgloss.NewStyle().Faint(true).Render(" (" + link.host + ")"))
					}
				} else {
					output.WriteString(" <" + link.destination + ">")
				}
			}
			currentLink = -1
		}
		rendered = rendered[markerIndex+len(marker):]
	}
	output.WriteString(rendered)
	return output.String()
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

func trimIncompleteTrailingMarkdownLink(markdown string) string {
	source := []byte(markdown)
	incompleteStart := -1
	for index := 0; index < len(source); index++ {
		if source[index] != '[' || escapedMarkdownCharacter(source, index) {
			continue
		}
		closingBracket := matchingMarkdownBracket(source, index)
		if closingBracket < 0 {
			incompleteStart = index
			continue
		}
		if closingBracket+1 >= len(source) || source[closingBracket+1] != '(' {
			continue
		}
		closingParen, complete := matchingMarkdownParen(source, closingBracket+1)
		if !complete {
			incompleteStart = index
			continue
		}
		index = closingParen
	}
	if incompleteStart >= 0 {
		return string(source[:incompleteStart])
	}
	return markdown
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
