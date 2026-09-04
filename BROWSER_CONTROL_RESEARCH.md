# CodeLocal Browser Control Research

Branch: `dev-browser-control`

Goal: give ChatGPT browser observation and interaction through MCP while keeping ChatGPT as the planner/orchestrator. CodeLocal must not run an autonomous browser agent loop.

## What we are borrowing from Browser Use / Browser Harness

### 1. One long-lived CDP websocket

Browser Harness keeps one browser-level DevTools websocket alive instead of reconnecting for every action. This matters on modern Chrome because a user-approved remote-debugging attach can be scoped to the live browser instance and repeated reconnects can cause repeated permission prompts.

CodeLocal adopts the same lifecycle idea in `src/browser-control.ts`.

### 2. Attach to the user's real Chromium browser

For supported Chromium-family browsers, CodeLocal scans profile roots for `DevToolsActivePort` and supports both:

- `/json/version` discovery;
- direct `DevToolsActivePort` websocket path fallback for newer Chrome builds where HTTP discovery can be unavailable on the default profile.

The user enables `chrome://inspect/#remote-debugging` in the browser instance and approves the local attach prompt when Chrome requires it.

CodeLocal does not launch a separate automation profile by default.

### 3. Accessibility tree first

Browser Use and Browser Harness prefer the Chrome accessibility tree for semantic interaction. CodeLocal follows that model:

- `Accessibility.getFullAXTree`;
- role/name/description;
- `backendDOMNodeId` for stable interaction;
- only fall back to coordinates when semantic targeting is not available.

This is better for ChatGPT than screenshots alone because MCP can return a compact structured UI tree.

### 4. Coordinates are derived from the element box

For an AX node with `backendDOMNodeId`, CodeLocal can call `DOM.getBoxModel`, calculate the center in viewport CSS pixels, and then dispatch mouse events through CDP.

This works through the browser compositor and avoids OS-level mouse automation for normal web content.

### 5. Screenshot is a first-class observation

`Page.captureScreenshot` returns PNG bytes. The local controller returns an MCP-image marker so the gateway can later surface it as an actual MCP image content item instead of a filesystem path.

### 6. Event-driven lifecycle, not agent autonomy

Browser Use has BrowserSession events and watchdogs for navigation, downloads, tabs, DOM state, and security. CodeLocal should port the event/lifecycle ideas, not the LLM agent loop.

Planned local event topics:

- `browser.connected`;
- `browser.disconnected`;
- `browser.tab_created`;
- `browser.tab_closed`;
- `browser.navigation_started`;
- `browser.navigation_finished`;
- `browser.console`;
- `browser.network_error`;
- `browser.download_started`;
- `browser.download_finished`.

These are observations. Side effects still require explicit MCP calls from ChatGPT.

### 7. Navigation security

Browser Use has pre-navigation and post-redirect checks, including allow/block domain handling and IP-address bypass protection. CodeLocal should port this idea into its existing policy layer.

Development defaults should make localhost/loopback frictionless while treating external navigation, uploads, downloads, clipboard access, and sensitive form entry as stronger actions.

## What we are NOT copying

- Browser Use `Agent(...)` loops;
- Browser Use system prompts;
- Browser Use LLM provider layer;
- autonomous retries that cause hidden user-visible actions;
- captcha / stealth / proxy cloud features;
- password manager integration;
- cookie extraction as a default capability;
- arbitrary browsing outside an MCP call.

ChatGPT remains the only planner.

## Default-browser-first design

`browser_open(url)` should always use the operating system's default browser opener.

After the default browser opens the URL:

1. if that browser is a supported Chromium-family browser with local remote debugging enabled, CodeLocal attaches over CDP and exposes full semantic tools;
2. if it is Safari or another unsupported browser, CodeLocal will later fall back to the macOS Accessibility + Screen Capture adapter;
3. CodeLocal must never silently open a second automation browser just because the default browser is unsupported.

## MCP surface target

Read-only observations:

- `browser_status`
- `browser_tabs`
- `browser_snapshot`
- `browser_screenshot`
- `browser_console`
- `browser_network_errors`

Explicit actions:

- `browser_open`
- `browser_select_tab`
- `browser_navigate`
- `browser_click`
- `browser_type`
- `browser_scroll`
- `browser_reload`

Later reviewed actions:

- `browser_upload`
- `browser_download`
- `browser_clipboard_read`
- `browser_clipboard_write`
- arbitrary JS execution

## Current foundation

`src/browser-control.ts` currently implements:

- Chromium profile discovery on macOS/Linux/Windows;
- modern `DevToolsActivePort` discovery and websocket fallback;
- one long-lived raw CDP connection;
- real tab listing and selection;
- OS-default URL opening;
- AX-tree snapshots;
- PNG screenshots;
- semantic element clicks through `backendDOMNodeId`;
- direct text insertion;
- in-tab navigation;
- no cookie/password/clipboard/upload API.

This file is intentionally not exposed through MCP yet. It must first pass CI and then be wired through the existing CodeLocal permission/idempotency/audit/gateway layers.

## Next integration gate

Before merging into `dev`:

1. compile and unit-test on macOS/Linux/Windows;
2. wire browser capability into the local client;
3. add MCP tool schemas and image content transport;
4. add browser policy decisions and audit events;
5. real E2E on the user's default browser with `http://localhost`;
6. ensure a single attach does not repeatedly trigger Chrome permission prompts;
7. confirm browser tools cannot read credential stores, cookies, clipboard, or password fields by default.

## Attribution

The architecture was informed by the MIT-licensed `browser-use/browser-use` and `browser-use/browser-harness` projects. CodeLocal's implementation is TypeScript and is written for its own MCP architecture rather than copying their Python agent runtime.
