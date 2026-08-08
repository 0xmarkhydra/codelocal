# CodeLocal Production Readiness Plan

## Goal

Ship CodeLocal as a real-world coding bridge that is safe, reliable, language-agnostic, observable, and pleasant enough to use daily as a remote counterpart to Codex CLI.

The target is not to duplicate the model/agent inside CodeLocal. ChatGPT/Codex remains the reasoning layer. CodeLocal must provide production-grade execution, semantic code intelligence, workspace context, editing, safety, identity, and reliability.

---

# Release philosophy

Do not call the project `1.0 stable` because it has many tools. Stable means the full coding loop is reliable under failure, reconnect, concurrent edits, multiple languages, and risky commands.

A stable release must satisfy these properties:

1. A model can understand a repo without scanning the whole repository.
2. Semantic queries work across common languages through one MCP API.
3. Commands cannot escape the intended security boundary silently.
4. Risky operations require explicit local approval.
5. Every tool call can be traced end-to-end.
6. Cancellation and retry do not cause duplicate writes or commands.
7. Device credentials can be paired, rotated and revoked independently.
8. Client/server restart does not corrupt session/process/workspace state.
9. Installation and upgrades are straightforward for non-developers.
10. The supported OS/language matrix is continuously tested in CI.

---

# Phase P0 — Freeze the protocol surface

Before deeper changes, stabilize the MCP and client protocol so later internals can change without breaking ChatGPT integrations.

## Tasks

- Introduce protocol version negotiation.
- Add explicit client capability advertisement.
- Add server capability advertisement.
- Standardize error codes instead of relying only on free-text messages.
- Standardize request envelope:

```text
requestId
sessionId
workspaceKey
tool
args
idempotencyKey?
deadline?
```

- Standardize result envelope:

```text
requestId
ok
result?
errorCode?
errorMessage?
metadata?
```

- Add backward compatibility policy for preview releases.

## Exit gate

Old preview clients either keep working or receive an explicit `CLIENT_UPGRADE_REQUIRED` error.

---

# Phase P1 — Execution Core v2

This is the first major production blocker.

## Process model

Replace the current basic process registry with a formal process manager.

Each process record must contain:

```text
processId
workspaceKey
ownerSessionId
pid
command
cwd
startedAt
lastActivityAt
status
exitCode
signal
stdoutCursor
stderrCursor
outputBytes
timeoutAt
```

## Required operations

```text
exec_start
exec_poll
exec_write
exec_signal
exec_cancel
exec_kill
process_list
```

Keep `run_command` as a high-level compatibility wrapper.

## True PTY

Implement PTY support behind a platform adapter.

Targets:

- macOS/Linux: `node-pty` or a small native helper
- Windows: ConPTY through `node-pty`

Add:

```text
pty_start
pty_poll
pty_write
pty_resize
pty_signal
pty_kill
```

## Output handling

- Separate stdout/stderr for non-PTY processes.
- Cursor-based incremental reads.
- Bounded ring buffers.
- Spill very large logs to local temporary state if necessary.
- Never return unbounded process output to the model.

## Cancellation

MCP cancellation must propagate:

```text
ChatGPT cancel
-> gateway pending request cancel
-> client tool cancellation
-> child process signal
-> final cancelled result
```

## Exit gate

Interactive CLI, dev server, test runner and cancellation tests pass on macOS, Linux and Windows.

---

# Phase P2 — Security Engine + Native Sandbox

Application regex policy remains useful, but arbitrary shell execution needs stronger containment.

## Unified risk policy

Create a dedicated policy module instead of embedding regexes in `client.ts`.

Risk classes:

```text
SAFE
REVIEW
HIGH
CRITICAL
BLOCKED
```

Every policy decision returns:

```text
riskLevel
matchedRules
requiresApproval
blocked
redactedCommand
reason
```

## Approval engine

Approval must be reusable across command, Git, filesystem and network operations.

Approval request contains:

```text
approvalId
requestId
operation
riskLevel
workspace
summary
redactedDetail
matchedRules
expiresAt
```

Support policies:

```text
prompt
deny
auto-safe
session-allow(rule)
```

Do not support global auto-dangerous as a normal production mode.

## Native sandbox abstraction

Define:

```ts
interface SandboxBackend {
  available(): Promise<boolean>;
  prepare(workspace: string, policy: SandboxPolicy): Promise<SandboxContext>;
  spawn(...): Promise<ChildProcess>;
}
```

### macOS

Prototype and validate the safest viable process/filesystem restriction available on supported macOS releases. If `sandbox-exec` cannot be relied on long term, isolate execution through a helper/container strategy instead of claiming sandboxing that is not enforced.

### Linux

Prefer bubblewrap/namespaces where available:

- workspace mounted read/write
- system paths read-only as needed
- secret directories unavailable
- optional network namespace restriction

### Windows

Use restricted process/token strategy plus Job Objects/ConPTY and workspace enforcement where practical.

## Network policy

Network should be independently configurable:

```text
network: deny | approval | allow
```

A build/test command should not automatically receive unrestricted network merely because filesystem execution is allowed.

## Exit gate

A malicious test suite attempting to read SSH/AWS credentials, write outside workspace, modify disks or execute elevated commands fails in the sandbox test suite.

---

# Phase P3 — Durable Identity and Pairing

Remove shared `DEVICE_TOKEN` as the normal product identity model.

## Pairing flow

```text
codelocal pair
-> generate short-lived pairing code
-> user confirms on authenticated gateway
-> gateway issues per-device credential
-> credential stored in local OS keychain/secure storage
```

Each device has:

```text
deviceId
deviceName
credentialId
createdAt
lastSeenAt
revokedAt?
capabilities
```

## Credential lifecycle

Add:

```text
list_devices
rename_device
rotate_device_credential
revoke_device
```

Shared bootstrap token remains only for migration/development and is disabled by default in stable builds.

## Gateway persistence

Persist at least:

- OAuth state required for stable operation
- users
- device identities
- workspace registrations
- credential revocations
- audit metadata

Do not persist source files, patches or command output unless explicitly enabled.

## Exit gate

Revoking one device immediately prevents reconnect without affecting another device.

---

# Phase P4 — Language-Agnostic Semantic Router

Replace the TypeScript-specific model-facing assumption with a unified semantic layer.

## Unified provider interface

```ts
interface SemanticProvider {
  id: string;
  languages: string[];
  available(): Promise<boolean>;
  definition(location): Promise<Location[]>;
  references(location): Promise<Location[]>;
  implementations?(location): Promise<Location[]>;
  hover?(location): Promise<Hover | null>;
  workspaceSymbols(query): Promise<Symbol[]>;
  documentSymbols(path): Promise<Symbol[]>;
  diagnostics(path?): Promise<Diagnostic[]>;
}
```

## Provider tiers

### Tier 1

- TypeScript/JavaScript: TypeScript Language Service / tsserver-compatible provider
- Python: Pyright/BasedPyright
- Rust: rust-analyzer
- Go: gopls

### Tier 2

- C/C++: clangd
- Java: Eclipse JDT LS
- Kotlin: Kotlin LSP where viable
- C#: Roslyn/compatible LSP
- PHP: Intelephense-compatible provider when installed

### Tier 3

- Swift: SourceKit-LSP
- Dart: Dart Analysis Server
- Ruby: language server provider
- Lua: lua-language-server
- Elixir: ElixirLS
- Zig: zls
- Solidity: available Solidity language server

## Fallback chain

```text
LSP semantic provider
-> Tree-sitter structural provider
-> ripgrep/text provider
```

No language should become unusable just because its LSP is absent.

## Polyglot workspace routing

A monorepo may run several semantic providers simultaneously.

Route based on file/project root, not one language per workspace.

## Unified MCP tools

Expose language-independent tools:

```text
semantic_info
workspace_symbols
document_symbols
find_definition
find_references
find_implementations
get_hover
get_diagnostics
get_callers
get_callees
get_import_graph
```

## Exit gate

Equivalent definition/reference/diagnostic integration tests pass for TS, Python, Rust and Go fixtures.

---

# Phase P5 — Project Context Engine

The model should not repeatedly reconstruct basic repo context.

## Project map

Build and cache a compact project map:

```text
languages
frameworks
workspace roots
entrypoints
source roots
test roots
package manifests
lockfiles
build commands
test commands
lint/typecheck commands
instruction files
dependency graph
module graph
```

## Smart retrieval

Use a retrieval planner internally:

```text
task hint
-> project map
-> semantic graph
-> relevant files/symbols/tests
-> targeted ranges
```

Do not return entire project maps when a compact relevant slice is enough.

## `.gitignore` behavior

Keep the existing principle:

```text
.gitignore = default retrieval policy
security policy = access control
```

Targeted reads of ignored dependencies/generated files remain possible unless security policy blocks them.

## Dependency intelligence

Extend beyond Node:

- Node/npm/pnpm/yarn/bun
- Python virtualenv/site-packages
- Cargo registry/workspace metadata
- Go module cache
- Maven/Gradle metadata where useful

## Exit gate

A representative monorepo task can locate relevant implementation and tests without broad `list_files`/repository-wide reads.

---

# Phase P6 — Editing and Verification Engine

## Structured edits

Add an atomic multi-edit tool:

```text
apply_edits([
  path,
  expectedHash,
  edits[]
])
```

Each edit uses offsets or line/column ranges with conflict detection.

## Atomicity

For multi-file changes:

1. validate every target/hash
2. prepare temporary files
3. apply all or none where feasible
4. emit changed-file metadata

## Formatting

Detect formatter by project/language:

- prettier/biome
- black/ruff format
- rustfmt
- gofmt
- clang-format
- language-specific equivalents

Expose `format_changed_files`.

## Verification loop

Add one high-level verification tool that orchestrates local primitives but does not add model reasoning:

```text
verify_changes
-> diagnostics
-> related tests
-> optional lint/typecheck
-> git diff summary
```

The model still decides whether to call it.

## Regression snapshots

Track before/after diagnostics:

```text
new
persisting
resolved
unknown
```

Inspired by scan-comparison style workflows.

## Exit gate

Concurrent file modification causes a clean conflict, never silent overwrite.

---

# Phase P7 — Git Production Workflow

Keep read-only Git actions automatic.

Approval-gated writes:

```text
git_stage
git_unstage
git_commit
git_push
```

Add safeguards:

- refuse commit if unexpected unrelated working-tree changes exist unless explicitly included
- show diff summary before approval
- never force-push by default
- protected-branch policy hook

## Exit gate

No Git write action occurs without an auditable approval decision under default production policy.

---

# Phase P8 — Reliability and Recovery

## Idempotency

Every side-effecting request gets an idempotency key.

Client keeps a bounded durable journal:

```text
idempotencyKey
operation
startedAt
completedAt
resultDigest
status
```

Duplicate retries return the previous result when safe instead of re-executing.

## Disconnect/reconnect

- fail or suspend pending calls deterministically
- reject stale results
- reconnect with jittered exponential backoff
- process ownership survives gateway reconnect while client stays alive

## Server restart

Workspace connections reconnect automatically.

Durable identity survives restart.

MCP sessions may expire cleanly, but must not leave ambiguous writes running unnoticed.

## Heartbeat

Record device health and last-seen state.

## Exit gate

Chaos tests randomly terminate gateway/client connections during read, write, patch and process operations without duplicate mutations.

---

# Phase P9 — Observability and Audit

## Structured tracing

Trace chain:

```text
mcpSessionId
requestId
workspaceKey
tool
processId?
approvalId?
duration
status
```

## Security audit console

Keep the local human-readable console and expand it to filesystem/Git/network approvals.

## Redaction

Redact:

- tokens
- passwords
- auth headers
- API keys
- cookies
- environment values
- source/patch contents from normal structured logs

Add redaction regression tests with representative secret formats.

## Optional local audit file

Allow a local JSONL audit log stored outside the project with secure permissions.

No cloud upload by default.

## Exit gate

Secret leakage test suite passes for server logs, client logs and approval/audit messages.

---

# Phase P10 — Packaging and UX

Developers should not need to clone the repository and manually manage environment variables for normal use.

## CLI

Target UX:

```bash
npm install -g codelocal
codelocal login
codelocal pair
codelocal doctor /path/to/project
codelocal start /path/to/project
```

or a standalone binary later.

## Config

User config outside repo:

```text
~/.codelocal/config.json
```

Sensitive credentials go to OS secure storage/keychain, not the JSON file.

Project-local optional config:

```text
.codelocal.json
```

for non-secret settings such as workspace name, allowed commands or test hints.

## Auto-update strategy

Start with explicit upgrade notifications. Do not silently self-update an executable with local shell privileges until signing and rollback are mature.

## Exit gate

Fresh-machine setup from install to first connected workspace takes fewer than five manual steps.

---

# Phase P11 — Test Matrix and Release Engineering

## OS matrix

- macOS Apple Silicon
- macOS Intel where feasible
- Ubuntu latest LTS
- Windows 11

## Runtime matrix

- supported Node LTS versions

## Language fixtures

At minimum:

- TypeScript/NestJS
- Next.js
- Python/FastAPI
- Rust
- Go
- Java or C++ as Tier-2 validation
- polyglot monorepo fixture

## Security fixtures

Include tests that attempt:

- path traversal
- symlink escape
- sensitive path reads
- command injection
- shell escape
- credential exfiltration
- network access under deny policy
- duplicate write replay

## CI gates

Required before stable release:

```text
typecheck
unit tests
integration tests
protocol compatibility
security policy tests
sandbox tests
OS matrix
language semantic fixtures
packaging smoke test
```

---

# Stable 1.0 release gates

`1.0.0` may be published only when all of the following are true:

- [ ] protocol versioning and explicit errors
- [ ] PTY execution on supported OSes
- [ ] end-to-end cancellation
- [ ] native/real sandbox strategy validated for supported OSes
- [ ] per-device pairing, rotation and revocation
- [ ] durable gateway identity state
- [ ] semantic router with TS/Python/Rust/Go Tier-1 support
- [ ] polyglot workspace support
- [ ] project context engine
- [ ] atomic/hash-safe structured edits
- [ ] verification/regression workflow
- [ ] idempotent side-effecting operations
- [ ] reconnect/chaos tests
- [ ] redaction/security regression suite
- [ ] install/pair/start CLI UX
- [ ] macOS/Linux/Windows CI smoke coverage
- [ ] production documentation and threat model

Until those are satisfied, keep versioning as `1.0.0-preview.x` or move to `1.0.0-beta.x` only after the security/identity blockers are resolved.

---

# Recommended implementation order

The order below maximizes real-world usability while minimizing unsafe complexity:

```text
P0 protocol freeze
-> P1 execution/PTY/cancel
-> P2 security/sandbox
-> P3 device pairing/state
-> P4 semantic router
-> P5 context engine
-> P6 editing/verification
-> P8 idempotency/recovery
-> P7 Git writes
-> P9 audit hardening
-> P10 packaging/CLI
-> P11 release matrix
```

Do not prioritize UI polish or cloud persistence of source data before sandbox, identity, cancellation and semantic routing are stable.

---

# Practical target: "Codex CLI-class coding loop"

The release is ready for serious daily use when this request works reliably:

```text
"Fix the login bug in this repository."
```

and the model can autonomously perform:

```text
select workspace
-> read scoped instructions
-> inspect project map
-> locate semantic symbols/references
-> read targeted code ranges
-> inspect dependency source only if needed
-> run diagnostics/tests
-> make conflict-safe edits
-> format changed files
-> re-run diagnostics/affected tests
-> inspect git diff
-> explain the result
```

while CodeLocal guarantees:

```text
workspace containment
explicit approval for risky actions
no secret logging
cancellable execution
no duplicate writes on retry
reliable reconnect
multi-language semantic support
traceable tool/process lifecycle
```
