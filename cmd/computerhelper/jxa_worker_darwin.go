//go:build darwin

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const macWorkerLockPoll = 10 * time.Millisecond

// macPersistentWorkerScript keeps one JXA/System Events process alive for the
// lifetime of the native computer helper. The previous implementation spawned
// osascript for every AX tree lookup/click/type operation, which dominated
// latency even though the outer Go helper itself was already persistent.
const macPersistentWorkerScript = `ObjC.import('Foundation'); ObjC.import('Vision');
function safe(fn,fb){try{return fn()}catch(e){return fb}}
function norm(v){return String(v==null?'':v).trim().toLowerCase().replace(/\s+/g,' ')}
function score(target,role,name,desc,value){
  var t=norm(target),best=0;
  function one(text,exact,inside){text=norm(text);if(!text)return;if(text===t)best=Math.max(best,exact);else if(text.indexOf(t)>=0||t.indexOf(text)>=0)best=Math.max(best,inside)}
  one(name,100,70);one(desc,85,55);one(value,65,45);one(role,35,20);
  var r=norm(role);if(best>0&&(r.indexOf('button')>=0||r.indexOf('link')>=0||r.indexOf('menu')>=0||r.indexOf('checkbox')>=0||r.indexOf('radio')>=0||r.indexOf('textfield')>=0||r.indexOf('text field')>=0))best+=10;
  return best;
}
function processFor(pid){
  var se=Application('System Events'),ps=se.applicationProcesses.whose({unixId:Number(pid)})();
  if(!ps.length)throw new Error('application process not found');
  return {se:se,p:ps[0]};
}
function nodeFor(e,pid,path,count,max,depth){
  if(count.n++>=max||depth>8)return null;
  var item={elementId:String(pid)+':'+path.join('.'),role:safe(function(){return String(e.role())},''),name:safe(function(){return String(e.name())},''),description:safe(function(){return String(e.description())},''),value:safe(function(){var v=e.value();return v==null?null:String(v)},null),enabled:safe(function(){return !!e.enabled()},true),children:[]};
  var pos=safe(function(){return e.position()},null),size=safe(function(){return e.size()},null);
  if(pos&&size&&pos.length>=2&&size.length>=2)item.bounds={x:Number(pos[0]),y:Number(pos[1]),width:Number(size[0]),height:Number(size[1])};
  var children=safe(function(){return e.uiElements()},[]);
  for(var i=0;i<children.length&&count.n<max;i++){var child=nodeFor(children[i],pid,path.concat([i]),count,max,depth+1);if(child)item.children.push(child)}
  return item;
}
function tree(req){
  var pid=Number(req.pid||0),windowIndex=Number(req.windowIndex==null?-1:req.windowIndex),max=Number(req.max||500),pp=processFor(pid),count={n:0},wins=safe(function(){return pp.p.windows()},[]),nodes=[];
  if(windowIndex>=0){
    if(windowIndex>=wins.length)throw new Error('window reference is stale');
    var selected=nodeFor(wins[windowIndex],pid,[windowIndex],count,max,0);if(selected)nodes.push(selected);
  }else{
    for(var i=0;i<wins.length&&count.n<max;i++){var n=nodeFor(wins[i],pid,[i],count,max,0);if(n)nodes.push(n)}
  }
  return {pid:pid,windowIndex:windowIndex,app:safe(function(){return String(pp.p.name())},''),nodes:nodes,truncated:count.n>=max,engine:'persistent-jxa'};
}
function semantic(req){
  var pid=Number(req.pid||0),windowIndex=Number(req.windowIndex==null?-1:req.windowIndex),target=String(req.target||''),op=String(req.operation||''),text=String(req.text||''),max=Number(req.max||500),pp=processFor(pid),count=0,best=null,bestScore=0,runnerUp=null,runnerUpScore=0;
  if(!target)throw new Error('semantic target required');
  function walk(e,path,depth){
    if(count++>=max||depth>8)return;
    var role=safe(function(){return String(e.role())},''),name=safe(function(){return String(e.name())},''),desc=safe(function(){return String(e.description())},''),value=safe(function(){var v=e.value();return v==null?'':String(v)},''),enabled=safe(function(){return !!e.enabled()},true);
    var s=score(target,role,name,desc,value);if(!enabled)s-=60;var candidateId=String(pid)+':'+path.join('.');
    if(s>bestScore){
      if(best){runnerUp={elementId:best.elementId};runnerUpScore=bestScore}
      var pos=safe(function(){return e.position()},null),size=safe(function(){return e.size()},null),bounds=null;
      if(pos&&size&&pos.length>=2&&size.length>=2)bounds={x:Number(pos[0]),y:Number(pos[1]),width:Number(size[0]),height:Number(size[1])};
      best={element:e,elementId:candidateId,role:role,name:name,description:desc,value:value,bounds:bounds};bestScore=s;
    }else if(candidateId!==(best&&best.elementId)&&s>runnerUpScore){runnerUp={elementId:candidateId};runnerUpScore=s}
    var children=safe(function(){return e.uiElements()},[]);for(var i=0;i<children.length&&count<max;i++)walk(children[i],path.concat([i]),depth+1);
  }
  var wins=safe(function(){return pp.p.windows()},[]);
  if(windowIndex>=0){
    if(windowIndex>=wins.length)throw new Error('window reference is stale');
    walk(wins[windowIndex],[windowIndex],0);
  }else{
    for(var i=0;i<wins.length&&count<max;i++)walk(wins[i],[i],0);
  }
  if(!best||bestScore<35)throw new Error('no accessible UI element matched '+JSON.stringify(target));
  if(runnerUp&&runnerUpScore>=35&&bestScore-runnerUpScore<8)throw new Error('ambiguous accessible UI target '+JSON.stringify(target)+': top matches '+best.elementId+' ('+bestScore+') and '+runnerUp.elementId+' ('+runnerUpScore+') are too close');
  if(op==='click'){
    try{best.element.click()}catch(e){throw new Error('background accessibility click failed: '+String(e))}
  }else if(op==='type'){
    try{best.element.value=text}catch(e){throw new Error('background accessibility value update failed: '+String(e))}
  }else throw new Error('unsupported semantic action');
  return {operation:op,background:true,physicalInput:false,engine:'persistent-jxa',resolvedTarget:{elementId:best.elementId,role:best.role,name:best.name,description:best.description,value:best.value,bounds:best.bounds,score:bestScore}};
}
function windows(){
  var se=Application('System Events'),ps=safe(function(){return se.applicationProcesses()},[]),out=[];
  for(var pi=0;pi<ps.length;pi++){
    var p=ps[pi],visible=safe(function(){return !!p.visible()},false);if(!visible)continue;
    var pid=Number(safe(function(){return p.unixId()},0)),app=safe(function(){return String(p.name())},'');if(!pid||!app)continue;
    var wins=safe(function(){return p.windows()},[]);
    for(var wi=0;wi<wins.length;wi++){
      var w=wins[wi],title=safe(function(){return String(w.name())},''),pos=safe(function(){return w.position()},null),size=safe(function(){return w.size()},null);
      if(!pos||!size||pos.length<2||size.length<2)continue;
      var width=Number(size[0]),height=Number(size[1]);if(width<=0||height<=0)continue;
      out.push({windowId:'ax:'+String(pid)+':'+String(wi),pid:pid,app:app,title:title,bounds:{x:Number(pos[0]),y:Number(pos[1]),width:width,height:height},engine:'persistent-jxa'});
    }
  }
  return out;
}
function visionFile(req){
  var path=String(req.path||''),wx=Number(req.x||0),wy=Number(req.y||0),ww=Number(req.width||0),wh=Number(req.height||0),windowId=String(req.windowId||'');
  if(!path||ww<=0||wh<=0)throw new Error('vision file requires path and non-empty bounds');
  var url=$.NSURL.fileURLWithPath(path),handler=$.VNImageRequestHandler.alloc.initWithURLOptions(url,$({})),request=$.VNRecognizeTextRequest.alloc.init;
  request.recognitionLevel=$.VNRequestTextRecognitionLevelAccurate;request.usesLanguageCorrection=true;
  var error=Ref();if(!handler.performRequestsError($([request]),error)){var message='Vision OCR failed';try{message=ObjC.unwrap(error[0].localizedDescription)}catch(e){}throw new Error(message)}
  var results=request.results,out=[],count=Number(results.count||0);
  for(var i=0;i<count&&out.length<300;i++){
    var observation=results.objectAtIndex(i),candidates=observation.topCandidates(1);if(Number(candidates.count||0)<1)continue;
    var candidate=candidates.objectAtIndex(0),text=String(ObjC.unwrap(candidate.string)||'').trim(),confidence=Number(candidate.confidence||0);if(!text||confidence<0.25)continue;
    var box=observation.boundingBox,x=wx+Number(box.origin.x)*ww,y=wy+(1-(Number(box.origin.y)+Number(box.size.height)))*wh,w=Number(box.size.width)*ww,h=Number(box.size.height)*wh,cx=x+w/2,cy=y+h/2;
    out.push({elementId:'vision:'+cx.toFixed(2)+':'+cy.toFixed(2),windowId:windowId,role:'visionText',name:text,description:'Vision OCR text',value:text,enabled:true,source:'vision',confidence:confidence,bounds:{x:x,y:y,width:w,height:h}});
  }
  return {nodes:out,engine:'persistent-jxa-vision'};
}
function handle(req){if(req.op==='ping')return {ready:true,engine:'persistent-jxa'};if(req.op==='windows')return windows();if(req.op==='tree')return tree(req);if(req.op==='semantic')return semantic(req);if(req.op==='vision_file')return visionFile(req);throw new Error('unsupported worker operation')}
function writeLine(out,obj){var s=$(JSON.stringify(obj)+'\n'),d=s.dataUsingEncoding($.NSUTF8StringEncoding);out.writeData(d)}
function run(){
  var input=$.NSFileHandle.fileHandleWithStandardInput,out=$.NSFileHandle.fileHandleWithStandardOutput,buffer='';
  while(true){
    var data=input.readDataOfLength(4096);if(Number(data.length||0)===0)break;
    var part=$.NSString.alloc.initWithDataEncoding(data,$.NSUTF8StringEncoding);buffer+=String(ObjC.unwrap(part));
    var lines=buffer.split('\n');buffer=lines.pop();
    for(var i=0;i<lines.length;i++){var line=String(lines[i]||'').trim();if(!line)continue;try{writeLine(out,{ok:true,result:handle(JSON.parse(line))})}catch(e){writeLine(out,{ok:false,error:String(e.message||e)})}}
  }
}`

type macWorkerResponse struct {
	OK     bool   `json:"ok"`
	Result any    `json:"result"`
	Error  string `json:"error"`
}

type macJXAWorker struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr bytes.Buffer
}

var sharedMacJXAWorker macJXAWorker

func (w *macJXAWorker) lock(ctx context.Context) error {
	ticker := time.NewTicker(macWorkerLockPoll)
	defer ticker.Stop()
	for {
		if w.mu.TryLock() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w *macJXAWorker) resetLocked() {
	if w.stdin != nil {
		_ = w.stdin.Close()
	}
	if w.cmd != nil && w.cmd.Process != nil {
		_ = w.cmd.Process.Kill()
		_, _ = w.cmd.Process.Wait()
	}
	w.cmd = nil
	w.stdin = nil
	w.stdout = nil
	w.stderr.Reset()
}

func (w *macJXAWorker) startLocked() error {
	if w.cmd != nil && w.cmd.Process != nil && w.stdin != nil && w.stdout != nil {
		return nil
	}
	cmd := exec.Command("/usr/bin/osascript", "-l", "JavaScript", "-e", macPersistentWorkerScript)
	cmd.Env = append(os.Environ(), "CI=1")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return err
	}
	w.stderr.Reset()
	cmd.Stderr = &w.stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return err
	}
	w.cmd = cmd
	w.stdin = stdin
	w.stdout = bufio.NewReaderSize(stdoutPipe, 64<<10)
	return nil
}

func (w *macJXAWorker) call(ctx context.Context, request map[string]any) (any, error) {
	if err := w.lock(ctx); err != nil {
		return nil, err
	}
	defer w.mu.Unlock()
	if err := w.startLocked(); err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(request)
	if _, err := w.stdin.Write(append(raw, '\n')); err != nil {
		w.resetLocked()
		return nil, err
	}
	type readResult struct {
		line []byte
		err  error
	}
	readCh := make(chan readResult, 1)
	go func(reader *bufio.Reader) {
		line, err := reader.ReadBytes('\n')
		readCh <- readResult{line: line, err: err}
	}(w.stdout)
	select {
	case <-ctx.Done():
		w.resetLocked()
		return nil, ctx.Err()
	case read := <-readCh:
		if read.err != nil {
			message := strings.TrimSpace(w.stderr.String())
			w.resetLocked()
			if message == "" {
				message = read.err.Error()
			}
			return nil, errors.New(message)
		}
		var response macWorkerResponse
		if err := json.Unmarshal(read.line, &response); err != nil {
			w.resetLocked()
			return nil, fmt.Errorf("invalid persistent JXA response: %w", err)
		}
		if !response.OK {
			if response.Error == "" {
				response.Error = "persistent JXA operation failed"
			}
			return nil, errors.New(response.Error)
		}
		return response.Result, nil
	}
}

func macPersistentWindows(ctx context.Context) ([]any, error) {
	value, err := sharedMacJXAWorker.call(ctx, map[string]any{"op": "windows"})
	if err != nil {
		return nil, err
	}
	items, ok := value.([]any)
	if !ok {
		return nil, errors.New("persistent JXA windows returned invalid payload")
	}
	return items, nil
}

func macPersistentTree(ctx context.Context, pid, windowIndex, max int) (map[string]any, error) {
	value, err := sharedMacJXAWorker.call(ctx, map[string]any{"op": "tree", "pid": pid, "windowIndex": windowIndex, "max": max})
	if err != nil {
		return nil, err
	}
	root, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("persistent JXA tree returned invalid payload")
	}
	return root, nil
}

func macPersistentSemanticAction(ctx context.Context, pid, windowIndex int, operation, target, text string) (any, error) {
	return sharedMacJXAWorker.call(ctx, map[string]any{
		"op": "semantic", "pid": pid, "windowIndex": windowIndex, "operation": operation, "target": target, "text": text, "max": 500,
	})
}

func macPersistentVisionFile(ctx context.Context, path, windowID string, bounds map[string]float64) ([]any, error) {
	value, err := sharedMacJXAWorker.call(ctx, map[string]any{
		"op": "vision_file", "path": path, "windowId": windowID,
		"x": bounds["x"], "y": bounds["y"], "width": bounds["width"], "height": bounds["height"],
	})
	if err != nil {
		return nil, err
	}
	root, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("persistent Vision returned invalid payload")
	}
	nodes, ok := root["nodes"].([]any)
	if !ok {
		return nil, errors.New("persistent Vision returned invalid nodes")
	}
	return nodes, nil
}

func macPersistentWorkerReady(ctx context.Context) bool {
	value, err := sharedMacJXAWorker.call(ctx, map[string]any{"op": "ping"})
	if err != nil {
		return false
	}
	root, _ := value.(map[string]any)
	ready, _ := root["ready"].(bool)
	return ready
}
