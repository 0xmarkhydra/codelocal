# CodeLocal Repository Structure and Cleanup Plan

Status: **Active architecture plan**  
Date: **2026-08-19**  
Owner: **CodeLocal**

> Goal: make the repository predictable enough that a new engineer or coding agent can identify the canonical runtime, find the owner of a feature, and know where a new file belongs without broad repository search.

---

## 1. Current diagnosis

The repository is functional, but navigation cost is too high.

### 1.1 Root is overloaded

The repository root currently mixes:

- build/release manifests;
- product roadmap/status;
- many `*_PLAN.md` / `*_MASTER_PLAN.md` files;
- Docker/release files;
- an unexplained `1.5.0` file;
- assets;
- source entrypoints and legacy source roots.

This weakens the signal of what is canonical.

### 1.2 Two runtime generations are visually peers

Production is Go-first under `cmd/` and `internal/`, but the quarantined TypeScript runtime still exists under `src/` with many implementation files and tests.

Even though release gates already prevent the legacy TypeScript runtime from becoming the production runtime, the directory name `src/` still looks authoritative to humans and tools.

### 1.3 Several Go packages are becoming internal monoliths

The highest-density packages are:

- `internal/cloud`
- `internal/cloudserver`
- `internal/automation`
- `internal/mcpgateway`
- `internal/project`

They contain valid code, but multiple subdomains now coexist inside one package. Further growth without ownership rules will turn these into dumping grounds.

### 1.4 Documentation has two homes

Some plans are already under `docs/`, while many major plans remain at root. Cross-links therefore use inconsistent relative paths and future agents are likely to keep adding more root documents.

---

## 2. Target top-level layout

The long-term repository should converge toward:

```text
/
├── AGENTS.md
├── README.md
├── ROADMAP.md
├── STATUS.md
├── go.mod
├── go.sum
├── package.json
├── package-lock.json
├── Dockerfile
├── Dockerfile.reviewer
├── railway.json
├── .github/
├── backend/                      # target Go cloud/API/server product surface
├── web/                          # Next.js + TypeScript browser product
├── app/                          # Flutter + Dart mobile app (iOS/Android)
├── desktop/                      # Flutter + Dart desktop app (macOS/Windows/Linux)
├── cli/                          # target Go CLI/local-runtime surface
├── native/                       # OS-specific bridges (Swift/macOS first)
├── tools/                        # reviewer/release/developer tooling
├── docs/
├── scripts/
├── cmd/                          # transitional Go entrypoints until migrated safely
├── internal/                     # transitional/canonical Go domains until migrated safely
├── src/                          # quarantined legacy TypeScript until deletion gate passes
└── legacy/                       # eventual quarantine destination only if deletion is not yet possible
```

The accepted technology ownership for these surfaces is defined in [`PRODUCT_STACK.md`](./PRODUCT_STACK.md). The product-level directories are a convergence target; `cmd/` and `internal/` remain canonical Go implementation locations until each dependency-safe migration lands.

The root should answer only four questions:

1. What is this project? → `README.md`
2. What is the current direction? → `ROADMAP.md`
3. What is the current state? → `STATUS.md`
4. How is it built/run? → manifests/configuration

Everything else should live under an owned directory.

---

## 3. Documentation taxonomy

Target:

```text
docs/
├── README.md
├── architecture/
│   ├── REPOSITORY_STRUCTURE.md
│   ├── SYSTEM_OVERVIEW.md
│   ├── PROJECT_BRAIN.md
│   ├── RUNTIME.md
│   ├── SECURITY_BOUNDARIES.md
│   └── UI_ARCHITECTURE.md
├── plans/
│   ├── intelligence/
│   ├── runtime/
│   ├── automation/
│   ├── execution/
│   ├── ui/
│   ├── integrations/
│   └── platform/
├── guides/
│   ├── USER_GUIDE.md
│   ├── DEVELOPER_GUIDE.md
│   └── HOW_CODELOCAL_WORKS.md
├── operations/
│   ├── RELEASE.md
│   ├── BETA_CHANNEL.md
│   ├── MIGRATION_ROLLOUT.md
│   └── EMAIL_NOTIFICATIONS.md
├── integrations/
│   └── openai/
│       └── submission/
└── archive/
```

### Root document migration map

Recommended destinations:

```text
AGENT_MEMORY_PLAN.md
  -> docs/plans/intelligence/AGENT_MEMORY_PLAN.md

KNOWLEDGE_V2_COLLECTIVE_QUALITY_MASTER_PLAN.md
MEMORY_GRAPH_PLAN.md
PROJECT_BRAIN_MASTER_PLAN.md
  -> docs/plans/intelligence/

UNIVERSAL_AGENT_RUNTIME_PLAN.md
SMART_COMPUTER_RUNTIME_PLAN.md
DESKTOP_AUTOMATION_PLAN.md
AUTOMATION_PLATFORM.md
  -> docs/plans/runtime/ or docs/plans/automation/

MCP_HUB.md
MCP_TOOL_SURFACE_PLAN.md
  -> docs/plans/integrations/

MULTI_REPO_MULTI_TASK_LOCAL_EXECUTION_PLAN.md
OPENSANDBOX_SAFE_EXECUTION_MASTER_PLAN.md
CLAUGE_WORKFLOW_INTEGRATION_PLAN.md
  -> docs/plans/execution/

PRODUCT_UI_MASTER_PLAN.md
NEURAL_CONTROL_PLANE_DASHBOARD_MASTER_PLAN.md
DESIGN_RECIPE_LIBRARY_MASTER_PLAN.md
  -> docs/plans/ui/

PRODUCTION_PLAN.md
  -> docs/operations/

SMART_IDE.md
  -> docs/plans/product/ or archive after deciding whether it is still canonical
```

Moves must be atomic with reference updates. Do not move all documents in a behavior-changing commit.

---

## 4. Canonical source ownership

### 4.1 Executables

```text
cmd/codelocal/          CLI/runtime entrypoint
cmd/codelocal-cloud/    Cloud server entrypoint
cmd/codelocal-reviewer/ Reviewer/test service entrypoint
cmd/computerhelper/     Cross-platform computer helper executable
cmd/computernative/     Native macOS Swift worker
cmd/release/            Release packaging orchestration
```

Rule: `cmd/*` composes dependencies. Business/domain logic belongs under `internal/`.

### 4.2 Core domains

```text
internal/agentruntime/      agent runtime registry/capabilities
internal/orchestration/     planner/executor/recovery/progress
internal/mcpgateway/        public MCP contract and tool surface
internal/mcphub/            MCP hub coordination

internal/project/           code/project intelligence
internal/projectbrain/      durable project knowledge compilation
internal/projectidentity/   logical project identity
internal/repository/        repository registry
internal/lsp/               language intelligence adapters

internal/runtime/           runtime behavior and cloud sync
internal/localclient/       local request execution/routing
internal/taskexecution/     task/worktree execution
internal/taskstate/         task persistence/state

internal/automation/        browser/computer automation domain
internal/approval/          approval broker/memory
internal/security/          local execution/security policy

internal/cloud/             cloud domain/persistence
internal/cloudserver/       HTTP transport + dashboard composition
internal/ui/                shared server-rendered UI primitives
internal/webauth/           web sessions/forms/security flows
internal/oauth/             OAuth protocol
internal/deviceauth/        device request signing
internal/webutil/           web request/security helpers

internal/localfs/           local filesystem boundary
internal/process/           process/PTY lifecycle
internal/osutil/            OS process utilities
internal/runtimecontrol/    local runtime control transport
internal/mediatransport/    media publication/transport
internal/mailer/            email provider adapters
internal/usage/             usage accounting
internal/version/           version contract
```

---

## 5. Legacy TypeScript quarantine

Current `src/` is intentionally retained because deletion gates are not yet fully satisfied.

### Immediate rule

Treat:

```text
src/**
```

as **legacy reference code, not a default modification target**.

### Desired end state

After confirming:

- no production script executes legacy runtime entrypoints;
- no external supported integration imports `src/*`;
- no release artifact requires it;
- required migration/regression evidence is captured;

then choose one of two outcomes:

#### Preferred

Delete the legacy TypeScript runtime entirely.

#### Transitional fallback

Move it to:

```text
legacy/typescript-runtime/
```

with its own README and isolated tooling.

Do not move it merely to make the repository look cleaner if release/test consumers still rely on paths.

---

## 6. Large-package decomposition plan

Package splitting should reduce domain coupling, not just file counts.

### 6.1 `internal/cloud`

Current concerns include auth/session storage, device/workspace state, Knowledge V2, canonical graph/embeddings, collective intelligence, experiences, skills, promotion, usage/admin, outbox and retention.

Target decomposition direction:

```text
internal/cloud/               shared cloud store/contracts only
internal/cloud/authstore/     user/session/security persistence
internal/cloud/runtime/       device/workspace/presence state
internal/cloud/knowledge/     canonical knowledge + sources + graph
internal/cloud/semantic/      embeddings/canary/retrieval
internal/cloud/learning/      outbox/promotion/experience/skills
internal/cloud/admin/         usage/referral/admin/organization
```

This is a direction, not permission for an immediate giant move. Extract one cohesive subdomain at a time.

### 6.2 `internal/cloudserver`

Separate transport composition by user-facing area:

```text
internal/cloudserver/
  server.go
  runtime_ws.go
  media.go
  release_*.go

internal/cloudserver/dashboard/
  overview.go
  knowledge.go
  code_graph.go
  devices.go
  workspaces.go
  account.go
  admin.go

internal/cloudserver/public/
  legal.go
  support.go
```

Go import-cycle constraints may require package boundaries different from this exact sketch. The principle is to stop one package from owning every page and every transport concern.

### 6.3 `internal/automation`

Target conceptual split:

```text
automation/browser
automation/computer
automation/inputpolicy
automation/scene
```

First extract pure policy/model code; keep controller integration stable until dependency direction is clear.

### 6.4 `internal/mcpgateway`

Target conceptual split:

```text
mcpgateway/server
mcpgateway/tools
mcpgateway/knowledge
mcpgateway/compat
mcpgateway/lifecycle
```

Public tool schema/version logic should have a clearly owned location because it is a compatibility boundary.

### 6.5 `internal/project`

Target conceptual split:

```text
project/model
project/scan
project/codegraph
project/query
project/impact
```

Do not split call graph/query/impact until public types and dependency direction are mapped; otherwise Go package cycles are likely.

---

## 7. File naming conventions

Use responsibility-based names.

Good:

```text
security_state.go
workspace_routing.go
token_family.go
code_graph_query.go
computer_scheduler.go
release_email.go
```

Avoid:

```text
utils.go
helpers.go
common.go
misc.go
new.go
manager2.go
stuff.go
temp.go
```

Tests stay beside the implementation they validate.

Platform-specific Go files use standard suffixes:

```text
*_darwin.go
*_linux.go
*_windows.go
*_unix.go
```

---

## 8. New-file decision tree

Before adding a new file:

```text
Is it documentation?
  yes -> docs/<category>/

Is it an executable entrypoint?
  yes -> cmd/<binary>/

Is it release/CI tooling?
  yes -> scripts/, cmd/release/, .github/

Is it product logic?
  yes -> identify the owning internal domain

Does no domain clearly own it?
  -> architecture smell: define ownership before adding a directory/file
```

A feature should not create a new root-level folder simply because existing packages feel crowded.

---

## 9. Migration phases

### RA0 — Guardrails now

- [x] Add root `AGENTS.md`.
- [x] Define canonical Go vs legacy TypeScript ownership.
- [x] Define documentation taxonomy.
- [ ] Add repository structure checks to CI later.

No production behavior changes.

### RA1 — Root/doc cleanup

- Move root plans into `docs/plans/<domain>/`.
- Update all Markdown references atomically.
- Move existing docs into the new guide/operations/integration taxonomy where useful.
- Investigate and remove/relocate the root `1.5.0` artifact if it is not required.
- Keep only README/ROADMAP/STATUS as human-facing root docs.

Verification:

```text
git diff --check
repo-wide link/reference search
npm/go checks only if scripts or paths changed
```

### RA2 — Legacy runtime evidence + removal decision

- Re-run legacy runtime dependency inventory.
- Confirm release workflow paths.
- Confirm external consumer evidence.
- Delete legacy TypeScript runtime when the existing gate passes; otherwise isolate under `legacy/` with path-compatible tooling.

Verification:

```text
npm run ci
go test ./...
go run ./cmd/release
npm pack --dry-run ./.release/npm
```

### RA3 — Next.js web migration

The destination browser architecture is `web/` using Next.js App Router + TypeScript. Align implementation with `PRODUCT_UI_MASTER_PLAN` while keeping the existing Go-rendered UI as a compatibility fallback until route-family parity is proven.

Target direction:

```text
web/
  src/app/                route families and layouts
  src/components/         reusable product UI
  src/features/           dashboard/graph/account feature boundaries
  src/lib/server/         server-only Go backend HTTP access
  src/styles/             shared design-system primitives when needed
```

Migration order:

1. establish Next.js foundation and design tokens;
2. define explicit Go backend DTO/API boundaries;
3. migrate auth with session/CSRF/rate-limit parity;
4. migrate dashboard route families incrementally;
5. migrate Knowledge Graph and Code Graph using real server data;
6. cut over only after parity tests and rollback proof;
7. remove the old Go rendering layer only after no production route depends on it.

Do not duplicate backend authorization, billing or Project Brain truth in TypeScript. Do not use fake telemetry during visual migration.

### RA4 — Cloud domain extraction

Start with the clearest low-coupling subdomain, likely auth/session store or usage/admin rather than Knowledge V2 core.

For each extraction:

1. dependency/caller map;
2. explicit package API;
3. move implementation + tests;
4. narrow tests;
5. `go test ./...`;
6. no behavior/UI changes in same commit.

### RA5 — Cloudserver surface split

Separate public/legal pages, dashboard surfaces and runtime/API transports without changing URLs.

### RA6 — Automation/MCP/project decomposition

Only after the higher-value root/legacy/cloud cleanup lands. These are core runtime packages and have higher regression risk.

### RA7 — Architecture enforcement

Add lightweight repository checks:

- reject new root `*_PLAN.md` documents;
- flag new production changes under legacy `src/` unless explicitly allowed;
- optionally warn on oversized files/packages;
- verify docs links;
- maintain package ownership map.

---

## 10. Commit strategy

Repository restructuring should use dedicated commits such as:

```text
chore(repo): add repository ownership guardrails
chore(docs): organize architecture and plans
chore(legacy): isolate obsolete TypeScript runtime
refactor(cloud): extract session store domain
refactor(ui): split dashboard design primitives
```

Never combine these with unrelated feature/security fixes.

This matters especially on the current hardening branch where security/process changes may already be in progress.

---

## 11. Definition of done

Repository cleanup is successful when:

- a new engineer can identify production source in under one minute;
- an agent does not choose legacy `src/` for new implementation by default;
- root contains no feature/master-plan sprawl;
- every major domain has an explicit owner/location;
- new docs have one predictable taxonomy;
- package extraction reduces coupling rather than just rearranging paths;
- public routes, MCP contracts, CLI commands and release artifacts remain compatible;
- `go test ./...` and release quality gates pass after structural migrations.

---

## 12. Immediate recommendation

Do **not** begin by moving hundreds of Go files.

Highest-value order is:

```text
AGENTS guardrails
  -> root/docs cleanup
  -> legacy TypeScript decision
  -> UI structure cleanup while redesigning
  -> cloud/cloudserver domain extraction
  -> automation/MCP/project package decomposition
```

That order improves discoverability quickly while keeping the highest-risk runtime code stable.
