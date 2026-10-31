package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"strings"

	rootly "github.com/rootlyhq/rootly-go"

	"github.com/rootlyhq/rootly-cli/internal/mcp"
)

type clientToolsKey struct{}

// WithClientTools makes the next StreamAiChatEvents call send these local
// MCP tools as the `client_tools` JSON body.
func WithClientTools(ctx context.Context, tools []mcp.ClientTool) context.Context {
	if len(tools) == 0 {
		return ctx
	}
	return context.WithValue(ctx, clientToolsKey{}, tools)
}

func clientToolsEditor(ctx context.Context) (rootly.RequestEditorFn, bool) {
	tools, ok := ctx.Value(clientToolsKey{}).([]mcp.ClientTool)
	if !ok || len(tools) == 0 {
		return nil, false
	}
	return func(_ context.Context, req *http.Request) error {
		body, err := json.Marshal(map[string]any{"client_tools": tools})
		if err != nil {
			return err
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
		req.ContentLength = int64(len(body))
		req.Header.Set("Content-Type", "application/vnd.api+json")
		return nil
	}, true
}

func (c *Client) streamAiChatEvents(ctx context.Context, params *rootly.StreamAiChatParams) (iter.Seq2[rootly.AiChatStreamEvent, error], error) {
	if editor, ok := clientToolsEditor(ctx); ok {
		return c.raw.StreamAiChatEvents(ctx, params, editor)
	}
	return c.raw.StreamAiChatEvents(ctx, params)
}

type ToolResult struct {
	SessionID string `json:"session_id"`
	CallID    string `json:"call_id"`
	Output    string `json:"output"`
	IsError   bool   `json:"is_error"`
}

// PostAiChatToolResult delivers a local tool's result to the blocked turn.
func (c *Client) PostAiChatToolResult(ctx context.Context, result ToolResult) error {
	body, err := json.Marshal(result)
	if err != nil {
		return err
	}
	response, err := c.doJSON(ctx, http.MethodPost, "/v1/ai/chat/tool_results", body)
	if err != nil {
		return fmt.Errorf("send local tool result: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil
	}
	return fmt.Errorf("send local tool result: %s", apiErrorDetail(response))
}

type AIConnector struct {
	ID               string
	Name             string
	ProviderType     string
	Status           string
	AvailableToAgent bool
	ToolCount        *int
	ManageURL        string
}

type AIConnectorPage struct {
	Connectors []AIConnector
	NextPage   int
	StatusCode int
}

// ListAIConnectors returns one page of the team's cloud MCP connectors.
func (c *Client) ListAIConnectors(ctx context.Context, page, pageSize int) (*AIConnectorPage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 25
	}
	response, err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/ai/connectors?page[number]=%d&page[size]=%d", page, pageSize), nil)
	if err != nil {
		return nil, fmt.Errorf("list connectors: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return &AIConnectorPage{StatusCode: response.StatusCode}, fmt.Errorf("list connectors: %s", apiErrorDetail(response))
	}
	var document struct {
		Data []struct {
			ID         string `json:"id"`
			Attributes struct {
				Name             string `json:"name"`
				ProviderType     string `json:"provider_type"`
				Status           string `json:"status"`
				AvailableToAgent bool   `json:"available_to_agent"`
				ToolCount        *int   `json:"tool_count"`
				ManageURL        string `json:"manage_url"`
			} `json:"attributes"`
		} `json:"data"`
		Meta struct {
			NextPage *int `json:"next_page"`
		} `json:"meta"`
		Links struct {
			Next *string `json:"next"`
		} `json:"links"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&document); err != nil {
		return nil, fmt.Errorf("list connectors: decode response: %w", err)
	}
	result := &AIConnectorPage{StatusCode: response.StatusCode}
	for _, item := range document.Data {
		attributes := item.Attributes
		result.Connectors = append(result.Connectors, AIConnector{
			ID: item.ID, Name: attributes.Name, ProviderType: attributes.ProviderType, Status: attributes.Status,
			AvailableToAgent: attributes.AvailableToAgent, ToolCount: attributes.ToolCount, ManageURL: attributes.ManageURL,
		})
	}
	switch {
	case document.Meta.NextPage != nil && *document.Meta.NextPage > page:
		result.NextPage = *document.Meta.NextPage
	case document.Links.Next != nil && *document.Links.Next != "":
		result.NextPage = page + 1
	case document.Meta.NextPage == nil && document.Links.Next == nil && len(document.Data) == pageSize:
		result.NextPage = page + 1
	}
	return result, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	var reader io.Reader = http.NoBody
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.endpoint, "/")+path, reader)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	request.Header.Set("Accept", "application/vnd.api+json, application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/vnd.api+json")
	}
	return c.httpClient.Do(request)
}

func apiErrorDetail(response *http.Response) string {
	data, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	var document struct {
		Errors []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		} `json:"errors"`
		Error string `json:"error"`
	}
	if json.Unmarshal(data, &document) == nil {
		if len(document.Errors) > 0 {
			if document.Errors[0].Detail != "" {
				return fmt.Sprintf("HTTP %d: %s", response.StatusCode, document.Errors[0].Detail)
			}
			if document.Errors[0].Title != "" {
				return fmt.Sprintf("HTTP %d: %s", response.StatusCode, document.Errors[0].Title)
			}
		}
		if document.Error != "" {
			return fmt.Sprintf("HTTP %d: %s", response.StatusCode, document.Error)
		}
	}
	return fmt.Sprintf("HTTP %d", response.StatusCode)
}
