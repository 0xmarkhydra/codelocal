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
Current child-process pipes support most builds/tests/dev servers and stdin, but not full terminal emulation/resize semantics required by every interactive CLI.

### Durable production identity/state
The gateway supports multiple live device/workspace registrations, but still uses the shared `DEVICE_TOKEN` bootstrap model and in-memory connection state. Per-device pairing/revocation and durable storage remain production-hardening work.

### Cross-language semantic parity
TypeScript/JavaScript has the first semantic index. Python/Rust/Go currently rely on filesystem/search/toolchain commands rather than persistent Pyright/rust-analyzer/gopls sessions.

### Full request cancellation/idempotency
Timeouts and process kill exist, but MCP cancellation propagation and write-operation replay protection are not yet complete end-to-end.

## Release rule

Do not call the project `1.0 stable` until native sandbox strategy, device credential lifecycle, and end-to-end cancellation/idempotency have been validated. Until then use `1.0.0-preview.x`.
