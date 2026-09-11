# CodeLocal MCP Tool Surface Simplification Plan

Status: Implemented — compact public cutover complete; production measurement continues
Branch baseline: `dev`
Baseline date: 2026-08-12
Previous public MCP tool count: **77**
Current public MCP tool count: **19 domain tools**
Private native runtime commands: **77 mapped to stable operation IDs**

## 1. Why this document exists

Before this migration, CodeLocal exposed 77 first-party MCP tools directly to ChatGPT. The capability set was strong, but the public MCP surface had grown around implementation details: individual LSP operations, file read variants, Git commands, process lifecycle operations, PTY operations, approval operations, and compatibility aliases were each represented as separate tools.

The goal is **not** to remove capability. The goal is to keep the full runtime capability while presenting ChatGPT with a smaller, clearer, domain-oriented tool surface.

The intended result is:

- fewer tool schemas sent to the model;
- less ambiguity between similar tools;
- fewer accidental or redundant tool calls;
- simpler orchestration instructions;
- easier compatibility across ChatGPT model versions;
- easier evolution of CodeLocal internals without continually adding public MCP tools;
- no loss of file, semantic/LSP, Git, terminal, PTY, approval, workspace, or MCP Hub functionality.

## 2. Current problem

### 2.1 Tool count

At baseline the Go MCP gateway built 77 public schemas in `toolDefinitions()`.
That registry has been removed. `serverFor` now registers only the 19 domain definitions
from `compactToolDefinitions()`.

### 2.2 Too much implementation detail is public

Examples:

#### Semantic/LSP

Today there are separate public tools such as:

- `semantic_info`
- `workspace_symbols`
- `find_symbol`
- `document_symbols`
- `find_definition`
- `find_references`
- `find_implementations`
- `get_hover`
- `get_diagnostics`
- `get_callers`
- `get_callees`
- `get_import_graph`

These are related operations in one semantic/code-intelligence domain.

#### File reading

Today:

- `file_info`
- `read_file`
- `read_file_range`
- `read_files`

The model has to decide which read primitive to use before it even starts reasoning about the code.

#### Process lifecycle

Today:

- `exec_start`
- `pty_start`
- `exec_poll`
- `pty_poll`
- `process_poll`
- `exec_write`
- `pty_write`
- `process_write`
- `pty_resize`
- `exec_signal`
- `pty_signal`
- `exec_kill`
- `pty_kill`
- `process_kill`
- `exec_cancel`
- `process_list`

Several of these exist because CodeLocal supports multiple execution modes and compatibility paths. Those implementation distinctions do not all need to become separate model choices.

### 2.3 Compatibility aliases increase model ambiguity

Examples include:

- `find_symbol` vs `workspace_symbols`
- `process_poll` vs `exec_poll` / `pty_poll`
- `process_write` vs `exec_write` / `pty_write`
- `process_kill` vs `exec_kill` / `pty_kill`

Compatibility should exist at the protocol/runtime layer without necessarily being advertised as a first-class model tool.

## 3. Reference direction: OpenCode

OpenCode is useful as a design reference because it keeps a relatively small set of model-facing tools and places multiple related operations behind coherent tools. A notable example is LSP: multiple semantic operations can be represented through one model-facing LSP capability instead of one top-level tool per operation.

CodeLocal should follow the same **principle**, not copy OpenCode literally:

> Keep the model-facing surface small and semantic; keep implementation richness behind it.

CodeLocal has requirements OpenCode does not have in exactly the same form, including remote MCP routing, sleeping/active workspaces, ChatGPT-mediated approvals, local approval memory, MCP Hub extensions, and multi-device routing. Therefore the right target is not an arbitrary tool-count match; it is a compact surface that still reflects CodeLocal's domains.

## 4. Design goals

### Required

1. Preserve all useful existing capability.
2. Reduce the default public MCP surface from 77 to roughly 15-18 tools.
3. Keep local security policy authoritative.
4. Keep ChatGPT confirmation for reviewed/critical side effects.
5. Preserve workspace-scoped routing through `workspaceKey`.
6. Preserve MCP session isolation between ChatGPT threads.
7. Preserve compatibility for old clients during migration.
8. Do not expose every installed MCP extension tool directly to ChatGPT.
9. Do not add high-volume per-call database logging just to support this migration.
10. Keep tool schemas understandable enough that reducing tool count does not create one giant ambiguous dispatcher.

### Non-goals

- Removing LSP features.
- Removing PTY support.
- Removing granular internal RPC operations.
- Replacing local policy with model judgment.
- Converting everything into one `codelocal(action=...)` mega-tool.
- Removing MCP Hub dynamic discovery.

## 5. Recommended target public surface

The current implemented surface is **19 public domain tools**: the compact coding/platform domains plus Browser Automation, Computer Use, and the bounded `agent` orchestration tool. Historical 18-tool estimates below are retained only where they describe an earlier measurement checkpoint.

### 5.1 `device`

Purpose: account/device identity operations.

Actions/capabilities:

- list active devices
- list paired identities
- rename paired device
- revoke paired device

Absorbs:

- `list_devices`
- `list_device_identities`
- `rename_device`
- `revoke_device`

### 5.2 `workspace`

Purpose: workspace discovery, selection and current workspace information.

Actions/capabilities:

- list
- select
- info

Absorbs:

- `list_workspaces`
- `select_workspace`
- `workspace_info`

Important: selection remains MCP-session scoped.

### 5.3 `project`

Purpose: stable project metadata and instructions.

Actions/capabilities:

- info
- map
- instructions

Absorbs:

- `project_info`
- `project_map`
- `read_instructions`

### 5.4 `context`

Purpose: semantic-first task context retrieval.

Keep this as a dedicated top-level tool because it is a high-value orchestration primitive, not merely another search mode.

Absorbs/replaces:

- `context_for_task`

Recommended behavior remains: use this early for coding/debugging/review/refactor tasks.

### 5.5 `read`

Purpose: all normal project file reads.

Modes/capabilities:

- metadata only
- full targeted file
- line range
- multiple targeted files

Absorbs:

- `file_info`
- `read_file`
- `read_file_range`
- `read_files`

The server can infer the cheapest operation from arguments where safe. Example: `path + startLine/endLine` means range read; `paths[]` means batch read.

### 5.6 `search`

Purpose: filesystem/project textual discovery.

Capabilities:

- list project files
- exact/literal text search
- normal text search

Absorbs:

- `list_files`
- `search_code`

Do not merge semantic/LSP search into this tool; that belongs to `lsp` or `context`.

### 5.7 `dependency`

Purpose: inspect and read installed project dependencies.

Actions:

- inspect
- read
- search

Absorbs:

- `inspect_dependency`
- `read_dependency`
- `search_dependency`

### 5.8 `lsp`

Purpose: all code-intelligence and semantic navigation.

Actions:

- info
- workspace_symbols
- document_symbols
- definition
- references
- implementations
- hover
- diagnostics
- callers
- callees
- import_graph

Absorbs:

- `semantic_info`
- `workspace_symbols`
- `find_symbol`
- `document_symbols`
- `find_definition`
- `find_references`
- `find_implementations`
- `get_hover`
- `get_diagnostics`
- `get_callers`
- `get_callees`
- `get_import_graph`

`find_symbol` should become a compatibility alias internally, not a separate advertised operation.

### 5.9 `edit`

Purpose: all normal source/file mutation operations.

Actions/modes:

- write/create
- exact replace
- structured edits
- unified patch
- format changed files

Absorbs:

- `write_file`
- `edit_file`
- `apply_patch`
- `apply_edits`
- `format_changed_files`

The schema must keep stale-hash protection available.

### 5.10 `verify`

Purpose: before/after diagnostics and post-edit verification.

Actions:

- snapshot
- verify changes

Absorbs:

- `snapshot_diagnostics`
- `verify_changes`

Keep this separate from `lsp` because it encodes a change-verification workflow rather than generic semantic navigation.

### 5.11 `git`

Purpose: Git read and write operations.

Actions:

- status
- diff
- log
- show
- blame
- file_history
- stage
- unstage
- commit
- push

Absorbs:

- `git_status`
- `git_diff`
- `git_log`
- `git_show`
- `git_blame`
- `git_file_history`
- `git_stage`
- `git_unstage`
- `git_commit`
- `git_push`

Security/approval classification must be derived from the selected action, not merely from the parent `git` tool name.

### 5.12 `terminal`

Purpose: command safety checks, terminal history and starting execution.

Actions:

- preflight
- history
- run
- start
- start_pty

Absorbs:

- `terminal_preflight`
- `terminal_history`
- `run_command`
- `exec_start`
- `pty_start`

Important: the preflight/approval workflow remains mandatory where policy requires it.

### 5.13 `process`

Purpose: lifecycle control of processes already started by CodeLocal.

Actions:

- list
- poll
- write
- resize
- signal
- kill
- cancel

Absorbs:

- `exec_poll`
- `pty_poll`
- `process_poll`
- `exec_write`
- `pty_write`
- `process_write`
- `pty_resize`
- `exec_signal`
- `pty_signal`
- `exec_kill`
- `pty_kill`
- `process_kill`
- `exec_cancel`
- `process_list`

The process record already knows whether it is PTY/non-PTY, so callers should not need to choose a different top-level tool just to poll or stop it. Where behavior truly differs, use an explicit field/action validation rather than duplicate tool names.

### 5.14 `approvals`

Purpose: locally remembered approval management.

Actions:

- list
- revoke
- reset

Absorbs:

- `approval_list`
- `approval_revoke`
- `approval_reset`

### 5.15 `security`

Purpose: inspect the active host-policy execution model.

Actions:

- info
- smoke_test (optional/debug-only)

Absorbs:

- `sandbox_info`
- `sandbox_smoke_test`

Consider making `smoke_test` non-default/debug-only if it is not useful for normal model workflows.

### 5.16 `mcp`

Purpose: dynamic MCP extension discovery and invocation without advertising every extension tool globally.

Actions:

- list servers
- search tools
- tool info/schema
- call

Absorbs:

- `mcp_list`
- `mcp_search_tools`
- `mcp_tool_info`
- `mcp_call`

This architecture should remain dynamic. Do not flatten installed third-party MCP tools into CodeLocal's default top-level catalog.

## 6. Current-to-target mapping summary

| Domain | Current public tools | Target |
|---|---:|---:|
| Device | 4 | 1 |
| Workspace | 3 | 1 |
| Project/context | 4 | 2 |
| File read/search | 6 | 2 |
| Dependencies | 3 | 1 |
| Semantic/LSP | 12 | 1 |
| Editing/format | 5 | 1 |
| Verification | 2 | 1 |
| Git | 10 | 1 |
| Terminal start/preflight/history | 5 | 1 |
| Process/PTY lifecycle | 14 | 1 |
| Approval memory | 3 | 1 |
| Security | 2 | 1 |
| MCP Hub | 4 | 1 |
| Browser Automation | 11 | 1 |
| Computer Use | 10 | 1 |
| **Total** | **98** | **19 current** |

Note: the exact grouping is intentionally subject to implementation validation. The target is a semantic surface, not a requirement to hit exactly 16 at any cost.

## 7. Schema design principles

Reducing tool count can make things worse if each replacement becomes an unstructured mega-tool. Follow these rules.

### 7.1 One coherent domain per tool

Good:

- `lsp(action=definition|references|hover|...)`
- `git(action=diff|commit|push|...)`

Bad:

- `codelocal(action=read|git|shell|workspace|mcp|...)`

### 7.2 Use a required `action` discriminator where operations differ materially

The model should not need to infer mutually exclusive argument shapes from dozens of optional fields.

Example concept:

```json
{
  "action": "definition",
  "path": "internal/foo.go",
  "line": 42,
  "column": 8,
  "workspaceKey": "..."
}
```

### 7.3 Keep action names short and obvious

Prefer:

- `definition`
- `references`
- `commit`
- `poll`

Avoid names that leak implementation versions such as `exec_v2_poll`.

### 7.4 Validate per action server-side

A combined schema should not weaken validation. The handler must reject arguments that do not make sense for the selected action.

### 7.5 Compute annotations and approval policy from action

Today MCP annotations are based largely on the top-level tool name. With domain tools, mutability/destructiveness/open-world/idempotency may differ by action.

Example:

- `git(action=status)` is read-only.
- `git(action=commit)` mutates local state.
- `git(action=push)` is open-world and requires approval policy.
- `device(action=list)` is read-only.
- `device(action=revoke)` is destructive.

Therefore the implementation must introduce an action-aware operation descriptor used by:

- MCP annotations where representable;
- local security policy;
- ChatGPT approval flow;
- audit labels;
- usage accounting.

Because standard MCP annotations are attached at tool-definition level, a combined tool cannot perfectly advertise different static annotations per action. **Local runtime policy must remain the source of truth.** If inaccurate static annotations materially hurt clients, retain separate public tools for the small number of safety-distinct operations instead of over-merging them.

## 8. Important safety constraint: do not over-optimize tool count

The target is not "smallest number possible".

A merge should be rejected when it causes any of these:

- a giant schema that costs as many tokens as the original tools;
- safety metadata becomes misleading;
- the model frequently supplies mutually incompatible fields;
- one tool crosses unrelated domains;
- approval UX becomes less understandable;
- error messages no longer tell the user what operation is being approved.

If testing shows a safety-sensitive operation deserves its own public tool, keep it separate. A final surface of 18-22 good tools is better than 12 confusing tools.

## 9. Proposed internal architecture

### 9.1 Separate operation definitions from public tool definitions

Previously `toolDefinitions()` directly represented the public MCP surface.

Refactor toward two concepts:

1. **Internal operations** — granular capabilities, close to today's 77 handlers.
2. **Public tools** — compact domain schemas that route to internal operations.

Conceptually:

```text
ChatGPT
  -> public MCP tool (`lsp`)
      -> action router (`definition`)
          -> internal operation (`find_definition`)
              -> workspace gateway/runtime
```

This lets CodeLocal preserve mature granular handlers while changing the model-facing contract.

### 9.2 Introduce stable operation IDs

Keep stable internal IDs such as:

```text
workspace.select
file.read_range
lsp.definition
git.push
terminal.run
process.poll
mcp.call
```

Use operation IDs for:

- security classification;
- approval memory keys;
- audit labels;
- usage counters;
- compatibility translation;
- testing.

Do not key important policy solely from the new parent tool name.

### 9.3 Compatibility adapter

Legacy top-level tool:

```text
find_definition(args)
```

should translate internally to:

```text
invoke("lsp.definition", args)
```

New compact tool:

```text
lsp({ action: "definition", ...args })
```

should translate to the same internal operation.

This avoids maintaining two implementations.

## 10. Migration plan

### Phase 0 — Baseline and contract tests

Before changing the public registry:

- freeze the existing list of 77 tools in a test fixture;
- add a test that every legacy tool resolves to an internal operation ID;
- add coverage for current side-effect classification;
- add coverage for approval-required actions;
- add coverage for protocol v1/v2 capability gating;
- record current MCP tool-schema byte/token estimate in tests or a local benchmark;
- record representative workflow tool-call counts.

Representative workflows should include:

1. Find and fix a TypeScript/Go bug.
2. Read several related files.
3. Find definition + references + callers.
4. Make structured edits and verify.
5. Run test command and poll output.
6. Run an interactive PTY command.
7. Stage, commit and push.
8. Switch between multiple workspaces.
9. Discover and invoke an installed MCP extension.

Do not add high-volume database logging for this benchmark. Use tests/local benchmark output or existing aggregate usage infrastructure.

### Phase 1 — Internal operation router

Implement the internal operation layer without changing the public MCP catalog yet.

Deliverables:

- stable operation IDs;
- operation metadata;
- common dispatch function;
- existing 77 handlers route through it;
- security/approval behavior unchanged;
- all existing tests pass.

This phase should produce little or no user-visible change.

### Phase 2 — Add compact public tools behind a feature flag

Add the 16-domain compact surface in parallel with the existing surface.

Possible capability/feature flag names:

```text
compactToolSurface
mcpToolSurface=v2
```

Requirements:

- old clients/sessions can continue to receive legacy tools;
- compact sessions receive compact tools;
- both surfaces route to the same internal operation IDs;
- no duplicate business logic.

### Phase 3 — Model/tool-selection evaluation

Compare compact vs legacy surface on the representative workflows.

Measure:

- advertised tool count;
- serialized schema size;
- estimated tool-schema tokens;
- number of tool calls needed;
- wrong-tool retries;
- invalid-argument errors;
- redundant reads/searches;
- successful task completion;
- approval prompts and whether their wording remains specific;
- latency from fewer MCP/model round trips where applicable.

Success is not merely "16 < 77". The compact surface must actually improve orchestration quality.

### Phase 4 — Make compact surface the default

When evaluation passes:

- advertise compact tools by default for supported sessions;
- keep legacy translation available for compatibility;
- update orchestration instructions to name compact tools only;
- update README/architecture docs;
- update tool metadata tests;
- ensure ChatGPT starts new sessions on the compact surface.

### Phase 5 — Deprecate legacy advertisement

After a compatibility window:

- stop advertising legacy aliases to modern clients;
- retain internal translation if inexpensive;
- remove obsolete top-level schemas;
- remove compatibility handlers only when no supported client depends on them.

Product decision on 2026-08-12: the public compatibility window was waived.
Users will be told to update, while the granular names remain private runtime
commands so the compact gateway does not lose capability.

### Implementation checkpoint — 2026-08-12

Implemented in the current working tree:

- the 77 native runtime commands are frozen in tests;
- every runtime command maps to a stable internal operation ID and shared metadata;
- 16 compact domain tools cover every current internal operation;
- the gateway always advertises the 16 compact tools; legacy/dual advertisement and the feature flag were removed;
- stale `CODELOCAL_MCP_TOOL_SURFACE` deployment values cannot re-enable granular public tools;
- representative compact calls are checked against their native runtime equivalents;
- a compact tool is invoked through the real MCP HTTP transport in tests, not only through its resolver;
- successful and error results expose compact JSON text plus MCP `structuredContent`;
- runtime duration metadata is carried back through WebSocket and cross-replica routing so gateway logs can separate local execution from relay/activation overhead;
- active workspaces connected to the current gateway use a direct fast path that avoids rebuilding the durable catalog and avoids a redundant Redis owner lookup on every tool call;
- usage accounting is removed from the request path: calls enter a bounded local queue, are aggregated into a Redis Stream, then one leased consumer persists idempotent batches to PostgreSQL sequentially;
- `read_files` preflights the byte budget and reads with a bounded six-worker pool while preserving input order; mutations remain ordered;
- code search streams results and stops the child process as soon as the requested result limit is exceeded instead of buffering unbounded stdout;
- `context_for_task` refreshes its structural/import index once, reuses warm LSP providers without cold-starting one, and runs one bounded text search for the task instead of one repository scan/process per term;
- process write, resize, signal and kill aliases now share the same side-effect serialization metadata as their canonical operations;
- after adding the two compact automation domains, the advertised schema estimate is 18,776 bytes versus the conservative 39,798-byte pre-automation baseline, a 52.8% reduction.

Still required after the compact cutover:

- run representative workflows with real ChatGPT sessions;
- measure task completion, invalid calls, retries, total model/tool turns and end-to-end latency;
- verify approval wording and confirmation behavior for mixed read/write domain tools;
- retire or explicitly isolate the legacy TypeScript implementation;
- decide whether `handoff` should be added as a seventeenth domain tool.

### Why Codex CLI feels faster, and what CodeLocal should optimize

The current CodeLocal runtime is not globally single-threaded. The native client
starts each incoming tool call in its own goroutine, the gateway tracks concurrent
requests by request ID, and cross-replica requests are also handled concurrently.
Only side-effecting local operations are serialized intentionally to protect
idempotency and mutation order.

The more important architectural difference is the path length:

```text
Codex CLI
  model/agent loop -> local filesystem, patch and shell

ChatGPT + CodeLocal
  ChatGPT model loop -> HTTPS MCP -> CodeLocal Cloud
  -> optional Redis cross-replica hop -> WebSocket -> local runtime
  -> WebSocket/Redis/HTTPS response -> next model decision
```

Codex CLI therefore keeps the coding loop next to the repository and installed
tools. It can also delegate independent investigations to subagents, but
parallelism is not the only or necessarily the largest advantage. ChatGPT may
issue independent MCP calls concurrently, and CodeLocal can execute them
concurrently, but dependent steps still require the model to inspect one result
before choosing the next call.

Official OpenAI guidance supports two relevant design choices:

- keep prompts and advertised tool sets lean, and expose only task-relevant tools;
- use parallel/programmatic calls only for bounded independent work, while keeping semantic judgment, approvals and final validation as direct calls.

References:

- https://developers.openai.com/api/docs/guides/latest-model
- https://learn.chatgpt.com/docs/codex/cli
- https://developers.openai.com/plugins/build/mcp-server

#### Phase 3 latency measurements

Measure warm and cold calls separately, at P50/P95 at minimum:

- `gateway duration`: entry into the common operation dispatcher through routed result;
- `runtime duration`: time spent inside the native local engine;
- `relay/activation duration`: gateway duration minus runtime duration;
- same-gateway-owner versus Redis cross-replica calls;
- active versus sleeping workspace calls;
- request/response bytes and estimated tokens;
- model/tool turn count for the whole workflow, which the MCP server cannot infer from one call alone.

The runtime now returns `runtimeDurationMs` as hidden protocol metadata and the
gateway logs `durationMs`, `runtimeDurationMs` and `relayDurationMs`. Do not draw
conclusions from average runtime duration alone; a fast local operation can still
feel slow when model and relay round trips dominate.

#### Optimization order

1. **Reduce model round trips.** Make `context`, `read(action=many)` and `verify`
   return bounded, decision-ready packets so the model does not need several
   predictable follow-up reads.
2. **Remove repeated routing work.** Same-gateway active calls now use the live
   WebSocket client as their workspace/capability source and bypass redundant
   catalog/owner lookups. Measure the remaining cross-replica path; if it is
   material, add a targeted active-workspace lookup or connection affinity rather
   than rebuilding the full catalog.
3. **Keep results small and structured.** Prefer MCP `structuredContent`, compact
   JSON, stable IDs, explicit limits and concise error shapes. Add output schemas
   only where they improve selection enough to justify their advertised token cost.
4. **Preserve read concurrency.** Independent tool calls can run concurrently and
   `read(action=many)` now uses bounded internal workers. Keep limits explicit and
   benchmark on real repositories before raising the default concurrency above six.
5. **Use server-side fan-out inside coherent tools.** It is safer and cheaper to
   let `context` gather/rank independent semantic evidence internally than to add
   a generic model-facing `batch` tool.
6. **Keep mutations ordered.** Do not parallelize edits, Git writes, terminal
   approvals or other side effects merely to reduce wall-clock time.

#### Usage persistence pipeline

```text
MCP response path
  -> non-blocking local channel
  -> 250 ms in-memory aggregation
  -> Redis Stream (durable handoff)
  -> one Redis-leased consumer
  -> idempotent PostgreSQL transaction
  -> Redis rolling usage windows
```

The request never falls back to a direct database write. Queue saturation is
observable through a dropped-event counter/log and intentionally affects only
approximate telemetry, not the user's tool result. Redis Stream batch IDs plus
`codelocal_mcp_usage_batches` prevent PostgreSQL double counting after retries or
consumer failover.

A generic `batch(actions=[...])` tool is not recommended for the default surface:
it weakens per-action safety metadata, complicates approval UX and can make one
large schema cost as many tokens as the legacy catalog. Prefer domain-local batch
operations such as `read(action=many)` and bounded internal fan-out.

## 11. Files likely to change

Primary:

- `internal/mcpgateway/server.go`
- `internal/mcpgateway/server_test.go`
- `internal/protocol/protocol.go`
- `internal/protocol/protocol_test.go`

Likely related:

- `internal/gateway/coordinator.go`
- `internal/localclient/engine.go`
- `internal/runtime/runtime.go`
- `internal/security/policy.go`
- `internal/security/policy_test.go`
- `internal/approval/*`
- `internal/usage/*`

Legacy TypeScript path may also need parity or explicit retirement decisions:

- `legacy/typescript-runtime/src/server-saas.ts`
- `legacy/typescript-runtime/src/server-v2.ts`
- `legacy/typescript-runtime/src/client-v2.ts`
- `legacy/typescript-runtime/src/protocol.ts`

Documentation:

- `README.md`
- `SMART_IDE.md`
- `MCP_HUB.md`

Exact files should be confirmed during implementation with `context_for_task` and references before edits.

## 12. Testing strategy

### Registry tests

- compact registry exposes only the intended public tools;
- names are stable;
- no private granular runtime command is advertised publicly;
- no target domain tool is missing.

### Mapping tests

Every private runtime command must map to exactly one internal operation ID or be explicitly marked obsolete.

### Schema tests

For every action:

- required fields are enforced;
- irrelevant fields are rejected or safely ignored according to policy;
- workspace routing is preserved;
- limits stay equivalent to current schemas.

### Security regression tests

At minimum:

- read-only operations remain read-only;
- file writes remain controlled;
- workspace escapes remain blocked;
- credential retrieval remains blocked;
- terminal preflight is still required;
- critical operations still require fresh ChatGPT approval;
- approval memory remains workspace-scoped;
- force push remains blocked;
- MCP extension calls remain subject to local policy.

### Compatibility tests

For each representative native runtime command, assert equivalent compact action behavior.

Example:

```text
find_definition(...) == lsp(action=definition, ...)
```

at the internal operation/result level.

### Workflow tests

Add high-level tests that simulate the common ChatGPT sequence instead of only testing isolated tools.

## 13. Rollout / rollback strategy

The product decision is a hard public cutover to the compact surface. There is
no runtime flag that can re-advertise the 77 granular commands. Rollback requires
an explicit code revert and Cloud restart, while workspace data, approvals,
credentials and user projects remain unchanged. The internal native command
router stays stable so the public cutover does not require a data migration.

## 14. Tool surface versioning

Introduce an explicit conceptual version for the public contract, for example:

```text
Tool Surface v1 = 77 granular tools
Tool Surface v2 = compact domain tools
```

This should be distinct from the transport protocol version when possible. Tool-surface evolution and wire-protocol evolution are different concerns.

A workspace/client capability can state supported surfaces, while the MCP server decides what to advertise for a session.

## 15. MCP Hub must stay lazy

One reason CodeLocal can remain compact is that external MCP capability is already discoverable dynamically.

Keep the pattern:

```text
mcp(action=search, query=...)
-> mcp(action=info, server=..., tool=...)
-> mcp(action=call, ...)
```

Do not expose every GitHub/Figma/Playwright/etc. extension tool at CodeLocal initialization time. Otherwise reducing 77 first-party tools would simply move the tool explosion elsewhere.

## 16. Orchestration guidance after migration

The server instructions can become simpler.

Proposed behavior:

1. Select/reuse a workspace.
2. For coding/debug/review/refactor, call `context` early.
3. Use `lsp` for exact code relationships.
4. Use `read` only for targeted context expansion.
5. Use `search` for literal strings/config/log text or structural file discovery.
6. Use `edit` for mutation.
7. Use `verify` after edits.
8. Use `terminal` + `process` for execution.
9. Use `git` only when Git operations are actually requested/needed.
10. Use `mcp` lazily for installed extension capability.

This preserves the semantic-first behavior CodeLocal already wants while reducing the number of decisions the model must make.

## 17. Acceptance criteria

The migration is considered successful when all of the following are true:

- [x] Modern sessions expose exactly 19 first-party CodeLocal tools.
- [x] All important capability from the current 98 tools remains reachable through compact operation mappings.
- [ ] No security/approval regression exists.
- [x] Workspace/thread routing behavior remains correct in gateway tests.
- [x] Existing projects require no data migration.
- [x] Native runtime protocol command compatibility remains intact.
- [x] MCP Hub remains lazy/dynamic.
- [x] Tool-schema serialized size remains materially lower than the conservative 77-tool baseline (52.8% after Browser and Computer Use in the current fixture).
- [ ] Representative coding workflows require the same or fewer model/tool round trips.
- [ ] Invalid tool-selection/retry rate does not regress.
- [x] Full Go tests pass.
- [ ] Relevant TypeScript compatibility tests pass while the legacy implementation remains supported.

## 18. Implemented order

1. Inventoried the 77 native runtime commands.
2. Introduced stable internal operation IDs and metadata.
3. Added 18 compact domain schemas and action resolvers, including Browser and Computer Use.
4. Added operation coverage, routing, safety and schema-size tests.
5. Removed granular public advertisement and its feature flag.
6. Kept native protocol commands private and stable.
7. Continued real ChatGPT workflow and latency evaluation after cutover.

## 19. Decision summary

**Decision:** CodeLocal should reduce its model-facing first-party MCP catalog substantially, but should not reduce its actual runtime capability.

**Current:** 19 public domain tools backed by the private native runtime command surface.

**Recommended target:** approximately **16 domain-oriented public tools**, allowing a practical final range of **15-20** after safety/schema testing.

**Architecture principle:**

```text
small public semantic surface
        ↓
stable internal operation router
        ↓
full granular CodeLocal capability
```

This gives ChatGPT fewer choices while keeping CodeLocal as capable as it is today.

## 20. Cross-thread continuity and long-conversation handoff

### 20.1 Do not depend on a ChatGPT thread ID

CodeLocal currently receives an MCP session through the MCP SDK and uses `req.Session.ID()` to isolate workspace routing. This is useful as a transport/session identity, but it must **not** be treated as the canonical ChatGPT conversation/thread ID.

The ChatGPT MCP integration does not currently provide CodeLocal with a documented, stable ChatGPT conversation ID contract. A transport session may be recreated, reconnected, or evolve independently from ChatGPT's internal conversation identity.

Therefore:

- keep using MCP session ID for in-session routing/isolation;
- never expose it to users as "ChatGPT thread ID";
- never make continuity depend on the MCP session surviving;
- introduce a CodeLocal-owned handoff identity for cross-thread continuation.

### 20.2 Proposed public tool: `handoff`

Add one compact first-party tool dedicated to conversation continuity.

Recommended actions:

- `create`
- `resume`
- `status` (optional)
- `delete` (optional/user privacy)

If implemented now, this would change the current compact surface from 19 to **20 public tools**, which is acceptable because continuity is a distinct user-facing capability rather than an implementation alias.

Conceptually:

```text
ChatGPT thread A
  -> handoff(action=create, structuredSummary=...)
      -> CodeLocal stores compact continuation state
          -> returns handoffId = "7K3M9Q8D"

User opens ChatGPT thread B
  -> "@Code tiếp tục 7K3M9Q8D"
      -> handoff(action=resume, id="7K3M9Q8D")
          -> restore task state + workspace route
              -> continue work
```

Do **not** require the user to paste the full summary into the new conversation.

### 20.3 Handoff ID

Use a short CodeLocal-owned ID, for example:

```text
7K3M9Q8D
```

Properties:

- generated server-side;
- scoped to authenticated CodeLocal user;
- case-insensitive if practical;
- avoid visually ambiguous characters (`0/O`, `1/I/L`) if using a human-facing alphabet;
- enough entropy to make guessing impractical;
- lookup must always include `userId`, never only the short ID.

An 8-character human-safe base32-style ID is a reasonable starting point. The exact length should be validated against expected record volume and collision strategy.

### 20.4 What a handoff record stores

Store a **compact structured continuation state**, not the full ChatGPT transcript.

Recommended fields:

```text
handoffId
userId
sourceMcpSessionId        // diagnostic only, not the continuity key
workspaceKey
workspaceName
branch
goal
userIntent
completedWork[]
currentState
importantDecisions[]
relevantFiles[]
changedFiles[]
commandsOrChecksRun[]
blockers[]
nextSteps[]
openQuestions[]
createdAt
updatedAt
expiresAt
```

Optional objective state can be added automatically by CodeLocal where cheap and safe:

- selected workspace;
- current branch;
- Git dirty/clean summary;
- changed path list;
- running CodeLocal process IDs;
- client/workspace availability.

Do not store:

- the complete conversation transcript;
- raw tool outputs by default;
- credentials, tokens, environment secrets, or sensitive file contents;
- unlimited source snippets.

The model-generated summary should be bounded, for example a low tens-of-KB hard maximum, with a substantially smaller normal target.

### 20.5 Resume behavior

`handoff(action=resume, id=...)` should:

1. authenticate the current user;
2. look up the handoff only inside that user's namespace;
3. load the structured summary;
4. identify the saved workspace;
5. lazily activate/select that workspace for the new MCP session when still authorized;
6. report branch/current Git state and detect if reality has diverged from the saved handoff;
7. return the compact continuation packet to ChatGPT;
8. let ChatGPT continue from `nextSteps` instead of rescanning the whole repository.

If the workspace no longer exists or is unauthorized, return the summary but do not silently route to another project.

If the branch/file state changed after the handoff, explicitly surface the divergence before mutation.

### 20.6 The server cannot know the full ChatGPT context window

CodeLocal sees its own MCP requests/results, not every ChatGPT user/assistant message and not the model's complete internal context state. Therefore the server cannot reliably say:

> "The ChatGPT context is 95% full."

Do not build a false exact context meter.

Instead use a **CodeLocal session-pressure heuristic** based only on observable MCP activity.

Possible signals:

- number of CodeLocal tool calls in the MCP session;
- cumulative tool-result bytes / estimated tokens;
- cumulative tool-argument bytes;
- number of large reads;
- number of distinct files returned;
- number of semantic/context packets;
- session age;
- number of edits/terminal workflows;
- repeated rescans of the same repository regions.

The metric should be described internally as `sessionPressure` or `toolContextPressure`, not `chatgptContextPercent`.

### 20.7 Suggested pressure levels

Use empirically tuned levels rather than pretending to know the model's real remaining context.

Concept:

```text
normal   -> no notice
high     -> recommend creating a handoff soon
critical -> create/refresh a handoff and strongly recommend a new chat
```

Thresholds should be benchmark-driven and configurable. Avoid hard-coding a context-window size because ChatGPT models and context management can change independently of CodeLocal.

### 20.8 Notice delivery

The MCP gateway already has a mechanism for adding one-time notices to tool results. Extend that concept to session-pressure notices.

Requirements:

- notice only when a threshold is crossed, not on every tool call;
- at most one warning per pressure level per MCP session unless state materially changes;
- never interrupt a critical mutation halfway through;
- ideally generate/refresh the handoff after the current coherent task step is completed;
- keep wording precise: CodeLocal observed a large amount of tool context, not that ChatGPT definitely exhausted its window.

Suggested user-facing Vietnamese copy after a handoff exists:

```text
Cuộc trò chuyện này đã tích lũy khá nhiều ngữ cảnh CodeLocal.
Để tiếp tục ổn định và tránh phải đọc lại dự án, bạn nên mở một cuộc trò chuyện mới và gửi:

@Code tiếp tục 7K3M9Q8D
```

Shorter variant when pressure is critical:

```text
Cuộc trò chuyện này đã khá dài. Mở chat mới và gửi:

@Code tiếp tục 7K3M9Q8D
```

Avoid telling the user that CodeLocal knows the real ChatGPT context percentage.

### 20.9 Who creates the summary

Best design: hybrid model + CodeLocal.

ChatGPT provides semantic conversation state that the server cannot infer reliably:

- what the user ultimately wants;
- decisions and tradeoffs already agreed;
- what has been tried and why;
- unresolved questions;
- intended next step.

CodeLocal augments it with objective project state:

- workspace key;
- branch;
- changed files;
- Git status/hash where appropriate;
- commands/checks that CodeLocal itself observed;
- runtime/process state.

This is better than asking CodeLocal to reconstruct the conversation from tool logs and better than asking the model to guess the repository state.

### 20.10 Handoff freshness

A handoff should be refreshable while the current conversation continues.

Possible behavior:

```text
handoff(create) -> 7K3M9Q8D
handoff(create, existingId=7K3M9Q8D) -> refresh same handoff
```

This prevents accumulating many handoff rows during one long task and gives the user one stable continuation code.

Recommended lifecycle:

- one active handoff per user + MCP session + workspace/task where practical;
- refresh on meaningful milestones or pressure-level changes;
- TTL/expiry for abandoned handoffs;
- explicit delete support if handoffs are persisted in cloud storage.

### 20.11 Storage and database constraints

This feature must stay lightweight.

Recommended:

- one compact row/document per active handoff;
- update-in-place rather than append one row per tool call;
- no raw terminal history duplication;
- no per-message transcript storage;
- bounded JSON payload;
- expiration cleanup;
- indexes on `(user_id, handoff_id)` and expiry as needed.

This aligns with CodeLocal's existing goal of avoiding high-volume database logging.

### 20.12 Security and privacy

Handoff records can contain sensitive project context even without raw source code.

Requirements:

- authenticated user scope is mandatory;
- another CodeLocal account must never resolve a guessed handoff ID;
- redact secrets before persistence;
- do not persist environment values;
- limit free-form model summary size;
- validate/sanitize structured fields;
- audit creation/resume/delete only as lightweight events, without copying the summary into audit logs;
- support expiry/deletion.

### 20.13 Interaction with workspace/session routing

Current responsibility split should become:

```text
MCP session ID
  = isolate routing inside the current MCP transport/session

workspaceKey
  = identify the authorized CodeLocal project

handoffId
  = durable user-facing continuation handle across ChatGPT threads/sessions
```

These three IDs solve different problems and should not be conflated.

### 20.14 Interaction with compact tool surface

If handoff is implemented, the current compact surface would become:

```text
device
workspace
project
context
agent
handoff
read
search
dependency
lsp
edit
verify
git
terminal
process
approvals
security
mcp
browser
computer
```

Total: **20 domain tools** before any future safety-driven split discovered during implementation.

`handoff` should remain its own tool rather than being hidden inside `context`, because it has durable cross-session semantics, persistence, ownership checks, and lifecycle operations.

### 20.15 Implementation phases for handoff

#### H0 — Verify transport assumptions

- add tests documenting current MCP session routing behavior;
- explicitly document that `req.Session.ID()` is not a ChatGPT conversation ID contract;
- ensure workspace selection continues to be scoped by MCP session.

#### H1 — Handoff store and API

- compact handoff data model;
- user-scoped ID generation/lookup;
- create/resume/delete operations;
- TTL cleanup;
- secret redaction and size limits.

#### H2 — Objective project-state augmentation

- save workspace + branch + changed paths;
- optionally save lightweight check/process state;
- compare saved state with current state on resume.

#### H3 — MCP public tool

- expose `handoff` in compact surface;
- keep schema small and action-discriminated;
- add tool metadata/tests;
- ensure resume can establish the new MCP session's workspace route.

#### H4 — Session-pressure heuristics

- track only in-memory/ephemeral aggregate MCP session metrics;
- use existing token/byte estimation utilities where appropriate;
- configure `high` and `critical` thresholds;
- do not write one DB record per tool call.

#### H5 — One-time continuation notice

- trigger a one-time notice at threshold crossing;
- create/refresh a handoff at a coherent milestone;
- return the exact continuation phrase to the model/user;
- avoid repeated nagging.

#### H6 — Real ChatGPT evaluation

Test at least:

1. long coding conversation -> warning -> new chat -> resume;
2. new chat on same workspace/branch;
3. new chat after branch changed;
4. workspace sleeping then resumed;
5. workspace revoked before resume;
6. guessed handoff ID from another account;
7. expired/deleted handoff;
8. multiple concurrent ChatGPT threads for the same repository;
9. handoff while uncommitted changes exist;
10. handoff after terminal/process work is still running.

### 20.16 Acceptance criteria for conversation continuity

- [ ] CodeLocal never labels MCP session ID as ChatGPT thread ID.
- [ ] A user can continue work in a fresh ChatGPT conversation using only a short CodeLocal handoff ID.
- [ ] Resume restores the correct workspace route for the new MCP session when authorized.
- [ ] The continuation packet preserves goal, decisions, current state, relevant files, and next steps.
- [ ] No full ChatGPT transcript needs to be stored.
- [ ] No per-tool-call database log is required.
- [ ] Handoff IDs are user-scoped and cannot cross accounts.
- [ ] Secrets are not persisted in handoff summaries.
- [ ] Saved-vs-current Git/project divergence is detected before continuing mutation.
- [ ] Long-session notices are one-time and non-spammy.
- [ ] Wording describes CodeLocal-observed context pressure rather than claiming exact ChatGPT context usage.

### 20.17 Recommended product behavior

The desired UX is:

```text
[CodeLocal detects high tool-context pressure]
        ↓
ChatGPT/CodeLocal refreshes a compact handoff
        ↓
User sees:
"Cuộc trò chuyện này đã khá dài. Mở chat mới và gửi:
 @Code tiếp tục 7K3M9Q8D"
        ↓
User opens a new ChatGPT conversation
        ↓
@Code tiếp tục 7K3M9Q8D
        ↓
CodeLocal resumes the saved workspace/task context
        ↓
ChatGPT continues without broad repository re-reading
```

This is more robust than relying on an opaque ChatGPT thread identifier and gives CodeLocal ownership of continuity across MCP sessions.
