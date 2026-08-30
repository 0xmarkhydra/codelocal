package mcpgateway

import (
	"context"

	"github.com/0xmarkhydra/codelocal/internal/social"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var publicSocialService = social.New()

func compactSocialToolDefinitions() []compactToolDef {
	tools := []compactToolDef{
		{
			Name:        "social",
			Title:       "Read public social content",
			Description: "Server-side social content gateway. Currently action=read accepts a public X/Twitter post URL and returns normalized post text, author, media and metrics without requiring a local CodeLocal runtime, social account credentials, cookies, OAuth, or an X API key.",
			Schema: objectSchema(map[string]any{
				"action": map[string]any{"type": "string", "enum": []string{"read"}, "description": "Social operation to perform."},
				"url":    str("Public X/Twitter status URL to read."),
			}, "action", "url"),
			Annotations: compactAnnotations("Read public social content", true, false, true),
			Execute: func(ctx context.Context, _ *Service, _ string, args map[string]any, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				action, _ := args["action"].(string)
				targetURL, _ := args["url"].(string)
				result, err := publicSocialService.Execute(ctx, social.Request{Action: action, URL: targetURL})
				if err != nil {
					return errorResult(err), nil
				}
				return textResult(result, false), nil
			},
		},
	}
	return append(tools, compactBlogToolDefinitions()...)
}
