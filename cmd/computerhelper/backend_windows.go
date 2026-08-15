//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func windowsPowerShell(ctx context.Context, script string, env map[string]string) (string, error) {
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
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

const windowsNativePrelude = `
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class CodeLocalWin32 {
 [StructLayout(LayoutKind.Sequential)] public struct RECT { public int Left,Top,Right,Bottom; }
 [StructLayout(LayoutKind.Sequential)] public struct INPUT { public uint type; public InputUnion U; }
 [StructLayout(LayoutKind.Explicit)] public struct InputUnion { [FieldOffset(0)] public MOUSEINPUT mi; [FieldOffset(0)] public KEYBDINPUT ki; }
 [StructLayout(LayoutKind.Sequential)] public struct MOUSEINPUT { public int dx,dy; public uint mouseData,dwFlags,time; public UIntPtr dwExtraInfo; }
 [StructLayout(LayoutKind.Sequential)] public struct KEYBDINPUT { public ushort wVk,wScan; public uint dwFlags,time; public UIntPtr dwExtraInfo; }
 [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr hWnd,out RECT rect);
 [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hWnd);
 [DllImport("user32.dll")] public static extern bool SetCursorPos(int x,int y);
 [DllImport("user32.dll",SetLastError=true)] public static extern uint SendInput(uint nInputs, INPUT[] pInputs, int cbSize);
 public const uint INPUT_MOUSE=0, INPUT_KEYBOARD=1;
 public const uint MOUSEEVENTF_MOVE=0x0001, MOUSEEVENTF_LEFTDOWN=0x0002, MOUSEEVENTF_LEFTUP=0x0004, MOUSEEVENTF_WHEEL=0x0800, MOUSEEVENTF_HWHEEL=0x01000;
 public const uint KEYEVENTF_KEYUP=0x0002, KEYEVENTF_UNICODE=0x0004;
 public static uint Mouse(uint flags,uint data=0){ var i=new INPUT(); i.type=INPUT_MOUSE; i.U.mi.dwFlags=flags; i.U.mi.mouseData=data; return SendInput(1,new[]{i},Marshal.SizeOf(typeof(INPUT))); }
 public static uint Key(ushort vk,bool up){ var i=new INPUT(); i.type=INPUT_KEYBOARD; i.U.ki.wVk=vk; i.U.ki.dwFlags=up?KEYEVENTF_KEYUP:0; return SendInput(1,new[]{i},Marshal.SizeOf(typeof(INPUT))); }
 public static uint UnicodeChar(char ch,bool up){ var i=new INPUT(); i.type=INPUT_KEYBOARD; i.U.ki.wScan=ch; i.U.ki.dwFlags=KEYEVENTF_UNICODE|(up?KEYEVENTF_KEYUP:0); return SendInput(1,new[]{i},Marshal.SizeOf(typeof(INPUT))); }
}
'@
`

func platformCapabilities() map[string]any {
	return map[string]any{
		"available":         true,
		"backend":           "windows-uia+win32-capture+sendinput",
		"screenCapture":     true,
		"uiTree":            true,
		"pointer":           true,
		"keyboard":          true,
		"clipboard":         false,
		"backgroundControl": true,
		"secureDesktop":     false,
		"notes":             []string{"Windows UIPI is respected; CodeLocal cannot inject input into higher-integrity or secure-desktop surfaces."},
	}
}

func windowsJSON(text string) (any, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return []any{}, nil
	}
	var out any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return nil, fmt.Errorf("decode Windows helper output: %w: %s", err, text)
	}
	return out, nil
}

func windowsList(ctx context.Context) (any, error) {
	script := windowsNativePrelude + `
$items=@(); Get-Process | Where-Object {$_.MainWindowHandle -ne 0} | ForEach-Object {
 $r=New-Object CodeLocalWin32+RECT; [void][CodeLocalWin32]::GetWindowRect($_.MainWindowHandle,[ref]$r);
 $items += [pscustomobject]@{windowId=$_.MainWindowHandle.ToInt64().ToString();pid=$_.Id;app=$_.ProcessName;title=$_.MainWindowTitle;bounds=[pscustomobject]@{x=$r.Left;y=$r.Top;width=$r.Right-$r.Left;height=$r.Bottom-$r.Top}}
}; $items | ConvertTo-Json -Depth 5 -Compress
`
	text, err := windowsPowerShell(ctx, script, nil)
	if err != nil {
		return nil, err
	}
	return windowsJSON(text)
}

const windowsUIAPrelude = `
Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes
function RuntimeKey($e){ try { return (($e.GetRuntimeId() | ForEach-Object {$_.ToString()}) -join '.') } catch { return '' } }
function Node($e,$walker,[ref]$count,$limit,$depth){
 if($null -eq $e -or $count.Value -ge $limit -or $depth -gt 8){return $null}; $count.Value++;
 $n=[pscustomobject]@{elementId=(RuntimeKey $e);name=$e.Current.Name;automationId=$e.Current.AutomationId;className=$e.Current.ClassName;controlType=$e.Current.ControlType.ProgrammaticName;enabled=$e.Current.IsEnabled;offscreen=$e.Current.IsOffscreen;children=@()};
 $child=$walker.GetFirstChild($e); while($null -ne $child -and $count.Value -lt $limit){$cn=Node $child $walker ([ref]$count.Value) $limit ($depth+1);if($null-ne$cn){$n.children+=@($cn)};$child=$walker.GetNextSibling($child)}; return $n
}
function FindRuntime($root,$key){$walker=[System.Windows.Automation.TreeWalker]::RawViewWalker;$stack=New-Object System.Collections.Stack;$stack.Push($root);$seen=0;while($stack.Count -gt 0 -and $seen -lt 5000){$e=$stack.Pop();$seen++;if((RuntimeKey $e) -eq $key){return $e};$child=$walker.GetFirstChild($e);while($null-ne$child){$stack.Push($child);$child=$walker.GetNextSibling($child)}};return $null}
`

func windowsUITree(ctx context.Context, windowID string) (any, error) {
	if strings.TrimSpace(windowID) == "" {
		return nil, errors.New("windowId is required")
	}
	script := windowsUIAPrelude + `
$h=[IntPtr]::new([Int64]$env:CODELOCAL_WINDOW);$root=[System.Windows.Automation.AutomationElement]::FromHandle($h);if($null-eq$root){throw 'window not found'};$walker=[System.Windows.Automation.TreeWalker]::RawViewWalker;$count=0;$node=Node $root $walker ([ref]$count) 500 0;[pscustomobject]@{windowId=$env:CODELOCAL_WINDOW;node=$node;truncated=($count-ge 500)}|ConvertTo-Json -Depth 20 -Compress
`
	text, err := windowsPowerShell(ctx, script, map[string]string{"CODELOCAL_WINDOW": windowID})
	if err != nil {
		return nil, err
	}
	return windowsJSON(text)
}

func windowsScreenshot(ctx context.Context, windowID string) (any, error) {
	script := windowsNativePrelude + `
Add-Type -AssemblyName System.Drawing
if($env:CODELOCAL_WINDOW){$h=[IntPtr]::new([Int64]$env:CODELOCAL_WINDOW);$r=New-Object CodeLocalWin32+RECT;if(-not [CodeLocalWin32]::GetWindowRect($h,[ref]$r)){throw 'window not found'};$x=$r.Left;$y=$r.Top;$w=$r.Right-$r.Left;$hgt=$r.Bottom-$r.Top}else{$b=[System.Windows.Forms.SystemInformation]::VirtualScreen;$x=$b.X;$y=$b.Y;$w=$b.Width;$hgt=$b.Height}
if($w-le 0 -or $hgt-le 0){throw 'invalid capture bounds'};$bmp=New-Object System.Drawing.Bitmap $w,$hgt;$g=[System.Drawing.Graphics]::FromImage($bmp);$g.CopyFromScreen($x,$y,0,0,$bmp.Size);$ms=New-Object System.IO.MemoryStream;$bmp.Save($ms,[System.Drawing.Imaging.ImageFormat]::Png);$g.Dispose();$bmp.Dispose();$data=[Convert]::ToBase64String($ms.ToArray());$ms.Dispose();[pscustomobject]@{windowId=$env:CODELOCAL_WINDOW;__mcpImage=[pscustomobject]@{mimeType='image/png';data=$data}}|ConvertTo-Json -Depth 4 -Compress
`
	// SystemInformation lives in Windows.Forms; add it without using SendKeys.
	script = "Add-Type -AssemblyName System.Windows.Forms\n" + script
	text, err := windowsPowerShell(ctx, script, map[string]string{"CODELOCAL_WINDOW": windowID})
	if err != nil {
		return nil, err
	}
	return windowsJSON(text)
}

func windowsFocus(ctx context.Context, windowID string) (any, error) {
	script := windowsNativePrelude + `$h=[IntPtr]::new([Int64]$env:CODELOCAL_WINDOW);$ok=[CodeLocalWin32]::SetForegroundWindow($h);[pscustomobject]@{focused=$ok;windowId=$env:CODELOCAL_WINDOW}|ConvertTo-Json -Compress`
	text, err := windowsPowerShell(ctx, script, map[string]string{"CODELOCAL_WINDOW": windowID})
	if err != nil {
		return nil, err
	}
	return windowsJSON(text)
}

func windowsInvoke(ctx context.Context, windowID, elementID string) (any, error) {
	if windowID == "" || elementID == "" {
		return nil, errors.New("windowId and elementId are required for UIA invoke")
	}
	script := windowsUIAPrelude + `
$h=[IntPtr]::new([Int64]$env:CODELOCAL_WINDOW);$root=[System.Windows.Automation.AutomationElement]::FromHandle($h);$e=FindRuntime $root $env:CODELOCAL_ELEMENT;if($null-eq$e){throw 'UI element is stale or not found'};$p=$null;if($e.TryGetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern,[ref]$p)){([System.Windows.Automation.InvokePattern]$p).Invoke();[pscustomobject]@{invoked=$true;elementId=$env:CODELOCAL_ELEMENT}|ConvertTo-Json -Compress}else{throw 'element has no InvokePattern; use approved coordinates'}`
	text, err := windowsPowerShell(ctx, script, map[string]string{"CODELOCAL_WINDOW": windowID, "CODELOCAL_ELEMENT": elementID})
	if err != nil {
		return nil, err
	}
	return windowsJSON(text)
}

func windowsClick(ctx context.Context, x, y float64) (any, error) {
	script := windowsNativePrelude + `$x=[int]$env:CODELOCAL_X;$y=[int]$env:CODELOCAL_Y;if(-not [CodeLocalWin32]::SetCursorPos($x,$y)){throw 'SetCursorPos failed'};if([CodeLocalWin32]::Mouse([CodeLocalWin32]::MOUSEEVENTF_LEFTDOWN)-eq 0){throw 'SendInput mouse down blocked'};if([CodeLocalWin32]::Mouse([CodeLocalWin32]::MOUSEEVENTF_LEFTUP)-eq 0){throw 'SendInput mouse up blocked'};[pscustomobject]@{clicked=$true;x=$x;y=$y}|ConvertTo-Json -Compress`
	text, err := windowsPowerShell(ctx, script, map[string]string{"CODELOCAL_X": strconv.Itoa(int(x)), "CODELOCAL_Y": strconv.Itoa(int(y))})
	if err != nil {
		return nil, err
	}
	return windowsJSON(text)
}

func windowsType(ctx context.Context, textValue string) (any, error) {
	script := windowsNativePrelude + `$text=$env:CODELOCAL_TEXT;foreach($ch in $text.ToCharArray()){if([CodeLocalWin32]::UnicodeChar([char]$ch,$false)-eq 0){throw 'SendInput unicode key blocked'};if([CodeLocalWin32]::UnicodeChar([char]$ch,$true)-eq 0){throw 'SendInput unicode key release blocked'}};[pscustomobject]@{typed=$true;characters=$text.Length}|ConvertTo-Json -Compress`
	text, err := windowsPowerShell(ctx, script, map[string]string{"CODELOCAL_TEXT": textValue})
	if err != nil {
		return nil, err
	}
	return windowsJSON(text)
}

func windowsKey(ctx context.Context, key string) (any, error) {
	key = strings.ToLower(strings.TrimSpace(key))
	virtualKeys := map[string]uint16{"enter": 0x0D, "return": 0x0D, "tab": 0x09, "escape": 0x1B, "space": 0x20, "backspace": 0x08, "delete": 0x2E, "left": 0x25, "up": 0x26, "right": 0x27, "down": 0x28, "pageup": 0x21, "pagedown": 0x22, "home": 0x24, "end": 0x23}
	vk, ok := virtualKeys[key]
	if !ok && len([]rune(key)) == 1 {
		r := []rune(strings.ToUpper(key))[0]
		if r >= 'A' && r <= 'Z' {
			vk = uint16(r)
			ok = true
		}
		if r >= '0' && r <= '9' {
			vk = uint16(r)
			ok = true
		}
	}
	if !ok {
		return nil, errors.New("unsupported Windows key")
	}
	script := windowsNativePrelude + `$vk=[uint16]$env:CODELOCAL_VK;if([CodeLocalWin32]::Key($vk,$false)-eq 0){throw 'SendInput key blocked'};if([CodeLocalWin32]::Key($vk,$true)-eq 0){throw 'SendInput key release blocked'};[pscustomobject]@{pressed=$env:CODELOCAL_KEY}|ConvertTo-Json -Compress`
	text, err := windowsPowerShell(ctx, script, map[string]string{"CODELOCAL_VK": strconv.Itoa(int(vk)), "CODELOCAL_KEY": key})
	if err != nil {
		return nil, err
	}
	return windowsJSON(text)
}

func windowsScroll(ctx context.Context, dx, dy float64) (any, error) {
	script := windowsNativePrelude + `$dx=[int]$env:CODELOCAL_DX;$dy=[int]$env:CODELOCAL_DY;if($dy-ne 0 -and [CodeLocalWin32]::Mouse([CodeLocalWin32]::MOUSEEVENTF_WHEEL,[uint32]$dy)-eq 0){throw 'SendInput wheel blocked'};if($dx-ne 0 -and [CodeLocalWin32]::Mouse([CodeLocalWin32]::MOUSEEVENTF_HWHEEL,[uint32]$dx)-eq 0){throw 'SendInput horizontal wheel blocked'};[pscustomobject]@{scrolled=$true;deltaX=$dx;deltaY=$dy}|ConvertTo-Json -Compress`
	text, err := windowsPowerShell(ctx, script, map[string]string{"CODELOCAL_DX": strconv.Itoa(int(dx)), "CODELOCAL_DY": strconv.Itoa(int(dy))})
	if err != nil {
		return nil, err
	}
	return windowsJSON(text)
}

func windowsDrag(ctx context.Context, x1, y1, x2, y2 float64) (any, error) {
	script := windowsNativePrelude + `$x1=[int]$env:X1;$y1=[int]$env:Y1;$x2=[int]$env:X2;$y2=[int]$env:Y2;[void][CodeLocalWin32]::SetCursorPos($x1,$y1);if([CodeLocalWin32]::Mouse([CodeLocalWin32]::MOUSEEVENTF_LEFTDOWN)-eq 0){throw 'SendInput drag blocked'};Start-Sleep -Milliseconds 40;[void][CodeLocalWin32]::SetCursorPos($x2,$y2);Start-Sleep -Milliseconds 40;if([CodeLocalWin32]::Mouse([CodeLocalWin32]::MOUSEEVENTF_LEFTUP)-eq 0){throw 'SendInput drag release blocked'};[pscustomobject]@{dragged=$true;from=[pscustomobject]@{x=$x1;y=$y1};to=[pscustomobject]@{x=$x2;y=$y2}}|ConvertTo-Json -Depth 3 -Compress`
	env := map[string]string{"X1": strconv.Itoa(int(x1)), "Y1": strconv.Itoa(int(y1)), "X2": strconv.Itoa(int(x2)), "Y2": strconv.Itoa(int(y2))}
	text, err := windowsPowerShell(ctx, script, env)
	if err != nil {
		return nil, err
	}
	return windowsJSON(text)
}

func platformHandle(ctx context.Context, input request) (any, error) {
	switch input.Operation {
	case "status":
		return platformCapabilities(), nil
	case "list_windows":
		return windowsList(ctx)
	case "ui_tree":
		return windowsUITree(ctx, stringValue(input.Arguments, "windowId"))
	case "screenshot":
		return windowsScreenshot(ctx, stringValue(input.Arguments, "windowId"))
	case "focus":
		return windowsFocus(ctx, stringValue(input.Arguments, "windowId"))
	case "click":
		if element := stringValue(input.Arguments, "elementId"); element != "" {
			return windowsInvoke(ctx, stringValue(input.Arguments, "windowId"), element)
		}
		x, okX := numberValue(input.Arguments, "x")
		y, okY := numberValue(input.Arguments, "y")
		if !okX || !okY {
			return nil, errors.New("computer_click requires elementId+windowId or x/y")
		}
		return windowsClick(ctx, x, y)
	case "type":
		return windowsType(ctx, stringValue(input.Arguments, "text"))
	case "key":
		return windowsKey(ctx, stringValue(input.Arguments, "key"))
	case "scroll":
		dx, _ := numberValue(input.Arguments, "deltaX")
		dy, _ := numberValue(input.Arguments, "deltaY")
		return windowsScroll(ctx, dx, dy)
	case "drag":
		x1, o1 := numberValue(input.Arguments, "fromX")
		y1, o2 := numberValue(input.Arguments, "fromY")
		x2, o3 := numberValue(input.Arguments, "toX")
		y2, o4 := numberValue(input.Arguments, "toY")
		if !o1 || !o2 || !o3 || !o4 {
			return nil, errors.New("computer_drag requires fromX/fromY/toX/toY")
		}
		return windowsDrag(ctx, x1, y1, x2, y2)
	default:
		return nil, errors.New("unsupported Windows Computer Use operation")
	}
}
