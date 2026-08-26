# CodeLocal implementation status

## Implemented in `1.0.0-preview.2`

### Gateway / ChatGPT side
- Streamable HTTP MCP endpoint
- OAuth for ChatGPT -> gateway
- authenticated WebSocket client registration
- multi-device / multi-workspace registry
- `list_devices`, `list_workspaces`, `select_workspace`, `workspace_info`
- request IDs, timeouts and structured logs
- heartbeat/pong device liveness
- automatic routing when only one workspace is online

### Local workspace safety
- absolute path rejection
- `..` traversal rejection
- symlink escape checks
- sensitive-path policy independent of `.gitignore`
- `.gitignore`-aware traversal/search
- ignored dependencies/build output can still be targeted deliberately
- binary detection
- file hashes + stale-write conflict checks

### Retrieval / code intelligence
- project and repo instruction discovery
- file metadata
- file/range/batch reads
- code search
- targeted Node dependency inspect/search/read
- TypeScript/JavaScript semantic project index
- symbol discovery
- definition/reference lookup
- callers/callees
- import graph
- TypeScript diagnostics
- filesystem watcher invalidation

### Editing / verification
- write/edit with optional expected hash
- unified patch validation/application
- Git status/diff/log/show/blame/file history
- project test/check command detection
- related-test search
- affected-test execution

### Runtime
- guarded shell
- local approval prompt for review/high-risk actions
- incremental process logs with cursors
- stdin / process list / kill
- realtime stdout/stderr mirror in local terminal
- reconnect with exponential backoff
- PTY support on Unix (`internal/process/pty_unix.go` via `creack/pty`, `pty_start/poll/write/resize/signal` in `server-v2.ts`) — Windows (`pty_windows.go`) still reports `not available`
- idempotency journal (`internal/idempotency/journal.go`, persisted `journals/*.json`, `Started/Completed/Failed/Abandon`, approval tokens not persisted) with replay safeguards for write operations
- durable device identity primitives (`internal/deviceauth/signing.go` ed25519 sign/verify, 90s skew; `internal/cloud/device_nonce.go` Redis Lua `ZADD` nonce TTL 3m; `internal/cloud/store_runtime.go` usage/outbox/retention/knowledgeHealth workers)

### Security observability
- structured server/client logs
- token/secret/content redaction in structured logs
- local security audit console for shell commands
- risk levels: SAFE / REVIEW / HIGH / CRITICAL
- explicit matched policy rules
- explicit blocked / reviewed / completed messages
- command-value redaction for common token/password/API-key forms

## Deliberately not marked complete yet

These require platform-specific work and should not be represented as finished:

### Native OS sandbox
The current guarded shell and workspace filesystem checks are strong application guardrails, but arbitrary shell execution is not yet contained by a true OS-level sandbox on macOS/Linux/Windows.

### True PTY terminal
Unix PTY is implemented (`internal/process/pty_unix.go` + `server-v2.ts` `pty_*` tools) with resize/signal; Windows PTY (`pty_windows.go`) is still stubbed as `not available` and full terminal emulation coverage remains incomplete.

### Durable production identity/state
Partially implemented: `internal/deviceauth/signing.go` (ed25519 device signing/verification, 90s clock skew), `internal/cloud/device_nonce.go` (Redis Lua nonce with 3m TTL via `ZADD`), and `internal/cloud/store_runtime.go` workers (usage/outbox/retention/knowledgeHealth) provide durable identity primitives. Remaining work: full per-device pairing/revocation UX migration away from shared `DEVICE_TOKEN` bootstrap and durable persistence validation for all connection state.

### Cross-language semantic parity
TypeScript/JavaScript has the first semantic index. Python/Rust/Go currently rely on filesystem/search/toolchain commands rather than persistent Pyright/rust-analyzer/gopls sessions.

### Full request cancellation/idempotency
Idempotency journal is implemented (`internal/idempotency/journal.go` persisted as `journals/*.json`, with `Started/Completed/Failed/Abandon`; approval tokens are intentionally not persisted) providing replay protection for write operations. Timeouts and process kill exist. Remaining work: full MCP cancellation propagation end-to-end and formal validation of replay handling under reconnect/retry.

## Release rule

Do not call the project `1.0 stable` until native sandbox strategy, device credential lifecycle, and end-to-end cancellation/idempotency have been validated. Until then use `1.0.0-preview.x`.
