package mediatransport

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestTransformPublishesMP4ArtifactAndRemovesLocalPath(t *testing.T) {
	data := []byte("fake-mp4-content")
	path := filepath.Join(t.TempDir(), "short.mp4")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/prepare":
			var input artifactPrepareRequest
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			if input.ContentType != "video/mp4" || input.Size != int64(len(data)) || input.SHA256 == "" {
				t.Fatalf("unexpected prepare input: %#v", input)
			}
			_ = json.NewEncoder(w).Encode(artifactPrepareResponse{
				ArtifactID: "video_test", PublicURL: "https://media.example.test/video_test.mp4",
				ContentType: "video/mp4", Size: input.Size, SHA256: input.SHA256,
				Upload: uploadGrant{Required: true, URL: server.URL + "/upload", Method: http.MethodPut},
			})
		case "/upload":
			uploaded, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			if string(uploaded) != string(data) {
				t.Fatalf("uploaded data mismatch: %q", uploaded)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	publisher := New(Config{
		ArtifactPrepareURL: server.URL + "/prepare",
		Authorize:          func(*http.Request, []byte) error { return nil },
	}, server.Client())
	result, err := publisher.Transform(context.Background(), map[string]any{
		"status":        "prepared",
		"__mcpArtifact": map[string]any{"path": path, "name": "short.mp4", "mimeType": "video/mp4", "kind": "video"},
	})
	if err != nil {
		t.Fatal(err)
	}
	root := result.(map[string]any)
	if _, leaked := root["__mcpArtifact"]; leaked {
		t.Fatalf("local artifact marker leaked: %#v", root)
	}
	if got := root["publicUrl"]; got != "https://media.example.test/video_test.mp4" {
		t.Fatalf("publicUrl=%v", got)
	}
	artifact, ok := root["artifact"].(ArtifactRef)
	if !ok || artifact.ArtifactID != "video_test" || artifact.Transport != "s3-public-artifact" {
		t.Fatalf("unexpected artifact ref: %#v", root["artifact"])
	}
}
