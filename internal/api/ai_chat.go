package api

import (
	"context"
	"net/url"

	openapi_types "github.com/oapi-codegen/runtime/types"
	rootly "github.com/rootlyhq/rootly-go"
)

func (c *Client) GetCurrentUserWithResponse(ctx context.Context) (*rootly.GetCurrentUserResponse, error) {
	return c.client.GetCurrentUserWithResponse(ctx)
}

func (c *Client) ListAiChatSessionsWithResponse(ctx context.Context, params *rootly.ListAiChatSessionsParams) (*rootly.ListAiChatSessionsResponse, error) {
	return c.client.ListAiChatSessionsWithResponse(ctx, params)
}

func (c *Client) ListAISkillsWithResponse(ctx context.Context, params *rootly.ListAISkillsParams, requestEditors ...rootly.RequestEditorFn) (*rootly.ListAISkillsResponse, error) {
	return c.client.ListAISkillsWithResponse(ctx, params, requestEditors...)
}

func (c *Client) ListAiChatSessionMessagesWithResponse(ctx context.Context, sessionID openapi_types.UUID, params *rootly.ListAiChatSessionMessagesParams) (*rootly.ListAiChatSessionMessagesResponse, error) {
	return c.client.ListAiChatSessionMessagesWithResponse(ctx, sessionID, params)
}

func (c *Client) UpdateAiChatSessionWithApplicationVndAPIPlusJSONBodyWithResponse(ctx context.Context, sessionID rootly.ID, body rootly.UpdateAiChatSessionApplicationVndAPIPlusJSONRequestBody) (*rootly.UpdateAiChatSessionResponse, error) {
	return c.client.UpdateAiChatSessionWithApplicationVndAPIPlusJSONBodyWithResponse(ctx, sessionID, body)
}

func (c *Client) DeleteAiChatSessionWithResponse(ctx context.Context, sessionID rootly.ID) (*rootly.DeleteAiChatSessionResponse, error) {
	return c.client.DeleteAiChatSessionWithResponse(ctx, sessionID)
}

func (c *Client) APIHost() string {
	parsed, err := url.Parse(c.endpoint)
	if err != nil {
		return c.endpoint
	}
	if parsed.Host != "" {
		return parsed.Host
	}
	return c.endpoint
}
