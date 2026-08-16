package cloudserver

import (
	"html"
	"io"
	"net/http"
	"os"
	"strings"
)

const publicPageCSS = `
:root{color-scheme:light;font-family:Inter,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;color:#1b2440;background:#f5f8ff}*{box-sizing:border-box}body{margin:0;background:radial-gradient(circle at top right,#e6edff 0,transparent 34%),#f5f8ff;color:#1b2440}.shell{width:min(920px,calc(100% - 32px));margin:0 auto;padding:28px 0 72px}.nav{display:flex;align-items:center;justify-content:space-between;gap:20px;margin-bottom:56px}.brand{display:inline-flex;align-items:center;gap:10px;font-weight:800;font-size:18px;letter-spacing:-.025em;text-decoration:none;color:#16203b}.brand img{width:36px;height:36px;border-radius:12px;background:#fff;box-shadow:0 9px 24px rgba(45,79,167,.13)}.navlinks{display:flex;gap:16px;flex-wrap:wrap}.navlinks a,.footer a{color:#536bdc;text-decoration:none;font-weight:650}.hero{padding:38px;border:1px solid #fff;border-radius:30px;background:rgba(255,255,255,.78);box-shadow:0 28px 80px rgba(61,82,139,.10)}.eyebrow{font-size:11px;font-weight:800;text-transform:uppercase;letter-spacing:.12em;color:#5c73dd}.hero h1{margin:14px 0 10px;font-size:clamp(38px,7vw,62px);line-height:1;letter-spacing:-.05em}.hero p{max-width:720px;color:#707a95;font-size:16px;line-height:1.7}.content{display:grid;gap:14px;margin-top:18px}.card{padding:26px;border:1px solid #fff;border-radius:22px;background:rgba(255,255,255,.72);box-shadow:0 14px 42px rgba(61,82,139,.06)}.card h2{margin:0 0 10px;font-size:18px}.card p,.card li{color:#68738f;font-size:14px;line-height:1.72}.card ul{margin:10px 0 0;padding-left:20px}.meta{margin-top:14px;color:#8b94aa;font-size:12px}.footer{display:flex;justify-content:space-between;gap:20px;flex-wrap:wrap;margin-top:36px;padding-top:24px;border-top:1px solid rgba(85,103,151,.12);color:#8992a8;font-size:12px}@media(max-width:640px){.hero{padding:26px}.nav{align-items:flex-start;flex-direction:column}.card{padding:22px}}
`

type publicPageSection struct {
	Title string
	Body  string
}

type publicPageSpec struct {
	Title    string
	Eyebrow  string
	Summary  string
	Sections []publicPageSection
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func publicPublisher() string    { return envOr("CODELOCAL_PUBLISHER_NAME", "CodeLocal") }
func publicSupportEmail() string { return envOr("CODELOCAL_SUPPORT_EMAIL", "support@codelocal.cloud") }
func publicSecurityEmail() string {
	return envOr("CODELOCAL_SECURITY_EMAIL", "security@codelocal.cloud")
}

func publicPage(spec publicPageSpec) string {
	var cards strings.Builder
	for _, section := range spec.Sections {
		cards.WriteString(`<section class="card"><h2>` + html.EscapeString(section.Title) + `</h2>` + section.Body + `</section>`)
	}
	return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="light"><title>` + html.EscapeString(spec.Title) + ` · CodeLocal</title><style>` + publicPageCSS + `</style></head><body><main class="shell"><nav class="nav"><a class="brand" href="/"><img src="/assets/codelocal-icon.png" alt=""><span>CodeLocal</span></a><div class="navlinks"><a href="/privacy">Privacy</a><a href="/security">Security</a><a href="/terms">Terms</a><a href="/support">Support</a></div></nav><section class="hero"><div class="eyebrow">` + html.EscapeString(spec.Eyebrow) + `</div><h1>` + html.EscapeString(spec.Title) + `</h1><p>` + html.EscapeString(spec.Summary) + `</p><div class="meta">Publisher: ` + html.EscapeString(publicPublisher()) + ` · Last updated: August 16, 2026</div></section><div class="content">` + cards.String() + `</div><footer class="footer"><span>© CodeLocal</span><span><a href="/">Home</a> · <a href="/support">Support</a></span></footer></main></body></html>`
}

func writePublicPage(w http.ResponseWriter, spec publicPageSpec) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = io.WriteString(w, publicPage(spec))
}

func (s *Server) privacyPage(w http.ResponseWriter, _ *http.Request) {
	writePublicPage(w, publicPageSpec{Title: "Privacy Policy", Eyebrow: "Data handling", Summary: "CodeLocal is designed to keep raw development data and execution on the user's machine while storing only the cloud data needed for identity, routing and durable Project Brain features.", Sections: privacySections()})
}

func privacySections() []publicPageSection {
	return []publicPageSection{
		{"What stays on your machine", `<p>Raw source code, project secrets, local credentials, terminal execution context, approval tokens and machine-specific runtime state are intended to remain on the paired computer unless a user explicitly asks a connected tool to send data elsewhere.</p>`},
		{"What CodeLocal Cloud stores", `<p>Cloud services may store account identity, paired-device and workspace routing metadata, sanitized durable Project Brain knowledge, verified Experience or portable-safe skill metadata, consent/preferences, usage counters and security/audit metadata needed to operate the service.</p>`},
		{"Project Brain and collective features", `<p>Durable Project Brain records are scoped to the authenticated user and logical project. Collective Intelligence is separately gated, disabled by default, and accepts only privacy-eligible structured/de-identified contributions when a user opts in. Raw cross-user project knowledge is not exposed to other users.</p>`},
		{"Retention and deletion", `<p>Account, workspace and Project Brain records are retained while needed to provide the service or until they are deleted, revoked or a deletion request is completed. CodeLocal does not promise automatic expiry for durable Project Brain records. Operational logs are designed to avoid raw source code, secrets and prompt text; infrastructure log retention follows the configured production hosting policy.</p><p>To request deletion or a copy of Cloud-stored personal data, contact <strong>` + html.EscapeString(publicSupportEmail()) + `</strong>. Revoking a workspace or device does not delete files from the user's computer.</p>`},
		{"Third parties and model providers", `<p>CodeLocal is model-provider neutral and does not require its own model API key for the standard MCP workflow. Content sent by an AI client to ChatGPT, Claude or another model/provider is governed by that product and the user's selected account/workspace settings. The OpenAI Plugin is one distribution channel for CodeLocal; locally installed MCP extensions and websites opened through browser automation are separate third parties with their own policies.</p>`},
	}
}

func (s *Server) termsPage(w http.ResponseWriter, _ *http.Request) {
	writePublicPage(w, publicPageSpec{Title: "Terms of Use", Eyebrow: "Service terms", Summary: "These terms describe the responsibilities and boundaries for using CodeLocal with authorized projects, local tools and AI agents.", Sections: termsSections()})
}

func termsSections() []publicPageSection {
	return []publicPageSection{
		{"Authorized use", `<p>You may use CodeLocal only with computers, repositories, accounts and services you own or are authorized to access. You are responsible for the instructions you give an AI model and for reviewing consequential changes before they are applied.</p>`},
		{"Local actions and approvals", `<p>CodeLocal provides permission and approval controls for sensitive operations, but no software safeguard replaces user judgment. Destructive, external or irreversible operations may require explicit confirmation and can still have consequences after approval.</p>`},
		{"Your content", `<p>You retain your rights in source code and project content. You grant CodeLocal only the permissions reasonably necessary to provide the requested service, including storing sanitized durable Project Brain records when those features are used.</p>`},
		{"Service availability", `<p>CodeLocal is provided on an as-available basis. Features may change as security, platform and MCP requirements evolve. Beta features may be modified, disabled or withdrawn without the stability guarantees of a stable release.</p>`},
		{"Acceptable use", `<p>Do not use CodeLocal to violate law, third-party rights, OpenAI policies, service-provider rules or access controls. Do not use it to introduce malware, steal credentials, bypass authorization or conceal harmful activity.</p>`},
	}
}

func (s *Server) supportPage(w http.ResponseWriter, _ *http.Request) {
	writePublicPage(w, publicPageSpec{Title: "Support", Eyebrow: "Help & diagnostics", Summary: "Get help with installation, pairing, workspace access, plugin connections, update mismatches and Project Brain behavior.", Sections: supportSections()})
}

func supportSections() []publicPageSection {
	return []publicPageSection{
		{"Contact", `<p>Email <strong>` + html.EscapeString(publicSupportEmail()) + `</strong>. Include the CodeLocal version, protocol version, runtime status and a correlation/request ID when available. Do not send passwords, OAuth tokens, approval tokens, .env files or private source code.</p>`},
		{"First checks", `<ul><li>Run <code>codelocal --version</code> and confirm the expected release channel.</li><li>Run <code>codelocal</code> and confirm the paired runtime is online.</li><li>Run <code>codelocal .</code> only inside a project you intend to authorize.</li><li>If an AI client shows an old tool surface, reconnect or refresh CodeLocal in that client after updating the runtime.</li></ul>`},
		{"Security or privacy issue", `<p>For suspected vulnerabilities or security-sensitive reports, use <strong>` + html.EscapeString(publicSecurityEmail()) + `</strong> instead of public issue trackers.</p>`},
	}
}

func (s *Server) securityPage(w http.ResponseWriter, _ *http.Request) {
	writePublicPage(w, publicPageSpec{Title: "Security", Eyebrow: "Trust boundary", Summary: "CodeLocal separates AI reasoning, cloud coordination and local execution so that access remains scoped to explicitly authorized workspaces and guarded actions.", Sections: securitySections()})
}

func securitySections() []publicPageSection {
	return []publicPageSection{
		{"Execution boundary", `<p>Filesystem, Git, terminal, browser and desktop execution occur through a paired CodeLocal runtime. Workspaces must be explicitly authorized. The Cloud gateway routes authenticated requests but does not turn the entire computer into a remotely browsable filesystem.</p>`},
		{"Approval model", `<p>Potentially destructive, open-world or otherwise sensitive operations are classified by policy and may require user confirmation. Approval tokens are scoped and must not be treated as reusable credentials.</p>`},
		{"MCP metadata", `<p>The public plugin exposes a compact MCP tool surface with explicit read-only, destructive and open-world annotations. CodeLocal treats annotations as review metadata; local security policy and user approval remain authoritative.</p>`},
		{"Report a vulnerability", `<p>Send security reports to <strong>` + html.EscapeString(publicSecurityEmail()) + `</strong>. Include reproduction steps and impact, but never include real user credentials or unrelated private data.</p>`},
	}
}

func (s *Server) openAIAppsChallenge(w http.ResponseWriter, _ *http.Request) {
	token := strings.TrimSpace(os.Getenv("OPENAI_APPS_CHALLENGE_TOKEN"))
	if token == "" {
		http.NotFound(w, nil)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, token)
}
