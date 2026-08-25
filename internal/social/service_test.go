package social

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestXPublicProviderReadsPublicStatusWithoutCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/huoshan007/status/2092115568686145893" {
			t.Fatalf("unexpected provider path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Fatal("public social reader must not send account credentials, API auth, or cookies")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 200,
			"tweet": {
				"id": "2092115568686145893",
				"url": "https://x.com/huoshan007/status/2092115568686145893",
				"text": "public post",
				"created_at": "Wed Aug 26 00:00:00 +0000 2026",
				"likes": 12,
				"replies": 3,
				"retweets": 4,
				"views": 99,
				"author": {"name": "Huoshan", "screen_name": "huoshan007", "avatar_url": "https://img.example/avatar.jpg"},
				"media": {"photos": [{"url": "https://img.example/photo.jpg"}]}
			}
		}`))
	}))
	defer server.Close()

	service := NewWithProviders(NewXPublicProvider(server.Client(), server.URL))
	result, err := service.Execute(context.Background(), Request{Action: "read", URL: "https://x.com/huoshan007/status/2092115568686145893?s=46"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Platform != "x" || result.Provider != "fxtwitter-public" {
		t.Fatalf("unexpected route: %#v", result)
	}
	if result.Post.Text != "public post" || result.Post.Author.Username != "huoshan007" {
		t.Fatalf("unexpected normalized post: %#v", result.Post)
	}
	if result.Post.Metrics.Likes != 12 || result.Post.Metrics.Views != 99 || len(result.Post.Media) != 1 {
		t.Fatalf("normalized metadata missing: %#v", result.Post)
	}
}

func TestXPublicProviderRejectsNonXAndMalformedStatusURLs(t *testing.T) {
	provider := NewXPublicProvider(nil, "")
	for _, raw := range []string{
		"https://example.com/user/status/123",
		"http://x.com/user/status/123",
		"https://x.com/user/not-status/123",
		"https://x.com/user/status/not-a-number",
	} {
		if provider.Supports(raw) {
			t.Fatalf("provider unexpectedly supports %q", raw)
		}
	}
}

func TestSocialServiceRejectsUnsupportedActionAndPlatform(t *testing.T) {
	service := NewWithProviders(NewXPublicProvider(nil, ""))
	if _, err := service.Execute(context.Background(), Request{Action: "search", URL: "https://x.com/a/status/1"}); err == nil {
		t.Fatal("unsupported action must fail")
	}
	if _, err := service.Execute(context.Background(), Request{Action: "read", URL: "https://example.com/post/1"}); err != ErrUnsupportedPlatform {
		t.Fatalf("unsupported platform error = %v, want %v", err, ErrUnsupportedPlatform)
	}
}
