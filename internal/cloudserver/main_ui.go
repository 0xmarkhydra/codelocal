package cloudserver

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/ui"
	"github.com/0xmarkhydra/codelocal/internal/webauth"
)

// MainUIHandler ports the completed Node control-plane presentation onto the
// Go cloud server. Backend/runtime behavior stays Go; the visual structure and
// product language intentionally follow main/src/web-ui.ts + main/src/dashboard.ts.
func (s *Server) MainUIHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/" {
			s.mainLanding(w, r)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/dashboard") {
			next.ServeHTTP(w, r)
			return
		}
		identity, _ := s.WebAuth.Identity(r)
		if identity == nil {
			http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
			return
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/dashboard":
			s.mainOverview(w, r, identity)
		case r.Method == http.MethodGet && r.URL.Path == "/dashboard/devices":
			s.mainDevices(w, r, identity)
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/dashboard/devices/") && strings.HasSuffix(r.URL.Path, "/revoke"):
			s.mainRevokeDevice(w, r, identity)
		case r.Method == http.MethodGet && r.URL.Path == "/dashboard/workspaces":
			s.mainWorkspaces(w, r, identity)
		case r.Method == http.MethodGet && r.URL.Path == "/dashboard/knowledge":
			s.knowledgeDashboard(w, r, identity)
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/dashboard/workspaces/") && strings.HasSuffix(r.URL.Path, "/remove"):
			s.mainRemoveWorkspace(w, r, identity)
		case r.Method == http.MethodGet && r.URL.Path == "/dashboard/usage":
			s.mainUsage(w, r, identity)
		case r.Method == http.MethodGet && r.URL.Path == "/dashboard/connect":
			s.mainConnect(w, r, identity)
		case r.Method == http.MethodGet && r.URL.Path == "/dashboard/invite":
			s.mainInvite(w, r, identity)
		case r.Method == http.MethodGet && r.URL.Path == "/dashboard/leaderboard":
			s.mainLeaderboard(w, r, identity)
		case r.Method == http.MethodGet && r.URL.Path == "/dashboard/admin":
			s.adminDashboard(w, r, identity)
		default:
			next.ServeHTTP(w, r)
		}
	})
}

func writeHTML(w http.ResponseWriter, html string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

func (s *Server) mainLanding(w http.ResponseWriter, r *http.Request) {
	identity, _ := s.WebAuth.Identity(r)
	writeHTML(w, ui.LandingPage(ui.LandingOptions{
		SignedIn: identity != nil,
		Endpoint: strings.TrimRight(s.WebAuth.PublicBaseURL, "/") + "/mcp",
	}))
}

func mainFlash(r *http.Request) string {
	if value := strings.TrimSpace(r.URL.Query().Get("ok")); value != "" {
		return `<div class="alert success">` + ui.Escape(value) + `</div>`
	}
	if value := strings.TrimSpace(r.URL.Query().Get("error")); value != "" {
		return `<div class="alert">` + ui.Escape(value) + `</div>`
	}
	return ""
}

func mainWorkspaceState(status string) (label, badge string) {
	switch status {
	case "active":
		return "Active", "green"
	case "sleeping":
		return "Sleeping", "blue"
	default:
		return "Device offline", "muted"
	}
}

const mainPageSize = 12

func mainQuery(r *http.Request) string {
	return strings.TrimSpace(r.URL.Query().Get("q"))
}

func mainPageBounds(r *http.Request, total int) (page, start, end, totalPages int) {
	page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	totalPages = (total + mainPageSize - 1) / mainPageSize
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	start = (page - 1) * mainPageSize
	if start > total {
		start = total
	}
	end = start + mainPageSize
	if end > total {
		end = total
	}
	return
}

func mainListToolbar(path, query string, total int) string {
	clear := ""
	if query != "" {
		clear = `<a class="btn small" href="` + path + `">Clear</a>`
	}
	placeholder := "Search name or ID"
	if path == "/dashboard/admin" {
		placeholder = "Search email or referral code"
	}
	return `<div class="list-toolbar"><form class="search-form" method="get" action="` + path + `"><input class="search-input" type="search" name="q" value="` + ui.Escape(query) + `" placeholder="` + placeholder + `"><button class="btn small" type="submit">Search</button>` + clear + `</form><span class="badge blue">` + strconv.Itoa(total) + ` result(s)</span></div>`
}

func mainPager(path, query string, page, totalPages int) string {
	if totalPages <= 1 {
		return ""
	}
	pageURL := func(value int) string {
		params := url.Values{}
		if query != "" {
			params.Set("q", query)
		}
		params.Set("page", strconv.Itoa(value))
		return path + "?" + params.Encode()
	}
	prev := `<span class="btn small disabled">Previous</span>`
	if page > 1 {
		prev = `<a class="btn small" href="` + pageURL(page-1) + `">Previous</a>`
	}
	next := `<span class="btn small disabled">Next</span>`
	if page < totalPages {
		next = `<a class="btn small" href="` + pageURL(page+1) + `">Next</a>`
	}
	return `<div class="pager">` + prev + `<span class="pager-meta">Page ` + strconv.Itoa(page) + ` of ` + strconv.Itoa(totalPages) + `</span>` + next + `</div>`
}

func (s *Server) mainOverview(w http.ResponseWriter, r *http.Request, identity *webauth.Identity) {
	devices, _ := s.Store.ListDevices(r.Context(), identity.User.ID)
	workspaces, _ := s.Workspaces.Catalog(r.Context(), identity.User.ID)
	usage24h, _ := s.Store.MCPUsageSummary(r.Context(), identity.User.ID, time.Now().Add(-24*time.Hour).UnixMilli())
	usage30d, _ := s.Store.MCPUsageSummary(r.Context(), identity.User.ID, time.Now().Add(-30*24*time.Hour).UnixMilli())
	usageAll, _ := s.Store.MCPUsageSummary(r.Context(), identity.User.ID, 0)

	paired := 0
	onlineDevices := 0
	for _, device := range devices {
		if device.RevokedAt != 0 {
			continue
		}
		paired++
		if online, _ := s.Activation.IsOnline(r.Context(), identity.User.ID, device.DeviceID); online {
			onlineDevices++
		}
	}
	activeWorkspaces := 0
	sleepingWorkspaces := 0
	for _, workspace := range workspaces {
		if workspace.Status == "active" {
			activeWorkspaces++
		} else if workspace.Status == "sleeping" {
			sleepingWorkspaces++
		}
	}

	var workspaceRows strings.Builder
	for i, workspace := range workspaces {
		if i >= 5 {
			break
		}
		label, badge := mainWorkspaceState(workspace.Status)
		folder := "folder"
		if workspace.Status == "active" {
			folder = "folderCheck"
		}
		workspaceRows.WriteString(`<div class="row"><div class="entity"><div class="entity-icon">` + ui.Icon(folder) + `</div><div class="entity-copy"><div class="row-title"><span class="row-title-text">` + ui.Escape(workspace.WorkspaceName) + `</span></div><div class="row-meta">` + ui.Escape(workspace.DeviceName) + ` · ` + ui.Escape(ui.FormatTime(workspace.LastSeenAt)) + `</div></div></div><span class="badge ` + badge + `">` + label + `</span></div>`)
	}
	if workspaceRows.Len() == 0 {
		workspaceRows.WriteString(`<div class="empty"><div class="empty-icon">` + ui.Icon("folder") + `</div>No workspace yet.<br><span class="muted">Run <code>codelocal .</code> once inside a project.</span></div>`)
	}

	heroTitle := "Your CodeLocal environment is ready."
	heroPill := `<span class="status-dot green"></span>Runtime connected`
	heroCopy := fmt.Sprintf("%d of %d paired machine runtime(s) online · %d active workspace(s) · %d sleeping workspace(s).", onlineDevices, paired, activeWorkspaces, sleepingWorkspaces)
	if paired == 0 {
		heroTitle = "Pair your first machine to get started."
		heroPill = `<span class="status-dot"></span>Setup required`
		heroCopy = "Install CodeLocal, pair this computer, then authorize only the project folders you want MCP-compatible AI clients to use."
	} else if onlineDevices == 0 {
		heroTitle = "Your projects are ready when your runtime is."
		heroPill = `<span class="status-dot"></span>Runtime offline`
		heroCopy = fmt.Sprintf("%d paired machine(s) · %d authorized workspace(s). Start codelocal on a paired machine to make them available to connected MCP clients.", paired, len(workspaces))
	}

	body := `<div class="grid">` +
		`<div class="card span12 overview-hero"><div class="overview-hero-copy"><div class="hero-pill">` + heroPill + `</div><div class="hero-title">` + ui.Escape(heroTitle) + `</div><div class="hero-copy">` + ui.Escape(heroCopy) + `</div><div class="hero-actions"><a class="btn primary" href="/dashboard/connect">Connect an AI client <span class="btn-icon-svg" aria-hidden="true">` + ui.Icon("arrowRight") + `</span></a><a class="btn" href="/dashboard/workspaces">View workspaces</a></div></div><div class="hero-flow" aria-hidden="true"><div class="flow-node flow-chatgpt">` + ui.Icon("sparkles") + `</div><div class="flow-connector"><span></span><div class="flow-link-icon">` + ui.Icon("connect") + `</div></div><div class="flow-node flow-codelocal"><img src="/assets/codelocal-icon.png" alt=""></div><div class="flow-connector"><span></span><div class="flow-link-icon">` + ui.Icon("arrowRight") + `</div></div><div class="flow-node flow-device">` + ui.Icon("device") + `</div><div class="flow-labels"><span>AI clients</span><span>CodeLocal</span><span>Your machine</span></div></div></div>` +
		ui.MetricCard("Machine runtimes", onlineDevices, fmt.Sprintf("%d paired device(s)", paired)) +
		ui.MetricCard("Active workspaces", activeWorkspaces, "Loaded for an MCP session") +
		ui.MetricCard("Sleeping workspaces", sleepingWorkspaces, "Authorized, zero heavy runtime") +
		`<div class="card span12 usage-note" id="token-usage"><div class="section-head"><div><div class="section-kicker">Usage</div><div class="title">Token usage</div><div class="label">Estimated MCP payload passing through CodeLocal — not OpenAI billing or full conversation tokens.</div></div><span class="badge blue">Reference value</span></div><div class="divider"></div><div class="grid">` + mainUsageMetric("Last 24 hours", usage24h) + mainUsageMetric("Last 30 days", usage30d) + mainUsageMetric("All time", usageAll) + `</div><div class="divider"></div><div class="label">USD is an illustrative 50/50 input-output reference using GPT-5.6 Sol API rates to make usage scale easier to understand. It is not an OpenAI charge and CodeLocal does not bill per token. Rolling 24-hour and 30-day counters expire automatically.</div></div>` +
		`<div class="card span12"><div class="section-head"><div><div class="title">Recent workspaces</div><div class="label">Only folders granted by you are visible here.</div></div><a class="btn small" href="/dashboard/workspaces">View all</a></div><div class="divider"></div><div class="list">` + workspaceRows.String() + `</div></div></div>`

	writeHTML(w, ui.DashboardPage(ui.DashboardOptions{
		Title: "Overview", Active: "overview", Email: identity.User.Email, CSRF: identity.CSRF,
		Subtitle: "A private control plane for the local machines and project folders you explicitly authorize.",
		Body:     body, IsAdmin: cloud.IsAdminEmail(identity.User.Email),
	}))
}

func (s *Server) mainDevices(w http.ResponseWriter, r *http.Request, identity *webauth.Identity) {
	devices, _ := s.Store.ListDevices(r.Context(), identity.User.ID)
	onlineByDevice := make(map[string]bool, len(devices))
	pairedCount := 0
	onlineCount := 0
	revokedCount := 0
	for _, device := range devices {
		if device.RevokedAt != 0 {
			revokedCount++
			continue
		}
		pairedCount++
		online, _ := s.Activation.IsOnline(r.Context(), identity.User.ID, device.DeviceID)
		onlineByDevice[device.DeviceID] = online
		if online {
			onlineCount++
		}
	}
	query := strings.ToLower(mainQuery(r))
	filtered := make([]cloud.Device, 0, len(devices))
	for _, device := range devices {
		haystack := strings.ToLower(device.DeviceName + " " + device.DeviceID)
		if query == "" || strings.Contains(haystack, query) {
			filtered = append(filtered, device)
		}
	}
	page, start, end, totalPages := mainPageBounds(r, len(filtered))
	var rows strings.Builder
	for _, device := range filtered[start:end] {
		online := onlineByDevice[device.DeviceID]
		status, badge := "Offline", "muted"
		if device.RevokedAt != 0 {
			status, badge = "Revoked", "red"
		} else if online {
			status, badge = "Online", "green"
		}
		actions := `<span class="badge ` + badge + `">` + status + `</span>`
		if device.RevokedAt == 0 {
			actions += `<details class="action-menu"><summary aria-label="Device actions"><span class="action-menu-icon">` + ui.Icon("more") + `</span></summary><div class="action-menu-panel"><form method="post" action="/dashboard/devices/` + url.PathEscape(device.CredentialID) + `/revoke">` + ui.Hidden(map[string]string{"csrf": identity.CSRF}) + `<button class="action-menu-item danger" type="submit" data-confirm data-confirm-title="Revoke this device?" data-confirm-label="Revoke device" data-confirm-message="This immediately disconnects the machine and prevents its credential from accessing CodeLocal Cloud. Local project files are not deleted.">Revoke device</button></form></div></details>`
		}
		rows.WriteString(`<div class="row"><div class="entity"><div class="entity-icon">` + ui.Icon("device") + `</div><div class="entity-copy"><div class="row-title"><span class="row-title-text">` + ui.Escape(device.DeviceName) + `</span></div><div class="row-meta">Paired ` + ui.Escape(ui.FormatTime(device.CreatedAt)) + ` · last seen ` + ui.Escape(ui.FormatTime(device.LastSeenAt)) + `</div></div></div><div class="actions">` + actions + `</div></div>`)
	}
	if rows.Len() == 0 {
		message := "No paired devices yet."
		if query != "" {
			message = "No devices match your search."
		}
		rows.WriteString(`<div class="empty"><div class="empty-icon">` + ui.Icon("device") + `</div>` + message + `</div>`)
	}
	originalQuery := mainQuery(r)
	body := mainFlash(r) + `<div class="grid">` +
		ui.MetricCard("Paired devices", pairedCount, "Trusted machine credentials") +
		ui.MetricCard("Online now", onlineCount, "Machine runtime reachable") +
		ui.MetricCard("Revoked", revokedCount, "Credentials no longer allowed") +
		`<div class="card span12"><div class="section-head"><div><div class="title">Paired machines</div><div class="label">Online means the lightweight <code>codelocal</code> runtime is reachable now.</div></div></div>` + mainListToolbar("/dashboard/devices", originalQuery, len(filtered)) + `<div class="divider"></div><div class="list">` + rows.String() + `</div>` + mainPager("/dashboard/devices", originalQuery, page, totalPages) + `</div></div>`
	writeHTML(w, ui.DashboardPage(ui.DashboardOptions{Title: "Devices", Active: "devices", Email: identity.User.Email, CSRF: identity.CSRF, Subtitle: "A paired device credential lets one CodeLocal machine runtime connect to your account. Revoke anything you no longer trust.", Body: body, IsAdmin: cloud.IsAdminEmail(identity.User.Email)}))
}

func (s *Server) mainWorkspaces(w http.ResponseWriter, r *http.Request, identity *webauth.Identity) {
	workspaces, _ := s.Workspaces.Catalog(r.Context(), identity.User.ID)
	activeCount := 0
	sleepingCount := 0
	offlineCount := 0
	for _, workspace := range workspaces {
		switch workspace.Status {
		case "active":
			activeCount++
		case "sleeping":
			sleepingCount++
		default:
			offlineCount++
		}
	}
	query := strings.ToLower(mainQuery(r))
	filtered := workspaces[:0]
	for _, workspace := range workspaces {
		haystack := strings.ToLower(workspace.WorkspaceName + " " + workspace.WorkspaceID + " " + workspace.DeviceName + " " + workspace.DeviceID)
		if query == "" || strings.Contains(haystack, query) {
			filtered = append(filtered, workspace)
		}
	}
	page, start, end, totalPages := mainPageBounds(r, len(filtered))
	var rows strings.Builder
	for _, workspace := range filtered[start:end] {
		label, badge := mainWorkspaceState(workspace.Status)
		folder := "folder"
		if workspace.Status == "active" {
			folder = "folderCheck"
		}
		remove := `<button class="action-menu-item" type="button" disabled>Remove access</button><div class="action-menu-hint">Start CodeLocal on this device before removing local authorization.</div>`
		if workspace.RuntimeOnline {
			remove = `<form method="post" action="/dashboard/workspaces/` + url.PathEscape(workspace.DeviceID) + `/` + url.PathEscape(workspace.WorkspaceID) + `/remove">` + ui.Hidden(map[string]string{"csrf": identity.CSRF}) + `<button class="action-menu-item danger" type="submit" data-confirm data-confirm-title="Remove workspace access?" data-confirm-label="Remove access" data-confirm-message="CodeLocal will revoke this folder from the local machine. The project and every file inside it stay untouched. You can authorize it again later with codelocal .">Remove access</button></form>`
		}
		menu := `<details class="action-menu"><summary aria-label="Workspace actions"><span class="action-menu-icon">` + ui.Icon("more") + `</span></summary><div class="action-menu-panel">` + remove + `</div></details>`
		rows.WriteString(`<div class="row"><div class="entity"><div class="entity-icon">` + ui.Icon(folder) + `</div><div class="entity-copy"><div class="row-title"><span class="row-title-text">` + ui.Escape(workspace.WorkspaceName) + `</span></div><div class="row-meta">Device ` + ui.Escape(workspace.DeviceName) + ` · last seen ` + ui.Escape(ui.FormatTime(workspace.LastSeenAt)) + `</div></div></div><div class="actions"><span class="badge ` + badge + `">` + label + `</span>` + menu + `</div></div>`)
	}
	if rows.Len() == 0 {
		message := `No authorized workspace has synced yet.<br><span class="muted">Open a project on your machine and run <code>codelocal .</code> once.</span>`
		if query != "" {
			message = "No workspaces match your search."
		}
		rows.WriteString(`<div class="empty"><div class="empty-icon">` + ui.Icon("folder") + `</div>` + message + `</div>`)
	}
	originalQuery := mainQuery(r)
	body := mainFlash(r) + `<div class="grid">` +
		ui.MetricCard("Authorized", len(workspaces), "Project folders you explicitly granted") +
		ui.MetricCard("Active", activeCount, "Loaded for an MCP session") +
		ui.MetricCard("Sleeping", sleepingCount, fmt.Sprintf("%d device-offline workspace(s)", offlineCount)) +
		`<div class="card span12"><div class="section-head"><div><div class="title">Authorized folders</div><div class="label">Remove access without deleting or modifying the project folder itself.</div></div><div class="badge blue">` + strconv.Itoa(len(workspaces)) + ` authorized</div></div>` + mainListToolbar("/dashboard/workspaces", originalQuery, len(filtered)) + `<div class="divider"></div><div class="list">` + rows.String() + `</div>` + mainPager("/dashboard/workspaces", originalQuery, page, totalPages) + `</div></div>`
	writeHTML(w, ui.DashboardPage(ui.DashboardOptions{Title: "Workspaces", Active: "workspaces", Email: identity.User.Email, CSRF: identity.CSRF, Subtitle: "A workspace is a local project folder you granted once. Sleeping workspaces consume no heavy project runtime until an MCP client selects them.", Body: body, IsAdmin: cloud.IsAdminEmail(identity.User.Email)}))
}

func (s *Server) mainUsage(w http.ResponseWriter, r *http.Request, identity *webauth.Identity) {
	http.Redirect(w, r, "/dashboard#token-usage", http.StatusFound)
}

func (s *Server) mainInvite(w http.ResponseWriter, r *http.Request, identity *webauth.Identity) {
	users, err := s.Store.ListInvitedUsers(r.Context(), identity.User.ReferralCode)
	loadError := err != nil
	direct := make([]adminUserState, 0, len(users))
	ids := make([]string, 0, len(users))
	for _, user := range users {
		direct = append(direct, adminUserState{AdminUser: user})
		ids = append(ids, user.ID)
	}
	runtimeActive, _ := s.Activation.UserOnlineMap(r.Context(), ids)
	mcpActive, _ := s.Store.UserMCPActiveMap(r.Context(), ids)
	activeCount := 0
	var rows strings.Builder
	for i := range direct {
		direct[i].RuntimeActive = runtimeActive[direct[i].ID]
		direct[i].MCPActive = mcpActive[direct[i].ID]
		if direct[i].RuntimeActive || direct[i].MCPActive {
			activeCount++
		}
		rows.WriteString(`<div class="row"><div class="entity"><div class="invite-avatar" aria-hidden="true">` + ui.Escape(mainLeaderboardInitial(direct[i].Email)) + `</div><div class="entity-copy"><div class="row-title"><span class="row-title-text">` + ui.Escape(maskLeaderboardEmail(direct[i].Email)) + `</span></div><div class="row-meta">Joined ` + ui.Escape(ui.FormatTime(direct[i].CreatedAt)) + `</div></div></div>` + adminStatus(direct[i]) + `</div>`)
	}
	if rows.Len() == 0 {
		if loadError {
			rows.WriteString(`<div class="empty">Unable to load invited members right now.</div>`)
		} else {
			rows.WriteString(`<div class="empty">No one has joined with your invite code yet.</div>`)
		}
	}

	invitedBy := identity.User.ReferredByCode
	if invitedBy == "" {
		invitedBy = "Root account"
	}
	inviteLink := strings.TrimRight(s.WebAuth.PublicBaseURL, "/") + "/register?ref=" + url.QueryEscape(identity.User.ReferralCode)
	body := `<div class="grid">` +
		`<div class="card span12 invite-card"><div class="invite-card-copy"><div class="invite-mark" aria-hidden="true">` + ui.Icon("userPlus") + `</div><div><div class="section-kicker">Your invite</div><div class="title">Invite code</div><div class="label">Share this six-character code with people you trust. Invited by: ` + ui.Escape(invitedBy) + `.</div></div></div><div class="invite-code-wrap"><div class="invite-code mono" id="referral-code">` + ui.Escape(identity.User.ReferralCode) + `</div><button class="btn primary" type="button" data-copy-target="#referral-code"><span class="btn-icon-svg" aria-hidden="true">` + ui.Icon("copy") + `</span>Copy code</button></div></div>` +
		ui.MetricCard("Direct invites", len(direct), "Accounts created with your code") +
		ui.MetricCard("Active invites", activeCount, "Runtime or MCP currently active") +
		`<div class="card span4"><div class="metric-label">Invite link</div><div class="metric-state">Ready to share</div><div class="metric-sub">Includes your referral code automatically.</div></div>` +
		`<div class="card span12"><div class="section-head"><div><div class="title">Invite link</div><div class="label">Anyone opening this link will have your code prefilled at registration.</div></div></div><div class="copy-row"><div class="code-block" id="invite-link">` + ui.Escape(inviteLink) + `</div><button class="btn" type="button" data-copy-target="#invite-link"><span class="btn-icon-svg" aria-hidden="true">` + ui.Icon("copy") + `</span>Copy link</button></div></div>` +
		`<div class="card span12"><div class="section-head"><div><div class="title">People you invited</div><div class="label">Direct referrals only. Emails are masked for privacy.</div></div><span class="badge blue">` + strconv.Itoa(len(direct)) + ` direct</span></div><div class="divider"></div><div class="list">` + rows.String() + `</div></div></div>`
	writeHTML(w, ui.DashboardPage(ui.DashboardOptions{Title: "Invite", Active: "invite", Email: identity.User.Email, CSRF: identity.CSRF, Subtitle: "Manage your invite code and see the people who joined through you.", Body: body, IsAdmin: cloud.IsAdminEmail(identity.User.Email)}))
}

func (s *Server) mainLeaderboard(w http.ResponseWriter, r *http.Request, identity *webauth.Identity) {
	body := `<div class="grid">` + s.mainUsageLeaderboard(r.Context(), identity.User.Email) + `</div>`
	writeHTML(w, ui.DashboardPage(ui.DashboardOptions{Title: "Leaderboard", Active: "leaderboard", Email: identity.User.Email, CSRF: identity.CSRF, Subtitle: "Top CodeLocal users by estimated MCP payload over the last 30 days.", Body: body, IsAdmin: cloud.IsAdminEmail(identity.User.Email)}))
}

func (s *Server) mainConnect(w http.ResponseWriter, r *http.Request, identity *webauth.Identity) {
	endpoint := strings.TrimRight(s.WebAuth.PublicBaseURL, "/") + "/mcp"
	body := `<div class="grid"><div class="card span8 connect-hero"><div class="connect-visual" aria-hidden="true"><div class="connect-app connect-chatgpt">` + ui.Icon("sparkles") + `</div><div class="connect-bridge"><span class="connect-beam"></span><span class="connect-link-icon">` + ui.Icon("connect") + `</span></div><div class="connect-app connect-codelocal"><img src="/assets/codelocal-icon.png" alt=""></div></div><div class="section-head"><div><div class="section-kicker">Secure MCP connection</div><div class="title">MCP server URL</div><div class="label">Add this endpoint to any compatible remote MCP client and use OAuth authentication. CodeLocal discovers and serves the OAuth endpoints automatically.</div></div><span class="badge green">OAuth</span></div><div class="divider"></div><div class="copy-row"><div class="code-block" id="mcp-endpoint">` + ui.Escape(endpoint) + `</div><button class="btn primary" type="button" data-copy-target="#mcp-endpoint"><span class="btn-icon-svg" aria-hidden="true">` + ui.Icon("copy") + `</span>Copy URL</button></div></div><div class="card span4 connect-steps"><div class="title">Connection flow</div><div class="label">Four steps, then this account is ready for compatible MCP clients.</div><div class="divider"></div><div class="kv"><div class="kv-key">1</div><div class="kv-value">Add MCP server</div></div><div class="kv"><div class="kv-key">2</div><div class="kv-value">Choose OAuth</div></div><div class="kv"><div class="kv-key">3</div><div class="kv-value">Sign in to CodeLocal</div></div><div class="kv"><div class="kv-key">4</div><div class="kv-value">Approve connection</div></div></div></div>`
	writeHTML(w, ui.DashboardPage(ui.DashboardOptions{Title: "MCP Connections", Active: "connect", Email: identity.User.Email, CSRF: identity.CSRF, Subtitle: "Connect CodeLocal once through MCP, then use the workspace and Project Brain from any compatible AI client or coding agent.", Actions: `<a class="btn" href="/#setup">Full setup guide</a>`, Body: body, IsAdmin: cloud.IsAdminEmail(identity.User.Email)}))
}

func (s *Server) mainRevokeDevice(w http.ResponseWriter, r *http.Request, identity *webauth.Identity) {
	if !s.WebAuth.VerifyCSRF(r) {
		http.Error(w, "Invalid security token. Reload the page and try again.", http.StatusForbidden)
		return
	}
	raw := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/dashboard/devices/"), "/revoke")
	credentialID, err := url.PathUnescape(strings.Trim(raw, "/"))
	if err != nil || credentialID == "" {
		http.Redirect(w, r, "/dashboard/devices?error=Invalid%20device.", http.StatusSeeOther)
		return
	}
	deviceID := ""
	if devices, listErr := s.Store.ListDevices(r.Context(), identity.User.ID); listErr == nil {
		for _, device := range devices {
			if device.CredentialID == credentialID {
				deviceID = device.DeviceID
				break
			}
		}
	}
	revoked, err := s.Store.RevokeDevice(r.Context(), identity.User.ID, credentialID)
	if err != nil {
		http.Redirect(w, r, "/dashboard/devices?error="+url.QueryEscape("Unable to revoke device."), http.StatusSeeOther)
		return
	}
	if revoked {
		s.disconnectCredentialEverywhere(identity.User.ID, credentialID)
		if deviceID != "" {
			presenceCtx, cancelPresence := context.WithTimeout(context.Background(), 2*time.Second)
			_ = s.Activation.ClearPresence(presenceCtx, identity.User.ID, deviceID)
			cancelPresence()
		}
		s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "device.revoked", DeviceID: deviceID, Detail: map[string]any{"credentialId": credentialID}})
	}
	http.Redirect(w, r, "/dashboard/devices?ok="+url.QueryEscape("Device access revoked."), http.StatusSeeOther)
}

func (s *Server) mainRemoveWorkspace(w http.ResponseWriter, r *http.Request, identity *webauth.Identity) {
	if !s.WebAuth.VerifyCSRF(r) {
		http.Error(w, "Invalid security token. Reload the page and try again.", http.StatusForbidden)
		return
	}
	raw := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/dashboard/workspaces/"), "/remove")
	parts := strings.Split(strings.Trim(raw, "/"), "/")
	if len(parts) != 2 {
		http.Redirect(w, r, "/dashboard/workspaces?error="+url.QueryEscape("Invalid workspace."), http.StatusSeeOther)
		return
	}
	deviceID, err1 := url.PathUnescape(parts[0])
	workspaceID, err2 := url.PathUnescape(parts[1])
	if err1 != nil || err2 != nil || deviceID == "" || workspaceID == "" {
		http.Redirect(w, r, "/dashboard/workspaces?error="+url.QueryEscape("Invalid workspace."), http.StatusSeeOther)
		return
	}
	catalog, _ := s.Workspaces.Catalog(r.Context(), identity.User.ID)
	workspaceName := workspaceID
	for _, workspace := range catalog {
		if workspace.DeviceID == deviceID && workspace.WorkspaceID == workspaceID {
			workspaceName = workspace.WorkspaceName
			break
		}
	}
	if err := s.Workspaces.Revoke(r.Context(), identity.User.ID, deviceID, workspaceID); err != nil {
		http.Redirect(w, r, "/dashboard/workspaces?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "workspace.revocation_requested", DeviceID: deviceID, WorkspaceID: workspaceID, Detail: map[string]any{"workspaceName": workspaceName}})
	http.Redirect(w, r, "/dashboard/workspaces?ok="+url.QueryEscape(workspaceName+" access removed. The project files were not changed."), http.StatusSeeOther)
}
