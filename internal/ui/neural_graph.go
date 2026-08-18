package ui

import (
	"encoding/json"
	"fmt"
	"strings"
)

type NeuralGraphFilter struct {
	Value string
	Label string
}

type NeuralGraphLegend struct {
	Label string
	Color string
}

type NeuralGraphOptions struct {
	Mode              string
	Data              any
	Query             string
	Depth             int
	RemoteSearch      bool
	SearchPlaceholder string
	InspectorLabel    string
	EmptyTitle        string
	EmptyCopy         string
	Help              string
	Filters           []NeuralGraphFilter
	Legend            []NeuralGraphLegend
}

func safeGraphJSON(value any) string {
	raw, _ := json.Marshal(value)
	return strings.ReplaceAll(string(raw), "</", `<\/`)
}

func neuralGraphOptions(options NeuralGraphOptions) map[string]any {
	return map[string]any{
		"mode": options.Mode, "query": options.Query, "depth": options.Depth, "remoteSearch": options.RemoteSearch,
		"inspectorLabel": options.InspectorLabel, "emptyTitle": options.EmptyTitle, "emptyCopy": options.EmptyCopy,
	}
}

func graphFilterOptions(filters []NeuralGraphFilter) string {
	var out strings.Builder
	for _, filter := range filters {
		out.WriteString(`<option value="` + Escape(filter.Value) + `">` + Escape(filter.Label) + `</option>`)
	}
	return out.String()
}

func graphLegend(legend []NeuralGraphLegend) string {
	var out strings.Builder
	for _, item := range legend {
		out.WriteString(`<span><i style="--signal-color:` + Escape(item.Color) + `"></i>` + Escape(item.Label) + `</span>`)
	}
	return out.String()
}

func NeuralGraph(options NeuralGraphOptions) string {
	if options.SearchPlaceholder == "" {
		options.SearchPlaceholder = "Search graph…"
	}
	if options.InspectorLabel == "" {
		options.InspectorLabel = "Graph inspector"
	}
	if options.EmptyTitle == "" {
		options.EmptyTitle = "Nothing to show yet"
	}
	if options.EmptyCopy == "" {
		options.EmptyCopy = "Graph data will appear here when it becomes available."
	}
	if options.Help == "" {
		options.Help = "Drag nodes · pan background · wheel/pinch to zoom · tap for details"
	}
	if options.Depth < 1 {
		options.Depth = 1
	}
	remoteButton := ""
	if options.RemoteSearch {
		remoteButton = `<button id="neural-inspect" class="btn small" type="button">Inspect</button>`
	}
	depth := ""
	if options.Mode == "code" {
		depth = `<select id="neural-depth" class="neural-select" aria-label="Graph depth"><option value="1">Depth 1</option><option value="2">Depth 2</option><option value="3">Depth 3</option></select>`
	}
	return fmt.Sprintf(`
<style>%s</style>
<div class="neural-card" data-neural-mode="%s">
 <div class="neural-toolbar">
  <div class="neural-search-wrap"><input id="neural-search" class="neural-search" type="search" value="%s" placeholder="%s" autocomplete="off" aria-label="Search graph">%s</div>
  <select id="neural-filter" class="neural-select" aria-label="Filter graph">%s</select>
  %s
  <button id="neural-reset" class="btn small" type="button">Reset view</button>
  <div class="neural-legend">%s</div>
 </div>
 <div class="neural-shell">
  <div class="neural-canvas-wrap">
   <canvas id="neural-canvas" class="neural-canvas" role="img" aria-label="Interactive CodeLocal neural graph"></canvas>
   <div class="neural-grid" aria-hidden="true"></div>
   <div id="neural-empty" class="neural-empty" hidden><strong>%s</strong><span>%s</span></div>
   <div class="neural-help"><span class="neural-signal-dot"></span>%s</div>
  </div>
  <aside id="neural-detail" class="neural-detail"></aside>
 </div>
</div>
<script id="neural-data" type="application/json">%s</script>
<script id="neural-config" type="application/json">%s</script>
<script>%s</script>`, neuralGraphCSS, Escape(options.Mode), Escape(options.Query), Escape(options.SearchPlaceholder), remoteButton,
		graphFilterOptions(options.Filters), depth, graphLegend(options.Legend), Escape(options.EmptyTitle), Escape(options.EmptyCopy), Escape(options.Help),
		safeGraphJSON(options.Data), safeGraphJSON(neuralGraphOptions(options)), neuralGraphScript)
}

const neuralGraphCSS = `
.neural-card{overflow:hidden;border:1px solid #182842;border-radius:20px;background:#050912;box-shadow:0 24px 70px rgba(8,18,38,.18)}
.neural-toolbar{display:flex;align-items:center;gap:9px;flex-wrap:wrap;padding:10px 12px;border-bottom:1px solid #17243a;background:#08101c;color:#d9e5f5}.neural-search-wrap{display:flex;gap:7px;flex:1;min-width:250px}.neural-search,.neural-select{height:38px;border:1px solid #20314d;border-radius:10px;background:#0b1524;color:#dce8f8;padding:0 11px;outline:none;font-size:11px}.neural-search{width:100%;min-width:160px}.neural-search::placeholder{color:#657892}.neural-search:focus,.neural-select:focus{border-color:#477ff4;box-shadow:0 0 0 3px rgba(71,127,244,.13)}.neural-select{min-width:120px}.neural-toolbar .btn{border-color:#263851;background:#0d1726;color:#b9c8da}.neural-toolbar .btn:hover{border-color:#3d5270;background:#111d30;color:#fff}.neural-legend{display:flex;align-items:center;gap:10px;flex-wrap:wrap;margin-left:auto;color:#71849e;font-size:9px}.neural-legend span{display:inline-flex;align-items:center;gap:5px}.neural-legend i{width:7px;height:7px;border-radius:50%;background:var(--signal-color);box-shadow:0 0 10px color-mix(in srgb,var(--signal-color) 60%,transparent)}
.neural-shell{display:grid;grid-template-columns:minmax(0,1fr) 330px;height:min(690px,calc(100dvh - 190px));min-height:520px}.neural-canvas-wrap{position:relative;min-width:0;min-height:0;overflow:hidden;background:radial-gradient(circle at 50% 42%,rgba(32,72,145,.13),transparent 40%),radial-gradient(circle at 18% 80%,rgba(104,61,190,.08),transparent 30%),#050912}.neural-grid{position:absolute;inset:0;pointer-events:none;opacity:.42;background-image:linear-gradient(rgba(82,116,170,.035) 1px,transparent 1px),linear-gradient(90deg,rgba(82,116,170,.035) 1px,transparent 1px);background-size:34px 34px;mask-image:radial-gradient(circle at center,#000 25%,transparent 92%)}.neural-canvas{position:relative;z-index:1;display:block;width:100%;height:100%;cursor:grab;touch-action:none}.neural-canvas.dragging{cursor:grabbing}.neural-help{position:absolute;z-index:2;left:13px;bottom:12px;display:flex;align-items:center;gap:7px;padding:7px 9px;border:1px solid #1d2e49;border-radius:9px;background:rgba(6,12,22,.82);backdrop-filter:blur(12px);color:#70839c;font-size:9.5px;pointer-events:none}.neural-signal-dot{width:6px;height:6px;border-radius:50%;background:#54e3b2;box-shadow:0 0 12px rgba(84,227,178,.7)}
.neural-empty{position:absolute;z-index:3;inset:0;display:grid;place-content:center;gap:7px;padding:30px;text-align:center;background:rgba(5,9,18,.80);color:#73869f;pointer-events:none}.neural-empty[hidden]{display:none}.neural-empty strong{color:#e5eef9;font-size:16px}.neural-empty span{max-width:390px;font-size:11px;line-height:1.65}.neural-detail{border-left:1px solid #17243a;overflow:auto;background:linear-gradient(180deg,#08101c,#060b14);color:#dfe9f6}.neural-detail-inner{padding:18px}.neural-detail-head{display:flex;align-items:center;justify-content:space-between;gap:10px;margin-bottom:18px}.neural-eyebrow{color:#62758e;font-size:9px;font-weight:800;text-transform:uppercase;letter-spacing:.13em}.neural-live{display:inline-flex;align-items:center;gap:6px;padding:5px 8px;border:1px solid #1d3343;border-radius:999px;background:#091a1a;color:#72d8b1;font-size:8.5px}.neural-live:before{content:'';width:5px;height:5px;border-radius:50%;background:#53dca9;box-shadow:0 0 10px rgba(83,220,169,.8)}.neural-empty-inspector{min-height:420px;display:grid;place-content:center;text-align:center;color:#687b94}.neural-orbit{position:relative;width:92px;height:92px;margin:0 auto 18px}.neural-orbit:before,.neural-orbit:after{content:'';position:absolute;inset:8px;border:1px solid #21334e;border-radius:50%}.neural-orbit:after{inset:25px;border-color:#423b78}.neural-core{position:absolute;left:50%;top:50%;width:16px;height:16px;transform:translate(-50%,-50%);border-radius:50%;background:#6694ff;box-shadow:0 0 25px rgba(102,148,255,.75)}.neural-empty-inspector strong{display:block;color:#dce6f4;font-size:15px}.neural-empty-inspector span{display:block;max-width:240px;margin:7px auto 0;font-size:10.5px;line-height:1.65}
.neural-node-card{padding:14px;border:1px solid #1b2b43;border-radius:15px;background:linear-gradient(145deg,#0c1625,#08111d)}.neural-node-top{display:flex;align-items:center;gap:11px}.neural-node-avatar{width:39px;height:39px;flex:0 0 39px;display:grid;place-items:center;border:1px solid color-mix(in srgb,var(--node-color) 34%,#1b2b43);border-radius:12px;background:color-mix(in srgb,var(--node-color) 12%,#09111d)}.neural-node-avatar:after{content:'';width:10px;height:10px;border-radius:50%;background:var(--node-color);box-shadow:0 0 16px color-mix(in srgb,var(--node-color) 80%,transparent)}.neural-node-kind{color:#70839b;font-size:8.5px;font-weight:760;text-transform:uppercase;letter-spacing:.09em}.neural-node-name{margin-top:4px;color:#eef4fb;font-size:15px;font-weight:720;line-height:1.3;word-break:break-word}.neural-summary{margin-top:12px;padding-top:12px;border-top:1px solid #17263b;color:#8495aa;font-size:10.5px;line-height:1.62;white-space:pre-wrap;word-break:break-word}.neural-section{margin-top:17px}.neural-section-title{margin-bottom:8px;color:#c4d1e1;font-size:10px;font-weight:700}.neural-properties{border:1px solid #17263b;border-radius:12px;overflow:hidden}.neural-property{display:flex;justify-content:space-between;gap:12px;padding:9px 10px;border-bottom:1px solid #152238;font-size:9.5px}.neural-property:last-child{border-bottom:0}.neural-property span{color:#667991}.neural-property strong{max-width:175px;color:#c9d5e5;font-weight:630;text-align:right;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.neural-relations{display:grid;gap:7px}.neural-relation{display:grid;grid-template-columns:24px minmax(0,1fr);gap:9px;align-items:center;padding:9px;border:1px solid #17263b;border-radius:11px;background:#09121f}.neural-relation-dot{width:24px;height:24px;display:grid;place-items:center;border-radius:8px;background:color-mix(in srgb,var(--relation-color) 12%,#08111c)}.neural-relation-dot:after{content:'';width:7px;height:7px;border-radius:50%;background:var(--relation-color);box-shadow:0 0 10px color-mix(in srgb,var(--relation-color) 55%,transparent)}.neural-relation-name{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:#cbd7e6;font-size:9.5px;font-weight:650}.neural-relation-type{margin-top:2px;color:#61738b;font-size:7.8px;text-transform:uppercase;letter-spacing:.06em}.neural-actions{display:flex;gap:7px;flex-wrap:wrap;margin-top:15px}.neural-actions .btn{border-color:#22344e;background:#0d1929;color:#aac0dc}.neural-warning{margin-top:10px;padding:9px 10px;border:1px solid #493d26;border-radius:10px;background:#17130b;color:#d7b469;font-size:9px;line-height:1.55}
@media(max-width:900px){.neural-shell{grid-template-columns:1fr;height:auto}.neural-canvas-wrap,.neural-canvas{height:540px;min-height:540px}.neural-detail{border-left:0;border-top:1px solid #17243a;min-height:260px}.neural-empty-inspector{min-height:250px}.neural-legend{width:100%;margin-left:0}}
@media(max-width:560px){.neural-toolbar{align-items:stretch}.neural-search-wrap{width:100%;min-width:0}.neural-search-wrap .btn{flex:0 0 auto}.neural-select{flex:1;min-width:0}.neural-canvas-wrap,.neural-canvas{height:500px;min-height:500px}.neural-detail-inner{padding:14px}}
@media(prefers-reduced-motion:reduce){.neural-signal-dot{box-shadow:none}}
`

const neuralGraphScript = `
(()=>{
 const dataEl=document.getElementById('neural-data'),configEl=document.getElementById('neural-config');if(!dataEl||!configEl)return;
 const payload=JSON.parse(dataEl.textContent||'{"nodes":[],"edges":[]}'),config=JSON.parse(configEl.textContent||'{}');
 const canvas=document.getElementById('neural-canvas'),wrap=canvas.parentElement,detail=document.getElementById('neural-detail'),search=document.getElementById('neural-search'),filter=document.getElementById('neural-filter'),depth=document.getElementById('neural-depth'),empty=document.getElementById('neural-empty'),ctx=canvas.getContext('2d');
 const reduced=window.matchMedia('(prefers-reduced-motion: reduce)').matches,memoryKinds=new Set(['event','artifact','goal','decision','preference','person','company','idea','constraint','milestone','problem','memory']);
 const palette={user:'#f1b55f',project:'#9b7cff',repository:'#58a6ff',workspace:'#6e8fff',device:'#91a0b7',event:'#50d9a6',artifact:'#53d6cf',goal:'#f2c94c',decision:'#ff8f76',preference:'#d98cff',person:'#ff8eb8',company:'#55d4e8',skill:'#f4a45f',symbol:'#6d9cff',method:'#6d9cff',function:'#6d9cff',callsite:'#7d91ad',file:'#50d9a6',external:'#a98bff',package:'#f0b35c',module:'#59c9df'};
 const nodes=(payload.nodes||[]).map((n,i)=>({...n,index:i,x:0,y:0,vx:0,vy:0})),edges=payload.edges||[],byId=new Map(nodes.map(n=>[n.id,n]));let selected=byId.get(payload.selectedId)||nodes.find(n=>n.selected)||null;
 let scale=1,panX=0,panY=0,dragNode=null,panning=false,lastX=0,lastY=0,moved=false,pinch=null,query='',kindFilter='all',sim=0,running=false,lastFrame=0;const pointers=new Map();
 empty.hidden=nodes.length>0;if(depth)depth.value=String(config.depth||payload.depth||1);
 function hash(s){let h=2166136261;for(let i=0;i<String(s).length;i++){h^=String(s).charCodeAt(i);h=Math.imul(h,16777619)}return h>>>0}
 function group(n){if(config.mode==='knowledge'&&memoryKinds.has(n.kind))return'memory';if(config.mode==='code'&&['symbol','method','function','callsite'].includes(n.kind))return'symbol';return n.kind||'symbol'}
 function visible(n){if(kindFilter!=='all'&&group(n)!==kindFilter)return false;if(!query)return true;const q=query.toLowerCase(),text=((n.name||'')+' '+(n.summary||'')+' '+(n.path||'')+' '+(n.qualifiedName||'')).toLowerCase();return text.includes(q)||n===selected}
 function color(n){return palette[n.kind]||'#63a0ff'}
 function radius(n){if(n.kind==='user')return 12;if(n.kind==='project')return 10;if(n.kind==='module')return 8.5;if(n.kind==='repository'||n.kind==='file')return 7.5;if(n.kind==='external')return 6.5;if(n===selected)return 9;return 5.5}
 function weak(e){return (Number(e.confidence||0)>0&&Number(e.confidence)<.7)||e.resolutionMode==='text'}
 function selectedEdge(e){return selected&&(e.from===selected.id||e.to===selected.id)}
 function init(){let i=0;for(const n of nodes){const a=((hash(n.id)%10000)/10000)*Math.PI*2;let r=150+(i%7)*32;if(n.kind==='user')r=0;else if(n.kind==='project')r=150;else if(n.kind==='repository'||n.kind==='package'||n.kind==='module')r=240;else if(n.kind==='file')r=300+(i%3)*28;n.x=Math.cos(a)*r;n.y=Math.sin(a)*r;i++}if(selected){selected.x=0;selected.y=0}}
 function resize(){const dpr=Math.min(2,window.devicePixelRatio||1),rect=wrap.getBoundingClientRect(),h=Math.max(420,rect.height||600);canvas.width=Math.floor(rect.width*dpr);canvas.height=Math.floor(h*dpr);canvas.style.height=h+'px';ctx.setTransform(dpr,0,0,dpr,0,0);draw(performance.now())}
 function screen(n){const r=canvas.getBoundingClientRect();return{x:r.width/2+panX+n.x*scale,y:r.height/2+panY+n.y*scale}}
 function world(x,y){const r=canvas.getBoundingClientRect();return{x:(x-r.width/2-panX)/scale,y:(y-r.height/2-panY)/scale}}
 function pulse(e,a,b,now){if(reduced)return;const hot=selectedEdge(e),slot=((now/1000)+(hash(e.id||e.from+e.to)%97)/17)%7;if(!hot&&slot>.72)return;const t=hot?((now/1250+(hash(e.id)%31)/31)%1):slot/.72,x=a.x+(b.x-a.x)*t,y=a.y+(b.y-a.y)*t;ctx.save();ctx.globalAlpha=hot?.95:.68;ctx.fillStyle=hot?'#9bc2ff':'#61a4ff';ctx.shadowColor=ctx.fillStyle;ctx.shadowBlur=12;ctx.beginPath();ctx.arc(x,y,hot?2.5:1.8,0,Math.PI*2);ctx.fill();ctx.restore()}
 function draw(now){const rect=canvas.getBoundingClientRect();ctx.clearRect(0,0,rect.width,rect.height);for(const e of edges){const a=byId.get(e.from),b=byId.get(e.to);if(!a||!b||!visible(a)||!visible(b))continue;const p=screen(a),q=screen(b),hot=selectedEdge(e),traffic=Math.min(1.6,Math.log2(Math.max(1,Number(e.count||1)))*.22);ctx.save();ctx.strokeStyle=hot?'rgba(103,157,255,.82)':selected?'rgba(76,105,151,.10)':weak(e)?'rgba(112,102,155,.18)':'rgba(76,112,171,.25)';ctx.lineWidth=(hot?1.65:weak(e)?.75:1)+traffic;if(weak(e))ctx.setLineDash([4,6]);if(hot){ctx.shadowColor='rgba(77,137,255,.65)';ctx.shadowBlur=8}ctx.beginPath();ctx.moveTo(p.x,p.y);ctx.lineTo(q.x,q.y);ctx.stroke();ctx.restore();pulse(e,p,q,now)}for(const n of nodes){if(!visible(n))continue;const p=screen(n),base=radius(n),breath=reduced?1:1+Math.sin(now/850+(hash(n.id)%37))*.035,r=base*breath,dim=selected&&n!==selected&&!edges.some(e=>selectedEdge(e)&&(e.from===n.id||e.to===n.id));ctx.save();ctx.globalAlpha=dim?.28:1;ctx.fillStyle=color(n);ctx.shadowColor=color(n);ctx.shadowBlur=n===selected?24:reduced?0:8;ctx.beginPath();ctx.arc(p.x,p.y,r,0,Math.PI*2);ctx.fill();ctx.shadowBlur=0;ctx.strokeStyle=n.canonical?'rgba(220,237,255,.72)':'rgba(160,178,203,.25)';ctx.lineWidth=n===selected?1.6:.7;ctx.stroke();if((scale>.85&&(n.kind==='project'||n.kind==='repository'||n.kind==='module'||n.kind==='file'))||n===selected){ctx.font='600 10px -apple-system,BlinkMacSystemFont,sans-serif';ctx.fillStyle=dim?'rgba(169,185,205,.35)':'rgba(210,224,241,.86)';ctx.textAlign='center';ctx.fillText(String(n.name||n.kind).slice(0,32),p.x,p.y+r+13)}ctx.restore()}}
 function physics(){if(sim>=80||dragNode)return;sim++;const active=nodes.filter(visible);for(const e of edges){const a=byId.get(e.from),b=byId.get(e.to);if(!a||!b||!visible(a)||!visible(b))continue;let dx=b.x-a.x,dy=b.y-a.y,d=Math.hypot(dx,dy)||1,target=e.relation==='IMPORTS'?110:125,f=(d-target)*.0016;dx/=d;dy/=d;a.vx+=dx*f;a.vy+=dy*f;b.vx-=dx*f;b.vy-=dy*f}for(let i=0;i<active.length;i++){const cap=Math.min(active.length,i+65);for(let j=i+1;j<cap;j++){const a=active[i],b=active[j];let dx=b.x-a.x,dy=b.y-a.y,d2=dx*dx+dy*dy+100;if(d2>85000)continue;const f=Math.min(.13,28/d2);a.vx-=dx*f;a.vy-=dy*f;b.vx+=dx*f;b.vy+=dy*f}}for(const n of active){if(n===selected)continue;n.vx+=-n.x*.000025;n.vy+=-n.y*.000025;n.vx*=.88;n.vy*=.88;n.x+=n.vx;n.y+=n.vy}}
 function frame(now){if(document.hidden){running=false;return}if(now-lastFrame>30){physics();draw(now);lastFrame=now}if(!reduced||sim<80){running=true;requestAnimationFrame(frame)}else running=false}
 function start(){if(running)return;running=true;requestAnimationFrame(frame)}
 function hit(x,y){let best=null,bd=22;for(const n of nodes){if(!visible(n))continue;const p=screen(n),d=Math.hypot(p.x-x,p.y-y);if(d<bd){best=n;bd=d}}return best}
 function esc(v){return String(v??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))}
 function pretty(v){return String(v||'').replace(/_/g,' ').replace(/\b\w/g,c=>c.toUpperCase())}
 function inspectorEmpty(){return '<div class="neural-detail-inner"><div class="neural-detail-head"><span class="neural-eyebrow">'+esc(config.inspectorLabel||'Graph inspector')+'</span><span class="neural-live">Live graph</span></div><div class="neural-empty-inspector"><div class="neural-orbit"><i class="neural-core"></i></div><strong>Select a neuron</strong><span>Choose a node to inspect its evidence and direct relationships.</span></div></div>'}
 function prop(label,value){if(value===undefined||value===null||value===''||value===0)return'';return'<div class="neural-property"><span>'+esc(label)+'</span><strong title="'+esc(value)+'">'+esc(value)+'</strong></div>'}
 function show(n){selected=n;if(!n){detail.innerHTML=inspectorEmpty();draw(performance.now());return}const relatedEdges=edges.filter(e=>e.from===n.id||e.to===n.id).slice(0,14),related=relatedEdges.map(e=>{const other=byId.get(e.from===n.id?e.to:e.from);if(!other)return'';return'<div class="neural-relation"><div class="neural-relation-dot" style="--relation-color:'+color(other)+'"></div><div><div class="neural-relation-name">'+esc(other.name||other.id)+'</div><div class="neural-relation-type">'+esc(pretty(e.relation))+(Number(e.count||0)>1?' · '+Number(e.count)+'×':'')+' · '+Math.round(Number(e.confidence||0)*100)+'%</div></div></div>'}).join('')||'<div class="neural-warning">No direct relationship is present in this bounded slice.</div>';let actions='';if(config.mode==='knowledge'&&n.kind==='repository')actions='<a class="btn small" href="/dashboard/code-graph">Explore in Code Graph</a>';if(config.mode==='code')actions='<a class="btn small" href="/dashboard/knowledge">Open Knowledge Graph</a>';const confidence=Math.round(Number(n.confidence||0)*100),warn=n.resolutionMode==='text'||(n.confidence&&Number(n.confidence)<.7)?'<div class="neural-warning">This relationship uses fallback evidence. Treat it as a possible connection, not authoritative semantics.</div>':'';detail.innerHTML='<div class="neural-detail-inner"><div class="neural-detail-head"><span class="neural-eyebrow">'+esc(config.inspectorLabel||'Graph inspector')+'</span><span class="neural-live">Selected</span></div><div class="neural-node-card"><div class="neural-node-top"><div class="neural-node-avatar" style="--node-color:'+color(n)+'"></div><div><div class="neural-node-kind">'+esc(pretty(n.kind))+'</div><div class="neural-node-name">'+esc(n.name||n.id)+'</div></div></div>'+(n.summary?'<div class="neural-summary">'+esc(n.summary)+'</div>':'')+'</div><div class="neural-section"><div class="neural-section-title">Evidence</div><div class="neural-properties">'+prop('Path',n.path)+(n.line?prop('Position',n.line+':'+(n.column||1)):'')+prop('Repository',n.repositoryPath||n.repositoryId)+prop('Provider',n.provider)+prop('Resolution',n.resolutionMode)+(confidence?prop('Confidence',confidence+'%'):'')+prop('Qualified name',n.qualifiedName)+prop('Scope',n.scope)+prop('Last seen',n.lastSeenAt?new Date(n.lastSeenAt).toLocaleString():'')+'</div>'+warn+'</div><div class="neural-section"><div class="neural-section-title">Direct signals · '+relatedEdges.length+'</div><div class="neural-relations">'+related+'</div></div><div class="neural-actions">'+actions+'</div></div>';draw(performance.now());start()}
 function pointerPos(e){const r=canvas.getBoundingClientRect();return{x:e.clientX-r.left,y:e.clientY-r.top}}
 canvas.addEventListener('pointerdown',e=>{const p=pointerPos(e);pointers.set(e.pointerId,p);canvas.setPointerCapture?.(e.pointerId);if(pointers.size===1){lastX=p.x;lastY=p.y;moved=false;dragNode=hit(p.x,p.y);panning=!dragNode;canvas.classList.add('dragging')}else if(pointers.size===2){const pts=[...pointers.values()],a=pts[0],b=pts[1],mid={x:(a.x+b.x)/2,y:(a.y+b.y)/2};pinch={dist:Math.max(1,Math.hypot(a.x-b.x,a.y-b.y)),scale,anchor:world(mid.x,mid.y)};dragNode=null;panning=false;moved=true}});
 canvas.addEventListener('pointermove',e=>{if(!pointers.has(e.pointerId))return;const p=pointerPos(e);pointers.set(e.pointerId,p);if(pointers.size>=2&&pinch){const pts=[...pointers.values()],a=pts[0],b=pts[1],mid={x:(a.x+b.x)/2,y:(a.y+b.y)/2},dist=Math.max(1,Math.hypot(a.x-b.x,a.y-b.y)),rect=canvas.getBoundingClientRect();scale=Math.max(.25,Math.min(3.2,pinch.scale*dist/pinch.dist));panX=mid.x-rect.width/2-pinch.anchor.x*scale;panY=mid.y-rect.height/2-pinch.anchor.y*scale;draw(performance.now());return}if(!dragNode&&!panning)return;const dx=p.x-lastX,dy=p.y-lastY;if(Math.abs(dx)+Math.abs(dy)>2)moved=true;if(dragNode){const w=world(p.x,p.y);dragNode.x=w.x;dragNode.y=w.y;dragNode.vx=dragNode.vy=0}else{panX+=dx;panY+=dy}lastX=p.x;lastY=p.y;draw(performance.now())});
 function endPointer(e){if(!pointers.has(e.pointerId))return;const single=pointers.size===1;pointers.delete(e.pointerId);if(single&&dragNode&&!moved)show(dragNode);if(pointers.size<2)pinch=null;dragNode=null;panning=false;canvas.classList.remove('dragging');start()}canvas.addEventListener('pointerup',endPointer);canvas.addEventListener('pointercancel',endPointer);
 canvas.addEventListener('wheel',e=>{e.preventDefault();const before=world(e.offsetX,e.offsetY),factor=Math.exp(-e.deltaY*.001);scale=Math.max(.25,Math.min(3.2,scale*factor));const after=world(e.offsetX,e.offsetY);panX+=(after.x-before.x)*scale;panY+=(after.y-before.y)*scale;draw(performance.now())},{passive:false});
 search.addEventListener('input',()=>{query=search.value.trim();sim=0;draw(performance.now());start()});filter.addEventListener('change',()=>{kindFilter=filter.value;sim=0;show(selected&&visible(selected)?selected:null);start()});
 document.getElementById('neural-reset').addEventListener('click',()=>{scale=1;panX=panY=0;query='';kindFilter='all';search.value='';filter.value='all';init();sim=0;show(null);start()});
 function navigateRemote(){const url=new URL(location.href);const value=search.value.trim();if(value)url.searchParams.set('symbol',value);else url.searchParams.delete('symbol');if(depth)url.searchParams.set('depth',depth.value);location.href=url.toString()}document.getElementById('neural-inspect')?.addEventListener('click',navigateRemote);search.addEventListener('keydown',e=>{if(e.key==='Enter'&&config.remoteSearch){e.preventDefault();navigateRemote()}});depth?.addEventListener('change',navigateRemote);
 document.addEventListener('visibilitychange',()=>{if(!document.hidden)start()});window.addEventListener('resize',resize);init();resize();show(selected);start();
})();
`
