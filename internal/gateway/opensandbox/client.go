package opensandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultAuthHeader = "OPEN-SANDBOX-API-KEY"
	defaultTimeout    = 30 * time.Second
	maxErrorBody      = 8 << 10
)

type Config struct {
	BaseURL    string
	APIKey     string
	AuthHeader string
	Timeout    time.Duration
	HTTPClient *http.Client
}

type Client struct {
	baseURL    string
	apiKey     string
	authHeader string
	httpClient *http.Client
}

type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	if strings.TrimSpace(e.Body) == "" {
		return fmt.Sprintf("opensandbox lifecycle request failed (%d)", e.StatusCode)
	}
	return fmt.Sprintf("opensandbox lifecycle request failed (%d): %s", e.StatusCode, e.Body)
}

func NewClient(config Config) (*Client, error) {
	baseURL, err := normalizeBaseURL(config.BaseURL)
	if err != nil {
		return nil, err
	}
	authHeader := strings.TrimSpace(config.AuthHeader)
	if authHeader == "" {
		authHeader = defaultAuthHeader
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		timeout := config.Timeout
		if timeout <= 0 {
			timeout = defaultTimeout
		}
		httpClient = &http.Client{Timeout: timeout}
	}
	return &Client{
		baseURL:    baseURL,
		apiKey:     config.APIKey,
		authHeader: authHeader,
		httpClient: httpClient,
	}, nil
}

func normalizeBaseURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("opensandbox base URL required")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid opensandbox base URL %q", value)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported opensandbox URL scheme %q", parsed.Scheme)
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		path = "/v1"
	} else if path != "/v1" && !strings.HasSuffix(path, "/v1") {
		path += "/v1"
	}
	parsed.Path = path
	return strings.TrimRight(parsed.String(), "/"), nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, output any) error {
	if c == nil || c.httpClient == nil {
		return errors.New("opensandbox client unavailable")
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode opensandbox request: %w", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("create opensandbox request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set(c.authHeader, c.apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("opensandbox request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		return &APIError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(raw))}
	}
	if output == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(output); err != nil {
		return fmt.Errorf("decode opensandbox response: %w", err)
	}
	return nil
}

// IsUnavailable reports infrastructure/transient failures that may safely allow
// Auto runtime routing to consider another provider. Authentication, validation
// and ownership failures intentionally return false.
func IsUnavailable(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		default:
			return false
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}
