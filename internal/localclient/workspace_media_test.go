package localclient

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceMediaReturnsPrivateTransportMarker(t *testing.T) {
	engine := newTestEngine(t)
	path := filepath.Join(engine.Root, "artifacts", "evidence", "proof.png")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	image := append([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, []byte("codelocal-proof")...)
	if err := os.WriteFile(path, image, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := engine.Handle(context.Background(), "read_media", map[string]any{"path": "artifacts/evidence/proof.png"}, HandleOptions{RequestID: "media-proof"})
	if err != nil {
		t.Fatal(err)
	}
	root, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("read_media returned %T", result)
	}
	if root["path"] != "artifacts/evidence/proof.png" || root["contentType"] != "image/png" || root["size"] != int64(len(image)) {
		t.Fatalf("unexpected workspace media metadata: %#v", root)
	}
	if hash, _ := root["sha256"].(string); len(hash) != 64 {
		t.Fatalf("unexpected sha256: %#v", root["sha256"])
	}
	marker, ok := root["__mcpImage"].(map[string]any)
	if !ok || marker["mimeType"] != "image/png" {
		t.Fatalf("private media marker missing: %#v", root)
	}
	decoded, err := base64.StdEncoding.DecodeString(marker["data"].(string))
	if err != nil || !bytes.Equal(decoded, image) {
		t.Fatalf("private media marker changed image bytes: err=%v", err)
	}
}

func TestWorkspaceMediaKeepsWorkspaceAndSensitivePathBoundaries(t *testing.T) {
	engine := newTestEngine(t)
	outside := filepath.Join(filepath.Dir(engine.Root), "outside.png")
	if err := os.WriteFile(outside, []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Handle(context.Background(), "read_media", map[string]any{"path": "../outside.png"}, HandleOptions{RequestID: "media-traversal"}); err == nil || !strings.Contains(err.Error(), "escapes PROJECT_ROOT") {
		t.Fatalf("workspace traversal must be blocked, err=%v", err)
	}

	secretPath := filepath.Join(engine.Root, ".env")
	if err := os.WriteFile(secretPath, []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Handle(context.Background(), "read_media", map[string]any{"path": ".env"}, HandleOptions{RequestID: "media-sensitive"}); err == nil || !strings.Contains(err.Error(), "sensitive-path policy") {
		t.Fatalf("sensitive path must be blocked, err=%v", err)
	}
}

func TestWorkspaceMediaRejectsUnsupportedFiles(t *testing.T) {
	engine := newTestEngine(t)
	if err := os.WriteFile(filepath.Join(engine.Root, "proof.txt"), []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Handle(context.Background(), "read_media", map[string]any{"path": "proof.txt"}, HandleOptions{RequestID: "media-invalid"}); err == nil || !strings.Contains(err.Error(), "must be PNG") {
		t.Fatalf("unsupported media must fail closed, err=%v", err)
	}
}
