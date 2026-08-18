# CodeLocal native macOS Computer worker

This directory contains the P1 native macOS Accessibility worker for Computer Engine v3.

The worker is intentionally a separate executable from the cross-platform Go `computerhelper`. The Go helper remains the stable protocol/safety boundary and prefers this native worker only after a successful health handshake. If the native binary is absent, CodeLocal continues to use the existing persistent JXA compatibility backend.

## Local build

```bash
swiftc cmd/computernative/main.swift -o computer-native-darwin-arm64
```

For local development, point the Go helper at the binary:

```bash
export CODELOCAL_COMPUTER_NATIVE_DAEMON=/absolute/path/to/computer-native-darwin-arm64
```

Packaged releases will eventually place the binary next to the existing Computer helper as:

```text
bin/helpers/computer-native-darwin-arm64
bin/helpers/computer-native-darwin-amd64
```

The release pipeline is intentionally not changed by this P1 slice. Until native binaries are built on macOS release runners, production packages continue to fall back to JXA.

## Protocol

Transport is newline-delimited JSON over stdin/stdout. One request produces one response.

Request:

```json
{"version":1,"op":"ping"}
```

Success:

```json
{"ok":true,"result":{}}
```

Failure:

```json
{"ok":false,"error":"message"}
```

Supported internal operations:

- `ping` — handshake and native capabilities.
- `windows` — enumerate real AX application windows with `ax:<pid>:<windowIndex>` identities.
- `tree` — bounded native AX tree traversal.
- `element_read` — re-read one exact accessibility element for targeted verification.
- `semantic` — background native AX click/value update.
- `semantic_batch` — bounded same-window semantic sequence with no replay after partial execution.
- `events` — drain AXObserver scene events accumulated since the previous drain.

The public MCP schema does **not** expose these operations directly. They are an implementation detail behind the existing `computer` tool.

## Event-driven cache invalidation

The Swift worker registers `AXObserver` notifications for application/window lifecycle and common window state changes. Events are queued locally and drained by the Go Computer controller before it trusts cached window/UI state.

This gives CodeLocal event-driven correctness while preserving the existing request/response protocol:

```text
AXObserver callback
  -> native event queue
  -> lightweight scene_events drain
  -> ComputerSceneEvent
  -> generation/dirty state
  -> cached tree reused only when still clean
```

Window-specific events carry a current `windowId` when it can be resolved. App-level events such as new/focused windows invalidate the global registry.

## Safety invariants

- Native semantic actions use `AXUIElementPerformAction` / `AXUIElementSetAttributeValue`; they do not synthesize the physical mouse or global keyboard.
- If a native semantic action/batch has been dispatched and fails, the Go bridge does not replay it through JXA.
- Physical fallback, foreground focus and approval policy remain in the existing Go controller.
- Secure text field values are not returned by the native worker.
- The native worker does not grant Accessibility or Screen Recording permission.
