package mediatransport

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testImageResult(data []byte) map[string]any {
	return map[string]any{
		"windowId": "ax:1:0",
		"__mcpImage": map[string]any{
			"mimeType": "image/png",
			"data":     base64.StdEncoding.EncodeToString(data),
		},
	}
}

func TestTransformUploadsPrivateImageAndRemovesBase64(t *testing.T) {
	image := []byte("fake-png-image-data")
	hash := imageHash(image)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("authorization header missing")
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		file, header, err := r.FormFile("media")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		body, _ := io.ReadAll(file)
		if string(body) != string(image) || header.Header.Get("Content-Type") != "image/png" {
			t.Fatalf("unexpected multipart image: type=%q data=%q", header.Header.Get("Content-Type"), body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"imageRef":    "sha256:" + hash,
			"url":         serverURL(r) + "/signed/image.png?sig=x",
			"contentType": "image/png",
			"size":        len(image),
			"sha256":      hash,
			"expiresAt":   time.Now().Add(3 * time.Minute).UnixMilli(),
		})
	}))
	defer server.Close()

	publisher := New(Config{UploadURL: server.URL, Token: "secret"}, server.Client())
	firstResult, err := publisher.Transform(context.Background(), testImageResult(image))
	if err != nil {
		t.Fatal(err)
	}
	root := firstResult.(map[string]any)
	if _, exists := root["__mcpImage"]; exists {
		t.Fatal("base64 marker must be removed before cloud transport")
	}
	ref := root["__mcpImageRef"].(map[string]any)
	if ref["transport"] != "signed-url" || ref["sha256"] != hash {
		t.Fatalf("unexpected media ref: %#v", ref)
	}

	if _, err := publisher.Transform(context.Background(), testImageResult(image)); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("same image should reuse cached signed URL, calls=%d", calls.Load())
	}
}

func serverURL(r *http.Request) string {
	return "http://" + r.Host
}

func TestTransformFailsClosedWhenConfiguredUploadFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no", http.StatusBadGateway)
	}))
	defer server.Close()
	publisher := New(Config{UploadURL: server.URL, Token: "secret"}, server.Client())
	result, err := publisher.Transform(context.Background(), testImageResult([]byte("image")))
	if err == nil {
		t.Fatal("configured signed-url transport must fail closed by default")
	}
	root := result.(map[string]any)
	if _, exists := root["__mcpImage"]; exists {
		t.Fatal("base64 must not leak to cloud after signed-url transport failure")
	}
	if !strings.Contains(root["visualTransportError"].(string), "HTTP 502") {
		t.Fatalf("unexpected transport error: %#v", root)
	}
}

func TestTransformCanExplicitlyFallbackToBase64(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no", http.StatusBadGateway)
	}))
	defer server.Close()
	publisher := New(Config{UploadURL: server.URL, Token: "secret", Base64Fallback: true}, server.Client())
	result, err := publisher.Transform(context.Background(), testImageResult([]byte("image")))
	if err != nil {
		t.Fatal(err)
	}
	root := result.(map[string]any)
	if root["__mcpImage"] == nil || root["visualTransportFallback"] != "base64" {
		t.Fatalf("explicit base64 fallback missing: %#v", root)
	}
}

func TestTransformDoesNothingWhenMediaTransportIsDisabled(t *testing.T) {
	publisher := New(Config{}, nil)
	input := testImageResult([]byte("image"))
	output, err := publisher.Transform(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	root, ok := output.(map[string]any)
	if !ok || root["__mcpImage"] == nil || root["windowId"] != "ax:1:0" {
		t.Fatalf("disabled transport must preserve existing behavior: %#v", output)
	}
}
