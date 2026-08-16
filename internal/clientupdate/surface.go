package clientupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/state"
)

const defaultToolSurfaceCacheTTL = 15 * time.Minute

type SurfaceFingerprint struct {
	Version int    `json:"version"`
	Hash    string `json:"hash"`
	Count   int    `json:"count"`
}

type SurfaceNotice struct {
	Previous SurfaceFingerprint
	Current  SurfaceFingerprint
}

type SurfaceOptions struct {
	BaseURL    string
	StateDir   string
	CacheTTL   time.Duration
	HTTPClient *http.Client
	Now        func() time.Time
}

type surfaceCache struct {
	CheckedAt int64              `json:"checkedAt"`
	Surface   SurfaceFingerprint `json:"surface"`
}

func SurfaceOptionsForServer(baseURL, stateDir string) SurfaceOptions {
	return SurfaceOptions{
		BaseURL:    baseURL,
		StateDir:   stateDir,
		CacheTTL:   defaultToolSurfaceCacheTTL,
		HTTPClient: &http.Client{Timeout: 1200 * time.Millisecond},
		Now:        time.Now,
	}
}

func normalizeSurfaceOptions(options SurfaceOptions) (SurfaceOptions, error) {
	base, err := url.Parse(strings.TrimSpace(options.BaseURL))
	if err != nil || base.Host == "" {
		return options, errors.New("invalid CodeLocal Cloud URL")
	}
	switch base.Scheme {
	case "wss":
		base.Scheme = "https"
	case "ws":
		base.Scheme = "http"
	case "http", "https":
	default:
		return options, errors.New("invalid CodeLocal Cloud URL scheme")
	}
	base.Path = ""
	base.RawQuery = ""
	base.Fragment = ""
	options.BaseURL = strings.TrimRight(base.String(), "/")
	if options.CacheTTL <= 0 {
		options.CacheTTL = defaultToolSurfaceCacheTTL
	}
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{Timeout: 1200 * time.Millisecond}
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	return options, nil
}

func toolSurfaceCachePath(stateDir string) string {
	return filepath.Join(stateDir, "mcp-tool-surface.json")
}

func validSurface(surface SurfaceFingerprint) bool {
	if surface.Version <= 0 || surface.Count <= 0 || len(surface.Hash) != 64 {
		return false
	}
	for _, r := range surface.Hash {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}

func readSurfaceCache(stateDir string) (surfaceCache, bool) {
	if strings.TrimSpace(stateDir) == "" {
		return surfaceCache{}, false
	}
	var cached surfaceCache
	if state.ReadJSON(toolSurfaceCachePath(stateDir), &cached) != nil || !validSurface(cached.Surface) {
		return surfaceCache{}, false
	}
	return cached, true
}

func writeSurfaceCache(stateDir string, cached surfaceCache) error {
	if strings.TrimSpace(stateDir) == "" {
		return nil
	}
	return state.WriteJSONAtomic(toolSurfaceCachePath(stateDir), cached)
}

func fetchToolSurface(ctx context.Context, options SurfaceOptions) (SurfaceFingerprint, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, options.BaseURL+"/health", nil)
	if err != nil {
		return SurfaceFingerprint{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "codelocal-tool-surface-check/1")
	resp, err := options.HTTPClient.Do(req)
	if err != nil {
		return SurfaceFingerprint{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return SurfaceFingerprint{}, fmt.Errorf("CodeLocal Cloud health returned %d", resp.StatusCode)
	}
	var payload struct {
		ToolSurface SurfaceFingerprint `json:"toolSurface"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return SurfaceFingerprint{}, err
	}
	if !validSurface(payload.ToolSurface) {
		return SurfaceFingerprint{}, errors.New("CodeLocal Cloud did not advertise a valid MCP tool surface")
	}
	return payload.ToolSurface, nil
}

// CheckToolSurface detects Cloud MCP schema changes seen by this machine. The
// first observation establishes a baseline. Later hash changes produce a
// one-time reconnect notice; definitive stale-thread detection still happens in
// the MCP gateway when ChatGPT actually calls an old/unknown tool.
func CheckToolSurface(ctx context.Context, options SurfaceOptions) (*SurfaceNotice, error) {
	normalized, err := normalizeSurfaceOptions(options)
	if err != nil {
		return nil, err
	}
	options = normalized
	now := options.Now()
	cached, hasCache := readSurfaceCache(options.StateDir)
	if hasCache && cached.CheckedAt > 0 {
		age := now.Sub(time.UnixMilli(cached.CheckedAt))
		if age >= 0 && age < options.CacheTTL {
			return nil, nil
		}
	}
	current, err := fetchToolSurface(ctx, options)
	if err != nil {
		return nil, err
	}
	fresh := surfaceCache{CheckedAt: now.UnixMilli(), Surface: current}
	if err := writeSurfaceCache(options.StateDir, fresh); err != nil {
		return nil, err
	}
	if !hasCache || cached.Surface.Hash == current.Hash {
		return nil, nil
	}
	return &SurfaceNotice{Previous: cached.Surface, Current: current}, nil
}

func RenderSurfaceCLI(notice SurfaceNotice) string {
	return strings.Join([]string{
		"",
		"╭─ CodeLocal MCP tools changed ─────────────────────────────╮",
		fmt.Sprintf("│  Tool surface: v%d / %d tools", notice.Current.Version, notice.Current.Count),
		"│  CodeLocal Cloud is exposing a newer/different MCP schema.",
		"│  Reconnect or refresh CodeLocal in your AI client to reload tools/actions.",
		"│  Local pairing and workspace grants will be kept.",
		"╰────────────────────────────────────────────────────────────╯",
		"",
	}, "\n")
}
