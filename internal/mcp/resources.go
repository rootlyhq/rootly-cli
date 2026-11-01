package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	maxDiscovered = 500
	// ResourceByteCap bounds how much of one resource is attached to a prompt.
	ResourceByteCap = 32 * 1024
)

type PromptArg struct {
	Name     string
	Required bool
}

type PromptInfo struct {
	Name        string
	Description string
	Arguments   []PromptArg
}

type ResourceInfo struct {
	URI         string
	Name        string
	Description string
	MIMEType    string
}

// discoverExtras lists prompts and resources, only for the capabilities the server declares.
func discoverExtras(ctx context.Context, session *sdk.ClientSession) (prompts []PromptInfo, resources []ResourceInfo) {
	init := session.InitializeResult()
	if init == nil || init.Capabilities == nil {
		return nil, nil
	}
	if init.Capabilities.Prompts != nil {
		for prompt, err := range session.Prompts(ctx, nil) {
			if err != nil || len(prompts) >= maxDiscovered {
				break
			}
			info := PromptInfo{Name: prompt.Name, Description: prompt.Description}
			for _, argument := range prompt.Arguments {
				info.Arguments = append(info.Arguments, PromptArg{Name: argument.Name, Required: argument.Required})
			}
			prompts = append(prompts, info)
		}
	}
	if init.Capabilities.Resources != nil {
		for resource, err := range session.Resources(ctx, nil) {
			if err != nil || len(resources) >= maxDiscovered {
				break
			}
			resources = append(resources, ResourceInfo{URI: resource.URI, Name: resource.Name, Description: resource.Description, MIMEType: resource.MIMEType})
		}
	}
	return prompts, resources
}

func (m *Manager) sessionFor(serverName string) (*sdk.ClientSession, time.Duration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, runtime := range m.servers {
		if runtime.server.Name == serverName && runtime.session != nil && !runtime.sessionOff {
			return runtime.session, runtime.server.Config.CallTimeout(connectTimeout), nil
		}
	}
	return nil, 0, fmt.Errorf("MCP server %s is not connected", serverName)
}

// GetPrompt renders an MCP prompt's text messages into one message.
func (m *Manager) GetPrompt(ctx context.Context, serverName, name string, arguments map[string]string) (string, error) {
	session, timeout, err := m.sessionFor(serverName)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := session.GetPrompt(ctx, &sdk.GetPromptParams{Name: name, Arguments: arguments})
	if err != nil {
		return "", err
	}
	var parts []string
	for _, message := range result.Messages {
		switch content := message.Content.(type) {
		case *sdk.TextContent:
			parts = append(parts, content.Text)
		case *sdk.EmbeddedResource:
			if content.Resource != nil && content.Resource.Text != "" {
				parts = append(parts, content.Resource.Text)
			}
		}
	}
	text := strings.TrimSpace(strings.Join(parts, "\n\n"))
	if text == "" {
		return "", errors.New("prompt has no text content")
	}
	return text, nil
}

// ReadResource returns a resource's text, cut to limit bytes on a rune boundary.
func (m *Manager) ReadResource(ctx context.Context, serverName, uri string, limit int) (text string, truncated bool, err error) {
	session, timeout, err := m.sessionFor(serverName)
	if err != nil {
		return "", false, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := session.ReadResource(ctx, &sdk.ReadResourceParams{URI: uri})
	if err != nil {
		return "", false, err
	}
	var joined strings.Builder
	for _, content := range result.Contents {
		if content == nil || content.Text == "" {
			continue
		}
		if joined.Len() > 0 {
			joined.WriteByte('\n')
		}
		if limit > 0 && joined.Len()+len(content.Text) > limit {
			joined.WriteString(content.Text[:min(len(content.Text), limit+1-joined.Len())])
			break
		}
		joined.WriteString(content.Text)
	}
	if joined.Len() == 0 {
		return "", false, errors.New("no text content")
	}
	return capBytes(joined.String(), limit)
}

func capBytes(text string, limit int) (capped string, truncated bool, err error) {
	if limit <= 0 || len(text) <= limit {
		return text, false, nil
	}
	for limit > 0 && !utf8.RuneStart(text[limit]) {
		limit--
	}
	return text[:limit], true, nil
}
