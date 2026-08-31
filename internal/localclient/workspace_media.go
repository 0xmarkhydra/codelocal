package localclient

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const workspaceMediaMaxBytes = int64(25 << 20)

func workspaceImageContentType(data []byte) string {
	switch {
	case len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n":
		return "image/png"
	case len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
		return "image/jpeg"
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return "image/webp"
	case len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a"):
		return "image/gif"
	default:
		return ""
	}
}

func (e *Engine) workspaceMedia(path string) (map[string]any, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("path is required")
	}
	file, err := e.FS.Existing(path)
	if err != nil {
		return nil, err
	}
	stat, err := os.Stat(file)
	if err != nil {
		return nil, err
	}
	if !stat.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: not a regular file", path)
	}
	if stat.Size() <= 0 || stat.Size() > workspaceMediaMaxBytes {
		return nil, fmt.Errorf("workspace media size must be between 1 and %d bytes", workspaceMediaMaxBytes)
	}
	handle, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer handle.Close()
	data, err := io.ReadAll(io.LimitReader(handle, workspaceMediaMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != stat.Size() || int64(len(data)) > workspaceMediaMaxBytes {
		return nil, errors.New("workspace media changed while reading or exceeds the upload limit")
	}
	contentType := workspaceImageContentType(data)
	if contentType == "" {
		return nil, errors.New("workspace media must be PNG, JPEG, WebP, or GIF")
	}
	digest := sha256.Sum256(data)
	hash := hex.EncodeToString(digest[:])
	return map[string]any{
		"path":        e.FS.Rel(file),
		"fileName":    filepath.Base(file),
		"contentType": contentType,
		"size":        int64(len(data)),
		"sha256":      hash,
		"__mcpImage": map[string]any{
			"mimeType": contentType,
			"data":     base64.StdEncoding.EncodeToString(data),
		},
	}, nil
}
