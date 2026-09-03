package cloudserver

import (
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

func TestScreenshotSharePayloadUsesCanonicalPublicRoutes(t *testing.T) {
	share := cloud.ScreenshotShare{
		ID: "AbCdEf0123_-", AssetID: "media_123", ContentType: "image/png", Size: 1024,
		Width: 1440, Height: 900, CreatedAt: 1234,
	}
	payload := screenshotSharePayload(share, "https://codelocal.cloud/", true)
	if payload.URL != "https://codelocal.cloud/s/AbCdEf0123_-" {
		t.Fatalf("unexpected share url: %q", payload.URL)
	}
	if payload.ImageURL != "https://codelocal.cloud/api/v1/public/shots/AbCdEf0123_-/image/original" {
		t.Fatalf("unexpected image url: %q", payload.ImageURL)
	}
	if payload.AssetID != share.AssetID || !strings.HasSuffix(payload.DownloadURL, "/image/original?download=1") {
		t.Fatalf("unexpected owner payload: %#v", payload)
	}
	publicPayload := screenshotSharePayload(share, "https://codelocal.cloud", false)
	if publicPayload.AssetID != "" {
		t.Fatalf("public payload leaked asset id: %#v", publicPayload)
	}
}

func TestScreenshotExtensionIsConservative(t *testing.T) {
	for contentType, want := range map[string]string{
		"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp", "text/html": ".webp",
	} {
		if got := screenshotExtension(contentType); got != want {
			t.Fatalf("extension for %q=%q want %q", contentType, got, want)
		}
	}
}
