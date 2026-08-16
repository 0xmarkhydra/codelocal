package cloudserver

import (
	"net/http"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/ui"
)

// RegisterDashboardExtras keeps the legacy connect surface available behind
// authenticated dashboard routing. Security/audit history is intentionally not
// exposed in the cloud dashboard.
func (s *Server) RegisterDashboardExtras() {
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
    <div class="section-head"><div><div class="section-title">MCP Connections</div><div class="section-label">Connect compatible AI clients through one CodeLocal MCP layer while local execution stays permission-controlled.</div></div><span class="badge green">Ready</span></div>
    <div class="row"><div class="row-title">MCP endpoint</div><div class="row-meta mono">` + ui.Escape(endpoint) + `</div></div>
  </section>
  <section class="card span6">
    <div class="section-head"><div><div class="section-title">1 · Install CodeLocal</div><div class="section-label">Install the current beta runtime once on your computer.</div></div></div>
    <div class="row"><div class="row-title mono">npm install -g codelocal</div></div>
  </section>
  <section class="card span6">
    <div class="section-head"><div><div class="section-title">2 · Authorize folders</div><div class="section-label">Grant only projects you want connected MCP clients to access.</div></div></div>
    <div class="row"><div class="row-title mono">cd /path/to/project<br>codelocal .</div></div>
  </section>
  <section class="card span12">
    <div class="section-head"><div><div class="section-title">3 · Start one machine runtime</div><div class="section-label">One runtime can serve every folder you previously authorized. You do not need one process per workspace.</div></div></div>
    <div class="row"><div class="row-title mono">codelocal</div><div class="row-meta">Then refresh CodeLocal in your MCP client and choose the workspace you want to use for that session.</div></div>
  </section>
</div>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(ui.Page("MCP Connections · CodeLocal Cloud", "Connect MCP-compatible AI clients to your authorized local projects through one CodeLocal runtime.", body)))
}
