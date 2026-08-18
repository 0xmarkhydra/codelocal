# CodeLocal Desktop Automation Plan

> **Historical design (superseded by Computer Engine V3).** Keep this file as the original threat-model/product sketch. Current desktop-control implementation, acceptance and remaining work live in `COMPUTER_ENGINE_V3_MASTER_PLAN.md`; unchecked items below must not be interpreted as current release blockers without checking the V3 plan and source.

## Goal

After the coding core is production-ready, extend CodeLocal so ChatGPT can optionally observe and control the user's computer for developer workflows such as opening an app, clicking through a local UI, taking screenshots, reproducing a bug, testing a desktop/web app, and validating visual results.

Desktop control is a separate capability domain from coding. It must be disabled by default and explicitly enabled by the user on each device.

---

# Security model

Desktop automation can expose far more private information than repository access. Treat it as a high-privilege capability.

Principles:

- Disabled by default.
- Separate capability flag from shell/filesystem access.
- Require OS permissions such as screen capture/accessibility where applicable.
- Default to the active app/window, not the entire desktop, when possible.
- Screenshot capture and input events are locally audited.
- Sensitive apps/windows can be deny-listed.
- Password fields, secure input, keychains, credential dialogs and system security settings should not be automated by default.
- Clipboard reads/writes require a separate permission.
- No continuous screen recording by default.
- No background input control without a visible active session indicator.
- User can immediately revoke control locally.

Suggested config:

```json
{
  "desktopAutomation": {
    "enabled": false,
    "screenCapture": "approval",
    "inputControl": "approval",
    "clipboard": "deny",
    "allowedApps": [],
    "blockedApps": ["Password Manager", "Keychain Access"],
    "maxScreenshotWidth": 1920,
    "maxScreenshotHeight": 1080
  }
}
```

---

# Architecture

```text
ChatGPT
  |
  | MCP
  v
CodeLocal Gateway
  |
  | authenticated tool request
  v
CodeLocal Client
  |
  +-- Coding Core
  |
  +-- Desktop Automation Router
        |
        +-- macOS adapter
        +-- Windows adapter
        +-- Linux adapter
```

The gateway should never perform desktop automation itself. All screen/input operations happen on the paired local device.

---

# Capability advertisement

Client registration should advertise:

```text
desktop.screenCapture
desktop.windowList
desktop.inputControl
desktop.keyboard
desktop.mouse
desktop.clipboard
desktop.accessibilityTree
```

Each capability has a state:

```text
unavailable
disabled
approval
allowed
```

---

# MCP tools

## Observation

```text
desktop_info
list_displays
list_windows
get_active_window
screenshot
screenshot_window
screenshot_region
```

`screenshot` returns image bytes/reference plus metadata:

```text
displayId
windowId?
width
height
scale
capturedAt
activeApp
```

Prefer window/region screenshots over full-desktop screenshots when sufficient.

## Accessibility / UI structure

Where supported:

```text
get_ui_tree
find_ui_element
get_focused_element
```

This should be preferred over blind coordinate clicking because it is more reliable and uses less visual trial-and-error.

Normalized UI element shape:

```text
role
name
value?
bounds
enabled
focused
actions[]
```

## Mouse

```text
mouse_move
mouse_click
mouse_double_click
mouse_drag
mouse_scroll
```

Coordinate operations should include the target display/window and screenshot revision when possible to prevent stale clicks.

## Keyboard

```text
key_press
key_combo
type_text
```

`type_text` must not be used for secrets unless explicitly supplied by the user for that action.

## Window/app control

```text
focus_window
open_application
close_application
```

Potentially destructive actions such as force-quit require approval.

## Clipboard

Separate opt-in tools:

```text
clipboard_read
clipboard_write
```

Clipboard access is denied by default.

---

# Screenshot lifecycle

```text
screenshot request
-> local policy
-> optional local approval
-> OS capture API
-> downscale/compress if necessary
-> return to ChatGPT
-> do not persist by default
```

Screenshots should have a short-lived local ID so later actions can target the same visual state:

```text
screenshotId
capturedAt
windowId
geometryRevision
```

Input calls can optionally require `screenshotId` to reduce stale-coordinate mistakes.

---

# Visual interaction loop

Target workflow:

```text
User: test the login UI

ChatGPT
-> open_application/browser
-> focus_window
-> screenshot_window
-> inspect UI / accessibility tree
-> click login button
-> screenshot_window
-> type test credentials supplied for this test
-> click submit
-> screenshot_window
-> inspect result
-> report or edit code
-> rebuild/restart app
-> repeat
```

This allows CodeLocal to bridge coding and actual UI validation.

---

# Platform adapters

## macOS

Adapter responsibilities:

- screen/window capture
- window enumeration
- accessibility/UI tree
- mouse/keyboard event injection
- application activation
- permission diagnostics

Expose a `desktop_doctor` check so the user can see whether required Screen Recording / Accessibility permissions are granted.

## Windows

Adapter responsibilities:

- screen/window capture
- UI Automation tree
- input injection
- window focus/enumeration
- permission/elevation diagnostics

## Linux

Support will depend on session type and desktop environment.

Adapter responsibilities:

- X11/Wayland-aware screenshot capture
- portal-based capture when necessary
- accessibility integration where available
- input injection only where the desktop/session permits it

Do not claim full Linux desktop control unless the active desktop/session backend is actually supported.

---

# Approval policy

Suggested risk classes:

```text
SAFE
- list displays/windows
- get active window

REVIEW
- screenshot active application
- focus window
- mouse move

HIGH
- full desktop screenshot
- keyboard typing
- mouse click/drag
- open/close app
- clipboard read/write

BLOCKED by default
- password manager automation
- keychain/credential dialogs
- system security preference changes
- typing into detected secure password fields
```

A user may grant a temporary session-level allowance, for example:

```text
Allow mouse/keyboard control for Chrome for 15 minutes
```

Do not make permanent broad desktop control the default.

---

# Audit

Human-readable local audit examples:

```text
[Desktop] screenshot_window
App: Chrome
Window: BIDDI - localhost
Decision: allowed

[Desktop] mouse_click
Target: Chrome
Position: 842, 611
Screenshot: shot_abc123
Decision: approved
```

Structured audit fields:

```text
requestId
sessionId
deviceId
operation
app
windowId
riskLevel
approvalId?
result
```

Do not log screenshot image content in normal logs.

---

# Reliability

- Attach geometry/window revisions to coordinate actions.
- Reject actions targeting closed/moved windows when stale state can be detected.
- Rate-limit input actions.
- Provide `desktop_stop` emergency cancellation.
- Stop automation when the local user takes over input if feasible.
- Cancel all active desktop permissions when the client disconnects or user locks the machine.

---

# Development phases

## D0 — Capability and policy layer

- desktop capability advertisement
- desktop config
- approval integration
- `desktop_info`
- `desktop_doctor`

## D1 — Screenshots

- display/window enumeration
- active window detection
- screenshot full display
- screenshot window
- screenshot region
- image size limits
- ephemeral screenshot IDs

## D2 — Accessibility tree

- UI element inspection
- element lookup by role/name
- focused element
- app/window targeting

## D3 — Input control

- mouse move/click/drag/scroll
- keyboard key press/combo/type
- stale screenshot/window protection
- emergency stop

## D4 — Application workflow

- focus/open/close application
- browser/app testing workflows
- screenshot -> action -> screenshot loops

## D5 — Cross-platform hardening

- macOS validation
- Windows validation
- supported Linux backends
- permissions/setup documentation
- audit/security regression tests

---

# Release gate

Desktop automation must remain experimental until:

- [ ] separate explicit opt-in from coding permissions
- [ ] screenshot permissions validated
- [ ] input permissions validated
- [ ] local approval and emergency stop work
- [ ] screenshots are not persisted by default
- [ ] sensitive-app/secure-field protections exist
- [ ] stale-coordinate safeguards exist
- [ ] audit logs do not leak screenshot contents or secrets
- [ ] macOS and Windows smoke tests pass

Desktop automation should ship as an optional capability after the main CodeLocal coding core reaches production readiness.
