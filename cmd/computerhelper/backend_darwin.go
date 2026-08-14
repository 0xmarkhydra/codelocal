//go:build darwin

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func runOSA(ctx context.Context, language, script string, args ...string) (string, error) {
	command := []string{}
	if language != "" {
		command = append(command, "-l", language)
	}
	command = append(command, "-e", script)
	if len(args) > 0 {
		command = append(command, "--")
		command = append(command, args...)
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/osascript", command...)
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return "", errors.New(text)
	}
	return text, nil
}

func macAccessibilityTrusted(ctx context.Context) bool {
	text, err := runOSA(ctx, "JavaScript", `function run(){var s=Application('System Events'); try { return String(s.uiElementsEnabled()); } catch(e) { return 'false'; }}`)
	if err == nil && strings.EqualFold(strings.TrimSpace(text), "true") {
		return true
	}
	// Older System Events dictionaries do not expose uiElementsEnabled in JXA.
	text, err = runOSA(ctx, "", `tell application "System Events" to return UI elements enabled`)
	return err == nil && strings.EqualFold(strings.TrimSpace(text), "true")
}

func platformCapabilities() map[string]any {
	ctx := context.Background()
	trusted := macAccessibilityTrusted(ctx)
	return map[string]any{
		"available":         true,
		"backend":           "macos-accessibility+coregraphics",
		"screenCapture":     true,
		"uiTree":            trusted,
		"visionFallback":    true,
		"pointer":           trusted,
		"keyboard":          trusted,
		"clipboard":         false,
		"backgroundControl": trusted,
		"secureDesktop":     false,
		"permissionRequired": map[string]any{
			"accessibility":   !trusted,
			"screenRecording": true,
		},
	}
}

const macWindowScript = `ObjC.import('CoreGraphics');
function run(){
  var raw=$.CGWindowListCopyWindowInfo($.kCGWindowListOptionOnScreenOnly,$.kCGNullWindowID);
  var list=ObjC.deepUnwrap(raw) || [];
  var out=[];
  for (var i=0;i<list.length;i++) {
    var w=list[i]||{};
    var layer=Number(w.kCGWindowLayer||0);
    var owner=String(w.kCGWindowOwnerName||'');
    var title=String(w.kCGWindowName||'');
    var alpha=Number(w.kCGWindowAlpha==null?1:w.kCGWindowAlpha);
    if(layer!==0 || !owner || alpha<=0) continue;
    var b=w.kCGWindowBounds||{};
    out.push({windowId:String(w.kCGWindowNumber||''),pid:Number(w.kCGWindowOwnerPID||0),app:owner,title:title,bounds:{x:Number(b.X||0),y:Number(b.Y||0),width:Number(b.Width||0),height:Number(b.Height||0)}});
  }
  return JSON.stringify(out);
}`

const macMainScreenScript = `ObjC.import('AppKit');
function run(){var s=$.NSScreen.mainScreen,f=s.frame;return JSON.stringify({x:0,y:0,width:Number(f.size.width),height:Number(f.size.height)})}`

func macMainScreenBounds(ctx context.Context) (map[string]float64, error) {
	text, err := runOSA(ctx, "JavaScript", macMainScreenScript)
	if err != nil {
		return nil, err
	}
	var out map[string]float64
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return nil, err
	}
	if out["width"] <= 0 || out["height"] <= 0 {
		return nil, errors.New("main screen bounds unavailable")
	}
	return out, nil
}

func macWindows(ctx context.Context) (any, error) {
	text, err := runOSA(ctx, "JavaScript", macWindowScript)
	if err == nil {
		var out []any
		if json.Unmarshal([]byte(text), &out) == nil && len(out) > 0 {
			return out, nil
		}
	}
	bounds, boundsErr := macMainScreenBounds(ctx)
	if boundsErr != nil {
		if err != nil {
			return nil, err
		}
		return nil, boundsErr
	}
	return []any{map[string]any{
		"windowId": "screen:main", "pid": float64(0), "app": "Desktop", "title": "Main Screen",
		"bounds":   map[string]any{"x": bounds["x"], "y": bounds["y"], "width": bounds["width"], "height": bounds["height"]},
		"fallback": true,
	}}, nil
}

const macTreeScript = `function safe(fn,fb){try{return fn()}catch(e){return fb}}
function run(argv){
 var pid=Number(argv[0]||0), max=Number(argv[1]||400);
 var se=Application('System Events');
 var ps=se.applicationProcesses.whose({unixId:pid})();
 if(!ps.length) throw new Error('application process not found');
 var p=ps[0], count=0;
 function walk(e,path,depth){
   if(count++>=max || depth>8) return null;
   var item={elementId:String(pid)+':'+path.join('.'),role:safe(function(){return String(e.role())},''),name:safe(function(){return String(e.name())},''),description:safe(function(){return String(e.description())},''),value:safe(function(){var v=e.value(); return v==null?null:String(v)},null),enabled:safe(function(){return !!e.enabled()},true),children:[]};
   var children=safe(function(){return e.uiElements()},[]);
   for(var i=0;i<children.length && count<max;i++){var child=walk(children[i],path.concat([i]),depth+1);if(child)item.children.push(child)}
   return item;
 }
 var wins=safe(function(){return p.windows()},[]), out=[];
 for(var i=0;i<wins.length && count<max;i++){var node=walk(wins[i],[i],0);if(node)out.push(node)}
 return JSON.stringify({pid:pid,app:safe(function(){return String(p.name())},''),nodes:out,truncated:count>=max});
}`

func macWindowPID(ctx context.Context, windowID string) (int, error) {
	windows, err := macWindows(ctx)
	if err != nil {
		return 0, err
	}
	items, _ := windows.([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if fmt.Sprint(item["windowId"]) == windowID {
			if pid, ok := item["pid"].(float64); ok {
				return int(pid), nil
			}
		}
	}
	return 0, errors.New("window not found")
}

func macWindowBounds(ctx context.Context, windowID string) (map[string]float64, error) {
	if windowID == "screen:main" {
		return macMainScreenBounds(ctx)
	}
	windows, err := macWindows(ctx)
	if err != nil {
		return nil, err
	}
	items, _ := windows.([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if fmt.Sprint(item["windowId"]) != windowID {
			continue
		}
		bounds, _ := item["bounds"].(map[string]any)
		result := map[string]float64{}
		for _, key := range []string{"x", "y", "width", "height"} {
			value, ok := bounds[key].(float64)
			if !ok {
				return nil, errors.New("window bounds unavailable")
			}
			result[key] = value
		}
		if result["width"] <= 0 || result["height"] <= 0 {
			return nil, errors.New("window bounds are empty")
		}
		return result, nil
	}
	return nil, errors.New("window not found")
}

func macTreeQuality(value any) (informative int, actionable int) {
	switch typed := value.(type) {
	case []any:
		for _, child := range typed {
			i, a := macTreeQuality(child)
			informative += i
			actionable += a
		}
	case map[string]any:
		role := strings.ToLower(strings.TrimSpace(fmt.Sprint(typed["role"])))
		name := strings.TrimSpace(fmt.Sprint(typed["name"]))
		desc := strings.TrimSpace(fmt.Sprint(typed["description"]))
		nodeValue := strings.TrimSpace(fmt.Sprint(typed["value"]))
		if role != "" && role != "axwindow" {
			if (name != "" && name != "<nil>") || (desc != "" && desc != "<nil>") || (nodeValue != "" && nodeValue != "<nil>") {
				informative++
			}
			for _, marker := range []string{"button", "link", "menu", "checkbox", "radio", "textfield", "text field", "combobox", "pop up", "popup"} {
				if strings.Contains(role, marker) {
					actionable++
					break
				}
			}
		}
		for _, key := range []string{"nodes", "node", "children"} {
			if child, ok := typed[key]; ok {
				i, a := macTreeQuality(child)
				informative += i
				actionable += a
			}
		}
	}
	return informative, actionable
}

func macTreeDegraded(value any) bool {
	informative, actionable := macTreeQuality(value)
	return informative == 0 || (actionable == 0 && informative < 3)
}

const macVisionScript = `ObjC.import('Vision'); ObjC.import('Foundation');
function run(argv){
  var path=String(argv[0]||''), windowId=String(argv[1]||'');
  var wx=Number(argv[2]||0), wy=Number(argv[3]||0), ww=Number(argv[4]||0), wh=Number(argv[5]||0);
  var url=$.NSURL.fileURLWithPath(path);
  var handler=$.VNImageRequestHandler.alloc.initWithURLOptions(url,$({}));
  var request=$.VNRecognizeTextRequest.alloc.init;
  request.recognitionLevel=$.VNRequestTextRecognitionLevelAccurate;
  request.usesLanguageCorrection=true;
  var error=Ref();
  if(!handler.performRequestsError($([request]),error)) {
    var message='Vision OCR failed'; try { message=ObjC.unwrap(error[0].localizedDescription); } catch(e) {}
    throw new Error(message);
  }
  var results=request.results, out=[], count=Number(results.count||0);
  for(var i=0;i<count && out.length<300;i++) {
    var observation=results.objectAtIndex(i), candidates=observation.topCandidates(1);
    if(Number(candidates.count||0)<1) continue;
    var candidate=candidates.objectAtIndex(0), text=String(ObjC.unwrap(candidate.string)||'').trim();
    var confidence=Number(candidate.confidence||0);
    if(!text || confidence<0.25) continue;
    var box=observation.boundingBox;
    var x=wx+Number(box.origin.x)*ww;
    var y=wy+(1-(Number(box.origin.y)+Number(box.size.height)))*wh;
    var w=Number(box.size.width)*ww, h=Number(box.size.height)*wh;
    var cx=x+w/2, cy=y+h/2;
    out.push({elementId:'vision:'+cx.toFixed(2)+':'+cy.toFixed(2), windowId:windowId, role:'visionText', name:text,
      description:'Vision OCR text', value:text, enabled:true, source:'vision', confidence:confidence,
      bounds:{x:x,y:y,width:w,height:h}});
  }
  return JSON.stringify(out);
}`

func macVisionTree(ctx context.Context, windowID string) ([]any, error) {
	bounds, err := macWindowBounds(ctx, windowID)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "codelocal-vision-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "window.png")
	x, y := int(math.Round(bounds["x"])), int(math.Round(bounds["y"]))
	w, h := int(math.Round(bounds["width"])), int(math.Round(bounds["height"]))
	args := []string{"-x", "-t", "png"}
	if windowID != "screen:main" {
		region := fmt.Sprintf("%d,%d,%d,%d", x, y, w, h)
		args = append(args, "-R"+region)
	}
	args = append(args, path)
	cmd := exec.CommandContext(ctx, "/usr/sbin/screencapture", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = "macOS Screen Recording permission may be required"
		}
		return nil, errors.New(message)
	}
	text, err := runOSA(ctx, "JavaScript", macVisionScript, path, windowID,
		strconv.FormatFloat(bounds["x"], 'f', -1, 64), strconv.FormatFloat(bounds["y"], 'f', -1, 64),
		strconv.FormatFloat(bounds["width"], 'f', -1, 64), strconv.FormatFloat(bounds["height"], 'f', -1, 64))
	if err != nil {
		return nil, err
	}
	var out []any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return nil, fmt.Errorf("decode macOS Vision OCR: %w", err)
	}
	return out, nil
}

func macUITree(ctx context.Context, windowID string) (any, error) {
	if windowID == "screen:main" {
		vision, err := macVisionTree(ctx, windowID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"nodes": vision, "source": "vision", "accessibilityDegraded": true, "visionFallback": true, "visionElementCount": len(vision)}, nil
	}
	if !macAccessibilityTrusted(ctx) {
		return nil, errors.New("macOS Accessibility permission is required")
	}
	pid, err := macWindowPID(ctx, windowID)
	if err != nil {
		return nil, err
	}
	text, err := runOSA(ctx, "JavaScript", macTreeScript, strconv.Itoa(pid), "500")
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return nil, err
	}
	if !macTreeDegraded(out) {
		out["source"] = "accessibility"
		out["accessibilityDegraded"] = false
		return out, nil
	}
	out["accessibilityDegraded"] = true
	vision, visionErr := macVisionTree(ctx, windowID)
	if visionErr != nil {
		out["source"] = "accessibility"
		out["visionError"] = visionErr.Error()
		return out, nil
	}
	nodes, _ := out["nodes"].([]any)
	out["nodes"] = append(nodes, vision...)
	out["source"] = "accessibility+vision"
	out["visionFallback"] = true
	out["visionElementCount"] = len(vision)
	return out, nil
}

func macScreenshot(ctx context.Context, windowID string) (any, error) {
	dir, err := os.MkdirTemp("", "codelocal-screen-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "screen.png")
	args := []string{"-x", "-t", "png"}
	if strings.TrimSpace(windowID) != "" && windowID != "screen:main" {
		bounds, err := macWindowBounds(ctx, windowID)
		if err != nil {
			return nil, err
		}
		region := fmt.Sprintf("%d,%d,%d,%d", int(math.Round(bounds["x"])), int(math.Round(bounds["y"])), int(math.Round(bounds["width"])), int(math.Round(bounds["height"])))
		args = append(args, "-R"+region)
	}
	args = append(args, path)
	cmd := exec.CommandContext(ctx, "/usr/sbin/screencapture", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = "macOS Screen Recording permission may be required"
		}
		return nil, errors.New(message)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return map[string]any{"windowId": windowID, "__mcpImage": map[string]any{"mimeType": "image/png", "data": base64.StdEncoding.EncodeToString(data)}}, nil
}

func macFocus(ctx context.Context, windowID string) (any, error) {
	if windowID == "screen:main" {
		return map[string]any{"focused": true, "windowId": windowID, "fallback": true}, nil
	}
	if !macAccessibilityTrusted(ctx) {
		return nil, errors.New("macOS Accessibility permission is required")
	}
	pid, err := macWindowPID(ctx, windowID)
	if err != nil {
		return nil, err
	}
	script := `function run(argv){var pid=Number(argv[0]);var s=Application('System Events');var ps=s.applicationProcesses.whose({unixId:pid})();if(!ps.length)throw new Error('process not found');ps[0].frontmost=true;return JSON.stringify({focused:true,pid:pid})}`
	text, err := runOSA(ctx, "JavaScript", script, strconv.Itoa(pid))
	if err != nil {
		return nil, err
	}
	var out any
	_ = json.Unmarshal([]byte(text), &out)
	return out, nil
}

func macVisionPoint(elementID string) (float64, float64, bool) {
	if !strings.HasPrefix(elementID, "vision:") {
		return 0, 0, false
	}
	parts := strings.Split(elementID, ":")
	if len(parts) != 3 {
		return 0, 0, false
	}
	x, errX := strconv.ParseFloat(parts[1], 64)
	y, errY := strconv.ParseFloat(parts[2], 64)
	if errX != nil || errY != nil {
		return 0, 0, false
	}
	return x, y, true
}

func macElementClick(ctx context.Context, elementID string) (any, error) {
	if strings.HasPrefix(elementID, "vision:") {
		x, y, ok := macVisionPoint(elementID)
		if !ok {
			return nil, errors.New("invalid Vision elementId")
		}
		return macPointer(ctx, "click", x, y)
	}
	parts := strings.SplitN(elementID, ":", 2)
	if len(parts) != 2 {
		return nil, errors.New("invalid elementId")
	}
	pid, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, errors.New("invalid elementId pid")
	}
	path := parts[1]
	script := `function run(argv){var pid=Number(argv[0]);var path=String(argv[1]||'').split('.').filter(function(x){return x!==''}).map(Number);var s=Application('System Events');var ps=s.applicationProcesses.whose({unixId:pid})();if(!ps.length)throw new Error('process not found');var e=ps[0];for(var i=0;i<path.length;i++){var list=(i===0?e.windows():e.uiElements());e=list[path[i]];if(!e)throw new Error('element path stale')}e.click();return JSON.stringify({clicked:true,elementId:String(pid)+':'+path.join('.')})}`
	text, err := runOSA(ctx, "JavaScript", script, strconv.Itoa(pid), path)
	if err != nil {
		return nil, err
	}
	var out any
	_ = json.Unmarshal([]byte(text), &out)
	return out, nil
}

const macPointerScript = `ObjC.import('ApplicationServices');
function post(type,x,y,button){var p=$.CGPointMake(Number(x),Number(y));var e=$.CGEventCreateMouseEvent(null,type,p,button);$.CGEventPost($.kCGHIDEventTap,e);$.CFRelease(e)}
function run(argv){
 var op=argv[0];
 if(op==='click'){var x=Number(argv[1]),y=Number(argv[2]);post($.kCGEventMouseMoved,x,y,$.kCGMouseButtonLeft);post($.kCGEventLeftMouseDown,x,y,$.kCGMouseButtonLeft);post($.kCGEventLeftMouseUp,x,y,$.kCGMouseButtonLeft);return JSON.stringify({clicked:true,x:x,y:y})}
 if(op==='drag'){var x1=Number(argv[1]),y1=Number(argv[2]),x2=Number(argv[3]),y2=Number(argv[4]);post($.kCGEventMouseMoved,x1,y1,$.kCGMouseButtonLeft);post($.kCGEventLeftMouseDown,x1,y1,$.kCGMouseButtonLeft);post($.kCGEventLeftMouseDragged,x2,y2,$.kCGMouseButtonLeft);post($.kCGEventLeftMouseUp,x2,y2,$.kCGMouseButtonLeft);return JSON.stringify({dragged:true,from:{x:x1,y:y1},to:{x:x2,y:y2}})}
 if(op==='scroll'){var dy=Number(argv[1]),dx=Number(argv[2]);var e=$.CGEventCreateScrollWheelEvent(null,$.kCGScrollEventUnitPixel,2,dy,dx);$.CGEventPost($.kCGHIDEventTap,e);$.CFRelease(e);return JSON.stringify({scrolled:true,deltaX:dx,deltaY:dy})}
 throw new Error('unknown pointer operation')
}`

func macPointer(ctx context.Context, operation string, values ...float64) (any, error) {
	if !macAccessibilityTrusted(ctx) {
		return nil, errors.New("macOS Accessibility permission is required")
	}
	args := []string{operation}
	for _, value := range values {
		args = append(args, strconv.FormatFloat(value, 'f', -1, 64))
	}
	text, err := runOSA(ctx, "JavaScript", macPointerScript, args...)
	if err != nil {
		return nil, err
	}
	var out any
	_ = json.Unmarshal([]byte(text), &out)
	return out, nil
}

func macType(ctx context.Context, text string) (any, error) {
	if !macAccessibilityTrusted(ctx) {
		return nil, errors.New("macOS Accessibility permission is required")
	}
	script := `function run(argv){var s=Application('System Events');s.keystroke(String(argv[0]||''));return JSON.stringify({typed:true,characters:Array.from(String(argv[0]||'')).length})}`
	out, err := runOSA(ctx, "JavaScript", script, text)
	if err != nil {
		return nil, err
	}
	var value any
	_ = json.Unmarshal([]byte(out), &value)
	return value, nil
}

func macKey(ctx context.Context, key string) (any, error) {
	if !macAccessibilityTrusted(ctx) {
		return nil, errors.New("macOS Accessibility permission is required")
	}
	key = strings.TrimSpace(strings.ToLower(key))
	codes := map[string]int{"enter": 36, "return": 36, "tab": 48, "space": 49, "backspace": 51, "delete": 51, "escape": 53, "left": 123, "right": 124, "down": 125, "up": 126, "pagedown": 121, "pageup": 116, "home": 115, "end": 119}
	if code, ok := codes[key]; ok {
		script := fmt.Sprintf(`tell application "System Events" to key code %d`, code)
		if _, err := runOSA(ctx, "", script); err != nil {
			return nil, err
		}
		return map[string]any{"pressed": key}, nil
	}
	if len([]rune(key)) == 1 {
		return macType(ctx, key)
	}
	return nil, errors.New("unsupported macOS key; use a named navigation key or one character")
}

func platformHandle(ctx context.Context, input request) (any, error) {
	switch input.Operation {
	case "status":
		return platformCapabilities(), nil
	case "list_windows":
		return macWindows(ctx)
	case "ui_tree":
		return macUITree(ctx, stringValue(input.Arguments, "windowId"))
	case "screenshot":
		return macScreenshot(ctx, stringValue(input.Arguments, "windowId"))
	case "focus":
		return macFocus(ctx, stringValue(input.Arguments, "windowId"))
	case "click":
		if element := stringValue(input.Arguments, "elementId"); element != "" {
			return macElementClick(ctx, element)
		}
		x, okX := numberValue(input.Arguments, "x")
		y, okY := numberValue(input.Arguments, "y")
		if !okX || !okY {
			return nil, errors.New("computer_click requires elementId or x/y")
		}
		return macPointer(ctx, "click", x, y)
	case "type":
		return macType(ctx, stringValue(input.Arguments, "text"))
	case "key":
		return macKey(ctx, stringValue(input.Arguments, "key"))
	case "scroll":
		dx, _ := numberValue(input.Arguments, "deltaX")
		dy, _ := numberValue(input.Arguments, "deltaY")
		return macPointer(ctx, "scroll", dy, dx)
	case "drag":
		x1, ok1 := numberValue(input.Arguments, "fromX")
		y1, ok2 := numberValue(input.Arguments, "fromY")
		x2, ok3 := numberValue(input.Arguments, "toX")
		y2, ok4 := numberValue(input.Arguments, "toY")
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return nil, errors.New("computer_drag requires fromX/fromY/toX/toY")
		}
		return macPointer(ctx, "drag", x1, y1, x2, y2)
	default:
		return nil, errors.New("unsupported macOS Computer Use operation")
	}
}
