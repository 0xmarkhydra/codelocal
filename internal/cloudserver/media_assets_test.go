package cloudserver

import (
	"image"
	"strings"
	"testing"
)

func TestValidateDurableMediaPrepare(t *testing.T) {
	validHash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	input, err := validateDurableMediaPrepare(mediaAssetPrepareRequest{
		SHA256: validHash, ContentType: "image/png", Size: 1024,
	}, 12<<20)
	if err != nil {
		t.Fatalf("valid prepare rejected: %v", err)
	}
	if input.ContentType != "image/png" || input.SHA256 != validHash {
		t.Fatalf("prepare normalization drifted: %#v", input)
	}
	for _, tc := range []mediaAssetPrepareRequest{
		{SHA256: "bad", ContentType: "image/png", Size: 1024},
		{SHA256: validHash, ContentType: "image/gif", Size: 1024},
		{SHA256: validHash, ContentType: "image/png", Size: 0},
		{SHA256: validHash, ContentType: "image/png", Size: (12 << 20) + 1},
	} {
		if _, err := validateDurableMediaPrepare(tc, 12<<20); err == nil {
			t.Fatalf("invalid prepare accepted: %#v", tc)
		}
	}
}

func TestDurableMediaPublicCachePolicyRequiresRevalidation(t *testing.T) {
	if durableMediaPublicCacheControl != "public,no-cache,must-revalidate" {
		t.Fatalf("cache policy=%q must require revalidation", durableMediaPublicCacheControl)
	}
	if strings.Contains(durableMediaPublicCacheControl, "immutable") {
		t.Fatalf("publication-gated media must not be immutable: %q", durableMediaPublicCacheControl)
	}
}

func TestResizeImageMaxEdgePreservesAspectAndNeverUpscales(t *testing.T) {
	wide := image.NewNRGBA(image.Rect(0, 0, 2000, 1000))
	resized := resizeImageMaxEdge(wide, 320)
	if got := resized.Bounds().Size(); got.X != 320 || got.Y != 160 {
		t.Fatalf("wide resize=%v want 320x160", got)
	}

	small := image.NewNRGBA(image.Rect(0, 0, 120, 80))
	unchanged := resizeImageMaxEdge(small, 320)
	if got := unchanged.Bounds().Size(); got.X != 120 || got.Y != 80 {
		t.Fatalf("small image upscaled: %v", got)
	}
}

func TestMediaAssetErrorCodeIsStable(t *testing.T) {
	cases := map[string]string{
		"media source sha256 mismatch": "sha256_mismatch",
		"image dimensions exceed safe limits": "dimensions_invalid",
		"decode image: broken": "decode_failed",
		"encode large webp: broken": "encode_failed",
		"storage unavailable": "processing_failed",
	}
	for message, want := range cases {
		if got := mediaAssetErrorCode(assertError(message)); got != want {
			t.Fatalf("error code=%q want %q for %q", got, want, message)
		}
	}
}

type stringError string

func (e stringError) Error() string { return string(e) }
func assertError(value string) error { return stringError(value) }
