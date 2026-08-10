package cloudserver

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/ui"
)

const securityPageSize = 25

// RegisterDashboardExtras restores the polished dashboard-only surfaces that
// intentionally keep cloud data metadata-only. Source code, terminal output,
// command text and local secrets never enter these pages.
func (s *Server) RegisterDashboardExtras() {
	s.Mux.Handle("GET /dashboard/security", s.WebAuth.Require(http.HandlerFunc(s.securityDashboard)))
	s.Mux.Handle("GET /dashboard/connect", s.WebAuth.Require(http.HandlerFunc(s.connectDashboard)))
}

func dashboardIdentitySource(email string) string {
	return `<div data-dashboard-email="` + ui.Escape(email) + `"></div>`
}

func (s *Server) connectDashboard(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.identity(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	endpoint := strings.TrimRight(s.WebAuth.PublicBaseURL, "/") + "/mcp"
	body := dashboardIdentitySource(identity.User.Email) + dashboardNav(identity) + `
<div class="grid">
  <section class="card glow span12">
    <div class="section-head"><div><div class="section-title">Connect ChatGPT</div><div class="section-label">ChatGPT stays the AI brain. CodeLocal securely routes approved tool calls to this machine.</div></div><span class="badge green">Ready</span></div>
    <div class="row"><div class="row-title">MCP endpoint</div><div class="row-meta mono">` + ui.Escape(endpoint) + `</div></div>
  </section>
  <section class="card span6">
    <div class="section-head"><div><div class="section-title">1 · Install CodeLocal</div><div class="section-label">Install the current beta runtime once on your computer.</div></div></div>
    <div class="row"><div class="row-title mono">npm install -g codelocal@beta</div></div>
  </section>
  <section class="card span6">
    <div class="section-head"><div><div class="section-title">2 · Authorize folders</div><div class="section-label">Grant only projects you want ChatGPT to access.</div></div></div>
    <div class="row"><div class="row-title mono">cd /path/to/project<br>codelocal .</div></div>
  </section>
  <section class="card span12">
    <div class="section-head"><div><div class="section-title">3 · Start one machine runtime</div><div class="section-label">One runtime can serve every folder you previously authorized. You do not need one process per workspace.</div></div></div>
    <div class="row"><div class="row-title mono">codelocal</div><div class="row-meta">Then refresh the CodeLocal app in ChatGPT and choose the workspace you want to use.</div></div>
  </section>
</div>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(ui.Page("Connect ChatGPT · CodeLocal Cloud", "Connect ChatGPT to your local projects without a separate OpenAI API key.", body)))
}

func (s *Server) securityDashboard(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.identity(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	page := 1
	if raw := strings.TrimSpace(r.URL.Query().Get("page")); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			page = value
		}
	}
	offset := (page - 1) * securityPageSize

	var total int
	if err := s.Store.DB.QueryRow(r.Context(), `SELECT COUNT(*) FROM codelocal_audit_logs WHERE user_id=$1 AND event='terminal.executed'`, identity.User.ID).Scan(&total); err != nil {
		http.Error(w, "Unable to load security activity", http.StatusInternalServerError)
		return
	}
	rows, err := s.Store.DB.Query(r.Context(), `
SELECT event,COALESCE(device_id,''),COALESCE(workspace_id,''),created_at
FROM codelocal_audit_logs
WHERE user_id=$1 AND event='terminal.executed'
ORDER BY created_at DESC
LIMIT $2 OFFSET $3`, identity.User.ID, securityPageSize, offset)
	if err != nil {
		http.Error(w, "Unable to load security activity", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var body strings.Builder
	body.WriteString(dashboardIdentitySource(identity.User.Email) + dashboardNav(identity))
	body.WriteString(`<div class="grid"><section class="card glow span12"><div class="section-head"><div><div class="section-title">Terminal activity</div><div class="section-label">Only execution metadata is stored in CodeLocal Cloud. Source code, MCP reads, terminal output, command text and local secrets stay off the server.</div></div><span class="badge blue">Metadata only</span></div><div class="stack">`)
	count := 0
	for rows.Next() {
		var event, deviceID, workspaceID string
		var createdAt int64
		if err := rows.Scan(&event, &deviceID, &workspaceID, &createdAt); err != nil {
			continue
		}
		count++
		meta := []string{time.UnixMilli(createdAt).Format("Jan 2, 2006, 3:04 PM")}
		if deviceID != "" {
			meta = append(meta, deviceID)
		}
		if workspaceID != "" {
			meta = append(meta, workspaceID)
		}
		body.WriteString(`<div class="row"><div class="row-title">Terminal command executed <span class="badge green">Executed</span></div><div class="row-meta mono">` + ui.Escape(strings.Join(meta, " · ")) + `</div></div>`)
	}
	if count == 0 {
		body.WriteString(`<div class="empty">No terminal execution metadata on this page yet.</div>`)
	}
	body.WriteString(`</div></section></div>`)

	pages := (total + securityPageSize - 1) / securityPageSize
	if pages < 1 {
		pages = 1
	}
	body.WriteString(`<div style="height:16px"></div><div class="actions">`)
	if page > 1 {
		body.WriteString(`<a class="btn" href="/dashboard/security?page=` + strconv.Itoa(page-1) + `">Previous</a>`)
	}
	body.WriteString(`<span class="btn" style="pointer-events:none">Page ` + strconv.Itoa(page) + ` of ` + strconv.Itoa(pages) + ` · ` + fmt.Sprintf("%d", total) + ` events</span>`)
	if page < pages {
		body.WriteString(`<a class="btn" href="/dashboard/security?page=` + strconv.Itoa(page+1) + `">Next</a>`)
	}
	body.WriteString(`</div>`)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(ui.Page("Security · CodeLocal Cloud", "Review terminal execution metadata without storing your code or terminal contents.", body.String())))
}
