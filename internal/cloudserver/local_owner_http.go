package cloudserver

import (
	"net/http"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/webauth"
)

const localOwnerPage = `<!doctype html><html><head><meta charset="utf-8"><title>CodeLocal self-host</title>
<style>body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:#0b0d12;color:#e6e8ee;display:flex;align-items:center;justify-content:center;height:100vh;margin:0}
.card{background:#151823;border:1px solid #262b3a;border-radius:14px;padding:34px 40px;max-width:520px}
h1{margin:0 0 10px;font-size:20px}code{background:#0b0d12;padding:2px 6px;border-radius:6px;color:#8ab4ff}
p{color:#9aa3b5;line-height:1.6;margin:8px 0}</style></head><body><div class="card">
<h1>CodeLocal self-host</h1><p>%s</p><p>Ch&#7871; &#273;&#7897; <code>CODELOCAL_LOCAL_OWNER</code> &#273;ang b&#7853;t: kh&#244;ng c&#7847;n &#273;&#259;ng nh&#7853;p.</p>
</div></body></html>`

// localOwnerAuthorizeGet auto-approves an MCP authorization request for the
// single local owner. Without CODELOCAL_LOCAL_OWNER the gateway falls through to
// the normal web approval flow.
func (s *Server) localOwnerAuthorizeGet(w http.ResponseWriter, r *http.Request) {
	if !webauth.LocalOwnerMode() {
		http.NotFound(w, r)
		return
	}
	owner, err := s.WebAuth.LocalOwnerIdentity(r.Context())
	if err != nil || owner == nil {
		http.Error(w, "local owner unavailable", http.StatusServiceUnavailable)
		return
	}
	s.OAuth.AutoAuthorizeLocalOwner(w, r, owner)
}

// localOwnerPairApproveGet reports that the device was already approved.
func (s *Server) localOwnerPairApproveGet(w http.ResponseWriter, r *http.Request) {
	if !webauth.LocalOwnerMode() {
		http.NotFound(w, r)
		return
	}
	if _, ok := s.identity(r); !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	message := "Thi&#7871;t b&#7883; &#273;&#227; &#273;&#432;&#7907;c ph&#234; duy&#7879;t t&#7921; &#273;&#7897;ng."
	if strings.TrimSpace(r.URL.Query().Get("approved")) == "1" {
		message = "Ho&#224;n t&#7845;t: thi&#7871;t b&#7883; &#273;&#227; &#273;&#432;&#7907;c gh&#233;p."
	}
	_, _ = w.Write([]byte(strings.Replace(localOwnerPage, "%s", message, 1)))
}
