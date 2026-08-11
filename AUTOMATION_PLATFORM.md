# CodeLocal Automation Platform

## Product rule

The user installs CodeLocal once and then uses one command:

```bash
npm i -g codelocal
codelocal
```

Playwright and future native Computer Use helpers are implementation details. Users must not be required to install a separate MCP, Python package, Homebrew package, or global Playwright CLI.

## First-run UX

The no-argument `codelocal` entrypoint performs capability setup before acquiring or starting the machine runtime.

Interactive first run:

1. Coding is always enabled for explicitly authorized workspaces.
2. Browser Automation is offered with a default of Yes.
3. Computer Use is a separate opt-in and is only offered when the current graphical session can support the platform backend.
4. Browser provisioning happens only after Browser Automation consent.
5. Setup is saved locally, then the CodeLocal runtime starts.

Non-interactive first run never invents consent. It starts Coding-only for that process, does not persist setup choices, and leaves the interactive onboarding pending for the next normal `codelocal` terminal run.

If Playwright browser provisioning fails, Coding still starts. The Browser preference remains enabled but unprepared so a later `codelocal` launch can retry automatically without making the user reinstall anything.

## Capability boundaries

Automation is split into separate trust domains:

- Coding: authorized files, Git, guarded terminal, local MCP extensions.
- Browser: Playwright CLI session managed by CodeLocal.
- Computer: OS-native UI/screen/input adapter.

Browser and Computer must not be modeled as unrestricted shell access.

## Browser engine

CodeLocal packages `@playwright/cli` as an npm dependency and passes its package-local path into the native runtime. First-run provisioning installs Chromium only. Browser sessions are workspace-scoped and use CodeLocal-owned profiles/state rather than the user's normal browser profile by default.

The model-facing MCP surface exposes one compact `browser(action=...)` tool rather than arbitrary `playwright-cli` shell strings. Its private runtime operations are:

- `browser_status`
- `browser_open`
- `browser_snapshot`
- `browser_find`
- `browser_click`
- `browser_fill`
- `browser_press`
- `browser_console`
- `browser_requests`
- `browser_close`

Existing Chrome/Edge attachment is a later elevated capability and must not be the default profile mode.

## Computer backends

The product architecture is cross-platform:

- macOS: Accessibility + ScreenCaptureKit, with native input fallback.
- Windows: UI Automation + Windows Graphics Capture, with SendInput fallback. UAC/secure desktop is out of scope.
- Linux X11: AT-SPI/X11 adapter.
- Linux Wayland: xdg-desktop-portal ScreenCast/RemoteDesktop and compositor-granted capabilities.

Computer Use must advertise granular capabilities such as screen capture, UI tree, pointer, keyboard, clipboard, and background control instead of one misleading `computerControl=true` flag.

## Safety model

First-run consent only enables a capability domain. It does not permanently approve every action inside that domain.

CodeLocal should keep local policy authoritative and require fresh confirmation for sensitive actions such as payments, sending/publishing, permission changes, destructive cloud actions, credential access, or secure-desktop operations. Remembered approvals may cover routine scoped actions, but critical actions remain fresh-confirmation only.

Automation sessions should be revocable and time-bounded. Disconnecting or stopping CodeLocal must terminate active input control.

## Delivery phases

1. Smart first-run onboarding + npm-bundled Playwright provisioning.
2. Typed Browser tools and workspace-scoped browser sessions.
3. Computer observation: capability doctor, apps/windows, UI tree, screenshots.
4. macOS/Windows input adapters.
5. Linux X11/Wayland input adapters.
6. Elevated existing-browser attachment and multi-app workflows.

A capability must not be advertised as ready until its local backend is actually available.
