package social

import "context"

// Request is the stable server-side social orchestration input. Platform-specific
// details stay behind Provider implementations instead of leaking into MCP.
type Request struct {
	Action string `json:"action"`
	URL    string `json:"url,omitempty"`
}

type Author struct {
	Name      string `json:"name,omitempty"`
	Username  string `json:"username,omitempty"`
	AvatarURL string `json:"avatarUrl,omitempty"`
}

type Media struct {
	Type         string `json:"type"`
	URL          string `json:"url,omitempty"`
	ThumbnailURL string `json:"thumbnailUrl,omitempty"`
}

type Metrics struct {
	Likes   int64 `json:"likes,omitempty"`
	Replies int64 `json:"replies,omitempty"`
	Reposts int64 `json:"reposts,omitempty"`
	Views   int64 `json:"views,omitempty"`
}

type Post struct {
	Platform  string  `json:"platform"`
	Type      string  `json:"type"`
	ID        string  `json:"id,omitempty"`
	URL       string  `json:"url"`
	Text      string  `json:"text,omitempty"`
	CreatedAt string  `json:"createdAt,omitempty"`
	Author    Author  `json:"author"`
	Media     []Media `json:"media,omitempty"`
	Metrics   Metrics `json:"metrics,omitempty"`
	Quote     *Post   `json:"quote,omitempty"`
}

type Result struct {
	Action   string `json:"action"`
	Platform string `json:"platform"`
	Provider string `json:"provider"`
	Post     Post   `json:"post"`
}

// Provider stays deliberately small so additional social backends can be added
// without changing the public MCP tool surface.
type Provider interface {
	Name() string
	Supports(rawURL string) bool
	ReadPublicPost(ctx context.Context, rawURL string) (Post, error)
}
