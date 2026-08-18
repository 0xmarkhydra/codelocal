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

func testAuthorize(request *http.Request, _ []byte) error {
	request.Header.Set("X-Test-Device-Auth", "signed")
	return nil
}

func TestTransformUsesPresignedDirectUploadAndRemovesBase64(t *testing.T) {
	image := []byte("fake-png-image-data")
	hash := imageHash(image)
	var prepareCalls atomic.Int32
	var uploadCalls atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/prepare":
			prepareCalls.Add(1)
			if r.Header.Get("X-Test-Device-Auth") != "signed" {
				t.Fatal("device authorization missing from presign request")
			}
			var input prepareRequest
			if json.NewDecoder(r.Body).Decode(&input) != nil || input.SHA256 != hash || input.Size != int64(len(image)) || input.ContentType != "image/png" {
				t.Fatalf("unexpected presign input: %#v", input)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"imageRef": "sha256:" + hash, "url": server.URL + "/object?read=1", "contentType": "image/png",
				"size": len(image), "sha256": hash, "expiresAt": time.Now().Add(3 * time.Minute).UnixMilli(),
				"upload": map[string]any{"required": true, "url": server.URL + "/object", "method": "PUT", "headers": map[string][]string{"Content-Type": {"image/png"}, "X-Test-Signed": {"yes"}}},
			})
		case "/object":
			uploadCalls.Add(1)
			if r.Method != http.MethodPut || r.Header.Get("Content-Type") != "image/png" || r.Header.Get("X-Test-Signed") != "yes" {
				t.Fatalf("unexpected direct upload request: method=%s headers=%v", r.Method, r.Header)
			}
			body, _ := io.ReadAll(r.Body)
			if string(body) != string(image) {
				t.Fatalf("unexpected direct upload body: %q", body)
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	publisher := New(Config{PrepareURL: server.URL + "/prepare", Authorize: testAuthorize}, server.Client())
	result, err := publisher.Transform(context.Background(), testImageResult(image))
	if err != nil {
		t.Fatal(err)
	}
	root := result.(map[string]any)
	if _, exists := root["__mcpImage"]; exists {
		t.Fatal("base64 marker must be removed before cloud transport")
	}
	ref := root["__mcpImageRef"].(map[string]any)
	if ref["transport"] != "signed-url-direct" || ref["sha256"] != hash {
		t.Fatalf("unexpected media ref: %#v", ref)
	}
	if _, err := publisher.Transform(context.Background(), testImageResult(image)); err != nil {
		t.Fatal(err)
	}
	if prepareCalls.Load() != 1 || uploadCalls.Load() != 1 {
		t.Fatalf("same image should reuse cached signed URL: prepare=%d upload=%d", prepareCalls.Load(), uploadCalls.Load())
	}
}

func TestTransformPreservesBase64WhenCloudMediaIsNotConfigured(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"media_not_configured"}`))
	}))
	defer server.Close()
	publisher := New(Config{PrepareURL: server.URL, Authorize: testAuthorize}, server.Client())
	result, err := publisher.Transform(context.Background(), testImageResult([]byte("image")))
	if err != nil {
		t.Fatal(err)
	}
	if result.(map[string]any)["__mcpImage"] == nil {
		t.Fatal("unconfigured cloud media must preserve compatibility base64")
	}
}

func TestTransformFailsClosedWhenConfiguredTransportFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no", http.StatusBadGateway)
	}))
	defer server.Close()
	publisher := New(Config{PrepareURL: server.URL, Authorize: testAuthorize}, server.Client())
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
	publisher := New(Config{PrepareURL: server.URL, Authorize: testAuthorize, Base64Fallback: true}, server.Client())
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
