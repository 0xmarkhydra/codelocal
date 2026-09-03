package localclient

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublishArtifactMarkerAcceptsWorkspaceMP4Only(t *testing.T) {
	root := t.TempDir()
	engine, err := New(root, "video-test", "Video Studio", "device::video-test", "device")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	videoPath := filepath.Join(root, "final.mp4")
	if err := os.WriteFile(videoPath, []byte("fake mp4"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := engine.publishArtifactMarker("final.mp4")
	if err != nil {
		t.Fatal(err)
	}
	marker, ok := result["__mcpArtifact"].(map[string]any)
	if !ok || marker["mimeType"] != "video/mp4" || marker["kind"] != "video" {
		t.Fatalf("unexpected artifact marker: %#v", result)
	}
	wantPath, err := filepath.EvalSymlinks(videoPath)
	if err != nil {
		t.Fatal(err)
	}
	if marker["path"] != wantPath {
		t.Fatalf("artifact path=%v want=%s", marker["path"], wantPath)
	}

	textPath := filepath.Join(root, "not-video.txt")
	if err := os.WriteFile(textPath, []byte("no"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.publishArtifactMarker("not-video.txt"); err == nil {
		t.Fatal("expected non-MP4 artifact to be rejected")
	}
}
