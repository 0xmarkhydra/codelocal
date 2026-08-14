//go:build darwin

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

var macCursorState struct {
	sync.Mutex
	hasPosition bool
	x           float64
	y           float64
	process     *exec.Cmd
}

const macElementCenterScript = `function run(argv){
 var pid=Number(argv[0]);
 var path=String(argv[1]||'').split('.').filter(function(x){return x!==''}).map(Number);
 var s=Application('System Events');
 var ps=s.applicationProcesses.whose({unixId:pid})();
 if(!ps.length) throw new Error('process not found');
 var e=ps[0];
 for(var i=0;i<path.length;i++){
   var list=(i===0?e.windows():e.uiElements());
   e=list[path[i]];
   if(!e) throw new Error('element path stale');
 }
 var p=e.position(), z=e.size();
 var x=Number(p[0]), y=Number(p[1]), w=Number(z[0]), h=Number(z[1]);
 if(!isFinite(x)||!isFinite(y)||!isFinite(w)||!isFinite(h)||w<=0||h<=0) throw new Error('element has no usable bounds');
 return JSON.stringify({x:x+w/2,y:y+h/2,width:w,height:h});
}`

type macElementCenterResult struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

func macElementCenter(ctx context.Context, elementID string) (macElementCenterResult, error) {
	parts := strings.SplitN(strings.TrimSpace(elementID), ":", 2)
	if len(parts) != 2 {
		return macElementCenterResult{}, errors.New("invalid elementId for agent cursor")
	}
	pid, err := strconv.Atoi(parts[0])
	if err != nil || pid <= 0 {
		return macElementCenterResult{}, errors.New("invalid elementId pid for agent cursor")
	}
	text, err := runOSA(ctx, "JavaScript", macElementCenterScript, strconv.Itoa(pid), parts[1])
	if err != nil {
		return macElementCenterResult{}, err
	}
	var center macElementCenterResult
	if err := json.Unmarshal([]byte(text), &center); err != nil {
		return macElementCenterResult{}, fmt.Errorf("decode agent cursor target: %w", err)
	}
	return center, nil
}

const macAgentCursorOverlayScript = `ObjC.import('Cocoa');
function pump(seconds){$.NSRunLoop.currentRunLoop.runUntilDate($.NSDate.dateWithTimeIntervalSinceNow(seconds));}
function run(argv){
 var x0=Number(argv[0]), y0=Number(argv[1]), x1=Number(argv[2]), y1=Number(argv[3]);
 var duration=Math.max(80,Math.min(400,Number(argv[4])||180));
 var screens=$.NSScreen.screens;
 if(Number(screens.count)!==1) return JSON.stringify({visible:false,reason:'agent cursor overlay is disabled for multi-display sessions'});
 var screen=screens.objectAtIndex(0).frame;
 var height=Number(screen.size.height), size=30;
 var app=$.NSApplication.sharedApplication;
 app.setActivationPolicy($.NSApplicationActivationPolicyAccessory);
 function origin(x,y){return $.NSMakePoint(x-size/2,height-y-size/2);}
 var rect=$.NSMakeRect(x0-size/2,height-y0-size/2,size,size);
 var win=$.NSWindow.alloc.initWithContentRectStyleMaskBackingDefer(rect,$.NSWindowStyleMaskBorderless,$.NSBackingStoreBuffered,false);
 win.setOpaque(false); win.setBackgroundColor($.NSColor.clearColor); win.setHasShadow(false); win.setIgnoresMouseEvents(true); win.setLevel($.NSFloatingWindowLevel);
 var marker=$.NSTextField.alloc.initWithFrame($.NSMakeRect(0,0,size,size));
 marker.setStringValue('◆'); marker.setBordered(false); marker.setBezeled(false); marker.setEditable(false); marker.setSelectable(false); marker.setDrawsBackground(false); marker.setAlignment($.NSTextAlignmentCenter); marker.setFont($.NSFont.boldSystemFontOfSize(20));
 win.setContentView(marker); win.orderFrontRegardless;
 var steps=14;
 for(var i=1;i<=steps;i++){
   var t=i/steps, e=1-Math.pow(1-t,3);
   var x=x0+(x1-x0)*e, y=y0+(y1-y0)*e;
   win.setFrameOrigin(origin(x,y));
   pump(duration/steps/1000);
 }
 marker.setStringValue('◉'); pump(0.22); win.orderOut(null);
 return JSON.stringify({visible:true,x:x1,y:y1});
}`

func startMacCursorOverlay(startX, startY, targetX, targetY float64, duration time.Duration) (*exec.Cmd, error) {
	args := []string{
		"-l", "JavaScript", "-e", macAgentCursorOverlayScript, "--",
		strconv.FormatFloat(startX, 'f', -1, 64),
		strconv.FormatFloat(startY, 'f', -1, 64),
		strconv.FormatFloat(targetX, 'f', -1, 64),
		strconv.FormatFloat(targetY, 'f', -1, 64),
		strconv.FormatInt(duration.Milliseconds(), 10),
	}
	cmd := exec.Command("/usr/bin/osascript", args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func platformCursor(ctx context.Context, input request) (any, error) {
	if !macAccessibilityTrusted(ctx) {
		return map[string]any{"visible": false, "reason": "macOS Accessibility permission is required"}, nil
	}
	var targetX, targetY float64
	if elementID := stringValue(input.Arguments, "elementId"); strings.TrimSpace(elementID) != "" {
		center, err := macElementCenter(ctx, elementID)
		if err != nil {
			return map[string]any{"visible": false, "reason": err.Error()}, nil
		}
		targetX, targetY = center.X, center.Y
	} else {
		x, okX := numberValue(input.Arguments, "x")
		y, okY := numberValue(input.Arguments, "y")
		if !okX || !okY {
			return nil, errors.New("agent cursor requires elementId or x/y")
		}
		targetX, targetY = x, y
	}

	duration := 180 * time.Millisecond
	macCursorState.Lock()
	startX, startY := targetX-48, targetY-36
	if macCursorState.hasPosition {
		startX, startY = macCursorState.x, macCursorState.y
	}
	if macCursorState.process != nil && macCursorState.process.Process != nil {
		_ = macCursorState.process.Process.Kill()
		macCursorState.process = nil
	}
	cmd, err := startMacCursorOverlay(startX, startY, targetX, targetY, duration)
	if err != nil {
		macCursorState.Unlock()
		return map[string]any{"visible": false, "reason": err.Error()}, nil
	}
	macCursorState.process = cmd
	macCursorState.hasPosition = true
	macCursorState.x, macCursorState.y = targetX, targetY
	macCursorState.Unlock()

	done := make(chan error, 1)
	go func(process *exec.Cmd) {
		err := process.Wait()
		macCursorState.Lock()
		if macCursorState.process == process {
			macCursorState.process = nil
		}
		macCursorState.Unlock()
		done <- err
	}(cmd)

	// A script that exits before the animation window means the overlay was not
	// actually visible (for example multi-display guard or a JXA/AppKit error).
	timer := time.NewTimer(duration + 30*time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return map[string]any{"visible": false, "reason": ctx.Err().Error()}, nil
	case err := <-done:
		reason := "agent cursor overlay exited before animation completed"
		if err != nil {
			reason = err.Error()
		}
		return map[string]any{"visible": false, "reason": reason}, nil
	case <-timer.C:
		return map[string]any{"visible": true, "x": targetX, "y": targetY, "independent": true}, nil
	}
}
