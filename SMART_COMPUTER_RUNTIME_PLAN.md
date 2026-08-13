# CodeLocal Smart Computer Runtime Plan

## Objective

Make ChatGPT + CodeLocal feel much closer to Codex Computer Use while preserving the existing user flow:

```bash
npm install -g codelocal
codelocal
```

The model remains responsible for intent, reasoning and planning. CodeLocal becomes a persistent local agent runtime that keeps workspace state warm and chooses the fastest execution backend for each action.

## Product principles

1. Do not turn every task into GUI automation.
2. Prefer structured interfaces before vision: DOM/CDP -> Accessibility/UI Automation -> visual grounding -> raw coordinates.
3. Reduce MCP round-trips by batching inspection, execution and verification locally.
4. Keep browser, computer, shell and filesystem as separate permission domains.
5. Keep all privileged desktop execution local on the paired user device.
6. Preserve the one-command CLI experience and package native helpers with the npm release.
7. Never make broad desktop control permanent by default.

## Target architecture

```text
ChatGPT
  |
  | compact MCP tools
  v
Smart MCP Layer
  |
  v
CodeLocal Orchestrator
  |-- Workspace/Task State
  |-- Execution Router
  |-- Verification/Recovery
  |
  +-- Code: context/LSP/edit/verify
  +-- Shell: terminal/process
  +-- Browser: DOM/CDP/Playwright
  +-- Computer: Accessibility/UIA/AT-SPI + vision fallback
  +-- Git
```

## Phase S0 - Foundation and safety

Status: mostly existing on dev.

- existing native helpers for macOS, Windows and Linux
- separate Computer Use opt-in
- browser and computer capability advertisement
- action-level authorization
- compact MCP surface
- persistent native ComputerController helper process
- browser session reuse

Release gate:

- no permission broadening
- no secrets in UI target logs
- secure desktop remains unsupported
- destructive actions still use existing authorizer

## Phase S1 - Smart native observation and semantic actions

Status: in progress on `feat/smart-computer-runtime`.

Deliverables:

- `computer action=observe`
  - one MCP action for fresh windows metadata
  - optional accessibility tree when `windowId` is supplied
- semantic click target
  - ChatGPT can send `target="Continue"`
  - CodeLocal resolves target locally against a fresh UI tree
  - raw `elementId` and coordinates remain fallback paths
- `verify=true`
  - execute an input action
  - immediately return a fresh post-action observation
  - removes a common extra MCP call
- deterministic semantic ranking
  - exact accessible name > description > value > role
  - interactive controls receive preference
  - disabled controls receive strong penalty
- tests for semantic ranking and compact MCP dispatch

Success criteria:

- common native UI click requires at most observe -> click+verify
- ChatGPT no longer has to manually traverse the full accessibility tree for obvious labels
- coordinates are not the default interaction path

## Phase S2 - Persistent task/workspace state

Goal: Chat conversations must not be the only working memory.

Add a local state store keyed by workspace + MCP session/task identity.

Suggested state:

```text
workspaceKey
sessionKey
taskSummary
activeBranch
touchedFiles
relevantSymbols
recentDiagnostics
recentCommands
recentTests
recentFailures
recentPatches
browserSession
computerWindow
lastObservationRevision
updatedAt
```

Requirements:

- bounded storage
- TTL for stale sessions
- no file contents or screenshots persisted by default
- no secrets
- cheap restore on a new MCP request
- state survives model context compression
- session state must not leak across ChatGPT conversations

Model-facing behavior:

- `context` should include a compact previous-task continuation hint when relevant
- repeated reads/searches should be avoided when source hashes have not changed
- test failures and patch attempts should remain available to verification/recovery logic

## Phase S3 - Hybrid execution router

Goal: choose the fastest backend instead of simulating a human for everything.

Decision priority:

```text
code/LSP/file operation
  -> shell/process
  -> browser DOM/CDP
  -> accessibility/UI automation
  -> visual grounding
  -> raw coordinate fallback
```

Examples:

- rename downloaded file -> filesystem, not Finder clicks
- inspect frontend console -> browser devtools, not visual browser interaction
- click native modal -> accessibility target
- canvas/drawing app -> visual/coordinate computer use

Router requirements:

- deterministic local fast paths for obvious cases
- model keeps final task-level decision authority
- local router must not silently cross permission domains
- return backend used and compact verification metadata

## Phase S4 - Faster coding loop

Goal: reduce tool calls and repeated repository work.

Work items:

- keep project index/LSP/session hot
- incremental context cache keyed by file hash
- dependency-neighborhood cache
- diagnostic snapshot reuse
- targeted test recommendation based on changed files/import graph
- run independent checks concurrently where safe
- classify failures: compile/type/test/runtime/environment/unrelated
- bounded auto-repair loop for mechanical failures

Target flow:

```text
context -> edit -> targeted verify/checks -> repair if mechanical -> broader verify
```

KPIs:

- 50%+ fewer MCP round-trips for common bug fixes
- no full repository rescan when workspace is unchanged
- targeted checks before full test suite
- no repeated read of unchanged files unless model explicitly asks

## Phase S5 - Browser runtime upgrade

- persistent Playwright/CDP session
- reuse real/managed browser profile according to policy
- DOM-first semantic action API
- observe/action/verify batching similar to Computer Use
- console/network state returned with relevant actions when requested
- screenshot only when structured browser state is insufficient

Potential reference implementations:

- OpenAI CUA sample app execution loop
- browser-use session/browser patterns

Do not embed an external Python agent runtime into the default npm UX.

## Phase S6 - Visual grounding fallback

Only used when DOM/accessibility data cannot resolve the task.

Observation pipeline:

```text
structured tree available? -> use it
otherwise screenshot -> grounding -> target bounding box -> action -> verify
```

Requirements:

- normalized coordinate system
- display/window geometry revision
- stale-coordinate protection
- bounded screenshot resolution
- no screenshot persistence by default
- allow future pluggable grounding providers/models

Potential references:

- Agent-S architecture
- OpenCUA grounding/data concepts

## Phase S7 - Virtual Agent Cursor UX

Goal: Codex-like visible agent interaction without making the user's physical pointer the only feedback mechanism.

Add a local overlay state stream:

```text
agent cursor position
target bounds
action label
click pulse
drag path
typing/wait state
```

Design:

- overlay is visual state owned by CodeLocal
- interpolate cursor animation locally for smoothness
- actual OS input remains separate
- hide/disable overlay on request
- visible active Computer Use indicator and emergency stop remain mandatory

Important: a virtual cursor is UX, not the grounding source.

## Phase S8 - Cross-platform hardening

macOS:

- Accessibility
- CoreGraphics/window capture
- permission doctor
- multi-display geometry

Windows:

- UI Automation
- SendInput/input backend
- DPI scaling
- elevation/secure-desktop boundaries

Linux:

- AT-SPI
- X11 supported path
- Wayland portal/session-specific restrictions
- report unsupported capabilities honestly

## Phase S9 - Packaging and release

User experience remains:

```bash
npm install -g codelocal
codelocal
```

Packaging requirements:

- native helper for target OS/arch bundled into npm release
- no Python requirement for normal users
- optional heavy grounding components are not bundled by default
- `codelocal doctor` reports missing OS permissions and unsupported backend
- safe feature flag rollout before making Smart Computer Runtime default

## Testing strategy

Unit:

- semantic target ranking
- capability gates
- compact MCP schema/resolution
- state TTL/isolation
- router decisions
- failure classification

Integration:

- fake ComputerController fixtures
- browser local deterministic pages
- workspace continuation across MCP sessions
- edit -> verify -> targeted test flow

Platform smoke:

- macOS Chrome/Finder/basic dialog
- Windows Chrome/Explorer/basic dialog
- supported Linux desktop session

Regression:

- existing coding tools unchanged
- npm entry point unchanged
- automation disabled behavior unchanged
- existing approval behavior unchanged

## Merge strategy

Do not merge all phases as one giant change.

Recommended PR sequence:

1. S1 semantic observation/actions
2. S2 task state
3. S3 execution router
4. S4 coding speed improvements
5. S5 browser batching
6. S6 visual fallback
7. S7 agent cursor
8. S8/S9 hardening and packaging

Each PR should be independently testable and backward compatible with the current compact MCP surface where possible.
