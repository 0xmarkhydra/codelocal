package mediatransport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type artifactPrepareRequest struct {
	SHA256      string `json:"sha256"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
	Name        string `json:"name,omitempty"`
}

type artifactPrepareResponse struct {
	ArtifactID   string      `json:"artifactId"`
	PublicURL    string      `json:"publicUrl"`
	ContentType  string      `json:"contentType"`
	Size         int64       `json:"size"`
	SHA256       string      `json:"sha256"`
	Deduplicated bool        `json:"deduplicated"`
	Upload       uploadGrant `json:"upload"`
}

type ArtifactRef struct {
	ArtifactID  string `json:"artifactId"`
	PublicURL   string `json:"publicUrl"`
	ContentType string `json:"mimeType"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Transport   string `json:"transport"`
}

func artifactMarker(result any) (map[string]any, map[string]any, bool) {
	root, ok := result.(map[string]any)
	if !ok {
		return nil, nil, false
	}
	marker, ok := root["__mcpArtifact"].(map[string]any)
	return root, marker, ok
}

func (p *Publisher) artifactEnabled() bool {
	return p != nil && validHTTPURL(p.config.ArtifactPrepareURL) && p.config.Authorize != nil
}

func hashArtifactFile(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func (p *Publisher) prepareArtifact(ctx context.Context, input artifactPrepareRequest) (artifactPrepareResponse, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return artifactPrepareResponse{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.ArtifactPrepareURL, strings.NewReader(string(raw)))
	if err != nil {
		return artifactPrepareResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	if err := p.config.Authorize(request, raw); err != nil {
		return artifactPrepareResponse{}, fmt.Errorf("authorize artifact presign: %w", err)
	}
	response, err := p.client.Do(request)
	if err != nil {
		return artifactPrepareResponse{}, fmt.Errorf("request artifact presign: %w", err)
	}
	payload, err := readLimitedResponse(response)
	if err != nil {
		return artifactPrepareResponse{}, err
	}
	if response.StatusCode == http.StatusServiceUnavailable {
		return artifactPrepareResponse{}, ErrNotConfigured
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return artifactPrepareResponse{}, fmt.Errorf("CodeLocal artifact presign returned HTTP %d", response.StatusCode)
	}
	var prepared artifactPrepareResponse
	if err := json.Unmarshal(payload, &prepared); err != nil {
		return artifactPrepareResponse{}, fmt.Errorf("decode artifact presign response: %w", err)
	}
	return prepared, nil
}

func (p *Publisher) directUploadArtifact(ctx context.Context, path string, grant uploadGrant) error {
	if !grant.Required {
		return nil
	}
	if !validHTTPURL(grant.URL) {
		return errors.New("artifact upload grant has invalid URL")
	}
	method := strings.ToUpper(strings.TrimSpace(grant.Method))
	if method == "" {
		method = http.MethodPut
	}
	if method != http.MethodPut {
		return errors.New("artifact upload grant must use PUT")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	request, err := http.NewRequestWithContext(ctx, method, grant.URL, file)
	if err != nil {
		return err
	}
	for key, values := range grant.Headers {
		for _, value := range values {
			request.Header.Add(key, value)
		}
	}
	response, err := p.client.Do(request)
	if err != nil {
		return fmt.Errorf("upload artifact directly to object storage: %w", err)
	}
	_, _ = readLimitedResponse(response)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("object storage artifact upload returned HTTP %d", response.StatusCode)
	}
	return nil
}

func (p *Publisher) transformArtifact(ctx context.Context, result any) (any, bool, error) {
	root, marker, ok := artifactMarker(result)
	if !ok {
		return result, false, nil
	}
	output := cloneMap(root)
	delete(output, "__mcpArtifact")
	if !p.artifactEnabled() {
		output["artifactTransportError"] = "durable artifact storage is not configured"
		return output, true, ErrNotConfigured
	}
	path, _ := marker["path"].(string)
	name, _ := marker["name"].(string)
	mimeType, _ := marker["mimeType"].(string)
	kind, _ := marker["kind"].(string)
	path = strings.TrimSpace(path)
	name = strings.TrimSpace(name)
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if path == "" || mimeType != "video/mp4" || strings.ToLower(filepath.Ext(path)) != ".mp4" {
		return output, true, errors.New("artifact marker is not a valid MP4")
	}
	if name == "" {
		name = filepath.Base(path)
	}
	hash, size, err := hashArtifactFile(path)
	if err != nil {
		return output, true, fmt.Errorf("read rendered artifact: %w", err)
	}
	prepared, err := p.prepareArtifact(ctx, artifactPrepareRequest{SHA256: hash, ContentType: mimeType, Size: size, Name: name})
	if err != nil {
		return output, true, err
	}
	if prepared.SHA256 != "" && !strings.EqualFold(prepared.SHA256, hash) {
		return output, true, errors.New("CodeLocal artifact returned mismatched hash")
	}
	if !validHTTPURL(prepared.PublicURL) {
		return output, true, errors.New("CodeLocal artifact returned invalid public URL")
	}
	if err := p.directUploadArtifact(ctx, path, prepared.Upload); err != nil {
		return output, true, err
	}
	ref := ArtifactRef{
		ArtifactID: prepared.ArtifactID, PublicURL: prepared.PublicURL, ContentType: first(prepared.ContentType, mimeType),
		Size: size, SHA256: hash, Name: name, Kind: first(kind, "video"), Transport: "s3-public-artifact",
	}
	output["status"] = "published"
	output["artifact"] = ref
	output["publicUrl"] = ref.PublicURL
	return output, true, nil
}
