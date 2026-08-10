package cloudserver

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/ui"
	"github.com/0xmarkhydra/codelocal/internal/webauth"
)

const securityPageSize = 25

// RegisterDashboardExtras restores dashboard surfaces that intentionally stay
// metadata-only in the Go Cloud. Source code, terminal output and secrets never
// enter these pages.
func (s *Server) RegisterDashboardExtras() {
	s.Mux.Handle("GET /dashboard/security", s.WebAuth.Require(http.HandlerFunc(s.securityDashboard)))
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
	body.WriteString(`<div class="row"><div class="row-title">Activity</div><div class="row-meta">Only terminal execution metadata is stored here. Source code, command output, MCP reads and local secrets are not stored in CodeLocal Cloud.</div></div><div style="height:12px"></div><div class="stack">`)
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
		body.WriteString(`<div class="row"><div class="row-title">Terminal command executed</div><div class="row-meta mono">` + ui.Escape(strings.Join(meta, " · ")) + `</div></div>`)
	}
	if count == 0 {
		body.WriteString(`<div class="row"><div class="row-title">No terminal activity on this page</div><div class="row-meta">CodeLocal only records metadata after an approved terminal command actually runs.</div></div>`)
	}
	body.WriteString(`</div>`)

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
	_, _ = w.Write([]byte(ui.Page("Security · CodeLocal Cloud", "Cloud audit metadata for terminal executions only.", body.String())))
}

var _ *webauth.Identity
