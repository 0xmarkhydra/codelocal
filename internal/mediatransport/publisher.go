package mediatransport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultRequestTimeout = 20 * time.Second
	maxResponseBytes      = 256 << 10
	cacheExpiryMargin     = 30 * time.Second
)

var ErrNotConfigured = errors.New("CodeLocal visual media is not configured on the cloud server")

type AuthorizeFunc func(*http.Request, []byte) error

type Config struct {
	PrepareURL     string
	Authorize      AuthorizeFunc
	Base64Fallback bool
}

type ImageRef struct {
	ImageRef    string `json:"imageRef"`
	URL         string `json:"url"`
	ContentType string `json:"mimeType"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	ExpiresAt   int64  `json:"expiresAt"`
	Transport   string `json:"transport"`
}

type prepareRequest struct {
	SHA256      string `json:"sha256"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
}

type uploadGrant struct {
	Required bool                `json:"required"`
	URL      string              `json:"url"`
	Method   string              `json:"method"`
	Headers  map[string][]string `json:"headers"`
}

type prepareResponse struct {
	ImageRef    string      `json:"imageRef"`
	URL         string      `json:"url"`
	ContentType string      `json:"contentType"`
	Size        int64       `json:"size"`
	SHA256      string      `json:"sha256"`
	ExpiresAt   int64       `json:"expiresAt"`
	Upload      uploadGrant `json:"upload"`
}

type cachedRef struct{ Ref ImageRef }

type Publisher struct {
	config Config
	client *http.Client
	mu     sync.Mutex
	cache  map[string]cachedRef
}

func Base64FallbackFromEnvironment() bool {
	return envBool("CODELOCAL_MEDIA_BASE64_FALLBACK", false)
}

func envBool(name string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return value
}

func New(config Config, client *http.Client) *Publisher {
	if client == nil {
		client = &http.Client{
			Timeout:       defaultRequestTimeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	return &Publisher{config: config, client: client, cache: map[string]cachedRef{}}
}

func (p *Publisher) Enabled() bool {
	return p != nil && validHTTPURL(p.config.PrepareURL) && p.config.Authorize != nil
}

func cloneMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input)+1)
	for key, value := range input {
		output[key] = value
	}
	return output
}

func imageMarker(result any) (map[string]any, map[string]any, bool) {
	root, ok := result.(map[string]any)
	if !ok {
		return nil, nil, false
	}
	marker, ok := root["__mcpImage"].(map[string]any)
	return root, marker, ok
}

func decodedImage(marker map[string]any) ([]byte, string, error) {
	encoded, _ := marker["data"].(string)
	mimeType, _ := marker["mimeType"].(string)
	encoded = strings.TrimSpace(encoded)
	mimeType = strings.TrimSpace(strings.ToLower(mimeType))
	if encoded == "" || mimeType == "" {
		return nil, "", errors.New("image marker is missing data or mimeType")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, "", fmt.Errorf("decode screenshot base64: %w", err)
	}
	if len(data) == 0 {
		return nil, "", errors.New("screenshot image is empty")
	}
	return data, mimeType, nil
}

func imageHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func validHTTPURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host != ""
}

func (p *Publisher) cached(hash string, now time.Time) (ImageRef, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, ok := p.cache[hash]
	if !ok || entry.Ref.ExpiresAt <= now.Add(cacheExpiryMargin).UnixMilli() {
		delete(p.cache, hash)
		return ImageRef{}, false
	}
	return entry.Ref, true
}

func (p *Publisher) remember(hash string, ref ImageRef) {
	p.mu.Lock()
	p.cache[hash] = cachedRef{Ref: ref}
	p.mu.Unlock()
}

func readLimitedResponse(response *http.Response) ([]byte, error) {
	defer response.Body.Close()
	return io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
}

func (p *Publisher) prepare(ctx context.Context, data []byte, mimeType, hash string) (prepareResponse, error) {
	raw, err := json.Marshal(prepareRequest{SHA256: hash, ContentType: mimeType, Size: int64(len(data))})
	if err != nil {
		return prepareResponse{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.PrepareURL, bytes.NewReader(raw))
	if err != nil {
		return prepareResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	if err := p.config.Authorize(request, raw); err != nil {
		return prepareResponse{}, fmt.Errorf("authorize visual presign: %w", err)
	}
	response, err := p.client.Do(request)
	if err != nil {
		return prepareResponse{}, fmt.Errorf("request visual presign: %w", err)
	}
	payload, err := readLimitedResponse(response)
	if err != nil {
		return prepareResponse{}, err
	}
	if response.StatusCode == http.StatusServiceUnavailable && bytes.Contains(payload, []byte("media_not_configured")) {
		return prepareResponse{}, ErrNotConfigured
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return prepareResponse{}, fmt.Errorf("CodeLocal media presign returned HTTP %d", response.StatusCode)
	}
	var prepared prepareResponse
	if err := json.Unmarshal(payload, &prepared); err != nil {
		return prepareResponse{}, fmt.Errorf("decode visual presign response: %w", err)
	}
	return prepared, nil
}

func (p *Publisher) directUpload(ctx context.Context, data []byte, grant uploadGrant) error {
	if !grant.Required {
		return nil
	}
	if !validHTTPURL(grant.URL) {
		return errors.New("visual upload grant has invalid URL")
	}
	method := strings.ToUpper(strings.TrimSpace(grant.Method))
	if method == "" {
		method = http.MethodPut
	}
	if method != http.MethodPut {
		return errors.New("visual upload grant must use PUT")
	}
	request, err := http.NewRequestWithContext(ctx, method, grant.URL, bytes.NewReader(data))
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
		return fmt.Errorf("upload visual directly to object storage: %w", err)
	}
	_, _ = readLimitedResponse(response)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("object storage visual upload returned HTTP %d", response.StatusCode)
	}
	return nil
}

func (p *Publisher) upload(ctx context.Context, data []byte, mimeType, hash string) (ImageRef, error) {
	if ref, ok := p.cached(hash, time.Now()); ok {
		return ref, nil
	}
	prepared, err := p.prepare(ctx, data, mimeType, hash)
	if err != nil {
		return ImageRef{}, err
	}
	if prepared.SHA256 != "" && !strings.EqualFold(prepared.SHA256, hash) {
		return ImageRef{}, errors.New("CodeLocal media returned mismatched image hash")
	}
	if !validHTTPURL(prepared.URL) || prepared.ExpiresAt <= time.Now().UnixMilli() {
		return ImageRef{}, errors.New("CodeLocal media returned invalid or expired signed URL")
	}
	if err := p.directUpload(ctx, data, prepared.Upload); err != nil {
		return ImageRef{}, err
	}
	ref := ImageRef{
		ImageRef: first(prepared.ImageRef, "sha256:"+hash), URL: prepared.URL,
		ContentType: first(prepared.ContentType, mimeType), Size: prepared.Size,
		SHA256: hash, ExpiresAt: prepared.ExpiresAt, Transport: "signed-url-direct",
	}
	if ref.Size <= 0 {
		ref.Size = int64(len(data))
	}
	p.remember(hash, ref)
	return ref, nil
}

func first(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func refMap(ref ImageRef) map[string]any {
	return map[string]any{
		"imageRef": ref.ImageRef, "url": ref.URL, "mimeType": ref.ContentType, "size": ref.Size,
		"sha256": ref.SHA256, "expiresAt": ref.ExpiresAt, "transport": ref.Transport,
	}
}

func (p *Publisher) Transform(ctx context.Context, result any) (any, error) {
	root, marker, ok := imageMarker(result)
	if !ok || !p.Enabled() {
		return result, nil
	}
	data, mimeType, err := decodedImage(marker)
	if err != nil {
		return p.transportFailure(root, err)
	}
	ref, err := p.upload(ctx, data, mimeType, imageHash(data))
	if errors.Is(err, ErrNotConfigured) {
		return result, nil
	}
	if err != nil {
		return p.transportFailure(root, err)
	}
	output := cloneMap(root)
	delete(output, "__mcpImage")
	output["__mcpImageRef"] = refMap(ref)
	return output, nil
}

func (p *Publisher) transportFailure(root map[string]any, cause error) (any, error) {
	if p.config.Base64Fallback {
		output := cloneMap(root)
		output["visualTransportFallback"] = "base64"
		return output, nil
	}
	output := cloneMap(root)
	delete(output, "__mcpImage")
	output["visualTransportError"] = cause.Error()
	return output, fmt.Errorf("private media transport failed: %w", cause)
}
