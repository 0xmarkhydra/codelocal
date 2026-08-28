# CodeLocal Cloud Workspace Runtime — RSR

Status: **Proposed implementation specification — branch source of truth**  
Date: **2026-08-29**  
Owner: **CodeLocal**  
Scope: **Workspace model, smart chat routing, multi-provider runtime, cloud sandbox execution, multi-user scheduling, persistence, media, Git, security, recovery and future extensibility**  
Target branch: **`feat/cloud-workspace-runtime-rsr`**

> RSR in this document means **Runtime & System Requirements**. This document turns the Cloud Workspace direction into implementable requirements and migration steps against the current `main` architecture.

---

# 1. Executive decision

CodeLocal MUST evolve from a local-worker-oriented runtime into a **workspace-centric, multi-provider execution platform** without weakening the existing Local Runtime.

The core architecture is:

```text
USER / CHAT
    │
    ▼
Context + Workspace Resolver
    │
    ▼
Persistent logical WORKSPACE
    │
    ▼
TASK
    │
    ▼
EXECUTION
    │
    ▼
Capability Resolver
    │
    ▼
Runtime Router
    │
    ├── LocalRuntimeProvider
    ├── OpenSandboxRuntimeProvider
    ├── future CodeLocalComputeProvider
    ├── future GPUProvider
    └── future EnterpriseProvider
```

The strategic invariant is:

> **Workspace is durable. Runtime is replaceable.**

OpenSandbox is an initial compute/runtime provider, not the definition of CodeLocal Cloud.

The user experience MUST default to:

> **User states the goal. CodeLocal resolves workspace, capabilities, engine and runtime automatically. Explicit user override always wins.**

---

# 2. Why this design is required

The near-term requirement is to let users execute work in CodeLocal Cloud when their own machine is offline or when they do not install a local client.

The long-term requirement is larger:

- entire source repositories can live and execute in CodeLocal Cloud;
- the same workspace can move between Local and Cloud without changing identity;
- one task can use multiple execution capabilities such as code, browser, video or GPU;
- multi-user load must be isolated, quota-controlled and schedulable;
- provider implementation can change without rewriting MCP tools, Agent logic or Workspace semantics;
- existing CodeLocal media, knowledge, identity and runtime behaviors must be reused where possible.

A cloud implementation that models `sandbox == workspace` would block these requirements because sandbox compute is ephemeral while project state must be durable.

---

# 3. Architectural invariants

The following rules are mandatory and are treated as non-negotiable implementation constraints.

## RSR-INV-001 — Workspace is not Runtime

A workspace MUST retain the same logical identity regardless of whether it is currently attached to:

- a user Mac;
- Windows/Linux Local Runtime;
- OpenSandbox;
- a future CodeLocal-managed VM/container;
- a GPU execution pool;
- an enterprise/self-hosted runtime.

Destroying or replacing a runtime MUST NOT destroy workspace identity.

## RSR-INV-002 — Workspace is not a local path

`LocalPath` may remain an important Local Runtime attribute, but it MUST NOT be the canonical identity of a workspace.

A workspace can have different materializations:

```text
Workspace ws_biddi
├── local materialization: ~/Documents/BIDDI
├── cloud materialization: persistent workspace state
└── runtime materialization: /workspace
```

## RSR-INV-003 — Sandbox is disposable

Sandbox IDs, container IDs, machine IDs, pod IDs and provider SDK objects MUST NOT leak into the product-level Workspace API or Agent-facing tool contracts.

## RSR-INV-004 — Tools are location-independent

Do not create duplicate MCP tools such as:

```text
local_shell
cloud_shell
local_read_file
cloud_read_file
```

Tools declare intent/capability. Runtime routing chooses execution location.

## RSR-INV-005 — Existing media is the media source of truth

Cloud execution MUST reuse CodeLocal's current media transport/publisher/presign path. Do not create a second artifact storage system merely for sandbox output.

## RSR-INV-006 — Existing Local Runtime remains first-class

Cloud is additive. Local behavior must continue to work if the cloud provider is unavailable.

## RSR-INV-007 — Explicit user choice beats Auto

If the user explicitly chooses a workspace, runtime or engine, that selection takes priority provided it is safe and capability-compatible.

## RSR-INV-008 — Multi-user ownership is server-authoritative

A client-provided workspace ID, runtime session ID, sandbox ID or job ID MUST always be ownership-validated server-side against the authenticated tenant/user.

---

# 4. Current `main` primitives to reuse

The current branch already contains useful primitives. The implementation SHOULD refactor and generalize these instead of replacing them.

## 4.1 Runtime lifecycle — reuse semantics

`internal/runtime/runtime.go` currently provides:

- a `Runtime` object;
- `WorkspaceWorker` instances keyed by workspace;
- worker reuse;
- worker health/lifecycle handling;
- per-workspace `lastUsed` tracking;
- a default `IdleWorkspace` timeout of 20 minutes;
- active-call cancellation bookkeeping;
- existing control-plane communication;
- media publisher ownership.

This is already close to the required logical session behavior.

Target migration:

```text
Current
Runtime
 └── workers[workspaceID] → WorkspaceWorker → localclient.Engine

Target
RuntimeSessionManager
 └── sessions[SessionKey] → RuntimeSession → RuntimeProvider
                                      ├── LocalProvider
                                      └── OpenSandboxProvider
```

The current local behavior SHOULD become the first provider implementation rather than remain hard-coded in the orchestration layer.

## 4.2 Media — reuse and generalize

Current relevant code includes:

- `internal/runtime/media.go`;
- `internal/mediatransport` publisher logic;
- runtime-owned `mediaPublisher`;
- existing presign/upload transformation flow.

The existing path appears image-first. The Cloud implementation MUST inspect and generalize the existing contract only as far as needed for:

```text
image
video
audio
file
```

Legacy image markers MUST remain backward compatible.

## 4.3 Runtime configuration and managed system projects — reuse

`internal/runtime/runtime_config.go` and related tests already provide managed runtime/system-project behavior, including current OpenMontage-specific materialization logic.

Required change:

```text
OpenMontage special case
        ↓
SystemProjectRegistry
        ├── openmontage
        ├── moneyprinterturbo
        └── future capabilities
```

System projects are capabilities/runtime dependencies, not user workspaces.

## 4.4 Realtime/control-plane protocol — reuse

`internal/runtime/realtime.go`, runtime control-plane synchronization and current protocol types SHOULD be extended rather than replaced for runtime/job progress where the existing abstractions fit.

## 4.5 Idempotency — reuse contract, verify backing store

`internal/idempotency` SHOULD be reused for execution request idempotency semantics.

However, Cloud multi-node correctness requires a shared/durable coordination mechanism. Any process-local journal is insufficient as the final authority for distributed leases and job ownership.

## 4.6 `internal/cloud` — do not overload incorrectly

The package currently serves existing cloud/control-plane materialization concerns. OpenSandbox execution MUST NOT simply be inserted into this package because its name contains `cloud`.

Provider-specific execution should have a clear runtime-provider boundary.

---

# 5. Product behavior: Smart Chat

The default UI MUST NOT require the user to choose OpenMontage, MoneyPrinterTurbo, OpenSandbox or a runtime machine before chatting.

Default mode:

```text
Workspace: Auto
Runtime: Auto
Engine: Auto
```

Only Workspace needs a visible product-level affordance, and even Workspace should be auto-resolved whenever safe.

## 5.1 Workspace resolution priority

`WorkspaceResolver` MUST evaluate in this order:

1. explicit workspace selected or named by user;
2. workspace already bound to the current conversation/thread;
3. strong repository/file/project reference in current request;
4. eligible recent workspace with high confidence;
5. temporary workspace when the task does not require an existing project;
6. ask the user only when ambiguity can materially damage or overwrite the wrong workspace.

The resolver MUST NOT ask merely because multiple workspaces exist if the task is independent of them.

Example:

```text
"Làm video TikTok từ link này"
→ no existing project required
→ temporary/personal media workspace is valid
→ do not ask user to select a code repository
```

Example:

```text
"Fix BID-173 trong BIDDI"
→ resolve BIDDI
→ no extra workspace prompt
```

## 5.2 Temporary workspace promotion

A task MAY start in a temporary workspace.

If the user later asks to retain or continue it, the workspace can be promoted to persistent state without changing the task/output history visible to the user.

## 5.3 Engine resolution

Engine choice is implementation detail by default.

Examples:

```text
social short / topic → video
→ MoneyPrinterTurbo eligible

reference video / custom motion / cinematic workflow
→ OpenMontage eligible

explicit "use OpenMontage"
→ OpenMontage override
```

## 5.4 Runtime resolution

Examples:

```text
local online + eligible + user policy allows
→ Local

local offline + cloud eligible
→ Cloud

requires GPU and local lacks GPU capability
→ GPU cloud pool

requires Xcode and no cloud macOS runtime exists
→ eligible Local macOS only
```

The routing decision SHOULD be explainable in diagnostics but SHOULD NOT add UX friction to normal chat.

---

# 6. Canonical domain model

## 6.1 Workspace

A Workspace is the durable logical project/context boundary.

Proposed conceptual model:

```go
type Workspace struct {
    ID        string
    TenantID  string
    OwnerID   string
    Name      string

    Source    WorkspaceSource
    Storage   WorkspaceStorageRef
    Settings  WorkspaceSettings

    CreatedAt time.Time
    UpdatedAt time.Time
}
```

A workspace MUST NOT contain provider-specific sandbox state as canonical identity.

## 6.2 WorkspaceSource

Possible source kinds:

```text
local
managed_git
external_git
uploaded
empty
template
```

The model must allow source code to migrate from a local-only materialization to cloud-managed source later.

## 6.3 Task

A Task represents the user's durable goal.

Examples:

```text
Fix BID-173
Create a 40-second TikTok video
Run the test suite and repair failures
Analyze this repository
```

A Task MAY contain multiple Executions.

## 6.4 Execution

An Execution is one schedulable/retryable unit of compute.

```text
Task
├── Execution #1: edit code
├── Execution #2: test
├── Execution #3: browser verification
└── Execution #4: render demo video
```

Execution is the correct unit for:

- queueing;
- retries;
- timeout;
- cancellation;
- resource accounting;
- audit;
- billing/metering;
- progress events.

## 6.5 RuntimeSession

A RuntimeSession represents an active attachment of a workspace to an execution environment.

RuntimeSession is ephemeral and may be recreated.

Conceptual identity:

```text
SessionKey = tenant + user + workspace + runtimeProfile + imageVersion
```

A single workspace MAY use multiple runtime sessions when capabilities differ.

## 6.6 RuntimeProvider

A provider is an adapter from CodeLocal runtime semantics to a concrete execution backend.

Initial implementations:

```text
LocalRuntimeProvider
OpenSandboxRuntimeProvider
```

Future implementations MUST be possible without changing tool semantics.

---

# 7. RuntimeProvider contract

The provider interface MUST model capabilities CodeLocal owns, not features unique to OpenSandbox.

Proposed Go direction:

```go
type RuntimeProvider interface {
    Name() string

    Acquire(ctx context.Context, req AcquireRequest) (*RuntimeSession, error)
    Health(ctx context.Context, session RuntimeSession) error

    Stage(ctx context.Context, session RuntimeSession, inputs []RuntimeInput) error
    Exec(ctx context.Context, session RuntimeSession, req ExecRequest) (<-chan RuntimeEvent, error)

    Suspend(ctx context.Context, session RuntimeSession) error
    Destroy(ctx context.Context, session RuntimeSession) error
}
```

Optional capabilities SHOULD be exposed through descriptors instead of ever-growing type assertions.

```go
type ProviderCapabilities struct {
    PersistentVolume bool
    Snapshot         bool
    Browser          bool
    Desktop          bool
    GPU              bool
    DirectUpload     bool
}
```

The provider interface MUST NOT expose `OpenSandboxSandbox` or provider SDK types above the adapter package.

---

# 8. Runtime request and capability model

Runtime routing MUST be based on a requirement description.

Conceptual request:

```go
type RuntimeRequest struct {
    TenantID    string
    UserID      string
    WorkspaceID string

    Mode        RuntimeMode // auto/local/cloud
    Profile     string

    Capabilities []Capability
    Constraints  RuntimeConstraints
}
```

Capability examples:

```text
shell
filesystem
git
node
python
browser
ffmpeg
video.moneyprinterturbo
video.openmontage
gpu
xcode
android-sdk
desktop
```

Runtime Router MUST select an eligible provider/session using requirements, user preference, state, quota, cost policy and availability.

---

# 9. Runtime routing algorithm

The initial routing order SHOULD be deterministic and simple.

```text
1. Validate explicit override.
2. Resolve hard capability constraints.
3. Reuse healthy compatible session if one exists.
4. Prefer eligible Local Runtime when policy says local-first.
5. Fall back to eligible Cloud Runtime.
6. Select resource profile.
7. Check quota/concurrency.
8. Acquire or queue.
```

Routing MUST NOT silently select a runtime containing stale/divergent workspace state if doing so risks destructive overwrite.

Routing result SHOULD contain diagnostic metadata:

```text
provider
reason
capabilities matched
profile
reusedSession
fallbackReason
```

This metadata can power debugging/admin UX without exposing complexity to normal users.

---

# 10. Cloud execution strategy decision

Three models were evaluated.

## Case A — permanent sandbox/VM per user

Rejected for default use because idle cost scales with account count rather than active compute.

## Case B — sandbox per command/job

Useful for stateless work but rejected as the default because iterative coding/video workflows repeatedly pay cold-start/materialization costs.

## Case C — workspace-affine ephemeral session — SELECTED

```text
workspace task arrives
→ reuse compatible healthy session if present
→ otherwise acquire sandbox
→ materialize workspace
→ execute one or more related operations
→ update last activity
→ idle timeout
→ persist/checkpoint required state
→ suspend/destroy compute
```

Initial idle timeout SHOULD reuse the current 20-minute runtime default unless product telemetry justifies a different value.

The timeout MUST be configurable by plan/profile later.

---

# 11. Multi-user session ownership and distributed leases

Current process-local `workers[workspaceID]` semantics are insufficient once multiple API/control-plane replicas can race.

Cloud runtime session ownership MUST have a durable/shared authority.

Conceptual table:

```text
runtime_sessions
----------------
id
tenant_id
user_id
workspace_id
provider
provider_session_id
runtime_profile
image_version
status
lease_owner
lease_expires_at
last_active_at
created_at
updated_at
```

## RSR-DIST-001

At most one active owner may mutate a given runtime session lease at a time.

## RSR-DIST-002

Lease acquisition MUST be atomic.

## RSR-DIST-003

A dead API/worker owner MUST lose the lease automatically after expiry.

## RSR-DIST-004

Provider session recreation MUST be idempotent from CodeLocal's perspective.

A distributed database lease is acceptable for MVP. Redis/other coordination can be introduced only if load or latency justifies it.

---

# 12. Scheduler and execution queues

CodeLocal MUST scale by active compute, not by total registered users.

Execution classes SHOULD start with:

```text
general
video-cpu
video-heavy
gpu
```

Potential future classes:

```text
browser
desktop
android
macos/xcode
enterprise-dedicated
```

The scheduler MUST consider:

- tenant/user concurrency;
- requested capabilities;
- resource profile;
- runtime/provider capacity;
- plan quota;
- priority;
- queue age;
- timeout;
- cancellation.

## 12.1 Fairness

Paid tiers may receive higher scheduling priority, but lower-priority work MUST age upward to prevent starvation.

## 12.2 Noisy neighbor protection

Every cloud runtime MUST receive hard resource limits appropriate to profile:

- CPU;
- RAM;
- disk;
- process/runtime duration;
- optionally network bandwidth;
- GPU allocation where relevant.

## 12.3 Initial profiles

Suggested product-neutral starting profiles:

```text
general-small
video-cpu
gpu-small
```

Exact CPU/RAM/GPU values belong in deploy configuration, not hard-coded routing business logic.

---

# 13. Workspace persistence and materialization

Persistent workspace state MUST be independent of sandbox root filesystem.

Logical separation:

```text
Persistent
├── workspace identity
├── source/Git state
├── user files requiring durability
├── workspace config
├── task/execution metadata
├── media references
└── knowledge/brain metadata

Ephemeral/cacheable
├── running processes
├── temp files
├── dependency caches
├── build cache
└── sandbox root filesystem
```

The implementation MAY use persistent volumes, snapshots, object storage or Git materialization depending on provider capabilities, but those mechanisms MUST sit behind CodeLocal abstractions.

## 13.1 WorkspaceStorage abstraction

A storage abstraction SHOULD support the minimum semantic operations:

```go
type WorkspaceStorage interface {
    Materialize(ctx context.Context, workspaceID string, target RuntimeSession) error
    Persist(ctx context.Context, workspaceID string, source RuntimeSession) error
}
```

Do not prematurely build a distributed filesystem if Git + existing file/media persistence is sufficient for MVP.

---

# 14. Source code and Git migration path

The architecture MUST support a future flow where a user moves a local repository into CodeLocal Cloud.

Target product flow:

```text
Local workspace
→ detect Git repository
→ identify committed + uncommitted state
→ establish cloud source/storage
→ synchronize safely
→ materialize cloud runtime
→ continue same logical workspace
```

Cloud Git operations SHOULD support CodeLocal-managed Git identity/account by default while retaining explicit user Git integrations where required.

## RSR-GIT-001

Runtime replacement MUST NOT implicitly become the source-of-truth transition.

## RSR-GIT-002

Local/cloud divergence MUST be detected before destructive synchronization.

## RSR-GIT-003

Uncommitted user changes MUST never be silently discarded during migration or runtime fallback.

---

# 15. OpenSandbox provider requirements

OpenSandbox is the first intended cloud execution backend.

`OpenSandboxRuntimeProvider` MUST translate only provider-neutral runtime operations:

```text
Acquire
Health
Stage input
Exec/stream
Cancel
Suspend where supported
Destroy
```

Provider configuration SHOULD include:

```text
endpoint
API key reference
default image/resource profiles
network policy
idle/lifecycle policy
```

Secrets MUST NOT be logged.

OpenSandbox authentication MUST be enabled for non-local production deployments.

The initial secure-runtime tier SHOULD prefer an isolation mode compatible with the selected host infrastructure; stronger isolation can be upgraded without changing RuntimeProvider semantics.

OpenSandbox metadata/state MUST NOT become the only durable source for CodeLocal workspace/session records.

---

# 16. Runtime images and capability images

Cloud runtimes SHOULD use prebuilt, pinned images rather than cloning/installing large dependency trees for every task.

Initial direction:

```text
codelocal/runtime-base
├── CodeLocal runtime/worker pieces
├── git
├── node
├── python
└── common tooling

codelocal/video-runtime
├── base
├── ffmpeg
├── MoneyPrinterTurbo dependencies
├── OpenMontage dependencies
└── Remotion/video tooling as required
```

Exact system-project source can remain separately versioned/mounted if licensing or update cadence requires it.

Image/version selection MUST be part of session compatibility; do not reuse a warm session if its environment does not satisfy the requested capability/version.

---

# 17. SystemProjectRegistry

Managed system projects MUST become registry-driven.

Conceptual entry:

```go
type SystemProjectDefinition struct {
    ID           string
    Source       string
    Managed      bool
    Hidden       bool
    Capabilities []string
    Materialize  MaterializationPolicy
}
```

Initial registry:

```text
openmontage
moneyprinterturbo
```

Requirements:

- preserve existing OpenMontage behavior during migration;
- reject spoofed/unknown managed system projects;
- MPT SHOULD be lazy/on-demand unless bundled in a runtime image;
- system projects MUST NOT appear as normal user workspaces;
- system projects SHOULD be mounted/read-only when feasible;
- version changes SHOULD be explicit/pinned enough to reproduce tasks.

---

# 18. Video execution router

Video is an initial Cloud workload, not a special architecture.

Normalized request may contain:

```text
intent
source type
aspect ratio
duration
tts preference
branding
engine override
```

Auto routing direction:

```text
simple social short / batch / topic-to-video
→ MoneyPrinterTurbo

reference-driven / custom motion / cinematic
→ OpenMontage
```

User-specific TTS/branding belongs in user/workspace profile, not global defaults.

The video engine MUST output normalized media descriptors so the existing CodeLocal media pipeline can publish results.

---

# 19. Existing media transport extension

Cloud MUST reuse the existing media publisher.

Desired generic marker direction:

```json
{
  "__mcpMedia": {
    "path": "/workspace/renders/final.mp4",
    "kind": "video",
    "mimeType": "video/mp4",
    "name": "final.mp4"
  }
}
```

Normalized result:

```json
{
  "__mcpMediaRef": {
    "kind": "video",
    "mimeType": "video/mp4",
    "name": "final.mp4",
    "url": "..."
  }
}
```

Compatibility requirement:

```text
__mcpImage
__mcpImageRef
```

must continue to work.

## RSR-MEDIA-001 — Direct transfer

Large sandbox outputs SHOULD upload directly to existing object storage through signed upload targets rather than streaming through the CodeLocal API process.

```text
Sandbox → signed upload → existing storage
                      ↑
              CodeLocal control metadata
```

The same principle applies to large sandbox inputs via signed download where supported.

---

# 20. Secret handling

CodeLocal MUST distinguish secret **use** from secret **exposure**.

Allowed after policy/approval where required:

```text
process receives secret reference/value in controlled environment
→ process calls authorized external API
→ secret is not emitted
```

Blocked:

```text
echo secret
print environment
log secret
return secret to model
persist secret into task output
```

Secrets SHOULD be referenced by opaque IDs in durable execution metadata.

Provider/runtime injection MUST be short-lived and scoped to the execution/session requiring it.

OpenSandbox credential/vault mechanisms MAY be used when they preserve these guarantees, but CodeLocal remains the policy authority.

---

# 21. Security and isolation requirements

Cloud sandbox MUST NOT receive:

- host filesystem access;
- host Docker socket;
- host cloud credentials;
- other tenant workspace volumes;
- unrestricted privileged mode;
- unscoped CodeLocal service credentials.

Required controls:

```text
tenant/user ownership validation
CPU/RAM/disk/time limits
network policy
authenticated sandbox API
scoped secret injection
read-only system projects where practical
sandbox cleanup
provider audit metadata
```

Every file/media/runtime API crossing tenant boundaries MUST validate tenant/user ownership.

---

# 22. Job lifecycle

Initial execution lifecycle:

```text
QUEUED
  ↓
ACQUIRING_RUNTIME
  ↓
MATERIALIZING
  ↓
RUNNING
  ↓
PUBLISHING_OUTPUTS
  ↓
SUCCEEDED
```

Terminal alternatives:

```text
FAILED
CANCELLED
TIMED_OUT
```

Recovery state MAY include:

```text
RECOVERING
```

Runtime-session lifecycle:

```text
REQUESTED
PROVISIONING
READY
BUSY
IDLE
SUSPENDING / PERSISTING
SUSPENDED / DESTROYED
LOST
```

State transitions MUST be explicit and idempotent enough to survive retrying API requests.

---

# 23. Retry and idempotency

A job retry MUST NOT blindly repeat expensive completed stages.

Example:

```text
script    ✓
tts       ✓
render    ✓
upload    ✗
```

Retry SHOULD resume at output publishing instead of rendering again when the render output is still available/persisted.

Execution APIs SHOULD accept an idempotency key.

Double-clicking or retrying the same submission MUST NOT create duplicate active executions when the prior request is still valid.

Provider `Acquire` calls MUST also tolerate client/server retry without creating uncontrolled duplicate sandbox sessions.

---

# 24. Progress and event model

Users MUST see meaningful progress for long-running cloud work.

Events SHOULD normalize into provider-independent types:

```text
runtime.requested
runtime.provisioning
runtime.ready
workspace.materializing
execution.started
execution.progress
execution.log
output.detected
output.publishing
output.ready
execution.completed
execution.failed
runtime.released
```

SSE is sufficient for one-way execution progress in the initial web/control-plane path.

Interactive terminal/desktop sessions may require WebSocket or a provider-specific streaming bridge later.

Normal UI SHOULD show human-readable states, not provider IDs.

---

# 25. Proposed persistent data model

The exact database technology/schema SHOULD follow current backend conventions, but the logical records are required.

## workspaces

Must support durable workspace identity independent of runtime.

Important fields:

```text
id
tenant_id
owner_id
name
source_kind
source_ref
storage_ref
settings
created_at
updated_at
```

## runtime_sessions

```text
id
tenant_id
user_id
workspace_id
provider
provider_session_id
runtime_profile
image_version
status
lease_owner
lease_expires_at
last_active_at
created_at
updated_at
```

## execution_jobs

```text
id
tenant_id
user_id
workspace_id
runtime_session_id nullable
status
intent/resource_class
priority
idempotency_key
started_at
finished_at
error_code
error_summary
```

## execution_usage

May be introduced when metering is needed:

```text
execution_id
cpu_seconds
memory_seconds
gpu_seconds
storage_bytes
egress_bytes
```

Do not add a duplicate media table if the existing media subsystem already owns the required metadata.

---

# 26. Proposed code boundaries

This is a direction, not a requirement to create every package on day one.

```text
internal/runtime/
    existing local-facing/runtime integration

internal/runtimeprovider/
    provider.go
    types.go
    local/
    opensandbox/

internal/runtimesession/
    manager.go
    lease.go

internal/runtimequeue/
    scheduler.go
    profiles.go

internal/workspace/
    existing registry/domain extended toward logical workspace identity

internal/systemprojects/
    registry.go
    materializer.go

internal/mediatransport/
    existing publisher extended for generic media where necessary
```

Do not move files solely to make the tree look clean. Refactor boundaries only when tests prove behavior remains intact.

---

# 27. Migration plan from current `main`

## Phase P0 — freeze invariants and tests

Before provider extraction:

- add/strengthen tests for current Local Runtime activation/reuse;
- verify 20-minute default idle semantics;
- verify call cancellation;
- verify media image behavior;
- verify OpenMontage managed-project behavior;
- capture runtime status/error compatibility.

Exit criterion: tests protect the Local Runtime behavior that must not regress.

## Phase P1 — extract RuntimeProvider with Local only

Goal: architecture changes, user behavior does not.

Steps:

1. introduce provider-neutral request/session/event types;
2. wrap existing local `WorkspaceWorker`/`localclient.Engine` path in `LocalRuntimeProvider`;
3. introduce `RuntimeSessionManager` using Local provider only;
4. keep current activation/control-plane interfaces stable;
5. preserve existing tests and add provider contract tests.

Exit criterion:

```text
all current Local Runtime behavior passes
no Cloud provider required
```

## Phase P2 — OpenSandbox provider proof

Implement minimal provider:

```text
Acquire
Health
Exec
Cancel
Destroy
```

Use a dev sandbox environment with mandatory authentication.

Acceptance test:

```text
Cloud runtime → echo/test command → streamed result
```

## Phase P3 — workspace materialization + durable session lease

Add:

- runtime session DB record;
- atomic lease;
- workspace materialization;
- session reuse;
- idle release;
- lost-session detection;
- multi-API-node race tests.

Acceptance test:

```text
request #1 via API node A → sandbox X
request #2 via API node B for same session key → reuse X or safely serialize
```

## Phase P4 — generic media output E2E

Inspect current media backend/UI and determine exact extension needed.

Implement the minimum generic media support while retaining image compatibility.

Primary acceptance flow:

```text
input image/audio
→ Cloud runtime
→ FFmpeg
→ final.mp4
→ existing media transport
→ playable/downloadable media reference in chat
```

No parallel media storage architecture.

## Phase P5 — SystemProjectRegistry + video engines

Refactor existing managed project logic into a registry.

Add:

```text
openmontage
moneyprinterturbo
```

Implement Video Router and normalized output.

Acceptance flows:

```text
"Make a 30s social video"
→ auto engine
→ render
→ media ref

"Use OpenMontage and follow this reference"
→ explicit engine override
→ OpenMontage pipeline
```

## Phase P6 — Scheduler + quota + resource pools

Introduce:

- queue;
- concurrency limits;
- general/video/GPU resource classes;
- priority + aging;
- usage metadata;
- autoscaling integration hooks.

This phase makes the architecture production-ready for larger multi-user load.

## Phase P7 — source-code-to-cloud migration

Only after the workspace/runtime boundary is proven:

- persistent cloud workspace source;
- Git synchronization;
- CodeLocal-managed cloud Git identity;
- local/cloud divergence protection;
- move-to-cloud UX;
- cloud-first workspaces that never existed locally.

This phase fulfills the long-term requirement of hosting entire code projects in CodeLocal Cloud.

---

# 28. Backward compatibility requirements

The migration MUST NOT break:

- existing paired Local Runtime workflows;
- existing workspace registry behavior without an explicit migration path;
- current media image references;
- current OpenMontage managed project materialization;
- existing device authorization;
- local operation when Cloud is unavailable;
- current tool names merely because execution can happen remotely.

Feature flags SHOULD gate Cloud routing during rollout.

Recommended rollout flags:

```text
runtime_provider_abstraction
cloud_runtime_enabled
cloud_auto_fallback
cloud_media_generic
system_project_registry
cloud_scheduler_enabled
```

---

# 29. Testing requirements

## 29.1 Provider contract tests

Every RuntimeProvider MUST pass common behavioral tests:

```text
acquire
health
exec success
exec failure
cancel
timeout
destroy
idempotent cleanup
```

## 29.2 Local regression tests

Existing Local Runtime behavior must be fully covered before routing becomes multi-provider.

## 29.3 Session/concurrency tests

Required race scenarios:

- two API replicas acquire same workspace session simultaneously;
- lease owner dies;
- session expires while a job is starting;
- provider returns created but client times out;
- repeated acquire request with same idempotency key;
- user requests two incompatible runtime profiles for same workspace.

## 29.4 Isolation tests

Must verify:

- user A cannot access user B runtime/session/files/media;
- guessed provider session ID is useless without ownership;
- system project mount cannot mutate host/control-plane state;
- secrets do not appear in logs/events/result payloads.

## 29.5 Media tests

Required:

```text
legacy image → still works
video → publish works
audio → publish works
large output → direct upload path
failed publish → retry without unnecessary rerender
```

## 29.6 Recovery tests

Simulate:

- provider node death;
- API node death;
- sandbox disappearance;
- output upload failure;
- network interruption;
- queue restart.

---

# 30. Observability requirements

Every execution SHOULD be traceable across:

```text
request_id
task_id
execution_id
tenant_id
workspace_id
runtime_session_id
provider
provider_session_id (server/admin logs only)
```

Metrics SHOULD include:

```text
queue wait
sandbox acquire latency
workspace materialization latency
execution duration
media publish duration
runtime reuse rate
cold-start rate
failure/retry rate
provider capacity
CPU/GPU usage when available
```

Secrets and raw sensitive environment values MUST be redacted before logging.

---

# 31. Cost-control requirements

Cost MUST scale primarily with active compute.

Do not allocate one always-on VM per account by default.

Cost controls:

- lazy runtime acquisition;
- workspace-affine reuse during active session;
- idle release;
- bounded concurrency;
- CPU-first profiles;
- GPU only when capability requires it;
- direct media transfers;
- optional warm pools only when traffic justifies them;
- reusable dependency/runtime images.

Warm pools SHOULD be an optimization after cold-start measurements, not an MVP requirement.

---

# 32. Future extension requirements

The design MUST allow these without rewriting the core Workspace/Task model:

## 32.1 Multiple runtime providers

```text
OpenSandbox
CodeLocal-managed compute
enterprise self-hosted
GPU-specific provider
macOS provider
```

## 32.2 Multiple runtimes per task/workspace

Example future task graph:

```text
Workspace BIDDI
├── coding runtime
├── browser runtime
└── video runtime
```

## 32.3 Organizations/teams

Tenant ID MUST exist in cloud-owned records even if initial products are user-centric.

Future policy may add:

```text
organization quota
shared workspace
roles
enterprise audit
runtime region policy
```

## 32.4 Background/long-running Agent work

Task/Execution separation allows future asynchronous Agent workflows without equating chat messages to shell processes.

## 32.5 Billing

Execution/resource metadata allows future billing without introducing billing logic into providers.

---

# 33. Explicit non-goals for initial MVP

Do NOT block delivery by building these prematurely:

- Kubernetes control plane owned by CodeLocal;
- custom distributed filesystem;
- permanent VM per user;
- multi-region scheduler;
- Kafka solely for runtime events;
- bespoke media/object storage layer;
- GPU for all users;
- automatic local/cloud bidirectional file sync before divergence semantics are correct;
- full desktop streaming before shell/files/media Cloud execution works.

---

# 34. Acceptance scenarios

The architecture is considered directionally correct when all scenarios can fit the same model.

## Scenario A — local coding

```text
User: "Fix tests in BIDDI"
WorkspaceResolver → BIDDI
RuntimeRouter → healthy Local Runtime
Task/Execution → LocalProvider
```

No behavior regression.

## Scenario B — local machine offline

```text
User: "Fix tests in BIDDI"
WorkspaceResolver → BIDDI
Local unavailable
Cloud workspace source available
RuntimeRouter → OpenSandboxProvider
```

User does not manually reconnect or select a sandbox.

## Scenario C — user with no installed client

```text
new user
→ create/import cloud workspace
→ chat
→ cloud runtime acquired on demand
→ edit/test/output
```

No personal computer required.

## Scenario D — social video

```text
"Make a 30-second TikTok from this article"
→ temporary/current workspace
→ video capability
→ auto MPT
→ video runtime
→ final.mp4
→ existing media publisher
```

## Scenario E — reference video

```text
"Use this reference and make a branded version with OpenMontage"
→ explicit OpenMontage override
→ compatible video runtime
→ output via existing media
```

## Scenario F — multi-user burst

```text
1000 logged-in users
200 active chats
50 compute executions
```

Capacity follows active compute, not 1000 permanent machines.

## Scenario G — entire codebase moved to Cloud

```text
Local BIDDI workspace
→ safe source migration
→ same Workspace ID
→ cloud materialization
→ Mac can be offline
→ future chats continue against cloud source/runtime
```

No workspace identity replacement is required.

---

# 35. Definition of Done for the first production Cloud Runtime release

The first production release is DONE only when:

- [ ] Local Runtime regression suite passes.
- [ ] `RuntimeProvider` boundary exists and Local uses it.
- [ ] OpenSandbox provider can acquire/exec/cancel/release.
- [ ] Runtime sessions are durable enough for multiple API replicas.
- [ ] Session lease prevents duplicate ownership.
- [ ] Cloud execution is tenant/user ownership-scoped.
- [ ] Workspace can be materialized independently of sandbox lifetime.
- [ ] Idle runtime is released automatically.
- [ ] Job cancellation and timeout work.
- [ ] Existing media path can publish required Cloud output types.
- [ ] Legacy image media behavior remains compatible.
- [ ] SystemProjectRegistry replaces OpenMontage-only special casing without regression.
- [ ] MoneyPrinterTurbo can be selected lazily/on-demand.
- [ ] Video output reaches the user through existing media.
- [ ] Secrets never appear in execution logs/results.
- [ ] Provider API credentials are not exposed to sandbox workloads unnecessarily.
- [ ] Basic quota/concurrency prevents one user from exhausting shared compute.
- [ ] Cloud feature can be disabled without breaking Local Runtime.
- [ ] Observability can trace a failed task from request to runtime/provider.

---

# 36. Recommended implementation sequence

For engineering execution, use this strict dependency order:

```text
1. Protect existing Local behavior with tests
2. Extract RuntimeProvider
3. Re-home current Local worker path under LocalProvider
4. Add RuntimeSessionManager
5. Add OpenSandboxProvider
6. Add durable session + distributed lease
7. Add workspace materialization/persistence seam
8. Extend existing media only as required
9. Generalize SystemProjectRegistry
10. Add MPT/OpenMontage routing
11. Add scheduler/quota/resource profiles
12. Add full source-code Cloud migration
13. Optimize warm pools/GPU/autoscaling from telemetry
```

Do not start by implementing provider-specific UI or a second Cloud-only tool surface.

---

# 37. Final architecture

```text
                         USER
                           │
                           ▼
                          CHAT
                           │
                           ▼
                Context / Workspace Resolver
                           │
                           ▼
                  DURABLE WORKSPACE
              ┌────────────┼─────────────┐
              │            │             │
            Source       Storage       Existing Media
              │            │             │
              └────────────┼─────────────┘
                           │
                           ▼
                          TASK
                           │
                           ▼
                       EXECUTION
                           │
                           ▼
                  Capability Resolver
                           │
                           ▼
                     Runtime Router
              ┌────────────┼──────────────┐
              │            │              │
              ▼            ▼              ▼
           Local       OpenSandbox    Future Provider
              │            │              │
              └────────────┼──────────────┘
                           ▼
                    Runtime Session
                           │
                    System Projects
                           │
                    Tools / Processes
                           │
                           ▼
                  Existing Media / Git
                           │
                           ▼
                          USER
```

The long-term platform rule is simple:

> **CodeLocal owns Workspace, Task, Execution, routing, policy and durable state. Runtime providers only supply replaceable compute.**

That boundary is what allows today's Local Runtime and OpenSandbox MVP to grow into a full CodeLocal Cloud development/workspace platform later without another architectural reset.
