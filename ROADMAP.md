# CodeLocal / codex-mcp Roadmap

> Project Brain architecture, durable knowledge, rules/skills discovery, cross-device learning, Context Compiler, and the measurable strategy for competing beyond Codex/Claude are defined in [`PROJECT_BRAIN_MASTER_PLAN.md`](./PROJECT_BRAIN_MASTER_PLAN.md). Treat that document as the source of truth for the intelligence layer.

## Goal

Build a remote MCP coding bridge that gives ChatGPT/Codex a local execution and code-intelligence layer comparable to the practical capabilities of Codex CLI, while keeping the model reasoning in ChatGPT and keeping filesystem/shell execution on the user's own machine.

Architecture:

```text
ChatGPT / Codex
    |  MCP over HTTPS + OAuth
    v
Railway MCP Gateway
    |  authenticated WebSocket
    v
CodeLocal Client
    |  sandboxed workspace tools
    v
PROJECT_ROOT
```

The MCP server is not the "brain". ChatGPT/Codex provides reasoning. CodeLocal provides high-quality local tools, repository intelligence, execution, safety, observability and state.

---

## Current state (V0.4.x)

### Server

- Streamable HTTP MCP endpoint at `/mcp`
- OAuth authentication for ChatGPT -> Railway
- `DEVICE_TOKEN` authentication for local client -> Railway
- WebSocket gateway at `/client`
- MCP session management
- tool forwarding with request IDs and timeouts
- structured server logging
- health endpoint

### Local client

- `PROJECT_ROOT` filesystem sandbox
- block absolute paths, `..` traversal and symlink escape
- `project_info`
- `read_instructions`
- `list_files`
- `read_file`
- `read_files`
- `search_code`
- `write_file`
- `edit_file`
- `apply_patch`
- `git_status`
- `git_diff`
- `run_command`
- `process_poll`
- `process_list`
- `process_write`
- `process_kill`
- shell command policy / guarded mode
- optional dangerous-shell override
- incremental process output
- realtime command stdout/stderr mirror to client terminal
- structured client logging

---

# Design principles

## 1. ChatGPT is the reasoning layer

Do not duplicate an AI agent inside CodeLocal. The client should expose accurate, composable tools and context. The model decides which tools to call and in what order.

## 2. Retrieval is selective

Never load an entire repository into context by default. Prefer:

```text
project map
-> instructions
-> symbols/references
-> targeted search
-> targeted file/range reads
-> edit
-> diagnostics/tests
```

## 3. `.gitignore` is retrieval policy, not security policy

Three categories:

### Normal
Source/config files normally indexed and searchable.

### Ignored
Dependency/build/generated/cache files such as `node_modules`, `dist`, `coverage`, `.next`, `target`.

- excluded from normal scans/indexing
- may be read/search targeted when the model explicitly needs them

### Sensitive
Secrets and credentials are separately protected regardless of `.gitignore`.

Examples:

- `.env*`
- private keys
- `.ssh`
- `.aws`
- `.gnupg`
- credential files

Security policy, not `.gitignore`, decides whether sensitive data can be accessed.

## 4. Workspace safety is enforced on the client

The Railway gateway is never trusted to enforce local filesystem boundaries. Every local operation is validated by the local client.

## 5. Observability is first class

Every tool call should be traceable end-to-end without leaking source contents or secrets.

```text
ChatGPT
-> Railway requestId
-> local client requestId
-> tool/process
-> result/error/duration
```

---

# V0.5 — Smart retrieval and workspace awareness

Priority: highest immediate upgrade.

## `.gitignore`-aware traversal

Replace hard-coded skip lists with repository-aware ignore handling.

Tools should expose optional behavior such as:

```text
list_files(path, includeIgnored=false)
search_code(query, includeIgnored=false)
read_file(path)
```

`read_file` may access ignored files when targeted and permitted by security policy.

## Targeted dependency inspection

Do not enumerate full dependency trees by default.

Add capabilities such as:

```text
inspect_dependency(name)
search_dependency(name, query)
read_dependency(name, path)
```

Support progressively:

- Node: `node_modules`, package exports/types
- Rust: Cargo metadata/source cache
- Python: venv/site-packages
- Go: module cache

Use package manifests and lockfiles to determine exact installed versions first.

## File-range reading

Add:

```text
read_file_range(path, startLine, endLine)
```

Avoid returning a 5,000-line file when only 80 lines are needed.

## File metadata and version hashes

Every read should optionally return:

```text
path
size
mtime
hash/version
```

Edits can accept an expected hash to prevent overwriting files that changed after the model read them.

## Better project map

`project_info` should detect:

- languages
- frameworks
- package manager
- scripts
- monorepo/workspaces
- lockfiles
- Git branch/root
- build/test/lint commands
- instructions files
- likely source/test directories

## Binary awareness

Do not decode arbitrary binary files as UTF-8.

Return file metadata/type for:

- images
- archives
- SQLite databases
- PDFs
- generated binaries

---

# V0.6 — CodeGraph + LSP intelligence

This is the largest improvement to code understanding.

## Symbol tools

Add:

```text
find_symbol
find_definition
find_references
get_callers
get_callees
get_import_graph
get_dependencies
get_dependents
get_symbol_context
```

## LSP integration

Use installed/native language servers where available.

Initial targets:

- TypeScript/JavaScript: TypeScript language service / typescript-language-server
- Python: Pyright
- Rust: rust-analyzer
- Go: gopls

Expose:

```text
lsp_definition
lsp_references
lsp_hover
lsp_diagnostics
lsp_workspace_symbols
lsp_document_symbols
```

## Hybrid CodeGraph

Use LSP as the source of semantic truth where possible and maintain a lightweight graph for fast traversal.

The graph should model:

```text
file -> imports -> file
symbol -> defined in -> file
symbol -> references -> locations
function -> callers/callees
class/interface -> implementations
```

## Incremental indexing

Do not rebuild the entire graph after every edit.

Add filesystem watching and invalidate/re-index only changed files.

---

# V0.7 — Diagnostics, tests and Git intelligence

## Diagnostics

Add a unified tool:

```text
get_diagnostics(path?)
```

Backed by LSP/compiler/toolchain.

## Test intelligence

Detect project test commands and test frameworks.

Add:

```text
detect_test_commands
find_related_tests
run_affected_tests
run_tests
```

Workflow after an edit:

```text
edit
-> diagnostics
-> affected tests
-> broader tests if needed
-> git diff
```

## Git history intelligence

Add read-only tools first:

```text
git_log
git_show
git_blame
git_file_history
git_branch_info
```

This lets the model understand why code exists, not only what the current code says.

Write Git actions should be separate and approval-gated:

```text
git_stage
git_commit
git_push
```

---

# V0.8 — Approval and strong sandboxing

Current regex shell policy is a guardrail, not a real operating-system sandbox.

## Action classification

Classify actions:

```text
AUTO_ALLOWED
- read/search
- diagnostics
- tests
- safe build commands

APPROVAL_REQUIRED
- package installation
- delete/rename large sets
- database migration
- Git commit/push
- network-affecting commands

BLOCKED
- credential access
- system administration
- disk/device modification
- workspace escape
```

## Approval protocol

Gateway/client protocol must support:

```text
approval_required
approval_granted
approval_denied
```

The local user should be able to see exactly:

- requested operation
- command/path
- reason
- risk level

## Native sandbox

Investigate platform-specific enforcement rather than only command matching.

Targets:

- macOS sandbox/process restrictions
- Linux namespaces / bubblewrap where practical
- Windows job/process/filesystem restrictions where practical

Network policy should eventually be independently controllable from filesystem policy.

---

# V0.9 — Terminal and process parity

## PTY support

Current stdio pipes cover many commands but not all interactive tools.

Add PTY support for:

- interactive CLIs
- dev servers
- REPLs
- package manager prompts
- migration tools

Tools:

```text
process_start
process_poll
process_write
process_resize
process_signal
process_kill
```

## Process lifecycle

- reconnect-safe process records where feasible
- explicit process ownership/session
- output cursors
- bounded logs
- idle cleanup
- timeout policies

---

# V1.0 — Multi-project, multi-device production architecture

After single-user coding quality is stable.

## Device model

```text
user
  -> device
      -> workspace/project
```

Example:

```text
Mong
  -> MacBook Pro
      -> BIDDI
      -> codex-mcp
  -> Mac mini
      -> Agent_X
```

## Pairing

Replace one shared `DEVICE_TOKEN` with per-device credentials.

Flow:

```text
user creates pairing code
-> local client exchanges it for device credential
-> server stores device identity
-> token can be revoked/rotated independently
```

## Workspace selection

Expose MCP tools/resources for:

```text
list_devices
list_workspaces
select_workspace
workspace_info
```

The model should never silently switch workspaces.

## Server state

Production gateway needs persistence for:

- users
- OAuth clients/sessions
- devices
- workspace registrations
- revoked credentials
- audit events

Do not persist source code contents unless explicitly designed and consented to.

---

# Logging and observability plan

## Client logs

Structured events:

```text
client.started
client.connecting
client.registered
client.disconnected

tool.received
tool.completed
tool.failed

process.started
process.stdout
process.stderr
process.exited
process.timeout
process.blocked
```

Realtime process output should be visible locally by default.

## Server logs

```text
server.started
http.request
oauth.*
mcp.initialize
mcp.session_open
mcp.session_close
client.socket_open
client.authenticated
client.socket_close
tool.dispatch
tool.complete
tool.failed
tool.timeout
```

## Redaction

Never log:

- access tokens
- device tokens
- OAuth secrets/passwords
- source file contents
- patch contents
- environment variable values
- stdin payloads that may contain secrets

Log path/tool metadata only when safe.

---

# Reliability plan

## Request cancellation

Add MCP/tool cancellation propagation:

```text
ChatGPT cancel
-> Railway cancels request
-> client stops operation/process where possible
```

## Disconnect handling

- fail pending tool calls immediately when client disconnects
- client automatically reconnects with backoff
- server rejects stale results

## Heartbeats

Add ping/pong and device health state.

## Idempotency

For write operations, include request identifiers and safeguards against accidental duplicate execution after reconnect/retry.

---

# Performance plan

## Avoid excessive context

Prefer:

- symbol-level responses
- line ranges
- capped search results
- incremental process logs
- summaries/metadata before full source reads

## Cache

Local cache candidates:

- `.gitignore` matcher
- project metadata
- package metadata
- symbol index
- LSP sessions
- dependency map

Invalidate via filesystem watcher.

## Compression

If tool payload sizes become a bottleneck, support compression at the WebSocket/protocol layer rather than changing model-facing semantics.

---

# Suggested final MCP tool surface

Keep the model-facing API coherent rather than exposing hundreds of tiny tools.

## Workspace

```text
project_info
read_instructions
list_files
file_info
read_file
read_file_range
read_files
search_code
```

## Semantic code intelligence

```text
find_symbol
find_definition
find_references
get_callers
get_callees
get_import_graph
get_diagnostics
```

## Dependencies

```text
inspect_dependency
search_dependency
read_dependency
```

## Editing

```text
write_file
edit_file
apply_patch
```

## Git

```text
git_status
git_diff
git_log
git_show
git_blame
```

## Runtime

```text
run_command
process_poll
process_list
process_write
process_kill
```

Additional write/destructive tools should be approval-gated rather than mixed with read-only tools.

---

# Definition of "close to Codex CLI"

CodeLocal is considered practically close to Codex CLI for repository work when ChatGPT can reliably perform this loop without manual copy/paste:

```text
understand task
-> inspect project/instructions
-> locate relevant symbols/references
-> read only necessary code
-> inspect dependencies when needed
-> run diagnostics/build/tests
-> edit/patch
-> re-run diagnostics/tests
-> inspect git diff
-> explain result
```

while preserving:

- local execution
- workspace boundaries
- approval for risky actions
- useful live logs
- low context waste
- no secret leakage

---

# Immediate implementation order

1. Finish `.gitignore`-aware retrieval and sensitive-path policy.
2. Add `read_file_range`, file hashes and conflict-safe edits.
3. Add dependency-aware targeted reads.
4. Build TypeScript/JavaScript LSP integration first.
5. Add semantic symbol/reference tools and lightweight CodeGraph.
6. Add diagnostics + affected-test workflow.
7. Add Git history tools.
8. Add approval protocol and stronger sandboxing.
9. Add PTY.
10. Move to multi-device/multi-workspace architecture.

Do not prioritize multi-user UX before the single-workspace coding loop is reliable and fast.
