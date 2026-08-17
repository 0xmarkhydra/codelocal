# CodeLocal Multi-Repo + Multi-Task Local Execution Plan

Status: Proposed implementation plan
Date: 2026-08-17
Target branch: `dev`
Scope: Multi-repository project support, concurrent task isolation, repo-aware Git/context/verification, and local Git worktree execution only.

## 0. Explicit scope decision

This phase does **not** implement OpenSandbox.

OpenSandbox remains a future execution provider behind a private execution abstraction, but the current implementation target is:

```text
Logical Project
  -> Workspace
  -> Task
  -> Task Execution Bundle
  -> local_worktree provider
  -> Repository-aware Git / Context / Verify
```

Do not add OpenSandbox SDKs, remote sandbox APIs, State Pack transport, Kubernetes execution, sandbox pools, or new public MCP tools in this phase.

The architecture must leave a provider boundary so a future `opensandbox` provider can be added without changing task identity, Project Brain identity, Git routing semantics, review states, or the public MCP surface.

---

# 1. Product goal

A user should be able to run CodeLocal once at a logical project root such as BIDDI even when that directory contains many independent Git repositories.

Example:

```text
BIDDI/
├── web/                 repo-web
├── app/                 repo-app
├── admin/               repo-admin
└── backend/
    ├── auth/            repo-auth
    ├── payment/         repo-payment
    └── user/            repo-user
```

CodeLocal must support:

- one logical Project Brain across all repositories;
- repository ownership for every relevant code artifact;
- Git operations routed to the correct repository;
- multiple tasks running concurrently without sharing mutable working copies;
- one task touching multiple repositories;
- task-aware context that selects only relevant repositories;
- per-repository verification and change reporting;
- compatibility with existing single-repository projects.

User-facing requirement:

```text
cd BIDDI
codelocal .
```

That must be enough. The user must not need to start one CodeLocal runtime per nested repository.

---

# 2. Core domain model

Keep these concepts distinct:

```text
Project
  logical cross-device product/codebase identity

Workspace
  one authorized local checkout of that project

Repository
  one Git repository inside the workspace

Task
  one unit of work against the logical project

TaskExecutionBundle
  isolated mutable execution state for one task

RepositoryExecutionBinding
  one repository-specific mutable binding owned by a task
```

Relationship:

```text
Project
  └── Workspace
       ├── Repository A
       ├── Repository B
       └── Repository C

Task T1
  └── ExecutionBundle T1
       ├── RepositoryBinding A -> worktree T1/A
       └── RepositoryBinding B -> worktree T1/B

Task T2
  └── ExecutionBundle T2
       └── RepositoryBinding A -> worktree T2/A
```

Task T1 and T2 may target the same repository, but they must not share a writable worktree by default.

---

# 3. Repository Registry

## Goal

Create one authoritative repository registry for the workspace.

Reuse the existing `internal/projectidentity` repository discovery and identity logic rather than creating a second discovery system.

Suggested private model:

```go
type RepositoryCheckout struct {
    RepositoryID   string
    RelativePath   string
    AbsoluteRoot   string
    Remote         string
    Lineage        string
    IdentitySource string
}
```

Required operations:

```go
Repositories() []RepositoryCheckout
RepositoryByID(id string) (RepositoryCheckout, bool)
RepositoryForPath(path string) (RepositoryCheckout, bool)
RepositoriesForPaths(paths []string) []RepositoryCheckout
```

`RepositoryForPath` must choose the deepest matching repository root.

## Discovery requirements

Support:

- normal `.git/` directory;
- `.git` file used by worktrees/submodules;
- nested repositories;
- repository without remote;
- repository with no commit yet;
- same repository identity appearing on another device/path;
- ignored/generated dependency directories not becoming repositories accidentally.

`.git` itself is discoverable as a repository boundary, but `.git/**` is never project context/index content.

---

# 4. Unified path ownership and policy

Every path entering index/search/context/Git/verification must first resolve:

```text
PathIdentity
- projectId
- workspaceId
- repositoryId?
- repositoryRoot?
- workspaceRelativePath
- repositoryRelativePath?
- classification
```

Recommended classifications:

```text
source
portable_codelocal
generated
dependency
runtime_private
sensitive
ephemeral_artifact
git_metadata
```

Required invariants:

```text
.git marker                 discoverable
.git/**                     never context/index
node_modules/**             never context/index
.next/**                    never context/index
dist/build/coverage/etc     never context/index
.codelocal/rules/**         portable/searchable
.codelocal/skills/**        portable/searchable
.codelocal/tmp/**           runtime-private
.codelocal/worktrees/**     runtime-private and hard excluded
secret-class paths          blocked from normal context/export
```

`includeIgnored=true` must not bypass hard runtime/sensitive/generated/dependency policy.

---

# 5. Repo-aware Git Router

Current workspace-root Git assumptions must be replaced by repository-aware routing.

## Path-scoped operations

For actions containing a path:

```text
diff
blame
file_history
stage
unstage
```

resolve the owning repository automatically.

Example:

```text
backend/auth/login.go
   -> repo-auth
   -> run git inside BIDDI/backend/auth
   -> pass login.go as repository-relative path
```

## Workspace-wide read operations

For multi-repo workspaces:

```text
git status
```

must aggregate per repository instead of failing at the logical project root.

Example result:

```text
repositories: 12
dirtyRepositories: 3

web
  M src/login.ts

auth
  M internal/login.go

payment
  ?? migration.sql
```

`git diff` without a path should aggregate dirty repositories with bounded output.

`git log` without a repository/path should either return a structured per-repository result or require/derive a repository when ambiguity materially affects the result.

## Mutating operations

Stage/unstage must group paths by repository.

Commit is always per repository. There is no fake atomic Git commit across multiple repositories.

For a multi-repo task:

```text
Task commit
  -> commit repo-web
  -> commit repo-auth
  -> return structured commit results
```

Partial failure must be surfaced explicitly.

Push is also per repository and approval identity must include repository + resolved remote so approval for one repository cannot silently authorize another.

Single-repo workspaces keep existing behavior.

---

# 6. Task model

Task should persist against logical project identity, not an absolute local path.

Suggested task fields:

```text
Task
- id
- projectId
- workspaceId?              current local execution binding only
- repositoryIds[]           known/inferred affected repositories
- title
- description
- status
- priority
- ownerType
- ownerId?
- activeSessionId?
- executionEnvironmentId?
- executionProvider         local_worktree for this phase
- branchName?
- reviewState
- verificationState
- verificationSummary?
- createdAt
- updatedAt
```

Local execution paths must never become cross-device task identity.

---

# 7. Task Execution Bundle

Implement a provider-neutral private contract now, but only ship `local_worktree`.

```text
TaskExecutionBundle
- id
- taskId
- projectId
- workspaceId
- provider = local_worktree
- state
- repositoryBindings[]
- createdAt
- updatedAt
```

```text
RepositoryExecutionBinding
- repositoryId
- sourceRoot
- sourceRevision
- branchName
- worktreePath             local-only
- state
- createdAt
- lastUsedAt
```

States:

```text
creating
ready
running
verifying
review
blocked
completed
failed
cleaning
cleaned
```

Do not expose `worktreePath` as durable cloud/project identity.

---

# 8. Local worktree provider

Directory layout:

```text
.codelocal/worktrees/
  <task-id>/
    <repository-id>/
```

Branch naming:

```text
codelocal/task/<short-task-id>-<slug>
```

Lifecycle:

```text
Task claimed
  -> infer repositories
  -> mutation needed?
      no  -> operate read-only on authorized workspace
      yes -> create/reuse TaskExecutionBundle
  -> create one worktree per affected repository
  -> run edits/commands in task bindings
  -> verify per repository
  -> Review
  -> governed commit/push
  -> safe cleanup after merge/abandon policy
```

Requirements:

- authoritative/main checkout remains clean by default;
- two concurrent mutating tasks never share writable worktrees;
- task resume reuses compatible task bindings;
- recursive `.codelocal/worktrees/**` stays hard excluded from project index/search/context;
- never delete an unmerged branch or dirty task worktree automatically without a safe policy/confirmation;
- cleanup is idempotent.

---

# 9. Multi-task concurrency model

Task ownership and execution ownership are separate concerns.

Required guarantees:

```text
Task A owner != Task B owner allowed
Task A and Task B can execute concurrently
Task A and Task B may touch the same repo
Task A writable path != Task B writable path
```

Example:

```text
repo-web main checkout

Task A
  -> .codelocal/worktrees/T-A/repo-web

Task B
  -> .codelocal/worktrees/T-B/repo-web
```

Add task leases so two sessions cannot accidentally drive the same task concurrently.

Lease properties:

```text
taskId
owner/session
workspace/device
issuedAt
expiresAt
heartbeat/version
```

Stale leases must be recoverable deterministically.

---

# 10. Repo-aware context routing

Do not build AI context from all repositories in the logical project.

Pipeline:

```text
Task objective
  -> semantic/literal evidence
  -> repository candidate scoring
  -> select primary repositories
  -> graph expansion to related repositories only
  -> per-repository retrieval
  -> global bounded context packet
```

Example:

```text
Task: fix web login

repo-web          high
repo-auth         high
repo-user         medium/only if graph requires
repo-payment      excluded
repo-admin        excluded
```

Every ranked file/snippet/symbol must carry `repositoryId`.

Context budget should be allocated based on relevance, not equally across all repositories.

Keep full Project Map for diagnostics, but model-facing `context_for_task` should contain compact repository summaries plus only relevant evidence.

---

# 11. Project Map and Knowledge Graph

Project Map becomes repository-aware:

```text
Project
  -> Repository
      -> Module
          -> File
              -> Symbol
```

Every graph node that represents code must preserve repository ownership.

Cross-repository edges are explicit:

```text
imports
calls
http_route
grpc
event
schema
config
database
build_dependency
unknown_relation
```

The graph may connect repositories, but repository identities must never be flattened away.

---

# 12. Project Brain behavior

Keep one logical Project Brain for the whole project.

Do not create unrelated brains per nested repository.

Knowledge provenance:

```text
projectId
repositoryId?
repositoryIds[]?
scopePath
source/revision metadata
```

Examples:

Repository-scoped:

```text
repo-auth: authentication token validation lives in internal/token
```

Cross-repository:

```text
repo-web login flow depends on repo-auth POST /login contract
```

Task execution state/worktree paths are not long-term Project Brain knowledge.

Only verified durable outcomes should be promoted.

---

# 13. Repo-aware verification

Verification must run from the correct repository/task binding.

Example task touching web + auth:

```text
web task worktree
  npm typecheck
  npm test -- login

auth task worktree
  go test ./...
```

Return structured verification:

```text
Task T1

repo-web
  typecheck PASS
  tests PASS

repo-auth
  tests PASS

Overall: PASS
```

Do not run the whole logical workspace blindly when narrower repository-aware checks are available.

Verification evidence must include repository + revision/worktree generation so stale test results cannot authorize newer edits.

---

# 14. Change tracking

Each task keeps repository-aware touched files:

```text
TaskChanges
  repo-web
    src/login.ts
    src/api/auth.ts

  repo-auth
    internal/login.go
```

Use this for:

- verification planning;
- review UI;
- commit expected-path checks;
- context refresh;
- cleanup safety;
- learned experience provenance.

---

# 15. Cross-device behavior

A Project is portable; a Workspace and its worktree paths are local.

When a task resumes on another machine:

```text
1. resolve same logical project
2. inspect available repository coverage
3. fetch logical task state
4. do not reuse old absolute paths
5. create new local execution bundle/bindings if mutation continues
6. preserve task/repository provenance
```

If another device still holds an active lease, show that before takeover.

Repository identity should prefer normalized remote/lineage identity, not absolute path.

---

# 16. Dashboard model

One task card, even when multiple repositories are involved.

Task detail:

```text
Fix login
Status: In Progress
Execution: Local isolated
Owner: Agent

Repositories
  web       modified 2 files
  auth      modified 1 file

Verification
  web       PASS
  auth      RUNNING
```

Do not expose `.codelocal/worktrees/...` paths in normal UI.

Advanced diagnostics may show repository IDs, branches, bindings, and local paths.

---

# 17. Implementation phases

## Phase MR0 — Contracts and invariants

Implement/freeze:

- RepositoryRegistry contract;
- PathIdentity/repository ownership;
- TaskExecutionBundle model;
- RepositoryExecutionBinding model;
- local-only privacy rules;
- compatibility rules for single-repo projects.

Exit:

- one source of truth for repository discovery/ownership;
- no task identity depends on worktree path;
- no public MCP tool growth required.

## Phase MR1 — Repo-aware Git routing

Implement:

- repository resolution for paths;
- aggregate multi-repo status;
- bounded multi-repo diff;
- repo-aware log/blame/history;
- grouped stage/unstage;
- per-repo commit/push;
- repository-scoped approval identity.

Exit:

- Git no longer fails simply because workspace root is not a Git repository;
- BIDDI-style nested repos work from one CodeLocal workspace.

## Phase MR2 — Local task execution bundles

Implement:

- task execution bundle persistence;
- local worktree provider;
- one worktree per task/repository;
- task lease/ownership;
- resume/reuse;
- cleanup safety.

Exit:

- two mutating tasks can safely target the same repository concurrently without sharing writable files;
- main checkout remains clean by default.

## Phase MR3 — Repo-aware project intelligence

Implement:

- repositoryId on map artifacts;
- repository-aware modules/entrypoints/symbols;
- graph repository ownership;
- cross-repository edges;
- repository-aware cache keys/invalidation.

Exit:

- project intelligence can answer which repository owns every surfaced code artifact;
- changing one repository does not require unnecessary full-project invalidation.

## Phase MR4 — Task context router

Implement:

- candidate repository scoring;
- relevant repository selection;
- per-repository context budgets;
- graph-based cross-repo expansion;
- compact model-facing repository summary;
- context token regression tests.

Exit:

- task in one/two repos does not drag unrelated repos into context;
- BIDDI context remains bounded regardless of total repo count.

## Phase MR5 — Repo-aware verification and review

Implement:

- per-binding verification root;
- per-repository verification evidence;
- task touched-file grouping;
- overall task quality gate;
- review UI grouped by repository.

Exit:

- verification is fresh and repository-correct;
- user can review one logical task across multiple repos.

## Phase MR6 — Project Brain and cross-device integration

Implement:

- verified repository provenance in durable task outcomes;
- cross-repo facts/relationships;
- cross-device task resume semantics;
- repository coverage validation;
- stale lease/takeover behavior.

Exit:

- logical Project Brain remains coherent across devices/checkouts;
- local execution details never leak into durable project identity.

## Phase MR7 — Hardening and benchmarks

Run:

- large multi-repo fixture;
- BIDDI-style 10+ repo fixture;
- same repo targeted by 2 concurrent tasks;
- one task touching 3 repos;
- repo without remote;
- repo without commit;
- `.git` file/worktree cases;
- dependency/generated exclusion cases;
- crash/restart/resume;
- stale task lease;
- partial multi-repo commit failure;
- cleanup safety.

Measure:

```text
repo discovery latency
context bytes/tokens per task
index invalidation scope
worktree creation latency
verification latency
multi-task concurrency correctness
Git routing correctness
```

Exit:

- no known cross-task writable collision;
- no unrelated repository context leakage in benchmark scenarios;
- no generated/dependency/runtime worktree leakage;
- single-repo regressions pass.

---

# 18. Suggested commit sequence

Keep changes reviewable:

```text
1. refactor: centralize repository registry and path ownership
2. fix: route git operations across nested repositories
3. feat: add task execution bundle domain model
4. feat: add local worktree execution provider
5. feat: isolate concurrent task mutations per repository
6. feat: make project intelligence repository-aware
7. feat: route task context by repository relevance
8. feat: verify and review changes per repository binding
9. feat: preserve repository provenance in project brain
10. test: add multi-repo multi-task regression matrix
11. perf: add repository-scoped invalidation and benchmarks
```

Do not combine all phases into one giant commit.

---

# 19. Explicit non-goals for this implementation

Do not implement now:

```text
OpenSandbox provider
Workspace State Engine / State Pack transport
remote execution
Kubernetes sandbox
sandbox pool
sandbox network broker
sandbox secret broker
sandbox browser
remote source upload
provider-specific OpenSandbox task fields
new sandbox MCP namespace/tools
```

The current provider abstraction should make those additions possible later, but they are not required for MR0-MR7 completion.

---

# 20. Definition of Done

The local multi-repo/multi-task milestone is complete when this works:

```text
User starts CodeLocal once in BIDDI
  -> CodeLocal discovers all nested repositories

Task A: fix login
  -> selects web + auth
  -> creates isolated task worktrees for both repos

Task B: refactor web dashboard
  -> also targets web
  -> receives a different isolated web worktree

Task A and Task B run concurrently
  -> no shared writable files
  -> no main-checkout mutation by default

Git
  -> status works across all repos
  -> path operations route automatically
  -> commits/pushes remain repository-scoped

Context
  -> Task A sees web/auth evidence
  -> payment/admin do not enter context unless graph relevance requires them

Verification
  -> runs from each task/repository binding
  -> produces fresh per-repository evidence

Project Brain
  -> records only verified durable conclusions with repository provenance
```

Required guarantees:

```text
[PASS] one logical project may contain many Git repositories
[PASS] one CodeLocal workspace manages the repository set
[PASS] one task may touch many repositories
[PASS] many tasks may run in one workspace concurrently
[PASS] mutating tasks never share writable task worktrees by default
[PASS] Git routes to repository roots rather than assuming workspace root
[PASS] context is repository-aware and bounded
[PASS] generated/dependency/runtime paths remain excluded
[PASS] verification is repository/task-binding aware
[PASS] single-repo behavior remains compatible
[PASS] OpenSandbox is not required for this milestone
```

---

# 21. Future integration seam

After this plan is complete, future safe-execution work may add another provider behind the same private execution contract:

```text
TaskExecutionBundle
  -> Execution Provider
       ├── local_worktree      implemented by this plan
       └── future provider     deferred
```

The future provider must reuse:

- Project/Workspace/Repository identity;
- Task identity and leases;
- repository-aware context;
- repository-aware verification;
- review/change provenance;
- Project Brain rules;
- existing public MCP intent surface.

That future work must not require redesigning the multi-repo/multi-task foundation defined here.
