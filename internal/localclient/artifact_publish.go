package localclient

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const maxPublishableVideoBytes = int64(2 << 30)

func (e *Engine) publishArtifactMarker(path string) (map[string]any, error) {
	if e == nil || e.FS == nil {
		return nil, errors.New("workspace filesystem is unavailable")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("artifact path is required")
	}
	absolute, err := e.FS.Existing(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("artifact must be a regular file")
	}
	if info.Size() <= 0 || info.Size() > maxPublishableVideoBytes {
		return nil, errors.New("video artifact size is outside the supported range")
	}
	if strings.ToLower(filepath.Ext(absolute)) != ".mp4" {
		return nil, errors.New("only MP4 video artifacts can be published")
	}
	return map[string]any{
		"status":        "prepared",
		"artifact":      map[string]any{"name": filepath.Base(absolute), "mimeType": "video/mp4", "size": info.Size(), "kind": "video"},
		"__mcpArtifact": map[string]any{"path": absolute, "name": filepath.Base(absolute), "mimeType": "video/mp4", "size": info.Size(), "kind": "video"},
	}, nil
}
