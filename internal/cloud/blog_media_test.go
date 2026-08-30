package cloud

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestBlogMediaSlotsExtractsCoverAndImageBlocks(t *testing.T) {
	content := json.RawMessage(`[
		{"type":"paragraph","text":"hello"},
		{"type":"image","assetId":"media_a1","alt":"one"},
		{"type":"image","assetId":"media_b2","caption":"two"}
	]`)
	got, err := blogMediaSlots("media_cover", content)
	if err != nil {
		t.Fatalf("blogMediaSlots error: %v", err)
	}
	want := map[string]string{
		"cover": "media_cover",
		"image:1": "media_a1",
		"image:2": "media_b2",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("slots=%#v want %#v", got, want)
	}
}

func TestBlogMediaSlotsRejectsInvalidImageAssetID(t *testing.T) {
	_, err := blogMediaSlots("", json.RawMessage(`[{"type":"image","assetId":"../../secret"}]`))
	if err == nil {
		t.Fatal("invalid image asset id accepted")
	}
}

func TestValidMediaSourceContentTypeIsStrict(t *testing.T) {
	for _, value := range []string{"image/jpeg", "image/png", "image/webp"} {
		if !validMediaSourceContentType(value) {
			t.Fatalf("supported media type rejected: %s", value)
		}
	}
	for _, value := range []string{"image/gif", "image/svg+xml", "image/avif", "text/html", "image/jpeg; charset=utf-8"} {
		if validMediaSourceContentType(value) {
			t.Fatalf("unsupported media type accepted: %s", value)
		}
	}
}
