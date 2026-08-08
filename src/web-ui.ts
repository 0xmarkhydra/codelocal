export function escapeHtml(value: unknown) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

export function formatTime(value?: number) {
  if (!value) return "—";
  return new Intl.DateTimeFormat("en", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}

const styles = `
:root{color-scheme:dark;--bg:#08090b;--panel:#111318;--panel2:#171a20;--line:#262a33;--text:#f5f7fb;--muted:#9299a8;--accent:#6c8cff;--accent2:#8e6cff;--green:#47d18c;--red:#ff6470;--yellow:#f5bd4f;--shadow:0 18px 60px rgba(0,0,0,.28)}
*{box-sizing:border-box}body{margin:0;background:radial-gradient(900px 500px at 15% -10%,rgba(108,140,255,.14),transparent 60%),var(--bg);color:var(--text);font-family:Inter,ui-sans-serif,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;line-height:1.45}
a{color:inherit;text-decoration:none}.shell{min-height:100vh;display:grid;grid-template-columns:240px 1fr}.sidebar{border-right:1px solid var(--line);padding:24px 16px;position:sticky;top:0;height:100vh;background:rgba(8,9,11,.82);backdrop-filter:blur(20px)}
.brand{display:flex;gap:11px;align-items:center;font-weight:750;letter-spacing:-.02em;padding:0 10px 24px}.logo{width:30px;height:30px;border-radius:9px;background:linear-gradient(135deg,var(--accent),var(--accent2));display:grid;place-items:center;box-shadow:0 8px 24px rgba(108,140,255,.28);font-size:14px}.nav{display:grid;gap:5px}.nav a{padding:10px 12px;border-radius:9px;color:var(--muted);font-size:14px}.nav a:hover,.nav a.active{background:var(--panel2);color:var(--text)}
.sidebar-foot{position:absolute;bottom:18px;left:16px;right:16px;border-top:1px solid var(--line);padding-top:14px;color:var(--muted);font-size:12px}.main{padding:38px 44px;max-width:1320px;width:100%;margin:0 auto}.top{display:flex;justify-content:space-between;align-items:flex-start;gap:20px;margin-bottom:28px}.eyebrow{font-size:12px;text-transform:uppercase;letter-spacing:.12em;color:var(--muted);font-weight:700}.h1{font-size:30px;font-weight:750;letter-spacing:-.035em;margin:5px 0}.sub{color:var(--muted);max-width:720px}.grid{display:grid;grid-template-columns:repeat(12,1fr);gap:16px}.card{background:linear-gradient(180deg,rgba(23,26,32,.94),rgba(17,19,24,.94));border:1px solid var(--line);border-radius:15px;padding:20px;box-shadow:var(--shadow)}.span3{grid-column:span 3}.span4{grid-column:span 4}.span6{grid-column:span 6}.span8{grid-column:span 8}.span12{grid-column:span 12}.metric{font-size:30px;font-weight:760;letter-spacing:-.04em;margin-top:9px}.label{color:var(--muted);font-size:13px}.title{font-weight:700;font-size:16px;margin-bottom:4px}.row{display:flex;align-items:center;justify-content:space-between;gap:16px;padding:14px 0;border-bottom:1px solid var(--line)}.row:last-child{border-bottom:0}.row-main{min-width:0}.row-title{font-weight:650;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.row-meta{font-size:12px;color:var(--muted);margin-top:3px;word-break:break-word}
.badge{font-size:11px;font-weight:700;padding:5px 8px;border-radius:999px;border:1px solid var(--line);color:var(--muted)}.badge.green{color:var(--green);background:rgba(71,209,140,.08);border-color:rgba(71,209,140,.2)}.badge.red{color:var(--red);background:rgba(255,100,112,.08);border-color:rgba(255,100,112,.2)}.badge.blue{color:#9eb2ff;background:rgba(108,140,255,.1);border-color:rgba(108,140,255,.22)}
.btn{display:inline-flex;align-items:center;justify-content:center;border:1px solid var(--line);background:var(--panel2);color:var(--text);height:38px;padding:0 14px;border-radius:9px;font-weight:650;font-size:13px;cursor:pointer}.btn:hover{filter:brightness(1.12)}.btn.primary{border-color:transparent;background:linear-gradient(135deg,var(--accent),var(--accent2))}.btn.danger{color:#ff8891;border-color:rgba(255,100,112,.25);background:rgba(255,100,112,.06)}.btn.small{height:32px;padding:0 10px;font-size:12px}.actions{display:flex;gap:8px;align-items:center;flex-wrap:wrap}
.form{display:grid;gap:14px}.field{display:grid;gap:7px}.field label{font-size:12px;color:var(--muted);font-weight:650}.input,.select,.textarea{width:100%;border:1px solid var(--line);background:#0d0f13;color:var(--text);border-radius:9px;padding:11px 12px;outline:none;font:inherit}.textarea{min-height:90px;resize:vertical}.input:focus,.select:focus,.textarea:focus{border-color:var(--accent)}.hint{font-size:12px;color:var(--muted)}.alert{border:1px solid rgba(255,100,112,.25);background:rgba(255,100,112,.07);color:#ffb0b6;border-radius:10px;padding:10px 12px;font-size:13px}.success{border-color:rgba(71,209,140,.25);background:rgba(71,209,140,.07);color:#8ce9b8}.empty{padding:34px 12px;text-align:center;color:var(--muted)}code,.code{font-family:"SFMono-Regular",Consolas,monospace;background:#0b0d11;border:1px solid var(--line);border-radius:8px;padding:2px 6px;font-size:12px}.code-block{font-family:"SFMono-Regular",Consolas,monospace;background:#0b0d11;border:1px solid var(--line);border-radius:10px;padding:14px;overflow:auto;color:#d8def0;font-size:12px;white-space:pre-wrap}.divider{height:1px;background:var(--line);margin:18px 0}
.auth-wrap{min-height:100vh;display:grid;place-items:center;padding:24px}.auth{width:min(440px,100%)}.auth-brand{justify-content:center;padding-bottom:22px}.auth .card{padding:28px}.auth h1{font-size:25px;letter-spacing:-.03em;margin:0 0 6px}.auth p{color:var(--muted);margin:0 0 22px}.auth-switch{text-align:center;color:var(--muted);font-size:13px;margin-top:18px}.auth-switch a{color:#aebcff}.mono{font-family:"SFMono-Regular",Consolas,monospace}.muted{color:var(--muted)}
@media(max-width:900px){.shell{grid-template-columns:1fr}.sidebar{height:auto;position:static;border-right:0;border-bottom:1px solid var(--line)}.sidebar-foot{display:none}.nav{display:flex;overflow:auto}.brand{padding-bottom:14px}.main{padding:26px 18px}.span3,.span4,.span6,.span8{grid-column:span 12}.top{flex-direction:column}}
`;

export function authPage(options: { title: string; subtitle: string; body: string }) {
  return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="dark"><title>${escapeHtml(options.title)} · CodeLocal</title><style>${styles}</style></head><body><div class="auth-wrap"><div class="auth"><div class="brand auth-brand"><span class="logo">C</span><span>CodeLocal</span></div><div class="card"><h1>${escapeHtml(options.title)}</h1><p>${escapeHtml(options.subtitle)}</p>${options.body}</div></div></div></body></html>`;
}

export function dashboardPage(options: {
  title: string;
  eyebrow?: string;
  subtitle?: string;
  active: "overview" | "devices" | "workspaces" | "mcp" | "connect" | "security";
  email: string;
  csrf: string;
  body: string;
  actions?: string;
}) {
  const nav = [
    ["overview", "/dashboard", "Overview"],
    ["devices", "/dashboard/devices", "Devices"],
    ["workspaces", "/dashboard/workspaces", "Workspaces"],
    ["mcp", "/dashboard/mcp", "MCP Extensions"],
    ["connect", "/dashboard/connect", "Connect ChatGPT"],
    ["security", "/dashboard/security", "Security"],
  ] as const;
  return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="dark"><title>${escapeHtml(options.title)} · CodeLocal</title><style>${styles}</style></head><body><div class="shell"><aside class="sidebar"><div class="brand"><span class="logo">C</span><span>CodeLocal</span></div><nav class="nav">${nav.map(([key, href, label]) => `<a class="${options.active === key ? "active" : ""}" href="${href}">${label}</a>`).join("")}</nav><div class="sidebar-foot"><div>${escapeHtml(options.email)}</div><form method="post" action="/logout" style="margin-top:8px"><input type="hidden" name="csrf" value="${escapeHtml(options.csrf)}"><button class="btn small" type="submit">Sign out</button></form></div></aside><main class="main"><header class="top"><div><div class="eyebrow">${escapeHtml(options.eyebrow ?? "CodeLocal Cloud")}</div><div class="h1">${escapeHtml(options.title)}</div>${options.subtitle ? `<div class="sub">${escapeHtml(options.subtitle)}</div>` : ""}</div>${options.actions ?? ""}</header>${options.body}</main></div></body></html>`;
}
