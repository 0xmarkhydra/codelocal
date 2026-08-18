package clientupdate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	surfaceHashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	surfaceHashB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func TestCheckToolSurfaceEstablishesBaselineThenDetectsChange(t *testing.T) {
	var hits atomic.Int32
	currentHash := surfaceHashA
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/health" {
			t.Fatalf("path=%q want /health", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"toolSurface":{"version":1,"hash":"` + currentHash + `","count":19}}`))
	}))
	defer server.Close()

	now := time.Unix(1_800_000_000, 0)
	options := SurfaceOptions{BaseURL: server.URL, StateDir: t.TempDir(), CacheTTL: time.Minute, HTTPClient: server.Client(), Now: func() time.Time { return now }}
	notice, err := CheckToolSurface(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if notice != nil {
		t.Fatalf("first surface observation should establish baseline: %#v", notice)
	}

	currentHash = surfaceHashB
	now = now.Add(2 * time.Minute)
	notice, err = CheckToolSurface(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if notice == nil || notice.Previous.Hash != surfaceHashA || notice.Current.Hash != surfaceHashB {
		t.Fatalf("surface change was not detected: %#v", notice)
	}
	if hits.Load() != 2 {
		t.Fatalf("health hits=%d want 2", hits.Load())
	}
}

func TestToolSurfaceLifecycleNotifiesOnceThenUsesNewBaseline(t *testing.T) {
	var hits atomic.Int32
	currentHash := surfaceHashA
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"toolSurface":{"version":1,"hash":"` + currentHash + `","count":19}}`))
	}))
	defer server.Close()

	now := time.Unix(1_800_000_000, 0)
	options := SurfaceOptions{BaseURL: server.URL, StateDir: t.TempDir(), CacheTTL: time.Minute, HTTPClient: server.Client(), Now: func() time.Time { return now }}
	if notice, err := CheckToolSurface(context.Background(), options); err != nil || notice != nil {
		t.Fatalf("baseline failed: notice=%#v err=%v", notice, err)
	}
	currentHash = surfaceHashB
	now = now.Add(2 * time.Minute)
	notice, err := CheckToolSurface(context.Background(), options)
	if err != nil || notice == nil || notice.Current.Hash != surfaceHashB {
		t.Fatalf("schema change did not request reconnect: notice=%#v err=%v", notice, err)
	}
	// The changed surface becomes the local baseline immediately. Until the next
	// TTL boundary, reconnecting/restarting does not spam another stale notice.
	if repeat, repeatErr := CheckToolSurface(context.Background(), options); repeatErr != nil || repeat != nil {
		t.Fatalf("new schema baseline should suppress duplicate notice: notice=%#v err=%v", repeat, repeatErr)
	}
	if hits.Load() != 2 {
		t.Fatalf("unexpected health requests after new baseline: %d", hits.Load())
	}
}

func TestCheckToolSurfaceFreshCacheSkipsNetwork(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(1_800_000_000, 0)
	if err := writeSurfaceCache(dir, surfaceCache{CheckedAt: now.UnixMilli(), Surface: SurfaceFingerprint{Version: 1, Hash: surfaceHashA, Count: 19}}); err != nil {
		t.Fatal(err)
	}
	notice, err := CheckToolSurface(context.Background(), SurfaceOptions{BaseURL: "http://127.0.0.1:1", StateDir: dir, CacheTTL: time.Hour, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if notice != nil {
		t.Fatalf("fresh cache should not produce a notice: %#v", notice)
	}
}

func TestCheckToolSurfaceAcceptsWebSocketCloudURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"toolSurface":{"version":1,"hash":"` + surfaceHashA + `","count":19}}`))
	}))
	defer server.Close()
	wsURL := strings.Replace(server.URL, "http://", "ws://", 1) + "/client"
	notice, err := CheckToolSurface(context.Background(), SurfaceOptions{BaseURL: wsURL, StateDir: t.TempDir(), CacheTTL: time.Minute, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if notice != nil {
		t.Fatalf("first observation should not notify: %#v", notice)
	}
}

func TestRenderSurfaceCLIIsActionable(t *testing.T) {
	text := RenderSurfaceCLI(SurfaceNotice{Previous: SurfaceFingerprint{Version: 1, Hash: surfaceHashA, Count: 19}, Current: SurfaceFingerprint{Version: 1, Hash: surfaceHashB, Count: 19}})
	for _, want := range []string{"MCP tools changed", "19 tools", "Reconnect or refresh CodeLocal in your AI client", "workspace grants"} {
		if !strings.Contains(text, want) {
			t.Fatalf("surface notice missing %q: %s", want, text)
		}
	}
}
