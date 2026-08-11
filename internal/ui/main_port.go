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
	"overview":    navIcons["overview"],
	"workspaces":  navIcons["workspaces"],
	"devices":     navIcons["devices"],
	"usage":       navIcons["usage"],
	"connect":     navIcons["connect"],
	"security":    navIcons["security"],
	"admin":       navIcons["admin"],
	"folder":      icon(`<path d="M3.5 7.5h6l2-2h9v13.5H3.5z"/><path d="M3.5 9.5h17"/>`),
	"folderCheck": icon(`<path d="M3.5 7.5h6l2-2h9v13.5H3.5z"/><path d="M3.5 9.5h17"/><path d="M9 14.5l2 2 4-4"/>`),
	"device":      icon(`<rect x="5" y="3" width="14" height="18" rx="3"/><path d="M9.5 6h5M11 18h2"/>`),
	"shield":      navIcons["security"],
}

const mainPortExtras = `
.card.flat{box-shadow:none}.metric-state{font-size:18px;letter-spacing:-.02em;margin-top:15px}.section-head .label{margin-top:4px}.btn-icon{font-size:12px;opacity:.9}.muted{color:var(--muted)}
.kv{display:grid;grid-template-columns:140px minmax(0,1fr);gap:10px;padding:10px 0;border-bottom:1px solid var(--line-soft);font-size:11px}.kv:last-child{border-bottom:0}.kv-key{color:var(--muted2)}.kv-value{min-width:0;overflow-wrap:anywhere}.empty-icon svg{width:20px;height:20px;display:block;stroke:currentColor;stroke-width:1.8;fill:none;stroke-linecap:round;stroke-linejoin:round}
dialog{width:min(440px,calc(100vw - 30px));padding:0;border:1px solid var(--line);border-radius:22px;background:var(--surface);color:var(--text);backdrop-filter:blur(28px);-webkit-backdrop-filter:blur(28px);box-shadow:0 36px 120px rgba(0,0,0,.3)}dialog::backdrop{background:rgba(10,12,18,.46);backdrop-filter:blur(10px)}.modal{padding:26px}.modal-icon{width:38px;height:38px;display:grid;place-items:center;border:0;border-radius:13px;background:rgba(215,0,21,.09);color:var(--red);margin-bottom:14px}.modal-title{font-size:19px;font-weight:670;letter-spacing:-.02em}.modal-copy{color:var(--muted);font-size:12px;line-height:1.65;margin-top:7px}.modal-actions{display:flex;justify-content:flex-end;gap:8px;margin-top:22px}
.privacy-note svg{width:17px;height:17px;stroke:var(--green);fill:none;stroke-width:1.8}.preview-field{margin-top:20px}.preview-label{color:var(--muted2);font-size:10px;font-weight:650;text-transform:uppercase;letter-spacing:.08em}.preview-value{margin-top:7px;padding:13px 14px;border:1px solid var(--line);border-radius:13px;background:var(--code);font:11.5px/1.5 "SFMono-Regular",Consolas,monospace;overflow-wrap:anywhere}
.landing-section-title,.section-title{max-width:780px;margin:10px 0 12px;font-size:clamp(35px,5vw,56px);font-weight:700;line-height:1.06;letter-spacing:-.05em}.setup-step{display:grid;grid-template-columns:48px minmax(0,1fr);gap:18px}.step-number{width:48px;height:48px;display:grid;place-items:center;border-radius:15px;background:rgba(0,113,227,.1);color:var(--accent);font-size:16px;font-weight:720}.step-details{margin:17px 0 0;padding-left:20px;color:var(--muted);font-size:12px;line-height:1.7}.step-details li+li{margin-top:7px}.step-details strong{color:var(--text);font-weight:620}.step-code{margin-top:15px}.step-note{margin-top:14px;padding:12px 14px;border-radius:13px;background:rgba(0,113,227,.07);color:var(--muted);font-size:11.5px;line-height:1.6}.step-note strong{color:var(--accent)}
.plugin-specs{display:grid;margin-top:20px;border-top:1px solid var(--line-soft)}.plugin-spec{display:grid;grid-template-columns:92px minmax(0,1fr);gap:12px;padding:13px 0;border-bottom:1px solid var(--line-soft);font-size:11.5px}.plugin-spec-key{color:var(--muted2)}.plugin-spec-value{min-width:0;font-weight:590;overflow-wrap:anywhere}.plugin-panel .btn{width:100%;margin-top:12px}.size-ok{display:flex;align-items:center;gap:7px;margin-top:11px;color:var(--green);font-size:10.5px}.size-ok:before{content:"✓";width:18px;height:18px;display:grid;place-items:center;border-radius:50%;background:rgba(36,161,72,.11);font-size:10px;font-weight:800}
.security-section{padding:30px 0 96px}.security-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:16px;margin-top:32px}.security-card{padding:24px;border:1px solid var(--line);border-radius:22px;background:var(--surface)}.security-icon{width:40px;height:40px;display:grid;place-items:center;border-radius:13px;background:rgba(0,113,227,.09);color:var(--accent);font-size:18px}.security-icon svg{width:21px;height:21px;display:block;stroke:currentColor;stroke-width:1.8;fill:none;stroke-linecap:round;stroke-linejoin:round}.security-title{margin-top:18px;font-size:16px;font-weight:650}.security-copy{margin-top:7px;color:var(--muted);font-size:12px;line-height:1.62}
.list-toolbar{display:flex;align-items:center;justify-content:space-between;gap:14px;margin-top:18px;padding-top:18px;border-top:1px solid var(--line-soft)}.search-form{display:flex;align-items:center;gap:8px;flex:1;max-width:680px}.search-input{width:100%;min-width:0;height:36px;padding:0 12px;border:1px solid var(--line);border-radius:10px;background:var(--code);color:var(--text);font:12px/1.2 inherit;outline:none}.search-input:focus{border-color:rgba(0,113,227,.55);box-shadow:0 0 0 3px rgba(0,113,227,.09)}.btn.disabled{opacity:.42;pointer-events:none}.pager{display:flex;align-items:center;justify-content:center;gap:12px;padding-top:18px;margin-top:4px;border-top:1px solid var(--line-soft)}.pager-meta{min-width:86px;text-align:center;color:var(--muted);font-size:11px}
.invite-card{display:flex;align-items:center;justify-content:space-between;gap:28px;background:linear-gradient(135deg,rgba(0,113,227,.075),rgba(255,255,255,0) 62%),var(--surface)}.invite-code-wrap{display:flex;align-items:center;gap:9px;flex-shrink:0}.invite-code{min-width:170px;padding:11px 15px;border:1px solid var(--line);border-radius:12px;background:var(--code);font-size:15px;font-weight:720;letter-spacing:.08em;text-align:center}.usage-note{background:linear-gradient(135deg,rgba(0,113,227,.045),transparent 60%),var(--surface)}
.admin-grid{align-items:start}.admin-hero{display:flex;align-items:center;justify-content:space-between;gap:20px;background:linear-gradient(135deg,rgba(0,113,227,.08),rgba(120,80,255,.035) 55%,transparent),var(--surface)}.admin-stat-grid{grid-column:span 12;display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:12px}.admin-stat{padding:20px;border:1px solid var(--line);border-radius:18px;background:var(--surface)}.admin-stat-label{color:var(--muted2);font-size:10px;font-weight:680;text-transform:uppercase;letter-spacing:.08em}.admin-stat-value{margin-top:9px;font-size:30px;font-weight:720;letter-spacing:-.04em}.admin-stat-sub{margin-top:5px;color:var(--muted);font-size:10.5px}.admin-table{margin-top:14px;border:1px solid var(--line-soft);border-radius:15px;overflow:hidden}.admin-row{display:grid;grid-template-columns:minmax(220px,1.45fr) 150px minmax(220px,1fr) minmax(220px,1fr);align-items:center;gap:16px;padding:15px 16px;border-bottom:1px solid var(--line-soft)}.admin-row:last-child{border-bottom:0}.admin-head{background:var(--code);color:var(--muted2);font-size:9.5px;font-weight:700;text-transform:uppercase;letter-spacing:.08em}.admin-user-email{font-size:12px;font-weight:650;overflow-wrap:anywhere}.admin-cell-sub{margin-top:4px;color:var(--muted);font-size:10.5px;line-height:1.45}.admin-code{font-size:11px;font-weight:650;letter-spacing:.04em}.admin-activity{font-size:11px;font-weight:590}.referral-tree-card .tree{display:grid;gap:8px;max-height:620px;overflow:auto;padding-right:4px}.tree-node{padding:11px 13px;border:1px solid var(--line-soft);border-radius:12px;background:var(--code)}.tree-node strong{font-size:11.5px}.tree-node .badge{margin-left:6px}
@media(max-width:1050px){.admin-stat-grid{grid-template-columns:repeat(2,minmax(0,1fr))}.admin-row{grid-template-columns:minmax(0,1fr) minmax(0,1fr)}.admin-head{display:none}}@media(max-width:900px){.security-grid{grid-template-columns:1fr}}@media(max-width:760px){.kv{grid-template-columns:1fr;gap:3px}.list-toolbar,.invite-card,.admin-hero{align-items:stretch;flex-direction:column}.search-form{max-width:none}.invite-code-wrap{width:100%}.invite-code{flex:1}.admin-stat-grid,.admin-row{grid-template-columns:1fr}.admin-status{margin-top:-4px}}@media(max-width:600px){.setup-step{grid-template-columns:1fr;padding:21px}.step-number{width:40px;height:40px;border-radius:12px}.plugin-spec{grid-template-columns:1fr;gap:4px}.search-form{flex-wrap:wrap}.search-input{flex-basis:100%}.invite-code-wrap{align-items:stretch;flex-direction:column}}
`

const dashboardScript = `
(() => {
  const dialog = document.getElementById('confirm-dialog');
  const title = document.getElementById('confirm-title');
  const copy = document.getElementById('confirm-copy');
  const confirmButton = document.getElementById('confirm-submit');
  let pendingForm = null;
  document.addEventListener('click', (event) => {
    const button = event.target.closest('[data-confirm]');
    if (!button || !dialog) return;
    event.preventDefault();
    pendingForm = button.closest('form');
    title.textContent = button.dataset.confirmTitle || 'Are you sure?';
    copy.textContent = button.dataset.confirmMessage || 'This action cannot be undone.';
    confirmButton.textContent = button.dataset.confirmLabel || 'Continue';
    confirmButton.className = 'btn ' + (button.dataset.confirmTone === 'danger' ? 'danger' : 'primary');
    dialog.showModal();
  });
  confirmButton?.addEventListener('click', () => {
    if (!pendingForm) return dialog?.close();
    const form = pendingForm;
    pendingForm = null;
    dialog?.close();
    form.requestSubmit();
  });
  document.querySelectorAll('[data-copy-target]').forEach((button) => {
    button.addEventListener('click', async () => {
      const target = document.querySelector(button.getAttribute('data-copy-target'));
      if (!target) return;
      try {
        await navigator.clipboard.writeText(target.textContent || '');
        const previous = button.textContent;
        button.textContent = 'Copied';
        setTimeout(() => button.textContent = previous, 1400);
      } catch {}
    });
  });
})();
`

const landingScript = `
document.querySelectorAll('[data-copy-value]').forEach((button) => {
  button.addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText(button.dataset.copyValue || '');
      const previous = button.textContent;
      button.textContent = 'Copied';
      setTimeout(() => button.textContent = previous, 1400);
    } catch {}
  });
});
`

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
	initial := "U"
	if strings.TrimSpace(options.Email) != "" {
		initial = strings.ToUpper(string([]rune(options.Email)[0]))
	}
	brand := `<div class="brand"><img class="logo" src="/assets/codelocal-icon.png" alt=""><span class="brand-name">CodeLocal</span><span class="brand-tag">CLOUD</span></div>`
	var nav strings.Builder
	nav.WriteString(`<div class="nav-label">Control plane</div><nav class="nav">`)
	nav.WriteString(navLink("/dashboard", "overview", "Overview", options.Active == "overview"))
	nav.WriteString(navLink("/dashboard/workspaces", "workspaces", "Workspaces", options.Active == "workspaces"))
	nav.WriteString(navLink("/dashboard/devices", "devices", "Devices", options.Active == "devices"))
	nav.WriteString(navLink("/dashboard/connect", "connect", "Connect ChatGPT", options.Active == "connect"))
	if options.IsAdmin {
		nav.WriteString(`</nav><div class="nav-label">Administration</div><nav class="nav">`)
		nav.WriteString(navLink("/dashboard/admin", "admin", "Admin", options.Active == "admin"))
	}
	nav.WriteString(`</nav>`)
	logout := `<form method="post" action="/logout">` + Hidden(map[string]string{"csrf": options.CSRF}) + `<button class="btn ghost small" type="submit">Sign out</button></form>`
	status := `<div class="sidebar-spacer"></div><div class="sidebar-status"><div class="sidebar-status-title"><span class="status-dot green"></span>Cloud connected</div><div class="sidebar-status-copy">Local source code and secrets stay on your machine.</div></div><div class="sidebar-foot"><div class="avatar">` + Escape(initial) + `</div><div class="account"><div class="account-email">` + Escape(options.Email) + `</div><div class="account-meta">CodeLocal account</div></div>` + logout + `</div>`
	subtitle := ""
	if strings.TrimSpace(options.Subtitle) != "" {
		subtitle = `<div class="sub">` + Escape(options.Subtitle) + `</div>`
	}
	actions := ""
	if strings.TrimSpace(options.Actions) != "" {
		actions = `<div class="top-actions">` + options.Actions + `</div>`
	}
	return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover"><meta name="color-scheme" content="light dark">` + brandHead + `<title>` + Escape(options.Title) + ` · CodeLocal</title><style>` + styles + mainPortExtras + `</style></head><body><div class="shell"><div class="sidebar-column"><aside class="sidebar">` + brand + nav.String() + status + `</aside></div><main class="main"><header class="top"><div class="top-copy"><div class="eyebrow"><span class="status-dot green"></span>CodeLocal Cloud</div><div class="h1">` + Escape(options.Title) + `</div>` + subtitle + `</div>` + actions + `</header>` + options.Body + `</main></div><dialog id="confirm-dialog"><div class="modal"><div class="modal-icon">!</div><div class="modal-title" id="confirm-title">Are you sure?</div><div class="modal-copy" id="confirm-copy">This action cannot be undone.</div><div class="modal-actions"><button class="btn" type="button" onclick="this.closest('dialog').close()">Cancel</button><button class="btn danger" id="confirm-submit" type="button">Continue</button></div></div></dialog><script>` + dashboardScript + `</script></body></html>`
}

func LandingPage(options LandingOptions) string {
	endpoint := Escape(options.Endpoint)
	accountAction := `<a class="btn" href="/login">Sign in</a><a class="btn primary" href="/register">Create account</a>`
	secondaryAction := `<a class="btn" href="/register">Create free account</a>`
	if options.SignedIn {
		accountAction = `<a class="btn primary" href="/dashboard">Open dashboard</a>`
		secondaryAction = `<a class="btn" href="/dashboard/connect">Connection settings</a>`
	}
	year := fmt.Sprint(time.Now().Year())
	return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover"><meta name="color-scheme" content="light dark">` + brandHead + `<meta name="description" content="Connect ChatGPT securely to project folders you explicitly authorize on your computer."><title>CodeLocal · Connect ChatGPT to your local code</title><style>` + styles + landingStyles + mainPortExtras + `</style></head><body><div class="landing">
<nav class="landing-nav"><div class="landing-nav-inner"><a class="landing-brand" href="/"><img class="landing-logo" src="/assets/codelocal-icon.png" alt="CodeLocal"><span>CodeLocal</span></a><div class="landing-links"><a class="btn ghost" href="#setup">Setup guide</a>` + accountAction + `</div></div></nav>
<main class="landing-main">
<section class="landing-hero"><div><div class="landing-kicker"><span class="status-dot green"></span>CodeLocal Cloud</div><h1>Connect ChatGPT to the code on your machine.</h1><div class="landing-hero-copy">Grant only the project folders you choose. CodeLocal keeps source code, secrets and terminal access local while ChatGPT works through a secure, permission-aware connection.</div><div class="landing-actions"><a class="btn primary" href="#setup">Set up CodeLocal</a>` + secondaryAction + `</div><div class="privacy-note"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3l7 3v5c0 4.6-2.8 8.3-7 10-4.2-1.7-7-5.4-7-10V6z"/><path d="M9.5 12l1.7 1.7 3.5-4"/></svg>Your files stay on your machine. Risky actions still require confirmation.</div></div><div class="connection-preview"><div class="preview-head"><img class="preview-icon" src="/assets/chatgpt-plugin-icon.png" alt="CodeLocal plugin icon"><div><div class="preview-title">CodeLocal</div><div class="preview-sub">Custom MCP plugin for ChatGPT</div></div></div><div class="preview-field"><div class="preview-label">Server URL</div><div class="preview-value">` + endpoint + `</div></div><div class="preview-row"><span>Authentication</span><strong>OAuth</strong></div><div class="preview-row"><span>Transport</span><strong>Remote MCP</strong></div><div class="preview-row"><span>Status</span><span class="badge green">Ready to connect</span></div></div></section>
<section class="setup-section" id="setup"><div class="section-kicker">Detailed setup</div><h2 class="section-title">Set up CodeLocal in five steps.</h2><div class="section-copy">Complete the ChatGPT connection first, then pair your computer and authorize only the projects you want ChatGPT to use.</div><div class="setup-layout"><div class="setup-list">
<article class="setup-step"><div class="step-number">1</div><div><div class="step-title">Connect CodeLocal with ChatGPT</div><div class="step-summary">Create a custom plugin using the remote CodeLocal MCP endpoint.</div><ol class="step-details"><li>Open <strong>ChatGPT</strong>, choose <strong>Plugins</strong> in the left sidebar, then press the <strong>+</strong> button.</li><li>Choose <strong>New Plugin</strong> and upload the CodeLocal icon shown on this page.</li><li>Enter <strong>CodeLocal</strong> for the name and <strong>Connect ChatGPT to project folders you authorize on your computer.</strong> for the description.</li><li>Keep <strong>Server URL</strong> selected and enter <strong>` + endpoint + `</strong>.</li><li>Set <strong>Authentication</strong> to <strong>OAuth</strong>. Leave Advanced OAuth settings on the automatically discovered values.</li><li>Read the custom MCP risk notice, tick <strong>I understand and want to continue</strong>, then save the plugin.</li><li>When the browser opens, sign in or create your CodeLocal account and approve the OAuth connection.</li></ol></div></article>
<article class="setup-step"><div class="step-number">2</div><div><div class="step-title">Install CodeLocal on your computer</div><div class="step-summary">Install the current beta runtime used by the dev environment.</div><div class="code-block step-code">npm install -g codelocal@beta</div><div class="step-note"><strong>If npm reports EEXIST:</strong> run <code>npm uninstall -g codelocal</code>, then install again. Avoid <code>--force</code> unless you verified the conflicting file.</div></div></article>
<article class="setup-step"><div class="step-number">3</div><div><div class="step-title">Pair this computer</div><div class="step-summary">The first start creates a private device credential for this machine.</div><div class="code-block step-code">codelocal</div><ol class="step-details"><li>Your browser opens the CodeLocal approval page automatically.</li><li>Sign in with the same account used for the ChatGPT plugin.</li><li>Check the machine name, approve the device, then return to the terminal.</li></ol></div></article>
<article class="setup-step"><div class="step-number">4</div><div><div class="step-title">Authorize a project folder</div><div class="step-summary">CodeLocal never exposes every folder on your computer. Grant each project explicitly.</div><div class="code-block step-code">cd /path/to/project
codelocal .</div><div class="step-note">Run <code>codelocal .</code> once inside each project you want ChatGPT to access.</div></div></article>
<article class="setup-step"><div class="step-number">5</div><div><div class="step-title">Start working from ChatGPT</div><div class="step-summary">Keep one lightweight machine runtime running, then select a workspace in chat.</div><div class="code-block step-code">codelocal</div><ol class="step-details"><li>Return to ChatGPT and enable <strong>CodeLocal</strong> for the conversation.</li><li>Ask CodeLocal to list your authorized workspaces.</li><li>Choose the project you want. Sleeping projects activate only when selected.</li><li>Approve sensitive terminal or file actions when CodeLocal asks.</li></ol></div></article>
</div><aside class="plugin-panel"><div class="plugin-panel-head"><img class="plugin-panel-icon" src="/assets/chatgpt-plugin-icon.png" alt=""><div><div class="plugin-panel-title">ChatGPT plugin icon</div><div class="plugin-panel-sub">Ready to upload in New Plugin</div></div></div><div class="plugin-specs"><div class="plugin-spec"><div class="plugin-spec-key">Format</div><div class="plugin-spec-value">PNG</div></div><div class="plugin-spec"><div class="plugin-spec-key">Dimensions</div><div class="plugin-spec-value">256 × 256 px</div></div><div class="plugin-spec"><div class="plugin-spec-key">Server URL</div><div class="plugin-spec-value">` + endpoint + `</div></div><div class="plugin-spec"><div class="plugin-spec-key">Authentication</div><div class="plugin-spec-value">OAuth</div></div></div><a class="btn primary" href="/assets/chatgpt-plugin-icon.png" download="codelocal-chatgpt-plugin-icon.png">Download plugin icon</a><button class="btn" type="button" data-copy-value="` + endpoint + `">Copy Server URL</button><div class="size-ok">Ready for ChatGPT plugin setup</div></aside></div></section>
<section class="security-section"><div class="section-kicker">Private by design</div><h2 class="section-title">Your machine stays in control.</h2><div class="security-grid"><div class="security-card"><div class="security-icon">` + UIIcons["folderCheck"] + `</div><div class="security-title">Explicit workspace access</div><div class="security-copy">Only folders you grant with CodeLocal appear in ChatGPT. Revoke a workspace without deleting its files.</div></div><div class="security-card"><div class="security-icon">✓</div><div class="security-title">Risk confirmation</div><div class="security-copy">Potentially destructive terminal or filesystem actions require approval before they run on your computer.</div></div><div class="security-card"><div class="security-icon">` + UIIcons["device"] + `</div><div class="security-title">Revocable devices</div><div class="security-copy">Review and revoke paired computers from your dashboard whenever a machine is lost or no longer trusted.</div></div></div></section>
<footer class="landing-footer"><span>© ` + year + ` CodeLocal</span><span>Local source code and secrets remain on your computer.</span></footer>
</main></div><script>` + landingScript + `</script></body></html>`
}

func MetricCard(label string, value any, sub string) string {
	return `<div class="card metric-card span4"><div class="metric-label">` + Escape(label) + `</div><div class="metric">` + Escape(fmt.Sprint(value)) + `</div><div class="metric-sub">` + Escape(sub) + `</div></div>`
}
