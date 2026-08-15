package cloudserver

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
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
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/gateway"
	"github.com/0xmarkhydra/codelocal/internal/learnedskills"
	"github.com/0xmarkhydra/codelocal/internal/mcpgateway"
	"github.com/0xmarkhydra/codelocal/internal/memory"
	"github.com/0xmarkhydra/codelocal/internal/oauth"
	"github.com/0xmarkhydra/codelocal/internal/projectbrain"
	"github.com/0xmarkhydra/codelocal/internal/projectidentity"
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
	Memory      *memory.Store
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

func projectBrainCloudSyncEnabled(userID, deviceID string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CODELOCAL_PROJECT_BRAIN_CLOUD_SYNC"))) {
	case "0", "false", "off", "disabled":
		return false
	}
	// Rollout is opt-in and fail-closed. Deploying a new server binary without
	// configuring a cohort must never silently turn Project Brain on for 100%
	// of users.
	raw := strings.TrimSpace(os.Getenv("CODELOCAL_PROJECT_BRAIN_ROLLOUT_PERCENT"))
	if raw == "" {
		return false
	}
	percent, err := strconv.Atoi(raw)
	if err != nil || percent <= 0 {
		return false
	}
	if percent >= 100 {
		return true
	}
	digest := sha256.Sum256([]byte(strings.TrimSpace(userID) + "\x00" + strings.TrimSpace(deviceID)))
	bucket := int(binary.BigEndian.Uint16(digest[:2])) % 10000
	return bucket < percent*100
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
	}, func(userID, credentialID string) {
		hub.DisconnectCredential(userID, credentialID)
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
	var memoryStore *memory.Store
	memoryFlag := strings.ToLower(strings.TrimSpace(os.Getenv("CODELOCAL_MEMORY_ENABLED")))
	if memoryFlag != "0" && memoryFlag != "false" && memoryFlag != "off" {
		memoryStore = memory.NewStore(store.DB, memory.EmbedderFromEnv())
		probeCtx, probeCancel := context.WithTimeout(ctx, 5*time.Second)
		_ = memoryStore.ProbeVector(probeCtx)
		probeCancel()

		graphFlag := strings.ToLower(strings.TrimSpace(os.Getenv("CODELOCAL_MEMORY_GRAPH_ENABLED")))
		graphEnabled := graphFlag != "0" && graphFlag != "false" && graphFlag != "off"
		memoryStore.SetGraphEnabled(graphEnabled)
		if graphEnabled {
			backfillCtx, backfillCancel := context.WithTimeout(ctx, 4*time.Second)
			if count, backfillErr := memoryStore.BackfillGraph(backfillCtx, 500); backfillErr != nil && backfillCtx.Err() == nil {
				slog.Warn("memory graph startup backfill failed; vector memory remains available", "error", backfillErr)
			} else if count > 0 {
				slog.Info("memory graph startup backfill projected existing memories", "count", count)
			}
			backfillCancel()
			go func() {
				backfillCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				total := 0
				for batch := 0; batch < 10 && backfillCtx.Err() == nil; batch++ {
					count, backfillErr := memoryStore.BackfillGraph(backfillCtx, 500)
					if backfillErr != nil {
						if backfillCtx.Err() == nil {
							slog.Warn("memory graph background backfill stopped; vector memory remains available", "error", backfillErr, "projected", total)
						}
						return
					}
					total += count
					if count < 500 {
						if total > 0 {
							slog.Info("memory graph background backfill complete", "projected", total)
						}
						return
					}
				}
			}()
		}
	}
	mcpService := mcpgateway.New(store, hub, workspaceService, memoryStore)

	s := &Server{
		Store:       store,
		Activation:  activation,
		Hub:         hub,
		Coordinator: coordinator,
		Workspaces:  workspaceService,
		WebAuth:     auth,
		OAuth:       oauthServer,
		MCP:         mcpService,
		Memory:      memoryStore,
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
	mux.HandleFunc("/", s.landing)
	mux.Handle("GET /dashboard", s.WebAuth.Require(http.HandlerFunc(s.dashboard)))
	mux.Handle("GET /dashboard/devices", s.WebAuth.Require(http.HandlerFunc(s.dashboard)))
	mux.Handle("GET /dashboard/workspaces", s.WebAuth.Require(http.HandlerFunc(s.dashboard)))
	mux.Handle("GET /dashboard/knowledge", s.WebAuth.Require(http.HandlerFunc(s.dashboard)))
	mux.Handle("GET /dashboard/usage", s.WebAuth.Require(http.HandlerFunc(s.dashboard)))
	mux.Handle("GET /dashboard/admin", s.WebAuth.Require(http.HandlerFunc(s.dashboard)))
	mux.Handle("GET /api/status", s.WebAuth.Require(http.HandlerFunc(s.apiStatus)))
	mux.Handle("GET /api/collective/preferences", s.WebAuth.Require(http.HandlerFunc(s.collectivePreferencesGet)))
	mux.Handle("POST /api/collective/preferences", s.WebAuth.Require(http.HandlerFunc(s.collectivePreferencesPost)))
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
	mux.HandleFunc("POST /api/client/auth/logout", s.clientAuthLogout)
	mux.HandleFunc("POST /api/client/workspaces/sync", s.workspaceSync)
	mux.HandleFunc("POST /api/client/knowledge/sync", s.knowledgeSync)
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
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	identity, _ := s.WebAuth.Identity(r)
	href := "/register"
	label := "Create free account"
	if identity != nil {
		href = "/dashboard"
		label = "Open dashboard"
	}
	action := `<div class="actions"><a class="btn primary" href="` + href + `">` + label + `</a></div>`
	body := `<div class="stack"><div class="row"><div class="row-title">You already have ChatGPT. Now let it code on your machine.</div><div class="row-meta">ChatGPT stays the AI brain. CodeLocal is the secure bridge to folders and tools you explicitly authorize.</div></div><div class="row"><div class="row-title">1 · Connect ChatGPT</div><div class="row-meta mono">` + ui.Escape(strings.TrimRight(s.WebAuth.PublicBaseURL, "/")+"/mcp") + `</div></div><div class="row"><div class="row-title">2 · Install</div><div class="row-meta mono">npm install -g codelocal</div></div><div class="row"><div class="row-title">3 · Authorize a project</div><div class="row-meta mono">cd /path/to/project<br>codelocal .</div></div><div class="row"><div class="row-title">4 · Start one machine runtime</div><div class="row-meta mono">codelocal</div></div>` + action + `</div>`
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
	if r.URL.Path == "/dashboard/knowledge" {
		s.knowledgeDashboard(w, r, identity)
		return
	}
	devices, _ := s.Store.ListDevices(r.Context(), identity.User.ID)
	workspaces, _ := s.Workspaces.Catalog(r.Context(), identity.User.ID)
	usage24h, _ := s.Store.MCPUsageSummary(r.Context(), identity.User.ID, time.Now().Add(-24*time.Hour).UnixMilli())
	usage30d, _ := s.Store.MCPUsageSummary(r.Context(), identity.User.ID, time.Now().Add(-30*24*time.Hour).UnixMilli())
	usageAll, _ := s.Store.MCPUsageSummary(r.Context(), identity.User.ID, 0)
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
	if r.URL.Path != "/dashboard/usage" {
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
	var walk func(adminUserState, int) string
	walk = func(user adminUserState, depth int) string {
		if visited[user.ID] {
			return ""
		}
		visited[user.ID] = true
		code := cloud.NormalizeReferralCode(user.ReferralCode)
		childUsers := children[code]
		node := `<div><strong>` + ui.Escape(user.Email) + `</strong> ` + adminStatus(user) + `</div><div class="row-meta mono">code ` + ui.Escape(user.ReferralCode) + ` · ` + fmt.Sprintf("%d", user.InviteCount) + ` direct invite(s)</div>`
		if len(childUsers) == 0 {
			return `<div class="tree-node">` + node + `</div>`
		}
		var nested strings.Builder
		for _, child := range childUsers {
			nested.WriteString(walk(child, depth+1))
		}
		open := ""
		if depth == 0 {
			open = " open"
		}
		return `<details class="tree-branch"` + open + `><summary class="tree-node">` + node + `<span class="tree-chevron">` + ui.Icon("chevronRight") + `</span></summary><div class="tree-children">` + nested.String() + `</div></details>`
	}

	var out strings.Builder
	if root, ok := byCode["MMON"]; ok {
		out.WriteString(walk(root, 0))
	} else if len(children["MMON"]) > 0 {
		var nested strings.Builder
		for _, child := range children["MMON"] {
			nested.WriteString(walk(child, 1))
		}
		out.WriteString(`<details class="tree-branch" open><summary class="tree-node"><div><strong>MMON</strong> <span class="badge muted">Legacy root</span></div><span class="tree-chevron">` + ui.Icon("chevronRight") + `</span></summary><div class="tree-children">` + nested.String() + `</div></details>`)
	}
	for _, user := range users {
		if !visited[user.ID] {
			out.WriteString(walk(user, 0))
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

	queryText := mainQuery(r)
	query := strings.ToLower(queryText)
	filtered := make([]adminUserState, 0, len(states))
	for _, user := range states {
		haystack := strings.ToLower(user.Email + " " + user.ReferralCode + " " + user.ReferredByCode)
		if query == "" || strings.Contains(haystack, query) {
			filtered = append(filtered, user)
		}
	}
	page, start, end, totalPages := mainPageBounds(r, len(filtered))
	var userRows strings.Builder
	for _, user := range filtered[start:end] {
		parent := user.ReferredByCode
		if parent == "" {
			parent = "—"
		}
		lastMCP := ui.FormatTime(user.LastMCPUsedAt)
		lastDevice := ui.FormatTime(user.LastDeviceSeenAt)
		userRows.WriteString(`<div class="admin-row"><div class="admin-user"><div class="admin-user-email">` + ui.Escape(user.Email) + `</div><div class="admin-cell-sub">Joined ` + ui.Escape(ui.FormatTime(user.CreatedAt)) + `</div></div><div class="admin-status">` + adminStatus(user) + `</div><div><div class="admin-code mono">` + ui.Escape(user.ReferralCode) + `</div><div class="admin-cell-sub">Invited by ` + ui.Escape(parent) + ` · ` + fmt.Sprintf("%d", user.InviteCount) + ` direct</div></div><div><div class="admin-activity">MCP ` + ui.Escape(lastMCP) + `</div><div class="admin-cell-sub">Device ` + ui.Escape(lastDevice) + `</div></div></div>`)
	}
	if userRows.Len() == 0 {
		userRows.WriteString(`<div class="empty">No users match your search.</div>`)
	}

	body := `<div class="grid admin-grid">` +
		`<div class="card span12 admin-hero"><div><div class="section-kicker">Administration</div><div class="title">User network at a glance</div><div class="label">Live runtime status, MCP activity and referral growth in one place.</div></div><span class="badge blue">Admin only</span></div>` +
		`<div class="admin-stat-grid"><div class="admin-stat"><div class="admin-stat-label">Total users</div><div class="admin-stat-value">` + fmt.Sprintf("%d", len(states)) + `</div><div class="admin-stat-sub">Registered accounts</div></div><div class="admin-stat"><div class="admin-stat-label">Active users</div><div class="admin-stat-value">` + fmt.Sprintf("%d", activeCount) + `</div><div class="admin-stat-sub">Runtime or MCP active</div></div><div class="admin-stat"><div class="admin-stat-label">Runtime online</div><div class="admin-stat-value">` + fmt.Sprintf("%d", runtimeCount) + `</div><div class="admin-stat-sub">Live machine heartbeat</div></div><div class="admin-stat"><div class="admin-stat-label">Using MCP now</div><div class="admin-stat-value">` + fmt.Sprintf("%d", usingCount) + `</div><div class="admin-stat-sub">Tool call in last 5 min</div></div></div>` +
		`<div class="card span12"><div class="section-head"><div><div class="title">Users</div><div class="label">Search by email, referral code or inviter code. Status is computed from live runtime and MCP activity.</div></div></div>` + mainListToolbar("/dashboard/admin", queryText, len(filtered)) + `<div class="admin-table"><div class="admin-row admin-head"><div>User</div><div>Status</div><div>Referral</div><div>Last activity</div></div>` + userRows.String() + `</div>` + mainPager("/dashboard/admin", queryText, page, totalPages) + `</div>` +
		`<div class="card span12 referral-tree-card"><div class="section-head"><div><div class="title">Referral tree</div><div class="label">Parent → child relationships. MMON remains the legacy root marker for pre-gating accounts.</div></div><span class="badge blue">` + fmt.Sprintf("%d", len(states)) + ` users</span></div><div class="divider"></div><div class="tree">` + renderReferralTree(states) + `</div></div></div>`

	writeHTML(w, ui.DashboardPage(ui.DashboardOptions{
		Title: "Administration", Active: "admin", Email: identity.User.Email, CSRF: identity.CSRF,
		Subtitle: "Monitor account activity and referral relationships without exposing local source code.",
		Body:     body, IsAdmin: true,
	}))
}

func usageRow(label string, value cloud.MCPUsageSummary) string {
	return `<div class="row"><div class="row-title">` + ui.Escape(label) + ` · ~` + fmt.Sprintf("%d", value.TotalTokensEst) + ` tokens</div><div class="row-meta mono">` + fmt.Sprintf("%d", value.Calls) + ` tool calls · ChatGPT → CodeLocal ~` + fmt.Sprintf("%d", value.InputTokensEst) + ` · CodeLocal → ChatGPT ~` + fmt.Sprintf("%d", value.OutputTokensEst) + `</div></div>`
}

func schemaMigrationPayload(status cloud.SchemaMigrationStatus, err error) map[string]any {
	payload := map[string]any{
		"available":            err == nil,
		"currentVersion":       status.CurrentVersion,
		"targetVersion":        status.TargetVersion,
		"appliedCount":         status.AppliedCount,
		"upToDate":             err == nil && status.UpToDate,
		"projectBrainPlanHash": status.ProjectBrainPlanHash,
	}
	if status.TargetVersion == 0 {
		payload["targetVersion"] = cloud.LatestSchemaMigrationVersion()
	}
	if status.ProjectBrainPlanHash == "" {
		payload["projectBrainPlanHash"] = cloud.ProjectBrainMigrationPlanHash()
	}
	return payload
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
	schemaStatus, schemaErr := s.Store.SchemaMigrationStatus(r.Context())
	durableLearning, _ := s.Store.DurableOutboxHealth(r.Context(), identity.User.ID)
	_, canonicalGraphFreshness, _ := s.Store.CanonicalGraphFreshness(r.Context(), identity.User.ID)
	for i := range devices {
		devices[i].SecretHash = ""
	}
	webutil.JSON(w, http.StatusOK, map[string]any{
		"user":                    map[string]any{"id": identity.User.ID, "email": identity.User.Email},
		"workspaces":              workspaces,
		"devices":                 devices,
		"gateway":                 s.InstanceID,
		"schemaMigration":         schemaMigrationPayload(schemaStatus, schemaErr),
		"durableLearning":         durableLearning,
		"canonicalGraphFreshness": canonicalGraphFreshness,
		"mcpTokenUsage": map[string]any{
			"estimated": true,
			"scope":     "MCP payload only; not full ChatGPT model/billing tokens",
			"last24h":   usage24h,
			"last30d":   usage30d,
			"allTime":   usageAll,
		},
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	schemaCtx, schemaCancel := context.WithTimeout(r.Context(), 500*time.Millisecond)
	schemaStatus, schemaErr := s.Store.SchemaMigrationStatus(schemaCtx)
	schemaCancel()
	outboxCtx, outboxCancel := context.WithTimeout(r.Context(), 500*time.Millisecond)
	durableLearning, _ := s.Store.DurableOutboxHealth(outboxCtx, "")
	outboxCancel()
	graphCtx, graphCancel := context.WithTimeout(r.Context(), 500*time.Millisecond)
	_, canonicalGraphFreshness, _ := s.Store.CanonicalGraphFreshness(graphCtx, "")
	graphCancel()
	agentMemory := map[string]any{"enabled": s.Memory != nil}
	if s.Memory != nil {
		agentMemory["vectorAvailable"] = s.Memory.VectorAvailable()
		agentMemory["vectorDimension"] = s.Memory.VectorDimension()
		agentMemory["embeddingProvider"] = s.Memory.EmbeddingProvider()
		agentMemory["embeddingModel"] = s.Memory.EmbeddingModel()
		agentMemory["graphEnabled"] = s.Memory.GraphEnabled()
	}
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
		"goroutines":              runtime.NumGoroutine(),
		"agentMemory":             agentMemory,
		"schemaMigration":         schemaMigrationPayload(schemaStatus, schemaErr),
		"durableLearning":         durableLearning,
		"canonicalGraphFreshness": canonicalGraphFreshness,
		"toolSurface":             s.MCP.ToolSurface(),
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
		"pairingId":      pairing.PairingID,
		"code":           pairing.Code,
		"expiresAt":      pairing.ExpiresAt,
		"approveUrl":     base + "/pair/approve?pairingId=" + url.QueryEscape(pairing.PairingID),
		"retrySafeClaim": true,
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
	body := `<div class="row"><div class="row-title">` + ui.Escape(pairing.DeviceName) + `</div><div class="row-meta mono">Device ID: ` + ui.Escape(pairing.DeviceID) + `</div></div><div style="height:14px"></div><form class="form" method="post" action="/pair/approve">` + ui.Hidden(map[string]string{"csrf": identity.CSRF, "pairingId": pairing.PairingID, "code": pairing.Code}) + `<button class="btn primary" type="submit">Approve device</button></form><div style="height:10px"></div><form method="post" action="/logout">` + ui.Hidden(map[string]string{"csrf": identity.CSRF, "next": r.URL.RequestURI()}) + `<button class="btn" type="submit">Use another account</button></form>`
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

func (s *Server) disconnectCredentialEverywhere(userID, credentialID string) {
	if userID == "" || credentialID == "" {
		return
	}
	s.Hub.DisconnectCredential(userID, credentialID)
	if s.Coordinator != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := s.Coordinator.BroadcastCredentialDisconnect(ctx, userID, credentialID); err != nil {
			slog.Warn("credential disconnect broadcast failed", "error", err, "credentialId", credentialID)
		}
	}
}

func (s *Server) pairClaim(w http.ResponseWriter, r *http.Request) {
	var input struct {
		PairingID            string `json:"pairingId"`
		Code                 string `json:"code"`
		CredentialID         string `json:"credentialId"`
		CredentialSecretHash string `json:"credentialSecretHash"`
	}
	if webutil.DecodeJSON(r, 64<<10, &input) != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	credentialID := strings.TrimSpace(input.CredentialID)
	secretHash := strings.TrimSpace(input.CredentialSecretHash)
	secret := ""
	if (credentialID == "") != (secretHash == "") {
		webutil.JSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_credential_claim"})
		return
	}
	if credentialID == "" {
		// Backward compatibility for clients that predate retry-safe claims.
		credentialID = "cld_" + randomID()
		secret = cloud.RandomHex(40)
		secretHash = cloud.HashSecret(secret)
	} else {
		decodedHash, hashErr := hex.DecodeString(secretHash)
		if !strings.HasPrefix(credentialID, "cld_") || len(credentialID) > 128 || hashErr != nil || len(decodedHash) != 32 {
			webutil.JSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_credential_claim"})
			return
		}
	}
	device, err := s.Store.ClaimPairing(r.Context(), input.PairingID, input.Code, credentialID, secretHash)
	if err != nil {
		webutil.JSON(w, http.StatusInternalServerError, map[string]any{"error": "pairing_failed"})
		return
	}
	if device == nil {
		webutil.JSON(w, http.StatusConflict, map[string]any{"error": "pairing_not_approved_or_expired"})
		return
	}
	if device.PreviousCredentialID != "" && device.PreviousCredentialID != credentialID {
		s.disconnectCredentialEverywhere(device.UserID, device.PreviousCredentialID)
	}
	s.Store.Audit(cloud.AuditEvent{UserID: device.UserID, Event: "device.paired", DeviceID: device.DeviceID, Detail: map[string]any{"credentialId": credentialID, "deviceName": device.DeviceName}})
	output := map[string]any{
		"credentialId": credentialID,
		"deviceId":     device.DeviceID,
		"deviceName":   device.DeviceName,
	}
	if user, userErr := s.Store.UserByID(r.Context(), device.UserID); userErr == nil && user != nil {
		output["email"] = user.Email
	}
	if secret != "" {
		output["credentialSecret"] = secret
	}
	webutil.JSON(w, http.StatusOK, output)
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
	output := map[string]any{"ok": true, "deviceId": device.DeviceID, "now": time.Now().UnixMilli()}
	if user, userErr := s.Store.UserByID(r.Context(), device.UserID); userErr == nil && user != nil {
		output["email"] = user.Email
	}
	webutil.JSON(w, http.StatusOK, output)
}

func (s *Server) clientAuthLogout(w http.ResponseWriter, r *http.Request) {
	device, err := s.authenticateDevice(r)
	if err != nil || device == nil {
		webutil.JSON(w, http.StatusUnauthorized, map[string]any{"error": "device_auth_failed"})
		return
	}
	credentialID, _ := deviceAuth(r)
	revoked, err := s.Store.RevokeDevice(r.Context(), device.UserID, credentialID)
	if err != nil {
		webutil.JSON(w, http.StatusInternalServerError, map[string]any{"error": "device_logout_failed"})
		return
	}
	s.disconnectCredentialEverywhere(device.UserID, credentialID)
	presenceCtx, cancelPresence := context.WithTimeout(context.Background(), 2*time.Second)
	_ = s.Activation.ClearPresence(presenceCtx, device.UserID, device.DeviceID)
	cancelPresence()
	s.Store.Audit(cloud.AuditEvent{UserID: device.UserID, Event: "device.logout", DeviceID: device.DeviceID, Detail: map[string]any{"credentialId": credentialID}})
	webutil.JSON(w, http.StatusOK, map[string]any{"ok": true, "revoked": revoked})
}

func (s *Server) knowledgeSync(w http.ResponseWriter, r *http.Request) {
	device, err := s.authenticateDevice(r)
	if err != nil || device == nil {
		webutil.JSON(w, http.StatusUnauthorized, map[string]any{"error": "device_auth_failed"})
		return
	}
	if !projectBrainCloudSyncEnabled(device.UserID, device.DeviceID) {
		webutil.JSON(w, http.StatusOK, cloud.KnowledgeManifestSyncResult{Disabled: true, ActiveRevisions: map[string]string{}})
		return
	}
	var input struct {
		WorkspaceID     string                     `json:"workspaceId"`
		ProjectIdentity projectidentity.Snapshot   `json:"projectIdentity"`
		Delta           projectbrain.ManifestDelta `json:"delta"`
	}
	if webutil.DecodeJSON(r, 320<<10, &input) != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	if !regexp.MustCompile(`^[A-Za-z0-9._-]{1,80}$`).MatchString(input.WorkspaceID) {
		webutil.JSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_workspace"})
		return
	}
	binding, err := s.Store.ResolveWorkspaceProject(r.Context(), device.UserID, device.DeviceID, input.WorkspaceID, input.ProjectIdentity)
	if err != nil || binding.ProjectID == "" {
		slog.Warn("project brain binding refresh failed; background sync will retry", "workspaceId", input.WorkspaceID, "error", err)
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]any{"error": "knowledge_binding_unavailable"})
		return
	}
	canonicalRepositories := cloud.CanonicalizeProjectRepositories(input.ProjectIdentity.Repositories, binding.RepositoryIDMap)
	result, err := s.Store.SyncKnowledgeDelta(r.Context(), device.UserID, device.DeviceID, input.WorkspaceID, binding.ProjectID, canonicalRepositories, input.Delta)
	if err != nil {
		slog.Warn("project brain delta sync failed; runtime remains usable", "workspaceId", input.WorkspaceID, "projectId", binding.ProjectID, "error", err)
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]any{"error": "knowledge_sync_failed"})
		return
	}
	webutil.JSON(w, http.StatusOK, result)
}

func (s *Server) workspaceSync(w http.ResponseWriter, r *http.Request) {
	device, err := s.authenticateDevice(r)
	if err != nil || device == nil {
		webutil.JSON(w, http.StatusUnauthorized, map[string]any{"error": "device_auth_failed"})
		return
	}
	brainEnabled := projectBrainCloudSyncEnabled(device.UserID, device.DeviceID)
	var input struct {
		ClientVersion string `json:"clientVersion"`
		Workspaces    []struct {
			WorkspaceID       string                       `json:"workspaceId"`
			WorkspaceName     string                       `json:"workspaceName"`
			ProjectIdentity   projectidentity.Snapshot     `json:"projectIdentity"`
			LearnedSkills     []cloud.LearnedSkillMetadata `json:"learnedSkills"`
			KnowledgeManifest *projectbrain.Manifest       `json:"knowledgeManifest,omitempty"`
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
	knowledgeResults := map[string]cloud.KnowledgeManifestSyncResult{}
	portableSkillResults := map[string][]learnedskills.PortableRecipe{}
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
			if err := s.Store.UpsertWorkspace(r.Context(), cloud.Workspace{
				UserID:          device.UserID,
				DeviceID:        device.DeviceID,
				WorkspaceID:     item.WorkspaceID,
				WorkspaceName:   name,
				ProtocolVersion: protocol.Version,
				Capabilities:    caps,
			}); err != nil {
				slog.Warn("workspace sync persistence failed", "workspaceId", item.WorkspaceID, "error", err)
				continue
			}
		}
		binding, bindingErr := s.Store.ResolveWorkspaceProject(r.Context(), device.UserID, device.DeviceID, item.WorkspaceID, item.ProjectIdentity)
		if bindingErr != nil {
			slog.Warn("workspace project identity resolution failed; workspace remains usable", "workspaceId", item.WorkspaceID, "error", bindingErr)
		}
		if brainEnabled && bindingErr == nil && binding.ProjectID != "" && item.KnowledgeManifest != nil {
			canonicalRepositories := cloud.CanonicalizeProjectRepositories(item.ProjectIdentity.Repositories, binding.RepositoryIDMap)
			result, knowledgeErr := s.Store.SyncKnowledgeManifest(r.Context(), device.UserID, device.DeviceID, item.WorkspaceID, binding.ProjectID, canonicalRepositories, *item.KnowledgeManifest)
			if knowledgeErr != nil {
				// Knowledge is an enrichment layer. Legacy clients may still attach a
				// full manifest to workspace sync, but a transient Project Brain/DB
				// failure must never make the runtime itself unavailable.
				slog.Warn("project knowledge manifest sync failed; workspace remains usable", "workspaceId", item.WorkspaceID, "projectId", binding.ProjectID, "error", knowledgeErr)
			} else {
				knowledgeResults[item.WorkspaceID] = result
			}
		}
		projectID := ""
		if bindingErr == nil {
			projectID = binding.ProjectID
		}
		if err := s.Store.SyncLearnedSkillMetadata(r.Context(), device.UserID, device.DeviceID, item.WorkspaceID, projectID, item.LearnedSkills); err != nil {
			slog.Warn("learned skill metadata sync failed; local skills remain authoritative", "workspaceId", item.WorkspaceID, "error", err)
		}
		if projectID != "" {
			portable, portableErr := s.Store.ProjectPortableLearnedSkills(r.Context(), device.UserID, projectID, 32)
			if portableErr != nil {
				slog.Warn("portable learned skill lookup failed; workspace remains usable", "workspaceId", item.WorkspaceID, "projectId", projectID, "error", portableErr)
			} else if len(portable) > 0 {
				portableSkillResults[item.WorkspaceID] = portable
			}
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
	webutil.JSON(w, http.StatusOK, map[string]any{
		"synced": synced, "removed": len(removed), "knowledge": knowledgeResults, "portableSkills": portableSkillResults, "syncedAt": time.Now().UnixMilli(),
		"projectBrain": map[string]any{"cloudSyncEnabled": brainEnabled},
	})
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
