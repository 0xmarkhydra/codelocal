# CodeLocal Code Graph + Technical Debt Master Plan

Status: **Approved for implementation**  
Date: **2026-08-18**  
Target branch: **dev**  
Owner: **CodeLocal**

Depends on:

- [`PROJECT_BRAIN_MASTER_PLAN.md`](./PROJECT_BRAIN_MASTER_PLAN.md)
- [`../MEMORY_GRAPH_PLAN.md`](../MEMORY_GRAPH_PLAN.md)
- [`../KNOWLEDGE_V2_COLLECTIVE_QUALITY_MASTER_PLAN.md`](../KNOWLEDGE_V2_COLLECTIVE_QUALITY_MASTER_PLAN.md)
- [`NEURAL_CONTROL_PLANE_DASHBOARD_MASTER_PLAN.md`](../ui/NEURAL_CONTROL_PLANE_DASHBOARD_MASTER_PLAN.md)
- [`../MULTI_REPO_MULTI_TASK_LOCAL_EXECUTION_PLAN.md`](../MULTI_REPO_MULTI_TASK_LOCAL_EXECUTION_PLAN.md)

This document is the source of truth for two connected workstreams:

1. **Code Graph** — make source-code relationships accurate, queryable and visible to users.
2. **Technical Debt Reduction** — remove structural risks that would otherwise make Code Graph, Project Brain and the dashboard harder to evolve safely.

The implementation order is intentionally dependency-driven: accuracy and runtime contracts come before visual polish.

---

# 1. Product decision

CodeLocal has two graph systems with different responsibilities.

```text
Project Brain / Knowledge Graph
        |
        | WHY
        | decisions / memory / skill / experience
        |
        +--------------------+
                             | stable semantic cross-link
                             v
                        Code Graph
                             |
                             | WHAT / HOW
                             | files / symbols / calls / imports
                             v
                        Source Code
```

## Knowledge Graph

- durable;
- cloud-backed;
- semantic and temporal;
- revision/provenance aware;
- user/project scoped;
- survives machine, checkout, branch and conversation changes.

## Code Graph

- local and repository-derived;
- revision/file-hash aware;
- rebuildable and disposable;
- optimized for symbol/import/call/type relationships;
- incrementally invalidated when source changes;
- must never become the durable Project Brain source of truth.

**Non-negotiable:** deleting or rebuilding Code Graph must never delete canonical Knowledge Graph data.

---

# 2. User-facing Code Graph placement

## 2.1 Primary route

Add a dedicated dashboard destination:

```text
CORE
  Overview
  Project Brain
  Code Graph
  Projects
  Skills
  Experiences
```

Primary route:

```text
/dashboard/code-graph
```

This is the only page that renders the full Code Graph explorer.

## 2.2 Workspace shortcut

Each authorized workspace may expose:

```text
[ Explore Code Graph ]
```

Deep-link with workspace/repository context already selected.

## 2.3 Project detail shortcut

Each repository under a logical project may expose:

```text
[ Explore code ]
```

Multi-repo projects must select the repository before symbol traversal.

## 2.4 Knowledge Graph bridge

Knowledge inspector may expose:

```text
Related code
  oauth.Register
  rotateTokenFamily
  token_family.go

[ Explore related code ]
```

Code Graph inspector may expose the reverse:

```text
Related knowledge
  2 Decisions
  1 Incident
  3 Verified Experiences
  1 Learned Skill

[ View in Project Brain ]
```

## 2.5 Overview

Do **not** render the full Code Graph on Overview. Overview may show a compact Code Intelligence shortcut only.

Reason: Code Graph is local-runtime dependent, revision-specific and potentially much larger than the durable Project Brain graph.

---

# 3. Primary Code Graph UX

```text
+-------------------------------------------------------------------+
| Code Graph                                           Runtime ●     |
|                                                                   |
| Project [CodeLocal v] Repository [codex-mcp v]                   |
| Checkout [MacBook/dev v] Revision [HEAD abc123]                   |
|                                                                   |
| Search symbol   [Node types v] [Relations v] [Depth 1 v] [Reset] |
+---------------------------------------------+---------------------+
|                                             |                     |
|                 GRAPH                       | Symbol Inspector    |
|                                             |                     |
|      passwordReset                          | UpdateUserPassword  |
|            |                                | Method              |
|            v                                | *cloud.Store        |
|   UpdateUserPassword                        | store.go:1354       |
|       |          |                          |                     |
|       v          v                          | Incoming      2     |
|      SQL       Audit                        | Outgoing      5     |
|                                             | References    3     |
|                                             | Provider     gopls  |
|                                             | Confidence   100%   |
|                                             |                     |
|                                             | Related Knowledge   |
+---------------------------------------------+---------------------+
```

V1 interactions:

- zoom / pan / drag;
- symbol search;
- focus selected symbol;
- expand incoming/outgoing relations;
- bounded traversal depth;
- node-kind filter;
- relation filter;
- reset/focus view;
- provider/confidence inspection;
- checkout/revision/freshness inspection;
- Knowledge Graph bridge when a stable cross-link exists.

---

# 4. Identity model — Project != Repo != Checkout != Revision

A Code Graph must never be identified only by `projectId`.

Required hierarchy:

```text
Logical Project
      |
      v
Repository
      |
      v
Checkout / Runtime
      |
      v
Revision
```

Example:

```text
CodeLocal
  |
  +-- codex-mcp
        |
        +-- MacBook A / dev / abc123
        |
        +-- MacBook B / feat-auth / def456
```

The two checkouts may contain different source and therefore different Code Graphs.

Minimum graph scope:

```text
repositoryId
workspace/device checkout identity
revision or equivalent source snapshot identity
```

---

# 5. Canonical Code Graph contracts

## 5.1 Node

Target DTO:

```json
{
  "id": "symbol:<stable-id>",
  "kind": "method",
  "repositoryId": "...",
  "path": "internal/cloud/store.go",
  "qualifiedName": "cloud.(*Store).UpdateUserPassword",
  "range": {"startLine": 1354, "startColumn": 1, "endLine": 1368, "endColumn": 2},
  "revision": "abc123",
  "language": "go",
  "provider": "gopls",
  "confidence": 1.0
}
```

## 5.2 Edge

```json
{
  "id": "edge:<stable-id>",
  "from": "symbol:...",
  "to": "symbol:...",
  "relation": "CALLS",
  "provider": "gopls",
  "confidence": 1.0,
  "revision": "abc123"
}
```

V1 relations:

```text
CONTAINS
IMPORTS
CALLS
REFERENCES
IMPLEMENTS
EXTENDS / EMBEDS
```

V2 candidates:

```text
TESTS
USES_TYPE
READS
WRITES
INSTANTIATES
OVERRIDES
```

Do not add relation types unless there is a reliable provider/fallback strategy.

---

# 6. Relationship authority and provenance

Authority order:

```text
LSP semantic relationship
        |
        v
language-aware AST / type analysis
        |
        v
structural parser
        |
        v
text discovery only
```

Rules:

- LSP/typed AST edges may be authoritative when resolved unambiguously.
- Structural/text results are discovery/fallback signals, not equal-confidence semantic edges.
- Every returned relationship must expose `provider`, `confidence`, `resolutionMode` and, when fallback occurs, `fallbackReason`.
- The UI must visually distinguish strong semantic edges from weak/possible relationships.

---

# 7. Confirmed current Code Graph technical debt

## CG-D1 — callers are text search

Current `get_callers` ultimately uses literal search for `name + "("`.

Consequence:

- declarations can be returned as callers;
- comments/strings/unrelated same-name symbols can match;
- scope/type/package identity is lost.

Confirmed example: `UpdateUserPassword(` matches two call sites **and the function declaration itself**.

## CG-D2 — callees are bounded regex scan

Current callees logic:

1. resolves the first approximate definition;
2. reads a fixed ~80-line window;
3. regex-matches `identifier(`;
4. deduplicates by bare name.

Consequences:

- false positives from following functions;
- false negatives after the 80-line window;
- type conversions/builtins may look like calls;
- same-name methods collapse;
- dynamic/interface dispatch is unresolved;
- generic calls may be missed.

## CG-D3 — no LSP call hierarchy

Real LSP call hierarchy is not implemented yet:

```text
textDocument/prepareCallHierarchy
callHierarchy/incomingCalls
callHierarchy/outgoingCalls
```

## CG-D4 — structural symbol index is regex-based

Current fallback symbol discovery is not a language AST.

Go receiver methods can be missed because `func (s *Store) Save(...)` does not match a simplistic `func <name>` shape reliably.

## CG-D5 — import graph is regex-based and language-unsafe

Current import extraction can parse import-looking text embedded inside another language's string fixture as a real edge.

Confirmed risk: JavaScript import fixture text inside a Go test source can become a false import graph edge.

## CG-D6 — fallback definition/reference resolution is fuzzy

Fallback definition/reference discovery does not reliably bind package, receiver, scope or overload identity.

## CG-D7 — cold polyglot provider behavior is incomplete

Workspace-symbol behavior may use only an already-active or first-started provider, leaving other languages to structural fallback until warmed.

**BA decision:** current callers/callees/import graph must not be presented to users as an authoritative Code Graph until CG0 accuracy work lands.

---

# 8. Graph scale and progressive expansion

Do not send/render the complete repository graph by default.

Bad model:

```text
2,000 files
35,000 symbols
150,000 edges
 -> browser
```

Required model:

```text
Repository
  -> Package / Module
  -> File
  -> Symbol
  -> Focused neighborhood
```

Default symbol exploration should return roughly:

```text
selected symbol
+ container
+ incoming depth 1
+ outgoing depth 1
```

Then user may request:

```text
Expand callers
Expand callees
Depth 2
```

Initial target budget: approximately **100–300 visible nodes**, bounded by server/runtime limits.

---

# 9. Freshness lifecycle

Code Graph must expose one of these states:

```text
CURRENT
STALE
REBUILDING
UNAVAILABLE / OFFLINE
UNSUPPORTED
```

Example UI:

```text
CURRENT · HEAD abc123 · indexed 4s ago
```

or:

```text
STALE · source changed since graph revision
```

or:

```text
REBUILDING · 154 / 402 files
```

Rules:

- source edits invalidate affected graph slices;
- file hash/revision must be part of freshness decisions;
- stale graph data must not silently masquerade as current;
- Cloud must not upload/store raw Code Graph merely to make offline rendering possible.

---

# 10. Offline and unsupported UX

When the relevant local runtime is offline:

```text
Code Graph requires the local runtime

Repository: codex-mcp
Last known checkout: MacBook Pro · dev
Runtime offline

Start CodeLocal on the device containing this checkout to inspect the current source graph.
Project Brain knowledge remains available.
```

For unsupported/missing language tooling:

- say which semantic provider is unavailable;
- show whether AST/structural fallback is active;
- expose reduced confidence rather than showing an empty graph without explanation.

---

# 11. Knowledge ↔ Code cross-link lifecycle

Knowledge cross-links use stable semantic identifiers, never canvas coordinates.

Target relationship examples:

```text
IMPLEMENTED_IN
RELATED_CODE
AFFECTS_CODE
VERIFIED_AGAINST
```

A link should retain enough identity to re-resolve after refactors:

```text
repositoryId
stableSymbolIdentity
lastKnownPath
lastKnownQualifiedName
lastResolvedRevision
status
```

Status:

```text
resolved
moved
renamed
stale
missing
ambiguous
```

Deleting/rebuilding Code Graph changes only resolution state; it must not delete durable knowledge.

---

# 12. User journeys / BA flows

## Flow A — find a function

```text
Code Graph
 -> search UpdateUserPassword
 -> resolve canonical symbol
 -> focus node
 -> inspect incoming/outgoing/reference relations
```

## Flow B — impact analysis

```text
select symbol
 -> expand incoming
 -> bounded depth 2
 -> show affected modules/tests/routes when provable
```

## Flow C — Knowledge to Code

```text
Project Brain decision/incident/experience
 -> Related Code
 -> Explore Code Graph
 -> open exact repository/checkout
 -> re-resolve target symbol
```

## Flow D — Code to Knowledge

```text
Code symbol
 -> Related Knowledge
 -> decision / incident / experience / skill
 -> View in Project Brain
```

## Flow E — multi-repo project

```text
Logical Project
 -> select repository
 -> select checkout/revision
 -> inspect graph
```

## Flow F — same project on two machines

```text
Mac A / dev / abc123
 !=
Mac B / feat-auth / def456
```

Edges must never be mixed across these graph snapshots.

## Flow G — source changes

```text
CURRENT
 -> file edit
 -> STALE
 -> rebuild affected slice
 -> CURRENT
```

---

# 13. Broader technical debt backlog

The Code Graph work must be coordinated with existing repository debt instead of adding another large layer on top.

## TD-D1 — `internal/cloud/store.go` God Object — P1

Observed characteristics from audit:

- ~2,300 physical lines;
- ~80 functions;
- owns DB/Redis, migrations, queues, workers, outbox, embeddings and multiple domain concerns;
- constructor also performs infrastructure initialization and starts workers.

Risk:

- changes have large blast radius;
- difficult targeted tests;
- lifecycle and failure semantics are coupled.

Direction:

```text
Store façade
  -> db/migrations
  -> usage pipeline
  -> knowledge store
  -> project store
  -> outbox/worker lifecycle
  -> vector/embedding adapter
```

Do not perform a giant rewrite. Extract along touched domain seams with compatibility wrappers.

## TD-D2 — oversized/high-complexity functions — P1

Audit hotspots include functions significantly above CodeLocal's own default quality policy for function length/complexity/nesting.

Known examples include migration, realtime runtime logic, cloud server construction, workspace sync, gateway HTTP serving, heartbeat and OAuth registration.

Direction:

- extract pure decision functions first;
- keep orchestration functions readable;
- add targeted tests before splitting behavior-heavy paths;
- enforce a ratchet: modified code must not worsen metrics.

## TD-D3 — CI does not run on direct `dev` pushes — P0

Current workflow push coverage does not protect the main development branch adequately.

Direction:

- run CI on pushes to `dev` as well as protected release branches;
- retain PR coverage;
- avoid changing release semantics in the same edit.

## TD-D4 — release provenance/version mismatch risk — P0

Current release flow can produce a package version that does not correspond cleanly to the source version recorded by the tag/commit.

Risk:

```text
tag -> source says version A
published npm artifact -> version B
```

Direction:

- define one source-of-truth version transition;
- ensure tag points at the exact source/artifact version being released;
- verify packaged metadata before publish;
- avoid CI-only mutation that is never committed.

## TD-D5 — legacy/shadow TypeScript runtime remains — P1

Native Go is the current primary runtime, but significant older TS runtime/control-plane code remains.

Risk:

- duplicate behavior;
- fixes land in wrong implementation;
- increased search/context noise;
- future contributors may mistake compatibility code for primary path.

Direction:

- inventory primary vs compatibility-only TS files;
- label ownership/status explicitly;
- isolate or retire unreachable paths gradually;
- never delete compatibility code until release/runtime usage proves it unused.

## TD-D6 — auth/security changes have high cross-layer blast radius — P0/P1

Authentication/session/password/device/IP security spans cloud server, web auth, store and runtime/gateway behavior.

Direction:

- treat auth as a protected subsystem;
- require focused tests for every security change;
- avoid mixing auth refactors with Code Graph implementation commits unless dependency is unavoidable.

## TD-D7 — migrations run during application startup — P1

Risk:

- startup availability tied to schema mutation;
- concurrent instances may compete;
- difficult rollback/release sequencing.

Direction:

- first make migration plan/versioning explicit and observable;
- later separate deploy migration gate from steady-state app startup where deployment architecture allows.

## TD-D8 — swallowed cleanup errors — P0/P1

Retention/maintenance paths contain best-effort DB operations whose errors can be discarded.

Risk:

- silent storage growth;
- degraded maintenance without observability.

Direction:

- classify cleanup as best-effort vs correctness-critical;
- emit structured warning/metric for best-effort failure;
- propagate correctness-critical failures.

## TD-D9 — background worker/bootstrap coupling — P1

Constructors/server bootstrap start long-lived workers/loops directly.

Risk:

- difficult lifecycle tests;
- shutdown ordering ambiguity;
- hidden side effects from `New(...)`.

Direction:

```text
New(...)      -> construct only
Start(ctx)    -> launch workers
Close()/Stop  -> deterministic shutdown
```

Apply incrementally to the highest-value components.

## TD-D10 — telemetry drop semantics need explicit contract — P1

Bounded telemetry queues may intentionally drop events when full.

This is acceptable only if the data is observational and non-authoritative.

Direction:

- document whether each queue is billing/correctness-authoritative or best-effort telemetry;
- count dropped events;
- never use a dropping path as the sole source for billing/security/correctness state.

## TD-D11 — oversized dashboard files / duplicated renderer risk — P0/P1

`internal/cloudserver/knowledge_dashboard.go` already contains a large amount of CSS/JS/rendering logic.

Do not clone it into a similarly large Code Graph file.

Direction:

```text
shared graph model
shared canvas renderer
shared toolbar primitives
shared inspector primitives
    /              \
Knowledge Graph   Code Graph
```

Extract incrementally before/during Code Graph UI work.

---

# 14. Combined dependency graph

```text
TD0 CI / release safety
        |
        +--------------------------------+
                                         |
CG0 semantic accuracy                    |
  canonical symbol identity              |
  LSP call hierarchy                     |
  Go AST/import fallback                 |
  provenance/confidence                  |
        |                                |
        v                                |
CG1 local Code Graph model               |
  revision + freshness                   |
  checkout scope                         |
        |                                |
        v                                |
CG2 bounded graph query service          |
        |                                |
        +------------------+-------------+
                           |
                           v
                    TD11 shared renderer
                           |
                           v
                    CG3 dashboard page
                           |
                           v
                    CG4 integrations
                           |
                           v
                    CG5 Knowledge bridge
                           |
                           v
                    CG6 impact analysis
                           |
                           v
                    CG7 architecture view
```

Broader debt (Store split, worker lifecycle, migrations, TS retirement) proceeds in parallel but must not block the first accurate Code Graph slice unless a direct dependency appears.

---

# 15. Implementation phases

## Phase TD0 — Development safety rails — P0

1. Add `dev` push coverage to CI.
2. Add regression tests/checks around release provenance before altering publish flow.
3. Make cleanup/telemetry failures observable where low-risk.
4. Keep auth/security isolated from unrelated refactors.

Exit criteria:

- direct dev pushes run CI;
- release provenance behavior is covered by tests or explicit validation;
- no new silent correctness failures are introduced.

## Phase CG0 — Code Intelligence accuracy — P0

1. Introduce canonical symbol identity using repo/path/qualified name/range/provider identity.
2. Implement LSP Call Hierarchy:
   - `textDocument/prepareCallHierarchy`
   - `callHierarchy/incomingCalls`
   - `callHierarchy/outgoingCalls`
3. Add `CallersAt` / `CalleesAt` project APIs that accept path/line/column/name.
4. Change localclient routing to use positional identity rather than bare name.
5. Keep text/regex callers/callees only as explicitly labeled fallback.
6. Add Go language-aware symbol/import/call fallback using `go/parser`, `go/ast`, and type information where practical.
7. Prevent embedded fixture strings/comments from creating import/call edges.
8. Return provider/confidence/resolutionMode/fallbackReason.

Mandatory tests:

- declaration is not a caller;
- Go receiver method resolves;
- same-name methods stay distinct;
- generic calls;
- long and short functions;
- comments/strings ignored;
- embedded fixture imports ignored;
- interface/implementation behavior documented/tested;
- polyglot provider fallback is explicit.

## Phase CG1 — Revision-aware local graph model — P0/P1

Add local rebuildable node/edge model with:

- repository ID;
- checkout identity;
- revision/file hashes;
- node kind;
- stable semantic symbol identity;
- edge provenance;
- freshness state.

Implement incremental invalidation where possible.

## Phase CG2 — Bounded query service — P1

Private runtime operations:

```text
resolve checkout
get graph status
find symbol
get neighborhood
get incoming
get outgoing
get imports
get implementations
```

Rules:

- bounded results;
- no full-repo graph dump by default;
- no public MCP tool-count expansion required;
- reuse compact `Code.lsp`/internal operation routing where appropriate.

## Phase TD11/CG3 — Shared renderer + `/dashboard/code-graph` — P1

Before duplicating Knowledge Graph UI:

- extract reusable Canvas2D graph renderer;
- share graph layout/input/inspector primitives;
- keep semantic styling supplied by each graph type.

Then add:

```text
/dashboard/code-graph
```

with project/repo/checkout/revision selectors and focused graph exploration.

## Phase CG4 — Entry-point integrations — P1/P2

Add:

- Workspace → Explore Code Graph;
- Project repository → Explore code;
- Overview → compact Code Intelligence shortcut.

## Phase CG5 — Knowledge ↔ Code bridge — P2

Implement stable cross-link DTO and re-resolution lifecycle.

Never store local canvas positions or raw graph payload as durable knowledge.

## Phase CG6 — Impact analysis — P2

For a selected symbol, derive bounded evidence such as:

```text
direct callers
transitive callers
implementations
related tests when provable
modules/packages touched
```

Risk labels must be explainable from graph evidence, not arbitrary AI scoring.

## Phase CG7 — Architecture/module view — P2/P3

Add collapsed views:

```text
Architecture
Files
Symbols
```

Architecture groups the graph by package/module/component so large repositories remain understandable.

## Phase TD1+ — Structural debt ratchet — parallel

Incrementally:

- split `internal/cloud/store.go` by stable domain seam;
- extract high-complexity functions when touched;
- separate constructor/start/stop lifecycle;
- move migration execution toward an explicit deploy/startup gate;
- inventory and isolate legacy TypeScript runtime paths;
- keep dashboard files bounded through shared components.

Do not perform a single giant refactor.

---

# 16. Acceptance criteria

Code Graph is not production-ready until all are true:

- [ ] declaration is not counted as caller;
- [ ] Go receiver methods resolve correctly;
- [ ] same-name symbols do not collapse across type/package boundaries;
- [ ] callers/callees expose provenance;
- [ ] graph is revision aware;
- [ ] source change causes stale/invalidate/rebuild behavior;
- [ ] multi-device checkouts do not mix graph snapshots;
- [ ] multi-repo projects route correctly;
- [ ] runtime offline state is explicit;
- [ ] unsupported semantic provider has explicit fallback state;
- [ ] Knowledge Graph remains usable while Code Graph is offline;
- [ ] deleting/rebuilding Code Graph never deletes durable knowledge;
- [ ] Knowledge cross-links can become stale/re-resolve instead of disappearing silently;
- [ ] raw source is not uploaded merely to render the dashboard;
- [ ] tenant/user routing cannot expose another user's runtime graph;
- [ ] graph expansion is bounded;
- [ ] UI does not represent weak text fallback as authoritative semantic truth;
- [ ] no fake relationships are rendered;
- [ ] old runtime versions fail gracefully/capability-gate advanced features;
- [ ] shared renderer prevents copy/paste duplication of Knowledge Graph implementation;
- [ ] direct `dev` pushes are protected by CI;
- [ ] release tags/artifacts have verifiable provenance;
- [ ] new work does not worsen repository quality-policy metrics.

---

# 17. Verification matrix

Every implementation slice starts narrow and widens only when shared contracts change.

Code intelligence:

```text
go test ./internal/lsp/...
go test ./internal/project/...
go test ./internal/localclient/...
```

Cloud/dashboard:

```text
go test ./internal/ui/...
go test ./internal/cloudserver/...
go test ./internal/cloud/...
```

Repository-wide gate when shared APIs/contracts change:

```text
git diff --check
go test ./...
go vet ./...
npm run typecheck
npm run test
```

Visual cases:

```text
runtime online
runtime offline
single repo
multi repo
same repo on two devices/branches
empty graph
small graph
large bounded neighborhood
missing LSP provider
fallback provider
stale/rebuilding graph
Knowledge cross-link resolved/stale/missing
mobile
reduced motion
```

---

# 18. Explicit non-goals

This program does **not** mean:

- uploading the repository to CodeLocal Cloud for graph rendering;
- merging Code Graph and Knowledge Graph storage;
- adding one public MCP tool per graph operation;
- rendering every symbol/edge in a repository at once;
- replacing the current Go runtime with a new graph framework;
- rewriting the dashboard in React/Next solely for this feature;
- deleting legacy TypeScript code without usage evidence;
- performing a giant `Store` rewrite before shipping Code Graph;
- inventing semantic confidence that the underlying provider cannot justify.

---

# 19. Final implementation rule

**Accuracy → identity → freshness → bounded query → shared renderer → UI → cross-link → impact analysis.**

Technical debt is handled as a ratchet around that sequence:

- fix P0 safety rails immediately;
- do not add new duplicated/oversized modules;
- extract unstable infrastructure only along proven seams;
- preserve compatibility while shrinking ambiguity.

A visually impressive but semantically incorrect Code Graph is considered a failed implementation.

---

# 20. Implementation progress — 2026-08-18

## Completed in the first implementation slice

- [x] **TD0 / CI:** direct pushes to `dev` now trigger the existing CodeLocal CI workflow.
- [x] **CG0 / LSP capability:** CodeLocal advertises and implements LSP Call Hierarchy using `textDocument/prepareCallHierarchy`, `callHierarchy/incomingCalls` and `callHierarchy/outgoingCalls`.
- [x] **CG0 / positional routing:** `get_callers` and `get_callees` now accept path/line/column and prefer semantic call hierarchy instead of bare-name search.
- [x] **CG0 / provenance:** semantic, AST and text fallback call relationships carry explicit provider/resolution/confidence metadata.
- [x] **CG0 / caller false positive:** text fallback filters function declarations instead of counting the declaration itself as a caller.
- [x] **CG0 / Go callee fallback:** Go AST traversal is scoped to the actual selected function body instead of a fixed 80-line window.
- [x] **CG0 / Go structural index:** Go methods/types/values/imports are indexed with Go AST rather than raw cross-language regex.
- [x] **CG0 / false import fixture:** JavaScript-looking import text embedded in Go strings no longer becomes a Go import edge.
- [x] **CG1 / repository source snapshot:** local Code Graph snapshot metadata includes repository identity, branch, commit, source hash, dirty state, indexed time, file count and symbol count.
- [x] **CG1 / revision identity:** graph revision combines Git commit identity with a source-content fingerprint so dirty working-tree changes produce a different graph revision.
- [x] **CG1 / canonical semantic symbol ID:** semantic LSP call-hierarchy items receive deterministic `symbolId`; AST/text fallbacks are intentionally not promoted to canonical symbols.
- [x] **CG1 / existing public surface:** Code Graph snapshot metadata is exposed through existing semantic info; the public MCP surface remains 19 tools.
- [x] **TD-D8 / retention observability:** retention cleanup moved out of the Store God Object into `internal/cloud/retention.go`; cleanup failures now emit structured warnings instead of being silently swallowed.
- [x] **TD-D1 ratchet:** the retention extraction reduces responsibility and executable LOC in `internal/cloud/store.go` without a large rewrite.

## Verification completed for this slice

- [x] focused `internal/lsp`, `internal/project`, `internal/localclient`, `internal/mcpgateway` tests;
- [x] `internal/cloud` tests;
- [x] `git diff --check`;
- [x] full `go test ./...`;
- [x] full `go vet ./...`;
- [x] `npm run typecheck`;
- [x] CodeLocal diagnostic regression: zero new diagnostics;
- [x] CodeLocal quality gate: ready after verification.

## Technical-debt progress after implementation

- [x] **TD-D4 release provenance — P0 resolved in source:** release now uses a two-stage transaction. The prepare stage materializes the reserved version into `package.json`, `package-lock.json` and `internal/version/version.go`, creates a dedicated release commit, tags that exact commit, pushes only the tag, then dispatches the publish stage from that tag. The publish stage refuses any non-tag/mismatched ref and never mutates version source files before npm publish. Static workflow invariant tests guard this contract. This avoids requiring a release bot commit to bypass `main` branch protection.
- [x] **TD-D2 localclient dispatch ratchet:** semantic/LSP/Code Graph routing moved from the oversized `Engine.handle` switch into `internal/localclient/code_intelligence.go`, reducing the touched God function instead of extending it with new graph behavior.
- [x] **TD-D3 dev CI:** direct `dev` pushes are covered by CI.
- [x] **TD-D8 cleanup observability:** retention cleanup errors are no longer silently discarded.
- [x] **TD-D9 lifecycle extraction — first safe stage:** DB/Redis bootstrap, store assembly, initialization and worker startup moved into `internal/cloud/store_runtime.go`; `New` is now a small coordinator and worker launch has one named boundary. This does not yet change migration timing, so deployment behavior remains compatible.
- [x] **Project intelligence God-file ratchet:** `project.go` dropped below the default 800 executable-LOC file limit by moving scanning to `project_scan.go` and task-context orchestration to `context_task.go`. The moved workflows were decomposed into bounded named helpers rather than relocating two oversized functions unchanged.
- [x] **TD-D10 telemetry authority contract:** bounded MCP usage is explicitly `telemetry-only`, non-durable and non-authoritative. A static regression test prevents `MCPUsageSummary` from leaking into decision packages outside the Cloud analytics/dashboard boundary, so dropped telemetry cannot silently become billing/entitlement/access authority.
- [x] **TD-D11 graph renderer duplication:** Knowledge and Code Graph share one renderer.

## Remaining debt / rollout gates after security + lifecycle hardening

- [x] **TD-D6 auth/security core hardening:** new browser sessions are bound to `user.security_version`, so security-changing password updates invalidate older sessions without relying only on timestamps; zero-version legacy sessions retain the timestamp compatibility path. Password-reset finalization is single-writer under concurrency and wrong OTP attempts use a per-token atomic Redis counter, preserving the advertised five-attempt ceiling. Existing IP/account rate limits, hashed device/network/agent signals and sensitive-route reauthentication remain in place. Future auth changes stay a protected subsystem and require focused regression tests.
- [x] **TD-D7 migration lifecycle — source implementation:** `CODELOCAL_MIGRATION_MODE` now provides `startup` (compatibility/default), `only` (migration process with no workers/HTTP server) and `external` (steady-state service skips DB schema DDL). Unknown modes fail closed. The Railway transition and rollback sequence is documented in `docs/operations/MIGRATION_ROLLOUT.md`. **Production rollout proof remains required** before removing the `startup` recovery path.
- [~] **TD-D5 legacy TypeScript runtime quarantined with a deletion gate:** root build/test/release scripts and `cmd/release` remain native-Go only; `legacy/typescript-runtime/README.md` defines measurable physical-deletion criteria. The files are isolated from canonical source roots and intentionally retained until deployed runtime inventory and external-consumer evidence satisfy that gate.
- [x] **TD-D1 Store God-file/domain split milestone:** `internal/cloud/store.go` fell from roughly 1,445 to roughly 594 executable LOC, below the default 800-LOC file limit. Runtime/bootstrap lives in `store_runtime.go`, retention in `retention.go`, user/auth/session persistence in `store_auth.go`, pairing/device/workspace persistence in `store_devices_workspaces.go`, and telemetry/admin persistence in `store_usage_admin.go`. Usage producer/consumer and persistence hot paths were also decomposed into named helpers without changing the telemetry authority contract. The historical `Store.Migrate` registry remains one isolated oversized-function advisory; its schema SQL/order is intentionally not bulk-rewritten inside this security/lifecycle transaction.

## Completed in the neural/dashboard implementation slice

- [x] **CG2 / bounded neighborhood:** local runtime exposes private `get_code_graph` with depth `1..3`, a hard node ceiling and bounded expansion count. It does not add a 20th public MCP tool.
- [x] **CG2 / compact transport:** callers/callees/import relationships are assembled locally and sent to Cloud as one bounded graph DTO instead of many browser-to-runtime round trips.
- [x] **CG2 / revision edges:** bounded graph edges have deterministic IDs derived from revision, endpoints, relation and evidence provider.
- [x] **CG2 / privacy:** graph DTOs contain identity/evidence metadata only; raw source bodies, AST payloads and LSP raw responses are not part of the dashboard transport contract.
- [x] **CG3 / shared renderer:** Knowledge Graph and Code Graph now share `internal/ui/neural_graph.go`; the old Knowledge Graph page no longer owns its own ~28 KB canvas/CSS/JS implementation.
- [x] **CG3 / neural visual language:** nodes have restrained halo/breathing, semantic paths can glow, occasional signal pulses travel along visible edges and low-confidence/text fallback edges are visually weaker/dashed.
- [x] **CG3 / accessibility/performance:** animation honors `prefers-reduced-motion`, pauses while the tab is hidden, caps device pixel ratio and renders only bounded payloads.
- [x] **CG4 / dashboard route:** `/dashboard/code-graph` is a first-class page next to Knowledge Graph.
- [x] **CG4 / context:** Code Graph exposes project/checkout/repository/revision context and shows current/offline/unsupported/unavailable states without pretending an empty graph is authoritative.
- [x] **CG4 / workspace shortcut:** Workspaces link directly into Code Graph; Overview also exposes an Explore Code Graph shortcut.
- [x] **CG5 / safe bridge:** Knowledge repository inspection can open Code Graph and Code Graph can open Knowledge Graph. Exact symbol-level deep links remain gated on a durable cross-link identity instead of guessing from display names.
- [x] **CG8 / Architecture View V1:** the default no-symbol graph aggregates imports into bounded structural module regions such as `internal/cloud`, `internal/project`, `cmd/codelocal` and `src`, with dependency clusters and aggregated edge counts. Users can switch to Files view; searching a symbol switches to the semantic bounded neighborhood.
- [x] **TD-D11 / renderer duplication:** resolved for graph pages by extracting one shared neural renderer rather than cloning `knowledge_dashboard.go`.

## Neural Graph product contract

The graph pages are CodeLocal's signature visual surface, not a global dashboard theme.

```text
normal dashboard
    |
    +-- Knowledge Graph  -> living neural presentation of durable knowledge
    |
    +-- Code Graph       -> living neural presentation of local source relationships
```

Visual rules:

1. **Utility remains primary.** Labels, inspector evidence, provider, confidence and fallback mode must remain readable before any animation is added.
2. **Breathing is subtle.** Visible nodes may vary radius only a few percent; there is no continuous aggressive flashing.
3. **Signals are sparse.** Only selected paths and an occasional deterministic subset of edges emit moving pulses.
4. **Evidence affects styling.** Semantic/high-confidence relations may glow strongest. AST/structural evidence is calmer. Text fallback is dashed/dim and explicitly warned in the inspector.
5. **Focus reduces noise.** Selecting a node brightens direct relationships and fades unrelated visible nodes.
6. **Reduced motion is authoritative.** `prefers-reduced-motion` disables signal travel and breathing effects rather than merely slowing them down.
7. **Hidden tabs stop animation work.** The renderer pauses its RAF loop while the document is hidden.
8. **No graph-wide spectacle on admin/account pages.** Neural FX stays scoped to graph surfaces so the rest of CodeLocal remains restrained and professional.

## Dashboard transport and bandwidth contract

The default architecture is **Local Full Graph -> bounded live projection -> browser**.

```text
Source + LSP + AST
      |
      v
Local Code Graph
      |
      | private get_code_graph
      | <= bounded node/edge slice
      v
Gateway / Cloud relay
      |
      v
Browser neural renderer
```

Rules:

- The full rebuildable Code Graph stays local.
- Cloud must not require repository upload to render Code Graph.
- The dashboard request is capped server-side (`180` visible nodes in the initial implementation; local engine hard cap `300`).
- Depth is capped at `3` and total local expansions are capped independently, preventing a high-degree symbol from exploding the response.
- The initial no-symbol view defaults to a smaller **Architecture** projection that aggregates import traffic into structural module/dependency regions; users may switch to bounded Files view, while symbol search switches to a bounded incoming/outgoing call neighborhood.
- One private runtime operation returns the assembled slice, avoiding N browser RPCs and reducing latency/bandwidth.
- If future offline Code Graph is introduced, it must use an explicitly approved compact projection/delta cache. It must not silently become a raw-source or full-graph Cloud backup.
- Future delta sync should key by repository + checkout/revision and transmit only added/removed/changed nodes/edges. The cost target is proportional to changed code, not repository size.

## Remaining Code Graph work

1. Extend language-aware structural parsing beyond Go where semantic LSP is unavailable, without reintroducing cross-language regex false edges.
2. Add asynchronous rebuild/freshness transitions (`STALE -> REBUILDING -> CURRENT`) if indexing becomes decoupled from the synchronous local query path. The current live query refreshes before responding, so it reports current rather than inventing stale state.
3. Add durable Knowledge -> Code cross-link records only after stable repository/symbol identity can survive rename/move/re-resolution; current UI provides safe graph-to-graph navigation without fabricating exact links.
4. **Impact Analysis V1 completed:** symbol views now summarize direct callers/callees, reverse reachable upstream callers, affected files, semantic/evidence edge counts, average confidence and a conservative bounded risk level. A truncated neighborhood is always treated conservatively. Future V2 may add test/module/API-route summaries only where explicit evidence exists.
5. **Architecture/module clustering V1 completed:** structural module regions and aggregated dependency edges are the default lightweight overview. Future V2 may add semantic component ownership only when backed by explicit project metadata/evidence.
6. **TD-D4 release provenance source fix completed:** two-stage immutable-tag publishing is implemented and guarded by workflow invariant tests. The first real GitHub run after push remains the deployment proof for repository rules/trusted-publishing configuration.
7. **Pre-main Go semantic relationship hardening completed:** workspace LSP symbols are normalized to path/line/column/container/kind with semantic provenance; Go fallback uses a fully type-checked `go/types` package before emitting `CONTAINS`, `IMPLEMENTS`, `EMBEDS` or interface `EXTENDS` edges. Type-check failures emit no type relationship instead of guessing.
8. **Duplicate-symbol ambiguity is fail-closed:** multiple receiver methods such as `Store.Save` and `Cache.Save` are never collapsed into the first textual match. Unqualified `Save` returns an explicit ambiguous state; a qualified receiver query can resolve the intended method.
9. **Generic call regression covered:** Go AST call extraction recognizes indexed/generic function and selector calls such as `Generic[int]()` and `service.Run[string]()` without inventing unrelated edges.
10. **Pre-main rollout proof remains external where appropriate:** deployed browser smoke and a real immutable-tag npm release are rollout proofs after CI is green; source tests do not pretend to prove a deployment that has not happened.

---

# 21. Neural Graph BA acceptance criteria

A neural graph release is accepted only when all of the following hold:

- [x] Opening Code Graph never requires uploading raw repository source.
- [x] The browser never receives an unbounded full-repository graph.
- [x] Workspace, repository and revision context are visible to the user.
- [x] Offline and unsupported runtimes explain the required action instead of showing a misleading empty graph.
- [x] Search can resolve a symbol and focus a bounded neighborhood.
- [x] Depth cannot exceed the server/runtime safety ceiling.
- [x] Semantic and fallback relationships are visually distinguishable.
- [x] Occasional signal pulses are drawn beneath nodes/labels and do not replace inspector evidence.
- [x] Reduced-motion users receive a static graph with the same information.
- [x] Background tabs stop the renderer animation loop.
- [x] Mobile CSS preserves a bounded canvas with the inspector moved below it.
- [x] Knowledge Graph still reads the same durable Cloud graph data after adopting the shared renderer.
- [x] Code Graph is a local rebuildable projection and has no delete path into canonical Knowledge Graph state.
- [x] The public MCP tool count remains stable at 19; Code Graph uses a private runtime operation.
- [x] Unit/full tests, JavaScript syntax check, vet, diff check and project typecheck pass before rollout.

Verified in the local `dev` working tree on 2026-08-18. This is a source/CI acceptance gate; the first deployed browser smoke test and first real two-stage npm release remain rollout proofs after the batch is committed/pushed.

The final visual target is **a living neural control surface backed by explicit evidence**, not decorative animation detached from graph truth.
