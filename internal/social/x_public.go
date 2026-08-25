package social

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultXPublicBaseURL = "https://api.fxtwitter.com"
	maxSocialResponseSize = 2 << 20
)

type xPublicProvider struct {
	client  *http.Client
	baseURL string
}

func NewXPublicProvider(client *http.Client, baseURL string) Provider {
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultXPublicBaseURL
	}
	return &xPublicProvider{client: client, baseURL: strings.TrimRight(baseURL, "/")}
}

func (p *xPublicProvider) Name() string { return "fxtwitter-public" }

func (p *xPublicProvider) Supports(rawURL string) bool {
	_, _, ok := parseXStatusURL(rawURL)
	return ok
}

func (p *xPublicProvider) ReadPublicPost(ctx context.Context, rawURL string) (Post, error) {
	username, statusID, ok := parseXStatusURL(rawURL)
	if !ok {
		return Post{}, ErrUnsupportedPlatform
	}
	endpoint := p.baseURL + "/" + url.PathEscape(username) + "/status/" + url.PathEscape(statusID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Post{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "CodeLocal-Social/1.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return Post{}, fmt.Errorf("public X reader request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Post{}, fmt.Errorf("public X reader returned HTTP %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxSocialResponseSize+1))
	if err != nil {
		return Post{}, err
	}
	if len(raw) > maxSocialResponseSize {
		return Post{}, errors.New("public X response exceeded size limit")
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return Post{}, fmt.Errorf("decode public X response: %w", err)
	}
	if code := int64Value(root["code"]); code != 0 && code != http.StatusOK {
		return Post{}, fmt.Errorf("public X reader returned code %d: %s", code, stringValue(root["message"]))
	}
	tweet := mapValue(root["tweet"])
	if tweet == nil {
		return Post{}, errors.New("public X response did not contain a tweet")
	}
	post := normalizeXPost(tweet, rawURL, 0)
	if strings.TrimSpace(post.ID) == "" {
		post.ID = statusID
	}
	if strings.TrimSpace(post.URL) == "" {
		post.URL = canonicalXURL(username, statusID)
	}
	return post, nil
}

func parseXStatusURL(raw string) (string, string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" {
		return "", "", false
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	switch host {
	case "x.com", "www.x.com", "twitter.com", "www.twitter.com", "mobile.twitter.com":
	default:
		return "", "", false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 3 || strings.ToLower(parts[1]) != "status" {
		return "", "", false
	}
	username, statusID := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[2])
	if username == "" || statusID == "" {
		return "", "", false
	}
	if _, err := strconv.ParseUint(statusID, 10, 64); err != nil {
		return "", "", false
	}
	return username, statusID, true
}

func canonicalXURL(username, statusID string) string {
	return "https://x.com/" + username + "/status/" + statusID
}

func normalizeXPost(tweet map[string]any, fallbackURL string, depth int) Post {
	author := mapValue(tweet["author"])
	post := Post{
		Platform:  "x",
		Type:      "post",
		ID:        firstString(tweet, "id", "tweet_id"),
		URL:       firstString(tweet, "url", "tweet_url"),
		Text:      firstString(tweet, "text", "full_text"),
		CreatedAt: firstString(tweet, "created_at", "createdAt"),
		Author: Author{
			Name:      firstString(author, "name"),
			Username:  firstString(author, "screen_name", "username"),
			AvatarURL: firstString(author, "avatar_url", "avatarUrl", "profile_image_url_https"),
		},
		Metrics: Metrics{
			Likes:   firstInt64(tweet, "likes", "favorite_count"),
			Replies: firstInt64(tweet, "replies", "reply_count"),
			Reposts: firstInt64(tweet, "retweets", "reposts", "retweet_count"),
			Views:   firstInt64(tweet, "views", "view_count"),
		},
	}
	if post.URL == "" {
		post.URL = strings.TrimSpace(fallbackURL)
	}
	post.Media = normalizeXMedia(mapValue(tweet["media"]))
	if depth == 0 {
		quote := mapValue(tweet["quote"])
		if quote == nil {
			quote = mapValue(tweet["quote_tweet"])
		}
		if quote != nil {
			normalized := normalizeXPost(quote, firstString(quote, "url", "tweet_url"), depth+1)
			post.Quote = &normalized
		}
	}
	return post
}

func normalizeXMedia(media map[string]any) []Media {
	if media == nil {
		return nil
	}
	out := []Media{}
	out = appendMedia(out, media["photos"], "photo")
	out = appendMedia(out, media["videos"], "video")
	return appendMedia(out, media["gifs"], "gif")
}

func appendMedia(out []Media, raw any, kind string) []Media {
	items, _ := raw.([]any)
	for _, item := range items {
		entry := mapValue(item)
		if entry == nil {
			continue
		}
		out = append(out, Media{
			Type:         kind,
			URL:          firstString(entry, "url", "media_url_https", "media_url"),
			ThumbnailURL: firstString(entry, "thumbnail_url", "thumbnailUrl"),
		})
	}
	return out
}

func mapValue(value any) map[string]any {
	mapped, _ := value.(map[string]any)
	return mapped
}

func stringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringValue(values[key]); value != "" {
			return value
		}
	}
	return ""
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case int:
		return int64(typed)
	case int64:
		return typed
	case json.Number:
		parsed, _ := typed.Int64()
		return parsed
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed
	default:
		return 0
	}
}

func firstInt64(values map[string]any, keys ...string) int64 {
	for _, key := range keys {
		if value := int64Value(values[key]); value != 0 {
			return value
		}
	}
	return 0
}
