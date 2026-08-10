package ui

import (
	"html/template"
	"strings"
)

func Escape(value string) string { return template.HTMLEscapeString(value) }

func Page(title, subtitle, body string) string {
	return `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>` + Escape(title) + ` · CodeLocal</title><style>
:root{color-scheme:light dark;--bg:#0b0d10;--panel:#12161b;--soft:#1a2028;--text:#f5f7fa;--muted:#9aa4b2;--line:#29313c;--accent:#72a7ff;--danger:#ff6b6b}*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--text);font:15px/1.55 ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}.shell{max-width:1040px;margin:0 auto;padding:64px 20px}.brand{display:flex;align-items:center;gap:10px;font-weight:800;margin-bottom:32px}.dot{width:11px;height:11px;border-radius:50%;background:#52d273;box-shadow:0 0 18px #52d27366}.card{background:var(--panel);border:1px solid var(--line);border-radius:18px;padding:28px;box-shadow:0 20px 60px #0005}h1{margin:0 0 8px;font-size:30px;line-height:1.2}.sub{color:var(--muted);margin-bottom:24px}.form{display:grid;gap:14px}.field{display:grid;gap:6px}.input{width:100%;padding:12px 13px;border:1px solid var(--line);border-radius:10px;background:var(--soft);color:var(--text);font:inherit}.btn{display:inline-flex;justify-content:center;align-items:center;gap:8px;padding:11px 15px;border-radius:10px;border:1px solid var(--line);background:var(--soft);color:var(--text);text-decoration:none;font-weight:700;cursor:pointer}.btn.primary{background:var(--accent);border-color:var(--accent);color:#07111f}.hint,.row-meta{color:var(--muted);font-size:13px}.alert{padding:11px 13px;border:1px solid #ff6b6b66;background:#ff6b6b14;color:#ffb3b3;border-radius:10px}.auth-switch{margin-top:18px;color:var(--muted)}a{color:#9ec2ff}.stack{display:grid;gap:12px}.row{padding:14px;border:1px solid var(--line);border-radius:12px;background:var(--soft)}.row-title{font-weight:700}.mono{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;word-break:break-all}.actions{display:flex;gap:10px;flex-wrap:wrap}.metrics{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:12px}.metric-box{padding:16px;border:1px solid var(--line);border-radius:12px;background:var(--soft)}.metric-label{color:var(--muted);font-size:12px}.metric-value{font-size:28px;font-weight:800;margin-top:4px}.badge{display:inline-flex;align-items:center;padding:3px 8px;border-radius:999px;border:1px solid var(--line);font-size:11px;font-weight:700;vertical-align:middle}.badge.green{color:#7be0a8;border-color:#2e6f4a;background:#173323}.badge.blue{color:#9fc5ff;border-color:#315a8b;background:#17283c}.badge.muted{color:var(--muted);background:#15191f}.tree{display:grid;gap:8px;margin-top:12px}.tree-node{position:relative;padding:10px 12px;border:1px solid var(--line);border-radius:10px;background:var(--soft)}.tree-node:before{content:"";position:absolute;left:-12px;top:50%;width:10px;border-top:1px solid var(--line)}@media(max-width:720px){.metrics{grid-template-columns:repeat(2,minmax(0,1fr))}.tree-node{margin-left:0!important}}@media(max-width:520px){.shell{padding:28px 14px}.card{padding:20px}h1{font-size:26px}.metrics{grid-template-columns:1fr}}
</style></head><body><main class="shell"><div class="brand"><span class="dot"></span>CodeLocal</div><section class="card"><h1>` + Escape(title) + `</h1><div class="sub">` + Escape(subtitle) + `</div>` + body + `</section></main></body></html>`
}

func Hidden(fields map[string]string) string {
	var out strings.Builder
	for key, value := range fields {
		out.WriteString(`<input type="hidden" name="` + Escape(key) + `" value="` + Escape(value) + `">`)
	}
	return out.String()
}
