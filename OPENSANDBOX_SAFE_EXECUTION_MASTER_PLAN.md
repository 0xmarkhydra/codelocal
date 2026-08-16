# CodeLocal Safe Execution + Workspace State Engine — OpenSandbox Master Plan

Status: **Proposed master architecture / implementation blueprint**  
Date: **2026-08-17**  
Owner: **CodeLocal**  
Primary implementation target: **native Go runtime + CodeLocal Cloud + OpenSandbox provider**  
Public MCP surface: **must remain compact; OpenSandbox is an internal execution provider, not a second public tool surface**  
Depends on: [`PROJECT_BRAIN_MASTER_PLAN.md`](./PROJECT_BRAIN_MASTER_PLAN.md), [`UNIVERSAL_AGENT_RUNTIME_PLAN.md`](./UNIVERSAL_AGENT_RUNTIME_PLAN.md)

> This document defines how CodeLocal safely executes untrusted or high-risk agent work without giving the agent direct write/execute authority over the user's authoritative workspace. It also defines the private Workspace State Engine used to move workspace state into isolated execution environments, reconcile results, support concurrent user edits, and provide task-level rollback/checkpoint semantics.

---

# 0. Executive decision

CodeLocal should integrate OpenSandbox **below** the existing CodeLocal execution/tool layer.

The user-facing and model-facing contract remains CodeLocal:

```text
ChatGPT / Claude / Codex / Gemini / future agents
                    |
                    | MCP / CodeLocal agent runtime
                    v
                CodeLocal
                    |
             Execution Router
              /           \
             v             v
      Trusted Local     Safe Execution
        Runtime            Runtime
                              |
                       OpenSandbox Provider
                              |
                      Docker / Kubernetes
```

OpenSandbox must not become a second public MCP surface that the model has to reason about.

The key architectural decision is to add two internal subsystems:

```text
1. Workspace State Engine
   authoritative snapshots/checkpoints/change reconciliation

2. Safe Execution Runtime
   isolated command/file/browser execution through providers such as OpenSandbox
```

The two subsystems integrate through a stable private contract.

---

# 1. Product goal

The user should be able to ask:

```text
"Clone/review this unknown project and see whether it runs."
"Install dependencies and run the tests."
"Let the agent fix this bug."
```

without needing to decide where every action runs.

CodeLocal should determine:

```text
safe read-only operation
    -> local runtime

trusted bounded project operation
    -> local runtime under existing policy

untrusted/high-risk execution
    -> safe execution environment

critical machine/account/production action
    -> explicit approval / local governed path
```

For isolated tasks, the target flow is:

```text
User Workspace
     |
     v
Workspace State Engine
     |
     v
Checkpoint C100
     |
     v
State Pack
     |
     v
OpenSandbox
     |
     +-- install
     +-- build
     +-- test
     +-- edit
     +-- browser automation when applicable
     |
     v
Result ChangeSet
     |
     v
CodeLocal Validation
     |
     v
State Merge / Apply
     |
     v
Authoritative User Workspace
```

The user's normal repository state, staged changes, branches, temporary edits, and local workflow must not be mutated merely to synchronize with the sandbox.

---

# 2. Core invariants

These invariants are non-negotiable.

## 2.1 CodeLocal owns the public contract

The model calls CodeLocal tools and agents, not OpenSandbox tools directly.

OpenSandbox is an implementation provider behind CodeLocal.

## 2.2 The authoritative workspace is never the sandbox working directory

Sandbox mutation occurs on an isolated reconstructed state.

The sandbox must not receive writable direct access to the authoritative user workspace in safe mode.

## 2.3 Synchronization does not alter the user's version-control workflow

Creating a checkpoint must not:

- stage user files;
- create user-visible commits;
- change branch;
- stash changes;
- reset files;
- rewrite history;
- alter the user's index;
- require the user to push unfinished code.

## 2.4 Domain language is CodeLocal-owned

Product/domain terminology:

```text
Checkpoint
Workspace State
State Object
State ID
State Pack
ChangeSet
State Merge
Session
Restore
```

Do not expose implementation-specific storage terminology in UI, MCP contracts, dashboard, audit summaries, or cross-subsystem interfaces.

The storage implementation may use a content-addressed backend internally, but that backend is replaceable and must not leak into domain contracts.

## 2.5 Secrets are excluded by default

The Workspace State Engine must not capture or export secrets merely because they exist under the workspace root.

## 2.6 Sandbox output is untrusted until verified

A successful sandbox process exit is not equivalent to a safe workspace change.

Every mutation result passes through:

```text
ChangeSet inspection
scope validation
security validation
conflict detection
project-aware verification
```

before becoming authoritative.

## 2.7 Local user edits win over stale agent assumptions

If the user edits the workspace while an isolated task is running, CodeLocal must not overwrite those edits silently.

Use common-base reconciliation and explicit conflicts.

## 2.8 No increase to public MCP tool count is required for the first rollout

Existing CodeLocal operations remain stable.

The execution router chooses the backend internally.

---

# 3. Target architecture

```text
                         AI CLIENT / USER
                               |
                               v
                    CodeLocal MCP / Agent Layer
                               |
                               v
                      Project Brain Context
                               |
                               v
                       Execution Router
                               |
                 +-------------+-------------+
                 |                           |
                 v                           v
         Trusted Local Path           Safe Execution Path
                 |                           |
                 |                    Workspace State Engine
                 |                           |
                 |                      Checkpoint Cn
                 |                           |
                 |                       State Pack
                 |                           |
                 |                           v
                 |                   Safe Execution Runtime
                 |                           |
                 |                   OpenSandbox Provider
                 |                           |
                 |                 +---------+---------+
                 |                 |                   |
                 |                 v                   v
                 |             Docker              Kubernetes
                 |                 |                   |
                 |                 +---------+---------+
                 |                           |
                 |                           v
                 |                     Isolated Session
                 |                           |
                 |                      execd / files
                 |                      commands / PTY
                 |                      browser optional
                 |                           |
                 |                           v
                 |                    Result ChangeSet
                 |                           |
                 +-------------+-------------+
                               |
                               v
                    Validation + State Merge
                               |
                               v
                    Authoritative Workspace
                               |
                               v
                         Verification
                               |
                               v
                          Experience
                               |
                               v
                         Project Brain
```

---

# 4. Subsystem boundaries

## 4.1 Project Brain

Owns:

- project identity;
- rules;
- durable knowledge;
- learned skills;
- task context;
- prior verified experience.

Does **not** own raw workspace snapshots.

## 4.2 Universal Agent Runtime

Owns:

- provider/agent selection;
- agent sessions;
- normalized agent events;
- agent lifecycle;
- cross-provider orchestration.

Does **not** own authoritative workspace-state persistence.

## 4.3 Workspace State Engine

Owns:

- state capture;
- checkpoint identity;
- content-addressed state objects;
- state packs;
- ChangeSets;
- conflict detection;
- state merge;
- restore/rollback semantics;
- task/session state ancestry.

## 4.4 Safe Execution Runtime

Owns:

- isolated execution provider abstraction;
- session create/destroy/pause/resume;
- file/command execution;
- resource limits;
- network policy;
- secret bindings;
- sandbox health;
- sandbox audit metadata.

## 4.5 OpenSandbox Provider

Owns provider-specific translation between CodeLocal Safe Execution Runtime contracts and OpenSandbox SDK/API behavior.

OpenSandbox-specific types must stop at the provider boundary.

---

# 5. Workspace State Engine terminology

## Workspace State

A complete logical representation of the relevant workspace filesystem at one point in time.

## Checkpoint

An immutable named reference to a Workspace State.

Example:

```text
C100 = workspace state when task started
C101 = state after user edited two files
A120 = isolated agent result state
```

## State Object

Content-addressed immutable data representing file content or structural metadata.

## State ID

Stable digest identifying a Workspace State.

## State Pack

Portable package containing enough state objects and metadata to reconstruct a target checkpoint in another execution environment.

## ChangeSet

Structured difference between two checkpoints/states.

## State Merge

Three-way reconciliation of:

```text
BASE
CURRENT
INCOMING
```

## Session

A bounded execution lineage beginning at one checkpoint and producing zero or more derived states/results.

---

# 6. Proposed package structure

Native Go target:

```text
internal/workspacestate/
    engine.go
    store.go
    checkpoint.go
    manifest.go
    object.go
    pack.go
    changeset.go
    merge.go
    restore.go
    policy.go
    scanner.go
    watcher.go
    lock.go
    gc.go
    metrics.go

internal/workspacestate/store/
    objectstore/
    memory/

internal/safeexec/
    runtime.go
    provider.go
    router.go
    session.go
    policy.go
    resources.go
    network.go
    secrets.go
    events.go
    audit.go
    health.go

internal/safeexec/providers/
    opensandbox/
        provider.go
        lifecycle.go
        execution.go
        files.go
        network.go
        secrets.go
        health.go
        mapping.go
```

Existing packages to integrate with:

```text
internal/security/
internal/approval/
internal/orchestration/
internal/taskstate/
internal/projectidentity/
internal/project/
internal/localclient/
internal/runtime/
internal/agentruntime/
internal/memory/
internal/learnedskills/
```

Avoid duplicating policy, task state, verification, project identity, or Project Brain logic.

---

# 7. Workspace State Store contract

Conceptual interface:

```go
type Store interface {
    Capture(ctx context.Context, req CaptureRequest) (Checkpoint, error)
    Resolve(ctx context.Context, id StateID) (WorkspaceState, error)
    Diff(ctx context.Context, from, to StateID) (ChangeSet, error)
    Export(ctx context.Context, req ExportRequest) (StatePack, error)
    Import(ctx context.Context, pack StatePack) (Checkpoint, error)
    Merge(ctx context.Context, req MergeRequest) (MergeResult, error)
    Restore(ctx context.Context, req RestoreRequest) (RestoreResult, error)
    Release(ctx context.Context, id StateID) error
}
```

The rest of CodeLocal must depend on this interface, not on the backing store.

---

# 8. Checkpoint model

Suggested model:

```text
Checkpoint
  id
  project_id
  workspace_id
  state_id
  parent_state_id nullable
  kind baseline|user|agent|verification|restore
  task_id nullable
  session_id nullable
  created_at
  source_generation
  manifest_digest
  policy_digest
  metadata safe-json
```

Checkpoint creation should be cheap after the first capture.

A new checkpoint stores only changed state objects plus a new immutable tree/manifest root.

---

# 9. Workspace manifest

A checkpoint manifest should include enough information for deterministic reconstruction and validation.

Conceptual shape:

```text
WorkspaceManifest
  schema_version
  project_id
  workspace_id
  root_state_id
  entries[]
  ignore_policy_digest
  security_policy_digest
  created_at
```

Entry:

```text
path
kind file|dir|symlink
content_id nullable
mode
size
executable
link_target nullable
```

Do not preserve unsafe host-specific filesystem metadata that is unnecessary for reconstruction.

---

# 10. Capture policy

Capture is not a blind recursive copy.

Order of filtering:

```text
1. CodeLocal hard security exclusions
2. explicit organization/project security policy
3. .codelocalignore
4. project ignore conventions where appropriate
5. generated/cache defaults
6. task-specific inclusion scope
```

Hard exclusions should include classes such as:

```text
private keys
SSH material
wallet material
browser profile/cookies
credential stores
environment secret files by default
OS keychains
runtime approval tokens
CodeLocal private local state
```

A project may explicitly request additional files, but secret-class items require a stronger dedicated flow rather than normal snapshot inclusion.

---

# 11. Initial capture strategy

The first capture for a workspace may require a bounded scan.

Optimize by:

- reusing the existing workspace index;
- reusing watcher state;
- skipping known generated directories;
- hashing only eligible files;
- using streaming reads;
- imposing file/total-size limits;
- recording manifest state for future incremental captures.

After initial capture:

```text
filesystem watcher
      |
      v
changed path set
      |
      v
next Capture()
      |
      v
hash only changed eligible paths
      |
      v
reuse unchanged state objects
```

---

# 12. State storage location

Do not place a second visible working repository inside the user's project.

Recommended runtime state location:

```text
~/.codelocal/
  projects/
    <logical-project-id>/
      state/
        objects/
        manifests/
        checkpoints/
        packs/
        locks/
```

Project-local `.codelocal/` remains for portable project configuration/identity where appropriate.

Large runtime state is local/private and ignored from the user's project source tree.

---

# 13. Internal storage backend

The initial store should be content-addressed and immutable.

Required behavior:

- deduplicate unchanged file contents;
- deduplicate identical content across checkpoints;
- stream large objects;
- verify object digests on import;
- allow garbage collection by reachability/retention;
- support fast diff/merge primitives;
- remain replaceable behind `workspacestate.Store`.

Implementation detail must not leak into:

- UI labels;
- public MCP schemas;
- Safe Execution contracts;
- Project Brain knowledge;
- Agent Runtime contracts.

---

# 14. State Pack protocol

A State Pack is the transport between Workspace State Engine and isolated execution.

Conceptual envelope:

```text
StatePack
  schema_version
  project_id
  checkpoint_id
  state_id
  base_state_id nullable
  manifest_digest
  object_index[]
  compressed_payload
  signature/mac optional
```

Two modes:

## Full Pack

Used when the destination has no compatible base state.

## Delta Pack

Used when destination already holds a known base checkpoint/state.

```text
Destination has C100
Need C105

Send only:
C100 -> C105 delta objects
```

This becomes important for long-lived sandbox sessions and remote execution.

---

# 15. Safe Execution Provider contract

Conceptual interface:

```go
type Provider interface {
    ID() string
    Probe(ctx context.Context) ProbeResult

    Create(ctx context.Context, req CreateRequest) (Session, error)
    Destroy(ctx context.Context, session Session) error
    Pause(ctx context.Context, session Session) error
    Resume(ctx context.Context, session Session) error

    ImportState(ctx context.Context, session Session, pack StatePack) error
    ExportState(ctx context.Context, session Session) (StatePack, error)

    Exec(ctx context.Context, session Session, req ExecRequest) (ExecResult, error)
    Files() FileOperations
    Network() NetworkOperations
    Secrets() SecretOperations
}
```

Provider-specific identifiers remain opaque metadata.

---

# 16. OpenSandbox provider role

The first production Safe Execution provider should map CodeLocal concepts to OpenSandbox:

```text
CodeLocal Create Session
        -> OpenSandbox lifecycle create

CodeLocal Exec
        -> OpenSandbox execution/execd command API

CodeLocal file operations
        -> OpenSandbox filesystem API

CodeLocal Network Policy
        -> OpenSandbox egress policy

CodeLocal Secret Binding
        -> OpenSandbox credential/egress secret mechanism where supported

CodeLocal Pause/Resume
        -> OpenSandbox lifecycle operations when backend supports them

CodeLocal Destroy
        -> OpenSandbox terminate/delete
```

The provider must normalize capability differences between local Docker-backed and remote Kubernetes-backed OpenSandbox deployments.

---

# 17. Session model

Suggested model:

```text
SafeExecutionSession
  id
  provider_id
  provider_session_id
  user_id
  project_id
  workspace_id
  task_id
  base_checkpoint_id
  base_state_id
  current_state_id nullable
  status
  runtime_class
  resource_policy
  network_policy_digest
  secret_binding_ids[]
  created_at
  last_activity_at
  expires_at
```

Statuses:

```text
creating
syncing
ready
running
waiting
paused
exporting
verifying
completed
failed
expired
destroying
destroyed
```

---

# 18. Execution Router

Add one centralized execution-routing decision point.

Do not let every tool independently choose its own backend.

Input signals:

```text
action kind
command/file/browser intent
source trust
workspace scope
network need
secret need
system mutation risk
production/account impact
provider capability
user/org policy
```

Output:

```text
LOCAL
SAFE_EXECUTION
APPROVAL_REQUIRED
DENY
```

Example initial deterministic policy:

```text
read known project file                 -> LOCAL
search/index/diagnostics                -> LOCAL
project status/diff inspection          -> LOCAL
run known deterministic test command    -> policy-dependent LOCAL or SAFE_EXECUTION
install package dependencies            -> SAFE_EXECUTION by default
execute downloaded/unknown binary       -> SAFE_EXECUTION
clone and execute unknown project       -> SAFE_EXECUTION
pipe remote content into shell          -> SAFE_EXECUTION or DENY by policy
browser research on unknown site        -> sandbox browser when available
production deploy                        -> APPROVAL_REQUIRED governed local/provider path
OS/security/account configuration        -> APPROVAL_REQUIRED or DENY
```

Security classification must be deterministic first.

Do not add an LLM risk classifier to the critical path until rules are proven insufficient.

---

# 19. Risk model

Initial levels:

```text
R0 SAFE_READ
R1 TRUSTED_BOUNDED
R2 UNTRUSTED_EXECUTION
R3 HIGH_RISK
R4 CRITICAL_EXTERNAL_IMPACT
```

Routing concept:

```text
R0 -> local
R1 -> local under policy or isolated based on user mode
R2 -> isolated by default
R3 -> isolated + approval depending on requested capability
R4 -> explicit approval; isolation does not replace external-impact authorization
```

A sandbox cannot make a production deployment or account action harmless merely by isolating the process.

---

# 20. Workspace reconstruction inside sandbox

Safe mode:

```text
Checkpoint C100
     |
     v
State Pack
     |
     v
OpenSandbox session
     |
     v
/workspace
```

Requirements:

- reconstruct exact eligible source state;
- maintain deterministic path mapping;
- record reconstruction state ID;
- reject corrupted pack/object digests;
- never rely on model prompt text to transfer source code;
- source transfer occurs as filesystem/state transport, not LLM context.

This avoids token usage proportional to repository size.

---

# 21. Dependency strategy

Do not capture dependency caches as normal workspace state unless a specialized cache provider is added later.

Examples normally excluded:

```text
node_modules
build outputs
.next
dist
coverage
language build caches
large package caches
```

The isolated environment reconstructs dependencies from manifests/lockfiles.

Later optimization may provide separately keyed immutable dependency caches:

```text
hash(runtime + lockfile + platform + toolchain)
       -> dependency cache
```

Dependency cache identity must remain separate from Workspace State identity.

---

# 22. Network policy

Safe Execution defaults to least privilege.

Recommended modes:

```text
OFFLINE
ALLOW_REQUIRED
OPEN_WITH_AUDIT
```

Default for unknown code should be `ALLOW_REQUIRED` or stricter.

Example Node build policy may allow:

```text
package registry
authorized source host
required artifact host
```

Unexpected domain request:

```text
sandbox
  -> network policy
      -> blocked + audited
```

Escalation to a new domain should require policy evaluation and, where necessary, user approval.

---

# 23. Secret Broker

Do not export normal local secret files into a State Pack.

Instead introduce an explicit broker contract:

```text
Task needs external credential
        |
        v
CodeLocal Secret Broker
        |
        v
temporary scoped binding
        |
        v
Safe Execution Provider
        |
        v
provider-side request injection / protected binding
```

Properties:

- scoped to destination/service;
- scoped to one session/task;
- expires automatically;
- not included in checkpoints;
- not included in State Packs;
- not logged in raw command output;
- revoke on session destroy;
- user/org policy remains authoritative.

Prefer request-time credential injection mechanisms where the provider supports them rather than exposing raw secret values to the agent process.

---

# 24. Browser isolation

Eventually support two explicit browser classes internally:

```text
PERSONAL_BROWSER
  authenticated user browser/session

ISOLATED_BROWSER
  disposable browser in Safe Execution session
```

Unknown-site research, downloads, scraping, and browser testing should prefer `ISOLATED_BROWSER` when capabilities fit.

Personal-account actions remain governed by CodeLocal computer/browser security and approval policy.

Do not copy the user's normal browser profile/cookies into an isolated session by default.

---

# 25. Result extraction

When agent execution completes:

```text
Sandbox Working State
       |
       v
capture isolated result state
       |
       v
compare to base state
       |
       v
Result ChangeSet
```

A ChangeSet should include:

```text
base_state_id
result_state_id
added_paths[]
modified_paths[]
deleted_paths[]
renamed_paths[] when confidently detected
mode_changes[]
summary metrics
content object references
```

The model does not need to reconstruct this from logs.

---

# 26. Validation before apply

Before applying a Result ChangeSet to the authoritative workspace:

```text
1. verify base state lineage
2. refresh current user workspace checkpoint
3. detect concurrent user changes
4. reject out-of-scope path mutations
5. scan newly introduced sensitive/binary/executable artifacts
6. enforce project/security rules
7. perform State Merge
8. materialize candidate changes
9. run project-aware verification
10. finalize or restore
```

A task that only produces logs/read findings may skip the apply phase entirely.

---

# 27. Concurrent user edits and State Merge

Example:

```text
Task begins:
C100

User keeps editing:
C100 -> C105

Agent works in isolation:
C100 -> A120
```

Reconciliation:

```text
              C105  CURRENT
             /
C100  BASE --
             \
              A120  INCOMING
```

Perform three-way State Merge.

Possible outcomes:

```text
CLEAN
  all incoming changes merge deterministically

CONFLICTED
  one or more paths/ranges require resolution

REJECTED
  incoming state violates policy/scope/integrity
```

Never overwrite `C105` just because the agent completed later.

---

# 28. Transaction semantics

Treat each mutating agent task like a workspace transaction.

```text
BEGIN
  capture base checkpoint C100
  create isolated session
  run agent actions
  capture incoming state A120
  validate
  merge
  verify

COMMIT
  authoritative checkpoint C121
```

On failure:

```text
ROLLBACK / ABORT
  authoritative workspace remains unchanged
```

If candidate changes were materialized locally for verification, retain a restore checkpoint so failure can deterministically return to the prior authoritative state.

---

# 29. Restore semantics

Expose domain-level restore internally and later in UI.

Example task history:

```text
Task T1: C100 -> C105
Task T2: C105 -> C109
Task T3: C109 -> C120
```

CodeLocal can show:

```text
Fix login          C100 -> C105
Refactor API       C105 -> C109
Optimize DB        C109 -> C120
```

Potential actions:

```text
View ChangeSet
Restore Checkpoint
Compare States
```

Restore must itself create a new checkpoint/event rather than silently deleting history.

---

# 30. Multi-agent isolation

Workspace State Engine replaces direct shared-working-tree mutation as the long-term isolation mechanism.

Example:

```text
                   C100
          +---------+---------+
          |         |         |
          v         v         v
       Session A Session B Session C
          |         |         |
          v         v         v
        A110      B115      C108
          \         |         /
           \        |        /
            +-- Compare/Merge
                    |
                    v
                  C120
```

Each agent gets the same base checkpoint without sharing a writable authoritative workspace.

This supports:

- implementer + reviewer;
- frontend/backend/test specialists;
- provider comparisons;
- safe parallel experimentation.

Parallel mutation must remain opt-in until merge/verification quality is proven.

---

# 31. Sandbox session reuse

Do not create a fresh sandbox for every command within one task.

Correct model:

```text
Task
  -> one Safe Execution Session
      -> install
      -> build
      -> edit
      -> test
      -> edit
      -> test
      -> export result
```

Reuse preserves dependency installation and intermediate state.

Session TTL extends on meaningful activity.

---

# 32. Pause/resume

Long-running tasks may pause isolated compute without losing logical task state.

CodeLocal contract:

```text
RUNNING -> PAUSED -> RUNNING
```

Provider decides whether pause/resume is native or implemented through provider-supported snapshot/restore behavior.

Workspace State Engine remains the durable logical state boundary even if provider runtime state is lost.

If a sandbox cannot resume safely:

```text
latest checkpoint/state pack
    -> create replacement session
    -> reconstruct
    -> continue
```

---

# 33. Local and cloud execution modes

## Local Safe Execution

```text
CodeLocal runtime
   -> local OpenSandbox service/runtime
   -> local container/secure runtime
```

Advantages:

- source stays on device;
- low transfer latency;
- useful for privacy-sensitive users.

## Remote Safe Execution

```text
CodeLocal runtime
   -> encrypted State Pack transport
   -> CodeLocal/OpenSandbox cloud control plane
   -> Kubernetes-backed sandbox
```

Requirements before default enablement:

- explicit user/org policy;
- strong tenant isolation;
- encrypted transport;
- scoped upload retention;
- source-state deletion policy;
- secret exclusion verified;
- regional/data-residency story where required.

Remote execution is a separate privacy/product decision from local sandbox integration.

---

# 34. Sandbox pool

After correctness is proven, add warm pools for latency.

Classes may include:

```text
node
python
go
browser
review-only
```

A pooled sandbox is not considered trusted merely because it is warm.

Before assignment:

- clean/replace previous state;
- establish fresh session identity;
- apply fresh network/secret policy;
- import target checkpoint;
- verify workspace state ID.

---

# 35. Resource policy

Each session receives bounded resources.

Conceptual policy:

```text
cpu
memory
disk
process count
wall-clock TTL
idle TTL
network bytes optional
max output bytes
max file count/max state bytes
```

Tiering can later map product plans to limits, but security defaults must not weaken on lower tiers.

---

# 36. Audit model

Record safe metadata without leaking source/secrets.

Example events:

```text
state.checkpoint.created
state.pack.exported
state.pack.imported
state.changeset.created
state.merge.completed
state.merge.conflicted
state.restore.completed

safeexec.route.selected
safeexec.session.created
safeexec.session.ready
safeexec.command.started
safeexec.command.completed
safeexec.network.blocked
safeexec.secret.bound
safeexec.session.paused
safeexec.session.resumed
safeexec.session.destroyed
safeexec.result.rejected
```

Metrics:

```text
checkpoint_latency_ms
incremental_capture_bytes
state_pack_bytes
state_pack_ratio
state_object_dedupe_ratio
sandbox_create_latency_ms
sandbox_ready_latency_ms
sandbox_session_reuse_rate
safeexec_command_count
blocked_network_count
merge_conflict_rate
result_rejection_rate
restore_count
isolated_vs_local_execution_rate
```

No raw secret values or unrestricted source snippets in normal telemetry.

---

# 37. UI/UX terminology

User-facing language should remain simple.

Examples:

```text
Execution: Local
Execution: Isolated

Created safe environment
Synced workspace
Running tests
3 changes ready
Review changes
Apply
Discard
Restore checkpoint
```

Do not expose implementation backend vocabulary unless the user opens an advanced diagnostics view.

Advanced diagnostics may show provider/runtime metadata, but domain actions remain CodeLocal terminology.

---

# 38. Compatibility with current MCP surface

Phase 1 should require **zero new public MCP tools**.

Existing operations continue to represent intent:

```text
read
edit
terminal
browser
computer
agent
verify
```

The backend path changes internally:

```text
request
  -> security classification
  -> execution route
  -> local adapter OR safe-exec adapter
```

This avoids:

- tool-schema growth;
- duplicate shell/file tools;
- model confusion;
- reconnect requirements caused solely by OpenSandbox integration.

If future user control requires new configuration, prefer dashboard/config fields before adding new public tool schemas.

---

# 39. Failure cases

The implementation is incomplete until these cases are handled.

## Workspace State

- huge repository;
- symlink escaping workspace;
- file changes during capture;
- watcher missed events;
- file deleted during hash;
- binary file changed;
- case-only rename;
- permission/executable-bit change;
- nested repository/module;
- ignored file explicitly required for build;
- secret accidentally located in normal source path;
- disk fills during object write;
- corrupted object/pack;
- interrupted checkpoint write.

## Synchronization

- sandbox dies during import;
- state pack upload interrupted;
- provider already has stale base state;
- result export interrupted;
- duplicate retry of import/export;
- user changes workspace while agent runs;
- task starts while another apply/restore is in progress.

## Execution

- provider unavailable;
- OpenSandbox server unreachable;
- runtime image unavailable;
- command hangs;
- process forks excessively;
- output floods logs;
- disk/memory/CPU limit exceeded;
- sandbox network unavailable;
- package registry unavailable;
- sandbox expires mid-task.

## Security

- malicious dependency install script;
- unknown binary attempts host escape;
- source attempts to read secret paths;
- source attempts unexpected outbound network;
- agent requests secret beyond task scope;
- result creates executable in sensitive project path;
- state pack tries path traversal;
- symlink points outside reconstructed workspace.

## Merge/apply

- user and agent edit same region;
- user deletes file agent edits;
- agent deletes file user edits;
- incoming path outside allowed scope;
- verification fails after clean merge;
- local process rewrites generated files during verification;
- restore target no longer matches current expected generation.

Each case needs an explicit deterministic response and regression test.

---

# 40. Security threat model

Safe Execution reduces risk but is not itself an authorization system.

Threats:

```text
host filesystem escape
container/runtime escape
secret exfiltration
network exfiltration
resource exhaustion
malicious build hooks
prompt-injected agent command
poisoned dependency
path traversal in State Pack
symlink escape
stale result overwrite
cross-tenant remote sandbox leakage
provider/control-plane compromise
```

Required defenses:

- least-privilege provider/runtime config;
- no writable host workspace mount in safe mode;
- hard secret exclusions;
- State Pack integrity validation;
- path normalization and traversal rejection;
- symlink policy;
- network deny/allow controls;
- resource limits;
- provider/runtime patching/version policy;
- local security/approval remains authoritative;
- tenant/session-scoped remote resources;
- verification before apply;
- audit trails;
- bounded retention/cleanup.

---

# 41. Testing strategy

## Unit tests

Workspace State Engine:

```text
capture unchanged workspace
incremental one-file change
add/delete/rename
binary object
symlink policy
hard exclusions
ChangeSet determinism
three-way clean merge
three-way conflict
restore semantics
object corruption detection
pack import/export integrity
GC reachability
```

Safe Execution Runtime:

```text
route classification
provider capability mapping
session lifecycle
TTL expiry
resource policy
network policy translation
secret binding lifecycle
provider failure normalization
idempotent destroy
```

## Integration tests

```text
capture -> pack -> reconstruct -> exact state ID
OpenSandbox create -> import -> exec -> export -> ChangeSet
unknown package install cannot touch host workspace
blocked network destination is denied
secret-class file not present in sandbox
user edits concurrently -> conflict/clean merge correct
result verification failure -> no authoritative commit
```

## Chaos tests

```text
kill sandbox mid-command
kill runtime during state export
corrupt one state object
network loss during pack upload
expire session during task
restart CodeLocal during pending apply
```

---

# 42. Benchmark baseline

Before rollout, record current direct-execution baseline.

Measure scenarios:

```text
small Node project
large monorepo
many small files
few large files
repeat task same workspace
one-file incremental change
unknown repository build
parallel agent experiment
```

Metrics:

```text
initial capture latency
incremental checkpoint latency
bytes transferred per task
sandbox cold-start latency
sandbox warm-start latency
time to first command
end-to-end verified completion
merge/conflict overhead
CPU/memory overhead on user machine
storage amplification
```

Do not claim isolation has negligible latency until measured.

---

# 43. Rollout roadmap

## WSE0 — Architecture freeze + terminology contract

Tasks:

- freeze Workspace State Engine domain model;
- freeze Checkpoint/State Pack/ChangeSet/State Merge vocabulary;
- define subsystem boundaries;
- define benchmark fixtures;
- document hard exclusions;
- define feature flags.

Exit gate:

- no public/domain interface depends on storage backend terminology;
- architecture reviewed against Project Brain and Universal Agent Runtime.

## WSE1 — State Store core

Implement:

```text
StateID
StateObject
WorkspaceManifest
Checkpoint
local Store interface
atomic object writes
integrity checks
```

Exit gate:

- identical input workspace produces deterministic equivalent state;
- failed writes cannot publish a partial checkpoint.

## WSE2 — Capture Engine

Implement:

- eligible path traversal;
- existing workspace-index reuse;
- ignore/security filters;
- content hashing;
- watcher-assisted incremental changed-path set;
- bounded file/size handling.

Exit gate:

- second unchanged capture is near-noop;
- one changed source file does not re-read the whole workspace.

## WSE3 — ChangeSet + State Merge

Implement:

- deterministic diff;
- add/modify/delete/mode changes;
- common-base model;
- clean three-way merge;
- explicit conflicts;
- scope validation.

Exit gate:

- concurrent user/agent edits never silently overwrite one another.

## WSE4 — State Pack

Implement:

- full pack;
- delta pack;
- compression;
- integrity verification;
- bounded streaming import/export;
- idempotent import identity.

Exit gate:

- a checkpoint can be reconstructed in an empty temp environment with identical logical state ID.

## SER0 — Safe Execution contracts + fake provider

Implement:

```text
safeexec.Provider
session model
resource policy
network policy model
secret binding model
fake deterministic provider
```

Exit gate:

- lifecycle and routing tests work without OpenSandbox dependency.

## SER1 — OpenSandbox local provider PoC

Implement minimum provider operations:

```text
probe
create
destroy
exec
file read/write or state import staging
health/status
```

Target local environment first.

Exit gate:

- command proven to run inside isolated environment rather than host runtime;
- provider failure surfaces cleanly.

## SER2 — WSE -> OpenSandbox synchronization

Implement:

```text
capture checkpoint
export State Pack
create session
import State Pack
reconstruct /workspace
verify imported state ID
```

Exit gate:

- current eligible workspace state appears correctly inside sandbox without modifying user workspace metadata/workflow.

## SER3 — Result ChangeSet round trip

Implement:

```text
run edits in sandbox
capture result state
export result/delta
import to local State Store
derive ChangeSet
review candidate
```

Do not auto-apply yet.

Exit gate:

- CodeLocal can show exact isolated changes against the task base checkpoint.

## SER4 — Validated apply + restore

Implement:

- refresh current user checkpoint;
- conflict detection;
- State Merge;
- transactional materialization;
- restore checkpoint;
- `verify.changes` integration;
- abort/restore on failed verification according to policy.

Exit gate:

- isolated agent can complete one real code-fix task safely end-to-end;
- user concurrent edit is preserved;
- failed result does not corrupt authoritative workspace.

## SER5 — Execution Router v1

Implement deterministic routing rules:

```text
LOCAL
SAFE_EXECUTION
APPROVAL_REQUIRED
DENY
```

Integrate existing security/approval policy.

Exit gate:

- unknown dependency execution routes to safe environment;
- read/search remain local;
- critical external-impact action is not treated as safe merely because sandboxed.

## SER6 — Network isolation

Implement:

- default network profile;
- per-task allow policy;
- provider translation;
- blocked-request audit;
- approval/escalation path.

Exit gate:

- test malware fixture cannot contact unexpected domain;
- required package registry path can be enabled intentionally.

## SER7 — Secret Broker integration

Implement:

- secret references/bindings;
- provider-side protected injection when supported;
- TTL/revocation;
- audit without value logging;
- hard guarantee secrets never enter State Packs.

Exit gate:

- sandbox can make one approved authenticated API request without exposing raw secret through normal workspace/env/file inspection where provider mechanism supports this guarantee.

## SER8 — Isolated browser

Implement disposable Chrome/Playwright-style sandbox environment behind existing browser intent routing.

Exit gate:

- unknown-site browsing/download task can run without user personal browser profile.

## SER9 — Session reuse + pause/resume

Implement:

- task-level sandbox reuse;
- idle TTL;
- renew;
- pause/resume capability mapping;
- reconstruction fallback if provider cannot resume.

Exit gate:

- multi-step coding task does not pay cold-start cost for every command.

## SER10 — Remote Kubernetes execution

Only after local path is stable.

Implement:

- remote provider endpoint/auth;
- encrypted State Pack transport;
- tenant-scoped sessions;
- retention/deletion policy;
- remote health/quotas;
- Kubernetes-backed OpenSandbox deployment.

Exit gate:

- remote execution passes privacy/security review and produces identical ChangeSet semantics to local execution.

## SER11 — Warm pools

Implement provider pool classes and clean-session assignment.

Exit gate:

- measured cold-start reduction without state leakage across users/tasks.

## SER12 — Multi-agent state isolation

Update Universal Agent Runtime parallel mutation to use Workspace State Engine sessions rather than shared writable workspace copies.

Exit gate:

- two mutating agents can independently derive states from one base checkpoint;
- CodeLocal can compare/merge outcomes deterministically;
- no concurrent direct writes to authoritative workspace.

---

# 44. Recommended implementation order

```text
WSE0 terminology/contracts
  ↓
WSE1 store core
  ↓
WSE2 capture engine
  ↓
WSE3 ChangeSet/State Merge
  ↓
WSE4 State Pack
  ↓
SER0 safe-exec contracts/fake provider
  ↓
SER1 OpenSandbox local PoC
  ↓
SER2 state synchronization
  ↓
SER3 result round trip
  ↓
SER4 validated apply/restore
  ↓
SER5 execution router
  ↓
SER6 network
  ↓
SER7 secrets
  ↓
SER8 isolated browser
  ↓
SER9 reuse/pause/resume
  ↓
SER10 remote Kubernetes
  ↓
SER11 pools
  ↓
SER12 multi-agent isolation
```

Do not start with Kubernetes, pooling, multi-agent parallelism, or remote execution.

First prove the local safety transaction:

```text
Checkpoint
-> isolated execution
-> ChangeSet
-> State Merge
-> verification
-> authoritative apply
```

---

# 45. First concrete implementation slice

The first engineering slice should be deliberately narrow.

Implement only:

```text
1. internal/workspacestate domain models
2. local content-addressed Store
3. Capture() with hard security exclusions
4. Checkpoint creation
5. ChangeSet between checkpoints
6. State Pack full export/import
7. internal/safeexec Provider interface
8. fake provider
9. OpenSandbox local provider create/exec/destroy
10. import one State Pack into /workspace
11. run one command
12. export result
13. derive ChangeSet locally
14. show/verify result without auto-apply
```

This slice proves the architecture while keeping authoritative workspace mutation out of the first OpenSandbox integration.

---

# 46. Definition of Done for isolated coding MVP

A production MVP is complete when this flow passes:

```text
User: "Review this project and fix the failing test"

CodeLocal
  -> classify task as isolated execution
  -> capture base Checkpoint C100
  -> create Safe Execution Session
  -> export/import State Pack
  -> reconstruct /workspace
  -> install/run/build/test/edit inside sandbox
  -> capture incoming state A110
  -> derive ChangeSet C100 -> A110
  -> refresh authoritative workspace C105
  -> State Merge(BASE=C100, CURRENT=C105, INCOMING=A110)
  -> validate scope/security
  -> materialize candidate
  -> run project verification
  -> create final authoritative Checkpoint C120
  -> destroy session
  -> record safe Experience summary
```

Required guarantees:

```text
[PASS] no unknown code executed directly on host
[PASS] user workspace was not used as writable sandbox mount
[PASS] user's normal source-control workflow was not mutated for synchronization
[PASS] secret-class files were not included in State Pack
[PASS] sandbox had bounded resource/network policy
[PASS] concurrent user edits were preserved or surfaced as conflicts
[PASS] sandbox result was independently verified
[PASS] failed/unsafe result could not silently become authoritative
[PASS] session cleaned up on success/failure/expiry
[PASS] public MCP tool surface did not grow merely for OpenSandbox
```

---

# 47. Feature flags

Suggested rollout flags:

```text
CODELOCAL_WORKSPACE_STATE_V1
CODELOCAL_SAFE_EXECUTION
CODELOCAL_SAFE_EXECUTION_OPENSANDBOX
CODELOCAL_SAFE_EXECUTION_AUTO_ROUTE
CODELOCAL_SAFE_EXECUTION_NETWORK_POLICY
CODELOCAL_SAFE_EXECUTION_SECRET_BROKER
CODELOCAL_SAFE_EXECUTION_BROWSER
CODELOCAL_SAFE_EXECUTION_REMOTE
```

Roll out independently so state-engine correctness can be proven before automatic routing.

---

# 48. Migration strategy

Do not rewrite existing file/edit/terminal infrastructure.

Migration path:

```text
existing local tool implementation
        |
        +-- remains Local Runtime
        |
        +-- add Execution Router in front
                  |
                  +-- Local Runtime
                  +-- Safe Execution Runtime
```

Existing verification remains authoritative after both paths.

Existing Project Brain and Learned Skills continue to produce context/workflow knowledge.

Workspace State Engine is execution state, **not** long-term knowledge.

---

# 49. Relationship to Project Brain

Keep the planes separate.

```text
Project Brain
  knows WHY / WHAT IS TRUE / WHAT WORKED

Workspace State Engine
  knows WHAT FILESYSTEM STATE EXISTED

Safe Execution Runtime
  knows WHERE/HOW A RISKY ACTION RAN
```

Example:

```text
Project Brain:
"payment writes require transactions"

Workspace State Engine:
C100 -> C105 changed internal/payment/service.go

Safe Execution Runtime:
Task executed in isolated session S55; tests passed; network blocked one unexpected request
```

Only compact verified durable value should be promoted to Project Brain.

Do not upload raw checkpoints/state packs as memory.

---

# 50. Relationship to Learned Skills

A learned skill may request abstract steps such as:

```text
install dependencies
run project tests
build application
```

Execution Router decides the backend each time based on current policy/risk.

A skill must never encode:

```text
"bypass sandbox"
"disable network policy"
"reuse approval token"
```

A previously trusted skill is still subject to current execution isolation policy.

---

# 51. Relationship to Universal Agent Runtime

Universal Agent Runtime chooses/hosts the reasoning worker.

Safe Execution Runtime chooses/hosts the execution environment.

They are orthogonal:

```text
Claude / Codex / Gemini
       |
Universal Agent Runtime
       |
Execution intent/actions
       |
Safe Execution Runtime
       |
OpenSandbox
```

A provider agent may run inside a Safe Execution Session, or may remain outside while its tool actions are mediated into the session, depending on provider integration quality.

Long-term preferred mode for untrusted autonomous agents:

```text
agent process + workspace + commands
inside the same validated isolated boundary
```

when provider compatibility allows it.

---

# 52. What NOT to do

Avoid these shortcuts:

- expose OpenSandbox as a parallel public MCP namespace by default;
- add duplicate `sandbox_shell`, `sandbox_read`, `sandbox_write` tools to the public surface unnecessarily;
- mount the authoritative workspace writable into sandbox safe mode;
- synchronize by staging/committing/stashing/changing the user's repository state;
- require unfinished local work to be pushed remotely;
- transfer repository source through LLM prompt/context;
- capture secret files in normal checkpoints;
- give unknown code unrestricted outbound network by default;
- assume sandboxing authorizes production/account side effects;
- auto-apply stale agent results over concurrent user edits;
- store raw workspace checkpoints in Project Brain;
- build Kubernetes/pools before local transaction correctness;
- bind core architecture directly to OpenSandbox-specific SDK types;
- market rollback/merge safety before conflict/restore tests exist.

---

# 53. Decision log — 2026-08-17

## SED1 — OpenSandbox is an internal provider

Reason: CodeLocal owns the model/user contract; execution providers remain replaceable.

## SED2 — Workspace State Engine is the synchronization source of truth

Reason: isolated execution needs an exact task-state representation without mutating the user's normal repository workflow.

## SED3 — Domain terminology is Checkpoint/ChangeSet/State Pack/State Merge

Reason: CodeLocal should own a stable product/architecture vocabulary independent of storage implementation.

## SED4 — Authoritative workspace is not a writable sandbox mount

Reason: isolation is undermined if sandbox mutations directly alter the user's live files.

## SED5 — Workspace State is not Project Brain knowledge

Reason: execution state and durable semantic knowledge have different retention, privacy, and retrieval requirements.

## SED6 — State Merge protects concurrent user work

Reason: long-running agents must not silently overwrite newer human changes.

## SED7 — Sandbox success is not authoritative success

Reason: isolated output still requires scope/security validation and CodeLocal verification.

## SED8 — Security classification precedes provider routing

Reason: the backend must be chosen from policy/risk, not convenience or model preference.

## SED9 — Isolation does not replace approval for external impact

Reason: a sandboxed process can still trigger production/account/network side effects when credentials are granted.

## SED10 — First rollout is local and provider-neutral

Reason: prove state transaction correctness and provider abstraction before remote/Kubernetes complexity.

---

# 54. Maintenance rule

Whenever Workspace State Engine or Safe Execution architecture changes:

1. update this document;
2. add a dated Decision Log entry;
3. state whether the change affects domain terminology/contracts;
4. state migration impact for stored checkpoints/state packs;
5. update threat model;
6. update relevant acceptance/chaos tests;
7. update Universal Agent Runtime integration if agent isolation behavior changes;
8. never silently weaken user-workspace or secret boundaries for compatibility.

---

# 55. Final architecture target

```text
                         USER / AI CLIENTS
                                |
                                v
                           CodeLocal
                    +-----------+-----------+
                    |                       |
                    v                       v
              Project Brain         Universal Agent Runtime
                    |                       |
                    +-----------+-----------+
                                |
                                v
                         Execution Router
                                |
                 +--------------+--------------+
                 |                             |
                 v                             v
           Trusted Local                Safe Execution
              Runtime                       Runtime
                 |                             |
                 |                       Provider API
                 |                             |
                 |                        OpenSandbox
                 |                             |
                 |                  Docker / Kubernetes
                 |                             |
                 |                      Isolated Session
                 |                             |
                 +--------------+--------------+
                                |
                                v
                      Workspace State Engine
                     Checkpoint / ChangeSet
                      State Pack / Merge
                                |
                                v
                    Authoritative Workspace
                                |
                                v
                         Verification
                                |
                                v
                           Experience
```

The intended mental model is:

```text
CodeLocal remembers the project.
CodeLocal chooses the worker.
CodeLocal decides where risky work runs.
CodeLocal isolates untrusted execution.
CodeLocal protects the user's live workspace.
CodeLocal verifies before accepting changes.
```

That is the boundary OpenSandbox should strengthen inside CodeLocal.
