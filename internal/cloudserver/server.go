package cloudserver

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/gateway"
	"github.com/0xmarkhydra/codelocal/internal/mcpgateway"
	"github.com/0xmarkhydra/codelocal/internal/oauth"
	"github.com/0xmarkhydra/codelocal/internal/protocol"
	"github.com/0xmarkhydra/codelocal/internal/ui"
	"github.com/0xmarkhydra/codelocal/internal/version"
	"github.com/0xmarkhydra/codelocal/internal/webauth"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

type Server struct {
	Store       *cloud.Store
	Activation  *cloud.ActivationStore
	Hub         *gateway.Hub
	Coordinator *gateway.Coordinator
	Workspaces  *gateway.WorkspaceService
	WebAuth     *webauth.Manager
	OAuth       *oauth.Server
	MCP         *mcpgateway.Service
	Mux         *http.ServeMux
	HTTP        *http.Server
	InstanceID  string
	startedAt   time.Time
	requests    atomic.Uint64
}

func randomID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err == nil {
		return hex.EncodeToString(buf)
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func New(ctx context.Context) (*Server, error) {
	base := strings.TrimRight(os.Getenv("PUBLIC_BASE_URL"), "/")
	if base == "" {
		base = "http://localhost:" + defaultPort()
	}
	secret := os.Getenv("MCP_AUTH_SECRET")
	if secret == "" {
		return nil, errors.New("MCP_AUTH_SECRET is required")
	}

	store, err := cloud.New(ctx)
	if err != nil {
		return nil, err
	}
	activation := cloud.NewActivationStore(ctx, store.Redis)
	instanceID := os.Getenv("CODELOCAL_GATEWAY_INSTANCE_ID")
	if instanceID == "" {
		instanceID = first(os.Getenv("RAILWAY_REPLICA_ID"), os.Getenv("HOSTNAME"), randomID())
	}

	hub := gateway.NewHub(store, instanceID)
	coordinator := gateway.NewCoordinator(ctx, store.Redis, instanceID, func(callCtx context.Context, call gateway.RoutedCall) gateway.RoutedResult {
		return hub.HandleRouted(callCtx, call)
	})
	hub.SetCoordinator(coordinator)
	workspaceService := &gateway.WorkspaceService{Store: store, Activation: activation, Hub: hub, Coordinator: coordinator}
	auth := webauth.New(store, base)
	oauthServer, err := oauth.New(store, auth, base, secret)
	if err != nil {
		_ = activation.Close()
		_ = coordinator.Close()
		store.Close()
		return nil, err
	}
	mcpService := mcpgateway.New(store, hub, workspaceService)

	s := &Server{
		Store:       store,
		Activation:  activation,
		Hub:         hub,
		Coordinator: coordinator,
		Workspaces:  workspaceService,
		WebAuth:     auth,
		OAuth:       oauthServer,
		MCP:         mcpService,
		Mux:         http.NewServeMux(),
		InstanceID:  instanceID,
		startedAt:   time.Now(),
	}
	s.routes()
	s.HTTP = &http.Server{
		Addr:              host() + ":" + defaultPort(),
		Handler:           s.middleware(s.Mux),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       75 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	go hub.HeartbeatLoop(ctx, 20*time.Second, 70*time.Second)
	return s, nil
}

func first(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func defaultPort() string {
	if value := os.Getenv("PORT"); value != "" {
		return value
	}
	return "3333"
}

func host() string {
	if value := os.Getenv("HOST"); value != "" {
		return value
	}
	return "0.0.0.0"
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		s.requests.Add(1)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-CodeLocal-Gateway", s.InstanceID)
		next.ServeHTTP(w, r)
		if r.URL.Path != "/health" {
			slog.Info("http request", "method", r.Method, "path", r.URL.Path, "durationMs", time.Since(started).Milliseconds(), "gateway", s.InstanceID)
		}
	})
}

func (s *Server) routes() {
	mux := s.Mux
	s.WebAuth.Register(mux)
	s.OAuth.Register(mux)
	mux.HandleFunc("GET /", s.landing)
	mux.Handle("GET /dashboard", s.WebAuth.Require(http.HandlerFunc(s.dashboard)))
	mux.Handle("GET /dashboard/devices", s.WebAuth.Require(http.HandlerFunc(s.dashboard)))
	mux.Handle("GET /dashboard/workspaces", s.WebAuth.Require(http.HandlerFunc(s.dashboard)))
	mux.Handle("GET /dashboard/usage", s.WebAuth.Require(http.HandlerFunc(s.dashboard)))
	mux.Handle("GET /dashboard/admin", s.WebAuth.Require(http.HandlerFunc(s.dashboard)))
	mux.Handle("GET /api/status", s.WebAuth.Require(http.HandlerFunc(s.apiStatus)))
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /assets/{name}", s.asset)
	mux.HandleFunc("GET /pair/approve", s.pairApproveGet)
	mux.Handle("POST /pair/approve", s.WebAuth.Require(http.HandlerFunc(s.pairApprovePost)))

	var pairStart http.Handler = http.HandlerFunc(s.pairStart)
	pairStart = webutil.RateLimit(s.Store, webutil.RateLimitOptions{
		Scope:  "pair-start-device",
		Limit:  12,
		Window: 10 * time.Minute,
		Subject: func(r *http.Request) string {
			var body struct {
				DeviceID string `json:"deviceId"`
			}
			raw, _ := readBodyReplay(r, 64<<10)
			_ = json.Unmarshal(raw, &body)
			return body.DeviceID
		},
	}, pairStart)
	mux.Handle("POST /pair/start", webutil.RateLimit(s.Store, webutil.RateLimitOptions{Scope: "pair-start-ip", Limit: 40, Window: 10 * time.Minute}, pairStart))

	var claim http.Handler = http.HandlerFunc(s.pairClaim)
	mux.Handle("POST /pair/claim", webutil.RateLimit(s.Store, webutil.RateLimitOptions{Scope: "pair-claim-ip", Limit: 300, Window: time.Minute}, claim))
	mux.HandleFunc("POST /api/client/auth/check", s.clientAuthCheck)
	mux.HandleFunc("POST /api/client/workspaces/sync", s.workspaceSync)
	mux.HandleFunc("POST /api/client/runtime/poll", s.runtimePoll)
	mux.HandleFunc("POST /api/client/runtime/revocation-ack", s.revocationAck)
	mux.Handle("/client", s.Hub)
	mux.Handle("/mcp", s.OAuth.RequireMCP(s.MCP.Handler()))

	if os.Getenv("CODELOCAL_ENABLE_PPROF") == "1" {
		mux.HandleFunc("GET /debug/pprof/", pprof.Index)
		mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)
	}
}

func readBodyReplay(r *http.Request, limit int64) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, limit))
	if err != nil {
		return nil, err
	}
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(raw))
	return raw, nil
}

func (s *Server) landing(w http.ResponseWriter, r *http.Request) {
	identity, _ := s.WebAuth.Identity(r)
	href := "/register"
	label := "Create free account"
	if identity != nil {
		href = "/dashboard"
		label = "Open dashboard"
	}
	action := `<div class="actions"><a class="btn primary" href="` + href + `">` + label + `</a></div>`
	body := `<div class="stack"><div class="row"><div class="row-title">You already have ChatGPT. Now let it code on your machine.</div><div class="row-meta">ChatGPT stays the AI brain. CodeLocal is the secure bridge to folders and tools you explicitly authorize.</div></div><div class="row"><div class="row-title">1 · Connect ChatGPT</div><div class="row-meta mono">` + ui.Escape(strings.TrimRight(s.WebAuth.PublicBaseURL, "/")+"/mcp") + `</div></div><div class="row"><div class="row-title">2 · Install</div><div class="row-meta mono">npm install -g codelocal@beta</div></div><div class="row"><div class="row-title">3 · Authorize a project</div><div class="row-meta mono">cd /path/to/project<br>codelocal .</div></div><div class="row"><div class="row-title">4 · Start one machine runtime</div><div class="row-meta mono">codelocal</div></div>` + action + `</div>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(ui.Page("CodeLocal", "ChatGPT does the thinking. CodeLocal gives it hands.", body)))
}

func (s *Server) asset(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.PathValue("name"))
	allowed := map[string]string{
		"chatgpt-plugin-icon.png": "image/png",
		"codelocal-icon.png":      "image/png",
		"apple-touch-icon.png":    "image/png",
		"favicon.ico":             "image/x-icon",
	}
	contentType := allowed[name]
	if contentType == "" {
		http.NotFound(w, r)
		return
	}
	data, err := os.ReadFile(filepath.Join("assets", name))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public,max-age=86400")
	_, _ = w.Write(data)
}

func (s *Server) identity(r *http.Request) (*webauth.Identity, bool) {
	identity, _ := s.WebAuth.Identity(r)
	return identity, identity != nil
}

func dashboardNav(identity *webauth.Identity) string {
	var out strings.Builder
	out.WriteString(`<div class="actions"><a class="btn" href="/dashboard">Overview</a><a class="btn" href="/dashboard/devices">Devices</a><a class="btn" href="/dashboard/workspaces">Workspaces</a><a class="btn" href="/dashboard/usage">Token usage</a>`)
	if cloud.IsAdminEmail(identity.User.Email) {
		out.WriteString(`<a class="btn" href="/dashboard/admin">Admin</a>`)
	}
	out.WriteString(`<form method="post" action="/logout">` + ui.Hidden(map[string]string{"csrf": identity.CSRF}) + `<button class="btn" type="submit">Sign out</button></form></div>`)
	return out.String()
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.identity(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.URL.Path == "/dashboard/admin" {
		s.adminDashboard(w, r, identity)
		return
	}
	devices, _ := s.Store.ListDevices(r.Context(), identity.User.ID)
	workspaces, _ := s.Workspaces.Catalog(r.Context(), identity.User.ID)
	usage24h, _ := s.Store.MCPUsageSummary(r.Context(), identity.User.ID, time.Now().Add(-24*time.Hour).UnixMilli())
	usage30d, _ := s.Store.MCPUsageSummary(r.Context(), identity.User.ID, time.Now().Add(-30*24*time.Hour).UnixMilli())
	usageAll, _ := s.Store.MCPUsageSummary(r.Context(), identity.User.ID, 0)
	recentUsage, _ := s.Store.RecentMCPUsage(r.Context(), identity.User.ID, 30)
	var body strings.Builder
	body.WriteString(dashboardNav(identity) + `<div style="height:18px"></div><div class="stack">`)
	inviteSource := identity.User.ReferredByCode
	if inviteSource == "" {
		inviteSource = "Root account"
	}
	body.WriteString(`<div class="row"><div class="row-title">Your referral code · <span class="mono">` + ui.Escape(identity.User.ReferralCode) + `</span></div><div class="row-meta">Share this code with people you want to invite. Invited by: ` + ui.Escape(inviteSource) + `.</div></div>`)
	body.WriteString(`<div class="row"><div class="row-title">Estimated MCP token usage</div><div class="row-meta">Counts only payload sent through CodeLocal MCP tool calls. ChatGPT does not expose the model's full conversation/billing token count to MCP servers, so these numbers are estimates rather than OpenAI billing tokens.</div></div>`)
	body.WriteString(usageRow("Last 24 hours", usage24h))
	body.WriteString(usageRow("Last 30 days", usage30d))
	body.WriteString(usageRow("All time", usageAll))
	if r.URL.Path == "/dashboard/usage" {
		for _, item := range recentUsage {
			total := item.InputTokensEst + item.OutputTokensEst
			body.WriteString(`<div class="row"><div class="row-title">` + ui.Escape(item.Tool) + ` · ` + fmt.Sprintf("%d", item.Calls) + ` calls · ~` + fmt.Sprintf("%d", total) + ` tokens</div><div class="row-meta mono">Hourly aggregate · ChatGPT → CodeLocal ~` + fmt.Sprintf("%d", item.InputTokensEst) + ` · CodeLocal → ChatGPT ~` + fmt.Sprintf("%d", item.OutputTokensEst) + ` · ` + ui.Escape(item.WorkspaceID) + ` · ` + time.UnixMilli(item.CreatedAt).Format(time.RFC3339) + `</div></div>`)
		}
	} else {
		body.WriteString(`<div class="row"><div class="row-title">Gateway</div><div class="row-meta mono">` + ui.Escape(s.InstanceID) + ` · Go ` + ui.Escape(runtime.Version()) + ` · ` + ui.Escape(version.Version) + `</div></div>`)
		for _, device := range devices {
			body.WriteString(`<div class="row"><div class="row-title">` + ui.Escape(device.DeviceName) + `</div><div class="row-meta mono">` + ui.Escape(device.DeviceID) + ` · last seen ` + time.UnixMilli(device.LastSeenAt).Format(time.RFC3339) + `</div></div>`)
		}
		for _, workspace := range workspaces {
			body.WriteString(`<div class="row"><div class="row-title">` + ui.Escape(workspace.WorkspaceName) + ` · ` + ui.Escape(workspace.Status) + `</div><div class="row-meta mono">` + ui.Escape(workspace.Key) + `</div></div>`)
		}
	}
	body.WriteString(`</div>`)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(ui.Page("CodeLocal Cloud", "Signed in as "+identity.User.Email, body.String())))
}

type adminUserState struct {
	cloud.AdminUser
	RuntimeActive bool
	MCPActive     bool
}

func adminStatus(user adminUserState) string {
	if user.MCPActive {
		return `<span class="badge green">Using MCP now</span>`
	}
	if user.RuntimeActive {
		return `<span class="badge blue">Runtime online</span>`
	}
	return `<span class="badge muted">Offline</span>`
}

func renderReferralTree(users []adminUserState) string {
	byCode := map[string]adminUserState{}
	children := map[string][]adminUserState{}
	for _, user := range users {
		code := cloud.NormalizeReferralCode(user.ReferralCode)
		byCode[code] = user
		parent := cloud.NormalizeReferralCode(user.ReferredByCode)
		children[parent] = append(children[parent], user)
	}
	visited := map[string]bool{}
	var out strings.Builder
	var walk func(adminUserState, int)
	walk = func(user adminUserState, depth int) {
		if visited[user.ID] {
			return
		}
		visited[user.ID] = true
		indent := depth * 22
		out.WriteString(`<div class="tree-node" style="margin-left:` + fmt.Sprintf("%d", indent) + `px"><div><strong>` + ui.Escape(user.Email) + `</strong> ` + adminStatus(user) + `</div><div class="row-meta mono">code ` + ui.Escape(user.ReferralCode) + ` · ` + fmt.Sprintf("%d", user.InviteCount) + ` direct invite(s)</div></div>`)
		for _, child := range children[cloud.NormalizeReferralCode(user.ReferralCode)] {
			walk(child, depth+1)
		}
	}
	if root, ok := byCode["MMON"]; ok {
		walk(root, 0)
	} else if len(children["MMON"]) > 0 {
		out.WriteString(`<div class="tree-node"><div><strong>MMON</strong> <span class="badge muted">Legacy root</span></div></div>`)
		for _, child := range children["MMON"] {
			walk(child, 1)
		}
	}
	for _, user := range users {
		if !visited[user.ID] {
			walk(user, 0)
		}
	}
	return out.String()
}

func (s *Server) adminDashboard(w http.ResponseWriter, r *http.Request, identity *webauth.Identity) {
	if !cloud.IsAdminEmail(identity.User.Email) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	users, err := s.Store.ListAdminUsers(r.Context())
	if err != nil {
		http.Error(w, "Unable to load admin dashboard", http.StatusInternalServerError)
		return
	}
	userIDs := make([]string, 0, len(users))
	for _, user := range users {
		userIDs = append(userIDs, user.ID)
	}
	runtimeActiveMap, _ := s.Activation.UserOnlineMap(r.Context(), userIDs)
	mcpActiveMap, _ := s.Store.UserMCPActiveMap(r.Context(), userIDs)
	states := make([]adminUserState, 0, len(users))
	runtimeCount := 0
	usingCount := 0
	activeCount := 0
	for _, user := range users {
		runtimeActive := runtimeActiveMap[user.ID]
		mcpActive := mcpActiveMap[user.ID]
		state := adminUserState{AdminUser: user, RuntimeActive: runtimeActive, MCPActive: mcpActive}
		states = append(states, state)
		if runtimeActive {
			runtimeCount++
		}
		if mcpActive {
			usingCount++
		}
		if runtimeActive || mcpActive {
			activeCount++
		}
	}
	var body strings.Builder
	body.WriteString(dashboardNav(identity) + `<div style="height:18px"></div>`)
	body.WriteString(`<div class="metrics"><div class="metric-box"><div class="metric-label">Total users</div><div class="metric-value">` + fmt.Sprintf("%d", len(states)) + `</div></div><div class="metric-box"><div class="metric-label">Active users</div><div class="metric-value">` + fmt.Sprintf("%d", activeCount) + `</div></div><div class="metric-box"><div class="metric-label">Runtime online</div><div class="metric-value">` + fmt.Sprintf("%d", runtimeCount) + `</div></div><div class="metric-box"><div class="metric-label">Using MCP now</div><div class="metric-value">` + fmt.Sprintf("%d", usingCount) + `</div></div></div>`)
	body.WriteString(`<div style="height:18px"></div><div class="row"><div class="row-title">Users</div><div class="row-meta">Using MCP now means at least one CodeLocal MCP tool call in the last 5 minutes. Runtime online comes from the live machine heartbeat.</div></div><div class="stack">`)
	for _, user := range states {
		parent := user.ReferredByCode
		if parent == "" {
			parent = "—"
		}
		lastUsed := "never"
		if user.LastMCPUsedAt > 0 {
			lastUsed = time.UnixMilli(user.LastMCPUsedAt).Format(time.RFC3339)
		}
		body.WriteString(`<div class="row"><div class="row-title">` + ui.Escape(user.Email) + ` ` + adminStatus(user) + `</div><div class="row-meta mono">code ` + ui.Escape(user.ReferralCode) + ` · invited by ` + ui.Escape(parent) + ` · ` + fmt.Sprintf("%d", user.InviteCount) + ` direct invite(s)</div><div class="row-meta">Joined ` + time.UnixMilli(user.CreatedAt).Format(time.RFC3339) + ` · last MCP bucket ` + ui.Escape(lastUsed) + `</div></div>`)
	}
	body.WriteString(`</div><div style="height:18px"></div><div class="row"><div class="row-title">Referral tree</div><div class="row-meta">Direct parent → child relationships. MMON is the root referral for accounts that existed before referral gating.</div></div><div class="tree">` + renderReferralTree(states) + `</div>`)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(ui.Page("Admin · CodeLocal Cloud", "User activity and referral relationships", body.String())))
}

func usageRow(label string, value cloud.MCPUsageSummary) string {
	return `<div class="row"><div class="row-title">` + ui.Escape(label) + ` · ~` + fmt.Sprintf("%d", value.TotalTokensEst) + ` tokens</div><div class="row-meta mono">` + fmt.Sprintf("%d", value.Calls) + ` tool calls · ChatGPT → CodeLocal ~` + fmt.Sprintf("%d", value.InputTokensEst) + ` · CodeLocal → ChatGPT ~` + fmt.Sprintf("%d", value.OutputTokensEst) + `</div></div>`
}

func (s *Server) apiStatus(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.identity(r)
	if !ok {
		webutil.JSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	workspaces, _ := s.Workspaces.Catalog(r.Context(), identity.User.ID)
	devices, _ := s.Store.ListDevices(r.Context(), identity.User.ID)
	usage24h, _ := s.Store.MCPUsageSummary(r.Context(), identity.User.ID, time.Now().Add(-24*time.Hour).UnixMilli())
	usage30d, _ := s.Store.MCPUsageSummary(r.Context(), identity.User.ID, time.Now().Add(-30*24*time.Hour).UnixMilli())
	usageAll, _ := s.Store.MCPUsageSummary(r.Context(), identity.User.ID, 0)
	for i := range devices {
		devices[i].SecretHash = ""
	}
	webutil.JSON(w, http.StatusOK, map[string]any{
		"user":       map[string]any{"id": identity.User.ID, "email": identity.User.Email},
		"workspaces": workspaces,
		"devices":    devices,
		"gateway":    s.InstanceID,
		"mcpTokenUsage": map[string]any{
			"estimated": true,
			"scope":     "MCP payload only; not full ChatGPT model/billing tokens",
			"last24h":   usage24h,
			"last30d":   usage30d,
			"allTime":   usageAll,
		},
	})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	webutil.JSON(w, http.StatusOK, map[string]any{
		"ok":                    true,
		"version":               version.Version,
		"protocolVersion":       protocol.Version,
		"goVersion":             runtime.Version(),
		"gateway":               s.InstanceID,
		"uptimeSeconds":         int64(time.Since(s.startedAt).Seconds()),
		"onlineLocalWorkspaces": len(s.Hub.LocalClients("")),
		"requests":              s.requests.Load(),
		"memory": map[string]any{
			"allocBytes":     mem.Alloc,
			"heapInUseBytes": mem.HeapInuse,
			"sysBytes":       mem.Sys,
			"gcCycles":       mem.NumGC,
		},
		"goroutines": runtime.NumGoroutine(),
	})
}

func (s *Server) pairStart(w http.ResponseWriter, r *http.Request) {
	var input struct {
		DeviceID   string `json:"deviceId"`
		DeviceName string `json:"deviceName"`
	}
	if err := webutil.DecodeJSON(r, 64<<10, &input); err != nil || strings.TrimSpace(input.DeviceID) == "" || len(input.DeviceID) > 200 {
		webutil.JSON(w, http.StatusBadRequest, map[string]any{"error": "deviceId_required"})
		return
	}
	if input.DeviceName == "" {
		input.DeviceName = input.DeviceID
	}
	if len(input.DeviceName) > 120 {
		input.DeviceName = input.DeviceName[:120]
	}
	pairing, err := s.Store.CreatePairing(r.Context(), input.DeviceID, input.DeviceName, 10*time.Minute)
	if err != nil {
		webutil.JSON(w, http.StatusInternalServerError, map[string]any{"error": "pairing_failed"})
		return
	}
	base := strings.TrimRight(s.WebAuth.PublicBaseURL, "/")
	webutil.JSON(w, http.StatusOK, map[string]any{
		"pairingId":  pairing.PairingID,
		"code":       pairing.Code,
		"expiresAt":  pairing.ExpiresAt,
		"approveUrl": base + "/pair/approve?pairingId=" + url.QueryEscape(pairing.PairingID),
	})
}

func (s *Server) pairApproveGet(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("pairingId")
	pairing, _ := s.Store.GetPairing(r.Context(), id)
	if pairing == nil || pairing.ExpiresAt <= time.Now().UnixMilli() || pairing.ClaimedAt != 0 {
		http.NotFound(w, r)
		return
	}
	identity, _ := s.WebAuth.Identity(r)
	if identity == nil {
		http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
		return
	}
	body := `<div class="row"><div class="row-title">` + ui.Escape(pairing.DeviceName) + `</div><div class="row-meta mono">Device ID: ` + ui.Escape(pairing.DeviceID) + `</div></div><div style="height:14px"></div><form class="form" method="post" action="/pair/approve">` + ui.Hidden(map[string]string{"csrf": identity.CSRF, "pairingId": pairing.PairingID, "code": pairing.Code}) + `<button class="btn primary" type="submit">Approve device</button></form>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(ui.Page("Approve device", "Pair this machine with "+identity.User.Email+".", body)))
}

func (s *Server) pairApprovePost(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.identity(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if !s.WebAuth.VerifyCSRF(r) {
		http.Error(w, "Invalid security token.", http.StatusForbidden)
		return
	}
	pairing, err := s.Store.ApprovePairing(r.Context(), r.FormValue("pairingId"), r.FormValue("code"), identity.User.ID)
	if err != nil || pairing == nil {
		http.Error(w, "Invalid or expired pairing request/code.", http.StatusBadRequest)
		return
	}
	s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "device.pairing_approved", DeviceID: pairing.DeviceID, Detail: map[string]any{"deviceName": pairing.DeviceName}})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(ui.Page("Device approved", "Return to your terminal. CodeLocal will claim its credential automatically.", `<a class="btn primary" href="/dashboard/devices">View devices</a>`)))
}

func (s *Server) pairClaim(w http.ResponseWriter, r *http.Request) {
	var input struct {
		PairingID string `json:"pairingId"`
		Code      string `json:"code"`
	}
	if webutil.DecodeJSON(r, 64<<10, &input) != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	credentialID := "cld_" + randomID()
	secret := cloud.RandomHex(40)
	device, err := s.Store.ClaimPairing(r.Context(), input.PairingID, input.Code, credentialID, cloud.HashSecret(secret))
	if err != nil {
		webutil.JSON(w, http.StatusInternalServerError, map[string]any{"error": "pairing_failed"})
		return
	}
	if device == nil {
		webutil.JSON(w, http.StatusConflict, map[string]any{"error": "pairing_not_approved_or_expired"})
		return
	}
	s.Store.Audit(cloud.AuditEvent{UserID: device.UserID, Event: "device.paired", DeviceID: device.DeviceID, Detail: map[string]any{"credentialId": credentialID, "deviceName": device.DeviceName}})
	webutil.JSON(w, http.StatusOK, map[string]any{
		"credentialId":     credentialID,
		"credentialSecret": secret,
		"deviceId":         device.DeviceID,
		"deviceName":       device.DeviceName,
	})
}

func deviceAuth(r *http.Request) (string, string) {
	id := r.Header.Get("X-CodeLocal-Credential-Id")
	auth := r.Header.Get("Authorization")
	secret := ""
	if strings.HasPrefix(auth, "Device ") {
		secret = strings.TrimPrefix(auth, "Device ")
	}
	return id, secret
}

func (s *Server) authenticateDevice(r *http.Request) (*cloud.Device, error) {
	id, secret := deviceAuth(r)
	if id == "" || secret == "" {
		return nil, nil
	}
	return s.Store.AuthenticateDevice(r.Context(), id, cloud.HashSecret(secret))
}

func (s *Server) clientAuthCheck(w http.ResponseWriter, r *http.Request) {
	device, err := s.authenticateDevice(r)
	if err != nil || device == nil {
		webutil.JSON(w, http.StatusUnauthorized, map[string]any{"error": "device_auth_failed"})
		return
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"ok": true, "deviceId": device.DeviceID, "now": time.Now().UnixMilli()})
}

func (s *Server) workspaceSync(w http.ResponseWriter, r *http.Request) {
	device, err := s.authenticateDevice(r)
	if err != nil || device == nil {
		webutil.JSON(w, http.StatusUnauthorized, map[string]any{"error": "device_auth_failed"})
		return
	}
	var input struct {
		ClientVersion string `json:"clientVersion"`
		Workspaces    []struct {
			WorkspaceID   string `json:"workspaceId"`
			WorkspaceName string `json:"workspaceName"`
		} `json:"workspaces"`
	}
	if webutil.DecodeJSON(r, 1<<20, &input) != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	if len(input.Workspaces) > 500 {
		input.Workspaces = input.Workspaces[:500]
	}

	ids := make([]string, 0, len(input.Workspaces))
	synced := 0
	validID := regexp.MustCompile(`^[A-Za-z0-9._-]{1,80}$`)
	for _, item := range input.Workspaces {
		if !validID.MatchString(item.WorkspaceID) {
			continue
		}
		name := strings.TrimSpace(item.WorkspaceName)
		if name == "" {
			name = item.WorkspaceID
		}
		if len(name) > 120 {
			name = name[:120]
		}
		ids = append(ids, item.WorkspaceID)
		key := gateway.ClientKey(device.UserID, device.DeviceID, item.WorkspaceID)
		owner, _ := s.Coordinator.Owner(r.Context(), key)
		if owner == "" {
			caps := map[string]any{"authorized": true, "sleeping": true}
			if input.ClientVersion != "" {
				caps["clientVersion"] = truncate(input.ClientVersion, 80)
			}
			_ = s.Store.UpsertWorkspace(r.Context(), cloud.Workspace{
				UserID:          device.UserID,
				DeviceID:        device.DeviceID,
				WorkspaceID:     item.WorkspaceID,
				WorkspaceName:   name,
				ProtocolVersion: protocol.Version,
				Capabilities:    caps,
			})
		}
		synced++
	}

	removed, err := s.Store.ReconcileWorkspaces(r.Context(), device.UserID, device.DeviceID, ids)
	if err != nil {
		webutil.JSON(w, http.StatusInternalServerError, map[string]any{"error": "workspace_sync_failed"})
		return
	}
	// Older runtimes acknowledge a revoke by removing the workspace locally and
	// syncing the reduced registry. Convert that sync into the same acknowledgement
	// event used by newer runtimes so users do not see a false timeout.
	for _, workspaceID := range removed {
		_, _ = s.Activation.AcknowledgeRevocation(r.Context(), device.UserID, device.DeviceID, workspaceID, "")
	}
	_ = s.Activation.Heartbeat(r.Context(), device.UserID, device.DeviceID, ids, 45*time.Second)
	s.Store.Audit(cloud.AuditEvent{UserID: device.UserID, Event: "runtime.workspaces_synced", DeviceID: device.DeviceID, Detail: map[string]any{"count": synced, "removed": len(removed)}})
	webutil.JSON(w, http.StatusOK, map[string]any{"synced": synced, "removed": len(removed), "syncedAt": time.Now().UnixMilli()})
}

func truncate(value string, n int) string {
	value = strings.TrimSpace(value)
	if len(value) > n {
		return value[:n]
	}
	return value
}

func (s *Server) runtimePoll(w http.ResponseWriter, r *http.Request) {
	device, err := s.authenticateDevice(r)
	if err != nil || device == nil {
		webutil.JSON(w, http.StatusUnauthorized, map[string]any{"error": "device_auth_failed"})
		return
	}
	var input struct {
		WorkspaceIDs []string `json:"workspaceIds"`
		WaitMS       int      `json:"waitMs"`
	}
	if webutil.DecodeJSON(r, 1<<20, &input) != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	if len(input.WorkspaceIDs) > 500 {
		input.WorkspaceIDs = input.WorkspaceIDs[:500]
	}
	wait := time.Duration(input.WaitMS) * time.Millisecond
	if wait < 0 {
		wait = 0
	}
	if wait > 30*time.Second {
		wait = 30 * time.Second
	}
	ttl := wait + 15*time.Second
	if ttl < 20*time.Second {
		ttl = 20 * time.Second
	}
	_ = s.Activation.Heartbeat(r.Context(), device.UserID, device.DeviceID, input.WorkspaceIDs, ttl)
	activation, revocation, err := s.Activation.WaitForNext(r.Context(), device.UserID, device.DeviceID, wait)
	if err != nil && r.Context().Err() == nil {
		webutil.JSON(w, http.StatusInternalServerError, map[string]any{"error": "runtime_poll_failed"})
		return
	}
	if activation != nil {
		authorized, _ := s.Activation.IsAuthorized(r.Context(), device.UserID, device.DeviceID, activation.WorkspaceID)
		if !authorized {
			s.Store.Audit(cloud.AuditEvent{UserID: device.UserID, Event: "workspace.activation_rejected", DeviceID: device.DeviceID, WorkspaceID: activation.WorkspaceID, Detail: map[string]any{"requestId": activation.RequestID, "reason": "not-authorized"}})
			activation = nil
		}
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"activation": activation, "revocation": revocation, "now": time.Now().UnixMilli()})
}

func (s *Server) revocationAck(w http.ResponseWriter, r *http.Request) {
	device, err := s.authenticateDevice(r)
	if err != nil || device == nil {
		webutil.JSON(w, http.StatusUnauthorized, map[string]any{"error": "device_auth_failed"})
		return
	}
	var input struct {
		RequestID   string `json:"requestId"`
		WorkspaceID string `json:"workspaceId"`
	}
	if webutil.DecodeJSON(r, 64<<10, &input) != nil || input.RequestID == "" {
		webutil.JSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	acked, err := s.Activation.AcknowledgeRevocation(r.Context(), device.UserID, device.DeviceID, input.WorkspaceID, input.RequestID)
	if err != nil {
		webutil.JSON(w, http.StatusInternalServerError, map[string]any{"error": "revocation_ack_failed"})
		return
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"ok": true, "acked": acked})
}

func (s *Server) ListenAndServe() error {
	slog.Info("CodeLocal Go Cloud starting", "addr", s.HTTP.Addr, "version", version.Version, "protocolVersion", protocol.Version, "gateway", s.InstanceID)
	err := s.HTTP.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) Shutdown(ctx context.Context) error {
	err := s.HTTP.Shutdown(ctx)
	s.Hub.Close()
	_ = s.Coordinator.Close()
	_ = s.Activation.Close()
	s.Store.Close()
	return err
}
