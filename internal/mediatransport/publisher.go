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
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultRequestTimeout = 12 * time.Second
	maxResponseBytes      = 256 << 10
	cacheExpiryMargin     = 30 * time.Second
)

type Config struct {
	UploadURL      string
	Token          string
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

type uploadResponse struct {
	ImageRef    string `json:"imageRef"`
	URL         string `json:"url"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	ExpiresAt   int64  `json:"expiresAt"`
}

type cachedRef struct {
	Ref ImageRef
}

type Publisher struct {
	config Config
	client *http.Client
	mu     sync.Mutex
	cache  map[string]cachedRef
}

func ConfigFromEnvironment() Config {
	return Config{
		UploadURL:      strings.TrimSpace(os.Getenv("CODELOCAL_MEDIA_UPLOAD_URL")),
		Token:          strings.TrimSpace(os.Getenv("CODELOCAL_MEDIA_UPLOAD_TOKEN")),
		Base64Fallback: envBool("CODELOCAL_MEDIA_BASE64_FALLBACK", false),
	}
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
		client = &http.Client{Timeout: defaultRequestTimeout}
	}
	return &Publisher{config: config, client: client, cache: map[string]cachedRef{}}
}

func (p *Publisher) Enabled() bool {
	return p != nil && strings.TrimSpace(p.config.UploadURL) != "" && strings.TrimSpace(p.config.Token) != ""
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
	if !ok {
		return root, nil, false
	}
	return root, marker, true
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

func imageFilename(mimeType string) string {
	switch mimeType {
	case "image/jpeg", "image/jpg":
		return "codelocal.jpg"
	case "image/webp":
		return "codelocal.webp"
	case "image/gif":
		return "codelocal.gif"
	default:
		return "codelocal.png"
	}
}

func multipartImage(data []byte, mimeType string) (*bytes.Buffer, string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="media"; filename="%s"`, imageFilename(mimeType)))
	header.Set("Content-Type", mimeType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(data); err != nil {
		return nil, "", err
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return body, writer.FormDataContentType(), nil
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
		if ok {
			delete(p.cache, hash)
		}
		return ImageRef{}, false
	}
	return entry.Ref, true
}

func (p *Publisher) remember(hash string, ref ImageRef) {
	p.mu.Lock()
	p.cache[hash] = cachedRef{Ref: ref}
	p.mu.Unlock()
}

func (p *Publisher) upload(ctx context.Context, data []byte, mimeType, hash string) (ImageRef, error) {
	if ref, ok := p.cached(hash, time.Now()); ok {
		return ref, nil
	}
	body, contentType, err := multipartImage(data, mimeType)
	if err != nil {
		return ImageRef{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.UploadURL, body)
	if err != nil {
		return ImageRef{}, err
	}
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("Authorization", "Bearer "+p.config.Token)
	response, err := p.client.Do(request)
	if err != nil {
		return ImageRef{}, fmt.Errorf("upload private visual: %w", err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return ImageRef{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ImageRef{}, fmt.Errorf("media server returned HTTP %d", response.StatusCode)
	}
	var uploaded uploadResponse
	if err := json.Unmarshal(payload, &uploaded); err != nil {
		return ImageRef{}, fmt.Errorf("decode media response: %w", err)
	}
	if uploaded.SHA256 != "" && !strings.EqualFold(uploaded.SHA256, hash) {
		return ImageRef{}, errors.New("media server returned mismatched image hash")
	}
	if !validHTTPURL(uploaded.URL) || uploaded.ExpiresAt <= time.Now().UnixMilli() {
		return ImageRef{}, errors.New("media server returned invalid or expired signed URL")
	}
	ref := ImageRef{
		ImageRef:    first(uploaded.ImageRef, "sha256:"+hash),
		URL:         uploaded.URL,
		ContentType: first(uploaded.ContentType, mimeType),
		Size:        uploaded.Size,
		SHA256:      hash,
		ExpiresAt:   uploaded.ExpiresAt,
		Transport:   "signed-url",
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
		"imageRef":  ref.ImageRef,
		"url":       ref.URL,
		"mimeType":  ref.ContentType,
		"size":      ref.Size,
		"sha256":    ref.SHA256,
		"expiresAt": ref.ExpiresAt,
		"transport": ref.Transport,
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
	hash := imageHash(data)
	ref, err := p.upload(ctx, data, mimeType, hash)
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
