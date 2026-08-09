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
:root{
  color-scheme:dark;
  --bg:#090a0d;--bg2:#0d0f13;--surface:#11141a;--surface2:#161a21;--surface3:#1b2029;
  --line:#242a35;--line-soft:#1c212a;--text:#f7f8fb;--muted:#929bad;--muted2:#6f7889;
  --accent:#8b7cff;--accent2:#5ca9ff;--green:#50d890;--red:#ff6978;--yellow:#f2bd57;--cyan:#62d8f4;
  --shadow:0 24px 70px rgba(0,0,0,.32);--radius:16px;--radius-sm:11px
}
*{box-sizing:border-box}html,body{max-width:100%;overflow-x:hidden}html{background:var(--bg)}
body{margin:0;color:var(--text);font-family:Inter,ui-sans-serif,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;line-height:1.45;background:
 radial-gradient(900px 520px at 8% -14%,rgba(139,124,255,.16),transparent 60%),
 radial-gradient(760px 500px at 100% 8%,rgba(92,169,255,.08),transparent 62%),var(--bg)}
button,input,select,textarea{font:inherit}a{color:inherit;text-decoration:none}code,.mono{font-family:"SFMono-Regular",Consolas,"Liberation Mono",monospace}
.shell{min-height:100vh;width:100%;display:grid;grid-template-columns:248px minmax(0,1fr)}
.sidebar{position:sticky;top:0;height:100vh;min-width:0;padding:20px 14px;border-right:1px solid var(--line-soft);background:rgba(9,10,13,.84);backdrop-filter:blur(24px);display:flex;flex-direction:column}
.brand{display:flex;align-items:center;gap:11px;padding:5px 8px 21px}.logo{width:31px;height:31px;flex:0 0 31px;display:grid;place-items:center;border-radius:10px;background:linear-gradient(145deg,var(--accent),var(--accent2));box-shadow:0 10px 28px rgba(112,126,255,.25);font-weight:900;font-size:14px}.brand-name{font-size:15px;font-weight:780;letter-spacing:-.025em}.brand-tag{margin-left:auto;border:1px solid var(--line);background:var(--surface);color:var(--muted);font-size:9px;font-weight:800;letter-spacing:.08em;padding:4px 6px;border-radius:999px}
.nav-label{padding:10px 10px 6px;color:var(--muted2);font-size:10px;font-weight:800;text-transform:uppercase;letter-spacing:.12em}.nav{display:grid;gap:3px}.nav a{display:flex;align-items:center;gap:10px;padding:9px 10px;border-radius:10px;color:var(--muted);font-size:13px;font-weight:600;transition:.15s ease}.nav a:hover{background:rgba(255,255,255,.035);color:var(--text)}.nav a.active{background:linear-gradient(90deg,rgba(139,124,255,.13),rgba(92,169,255,.06));box-shadow:inset 0 0 0 1px rgba(139,124,255,.13);color:#fff}.nav-icon{width:19px;height:19px;display:grid;place-items:center;color:inherit;font-size:13px}.nav a.active .nav-icon{color:#b8afff}
.sidebar-spacer{flex:1}.sidebar-status{margin:14px 7px;padding:12px;border:1px solid var(--line-soft);border-radius:12px;background:rgba(17,20,26,.72)}.sidebar-status-title{display:flex;align-items:center;gap:8px;font-size:11px;font-weight:700}.status-dot{display:inline-block;width:7px;height:7px;border-radius:50%;background:var(--muted2);box-shadow:0 0 0 3px rgba(146,155,173,.06)}.status-dot.green{background:var(--green);box-shadow:0 0 0 3px rgba(80,216,144,.09),0 0 14px rgba(80,216,144,.2)}.sidebar-status-copy{color:var(--muted2);font-size:10px;margin-top:5px;line-height:1.45}.sidebar-foot{padding:12px 8px 2px;border-top:1px solid var(--line-soft);display:flex;align-items:center;gap:9px}.avatar{width:28px;height:28px;border-radius:9px;display:grid;place-items:center;background:var(--surface3);border:1px solid var(--line);font-size:11px;font-weight:800}.account{min-width:0;flex:1}.account-email{white-space:nowrap;overflow:hidden;text-overflow:ellipsis;font-size:11px;font-weight:650}.account-meta{font-size:10px;color:var(--muted2)}
.main{min-width:0;width:100%;max-width:1320px;margin:0 auto;padding:38px 44px 64px}.top{display:flex;justify-content:space-between;align-items:flex-start;gap:22px;margin-bottom:28px}.top-copy{min-width:0}.eyebrow{display:flex;align-items:center;gap:7px;color:var(--muted2);font-size:10px;font-weight:800;text-transform:uppercase;letter-spacing:.13em}.h1{font-size:31px;font-weight:790;letter-spacing:-.04em;margin:7px 0 4px}.sub{max-width:760px;color:var(--muted);font-size:13px;line-height:1.6}.top-actions{display:flex;align-items:center;gap:9px;flex-wrap:wrap}
.grid{min-width:0;display:grid;grid-template-columns:repeat(12,minmax(0,1fr));gap:15px}.span3{grid-column:span 3}.span4{grid-column:span 4}.span5{grid-column:span 5}.span6{grid-column:span 6}.span7{grid-column:span 7}.span8{grid-column:span 8}.span12{grid-column:span 12}
.card{min-width:0;border:1px solid var(--line);border-radius:var(--radius);background:linear-gradient(180deg,rgba(20,24,31,.95),rgba(15,18,23,.96));box-shadow:var(--shadow);padding:20px}.card.flat{box-shadow:none}.card.glow{background:linear-gradient(160deg,rgba(139,124,255,.12),rgba(20,24,31,.95) 48%,rgba(15,18,23,.97));border-color:rgba(139,124,255,.2)}
.metric-card{position:relative;overflow:hidden;min-height:118px}.metric-card:after{content:"";position:absolute;width:90px;height:90px;border-radius:50%;right:-36px;top:-38px;background:radial-gradient(circle,rgba(139,124,255,.15),transparent 70%)}.metric-label{color:var(--muted);font-size:11px;font-weight:700}.metric{font-size:31px;font-weight:800;letter-spacing:-.045em;margin-top:12px}.metric-sub{margin-top:3px;color:var(--muted2);font-size:10px}.metric-state{font-size:18px;letter-spacing:-.02em;margin-top:15px}
.title{font-size:15px;font-weight:730;letter-spacing:-.015em}.label{color:var(--muted);font-size:12px;line-height:1.55}.section-head{display:flex;align-items:flex-start;justify-content:space-between;gap:16px;margin-bottom:12px}.section-head .label{margin-top:4px}.divider{height:1px;background:var(--line-soft);margin:16px 0}
.list{display:grid}.row{min-width:0;display:flex;align-items:center;justify-content:space-between;gap:14px;padding:14px 0;border-bottom:1px solid var(--line-soft)}.row:first-child{padding-top:4px}.row:last-child{border-bottom:0;padding-bottom:4px}.row-main{min-width:0;flex:1}.row-title{display:flex;align-items:center;gap:8px;min-width:0;font-size:13px;font-weight:680}.row-title-text{white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.row-meta{color:var(--muted2);font-size:10.5px;margin-top:4px;overflow-wrap:anywhere;word-break:break-word;line-height:1.5}.entity{display:flex;align-items:center;gap:11px;min-width:0}.entity-icon{width:34px;height:34px;flex:0 0 34px;border-radius:10px;display:grid;place-items:center;background:var(--surface3);border:1px solid var(--line);font-size:13px;color:#c6cdfd}.entity-copy{min-width:0}
.badge{display:inline-flex;align-items:center;gap:6px;flex:0 0 auto;max-width:100%;border:1px solid var(--line);border-radius:999px;padding:5px 8px;color:var(--muted);font-size:9.5px;font-weight:780;white-space:nowrap}.badge:before{content:"";width:5px;height:5px;border-radius:50%;background:currentColor;opacity:.72}.badge.green{color:#75e6aa;background:rgba(80,216,144,.07);border-color:rgba(80,216,144,.18)}.badge.red{color:#ff929c;background:rgba(255,105,120,.07);border-color:rgba(255,105,120,.18)}.badge.blue{color:#a9b8ff;background:rgba(139,124,255,.08);border-color:rgba(139,124,255,.2)}.badge.yellow{color:#ffd37e;background:rgba(242,189,87,.07);border-color:rgba(242,189,87,.18)}.badge.muted{color:var(--muted);background:rgba(146,155,173,.04)}
.actions{display:flex;align-items:center;gap:7px;flex-wrap:wrap;flex:0 0 auto}.btn{appearance:none;border:1px solid var(--line);background:linear-gradient(180deg,var(--surface3),var(--surface2));color:var(--text);min-height:36px;padding:8px 12px;border-radius:10px;font-size:11px;font-weight:700;cursor:pointer;display:inline-flex;align-items:center;justify-content:center;gap:7px;transition:.15s ease}.btn:hover{transform:translateY(-1px);border-color:#343b48;filter:brightness(1.06)}.btn:active{transform:translateY(0)}.btn[disabled]{opacity:.38;cursor:not-allowed;transform:none;filter:none}.btn.primary{border-color:transparent;background:linear-gradient(135deg,#8879ff,#5b9fff);box-shadow:0 8px 24px rgba(105,117,255,.2)}.btn.danger{color:#ff9aa4;border-color:rgba(255,105,120,.2);background:rgba(255,105,120,.055)}.btn.ghost{background:transparent}.btn.small{min-height:31px;padding:6px 9px;font-size:10px}.btn-icon{font-size:12px;opacity:.9}
form{min-width:0;margin:0}.form{display:grid;gap:13px}.field{display:grid;gap:6px}.field label{color:var(--muted);font-size:10.5px;font-weight:700}.input,.select,.textarea{width:100%;min-width:0;max-width:100%;border:1px solid var(--line);background:#0c0f14;color:var(--text);border-radius:10px;padding:10px 11px;outline:none;font-size:12px}.input:focus,.select:focus,.textarea:focus{border-color:rgba(139,124,255,.72);box-shadow:0 0 0 3px rgba(139,124,255,.08)}.textarea{min-height:88px;resize:vertical}.hint{color:var(--muted2);font-size:10px;line-height:1.5}
.alert{border:1px solid rgba(255,105,120,.2);background:rgba(255,105,120,.06);color:#ffb1b9;border-radius:11px;padding:10px 12px;font-size:11px;margin-bottom:14px}.alert.success{border-color:rgba(80,216,144,.2);background:rgba(80,216,144,.06);color:#95ebbd}.empty{padding:38px 16px;text-align:center;color:var(--muted2);font-size:12px}.empty-icon{width:42px;height:42px;border-radius:12px;display:grid;place-items:center;margin:0 auto 11px;background:var(--surface3);border:1px solid var(--line);color:var(--muted)}
code,.code{background:#0b0d11;border:1px solid var(--line-soft);border-radius:7px;padding:2px 5px;font-size:10.5px;overflow-wrap:anywhere}.code-block{position:relative;max-width:100%;background:#0b0e13;border:1px solid var(--line);border-radius:12px;padding:14px;color:#dce3f6;font-size:11px;white-space:pre-wrap;overflow:auto;overflow-wrap:anywhere;word-break:break-word}.copy-row{display:flex;align-items:center;gap:9px}.copy-row .code-block{flex:1}.kv{display:grid;grid-template-columns:140px minmax(0,1fr);gap:10px;padding:10px 0;border-bottom:1px solid var(--line-soft);font-size:11px}.kv:last-child{border-bottom:0}.kv-key{color:var(--muted2)}.kv-value{min-width:0;overflow-wrap:anywhere}
.activity{display:flex;gap:11px;padding:11px 0;border-bottom:1px solid var(--line-soft)}.activity:last-child{border-bottom:0}.activity-icon{width:27px;height:27px;flex:0 0 27px;border:1px solid var(--line);background:var(--surface3);border-radius:9px;display:grid;place-items:center;color:#aeb7ca;font-size:10px}.activity-title{font-size:11px;font-weight:660}.activity-meta{font-size:9.5px;color:var(--muted2);margin-top:3px}
.auth-wrap{min-height:100vh;display:grid;place-items:center;padding:24px}.auth{width:min(430px,100%)}.auth .brand{justify-content:center;padding-bottom:18px}.auth .brand-tag{display:none}.auth .card{padding:27px}.auth h1{font-size:25px;letter-spacing:-.035em;margin:0 0 6px}.auth p{color:var(--muted);font-size:12px;margin:0 0 22px;line-height:1.6}.auth-switch{text-align:center;color:var(--muted);font-size:11px;margin-top:16px}.auth-switch a{color:#b4adff}.muted{color:var(--muted)}
dialog{width:min(440px,calc(100vw - 30px));padding:0;border:1px solid var(--line);border-radius:16px;background:#11151b;color:var(--text);box-shadow:0 32px 100px rgba(0,0,0,.55)}dialog::backdrop{background:rgba(3,5,8,.72);backdrop-filter:blur(5px)}.modal{padding:22px}.modal-icon{width:38px;height:38px;display:grid;place-items:center;border-radius:11px;background:rgba(255,105,120,.08);border:1px solid rgba(255,105,120,.18);color:#ff8e99;margin-bottom:14px}.modal-title{font-size:17px;font-weight:760;letter-spacing:-.02em}.modal-copy{color:var(--muted);font-size:11.5px;line-height:1.65;margin-top:7px}.modal-actions{display:flex;justify-content:flex-end;gap:8px;margin-top:19px}
@media(max-width:980px){.shell{grid-template-columns:218px minmax(0,1fr)}.main{padding:32px 28px 56px}.span3{grid-column:span 6}.span4,.span5,.span6,.span7,.span8{grid-column:span 12}}
@media(max-width:760px){.shell{display:block}.sidebar{position:relative;height:auto;border-right:0;border-bottom:1px solid var(--line-soft);padding:12px}.brand{padding:3px 6px 11px}.nav-label,.sidebar-status,.sidebar-foot{display:none}.nav{display:flex;overflow-x:auto;gap:5px}.nav a{white-space:nowrap;flex:0 0 auto;padding:8px 10px}.main{padding:24px 15px 46px}.top{flex-direction:column;margin-bottom:20px}.top-actions{width:100%}.h1{font-size:27px}.span3,.span4,.span5,.span6,.span7,.span8{grid-column:span 12}.row{align-items:flex-start;flex-wrap:wrap}.actions{width:100%;justify-content:flex-start}.section-head{flex-direction:column}.copy-row{align-items:stretch;flex-direction:column}.kv{grid-template-columns:1fr;gap:3px}.auth-wrap{padding:14px}.auth .card{padding:22px 17px}}
`;

const dashboardScript = `
(() => {
  const dialog = document.getElementById('confirm-dialog');
  const title = document.getElementById('confirm-title');
  const copy = document.getElementById('confirm-copy');
  const confirm = document.getElementById('confirm-submit');
  let pendingForm = null;
  document.addEventListener('click', (event) => {
    const button = event.target.closest('[data-confirm]');
    if (!button || !dialog) return;
    event.preventDefault();
    pendingForm = button.closest('form');
    title.textContent = button.dataset.confirmTitle || 'Are you sure?';
    copy.textContent = button.dataset.confirmMessage || 'This action cannot be undone.';
    confirm.textContent = button.dataset.confirmLabel || 'Continue';
    confirm.className = 'btn ' + (button.dataset.confirmTone === 'danger' ? 'danger' : 'primary');
    dialog.showModal();
  });
  confirm?.addEventListener('click', () => {
    if (!pendingForm) return dialog?.close();
    const form = pendingForm;
    pendingForm = null;
    dialog?.close();
    form.requestSubmit();
  });
  document.querySelectorAll('[data-copy-target]').forEach((button) => {
    button.addEventListener('click', async () => {
      const target = document.querySelector(button.dataset.copyTarget);
      if (!target) return;
      const value = target.textContent || '';
      try {
        await navigator.clipboard.writeText(value.trim());
        const previous = button.textContent;
        button.textContent = 'Copied';
        setTimeout(() => button.textContent = previous, 1400);
      } catch {}
    });
  });
})();`;

export function authPage(options: { title: string; subtitle: string; body: string }) {
  return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="dark"><title>${escapeHtml(options.title)} · CodeLocal</title><style>${styles}</style></head><body><div class="auth-wrap"><div class="auth"><div class="brand"><span class="logo">◆</span><span class="brand-name">CodeLocal</span></div><div class="card glow"><h1>${escapeHtml(options.title)}</h1><p>${escapeHtml(options.subtitle)}</p>${options.body}</div></div></div></body></html>`;
}

export function dashboardPage(options: {
  title: string;
  eyebrow?: string;
  subtitle?: string;
  active: "overview" | "devices" | "workspaces" | "connect" | "security";
  email: string;
  csrf: string;
  body: string;
  actions?: string;
}) {
  const nav = [
    ["overview", "/dashboard", "⌂", "Overview"],
    ["workspaces", "/dashboard/workspaces", "◇", "Workspaces"],
    ["devices", "/dashboard/devices", "◉", "Devices"],
    ["connect", "/dashboard/connect", "↗", "Connect ChatGPT"],
    ["security", "/dashboard/security", "⌁", "Security"],
  ] as const;
  const initial = options.email.trim().slice(0, 1).toUpperCase() || "U";
  return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="dark"><title>${escapeHtml(options.title)} · CodeLocal</title><style>${styles}</style></head><body><div class="shell"><aside class="sidebar"><div class="brand"><span class="logo">◆</span><span class="brand-name">CodeLocal</span><span class="brand-tag">CLOUD</span></div><div class="nav-label">Control plane</div><nav class="nav">${nav.map(([key, href, icon, label]) => `<a class="${options.active === key ? "active" : ""}" href="${href}"><span class="nav-icon">${icon}</span><span>${label}</span></a>`).join("")}</nav><div class="sidebar-spacer"></div><div class="sidebar-status"><div class="sidebar-status-title"><span class="status-dot green"></span>Cloud connected</div><div class="sidebar-status-copy">Local source code and secrets stay on your machine.</div></div><div class="sidebar-foot"><div class="avatar">${escapeHtml(initial)}</div><div class="account"><div class="account-email">${escapeHtml(options.email)}</div><div class="account-meta">CodeLocal account</div></div><form method="post" action="/logout"><input type="hidden" name="csrf" value="${escapeHtml(options.csrf)}"><button class="btn ghost small" type="submit">Sign out</button></form></div></aside><main class="main"><header class="top"><div class="top-copy"><div class="eyebrow"><span class="status-dot green"></span>${escapeHtml(options.eyebrow ?? "CodeLocal Cloud")}</div><div class="h1">${escapeHtml(options.title)}</div>${options.subtitle ? `<div class="sub">${escapeHtml(options.subtitle)}</div>` : ""}</div>${options.actions ? `<div class="top-actions">${options.actions}</div>` : ""}</header>${options.body}</main></div><dialog id="confirm-dialog"><div class="modal"><div class="modal-icon">!</div><div class="modal-title" id="confirm-title">Are you sure?</div><div class="modal-copy" id="confirm-copy">This action cannot be undone.</div><div class="modal-actions"><button class="btn" type="button" onclick="this.closest('dialog').close()">Cancel</button><button class="btn danger" id="confirm-submit" type="button">Continue</button></div></div></dialog><script>${dashboardScript}</script></body></html>`;
}
