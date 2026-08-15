package cloudserver

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/ui"
	"github.com/0xmarkhydra/codelocal/internal/webauth"
)

func statValue(stats map[string]int, key string) int {
	if stats == nil {
		return 0
	}
	return stats[key]
}

func (s *Server) knowledgeDashboard(w http.ResponseWriter, r *http.Request, identity *webauth.Identity) {
	graph, err := s.Store.KnowledgeGraph(r.Context(), identity.User.ID, 320)
	if err != nil {
		page := ui.DashboardPage(ui.DashboardOptions{
			Title: "Knowledge Graph", Active: "knowledge", Email: identity.User.Email, CSRF: identity.CSRF,
			Subtitle: "A living map of the projects, repositories and durable knowledge CodeLocal has learned for your account.",
			Body:     `<div class="alert">Knowledge Graph is temporarily unavailable. Please try again after the graph store is reachable.</div>`,
			IsAdmin:  cloud.IsAdminEmail(identity.User.Email),
		})
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(page))
		return
	}
	raw, _ := json.Marshal(graph)
	body := fmt.Sprintf(`
<style>
.knowledge-stats{display:grid;grid-template-columns:repeat(5,minmax(0,1fr));gap:12px;margin-bottom:16px}.knowledge-stat{padding:16px 18px}.knowledge-stat strong{display:block;font-size:25px;letter-spacing:-.03em}.knowledge-stat span{font-size:12px;color:var(--muted)}
.knowledge-toolbar{display:flex;gap:10px;align-items:center;flex-wrap:wrap;padding:12px;border-bottom:1px solid var(--line)}.knowledge-search,.knowledge-filter{height:38px;border:1px solid var(--line);border-radius:10px;background:var(--surface);color:var(--text);padding:0 12px}.knowledge-search{flex:1;min-width:220px}.knowledge-filter{min-width:150px}.knowledge-shell{display:grid;grid-template-columns:minmax(0,1fr) 290px;min-height:610px}.knowledge-canvas-wrap{position:relative;min-height:610px;overflow:hidden;background:radial-gradient(circle at 50%% 45%%,color-mix(in srgb,var(--surface) 92%%,transparent),var(--bg))}.knowledge-canvas{display:block;width:100%%;height:610px;cursor:grab;touch-action:none}.knowledge-canvas.dragging{cursor:grabbing}.knowledge-help{position:absolute;left:14px;bottom:12px;padding:7px 10px;border:1px solid var(--line);border-radius:9px;background:color-mix(in srgb,var(--surface) 90%%,transparent);font-size:11px;color:var(--muted);pointer-events:none}.knowledge-detail{border-left:1px solid var(--line);padding:18px;overflow:auto}.knowledge-detail-empty{color:var(--muted);font-size:13px;line-height:1.6}.knowledge-kind{display:inline-flex;padding:4px 8px;border-radius:999px;background:var(--surface2);font-size:10px;text-transform:uppercase;letter-spacing:.08em;color:var(--muted);margin-bottom:10px}.knowledge-name{font-size:18px;font-weight:700;word-break:break-word}.knowledge-summary{font-size:13px;color:var(--muted);line-height:1.55;margin-top:10px;white-space:pre-wrap;word-break:break-word}.knowledge-meta{margin-top:16px;display:grid;gap:8px;font-size:12px}.knowledge-meta-row{display:flex;justify-content:space-between;gap:12px}.knowledge-meta-row span:first-child{color:var(--muted)}.knowledge-legend{display:flex;gap:12px;align-items:center;flex-wrap:wrap;font-size:11px;color:var(--muted)}.knowledge-dot{width:8px;height:8px;border-radius:50%%;display:inline-block;margin-right:5px}.knowledge-empty{position:absolute;inset:0;display:grid;place-content:center;gap:7px;padding:28px;text-align:center;background:color-mix(in srgb,var(--surface) 72%%,transparent);color:var(--muted);pointer-events:none}.knowledge-empty[hidden]{display:none}.knowledge-empty strong{color:var(--text);font-size:16px}.knowledge-empty span{max-width:390px;font-size:12px;line-height:1.6}
@media(max-width:900px){.knowledge-stats{grid-template-columns:repeat(2,minmax(0,1fr))}.knowledge-shell{grid-template-columns:1fr}.knowledge-detail{border-left:0;border-top:1px solid var(--line);min-height:180px}.knowledge-canvas,.knowledge-canvas-wrap{min-height:520px;height:520px}}
</style>
<div class="knowledge-stats">
 <div class="card knowledge-stat"><strong>%d</strong><span>Projects</span></div>
 <div class="card knowledge-stat"><strong>%d</strong><span>Repositories</span></div>
 <div class="card knowledge-stat"><strong>%d</strong><span>Learned skills</span></div>
 <div class="card knowledge-stat"><strong>%d</strong><span>Remembered nodes</span></div>
 <div class="card knowledge-stat"><strong>%d</strong><span>Relationships</span></div>
</div>
<div class="card" style="overflow:hidden">
 <div class="knowledge-toolbar">
  <input id="knowledge-search" class="knowledge-search" type="search" placeholder="Search what CodeLocal knows…" autocomplete="off">
  <select id="knowledge-filter" class="knowledge-filter"><option value="all">All knowledge</option><option value="project">Projects</option><option value="repository">Repositories</option><option value="workspace">Workspaces</option><option value="memory">Memories</option><option value="skill">Learned skills</option><option value="device">Devices</option></select>
  <button id="knowledge-reset" class="btn small" type="button">Reset view</button>
  <div class="knowledge-legend"><span><i class="knowledge-dot" style="background:#7c6df2"></i>Project</span><span><i class="knowledge-dot" style="background:#3ea6ff"></i>Repo</span><span><i class="knowledge-dot" style="background:#3ccf91"></i>Memory</span></div>
 </div>
 <div class="knowledge-shell">
  <div class="knowledge-canvas-wrap"><canvas id="knowledge-canvas" class="knowledge-canvas" role="img" aria-label="Interactive CodeLocal knowledge graph showing projects, repositories and learned knowledge relationships"></canvas><div id="knowledge-empty" class="knowledge-empty" hidden><strong>No knowledge yet</strong><span>Use CodeLocal on a project and let project memory accumulate. Learned relationships will appear here as the graph is populated.</span></div><div class="knowledge-help">Drag nodes · drag background to pan · wheel/pinch to zoom · tap for details</div></div>
  <aside id="knowledge-detail" class="knowledge-detail"><div class="knowledge-detail-empty">Select a node to inspect what CodeLocal knows and where it belongs. The dashboard only receives sanitized graph metadata; local paths, approval tokens and learned-skill recipe steps stay off this page.</div></aside>
 </div>
</div>
<script id="knowledge-data" type="application/json">%s</script>
<script>
(()=>{
 const payload=JSON.parse(document.getElementById('knowledge-data').textContent||'{"nodes":[],"edges":[]}');
 const canvas=document.getElementById('knowledge-canvas'),wrap=canvas.parentElement,detail=document.getElementById('knowledge-detail'),search=document.getElementById('knowledge-search'),filter=document.getElementById('knowledge-filter'),empty=document.getElementById('knowledge-empty');
 const ctx=canvas.getContext('2d'); const themeText=getComputedStyle(document.querySelector('.shell')).getPropertyValue('--text').trim()||'#101828'; const nodes=(payload.nodes||[]).map((n,i)=>({...n,vx:0,vy:0,x:0,y:0,index:i})); const edges=payload.edges||[]; const byId=new Map(nodes.map(n=>[n.id,n])); empty.hidden=nodes.length>0;
 const palette={user:'#ffb347',project:'#7c6df2',repository:'#3ea6ff',workspace:'#5d8cff',device:'#9aa4b2',event:'#3ccf91',artifact:'#4fd1c5',goal:'#f7c948',decision:'#f08c78',preference:'#d783ff',person:'#ff8fb1',company:'#5ad1e6',skill:'#f5a65b'};
 const memoryKinds=new Set(['event','artifact','goal','decision','preference','person','company','idea','constraint','milestone','problem','memory']);
 function hash(s){let h=2166136261;for(let i=0;i<s.length;i++){h^=s.charCodeAt(i);h=Math.imul(h,16777619)}return h>>>0}
 function init(){let pc=0,rc=0,mc=0,wc=0;nodes.forEach(n=>{const a=(hash(n.id)%%10000)/10000*Math.PI*2;if(n.kind==='user'){n.x=0;n.y=0}else if(n.kind==='project'){const r=170;n.x=Math.cos(a)*r;n.y=Math.sin(a)*r;pc++}else if(n.kind==='repository'){const r=310+(rc%%3)*22;n.x=Math.cos(a)*r;n.y=Math.sin(a)*r;rc++}else if(n.kind==='workspace'||n.kind==='device'){const r=420+(wc%%2)*25;n.x=Math.cos(a)*r;n.y=Math.sin(a)*r;wc++}else{const r=230+(mc%%7)*30;n.x=Math.cos(a)*r;n.y=Math.sin(a)*r;mc++}})}
 let scale=1,panX=0,panY=0,selected=null,dragNode=null,panning=false,lastX=0,lastY=0,moved=false,query='',kindFilter='all',sim=0,pinch=null; const pointers=new Map();
 function group(n){return memoryKinds.has(n.kind)?'memory':n.kind}
 function visible(n){if(kindFilter!=='all'&&group(n)!==kindFilter&&n.kind!=='user'&&n.kind!=='project')return false;if(!query)return true;const q=query.toLowerCase();return ((n.name||'')+' '+(n.summary||'')+' '+(n.kind||'')).toLowerCase().includes(q)||n.kind==='user'||n.kind==='project'}
 function radius(n){if(n.kind==='user')return 13;if(n.kind==='project')return 10;if(n.kind==='repository')return 8;return 6+Math.max(0,Math.min(4,(n.importance||0)*4))}
 function color(n){return palette[n.kind]||'#3ccf91'}
 function resize(){const dpr=Math.min(2,window.devicePixelRatio||1),rect=wrap.getBoundingClientRect(),h=Math.max(420,rect.height||610);canvas.width=Math.floor(rect.width*dpr);canvas.height=Math.floor(h*dpr);canvas.style.height=h+'px';ctx.setTransform(dpr,0,0,dpr,0,0);draw()}
 function screen(n){const r=canvas.getBoundingClientRect();return{x:r.width/2+panX+n.x*scale,y:r.height/2+panY+n.y*scale}}
 function world(x,y){const r=canvas.getBoundingClientRect();return{x:(x-r.width/2-panX)/scale,y:(y-r.height/2-panY)/scale}}
 function draw(){const rect=canvas.getBoundingClientRect();ctx.clearRect(0,0,rect.width,rect.height);ctx.lineWidth=1;for(const e of edges){const a=byId.get(e.from),b=byId.get(e.to);if(!a||!b||!visible(a)||!visible(b))continue;const p=screen(a),q=screen(b);ctx.strokeStyle='rgba(130,140,165,.20)';ctx.beginPath();ctx.moveTo(p.x,p.y);ctx.lineTo(q.x,q.y);ctx.stroke()}for(const n of nodes){if(!visible(n))continue;const p=screen(n),r=radius(n)*(selected===n?1.35:1);ctx.beginPath();ctx.arc(p.x,p.y,r,0,Math.PI*2);ctx.fillStyle=color(n);ctx.globalAlpha=query&&((n.name||'')+' '+(n.summary||'')).toLowerCase().includes(query.toLowerCase())?1:.9;ctx.fill();ctx.globalAlpha=1;if(selected===n){ctx.strokeStyle='rgba(255,255,255,.85)';ctx.lineWidth=2;ctx.stroke()}if(scale>.78&&(n.kind==='project'||n.kind==='user'||selected===n)){ctx.font='600 11px -apple-system,BlinkMacSystemFont,sans-serif';ctx.fillStyle=themeText;ctx.textAlign='center';ctx.fillText((n.name||n.kind).slice(0,28),p.x,p.y+r+14)}}}
 function physics(){if(sim>110||dragNode)return;sim++;const active=nodes.filter(visible);for(const e of edges){const a=byId.get(e.from),b=byId.get(e.to);if(!a||!b||!visible(a)||!visible(b))continue;let dx=b.x-a.x,dy=b.y-a.y,d=Math.hypot(dx,dy)||1,target=e.relation==='HAS_MEMORY'?85:e.relation==='CONTAINS_REPO'?145:125,f=(d-target)*.0018;dx/=d;dy/=d;a.vx+=dx*f;a.vy+=dy*f;b.vx-=dx*f;b.vy-=dy*f}for(let i=0;i<active.length;i++){for(let j=i+1;j<active.length;j++){const a=active[i],b=active[j];let dx=b.x-a.x,dy=b.y-a.y,d2=dx*dx+dy*dy+90;if(d2>90000)continue;const f=Math.min(.16,30/d2);a.vx-=dx*f;a.vy-=dy*f;b.vx+=dx*f;b.vy+=dy*f}}for(const n of active){if(n.kind==='user')continue;n.vx+=-n.x*.00003;n.vy+=-n.y*.00003;n.vx*=.88;n.vy*=.88;n.x+=n.vx;n.y+=n.vy}draw();requestAnimationFrame(physics)}
 function hit(x,y){let best=null,bd=22;for(const n of nodes){if(!visible(n))continue;const p=screen(n),d=Math.hypot(p.x-x,p.y-y);if(d<bd){best=n;bd=d}}return best}
 function show(n){selected=n;if(!n){detail.innerHTML='<div class="knowledge-detail-empty">Select a node to inspect its relationships and provenance.</div>';draw();return}const related=edges.filter(e=>e.from===n.id||e.to===n.id).slice(0,12).map(e=>{const other=byId.get(e.from===n.id?e.to:e.from);return '<div class="knowledge-meta-row"><span>'+esc(e.relation)+'</span><strong>'+esc(other?.name||'')+'</strong></div>'}).join('');detail.innerHTML='<div class="knowledge-kind">'+esc(n.kind)+'</div><div class="knowledge-name">'+esc(n.name||n.id)+'</div>'+(n.summary?'<div class="knowledge-summary">'+esc(n.summary)+'</div>':'')+'<div class="knowledge-meta"><div class="knowledge-meta-row"><span>Scope</span><strong>'+esc(n.scope||'—')+'</strong></div><div class="knowledge-meta-row"><span>Confidence</span><strong>'+Math.round((n.confidence||0)*100)+'%%</strong></div><div class="knowledge-meta-row"><span>Importance</span><strong>'+Math.round((n.importance||0)*100)+'%%</strong></div>'+(n.sourceMemoryId?'<div class="knowledge-meta-row"><span>Source</span><strong>Memory</strong></div>':'')+related+'</div>';draw()}
 function esc(v){return String(v??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))}
 function pointerPos(e){const r=canvas.getBoundingClientRect();return{x:e.clientX-r.left,y:e.clientY-r.top}}
 canvas.addEventListener('pointerdown',e=>{const p=pointerPos(e);pointers.set(e.pointerId,p);canvas.setPointerCapture?.(e.pointerId);if(pointers.size===1){lastX=p.x;lastY=p.y;moved=false;dragNode=hit(lastX,lastY);panning=!dragNode;canvas.classList.add('dragging');return}if(pointers.size===2){const pts=[...pointers.values()],a=pts[0],b=pts[1],mid={x:(a.x+b.x)/2,y:(a.y+b.y)/2};pinch={dist:Math.max(1,Math.hypot(a.x-b.x,a.y-b.y)),scale,anchor:world(mid.x,mid.y)};dragNode=null;panning=false;moved=true}});
 canvas.addEventListener('pointermove',e=>{if(!pointers.has(e.pointerId))return;const p=pointerPos(e);pointers.set(e.pointerId,p);if(pointers.size>=2&&pinch){const pts=[...pointers.values()],a=pts[0],b=pts[1],mid={x:(a.x+b.x)/2,y:(a.y+b.y)/2},dist=Math.max(1,Math.hypot(a.x-b.x,a.y-b.y)),rect=canvas.getBoundingClientRect();scale=Math.max(.25,Math.min(3.2,pinch.scale*(dist/pinch.dist)));panX=mid.x-rect.width/2-pinch.anchor.x*scale;panY=mid.y-rect.height/2-pinch.anchor.y*scale;draw();return}if(!dragNode&&!panning)return;const dx=p.x-lastX,dy=p.y-lastY;if(Math.abs(dx)+Math.abs(dy)>2)moved=true;if(dragNode){const w=world(p.x,p.y);dragNode.x=w.x;dragNode.y=w.y;dragNode.vx=dragNode.vy=0}else{panX+=dx;panY+=dy}lastX=p.x;lastY=p.y;draw()});
 function endPointer(e){if(!pointers.has(e.pointerId))return;const wasSingle=pointers.size===1;pointers.delete(e.pointerId);if(wasSingle&&dragNode&&!moved)show(dragNode);if(pointers.size<2)pinch=null;dragNode=null;panning=false;canvas.classList.remove('dragging')}
 canvas.addEventListener('pointerup',endPointer);canvas.addEventListener('pointercancel',endPointer);canvas.addEventListener('wheel',e=>{e.preventDefault();const before=world(e.offsetX,e.offsetY),factor=Math.exp(-e.deltaY*.001);scale=Math.max(.25,Math.min(3.2,scale*factor));const after=world(e.offsetX,e.offsetY);panX+=(after.x-before.x)*scale;panY+=(after.y-before.y)*scale;draw()},{passive:false});
 search.addEventListener('input',()=>{query=search.value.trim();sim=0;draw();const match=query&&nodes.find(n=>((n.name||'')+' '+(n.summary||'')).toLowerCase().includes(query.toLowerCase()));if(match){show(match);panX=-match.x*scale;panY=-match.y*scale}requestAnimationFrame(physics)});filter.addEventListener('change',()=>{kindFilter=filter.value;sim=0;show(null);requestAnimationFrame(physics)});document.getElementById('knowledge-reset').addEventListener('click',()=>{scale=1;panX=panY=0;query='';kindFilter='all';search.value='';filter.value='all';show(null);init();sim=0;requestAnimationFrame(physics)});
 init();window.addEventListener('resize',resize);resize();requestAnimationFrame(physics);
})();
</script>`, statValue(graph.Stats, "projects"), statValue(graph.Stats, "repositories"), statValue(graph.Stats, "skills"), statValue(graph.Stats, "memories"), statValue(graph.Stats, "edges"), string(raw))
	writeHTML(w, ui.DashboardPage(ui.DashboardOptions{
		Title: "Knowledge Graph", Active: "knowledge", Email: identity.User.Email, CSRF: identity.CSRF,
		Subtitle: "A living map of the projects, repositories and durable knowledge CodeLocal has learned for your account.",
		Body:     body, IsAdmin: cloud.IsAdminEmail(identity.User.Email),
	}))
}
