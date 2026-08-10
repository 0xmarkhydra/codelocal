package ui

import (
	"fmt"
	"strings"
	"time"
)

type DashboardOptions struct {
	Title    string
	Active   string
	Email    string
	CSRF     string
	Subtitle string
	Actions  string
	Body     string
	IsAdmin  bool
}

type LandingOptions struct {
	SignedIn bool
	Endpoint string
}

var UIIcons = map[string]string{
	"overview": navIcons["overview"],
	"workspaces": navIcons["workspaces"],
	"devices": navIcons["devices"],
	"usage": navIcons["usage"],
	"connect": navIcons["connect"],
	"security": navIcons["security"],
	"admin": navIcons["admin"],
	"folder": icon(`<path d="M3.5 7.5h6l2-2h9v13.5H3.5z"/><path d="M3.5 9.5h17"/>`),
	"folderCheck": icon(`<path d="M3.5 7.5h6l2-2h9v13.5H3.5z"/><path d="M3.5 9.5h17"/><path d="M9 14.5l2 2 4-4"/>`),
	"device": icon(`<rect x="5" y="3" width="14" height="18" rx="3"/><path d="M9.5 6h5M11 18h2"/>`),
	"shield": navIcons["security"],
}

func Icon(name string) string { return UIIcons[name] }

func FormatTime(value int64) string {
	if value <= 0 {
		return "—"
	}
	return time.UnixMilli(value).Format("Jan 2, 2006, 3:04 PM")
}

func navLink(href, key, label string, active bool) string {
	className := ""
	if active {
		className = ` class="active"`
	}
	return `<a href="` + href + `"` + className + `><span class="nav-icon">` + UIIcons[key] + `</span><span>` + Escape(label) + `</span></a>`
}

func DashboardPage(options DashboardOptions) string {
	initial := "C"
	if strings.TrimSpace(options.Email) != "" {
		initial = strings.ToUpper(string([]rune(options.Email)[0]))
	}
	brand := `<div class="brand"><img class="logo" src="/assets/codelocal-icon.png" alt=""><span class="brand-name">CodeLocal</span><span class="brand-tag">CLOUD</span></div>`
	var nav strings.Builder
	nav.WriteString(`<div class="nav-label">Control plane</div><nav class="nav">`)
	nav.WriteString(navLink("/dashboard", "overview", "Overview", options.Active == "overview"))
	nav.WriteString(navLink("/dashboard/workspaces", "workspaces", "Workspaces", options.Active == "workspaces"))
	nav.WriteString(navLink("/dashboard/devices", "devices", "Devices", options.Active == "devices"))
	nav.WriteString(navLink("/dashboard/usage", "usage", "Token usage", options.Active == "usage"))
	nav.WriteString(navLink("/dashboard/connect", "connect", "Connect ChatGPT", options.Active == "connect"))
	nav.WriteString(navLink("/dashboard/security", "security", "Security", options.Active == "security"))
	if options.IsAdmin {
		nav.WriteString(`<div class="nav-label">Administration</div>`)
		nav.WriteString(navLink("/dashboard/admin", "admin", "Admin", options.Active == "admin"))
	}
	nav.WriteString(`</nav>`)
	logout := `<form method="post" action="/logout">` + Hidden(map[string]string{"csrf": options.CSRF}) + `<button class="btn ghost small" type="submit" aria-label="Sign out" title="Sign out">↗</button></form>`
	status := `<div class="sidebar-spacer"></div><div class="sidebar-status"><div class="sidebar-status-title"><span class="status-dot green"></span>Cloud connected</div><div class="sidebar-status-copy">Local source code and secrets stay on your machine.</div></div><div class="sidebar-foot"><div class="avatar">` + Escape(initial) + `</div><div class="account"><div class="account-email">` + Escape(options.Email) + `</div><div class="account-meta">CodeLocal account</div></div>` + logout + `</div>`
	return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover"><meta name="color-scheme" content="light dark">` + brandHead + `<meta name="theme-color" content="#090a0d"><title>` + Escape(options.Title) + ` · CodeLocal</title><style>` + styles + `</style></head><body><div class="shell"><aside class="sidebar">` + brand + nav.String() + status + `</aside><main class="main"><header class="top"><div class="top-copy"><div class="eyebrow"><span class="status-dot green"></span>CodeLocal Cloud</div><h1 class="h1">` + Escape(options.Title) + `</h1><div class="sub">` + Escape(options.Subtitle) + `</div></div><div class="top-actions">` + options.Actions + `</div></header>` + options.Body + `</main></div><script>(()=>{document.querySelectorAll('[data-copy-target]').forEach(btn=>btn.addEventListener('click',async()=>{const el=document.querySelector(btn.getAttribute('data-copy-target'));if(!el)return;await navigator.clipboard.writeText(el.textContent||'');const old=btn.textContent;btn.textContent='Copied';setTimeout(()=>btn.textContent=old,1200)}));document.querySelectorAll('[data-confirm]').forEach(btn=>btn.addEventListener('click',e=>{const message=btn.getAttribute('data-confirm-message')||'Continue?';if(!confirm(message))e.preventDefault()}));})();</script></body></html>`
}

func LandingPage(options LandingOptions) string {
	authHref, authLabel := "/register", "Create free account"
	if options.SignedIn {
		authHref, authLabel = "/dashboard", "Open dashboard"
	}
	endpoint := Escape(options.Endpoint)
	return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover"><meta name="color-scheme" content="light dark">` + brandHead + `<meta name="description" content="Connect ChatGPT securely to project folders you explicitly authorize on your computer."><meta property="og:title" content="CodeLocal · Give ChatGPT hands on your local code"><meta property="og:description" content="A private, permission-aware bridge between ChatGPT and the project folders you authorize."><meta property="og:image" content="/assets/codelocal-icon.png"><title>CodeLocal · Connect ChatGPT to your local code</title><style>` + styles + landingStyles + `</style></head><body><div class="landing"><nav class="landing-nav"><div class="landing-nav-inner"><a class="landing-brand" href="/"><img class="landing-logo" src="/assets/codelocal-icon.png" alt="CodeLocal"><span>CodeLocal</span></a><div class="landing-links"><a class="btn ghost" href="#setup">Setup guide</a><a class="btn primary" href="` + authHref + `">` + authLabel + `</a></div></div></nav><main class="landing-main"><section class="landing-hero"><div><div class="landing-kicker"><span class="status-dot green"></span>ChatGPT ↔ your computer</div><h1>You already have ChatGPT. Now let it code on your machine.</h1><div class="landing-hero-copy">ChatGPT stays the AI brain. CodeLocal is the secure bridge to folders and developer tools you explicitly authorize — no separate OpenAI API key and no per-token billing from CodeLocal.</div><div class="landing-actions"><a class="btn primary" href="#setup">Set up CodeLocal</a><a class="btn" href="` + authHref + `">` + authLabel + `</a></div><div class="privacy-note">Your source code, secrets and terminal output stay on your machine.</div></div><div class="connection-preview"><div class="preview-head"><img class="preview-icon" src="/assets/chatgpt-plugin-icon.png" alt="CodeLocal plugin icon"><div><div class="preview-title">CodeLocal</div><div class="preview-sub">Remote MCP for ChatGPT</div></div></div><div class="preview-row"><span>Flow</span><strong>You → ChatGPT → MCP → CodeLocal</strong></div><div class="preview-row"><span>Authentication</span><strong>OAuth</strong></div><div class="preview-row"><span>Runtime</span><strong>Local Go client</strong></div><div class="preview-row"><span>Status</span><span class="badge green">Ready to connect</span></div></div></section><section class="setup-section" id="setup"><div class="section-kicker">Detailed setup</div><h2 class="landing-section-title">ChatGPT is the brain. CodeLocal gives it hands.</h2><div class="section-copy">One lightweight machine runtime can serve every project you explicitly authorize. Workspace selection remains isolated per ChatGPT MCP session.</div><div class="setup-layout"><div class="setup-list"><article class="setup-step"><div class="step-title">1 · Connect ChatGPT</div><div class="step-summary">Add CodeLocal as a remote MCP and authenticate with your CodeLocal account.</div><div style="height:14px"></div><div class="copy-row"><div class="code-block" id="landing-endpoint">` + endpoint + `</div><button class="btn" type="button" data-copy-target="#landing-endpoint">Copy</button></div></article><article class="setup-step"><div class="step-title">2 · Install CodeLocal</div><div class="step-summary">Install the lightweight native runtime.</div><div style="height:14px"></div><div class="code-block">npm install -g codelocal@beta</div></article><article class="setup-step"><div class="step-title">3 · Authorize a project</div><div class="step-summary">Run this once inside each folder you want ChatGPT to access.</div><div style="height:14px"></div><div class="code-block">cd /path/to/project
codelocal .</div></article><article class="setup-step"><div class="step-title">4 · Start one machine runtime</div><div class="step-summary">Run CodeLocal once. You do not need one process per workspace.</div><div style="height:14px"></div><div class="code-block">codelocal</div></article><article class="setup-step"><div class="step-title">5 · Ask ChatGPT to use CodeLocal</div><div class="step-summary">Select the workspace in ChatGPT, then read files, edit code, run approved commands and inspect Git through the MCP.</div></article></div><aside class="plugin-panel"><div class="plugin-panel-head"><img class="plugin-panel-icon" src="/assets/chatgpt-plugin-icon.png" alt=""><div><div class="plugin-panel-title">CodeLocal for ChatGPT</div><div class="plugin-panel-sub">Production plugin asset</div></div></div><div class="divider"></div><div class="label">No separate OpenAI API key. CodeLocal does not add a per-token charge. MCP usage remains governed by the ChatGPT plan and product limits.</div><div style="height:16px"></div><a class="btn primary" style="width:100%" href="/assets/chatgpt-plugin-icon.png" download="codelocal-chatgpt-plugin-icon.png">Download plugin icon</a></aside></div></section><footer class="landing-footer"><span>© CodeLocal</span><span>Local source code and secrets remain on your computer.</span></footer></main></div><script>document.querySelectorAll('[data-copy-target]').forEach(btn=>btn.addEventListener('click',async()=>{const el=document.querySelector(btn.getAttribute('data-copy-target'));if(!el)return;await navigator.clipboard.writeText(el.textContent||'');const old=btn.textContent;btn.textContent='Copied';setTimeout(()=>btn.textContent=old,1200)}));</script></body></html>`
}

func MetricCard(label string, value any, sub string) string {
	return `<div class="card metric-card span4"><div class="metric-label">` + Escape(label) + `</div><div class="metric">` + Escape(fmt.Sprint(value)) + `</div><div class="metric-sub">` + Escape(sub) + `</div></div>`
}
