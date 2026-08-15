package clientupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/state"
)

const (
	defaultRegistryURL = "https://registry.npmjs.org"
	defaultCacheTTL    = time.Hour
)

type StartupOptions struct {
	InstalledVersion string
	StateDir         string
	Channel          string
	RegistryURL      string
	CacheTTL         time.Duration
	HTTPClient       *http.Client
	Now              func() time.Time
}

type startupCache struct {
	CheckedAt     int64  `json:"checkedAt"`
	Channel       string `json:"channel"`
	LatestVersion string `json:"latestVersion"`
}

func StartupOptionsFromEnv(installedVersion, stateDir string) StartupOptions {
	channel := strings.TrimSpace(os.Getenv("CODELOCAL_RELEASE_CHANNEL"))
	if !channelRE.MatchString(channel) {
		channel = "latest"
		if parsed, ok := parse(installedVersion); ok && len(parsed.prerelease) > 0 {
			channel = "beta"
		}
	}
	registry := strings.TrimSpace(os.Getenv("CODELOCAL_UPDATE_REGISTRY"))
	if registry == "" {
		registry = strings.TrimSpace(os.Getenv("npm_config_registry"))
	}
	if registry == "" {
		registry = defaultRegistryURL
	}
	return StartupOptions{
		InstalledVersion: installedVersion,
		StateDir:         stateDir,
		Channel:          channel,
		RegistryURL:      registry,
		CacheTTL:         defaultCacheTTL,
		HTTPClient:       &http.Client{Timeout: 1200 * time.Millisecond},
		Now:              time.Now,
	}
}

func StartupCheckEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CODELOCAL_UPDATE_CHECK"))) {
	case "0", "false", "off", "no":
		return false
	default:
		return true
	}
}

func normalizeStartupOptions(options StartupOptions) StartupOptions {
	options.InstalledVersion = normalizeVersion(options.InstalledVersion)
	options.Channel = strings.TrimSpace(options.Channel)
	if !channelRE.MatchString(options.Channel) {
		options.Channel = "latest"
	}
	options.RegistryURL = strings.TrimRight(strings.TrimSpace(options.RegistryURL), "/")
	if options.RegistryURL == "" {
		options.RegistryURL = defaultRegistryURL
	}
	if options.CacheTTL <= 0 {
		options.CacheTTL = defaultCacheTTL
	}
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{Timeout: 1200 * time.Millisecond}
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	return options
}

func startupCachePath(stateDir string) string {
	return filepath.Join(stateDir, "update-check.json")
}

func readStartupCache(stateDir string) (startupCache, bool) {
	if strings.TrimSpace(stateDir) == "" {
		return startupCache{}, false
	}
	var cached startupCache
	if err := state.ReadJSON(startupCachePath(stateDir), &cached); err != nil {
		return startupCache{}, false
	}
	if _, ok := parse(cached.LatestVersion); !ok {
		return startupCache{}, false
	}
	return cached, true
}

func writeStartupCache(stateDir string, cached startupCache) error {
	if strings.TrimSpace(stateDir) == "" {
		return nil
	}
	return state.WriteJSONAtomic(startupCachePath(stateDir), cached)
}

func noticeForVersion(installed, latest, channel string) *Notice {
	manifest := Manifest{
		LatestVersion:  latest,
		Channel:        channel,
		UpdateCommand:  "npm i -g codelocal@" + channel,
		RestartCommand: "codelocal",
		Message:        "A newer CodeLocal runtime/tool release is available.",
	}
	return Evaluate(installed, manifest)
}

func cacheNotice(options StartupOptions, cached startupCache) *Notice {
	if cached.Channel != options.Channel {
		return nil
	}
	return noticeForVersion(options.InstalledVersion, cached.LatestVersion, options.Channel)
}

func fetchDistTags(ctx context.Context, options StartupOptions) (map[string]string, error) {
	base, err := url.Parse(options.RegistryURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return nil, errors.New("invalid CodeLocal update registry URL")
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/-/package/codelocal/dist-tags"
	base.RawQuery = ""
	base.Fragment = ""
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "codelocal-update-check/1")
	resp, err := options.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("CodeLocal update registry returned %d", resp.StatusCode)
	}
	var tags map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, err
	}
	return tags, nil
}

// CheckStartup checks the npm dist-tag for the configured release channel. A
// private cache keeps normal startup fast and lets an already-known update be
// shown while offline. Network/cache failures never make CodeLocal startup fail.
func CheckStartup(ctx context.Context, options StartupOptions) (*Notice, error) {
	options = normalizeStartupOptions(options)
	now := options.Now()
	cached, hasCache := readStartupCache(options.StateDir)
	cacheAge := time.Duration(-1)
	if hasCache && cached.CheckedAt > 0 {
		cacheAge = now.Sub(time.UnixMilli(cached.CheckedAt))
	}
	if hasCache && cached.Channel == options.Channel && cacheAge >= 0 && cacheAge < options.CacheTTL {
		return cacheNotice(options, cached), nil
	}

	tags, err := fetchDistTags(ctx, options)
	if err != nil {
		if hasCache {
			return cacheNotice(options, cached), err
		}
		return nil, err
	}
	latest := normalizeVersion(tags[options.Channel])
	if _, ok := parse(latest); !ok {
		if hasCache {
			return cacheNotice(options, cached), errors.New("CodeLocal update registry returned an invalid version")
		}
		return nil, errors.New("CodeLocal update registry returned an invalid version")
	}
	fresh := startupCache{CheckedAt: now.UnixMilli(), Channel: options.Channel, LatestVersion: latest}
	if cacheErr := writeStartupCache(options.StateDir, fresh); cacheErr != nil {
		// The registry result is still valid for this startup; cache persistence is
		// an optimization and must not suppress an update notice.
		return noticeForVersion(options.InstalledVersion, latest, options.Channel), cacheErr
	}
	return noticeForVersion(options.InstalledVersion, latest, options.Channel), nil
}

func RenderCLI(notice Notice) string {
	return strings.Join([]string{
		"",
		"╭─ CodeLocal update available ─────────────────────────────╮",
		fmt.Sprintf("│  Installed: %-13s Latest: %-18s│", notice.InstalledVersion, notice.LatestVersion),
		"│  New runtime/tools are available.                         │",
		"│  Update: " + notice.UpdateCommand,
		"│  Then run: " + notice.RestartCommand,
		"╰────────────────────────────────────────────────────────────╯",
		"",
	}, "\n")
}
