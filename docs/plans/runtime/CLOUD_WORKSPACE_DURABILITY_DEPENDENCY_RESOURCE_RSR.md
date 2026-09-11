# CodeLocal Cloud Workspace — Durability, Dependency Cache & Resource RSR

Status: **Implementation requirements — companion to CLOUD_WORKSPACE_RUNTIME_RSR.md**  
Date: **2026-08-29**  
Owner: **CodeLocal**  
Branch: **`feat/cloud-workspace-runtime-rsr`**

> This document locks the persistence, rebuild-safety, dependency isolation, cache sharing, runtime image, quota and resource-management rules required for CodeLocal Cloud to scale without losing user data or wasting compute.

---

# 1. Executive decision

CodeLocal Cloud MUST follow four non-negotiable rules:

1. **User data never depends on Railway deployment filesystem.**
2. **Runtime/Sandbox may be destroyed at any time without losing canonical workspace state.**
3. **Dependency cache may be shared; writable dependency environments must remain workspace-isolated.**
4. **Heavy/common dependencies belong in versioned runtime images rather than being installed from scratch for every task.**

Canonical architecture:

```text
                         CODELOCAL CLOUD

                    Railway Control Plane
              API / MCP / Web / Router / Scheduler
                              │
             ┌────────────────┼────────────────┐
             │                │                │
             ▼                ▼                ▼
         PostgreSQL      Object Storage     Git Source
       metadata/state   snapshots/media     code history
             │                │                │
             └────────────────┼────────────────┘
                              │
                       Durable Workspace
                              │
                              ▼
                       Runtime Session
                              │
                              ▼
                    Disposable Sandbox
                              │
                 workspace materialization
                              │
          ┌───────────────────┼──────────────────┐
          ▼                   ▼                  ▼
     Base Runtime        Shared Caches      Workspace Env
        Image             read/cache          isolated
```

Strategic invariant:

> **Durable state lives outside compute. Compute is replaceable.**

---

# 2. Failure model

The architecture MUST assume all of the following can happen at any time:

- Railway deploy/redeploy;
- API replica restart;
- API replica replacement;
- sandbox crash;
- sandbox eviction;
- worker node restart;
- provider outage;
- task timeout;
- runtime image rollout;
- dependency cache eviction;
- network interruption;
- user reconnect from another device.

None of these events may cause loss of committed/canonical workspace data.

The system MUST distinguish between:

```text
CANONICAL STATE
- workspace identity
- source metadata
- Git refs/commit state
- user-created files that must persist
- workspace manifest
- task state
- execution state
- media refs
- secret references
- configuration

RECONSTRUCTABLE STATE
- node_modules
- Python virtualenv
- package download cache
- build output cache
- temp renders
- process memory
- running processes
- stdout ring buffers
- sandbox filesystem outside durable workspace paths
```

Canonical state MUST survive compute replacement.

Reconstructable state MAY be discarded.

---

# 3. Railway deployment safety

## RSR-DUR-001 — Railway service filesystem is not canonical storage

CodeLocal MUST NOT persist user workspace source, task state, user media, Git ownership state or secret values solely in the writable filesystem of a Railway service deployment.

Application containers are treated as replaceable deployment units.

Permitted local filesystem usage in Railway services:

- temporary serialization;
- ephemeral request buffering;
- disposable caches;
- local process temp files;
- generated artifacts awaiting successful upload, provided failure recovery exists.

Prohibited as sole storage:

- user source trees;
- task history;
- workspace metadata;
- canonical media;
- durable runtime session ownership;
- billing/usage authority;
- secrets.

## RSR-DUR-002 — Rebuild invariant

A successful CodeLocal control-plane redeploy MUST NOT require user action to recover an existing workspace.

Expected behavior:

```text
old deployment
      ↓ replaced
new deployment
      ↓
loads durable metadata
      ↓
reacquires leases/session state
      ↓
continues accepting work
```

## RSR-DUR-003 — No per-user Railway volume architecture

Do NOT model:

```text
1 user = 1 Railway Volume
```

or:

```text
1 workspace = 1 Railway Volume
```

as the primary platform architecture.

Reasons:

- volume count does not map cleanly to large multi-tenant scale;
- compute placement becomes coupled to storage placement;
- runtime-provider portability is reduced;
- multi-region expansion becomes harder;
- user/workspace count would directly create infrastructure objects.

Railway Volumes MAY be used for service-level persistence where appropriate, but user workspace durability must use provider-neutral storage abstractions.

---

# 4. Canonical durable storage model

CodeLocal SHOULD split durable data into four categories.

## 4.1 Metadata — PostgreSQL

Examples:

```text
users
workspaces
workspace_sources
workspace_materializations
runtime_sessions
execution_jobs
job_events
usage_records
workspace_snapshots
media metadata / refs if already represented by existing system
secret refs
runtime profiles
```

PostgreSQL is authoritative for ownership and state transitions.

## 4.2 Source history — Git

Source-code workspaces SHOULD use Git as primary versioned source history whenever possible.

Possible source modes:

```text
external_git
codelocal_managed_git
local_git_synced
uploaded_source
empty/template
```

Git is NOT the only persistence mechanism because uncommitted state may need preservation.

## 4.3 Workspace snapshots / large files — object storage

Object storage SHOULD hold:

- workspace snapshots/checkpoints;
- uncommitted working-tree bundles where required;
- uploaded inputs;
- generated artifacts routed through existing CodeLocal media system;
- large binary workspace files unsuitable for database rows.

## 4.4 Media — existing CodeLocal media system

Do not create a second artifact/media subsystem.

Sandbox output flow:

```text
/workspace/renders/final.mp4
        ↓
existing CodeLocal media publisher
        ↓
presigned upload
        ↓
object storage
        ↓
media ref
        ↓
Chat / UI
```

---

# 5. Workspace persistence states

A workspace MAY have multiple materializations, but one logical ID.

Conceptual model:

```text
Workspace ws_123
├── metadata
├── source
├── latest durable snapshot
├── local materialization(s)
└── cloud materialization(s)
```

Suggested materialization states:

```text
ABSENT
MATERIALIZING
READY
DIRTY
SNAPSHOTTING
SYNCED
STALE
CONFLICTED
ERROR
```

## RSR-DUR-010 — Dirty state preservation

Before destroying a cloud runtime containing user changes that are not already durable, CodeLocal MUST either:

A. persist the changes successfully; or
B. keep the runtime alive/retry preservation; or
C. surface a hard failure and block destructive teardown.

Never silently destroy a dirty workspace.

## RSR-DUR-011 — Safe idle teardown

Idle teardown flow:

```text
ACTIVE
  ↓ idle threshold reached
DRAINING
  ↓ stop accepting new execution on session
PERSISTING
  ↓ snapshot/sync dirty state
SYNCED
  ↓
DESTROY SANDBOX
```

If persistence fails:

```text
PERSISTING
   ↓ error
RECOVERY_REQUIRED
```

The runtime MUST NOT be destroyed until policy determines data is safe.

---

# 6. Snapshot strategy

Snapshots are recovery aids, not a replacement for Git.

Suggested snapshot contents:

- workspace working tree excluding reconstructable paths;
- untracked source files;
- workspace configuration;
- optionally tool-specific durable state.

Default exclusions SHOULD include:

```text
node_modules/
.venv/
venv/
__pycache__/
.next/cache/
dist/ when reconstructable
build/ when reconstructable
.tmp/
tmp/
.cache/ package download caches
large generated render intermediates
```

Do not blindly snapshot every byte in `/workspace`.

## Snapshot triggers

Initial MVP:

- before idle destroy if workspace dirty;
- before provider migration;
- before destructive restore/rebase operation;
- optional periodic checkpoint for long-running tasks;
- explicit user checkpoint.

---

# 7. Dependency architecture

Dependency management MUST distinguish three layers.

```text
Layer 1 — Runtime Image
Layer 2 — Shared Package Cache
Layer 3 — Workspace Environment
```

This is mandatory for both performance and isolation.

---

# 8. Layer 1 — Runtime images

Runtime images contain expensive/common platform dependencies.

Initial profiles:

## general-small

```text
Linux base
CodeLocal runtime worker
Git
Node.js
npm/corepack/pnpm
Python
uv/pip
common shell utilities
certificates
```

Target resources:

```text
2 vCPU
4 GB RAM
```

## browser

Includes general-small plus:

```text
Chromium
Playwright runtime dependencies
browser fonts
```

Target resources:

```text
2–4 vCPU
4–8 GB RAM
```

## video-cpu

Includes general-small plus:

```text
FFmpeg
Node/Remotion dependencies suitable for rendering
common media codecs
runtime support required by managed video engines
```

Managed projects SHOULD be baked/materialized efficiently according to licensing and update policy rather than cloned from the public Internet on every task.

Target resources:

```text
4–8 vCPU
8–16 GB RAM
```

## gpu-ai

Includes selected GPU stack:

```text
GPU runtime
CUDA-compatible dependencies when applicable
Python AI stack
model runtime
```

GPU profiles MUST use separate scheduling pools and quotas.

## RSR-DEP-001 — Version images immutably

Never depend on mutable `latest` as durable runtime identity.

Example:

```text
codelocal/general-runtime:2026.08.29-1
codelocal/video-runtime:2026.08.29-2
```

RuntimeSession records SHOULD persist image version.

---

# 9. Layer 2 — Shared dependency cache

Shared caches MAY be reused across tenants only when the data is non-secret, content-addressed or otherwise safe to share.

Examples:

```text
npm package tarball cache
pnpm content-addressable store
pip wheel/download cache
uv cache
Playwright browser binary cache if immutable
language SDK archives
container image layers
public model weights where licensing permits
```

## RSR-DEP-010 — Share cache, never mutable installation

Safe:

```text
User A ─┐
        ├─ read package blob sha256:abc...
User B ─┘
```

Unsafe:

```text
User A writes /shared/node_modules
User B executes /shared/node_modules
```

Workspace installations MUST remain logically isolated.

## RSR-DEP-011 — Cache keys

Cache objects SHOULD be keyed by immutable identity where practical:

```text
package ecosystem
package name
version
integrity hash
runtime platform
architecture
```

## RSR-DEP-012 — Cache is disposable

Loss of dependency cache MUST affect performance only, not correctness or workspace durability.

---

# 10. Layer 3 — Workspace dependency environment

Workspace environment reflects the dependency lockfiles/configuration owned by the project.

Examples:

```text
package.json
pnpm-lock.yaml
package-lock.json
yarn.lock
pyproject.toml
uv.lock
requirements.txt
poetry.lock
Gemfile.lock
go.mod
go.sum
Cargo.lock
```

Generated installations such as `node_modules` or `.venv` MAY be ephemeral.

## RSR-DEP-020 — Lockfiles are durable

Dependency manifests and lockfiles are canonical workspace state and MUST persist.

## RSR-DEP-021 — Installations are reconstructable by default

`node_modules`, `.venv`, package caches and build caches SHOULD be treated as reconstructable unless a workload has a proven reason to persist them.

## RSR-DEP-022 — Environment fingerprint

CodeLocal SHOULD compute an environment fingerprint such as:

```text
runtime_image_version
+ os/arch
+ package_manager
+ lockfile hash
+ selected toolchain versions
```

Example:

```text
sha256(video-runtime:v3 + linux-amd64 + pnpm + pnpm-lock hash)
```

This fingerprint MAY be used to reuse a prepared workspace environment safely.

---

# 11. Warm environment reuse

A workspace can reuse an existing prepared environment when all conditions match:

- same tenant/workspace;
- compatible runtime image;
- same dependency fingerprint;
- environment passes basic integrity check;
- no policy requires clean rebuild.

Potential flow:

```text
Acquire runtime
    ↓
restore workspace
    ↓
compute dependency fingerprint
    ↓
prepared env exists?
   / \
 yes  no
  │    │
reuse  install using shared cache
```

Do NOT share a writable prepared environment between unrelated tenants.

---

# 12. Heavy dependency promotion policy

If a package/tool is repeatedly installed across many workloads and is expensive to install, it SHOULD be considered for promotion into a runtime image.

Candidates:

- FFmpeg;
- Chromium/Playwright;
- Android SDK components;
- Remotion runtime dependencies;
- OpenMontage runtime dependencies;
- MoneyPrinterTurbo runtime dependencies;
- common AI libraries;
- large language runtimes.

Promotion MUST consider:

- image size;
- update frequency;
- security patch cadence;
- licensing;
- architecture compatibility;
- percentage of workloads using it;
- cold-start benefit.

Not every popular npm/pip dependency should be baked into the base image.

---

# 13. System project dependency rules

Managed projects such as OpenMontage and MoneyPrinterTurbo are system capabilities, not user dependencies.

They MUST NOT be writable shared folders across tenants.

Recommended model:

```text
/opt/codelocal/system-projects/
├── openmontage/          readonly/versioned
└── moneyprinterturbo/    readonly/versioned

/workspace/
└── user files            writable/isolated
```

If a system project requires writable runtime state, it must write to tenant/workspace-scoped paths.

---

# 14. Resource model

Resource scheduling MUST be based on active execution, not registered user count.

Example:

```text
1,000 registered users
       ↓
200 active in product
       ↓
50 executing tasks
       ↓
30–50 runtime sessions depending on reuse/concurrency
```

No architecture should allocate one permanent VM/container per account.

---

# 15. Resource profiles

Initial profiles SHOULD be simple and explicit.

```text
general-small
  CPU: 2
  RAM: 4 GB
  use: normal coding/file/shell

general-large
  CPU: 4
  RAM: 8 GB
  use: large builds/tests

browser
  CPU: 2–4
  RAM: 4–8 GB
  use: Chromium/Playwright

video-cpu
  CPU: 4–8
  RAM: 8–16 GB
  use: FFmpeg/Remotion/MPT/OpenMontage

gpu-ai
  GPU: provider-defined
  CPU/RAM: provider-defined
  use: GPU-required AI/render workloads
```

Profiles are policy objects, not hardcoded OpenSandbox instance types.

---

# 16. Quotas

CodeLocal MUST enforce at least:

- concurrent executions per user;
- concurrent runtime sessions per workspace;
- CPU-seconds/minutes or provider usage accounting;
- maximum runtime duration;
- workspace storage quota;
- media/storage quota;
- upload size limits;
- GPU usage quota;
- queue depth limits;
- abuse/rate limits.

Quota enforcement MUST happen before provider provisioning where possible.

---

# 17. Scheduler requirements

Initial scheduler queues:

```text
general
browser
video-cpu
gpu
```

A request MUST declare or resolve to a resource profile before provisioning.

Conceptual flow:

```text
Execution created
      ↓
Capability Resolver
      ↓
Resource Profile
      ↓
Quota check
      ↓
Queue
      ↓
Runtime session reuse/acquire
      ↓
RUNNING
```

## RSR-RES-001 — Queue instead of overload

When eligible compute is unavailable, jobs SHOULD enter a queue rather than causing uncontrolled provider creation or service crash.

## RSR-RES-002 — Fairness

A single user/workspace MUST NOT monopolize all available execution slots.

Initial fairness can use:

- per-user concurrency limits;
- per-plan concurrency limits;
- FIFO within priority class;
- aging to avoid starvation.

---

# 18. Runtime session reuse

Default session affinity key:

```text
tenant_id
+ user_id
+ workspace_id
+ runtime_profile
+ runtime_image_version
```

A healthy compatible session SHOULD be reused before creating a new sandbox.

Default idle TTL may start around the existing CodeLocal behavior of ~20 minutes and should remain configurable.

Reuse MUST NOT bypass:

- ownership validation;
- capability validation;
- image compatibility;
- dirty-state conflict detection;
- quota policy.

---

# 19. Idle lifecycle

Recommended initial lifecycle:

```text
PROVISIONING
READY
RUNNING
IDLE
DRAINING
PERSISTING
DESTROYING
DESTROYED
```

Additional recovery states:

```text
FAILED
RECOVERING
RECOVERY_REQUIRED
TIMED_OUT
CANCELLED
```

Idle timer resets on meaningful execution activity, not on passive UI polling.

---

# 20. Long-running work

Long-running jobs MUST not depend on a frontend connection remaining open.

Examples:

- long build;
- large test suite;
- video render;
- repository indexing;
- model download/preparation.

Execution state is durable in control plane.

Client reconnect flow:

```text
client reconnect
      ↓
fetch task/execution state
      ↓
attach to event stream
      ↓
continue progress UI
```

---

# 21. Stage-aware retries

Retry whole jobs only when safe.

Prefer stage-aware recovery.

Example media job:

```text
render succeeded
upload failed
```

Correct:

```text
retry upload
```

Incorrect:

```text
rerender entire video automatically
```

Execution stages SHOULD support idempotency keys and durable stage status.

---

# 22. User-installed packages

The user MAY install dependencies required by their workspace subject to security/resource policy.

Example:

```text
npm install axios
pip install opencv-python
uv add fastapi
```

The package manager changes durable manifests/lockfiles in the workspace.

The resulting installed environment is workspace-scoped.

A later sandbox can reconstruct it from:

```text
runtime image
+ lockfiles
+ shared package cache
```

This is the desired recovery contract.

---

# 23. Apt/system-level package requests

Arbitrary root-level package installation needs stricter policy.

Preferred order:

1. capability/runtime image already contains package;
2. user-space installation where supported;
3. ephemeral isolated runtime customization;
4. promote common package into future runtime image;
5. reject unsafe privileged host mutation.

Never grant tenant workloads permission to mutate the underlying sandbox host or container runtime.

---

# 24. Custom runtime environments — future extension

Future paid/enterprise feature may support workspace runtime manifests.

Example conceptual file:

```yaml
runtime:
  base: general-small
  node: 24
  python: 3.12
  capabilities:
    - browser
  packages:
    apt:
      - imagemagick
```

This manifest must compile to a safe, cached runtime environment.

It MUST NOT translate directly into unrestricted privileged Docker instructions.

---

# 25. Multi-tenant isolation

Mandatory isolation rules:

- each writable workspace belongs to one authorized tenant/workspace boundary;
- credentials are scoped and temporary;
- no shared writable home directory between tenants;
- no host filesystem mount;
- no Docker socket mount;
- no provider admin credentials inside workload containers;
- no arbitrary sandbox ID accepted without ownership validation;
- package cache contains no user secrets;
- private package credentials must never enter a globally shared cache key/value namespace.

---

# 26. Secret handling during dependency install

Private registries may require credentials.

Examples:

```text
NPM_TOKEN
GitHub package token
private pip registry credentials
```

Requirements:

- secrets injected only for the required process/session;
- never written to shared package caches;
- never logged;
- redact known secret patterns from stdout/stderr where practical;
- `.npmrc`, pip config or equivalent secret-bearing temporary files must be scoped and cleaned;
- local secret use is allowed without exposing the secret value to the model/user output.

---

# 27. Storage accounting

Usage SHOULD distinguish:

```text
canonical workspace storage
media storage
snapshots
cache storage
runtime ephemeral disk
```

Cache storage SHOULD NOT be charged to a single user when globally shared.

User quota SHOULD focus on user-owned durable bytes.

---

# 28. Garbage collection

GC categories:

## Workspace snapshots

Keep policy-controlled generations; remove obsolete snapshots after newer durable checkpoints exist.

## Media

Follow existing CodeLocal media retention/product rules.

## Package cache

LRU/size-based eviction is acceptable because cache is reconstructable.

## Runtime environments

Destroy after idle TTL unless warm-pool policy retains capacity.

## Failed temp artifacts

Delete after bounded retention unless required for debugging.

GC MUST never delete the only canonical copy of user data.

---

# 29. Runtime image rollout

Image upgrades MUST not force immediate destructive migration of all active sessions.

Recommended:

```text
v3 becomes default for new sessions
existing healthy v2 sessions continue until idle/drain
new acquire uses v3
```

If a workspace requires rebuilding dependencies under the new image, dependency fingerprint changes naturally trigger reconstruction.

Rollback MUST be possible by selecting prior known-good image version.

---

# 30. Observability

Track at minimum:

- active runtime sessions;
- provision latency;
- restore latency;
- dependency install latency;
- dependency cache hit rate;
- image pull latency;
- workspace snapshot latency;
- snapshot failures;
- dirty workspace count;
- queue depth by profile;
- CPU/GPU usage;
- runtime OOM count;
- disk exhaustion;
- job success/failure/retry;
- upload retry;
- cold-start vs warm-start ratio.

Critical alerts:

- persistence failure before destroy;
- ownership mismatch;
- repeated snapshot restore corruption;
- cache poisoning/integrity failure;
- high OOM rate;
- queue saturation;
- provider outage.

---

# 31. Cost-control principles

Cost optimization order:

1. reuse healthy runtime session;
2. use smallest eligible resource profile;
3. shared immutable cache;
4. prebuilt heavy runtime image;
5. idle destroy;
6. queue rather than uncontrolled scale;
7. GPU only when capability requires it;
8. later introduce warm pools based on real metrics.

Do NOT prematurely maintain large always-on warm pools.

---

# 32. MVP implementation phases

## Phase D0 — Durability contract

Goal: prove a deploy/runtime replacement cannot destroy user state.

Tasks:

- define canonical vs reconstructable paths/state;
- define workspace persistence interface;
- add dirty-state tracking contract;
- add persistence-before-destroy invariant tests;
- ensure runtime metadata is not authoritative only in memory.

Acceptance:

```text
create workspace
modify files
persist
kill runtime
recreate runtime
restore
files identical
```

## Phase D1 — Dependency layers

Tasks:

- define runtime image versions/profiles;
- dependency fingerprint;
- npm/pnpm cache adapter;
- pip/uv cache adapter;
- workspace-local environment materialization;
- cache integrity checks.

Acceptance:

```text
first npm/pip install = cold
second compatible install = cache hit
workspace A and B remain isolated
```

## Phase D2 — Resource profiles and scheduler

Tasks:

- profile resolver;
- queue classes;
- per-user concurrency;
- timeout/cancel;
- usage accounting;
- idle TTL reuse.

## Phase D3 — Heavy runtime images

Tasks:

- general runtime image;
- browser image;
- video-cpu image;
- image version pinning;
- rollout/rollback policy.

## Phase D4 — Recovery hardening

Tasks:

- simulate Railway API restart;
- simulate sandbox crash;
- simulate provider timeout;
- simulate upload failure;
- simulate cache eviction;
- restore task progress from durable state;
- stage-aware retry.

---

# 33. Required tests

## Durability

- Railway/control-plane process restart does not lose workspace metadata.
- Runtime destroy does not lose persisted file changes.
- Dirty workspace cannot be destroyed before successful persistence.
- Snapshot restore reproduces expected source state.
- Git state and uncommitted snapshot state do not overwrite each other silently.

## Dependency isolation

- two users can install different versions of same package;
- shared cache improves install without writable cross-user state;
- cache eviction causes reinstall, not correctness failure;
- private registry secret never appears in cache/log;
- dependency fingerprint changes after lockfile change.

## Runtime image

- pinned image version is recorded in runtime session;
- new image rollout does not break existing healthy session;
- rollback selects previous image;
- heavy profile contains expected capabilities.

## Scheduler/resource

- per-user concurrency enforced;
- jobs queue when pool exhausted;
- cancellation frees slot;
- idle runtime destroyed after persistence;
- GPU workload never lands on CPU-only profile.

## Recovery

- API restart reconnects to durable job state;
- sandbox crash creates replacement session;
- successful stage is not unnecessarily repeated;
- output upload retries independently from render stage.

---

# 34. Anti-patterns explicitly forbidden

Do NOT implement:

```text
Railway container filesystem = user storage
```

Do NOT implement:

```text
sandbox filesystem = canonical workspace
```

Do NOT implement:

```text
1 user = 1 always-on sandbox
```

Do NOT implement:

```text
all users share writable node_modules/.venv
```

Do NOT implement:

```text
install FFmpeg/Chromium/large system tool from Internet every task
```

Do NOT implement:

```text
shared cache contains user credentials
```

Do NOT implement:

```text
runtime image = mutable latest with no recorded version
```

Do NOT implement:

```text
idle destroy before dirty state persistence succeeds
```

---

# 35. Initial physical deployment recommendation

```text
RAILWAY
├── codelocal-api
├── codelocal-web
├── scheduler/worker control components
├── PostgreSQL
├── Redis/queue if selected
└── existing media/control services

EXTERNAL EXECUTION PLANE
└── OpenSandbox provider
    ├── general pool
    ├── browser pool
    ├── video CPU pool
    └── GPU pool later

DURABLE STORAGE
├── PostgreSQL metadata
├── Git source
├── object storage snapshots
└── existing CodeLocal media storage
```

Railway remains the default deployment/control-plane platform without becoming a hard dependency of runtime compute semantics.

---

# 36. Future provider portability

All durability/dependency/resource contracts in this document MUST continue to work if execution later moves from OpenSandbox to:

```text
CodeLocal-owned Kubernetes
managed VM provider
Railway future VM/compute offering
enterprise self-hosted cluster
regional GPU provider
```

Changing RuntimeProvider MUST NOT change Workspace identity or dependency semantics.

---

# 37. Definition of Done

This architecture is considered production-ready for the first Cloud Workspace release when all are true:

- a CodeLocal/Railway redeploy does not lose user workspace state;
- a sandbox can be destroyed and recreated with workspace recovery;
- user source/lockfiles persist independently of runtime;
- shared dependency cache works without writable cross-user sharing;
- heavy common tools are supplied by pinned runtime images;
- runtime image versions are observable and rollbackable;
- scheduler enforces concurrency/resource profiles;
- idle teardown persists dirty state before destroy;
- media output uses the existing CodeLocal media path;
- secrets do not leak into shared caches or logs;
- multi-user ownership is validated server-side;
- a user can resume work without caring that their previous sandbox disappeared.

Final product invariant:

> **The user owns a durable CodeLocal Workspace, not a fragile container. CodeLocal may rebuild, reschedule or replace compute freely while preserving the user's work and reconstructing dependencies efficiently.**
