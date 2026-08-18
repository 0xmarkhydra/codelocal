# CodeLocal Computer Engine v3 — Background Agent Master Plan

Status: Active implementation blueprint
Date: 2026-08-18
Target branch: `dev`
Primary goal: Codex-like smooth desktop control with zero user interference on the normal path.

## 0. Product invariant

The user owns the physical computer session.

CodeLocal must prefer background-native interaction so the user can keep moving the mouse, typing, switching apps, and working normally while an agent operates another supported window.

Normal-path targets:

```text
userInterferenceRate      = 0%
physicalInputRate         < 1-5%
foregroundFocusSteals     = 0
cloudRoundTripsPerStep    ~ 0 inside bounded runs
screenshotsPerStep        ~ 0
```

Physical mouse/keyboard synthesis is a compatibility fallback, never the default execution model.

## 1. Interaction lanes

### Level 1 — BACKGROUND_NATIVE

Preferred lane.

- macOS: Accessibility/AX actions.
- Windows: UI Automation patterns.
- Linux: AT-SPI where available.
- No physical pointer movement.
- No global keyboard injection.
- No foreground activation unless an application API strictly requires it.

Expected result metadata:

```json
{
  "interactionMode": "background_native",
  "usesPhysicalInput": false,
  "requiresForeground": false,
  "userInterference": false
}
```

### Level 2 — ISOLATED_VISUAL

Use an isolated browser/app execution context when the target cannot be controlled safely inside the user's foreground session.

- Managed browser profile/context.
- Agent-owned tab/window.
- Visual reasoning allowed without hijacking the user's input devices.

### Level 3 — PHYSICAL_FOREGROUND

Last resort only.

- CGEvent / SendInput / global keyboard.
- May move the real pointer or require focus.
- Must remain explicitly classified as physical input.
- Must require the existing fresh approval policy where applicable.
- Must never be silently selected after a failed background action.

## 2. Target architecture

```text
AI / MCP client
      |
      | goal / bounded action program
      v
CodeLocal cloud relay
      |
      | one tool call where possible
      v
Local Device Agent Engine
      |
      +----------------------+----------------------+
      |                      |                      |
      v                      v                      v
Scene Engine           Action Engine          Visual Engine
window registry        semantic batch         local crop/diff
AX/UIA graph            targeted verify        OCR/Vision
semantic index          no-blind-retry         media references
AX event stream         idempotency            lazy image fetch
      |                      |                      |
      +----------------------+----------------------+
                             |
                             v
                    Device Scheduler
                     /      |       \
                    /       |        \
          background     isolated    physical
             native       context     fallback
```

## 3. P0 — Native semantic batching

Problem before v3 P0:

```text
computer_run (one MCP call)
  -> SemanticAction step 1 -> helper round trip -> AX traversal
  -> SemanticAction step 2 -> helper round trip -> AX traversal
  -> SemanticAction step 3 -> helper round trip -> AX traversal
```

The API looked batched, but local execution was still one helper call per step.

Target:

```text
computer_run
  -> semantic_batch (one helper call)
       -> persistent native worker
       -> resolve + execute step 1
       -> resolve + execute step 2
       -> resolve + execute step 3
       -> one structured result
```

Safety invariants:

- Same application window only.
- Semantic `click` and `type` only in this P0 slice.
- No coordinates.
- No physical fallback.
- Maximum bounded step count remains enforced by the controller.
- Existing approval classification remains authoritative.
- If a batch fails after any step may have executed, do not retry the batch and do not replay individual steps.
- Return the failed step number and completed step count.

## 4. P0 — Runtime health and honest capability reporting

A static capability such as `windowList: true` is insufficient if the live backend currently returns only the synthetic `screen:main` fallback.

`computer_status` must include live health such as:

```json
{
  "health": {
    "status": "degraded",
    "reason": "WINDOW_ENUMERATION_FALLBACK_ONLY",
    "applicationWindowCount": 0,
    "fallbackWindowCount": 1,
    "durationMs": 12
  }
}
```

This prevents the planner from assuming safe background semantic control when there is no usable application window identity.

## 5. P1 — Native macOS daemon

Replace JXA/System Events as the normal hot path with a Swift/Objective-C native helper using:

- `AXUIElement` for background interaction.
- `AXObserver` for event-driven invalidation.
- `ScreenCaptureKit` / supported window capture APIs for visual context.
- Vision for local OCR/fallback reasoning.

JXA remains compatibility fallback during migration, not the long-term execution core.

Expected benefits:

- lower action latency;
- fewer process/runtime translation layers;
- stronger typed accessibility handling;
- better event subscriptions;
- more reliable background actions;
- easier per-phase telemetry.

## 6. P1 — Event-driven Scene Graph

Replace short TTL correctness assumptions with event-driven invalidation.

Scene state should track:

```text
sceneGeneration
window registry
AX node identities
semantic index
dirty subtrees
focused element
last visual hash
```

AX/UIA notifications should invalidate only affected state where possible.

Examples:

- value changed -> dirty target element/value;
- child created -> dirty parent subtree;
- window created/destroyed -> refresh window registry;
- layout changed -> invalidate affected geometry;
- app terminated -> drop app scene.

## 7. P1 — Targeted verification

Do not verify every action by re-reading an entire UI tree.

Verification modes:

```text
none   -> trusted low-risk deterministic local action
 target -> re-read only affected element/state (default goal)
 scene  -> refresh the current window scene after navigation/dialog changes
```

Examples:

- type into field -> verify that field's AX value;
- checkbox press -> verify checked/value state;
- Save opening a dialog -> wait for window/child event then verify scene generation.

## 8. P1 — User-first Device Scheduler

User activity has the highest priority.

```text
User physical activity     priority 100
Background native agent    priority 50
Physical fallback agent    priority 10
```

Before any physical fallback:

```text
physical action requested
  -> user recently active?
       YES -> do not steal input; retry background/isolated lane or fail safely
       NO  -> existing approval policy still applies
```

Background-native actions do not need to wait for user inactivity because they do not consume the user's physical input devices.

## 9. P1 — Visual transport without base64 on the hot path

Default observation is semantic-first:

```text
window metadata + AX/UIA tree -> AI
```

When vision is required:

```text
capture local region
 -> crop changed region
 -> resize/compress appropriately
 -> content hash
 -> private MediaUpload object
 -> short-lived signed HTTPS URL / imageRef
```

MCP should carry lightweight metadata instead of multi-megabyte base64 whenever the client/model supports image references.

Example:

```json
{
  "sceneId": "scene_219",
  "visual": {
    "imageRef": "img_abc",
    "url": "https://media.example/signed/...",
    "width": 1280,
    "height": 720,
    "expiresAt": 0
  }
}
```

Base64 remains a compatibility fallback for clients that cannot dereference an image URL.

Media requirements:

- private by default;
- signed URL TTL around 1-5 minutes;
- content-hash deduplication;
- automatic expiry/deletion;
- no public directory listing;
- no upload when the visual hash has not changed;
- prefer region/crop over full-screen capture.

## 10. P2 — Windows persistent UIA worker

The current Windows implementation still launches PowerShell for many operations. Replace the hot path with a persistent native UIA/Win32 worker.

Target capabilities:

- persistent window registry;
- UIA element cache;
- Invoke/Value/Selection/Scroll patterns;
- event subscriptions;
- semantic batch;
- background-native result metadata;
- physical SendInput only as explicit fallback.

## 11. P2 — Unified semantic resolver

Semantic matching must have one canonical implementation per engine generation.

Avoid controller/native resolver drift where the same target can resolve to different elements.

Native resolver result should include:

```json
{
  "elementId": "...",
  "role": "button",
  "name": "Save",
  "score": 110,
  "runnerUpScore": 40,
  "confidence": "high"
}
```

Ambiguous near-ties remain a hard failure rather than traversal-order clicking.

## 12. Telemetry

Track locally and return compact timing metadata where useful:

```text
windowResolveMs
sceneReadMs
semanticResolveMs
actionMs
verifyMs
visualCaptureMs
mediaUploadMs
totalMs
helperCalls
physicalInputUsed
foregroundRequired
```

Do not include typed secrets in logs or telemetry.

Primary operational KPI is not only latency. It is:

```text
User Interference Rate
```

Any normal workflow that steals focus, moves the physical cursor, or types into the user's active app is a regression.

## 13. Delivery phases

### Phase A — P0 now

- [x] Existing persistent outer computer helper.
- [x] Existing persistent macOS JXA worker.
- [x] Existing semantic background click/type path.
- [x] True `semantic_batch` request to persistent macOS worker.
- [x] `computer_run` uses one native helper call when native batching is advertised.
- [x] No replay after a partially executed native batch failure.
- [x] Live window-enumeration health in `computer_status`.
- [x] Foreground `computer_focus` requires fresh critical approval and cannot be silently auto-approved in Agent Mode.
- [x] Focused tests + verification.

### Phase B — P1 macOS quality

- [x] Native Swift AX daemon source with persistent JSON-line protocol, real AX window enumeration/tree traversal, targeted element read, semantic click/type and bounded semantic batch.
- [x] Go native-daemon bridge prefers the Swift worker only after a successful handshake and keeps JXA as compatibility fallback without changing the public MCP schema.
- [x] Scene generation + native event invalidation boundary (`ComputerSceneEvent`) so AXObserver/UIA can plug in without changing the public MCP schema.
- [x] Real macOS `AXObserver` event source with a bounded local event queue; the Go controller drains events before trusting cached window/UI state.
- [x] Window-specific AX events dirty only the affected scene where a current AX window id can be resolved; app/window lifecycle events can invalidate the global registry.
- [x] Native live smoke test enumerates real application windows (Chrome/Finder/Terminal/Zalo/Telegram/System Settings/ChatGPT) instead of synthetic `screen:main`, and returns a native Chrome AX tree.
- [x] Targeted verification for semantic click/type and `computer_run` using exact `element_read`; full scene verification remains available with `verifyMode=scene`.
- [x] Target verification avoids echoing typed field contents; it returns match/length metadata and fails verification safely when the value cannot be confirmed.
- [x] Device user-activity guard for foreground focus and physical-input fallback on macOS; background semantic AX actions are not delayed by user activity.
- [x] Lifecycle-managed macOS menu-bar Computer Use indicator owned by the native worker: idle (`○`), background control (`●`), viewing/capture (`◉`), foreground/physical (`⚠︎`), paused (`Ⅱ`), and stopped (`■`).
- [x] User-visible Pause/Resume/Stop controls are hard gates rather than decoration: Go blocks every fallback lane and the native worker rechecks control state before AX mutations, capture, and each semantic-batch step.
- [x] `paused` blocks mutating input while observation remains available; `stopped` blocks both observation and input while status/user-activity probes remain available for recovery and explanation.
- [x] `computer_status` exposes `activityIndicator`, `userControlGate`, and current `controlState`; ScreenCaptureKit/Vision transitions the CodeLocal indicator to `Viewing Screen` while macOS retains its own system capture/privacy indicator.
- [x] Desktop backend migration contract exposes whether native AX, event-driven scene, targeted verification, and user-activity guard are actually active.
- [x] Package the Swift native worker in npm releases from architecture-matched macOS runners (`arm64` + `amd64`), then inject both artifacts into `.release/npm/bin/helpers` while retaining the Go/JXA compatibility path.
- [ ] Add Apple Developer ID signing/notarization hardening for the packaged native worker; packaging does not claim signed/notarized status yet.
- [x] Exact AX element click/type from a UI-tree `elementId` uses native `AXPress` / `AXValue` in background; exact typing is no longer misclassified as global physical keyboard input.
- [ ] Remove remaining `osascript` compatibility paths after native packaging reaches production; foreground focus/raw physical keyboard-pointer fallbacks intentionally remain outside the normal background lane.
- [x] ScreenCaptureKit in-memory app-window capture on the native path; captured windows are bounded to 1440px wide and the disk-based `screencapture` path remains compatibility fallback only.
- [x] In-memory native Vision OCR for AX application windows (`ScreenCaptureKit -> CGImage -> Vision -> semantic nodes`); the temp-PNG/JXA Vision path remains a compatibility fallback for `screen:main`, older macOS, or windows ScreenCaptureKit cannot capture.
- [x] Private visual transport is now owned by CodeLocal Go Cloud: the runtime sends only signed device-auth metadata (`sha256`, MIME, size) to `/api/client/media/presign`, then uploads bytes directly to private S3-compatible storage with a presigned PUT. No separate MediaUpload service/static media token is required, and `__mcpImage` base64 is removed before the WebSocket payload when the Go media backend is configured.
- [x] MCP gateway converts signed image metadata into `mcp.ResourceLink`; base64 `mcp.ImageContent` remains only when URL transport is disabled or `CODELOCAL_MEDIA_BASE64_FALLBACK=1` is explicitly enabled.
- [x] Visual content SHA-256, client-side signed-URL cache and server-side object dedupe by content hash; repeated identical captures do not upload a new object.
- [x] Window crop/resize: native ScreenCaptureKit captures the requested app window instead of the full desktop and scales large windows down for AI transport.
- [x] Adaptive screenshot encoding on the native path: keep PNG for small/text-sensitive captures, and use WebP quality 0.92 only when a >=64 KB frame shrinks by at least 20%; screenshot semantics remain a complete standalone window image.
- [ ] Changed-region/pixel diff requires an explicit receiver contract (`baseFrameId`/`sinceFrameId`) before shipping. Do not send a dirty crop as if it were a complete `computer_screenshot`; that would break standalone screenshot semantics.

### Phase C — cross-platform parity

- [ ] Persistent Windows UIA worker.
- [ ] Linux AT-SPI event-driven path where supported.
- [ ] Unified result/health/telemetry contract.
- [ ] Multi-agent concurrency tests.

## 14. Acceptance tests

### Background interference test

While a human continuously types in app A, agent operates supported app B:

- human keystrokes remain in app A;
- physical pointer position is unchanged by background actions;
- app A does not lose focus;
- agent action in app B succeeds.

### Batch test

For N same-window semantic steps:

- one MCP `computer_run` call;
- one outer helper `semantic_batch` call on supported native batch backend;
- one persistent worker request;
- completed count is exact;
- failure reports exact failed step;
- no replay after failure.

### Degraded health test

If window enumeration returns only `screen:main` fallback:

```text
health.status = degraded
health.reason = WINDOW_ENUMERATION_FALLBACK_ONLY
applicationWindowCount = 0
```

### Safety regression test

A semantic batch script must not contain physical input synthesis APIs such as CGEvent mouse posting.

## 15. Non-goals for the P0 slice

P0 does not claim full Codex-equivalent desktop isolation yet.

It does not:

- create a second OS pointer;
- guarantee zero-interference for Canvas/Metal/game/custom-rendered apps;
- virtualize arbitrary native apps;
- replace all JXA paths immediately;
- add Windows native batching in the same patch;
- make physical fallback automatic.

The P0 purpose is to make the existing background architecture honest, measurably faster, and safe enough to support the deeper native v3 migration.
