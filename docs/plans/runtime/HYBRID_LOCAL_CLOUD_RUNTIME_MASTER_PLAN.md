# CodeLocal Hybrid Local + Cloud Runtime — Master Implementation Plan

Status: **P0/P1 strategic platform plan — Accepted product direction / implementation required**  
Date: **2026-08-26**  
Owner: **CodeLocal**  
Primary implementation: **Go backend/runtime + Next.js web + provider adapters**  
Related plans:

- `UNIVERSAL_AGENT_RUNTIME_PLAN.md`
- `SESSION_CONTINUITY_AND_RECOVERY_MASTER_PLAN.md`
- `SMART_COMPUTER_RUNTIME_PLAN.md`
- `../intelligence/PROJECT_BRAIN_MASTER_PLAN.md`
- `../../architecture/PRODUCT_STACK.md`
- `../../architecture/SECURITY_AND_PRIVACY.md`

> This document defines how CodeLocal adds a managed Cloud Sandbox runtime without removing or weakening the existing Local Runtime. The product target is one CodeLocal execution platform with interchangeable runtime locations and interchangeable AI models.

---

# 0. Executive decision

CodeLocal will become a **Hybrid Runtime Platform**.

The user can work through either:

```text
A. Local Runtime
   Mac / Windows / Linux owned by the user

B. CodeLocal Cloud Runtime
   managed isolated Linux development sandbox provisioned by CodeLocal

C. Hybrid / Auto
   CodeLocal chooses or the user pins the runtime per workspace/task
```

The existing Local Runtime remains a first-class product capability.

The Cloud Runtime is an addition, not a replacement.

The long-term product model is:

```text
AI model          = interchangeable reasoning worker
CodeLocal         = durable execution/workspace platform
Project Brain     = durable project intelligence
Runtime Provider  = where work executes
Git layer         = CodeLocal-managed project version-control identity/authorization
```

Primary strategic invariant:

> **CodeLocal owns the execution and workspace layer. AI models are replaceable workers.**

This allows CodeLocal to compete on durable engineering infrastructure rather than only model quality.

---

# 1. User promise

The default cloud experience should feel like:

```text
open ChatGPT / CodeLocal
        ↓
ask for work
        ↓
CodeLocal Cloud Computer is available
        ↓
repository/workspace opens automatically
        ↓
CodeLocal Runtime is already running
        ↓
AI reads / edits / tests / commits / pushes
```

The user should not need to understand:

```text
VM
Docker host
Railway
Daytona
runtime daemon
pairing
SSH
VNC
workspace socket
```

For Cloud Runtime, these are implementation details.

Local remains available for users who need:

- files that exist only on their own machine;
- Xcode/iOS simulator;
- Android devices;
- native desktop applications;
- private local services;
- powerful local hardware;
- existing local workflows;
- zero CodeLocal cloud-compute cost.

---

# 2. Three product cases

## Case A — Cloud-first user

Target: onboarding, mobile-only usage, casual developers, users who do not want a local daemon.

```text
User
  ↓
ChatGPT / CodeLocal Web
  ↓
CodeLocal Cloud
  ↓
Cloud Runtime Router
  ↓
Managed Sandbox
  ↓
CodeLocal Runtime preinstalled + auto-authenticated
  ↓
Git / Files / Shell / Process / Browser / Preview
```

Expected UX:

```text
Sign in
→ connect/select project
→ ask
→ work starts
```

No local computer is required.

## Case B — Local-first user

Target: existing CodeLocal developers and advanced/native workflows.

```text
User
  ↓
ChatGPT / CodeLocal Web
  ↓
CodeLocal Cloud control plane
  ↓
paired Local Runtime
  ↓
Mac / Windows / Linux
```

The current CodeLocal local runtime path remains supported.

## Case C — Hybrid / Auto user

Target: professional users and teams.

Example:

```text
Project A → Cloud Runtime
Project B → MacBook
Project C → Office PC
Project D → Cloud Runtime with Desktop capability
```

Possible future auto-routing:

```text
local device online + task needs Xcode
→ Local

local device offline + cloud-capable repo task
→ Cloud

simple read/edit/test task
→ cheapest eligible runtime

browser/GUI task
→ runtime with required visual capability
```

Auto routing must remain explainable and user-overridable.

---

# 3. Core architecture

```text
                         USER / CHATGPT / APPS
                                  |
                                  v
                         CodeLocal MCP / API
                                  |
                                  v
                         CodeLocal Control Plane
                                  |
                     +------------+-------------+
                     |                          |
                     v                          v
               Project Brain              Runtime Router
                                                |
                             +------------------+------------------+
                             |                                     |
                             v                                     v
                    LocalRuntimeProvider                  CloudRuntimeProvider
                             |                                     |
                    User Mac/PC/Linux                     Compute Orchestrator
                                                                  |
                                                +-----------------+----------------+
                                                |                                  |
                                                v                                  v
                                       Railway Sandbox/VM                    Daytona Sandbox
                                         (provider option)                   (provider option)
                                                |                                  |
                                                +-----------------+----------------+
                                                                  |
                                                                  v
                                                       CodeLocal Runtime Worker
                                                                  |
                                              Files / Git / Shell / Process
                                              Browser / Preview / Automation
```

Important:

- MCP public tool surface should remain coherent and should not double because Cloud is added;
- runtime location is routing metadata, not a separate set of tools;
- existing Go runtime/tool implementations should be reused whenever practical;
- provider-specific SDKs must stay behind a provider interface;
- Cloud Sandbox is a managed execution host, not a second CodeLocal product.

---

# 4. Logical computer vs physical VM

Do **not** implement:

```text
1 account = 1 VM running 24/7
```

Implement:

```text
1 account = entitlement to a logical CodeLocal Cloud Computer
```

Compute is allocated only when needed.

Conceptual lifecycle:

```text
NONE
  ↓ first cloud task
PROVISIONING
  ↓
READY / RUNNING
  ↓ idle
CHECKPOINTING
  ↓
SLEEPING / STOPPED
  ↓ next task
RESTORING
  ↓
RUNNING
```

The user experiences one persistent computer/workspace.

Infrastructure may create/destroy/restore underlying compute instances as needed.

---

# 5. Runtime abstraction

Introduce a canonical runtime-location/provider boundary.

Conceptual Go contract:

```go
type RuntimeProvider interface {
    ID() string
    Capabilities(ctx context.Context, target RuntimeTarget) (RuntimeCapabilities, error)

    Ensure(ctx context.Context, req EnsureRuntimeRequest) (RuntimeHandle, error)
    Status(ctx context.Context, handle RuntimeHandle) (RuntimeStatus, error)
    Wake(ctx context.Context, handle RuntimeHandle) (RuntimeHandle, error)
    Suspend(ctx context.Context, handle RuntimeHandle) error
    Destroy(ctx context.Context, handle RuntimeHandle) error

    Exec(ctx context.Context, handle RuntimeHandle, req ExecRequest) (ExecResult, error)
    OpenSession(ctx context.Context, handle RuntimeHandle, req SessionRequest) (SessionHandle, error)
}
```

Do not force Local Runtime to pretend it is a vendor sandbox API.

A better internal layering may be:

```text
RuntimeTarget
  location = local | cloud

RuntimeRouter
  selects target

Local Runtime Transport
  existing WebSocket/device path

Cloud Compute Provider
  provisions execution host

Canonical Workspace Runtime
  existing CodeLocal tool engine where reusable
```

Exact interfaces should be finalized after a short code-spike to avoid unnecessary abstraction.

---

# 6. Cloud compute provider boundary

Create a provider-neutral compute layer.

Suggested ownership:

```text
internal/cloudcompute/
    provider.go
    types.go
    orchestrator.go
    lifecycle.go
    quota.go
    capacity.go
    credentials.go
    errors.go

internal/cloudcompute/providers/
    railway/
    daytona/
```

If package-size/dependency review suggests another location, preserve the domain boundary even if the physical package differs.

Provider interface should expose primitives such as:

```text
create sandbox
restore/fork/checkpoint
start/stop
exec bootstrap command
read provider status
obtain preview/port route
obtain terminal/desktop capability metadata
collect resource usage
terminate
```

Provider APIs must not leak into MCP handlers, dashboard route code, Project Brain, Git policy, or agent adapters.

---

# 7. Provider strategy

## Railway

Use Railway as a strong candidate when:

- integration simplicity matters;
- CodeLocal control plane already runs there;
- Railway Sandbox/VM primitives provide required lifecycle and isolation;
- pricing/performance is acceptable for the workload;
- beta maturity satisfies production gates.

## Daytona

Use Daytona as a strong candidate when:

- fast dev-sandbox provisioning is superior;
- pause/archive/snapshot lifecycle is more economical;
- VNC/Desktop/Computer Use primitives materially reduce implementation work;
- per-user active compute pricing is better;
- provider reliability meets requirements.

## Decision

Do **not** hard-code the product to either provider.

MVP should implement one production provider and one adapter contract that can support the second.

Recommended evaluation:

```text
Provider benchmark
- cold boot latency
- restore latency
- API reliability
- per-hour CPU/RAM price
- storage price
- egress price
- snapshot/checkpoint behavior
- Docker support
- preview URL support
- VNC/Desktop support
- region availability
- rate limits
- tenant isolation
- auditability
```

Initial product preference:

```text
Control Plane: Railway remains acceptable/current
Compute Plane: benchmark Railway Sandbox vs Daytona before locking MVP
```

---

# 8. Cloud Runtime bootstrap

Every cloud sandbox must boot from a CodeLocal-owned image/template.

Conceptual image:

```text
codelocal-cloud-runtime
├── CodeLocal Go runtime
├── git
├── openssh/client tooling where needed
├── Node.js
├── npm/pnpm
├── Python
├── Go
├── common build tooling
├── Docker client/runtime only where provider supports safe nested/container use
├── Playwright dependencies optional/on-demand
└── health/bootstrap entrypoint
```

Do not install every large SDK into the base image.

Use layers/capabilities:

```text
Base
  Files + Git + Shell + Process + common languages

Dev
  Docker/build extras

Browser
  Chromium/Playwright

Desktop
  X server / compositor + VNC/noVNC or provider-native VNC
```

The runtime process should start automatically.

User does not run:

```bash
codelocal .
```

inside a managed Cloud Sandbox.

---

# 9. Managed cloud runtime identity

Cloud sandbox identity must not use the manual local pairing UX.

Introduce a distinct managed runtime credential model.

Conceptual identity:

```text
RuntimeIdentity
- runtimeId
- userId
- logicalComputerId
- workspaceId/projectId
- provider
- providerInstanceId
- credentialId
- credential public key
- createdAt
- expires/rotatesAt
- revokedAt
```

Cloud bootstrap receives a short-lived bootstrap credential or signed claim.

Then:

```text
sandbox boots
→ exchanges bootstrap claim
→ receives runtime credential
→ registers to CodeLocal Cloud
→ workspace becomes routable
```

Never bake a reusable global CodeLocal secret into the sandbox image.

Never expose provider API credentials to the user sandbox.

---

# 10. Device/workspace model evolution

Current model is strongly shaped around:

```text
user → device → workspace
```

Extend without destroying compatibility.

Recommended conceptual model:

```text
ExecutionNode
- id
- userId
- kind: local | cloud
- displayName
- provider?        // cloud only
- providerRef?     // cloud only
- status
- capabilities
- lastSeenAt
```

Existing local Device may remain the persistence model during migration, but product logic should stop assuming every execution node is a physical user device.

Cloud may initially appear as:

```text
☁ CodeLocal Cloud
```

Local continues to appear as:

```text
💻 Mong's MacBook
🖥 Office PC
```

Do not force Cloud into manual pairing semantics merely to reuse a table.

---

# 11. Workspace model

A logical project can have multiple runtime bindings.

Conceptual:

```text
Project
  ├── Cloud Workspace
  ├── MacBook Workspace
  └── Office PC Workspace
```

Add a canonical runtime binding:

```text
WorkspaceRuntimeBinding
- userId
- projectId
- workspaceId
- executionNodeId
- runtimeKind
- provider?
- state
- preferred/primary
- lastUsedAt
```

Do not silently switch a mutating task between materially different filesystem states.

Runtime switching requires repository-state reconciliation.

---

# 12. Git architecture — mandatory CodeLocal-managed identity

Product requirement:

> **Projects executed through CodeLocal Cloud use CodeLocal-managed Git identity and authorization. They do not depend on the user's local Git credentials.**

Separate:

```text
Commit identity
from
Repository authorization
```

## Commit identity

Default cloud commit identity can be:

```text
Author/Committer policy managed by CodeLocal
```

Possible models:

### Model 1 — CodeLocal as author and committer

```text
Author: CodeLocal
Committer: CodeLocal
```

Simple but weak attribution to the requesting human.

### Model 2 — User attribution + CodeLocal committer — recommended long term

```text
Author: verified CodeLocal user identity
Committer: CodeLocal
```

This preserves human intent attribution while making execution provenance explicit.

### Model 3 — CodeLocal bot identity plus signed metadata

Useful for organization/team workflows where repository policy prefers a bot account/app identity.

Final commit identity policy should be configurable at organization/project level while remaining CodeLocal-managed.

## Repository authorization

Recommended:

```text
Sandbox
  ↓ short-lived repo credential
CodeLocal Git Broker
  ↓ authorization check
GitHub App installation / scoped token
  ↓
Repository
```

Do not inject a long-lived global PAT into sandboxes.

Do not copy the user's local `~/.gitconfig`, SSH private keys, GitHub CLI credentials, or credential helper database into Cloud Runtime.

## Required Git capabilities

```text
clone
fetch
status
diff
branch/worktree
commit
push
PR creation later
```

Mutating Git actions remain subject to CodeLocal policy/approval/product configuration.

---

# 13. Git Broker

Introduce a CodeLocal Git authorization service/domain.

Conceptual responsibilities:

```text
resolve repo identity
verify user/org entitlement
verify GitHub App installation scope
issue short-lived credential
bind credential to repo + operation + runtime
rotate/revoke
record safe audit metadata
```

Suggested token properties:

```text
short TTL
repository-scoped
minimum permissions
runtime-bound where practical
not persisted in Project Brain
not logged
```

Cloud sandbox should request credentials only when needed.

No permanent repository secret in checkpoint images.

---

# 14. Repository bootstrap flow

First cloud use:

```text
User selects/connects repository
        ↓
CodeLocal validates repository authorization
        ↓
Ensure Cloud Runtime
        ↓
Request short-lived Git credential
        ↓
Clone into canonical workspace path
        ↓
Detect project identity
        ↓
Project Brain context attaches
        ↓
Runtime registers ready
```

Returning use:

```text
Restore cloud workspace
        ↓
validate filesystem snapshot
        ↓
refresh Git authorization
        ↓
fetch/reconcile remote
        ↓
resume task/workspace
```

Do not assume a restored checkpoint's old Git credential remains valid.

---

# 15. Cloud filesystem persistence

Separate persistent project state from disposable compute.

Required categories:

```text
A. Git canonical source state
   remote repository + commits

B. Active uncommitted workspace state
   must survive normal sleep/restore according to plan

C. Dependency/build caches
   disposable / regenerable / quota-bound

D. secrets
   injected dynamically; must not become snapshots unless explicitly safe

E. Project Brain
   durable cloud metadata, independent of sandbox disk
```

Storage strategy may use provider checkpoint/snapshot first, then evolve to explicit persistent volume/object-storage patterns if needed.

Acceptance requirement:

> Sleeping a normal cloud workspace must not silently lose uncommitted user work.

---

# 16. Secrets model

Never use sandbox persistence as a secret vault.

Cloud Runtime secrets should be delivered through scoped secret injection.

Sources may include:

```text
CodeLocal-managed project secrets
Git short-lived tokens
provider/model session token if explicitly supported
user/org integration secrets
```

Requirements:

- encrypted at rest in the appropriate secret store;
- scoped by user/org/project/runtime;
- never copied into Project Brain;
- redacted from logs;
- excluded from checkpoint/export where provider semantics require it;
- revocable;
- available only to authorized execution.

Do not automatically import local `.env` secrets into Cloud Runtime.

---

# 17. Security boundary

Cloud Sandbox user code is untrusted relative to CodeLocal infrastructure.

Required boundary:

```text
Cloud Sandbox
  CAN access:
    own workspace
    approved network destinations
    CodeLocal runtime APIs required for work
    repository through scoped Git credentials

  CANNOT access:
    CodeLocal production database directly
    Redis directly
    provider control-plane API key
    other users' sandboxes
    other users' workspaces
    CodeLocal signing/bootstrap master secrets
```

Do not place user sandbox on a trusted private network with unrestricted access to CodeLocal production services.

Prefer brokered APIs with scoped runtime identity.

---

# 18. Sandbox privileges

Within the sandbox, the user/agent may need broad developer rights.

Allowed inside isolation boundary can include:

```text
create/delete files inside workspace
run compilers/tests
install project dependencies
start dev servers
use Docker where safely supported
run Chromium/Playwright when enabled
bind preview ports through managed proxy
```

Host-level/production privileges remain forbidden.

The phrase `root in sandbox` must never imply `root on provider host` or CodeLocal infrastructure access.

---

# 19. Network policy

Default Cloud Runtime network policy should be explicit.

Possible initial mode:

```text
outbound internet: allowed with abuse controls
inbound: denied except managed preview/terminal/desktop proxy
CodeLocal internal: only scoped public/service endpoints
production DB/private infra: denied
metadata/provider control endpoints: denied where possible
```

Later enterprise controls:

```text
allowlist domains
block public internet
private package registry
organization egress gateway
fixed egress IP
```

---

# 20. Capability-on-demand

Do not run every expensive feature all the time.

Canonical capabilities:

```text
filesystem
git
shell
pty
process
lsp
browser_headless
preview
container_build
desktop_gui
computer_use
gpu       // future
```

Start minimal.

Example:

```text
read/edit/test backend task
→ no GUI

frontend visual verification
→ start Chromium headless

user wants interactive browser/desktop
→ start GUI/VNC capability

finish task / idle
→ tear down browser/desktop processes
```

---

# 21. GUI strategy

Cloud Runtime must support user interaction, but full Desktop should **not** be the default MVP surface.

## GUI Level 1 — Workspace UI — MVP default

Next.js product surface:

```text
Files
Editor/read-only or lightweight edit
Terminal
Processes/logs
Git status/diff
Preview
Runtime status
Agent activity
```

This gives mobile/browser users direct visibility without running a Linux desktop stack.

## GUI Level 2 — Web preview — MVP important

When a dev server listens on a port:

```text
runtime port
→ CodeLocal preview gateway/provider preview URL
→ authenticated preview
```

Required:

- user/workspace authorization;
- non-guessable/signed routes;
- lifecycle cleanup;
- WebSocket support where frameworks require it;
- no public exposure by default.

## GUI Level 3 — Full desktop — later/on-demand

```text
Linux desktop
Chromium GUI
File manager/terminal where useful
VNC/noVNC or provider-native desktop
Computer Use
```

Only start when requested/needed.

---

# 22. Web workspace product surface

Suggested route family:

```text
/dashboard/workspaces/[id]
```

Conceptual layout:

```text
┌──────────────────────────────────────────────────────────┐
│ Project Name                ☁ CodeLocal Cloud • Running │
├───────────────┬───────────────────────────┬──────────────┤
│ Files         │ Editor / Preview          │ Agent        │
│               │                           │ Activity     │
├───────────────┴───────────────────────────┴──────────────┤
│ Terminal / Logs / Processes / Git                       │
└──────────────────────────────────────────────────────────┘
```

Do not clone a full desktop IDE before proving the simple workspace surface.

The primary user promise is AI execution, not replacing VS Code feature-for-feature.

---

# 23. Runtime selection UX

Keep simple.

Example:

```text
Run on

☁ CodeLocal Cloud     Ready
💻 My MacBook          Online
🖥 Office PC           Offline
```

Per-project preference:

```text
Default runtime
- Cloud
- Local device X
- Auto
```

Suggested onboarding default:

```text
Cloud = available without setup
Local = optional connection for additional capabilities
```

Do not force a new user to install the local client before they can experience CodeLocal.

---

# 24. Runtime Router

Introduce deterministic routing before smart routing.

Inputs:

```text
explicit user selection
project runtime preference
runtime online/status
required capability
repository availability/state
cost/quota
platform requirement
security requirement
```

Hard gates first.

Example:

```text
requires Xcode
→ eligible: macOS local only unless future macOS cloud exists

requires cloud-only repo + local offline
→ cloud

requires user's private local file
→ local
```

Do not route based only on cheapest cost if it would use a stale/different working tree.

---

# 25. Runtime state reconciliation

Switching runtime location may change filesystem state.

Before mutation after a switch:

```text
resolve project/repository identity
compare branch
compare HEAD
fetch remote
inspect uncommitted changes
inspect task continuation state
```

Possible outcomes:

```text
SAFE_TO_CONTINUE
SYNC_REQUIRED
LOCAL_ONLY_UNCOMMITTED_WORK
CLOUD_ONLY_UNCOMMITTED_WORK
BRANCH_DIVERGED
REMOTE_DIVERGED
CONFLICT
```

Never silently overwrite one runtime's uncommitted work with another runtime's state.

---

# 26. Session continuity integration

Cloud Runtime must reuse the continuity principles from `SESSION_CONTINUITY_AND_RECOVERY_MASTER_PLAN.md`.

Important identity distinction:

```text
MCP session       ephemeral
runtime instance  replaceable
logical computer  durable
workspace         durable logical binding
project           durable
continuation/task durable enough to resume safely
```

A provider sandbox being replaced must not imply a new CodeLocal project/task identity.

Cloud wake/restore is a normal continuity event.

---

# 27. Runtime lifecycle orchestration

Suggested cloud runtime state machine:

```text
UNALLOCATED
  ↓ ensure
QUEUED
  ↓ provider create/restore
PROVISIONING
  ↓ bootstrap
REGISTERING
  ↓ runtime connected
READY
  ↓ first active task
RUNNING
  ↓ idle threshold
IDLE
  ↓
CHECKPOINTING
  ↓
SUSPENDED
  ↓ request
RESTORING
  ↓
REGISTERING
  ↓
READY
```

Terminal states:

```text
FAILED
DESTROYED
QUARANTINED
```

Recovery must distinguish provider failure from user code failure.

---

# 28. Lazy provisioning

Do not create a sandbox at signup.

Signup should create:

```text
account
entitlement
logical cloud-computer record optional
```

First actual cloud execution request triggers compute.

This prevents inactive accounts from creating compute cost.

---

# 29. Warm pool

Cold boot latency can damage first-use UX.

Optional optimization after measuring baseline:

```text
small pool of prebuilt unattached runtimes
        ↓
claim atomically
        ↓
rotate credentials
        ↓
attach user/project
        ↓
ready
```

Warm pool must never reuse another user's filesystem/secrets.

Use pristine base snapshot/template or cryptographically/operationally verified reset semantics.

---

# 30. Idle policy

Suggested initial policy for MVP benchmarking, not a hard final number:

```text
active process / interactive terminal / agent running
→ RUNNING

no activity for ~15–30 minutes
→ suspend/checkpoint candidate

long inactivity
→ archive/destroy disposable compute while retaining required persistent workspace state
```

Do not suspend while:

```text
build is running
long test is running
dev server is explicitly pinned
user has active terminal session
foreground agent task is active
```

Billing/plan may allow longer pinning.

---

# 31. Quota and cost control

Cloud compute is an entitlement, not unlimited free infrastructure.

Track:

```text
active_runtime_seconds
vCPU_seconds
memory_GB_seconds
storage_GB_days
egress_bytes
browser_seconds
desktop_seconds
preview_seconds
provider cost estimate
```

Budget controls:

```text
per-user monthly cloud allowance
per-org allowance
max concurrent runtimes
max vCPU/RAM
idle timeout
max pinned runtime duration
max storage
abuse/rate limits
```

Local Runtime does not consume CodeLocal compute quota.

---

# 32. Suggested product tiers

Exact prices are a business decision, but architecture should support:

```text
Free
- Local Runtime
- small Cloud allowance
- 1 cloud runtime
- aggressive idle sleep

Pro
- Local Runtime
- larger Cloud allowance
- better CPU/RAM
- more storage
- longer active windows

Team/Business
- shared organization projects
- policy controls
- larger quotas
- audit
- Git organization integration
- private package/network options later
```

Do not promise unlimited cloud compute without an economic model.

---

# 33. Usage/billing integration

Reuse existing usage domain rather than building a parallel billing meter.

New usage categories may include:

```text
cloud.runtime.active_ms
cloud.runtime.cpu_ms
cloud.runtime.memory_gb_ms
cloud.storage.gb_hours
cloud.network.egress_bytes
cloud.browser.active_ms
cloud.desktop.active_ms
```

Provider-reported usage is evidence; CodeLocal should also maintain its own lifecycle/event measurements.

Reconcile billing records asynchronously.

Do not block tool execution on slow billing analytics unless entitlement/quota is actually exhausted.

---

# 34. Cloud runtime database model

Conceptual tables/entities; exact migration should follow current Store conventions.

## `codelocal_execution_nodes`

```text
node_id
user_id
kind                  local | cloud
name
provider nullable
provider_ref nullable
status
capabilities jsonb
created_at
last_seen_at
revoked_at nullable
```

## `codelocal_cloud_runtimes`

```text
runtime_id
user_id
node_id
provider
provider_instance_id nullable
state
image_version
region nullable
cpu_millis nullable
memory_mb nullable
checkpoint_ref nullable
last_active_at
suspended_at nullable
created_at
updated_at
```

## `codelocal_workspace_runtime_bindings`

```text
user_id
project_id
workspace_id
node_id
runtime_id nullable
kind
is_preferred
last_used_at
```

## `codelocal_runtime_leases`

Used to prevent duplicate provisioning/wake.

```text
lease_id
runtime_id
holder
operation
expires_at
```

## `codelocal_runtime_usage`

Provider-neutral normalized compute usage events/rollups.

## Git-specific entities

Prefer separate bounded tables/domains for:

```text
Git provider installation metadata
repo authorization metadata
token issuance audit metadata
commit identity policy
```

Never store raw access tokens in ordinary metadata tables.

---

# 35. API surface

Suggested backend APIs; naming may adapt to existing conventions.

```text
GET  /api/runtime/nodes
GET  /api/runtime/cloud/status
POST /api/runtime/cloud/ensure
POST /api/runtime/cloud/wake
POST /api/runtime/cloud/suspend
POST /api/runtime/cloud/destroy

GET  /api/projects/:id/runtime-bindings
POST /api/projects/:id/runtime-preference

GET  /api/workspaces/:id/preview
POST /api/workspaces/:id/preview/ensure

GET  /api/workspaces/:id/terminal-sessions
POST /api/workspaces/:id/terminal-sessions

GET  /api/git/installations
GET  /api/git/repositories
POST /api/git/repositories/:id/connect
```

MCP does not need separate Cloud versions of filesystem/Git/shell tools.

---

# 36. MCP routing contract

Current public tools should continue routing to an authorized workspace.

Enhance internal routing metadata with:

```text
runtimeKind
executionNodeId
runtimeId nullable
provider nullable
wakeAllowed
restoreAllowed
```

If Cloud workspace is sleeping:

```text
MCP tool request
→ resolve workspace
→ ensure/wake runtime
→ wait within bounded activation budget
→ dispatch existing tool
```

The model should receive a structured temporary state if activation exceeds a safe synchronous budget rather than a generic `tool disabled` failure.

---

# 37. Activation latency UX

States should be explicit:

```text
Preparing cloud computer…
Restoring workspace…
Starting CodeLocal Runtime…
Syncing repository…
Ready
```

Do not expose provider-specific jargon by default.

Fast path:

```text
already running
→ immediate
```

Restore path should target user-perceived latency that is competitive with normal SaaS app loading.

Measure before setting strict SLA.

---

# 38. Cloud health and recovery

Provider/runtime failures:

```text
PROVIDER_API_UNAVAILABLE
PROVISION_TIMEOUT
BOOTSTRAP_FAILED
RUNTIME_REGISTER_TIMEOUT
CHECKPOINT_FAILED
RESTORE_FAILED
RESOURCE_LIMIT
QUOTA_EXCEEDED
IMAGE_INCOMPATIBLE
GIT_AUTH_FAILED
WORKSPACE_STATE_DIVERGED
```

Recovery examples:

```text
runtime process crashed but VM alive
→ restart runtime

VM disappeared, valid checkpoint exists
→ restore replacement

checkpoint corrupt but Git clean
→ rebuild from Git + Project Brain

uncommitted workspace cannot be restored
→ do not pretend success; surface recovery state
```

---

# 39. Cloud image/version rollout

Managed sandboxes must report:

```text
runtime version
image version
protocol version
provider adapter version
```

Rollout strategy:

```text
canary image
→ internal test users
→ small percentage new runtimes
→ restore compatibility verification
→ wider rollout
```

Existing sleeping checkpoints may run older images.

Define upgrade behavior explicitly:

```text
resume old image temporarily
or
migrate workspace to new runtime
```

Never silently invalidate uncommitted state for image upgrade.

---

# 40. Cloud Runtime and Universal Agent Runtime

These plans complement each other.

```text
Universal Agent Runtime
  decides which AI/coding engine works

Hybrid Runtime
  decides where the execution environment lives
```

Matrix:

```text
             Local        Cloud
Codex          ✓            ✓ if supported
Claude         ✓            ✓ if supported
OpenCode       ✓            ✓
Future model   ✓            ✓
```

Do not couple `engine == cloud` or `engine == local`.

Agent Engine and Runtime Location are orthogonal dimensions.

---

# 41. Cloud Runtime and Project Brain

Project Brain remains logical-project-centric, not VM-centric.

A project may move:

```text
MacBook
→ Cloud
→ Office PC
→ Cloud
```

without losing:

```text
project identity
decisions
rules
verified experience
learned skills
knowledge graph
routing history
```

Raw local source privacy rules continue to apply for Local Runtime.

Cloud Runtime necessarily stores/executes source in CodeLocal-managed compute and therefore requires a distinct transparent privacy policy and user agreement.

`SECURITY_AND_PRIVACY.md` must be updated before production Cloud launch.

---

# 42. Privacy model split

The product must clearly distinguish:

## Local project

```text
source execution boundary = user's device
```

## Cloud project

```text
source execution boundary = CodeLocal-managed isolated compute
```

The dashboard/onboarding should state this plainly.

Do not retain old messaging such as “source always stays on your computer” once a user explicitly chooses Cloud Runtime.

Instead:

```text
Local mode: source remains on your authorized device.
Cloud mode: source is cloned/executed in your isolated CodeLocal Cloud workspace.
```

---

# 43. Marketplace / app-store impact

Cloud Runtime should become the default reviewer/demo path because it removes local setup friction.

Reviewer flow:

```text
install/connect CodeLocal app
→ sign in
→ open prepared/reviewer project or authorized repository
→ Cloud Runtime starts automatically
→ test tools/workflow
```

This does not remove security/approval requirements.

It improves:

```text
first-run reliability
review reproducibility
mobile/browser usability
no local daemon dependency
no local environment dependency
```

Keep Local Runtime as an optional advanced capability.

---

# 44. Observability

Events:

```text
runtime.route.selected
runtime.cloud.ensure.requested
runtime.cloud.provision.started
runtime.cloud.provision.completed
runtime.cloud.bootstrap.started
runtime.cloud.registered
runtime.cloud.ready
runtime.cloud.idle
runtime.cloud.checkpoint.started
runtime.cloud.checkpoint.completed
runtime.cloud.suspended
runtime.cloud.restore.started
runtime.cloud.restore.completed
runtime.cloud.destroyed
runtime.cloud.failed
runtime.cloud.quota.blocked
runtime.git.credential.issued
runtime.git.credential.revoked
runtime.preview.started
runtime.preview.stopped
runtime.desktop.started
runtime.desktop.stopped
```

Metrics:

```text
cloud_first_task_success_rate
cold_start_ms
restore_ms
time_to_first_tool_ms
runtime_registration_ms
cloud_task_success_rate
cloud_tool_latency_ms
cloud_runtime_active_seconds
cloud_cost_per_active_user
cloud_cost_per_verified_task
idle_suspend_success_rate
restore_success_rate
checkpoint_failure_rate
provider_error_rate
runtime_rebuild_rate
git_auth_failure_rate
preview_start_ms
desktop_start_ms
```

Product metric:

```text
new_user_to_first_successful_code_task
```

This should materially improve compared with mandatory local installation/pairing.

---

# 45. Cost benchmark

Before public Free-tier launch, run controlled benchmarks for representative profiles.

## Profile L — Light

```text
1 vCPU / 1–2 GB RAM
30 minutes/day
simple repo edits/tests
no desktop
```

## Profile D — Developer

```text
2 vCPU / 4 GB
1–2 hours/day
build/test/dev server
occasional browser
```

## Profile P — Power

```text
4 vCPU / 8 GB or more
heavy build/container
long sessions
```

Measure:

```text
monthly compute per active user
storage
network
snapshot/checkpoint
provider overhead
warm-pool waste
browser/desktop uplift
```

Do not choose provider from headline per-vCPU price alone.

---

# 46. Abuse controls

Cloud compute can be abused for mining, scanning, proxying, botting, or unrelated hosting.

Required controls:

```text
account/risk rate limits
runtime time quota
CPU/memory quota
network abuse detection
port exposure only through managed preview
no unrestricted permanent public server on free tier
concurrency limits
provider-level abuse signals
manual/automatic suspension path
```

Do not weaken legitimate developer workflows with overly broad command regexes; prefer infrastructure-level quotas and network controls where practical.

---

# 47. Data deletion

User must be able to remove Cloud Runtime state.

Delete semantics:

```text
Delete Cloud Workspace
→ revoke runtime credentials
→ stop/destroy compute
→ delete checkpoints/volumes according to retention policy
→ delete runtime-specific caches
→ revoke Git ephemeral credentials
→ keep/delete Project Brain according to explicit project deletion choice
```

Account deletion must include cloud compute/storage cleanup.

---

# 48. Backup/recovery

Do not claim checkpoints are backups unless verified.

For important state:

```text
Git remote = committed source durability
Project Brain DB = durable project intelligence
workspace checkpoint = active environment continuity
```

Uncommitted work requires explicit persistence guarantee.

Periodically test restore from checkpoint/snapshot for active cloud workspaces.

---

# 49. Implementation phases

## HCR0 — Architecture freeze + provider benchmark

Tasks:

- document Runtime Location vs Agent Engine distinction;
- inspect current device/workspace/runtime assumptions;
- map code paths that assume `project_root` is local;
- benchmark Railway vs Daytona primitives;
- define Cloud Runtime security threat model;
- define Git managed-identity policy;
- define storage/uncommitted-work guarantee;
- define minimal GUI MVP.

Exit gate:

- one selected MVP compute provider;
- provider-neutral interfaces accepted;
- security and Git credential model accepted;
- no code assumes Cloud needs manual local pairing.

## HCR1 — Canonical execution-node/runtime model

Implement:

- execution node kind local/cloud;
- runtime binding types;
- persistence migrations;
- API read model;
- backward-compatible projection of existing local devices/workspaces.

Exit gate:

- existing Local Runtime works unchanged;
- backend can represent a Cloud execution node without fake manual pairing.

## HCR2 — Cloud compute provider interface + fake provider

Implement:

- provider contract;
- lifecycle orchestrator;
- fake deterministic provider;
- provisioning lease/idempotency;
- state machine tests;
- quota hooks.

Exit gate:

- fake cloud runtime can progress through provision → ready → suspend → restore → destroy deterministically.

## HCR3 — First real compute provider

Implement the selected provider adapter.

Requirements:

- create/restore/suspend/destroy;
- bootstrap command/image;
- provider status mapping;
- safe error taxonomy;
- resource labels/tags for cleanup and audit;
- provider credentials only server-side.

Exit gate:

- backend can provision a sandbox and execute a health command without manual provider dashboard steps.

## HCR4 — Managed CodeLocal Runtime bootstrap

Implement:

- cloud runtime image/template;
- bootstrap claim exchange;
- managed runtime credential;
- auto registration;
- health/heartbeat;
- runtime version reporting;
- restart behavior.

Exit gate:

```text
ensure cloud runtime
→ sandbox starts
→ CodeLocal runtime registers automatically
→ no user pairing/local CLI required
```

## HCR5 — Cloud workspace filesystem + Git read path

Implement:

- repository connection selection;
- CodeLocal Git Broker foundation;
- short-lived repository credentials;
- clone/fetch;
- project identity discovery;
- files/search/read/status/diff through existing canonical tool engine where feasible.

Exit gate:

- ChatGPT can open/read/search a repo through Cloud Runtime with no local computer online.

## HCR6 — Cloud mutation + managed Git identity

Implement:

- file edits;
- shell/process;
- test/build;
- CodeLocal-managed commit identity;
- commit/push through scoped authorization;
- Git audit metadata;
- approval/security gates.

Exit gate:

```text
user asks fix
→ cloud edits
→ tests
→ commit
→ push
```

without using the user's local Git configuration or credentials.

## HCR7 — Persistence + idle suspend/restore

Implement:

- uncommitted workspace persistence;
- checkpoint/snapshot abstraction;
- idle detection;
- suspend;
- restore;
- Git credential refresh after restore;
- runtime re-registration;
- continuity reconciliation.

Exit gate:

- user leaves with uncommitted work, returns later, and safely resumes it.

## HCR8 — Runtime Router + Hybrid UX

Implement:

- local/cloud runtime selection;
- project preference;
- user override;
- capability gates;
- structured runtime-unavailable states;
- local remains fully functional.

Exit gate:

- same CodeLocal tool surface can target either local or cloud workspace intentionally.

## HCR9 — Web Workspace GUI Level 1

Implement:

- workspace status;
- files;
- terminal/process UI;
- Git status/diff;
- basic runtime controls;
- agent activity;
- mobile-responsive layout.

Exit gate:

- user can inspect/control a Cloud Runtime from browser/mobile without SSH/VNC.

## HCR10 — Authenticated Preview

Implement:

- dev-server port discovery/selection;
- secure preview routing;
- authentication;
- lifecycle cleanup;
- WebSocket support;
- preview status in dashboard.

Exit gate:

- frontend app started in Cloud Runtime is viewable safely from CodeLocal UI.

## HCR11 — Capability-on-demand browser

Implement:

- Playwright/Chromium enablement only when needed;
- resource metering;
- cleanup on task completion/idle;
- browser verification integration.

Exit gate:

- UI tasks can run browser verification without keeping Chromium alive for unrelated tasks.

## HCR12 — Full Desktop/VNC optional capability

Implement only if benchmark/user need justifies it.

Requirements:

- authenticated VNC/noVNC or provider-native equivalent;
- desktop process lifecycle;
- input/session security;
- resource quota;
- Computer Use integration where safe.

Exit gate:

- user can open an interactive cloud desktop when needed, while normal users do not pay its idle cost.

## HCR13 — Cost/quota production controls

Implement:

- normalized provider usage;
- per-plan quota;
- concurrency limits;
- suspend policies;
- admin cost dashboard;
- abuse controls;
- provider cleanup reconciliation.

Exit gate:

- CodeLocal can prove bounded monthly exposure for Free and Pro plans.

## HCR14 — Marketplace/reviewer cloud path

Implement:

- deterministic reviewer/demo cloud workspace;
- first-run flow;
- clear privacy disclosures;
- no local daemon requirement for reviewer scenario;
- acceptance test pack.

Exit gate:

- reviewer can complete representative CodeLocal workflow from ChatGPT without installing local runtime.

## HCR15 — Second compute provider / provider failover readiness

Implement second provider only after first path is stable and economics justify it.

Exit gate:

- provider-specific code remains isolated;
- same lifecycle contract passes for both;
- migration/rebuild path is documented.

---

# 50. Recommended implementation order

```text
HCR0 architecture + provider benchmark
  ↓
HCR1 execution-node model
  ↓
HCR2 provider interface + fake provider
  ↓
HCR3 first real provider
  ↓
HCR4 managed CodeLocal bootstrap
  ↓
HCR5 Cloud read path + Git Broker
  ↓
HCR6 mutation + CodeLocal Git identity
  ↓
HCR7 persistence + sleep/restore
  ↓
HCR8 Hybrid router/UX
  ↓
HCR9 Workspace GUI
  ↓
HCR10 Preview
  ↓
HCR11 browser on demand
  ↓
HCR13 quota/cost production controls
  ↓
HCR14 marketplace reviewer path
  ↓
HCR12 full desktop only if justified
  ↓
HCR15 second provider
```

Important ordering decision:

> Do not start by building a full Linux desktop. First prove headless Cloud CodeLocal execution, Git, persistence, hybrid routing and cost controls.

---

# 51. First MVP slice

Smallest convincing product slice:

```text
1. Cloud execution-node model
2. provider interface + selected provider
3. CodeLocal cloud runtime base image
4. managed auto-auth/bootstrap
5. one Cloud project/repository
6. CodeLocal-managed Git read authorization
7. Files + Search + Shell + Process
8. Edit + test
9. managed Git commit + push
10. idle checkpoint/suspend
11. restore on next tool request
12. Cloud/Local selector
13. basic web status + terminal + Git diff
14. usage/cost instrumentation
```

Explicitly postpone from first MVP:

```text
full Linux desktop
GPU
multi-provider failover
complex IDE editor
enterprise private networking
large warm pool
learned runtime routing
unlimited background servers
```

---

# 52. Acceptance tests — MVP

Required end-to-end scenarios:

1. New user with no local client can trigger a cloud task.
2. First cloud task provisions/claims runtime exactly once under concurrent requests.
3. Cloud runtime self-registers without pairing UI.
4. User can connect an authorized repository without local Git credentials.
5. Git credential issued to sandbox is repository-scoped and short-lived.
6. User can read/search files.
7. User can edit files and run tests.
8. User can commit/push using CodeLocal-managed Git identity.
9. Sandbox cannot directly access CodeLocal production DB/Redis/provider API credentials.
10. Two users cannot access each other's runtime/workspace.
11. Idle runtime suspends according to policy.
12. Uncommitted work survives normal suspend/restore.
13. Restored runtime receives fresh Git authorization instead of reusing stale token.
14. User can keep using Local Runtime exactly as before.
15. One account can have both Cloud and Local execution nodes.
16. Runtime switch with divergent branch/uncommitted state returns explicit reconciliation result.
17. Cloud runtime provider outage returns structured failure, not generic `tool disabled`.
18. Usage/cost events are recorded for active compute.
19. Cloud runtime is destroyed/revoked correctly when user deletes workspace.
20. Reviewer can complete a representative workflow with no local daemon.

---

# 53. Security acceptance tests

Required:

```text
cross-tenant filesystem attempt blocked
cross-tenant runtime-ID enumeration does not grant access
bootstrap token replay rejected
revoked runtime credential rejected
provider API secret absent from sandbox
Git token TTL/scope verified
Git token redacted from logs
checkpoint does not persist expired Git credential as usable auth
metadata-service/provider-control endpoint blocked where applicable
preview URL requires correct authorization
terminal cannot reach production DB directly
runtime cannot register as another user/project
cloud tool dispatch preserves approval/security policy
```

Threat model must include malicious repository code and prompt injection originating from repository contents.

---

# 54. Failure/chaos tests

Test:

```text
provider create returns timeout after actually creating sandbox
runtime registration response lost
backend restarts during provisioning
Redis lease expires during slow provision
sandbox dies during git clone
sandbox dies with uncommitted work
checkpoint API succeeds but response is lost
restore produces runtime with stale image
GitHub token expires during push
network disappears during push
Cloud reconnect occurs during long build
quota becomes exhausted during idle pinned runtime
provider returns duplicate/reused instance reference
```

Idempotency and reconciliation are mandatory.

---

# 55. Performance targets to measure

Do not invent final SLOs before benchmark, but instrument:

```text
ensure-to-runtime-ready
restore-to-runtime-ready
first-MCP-tool latency
repo clone latency
workspace resume latency
preview start latency
browser start latency
```

Optimization order:

```text
1. base image size
2. restore path
3. repository/dependency cache strategy
4. warm pool if justified
5. region placement
```

---

# 56. Product positioning

Do not position CodeLocal primarily as:

```text
"19 MCP tools"
"a local bridge"
"a wrapper around ChatGPT"
```

Long-term positioning:

> **CodeLocal is the development computer and durable engineering environment for AI.**

Possible concise promise:

> **A computer for your AI.**

or

> **Give your AI a development computer.**

The product moat is the combined durable layer:

```text
Execution
Workspace state
Git
Project Brain
Knowledge
Permissions
Verification
Runtime routing
Cloud + Local computers
Agent/model routing
```

Model quality can change without forcing the user to abandon this environment.

---

# 57. Decision log — 2026-08-26

## HCRD1 — Local remains first-class

Cloud is added; Local is not removed.

## HCRD2 — Cloud is default-on-demand, not always-on

User should experience an available cloud computer without CodeLocal funding one permanent VM per account.

## HCRD3 — Cloud Runtime self-starts CodeLocal

Managed sandbox must boot and register CodeLocal automatically. User should not run the local pairing/onboarding flow inside managed cloud compute.

## HCRD4 — Provider-neutral compute boundary

Railway, Daytona and future providers are infrastructure choices, not product identities.

## HCRD5 — CodeLocal-managed Git

Cloud Git identity/authorization is managed by CodeLocal and does not rely on user's local Git setup.

## HCRD6 — Git authorization is scoped and short-lived

Do not put a global CodeLocal PAT or user local Git credentials inside sandboxes.

## HCRD7 — GUI is layered

Files/Terminal/Git/Preview first; full desktop only on demand.

## HCRD8 — Capability-on-demand controls cost

Browser/Desktop/container-heavy capabilities run only when tasks need them.

## HCRD9 — Agent Engine and Runtime Location are independent

GPT/Claude/OpenCode/etc. are workers; Local/Cloud are execution locations.

## HCRD10 — Project Brain survives runtime changes

Durable project intelligence belongs to logical project identity, not sandbox instance.

## HCRD11 — Cloud source has a distinct privacy disclosure

Local source remains local; Cloud projects intentionally place source in isolated CodeLocal-managed compute.

## HCRD12 — Marketplace path should prefer Cloud

Cloud makes first-run/reviewer workflows reproducible without local installation, while Local remains an advanced/optional runtime.

---

# 58. What NOT to do

Avoid:

- one always-running VM for every signup;
- creating a Railway Project/Environment per user as the primary tenant abstraction;
- removing Local Runtime;
- duplicating MCP tools into `cloud_*` and `local_*` variants;
- coupling cloud product logic directly to Railway/Daytona SDK objects;
- baking provider control-plane secrets into the image;
- storing a global Git PAT in every sandbox;
- copying user local Git credentials into Cloud;
- assuming checkpoints are backups without restore tests;
- losing uncommitted work on idle cleanup;
- starting Chromium/VNC/Desktop for every runtime;
- building a full VS Code competitor before core execution works;
- silently switching between divergent local/cloud working trees;
- placing untrusted sandbox on unrestricted CodeLocal production private network;
- using model/provider name as runtime location;
- storing secrets in Project Brain;
- claiming “source always stays local” for cloud projects;
- enabling free unlimited background hosting.

---

# 59. Definition of success

The architecture is successful when a user can hold this simple mental model:

```text
I use CodeLocal.

If I do nothing, CodeLocal can give me a cloud development computer.
If I want, I can connect my own Mac/Windows/Linux machine too.
My projects, Git history, Project Brain, rules and verified experience stay under one CodeLocal account.
I can use GPT, Claude or another model without rebuilding my development environment.
```

Representative Cloud flow:

```text
User on phone asks ChatGPT:
"Fix the login bug and push it."

        ↓
CodeLocal resolves project
        ↓
Cloud runtime sleeping/not allocated
        ↓
ensure/restore sandbox
        ↓
CodeLocal runtime auto-registers
        ↓
Git Broker grants scoped repo access
        ↓
Project Brain supplies context
        ↓
selected agent/model works
        ↓
files changed + tests run
        ↓
CodeLocal verification
        ↓
managed commit/push
        ↓
Experience recorded
        ↓
idle → checkpoint/suspend
```

Representative Local flow remains:

```text
User selects MacBook workspace
        ↓
existing CodeLocal Local Runtime
        ↓
normal local files/Git/shell/browser/computer workflow
```

Both flows use one product, one project identity and one durable engineering brain.

---

# 60. Immediate next actions

1. Accept this plan as the source of truth for Hybrid Runtime.
2. Run HCR0 provider benchmark: Railway Sandbox vs Daytona.
3. Produce a concrete threat model for managed Cloud Runtime.
4. Inspect current database migrations/device/workspace assumptions for HCR1.
5. Define `ExecutionNode`, `CloudRuntime`, `RuntimeBinding` contracts.
6. Define GitHub App/Git Broker managed Git flow.
7. Build fake compute provider and lifecycle tests before any provider SDK integration.
8. Implement one real provider only after fake-provider state machine passes.
9. Build managed CodeLocal cloud image/bootstrap.
10. Prove one repo read/edit/test/commit/push end to end.
11. Add idle suspend/restore with uncommitted-state preservation.
12. Add Local/Cloud selector in web dashboard.
13. Add Files/Terminal/Git/Preview GUI before full Desktop.
14. Measure actual per-active-user cloud cost before setting Free/Pro quotas.
15. Update `SECURITY_AND_PRIVACY.md`, user guide and marketplace submission pack before public rollout.

---

# 61. Maintenance rule

Any future change to CodeLocal runtime architecture must answer:

```text
Does Local still work?
Does Cloud still work without local setup?
Is provider-specific logic isolated?
Is Git authorization still scoped/short-lived?
Can uncommitted work survive normal lifecycle transitions?
Are secrets excluded from Project Brain/checkpoints/logs?
Is runtime switching reconciled safely?
Is the feature metered/cleanup-safe?
Does the user need to understand infrastructure jargon? (prefer no)
```

If a proposed implementation violates these invariants, update this decision document explicitly before changing production behavior.