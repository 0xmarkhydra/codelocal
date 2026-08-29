# CodeLocal Hybrid Local + Cloud Runtime — Master Implementation Plan

Status: **P0/P1 strategic platform plan — accepted product direction / implementation required**  
Date: **2026-08-26**  
Owner: **CodeLocal**  
Primary implementation: **Go backend/runtime + Next.js web + provider adapters**

> This is the source of truth for adding a managed Cloud Runtime without removing or weakening CodeLocal's existing Local Runtime.

---

# 1. Executive decision

CodeLocal becomes a **Hybrid Runtime Platform** with three supported product cases:

```text
A. Local Runtime
   Mac / Windows / Linux owned by the user

B. CodeLocal Cloud Runtime
   isolated managed development sandbox provisioned on demand

C. Hybrid / Auto
   user pins a runtime or CodeLocal selects an eligible runtime per workspace/task
```

The Cloud Runtime is an addition, not a replacement.

Strategic invariant:

> **CodeLocal owns the execution/workspace layer. AI models are replaceable reasoning workers.**

Long-term product stack:

```text
AI model          = GPT / Claude / Gemini / OpenCode / future models
CodeLocal         = durable execution/workspace platform
Project Brain     = durable project intelligence
Runtime Router    = chooses Local or Cloud execution
Git layer         = CodeLocal-managed repository identity/authorization
Compute provider  = Railway / Daytona / future provider
```

---

# 2. User promise

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

A Cloud user should not need to understand or manually operate:

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

Local remains first-class for Xcode/iOS, Android devices, native apps, private local files/services, powerful personal hardware, and zero CodeLocal compute cost.

---

# 3. Product cases

## Case A — Cloud-first

```text
User
  ↓
ChatGPT / CodeLocal Web
  ↓
CodeLocal Control Plane
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

No personal computer must remain online.

## Case B — Local-first

```text
User
  ↓
ChatGPT / CodeLocal Web
  ↓
CodeLocal Control Plane
  ↓
paired Local Runtime
  ↓
Mac / Windows / Linux
```

Current CodeLocal local execution remains supported.

## Case C — Hybrid / Auto

Example:

```text
Project A → Cloud Runtime
Project B → MacBook
Project C → Office PC
Project D → Cloud Runtime with Desktop capability
```

Routing examples:

```text
requires Xcode
→ Local macOS

local offline + repo task
→ Cloud

simple read/edit/test
→ cheapest eligible runtime

browser/GUI task
→ runtime with visual capability
```

Auto routing must always remain explainable and user-overridable.

---

# 4. Core architecture

```text
                     USER / CHATGPT / APPS
                              │
                              ▼
                     CodeLocal MCP / API
                              │
                              ▼
                    CodeLocal Control Plane
                              │
                  ┌───────────┴───────────┐
                  ▼                       ▼
             Project Brain           Runtime Router
                                          │
                         ┌────────────────┴────────────────┐
                         ▼                                 ▼
               Local Runtime Provider            Cloud Runtime Provider
                         │                                 │
                User Mac/PC/Linux                 Compute Orchestrator
                                                           │
                                               ┌───────────┴───────────┐
                                               ▼                       ▼
                                          Railway option          Daytona option
                                               │                       │
                                               └───────────┬───────────┘
                                                           ▼
                                                  CodeLocal Runtime
                                                           │
                                          Files / Git / Shell / Process
                                          Browser / Preview / Automation
```

Rules:

- do not duplicate MCP tools into `local_*` and `cloud_*` variants;
- runtime location is routing metadata;
- provider SDK objects never leak into MCP handlers or Project Brain;
- reuse the canonical Go execution/tool engine where practical;
- Local remains independent of cloud-compute availability.

---

# 5. Logical computer, not permanent VM

Never implement:

```text
1 account = 1 VM running 24/7
```

Implement:

```text
1 account = entitlement to a logical CodeLocal Cloud Computer
```

Compute is allocated only when needed.

Lifecycle:

```text
UNALLOCATED
  ↓ first cloud task
PROVISIONING
  ↓
READY / RUNNING
  ↓ idle
CHECKPOINTING
  ↓
SUSPENDED
  ↓ next task
RESTORING
  ↓
READY / RUNNING
```

The user experiences a persistent computer/workspace while underlying compute can be replaced, checkpointed or suspended.

---

# 6. Runtime abstraction

Canonical concepts:

```text
RuntimeTarget
- location: local | cloud
- executionNodeId
- workspaceId/projectId
- required capabilities

RuntimeRouter
- resolves explicit user preference
- validates capability/platform requirements
- evaluates runtime state and quota
- never silently overwrites divergent workspace state

Local Runtime Transport
- existing authenticated WebSocket/device path

Cloud Compute Provider
- provisions and manages isolated execution hosts
```

Provider-neutral conceptual interface:

```go
type ComputeProvider interface {
    ID() string
    Ensure(ctx context.Context, req EnsureRequest) (Handle, error)
    Status(ctx context.Context, handle Handle) (Status, error)
    Wake(ctx context.Context, handle Handle) (Handle, error)
    Suspend(ctx context.Context, handle Handle) error
    Destroy(ctx context.Context, handle Handle) error
    Exec(ctx context.Context, handle Handle, req ExecRequest) (ExecResult, error)
}
```

Do not force Local Runtime to pretend it is a vendor sandbox API.

---

# 7. Provider strategy

CodeLocal must remain provider-neutral.

## Railway candidate

Strengths:

- existing CodeLocal infrastructure already uses Railway;
- simple operational integration;
- Sandbox/VM primitives can support managed development compute.

Risks:

- sandbox/VM maturity and pricing must be benchmarked;
- do not create one Railway Project/Environment per user.

## Daytona candidate

Strengths:

- purpose-built development sandboxes;
- pause/archive/snapshot lifecycle;
- VNC/Desktop/Computer Use primitives can reduce implementation work;
- potentially better active-compute economics.

## HCR0 provider benchmark

Measure:

```text
cold boot latency
restore latency
API reliability
CPU/RAM cost
storage cost
egress cost
checkpoint semantics
Docker support
preview URL support
VNC/Desktop support
regions
rate limits
isolation
auditability
```

Select one MVP provider, but keep adapters swappable.

---

# 8. Managed Cloud Runtime image

Base image/template:

```text
codelocal-cloud-runtime
├── CodeLocal runtime
├── git
├── Node.js
├── npm/pnpm
├── Python
├── Go
├── common build tools
└── health/bootstrap entrypoint
```

Capabilities are layered and started on demand:

```text
Base
  Files + Git + Shell + Process

Dev
  Docker/build extras

Browser
  Chromium/Playwright

Desktop
  GUI + VNC/noVNC or provider-native desktop
```

The CodeLocal runtime process starts automatically. A managed Cloud user never runs `codelocal .` or manual pairing inside the sandbox.

---

# 9. Managed runtime identity

Cloud sandboxes use managed runtime credentials, not the local pairing UX.

Conceptual identity:

```text
RuntimeIdentity
- runtimeId
- userId
- logicalComputerId
- project/workspace binding
- provider
- providerInstanceId
- credentialId/public key
- createdAt
- rotation/expiry
- revokedAt
```

Bootstrap flow:

```text
sandbox boots
→ exchanges one-time/short-lived bootstrap claim
→ receives runtime credential
→ registers with CodeLocal Cloud
→ workspace becomes routable
```

Never bake reusable global secrets or provider API keys into the sandbox image.

---

# 10. Execution node and workspace model

Evolve the current `user → device → workspace` model into a generic execution-node model without breaking Local compatibility.

```text
ExecutionNode
- id
- userId
- kind: local | cloud
- displayName
- provider/providerRef when cloud
- status
- capabilities
- lastSeenAt
```

A project can bind multiple runtimes:

```text
Project
├── ☁ CodeLocal Cloud
├── 💻 MacBook
└── 🖥 Office PC
```

Workspace binding concept:

```text
WorkspaceRuntimeBinding
- userId
- projectId
- workspaceId
- executionNodeId
- runtimeKind
- provider
- state
- preferred
- lastUsedAt
```

Do not force a Cloud Runtime through manual pairing merely to reuse a local-device table.

---

# 11. CodeLocal-managed Git — mandatory

Product requirement:

> **Git operations executed through CodeLocal Cloud use CodeLocal-managed identity and authorization. They never depend on the user's local Git configuration or credentials.**

Separate commit attribution from repository authorization.

Recommended long-term attribution:

```text
Author    = verified CodeLocal user who requested the work
Committer = CodeLocal
```

Repository authorization:

```text
Sandbox
  ↓ short-lived repo credential
CodeLocal Git Broker
  ↓ user/org/repository authorization check
GitHub App installation / scoped token
  ↓
Repository
```

Never copy into cloud sandboxes:

```text
user ~/.gitconfig credentials
SSH private keys
GitHub CLI credentials
long-lived user PAT
global CodeLocal PAT
```

Git Broker responsibilities:

```text
resolve repository identity
validate user/org entitlement
validate GitHub App installation scope
issue short-lived minimum-permission token
bind issuance to repo/runtime/operation where practical
rotate/revoke
record safe audit metadata
```

Required capabilities:

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

Mutating actions remain governed by CodeLocal policy/approval configuration.

---

# 12. Repository bootstrap

First Cloud use:

```text
User connects/selects repository
→ validate repository authorization
→ ensure Cloud Runtime
→ issue short-lived Git credential
→ clone canonical workspace
→ detect project identity
→ attach Project Brain
→ runtime reports ready
```

Returning use:

```text
restore workspace
→ validate persisted filesystem state
→ issue fresh Git authorization
→ fetch/reconcile remote
→ resume task
```

Never assume a checkpoint's old Git credential remains valid.

---

# 13. Persistence model

Separate disposable compute from durable project state.

```text
A. committed source
   Git remote

B. active uncommitted workspace
   must survive normal suspend/restore

C. dependency/build caches
   disposable and quota-bound

D. secrets
   injected dynamically, not treated as workspace persistence

E. Project Brain
   durable cloud metadata independent of sandbox disk
```

Acceptance invariant:

> Normal idle cleanup must never silently lose uncommitted user work.

Provider checkpoints/snapshots can be the MVP mechanism, but CodeLocal must verify restore semantics rather than assuming checkpoints are backups.

---

# 14. Security boundary

Treat repository/user code inside Cloud Runtime as untrusted relative to CodeLocal infrastructure.

Cloud sandbox may access:

```text
its own workspace
approved internet destinations
scoped CodeLocal runtime APIs
repository through short-lived Git authorization
```

It must not directly access:

```text
CodeLocal production DB
Redis
provider control-plane credentials
other users' sandboxes/workspaces
CodeLocal master signing/bootstrap secrets
```

Prefer brokered APIs over placing untrusted sandboxes on unrestricted production private networks.

Inside the isolation boundary, developer permissions may be broad enough to:

```text
create/delete workspace files
install project dependencies
run compilers/tests
start dev servers
use Docker when safely supported
run browser tooling when enabled
```

`root inside sandbox` must never mean host or CodeLocal-infrastructure root.

---

# 15. Secrets model

Cloud Runtime secrets are dynamically injected and scoped by user/org/project/runtime.

Requirements:

- encrypted at rest in the appropriate secret store;
- short-lived where possible;
- redacted from logs;
- never written to Project Brain;
- excluded from reusable base images;
- revocable;
- not automatically imported from a user's local `.env`.

Git credentials are requested only when needed and should not be considered durable workspace state.

---

# 16. Network policy

Initial Cloud Runtime policy:

```text
outbound internet: allowed with abuse controls
inbound: denied except managed preview/terminal/desktop proxy
CodeLocal APIs: scoped endpoints only
production DB/private infra: denied
provider control endpoints: denied where possible
```

Future enterprise controls:

```text
domain allowlists
public-internet disable
private package registry
organization egress gateway
fixed egress IP
```

---

# 17. Capability-on-demand

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
gpu // future
```

Examples:

```text
backend edit/test
→ no GUI

frontend visual verification
→ start headless browser

user requests interactive desktop
→ start Desktop capability

finished/idle
→ tear expensive capabilities down
```

Do not keep Chromium, VNC or desktop processes running for users who do not need them.

---

# 18. GUI strategy

## Level 1 — Workspace UI — MVP default

Next.js surface:

```text
Files
lightweight Editor/read view
Terminal
Processes/logs
Git status/diff
Runtime status
Agent activity
```

## Level 2 — Authenticated Web Preview — MVP important

```text
dev server port
→ CodeLocal/provider preview gateway
→ authenticated signed route
→ user browser/mobile
```

Must support lifecycle cleanup, authorization, non-guessable routes and WebSockets where required.

## Level 3 — Full Desktop — later/on-demand

```text
Linux desktop
Chromium GUI
VNC/noVNC or provider-native desktop
Computer Use
```

Do not build a full VS Code/Desktop replacement before core headless execution is proven.

---

# 19. Runtime selection UX

Keep it simple:

```text
Run on

☁ CodeLocal Cloud     Ready
💻 My MacBook          Online
🖥 Office PC           Offline
```

Per-project default:

```text
Cloud
Local device X
Auto
```

Onboarding default should make Cloud usable immediately while Local is offered as an additional capability.

---

# 20. Runtime routing and state reconciliation

Router inputs:

```text
explicit user selection
project preference
runtime availability
required capability/platform
repository/workspace state
quota/cost
security requirements
```

Before mutating after a runtime switch, reconcile:

```text
repository identity
branch
HEAD
remote state
uncommitted changes
continuation/task state
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

Never silently overwrite divergent Local/Cloud working trees.

---

# 21. Session continuity

Identity hierarchy:

```text
MCP session       = ephemeral
provider instance = replaceable
logical computer  = durable
workspace         = durable logical binding
project           = durable
Project Brain     = durable intelligence
```

A sandbox replacement must not create a new logical project/task identity.

Wake/restore is a normal continuation event, not a new workspace.

---

# 22. Lifecycle orchestration

State machine:

```text
UNALLOCATED
→ QUEUED
→ PROVISIONING
→ REGISTERING
→ READY
→ RUNNING
→ IDLE
→ CHECKPOINTING
→ SUSPENDED
→ RESTORING
→ REGISTERING
→ READY
```

Terminal/recovery states:

```text
FAILED
DESTROYED
QUARANTINED
```

Do not suspend while a foreground agent task, build, test, pinned dev server or active terminal session is running.

Suggested initial idle benchmark window: approximately 15–30 minutes, then tune from measured usage/cost.

---

# 23. Lazy provisioning and warm pool

Do not allocate a VM at signup.

Signup creates account/entitlement/logical cloud-computer metadata only.

First actual Cloud execution triggers compute.

After measuring cold-start latency, CodeLocal may keep a small pristine warm pool:

```text
prebuilt unattached runtime
→ atomically claim
→ rotate credentials
→ attach project/user
→ ready
```

A warm instance must never retain another tenant's filesystem or secrets.

---

# 24. Cost and quota controls

Meter normalized resource usage:

```text
active_runtime_seconds
vCPU_seconds
memory_GB_seconds
storage_GB_days
egress_bytes
browser_seconds
desktop_seconds
preview_seconds
provider_cost_estimate
```

Controls:

```text
monthly user/org allowance
concurrent runtime limit
CPU/RAM ceiling
idle timeout
max pinned runtime duration
storage limit
abuse/rate limits
```

Local Runtime consumes no CodeLocal compute quota.

Architecture should support product tiers such as:

```text
Free
- Local Runtime
- small Cloud allowance
- aggressive sleep

Pro
- larger Cloud allowance
- better CPU/RAM/storage
- longer active windows

Team/Business
- organization policies
- larger quota
- Git org integration
- audit/private networking later
```

Never market unlimited cloud compute without a sustainable cost model.

---

# 25. Database/domain model

Conceptual entities:

```text
ExecutionNode
- node_id
- user_id
- kind local|cloud
- name
- provider/provider_ref
- status
- capabilities
- last_seen_at

CloudRuntime
- runtime_id
- user_id
- node_id
- provider
- provider_instance_id
- state
- image_version
- region
- resource allocation
- checkpoint_ref
- last_active_at

WorkspaceRuntimeBinding
- user_id
- project_id
- workspace_id
- node_id
- runtime_id
- kind
- preferred
- last_used_at

RuntimeLease
- prevents duplicate ensure/wake/provision operations

RuntimeUsage
- provider-neutral usage events/rollups
```

Git provider installation metadata, repository authorization and commit identity policy belong to a bounded Git domain. Raw access tokens must not live in ordinary metadata tables.

---

# 26. API/MCP direction

Suggested control APIs:

```text
GET  /api/runtime/nodes
GET  /api/runtime/cloud/status
POST /api/runtime/cloud/ensure
POST /api/runtime/cloud/wake
POST /api/runtime/cloud/suspend
POST /api/runtime/cloud/destroy

GET  /api/projects/:id/runtime-bindings
POST /api/projects/:id/runtime-preference

GET/POST /api/workspaces/:id/preview
GET/POST /api/workspaces/:id/terminal-sessions

GET  /api/git/installations
GET  /api/git/repositories
POST /api/git/repositories/:id/connect
```

MCP filesystem/Git/shell tools remain one coherent public surface.

Cloud-sleeping dispatch path:

```text
MCP request
→ resolve workspace
→ ensure/wake Cloud Runtime
→ bounded activation wait
→ dispatch canonical tool
```

If activation cannot complete within a safe synchronous budget, return structured temporary state instead of generic `tool disabled`.

---

# 27. Failure taxonomy and recovery

Structured failures:

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
runtime process crashed, VM alive
→ restart runtime

VM disappeared, checkpoint valid
→ restore replacement

checkpoint invalid, Git state clean
→ rebuild from Git + Project Brain

uncommitted workspace cannot be recovered
→ surface explicit recovery state; never claim success
```

Provision/wake/checkpoint flows must be idempotent because provider operations can succeed while their API response is lost.

---

# 28. Project Brain and model independence

Project Brain is logical-project-centric, not machine-centric.

A project can move:

```text
MacBook → Cloud → Office PC → Cloud
```

without losing project identity, rules, decisions, verified experience, learned skills or knowledge graph.

Agent Engine and Runtime Location are orthogonal:

```text
             Local   Cloud
GPT worker      ✓       ✓
Claude worker   ✓       ✓
OpenCode        ✓       ✓
Future model    ✓       ✓
```

Do not encode `model == runtime` or `provider == product` assumptions.

---

# 29. Privacy model

The UI and policies must clearly distinguish:

```text
Local mode
source execution boundary = user's authorized device

Cloud mode
source execution boundary = isolated CodeLocal-managed compute
```

Do not claim “source always stays on your computer” for a project the user intentionally runs in Cloud.

Update `SECURITY_AND_PRIVACY.md` before public Cloud launch.

---

# 30. Marketplace/reviewer path

Cloud becomes the default first-run/reviewer path:

```text
install/connect CodeLocal
→ sign in
→ select prepared/authorized repository
→ Cloud Runtime auto-starts
→ run representative workflow
```

This removes local-daemon setup as a prerequisite while preserving Local as an advanced first-class capability.

The marketplace product should be positioned as a development computer/execution environment, not merely “19 MCP tools”.

Possible promise:

> **A computer for your AI.**

---

# 31. Observability

Core events:

```text
runtime.route.selected
runtime.cloud.ensure.requested
runtime.cloud.provision.started/completed
runtime.cloud.bootstrap.started
runtime.cloud.registered
runtime.cloud.ready
runtime.cloud.idle
runtime.cloud.checkpoint.started/completed
runtime.cloud.suspended
runtime.cloud.restore.started/completed
runtime.cloud.destroyed
runtime.cloud.failed
runtime.cloud.quota.blocked
runtime.git.credential.issued/revoked
runtime.preview.started/stopped
runtime.desktop.started/stopped
```

Core metrics:

```text
cold_start_ms
restore_ms
time_to_first_tool_ms
cloud_first_task_success_rate
cloud_task_success_rate
cloud_cost_per_active_user
cloud_cost_per_verified_task
idle_suspend_success_rate
restore_success_rate
checkpoint_failure_rate
provider_error_rate
git_auth_failure_rate
preview_start_ms
desktop_start_ms
```

Key product metric:

```text
new_user_to_first_successful_code_task
```

---

# 32. Abuse controls

Cloud compute can be abused for mining, scanning, proxies, botting or unrelated permanent hosting.

Required controls:

```text
runtime time quota
CPU/memory quota
concurrency limit
network abuse detection
managed port exposure only
no unrestricted permanent server on free tier
account/risk rate limits
provider abuse signals
manual/automatic suspension path
```

Prefer infrastructure quotas/network boundaries over brittle command-regex blocking for legitimate developer workflows.

---

# 33. CI/CD ownership boundary

Hybrid Runtime work must preserve component-owned CI/CD.

Repository zones:

```text
Runtime / CLI
→ CodeLocal CI
→ local runtime, native helpers, npm packaging/release

Cloud backend
→ CodeLocal Cloud CI
→ codelocal-cloud + cloud dependency region + backend container/Railway

Web
→ CodeLocal Web CI
→ web/** + Dockerfile.web + railway.web.json

Docs/plans
→ no build/deploy by default
```

Shared dependencies may intentionally trigger more than one zone when both binaries truly import them.

Release invariant:

> A Cloud-only or Web-only change must never cause an npm/native CodeLocal release merely because it landed on `main`.

Railway source deploys must use component watch patterns so a docs/web/local-only push does not rebuild the backend service.

---

# 34. Implementation phases

## HCR0 — Architecture freeze + provider benchmark

- map current device/workspace/runtime assumptions;
- benchmark Railway vs Daytona;
- define threat model;
- define Git managed identity;
- define persistence guarantee;
- define minimum GUI MVP.

Exit: one MVP provider selected and provider-neutral contract accepted.

## HCR1 — Execution node/runtime model

- execution node local/cloud;
- runtime binding types;
- persistence migrations;
- backward-compatible Local projection.

Exit: backend represents Cloud without fake manual pairing; Local remains unchanged.

## HCR2 — Compute provider contract + fake provider

- provider interface;
- lifecycle orchestrator;
- provisioning lease/idempotency;
- deterministic fake provider;
- state-machine tests;
- quota hooks.

Exit: provision → ready → suspend → restore → destroy passes deterministically.

## HCR3 — First real provider

- create/restore/suspend/destroy;
- bootstrap execution;
- provider status/error mapping;
- resource tagging for cleanup/audit.

Exit: backend provisions an isolated sandbox and runs a health command without manual dashboard operations.

## HCR4 — Managed CodeLocal bootstrap

- cloud runtime image/template;
- bootstrap claim exchange;
- managed credential;
- auto-registration;
- heartbeat/version reporting;
- restart behavior.

Exit:

```text
ensure cloud runtime
→ sandbox starts
→ CodeLocal registers automatically
→ no local CLI/pairing
```

## HCR5 — Cloud repository + Git read path

- repository connection;
- Git Broker foundation;
- short-lived authorization;
- clone/fetch;
- project identity;
- Project Brain attach;
- Files/Search/Read/Git status/diff.

Exit: ChatGPT reads/searches a repo with no personal computer online.

## HCR6 — Mutation + managed Git identity

- edits;
- shell/process;
- tests/build;
- CodeLocal-managed commit identity;
- scoped commit/push;
- audit/policy gates.

Exit:

```text
ask fix
→ edit
→ test
→ commit
→ push
```

without local Git credentials.

## HCR7 — Persistence + idle suspend/restore

- uncommitted-state persistence;
- checkpoint abstraction;
- idle detection;
- suspend/restore;
- fresh Git authorization after restore;
- runtime re-registration.

Exit: uncommitted work safely resumes after normal idle sleep.

## HCR8 — Hybrid Runtime Router + UX

- Cloud/Local selection;
- project preference;
- capability gates;
- explicit reconciliation states;
- structured unavailable/wake states.

Exit: same CodeLocal tools intentionally target either runtime location.

## HCR9 — Workspace GUI Level 1

- runtime status;
- Files;
- Terminal/process UI;
- Git status/diff;
- Agent activity;
- mobile-responsive shell.

Exit: Cloud workspace is inspectable/controlable from browser/mobile without VNC.

## HCR10 — Authenticated Preview

- port discovery/selection;
- signed/authenticated preview routes;
- WebSocket support;
- lifecycle cleanup.

Exit: a web app running in Cloud Runtime is safely viewable from CodeLocal UI.

## HCR11 — Browser on demand

- Playwright/Chromium capability;
- resource metering;
- cleanup on task completion/idle.

Exit: visual verification works without permanent browser cost.

## HCR12 — Full Desktop optional

Only if user demand/economics justify it:

- VNC/noVNC or provider-native GUI;
- Computer Use;
- secure input/session handling;
- quota/lifecycle controls.

## HCR13 — Production cost/quota controls

- normalized provider usage;
- plan quotas;
- concurrency/resource limits;
- admin cost dashboard;
- orphan cleanup/reconciliation;
- abuse controls.

Exit: bounded monthly exposure is measurable for Free/Pro.

## HCR14 — Marketplace/reviewer Cloud path

- deterministic reviewer project;
- first-run flow;
- privacy disclosure;
- no local daemon requirement;
- acceptance pack.

Exit: representative reviewer workflow completes entirely from ChatGPT/web.

## HCR15 — Second provider readiness

Add second provider only after first provider is stable and economics justify it.

Exit: same lifecycle contract passes for both and provider-specific code stays isolated.

---

# 35. MVP implementation order

```text
HCR0 provider/security/Git decisions
→ HCR1 execution-node model
→ HCR2 provider interface + fake
→ HCR3 real provider
→ HCR4 auto-running CodeLocal sandbox
→ HCR5 repo/Git read
→ HCR6 edit/test/commit/push
→ HCR7 sleep/restore
→ HCR8 Hybrid Router
→ HCR9 Files/Terminal/Git GUI
→ HCR10 Preview
→ HCR11 browser on demand
→ HCR13 quota/cost
→ HCR14 marketplace path
→ HCR12 full desktop if justified
→ HCR15 second provider
```

Do not start by building a Linux desktop. Prove headless execution, Git, persistence, hybrid routing and economics first.

---

# 36. MVP acceptance tests

Required end-to-end scenarios:

1. New user with no local client can trigger a Cloud task.
2. Concurrent first requests provision exactly one logical runtime.
3. Cloud Runtime self-registers without pairing UI.
4. Authorized repository works without local Git credentials.
5. Git credential is repository-scoped and short-lived.
6. Files/search/read works.
7. Edit + shell/test/build works.
8. Commit/push uses CodeLocal-managed identity.
9. Sandbox cannot directly access production DB/Redis/provider secrets.
10. Cross-tenant runtime/workspace access is blocked.
11. Idle runtime suspends according to policy.
12. Uncommitted work survives normal suspend/restore.
13. Restored runtime receives fresh Git authorization.
14. Existing Local Runtime works exactly as before.
15. One account can have both Cloud and Local execution nodes.
16. Divergent runtime state returns explicit reconciliation result.
17. Provider outage returns structured failure, not generic `tool disabled`.
18. Resource usage/cost is recorded.
19. Workspace deletion revokes runtime and deletes compute/storage according to retention policy.
20. Marketplace reviewer completes representative flow without local daemon.

---

# 37. Security acceptance tests

```text
cross-tenant filesystem access blocked
runtime-ID enumeration grants no access
bootstrap token replay rejected
revoked runtime credential rejected
provider API secret absent from sandbox
Git token TTL/scope verified
Git token redacted from logs
stale checkpoint credential is not reusable
provider metadata/control endpoint blocked where practical
preview requires authorization
terminal cannot reach production DB directly
runtime cannot register as another user/project
MCP cloud dispatch preserves approval/security policy
```

Threat model must include malicious repository code and prompt injection originating from repository contents.

---

# 38. Failure/chaos tests

Test at minimum:

```text
provider create times out after actually creating sandbox
runtime registration response lost
backend restarts during provisioning
provisioning lease expires
sandbox dies during clone
sandbox dies with uncommitted work
checkpoint succeeds but response is lost
restore returns stale image
Git token expires during push
network drops during push
Cloud reconnect during long build
quota exhaustion during pinned runtime
provider returns duplicate instance reference
```

Idempotency and reconciliation are mandatory.

---

# 39. Decision log — 2026-08-26

**HCRD1 — Local remains first-class.** Cloud is added; Local is not removed.

**HCRD2 — Cloud is default-on-demand, not always-on.** A logical cloud computer does not imply one permanent VM per account.

**HCRD3 — Cloud Runtime self-starts CodeLocal.** No manual local CLI/pairing inside managed compute.

**HCRD4 — Provider-neutral compute boundary.** Railway, Daytona and future providers are infrastructure choices.

**HCRD5 — CodeLocal-managed Git.** Cloud Git identity/authorization never depends on the user's local Git setup.

**HCRD6 — Git authorization is scoped and short-lived.** No global PAT in sandbox images/checkpoints.

**HCRD7 — GUI is layered.** Files/Terminal/Git/Preview first; full Desktop only on demand.

**HCRD8 — Capability-on-demand controls cost.** Browser/Desktop/container-heavy features run only when needed.

**HCRD9 — Agent Engine and Runtime Location are independent.** Models are workers; Local/Cloud are execution locations.

**HCRD10 — Project Brain survives runtime changes.** Intelligence belongs to logical project identity, not a VM.

**HCRD11 — Cloud source has explicit privacy disclosure.** Local source stays on Local; Cloud source intentionally executes in isolated managed compute.

**HCRD12 — Marketplace happy path prefers Cloud.** Local remains optional/advanced and first-class.

**HCRD13 — CI/CD is component-owned.** Runtime, Cloud and Web pipelines trigger from their own code regions; docs-only pushes build nothing.

---

# 40. What not to do

Avoid:

- one always-running VM for every signup;
- one Railway Project/Environment per user as the tenancy abstraction;
- removing Local Runtime;
- duplicating MCP tools into Cloud/Local variants;
- coupling product logic to Railway/Daytona objects;
- provider secrets inside sandbox image;
- global Git PAT in sandboxes;
- copying local SSH/Git credentials to Cloud;
- treating checkpoints as backups without restore tests;
- losing uncommitted work during idle cleanup;
- permanent Chromium/VNC/Desktop processes for every user;
- building a full IDE before execution fundamentals work;
- silently switching between divergent working trees;
- unrestricted sandbox access to production private infrastructure;
- coupling model name to runtime location;
- storing secrets in Project Brain;
- claiming cloud source “always stays local”;
- free unlimited background hosting;
- rebuilding/deploying every product component on every `main` push.

---

# 41. Definition of success

The user should be able to hold this mental model:

```text
I use CodeLocal.

If I do nothing, CodeLocal can give me a cloud development computer.
If I want, I can connect my own Mac/Windows/Linux machine too.
My projects, Git history, Project Brain, rules and verified experience stay under one CodeLocal account.
I can change AI models without rebuilding my development environment.
```

Representative Cloud flow:

```text
User on phone:
"Fix the login bug and push it."
        ↓
resolve project
        ↓
ensure/restore Cloud Runtime
        ↓
CodeLocal auto-registers
        ↓
Git Broker grants scoped repo access
        ↓
Project Brain supplies context
        ↓
selected model/agent works
        ↓
edit + tests + verification
        ↓
managed commit/push
        ↓
record verified experience
        ↓
idle → checkpoint/suspend
```

Representative Local flow remains:

```text
select MacBook workspace
→ existing CodeLocal Local Runtime
→ normal files/Git/shell/browser/computer workflow
```

Both are one product, one project identity and one durable engineering brain.

---

# 42. Immediate next actions

1. Treat this plan as the Hybrid Runtime source of truth.
2. Complete HCR0 Railway-vs-Daytona benchmark.
3. Produce managed Cloud Runtime threat model.
4. Inspect device/workspace persistence for HCR1.
5. Define `ExecutionNode`, `CloudRuntime`, `RuntimeBinding` contracts.
6. Define GitHub App/Git Broker flow.
7. Build fake provider and lifecycle tests before provider SDK integration.
8. Implement one real provider.
9. Build managed CodeLocal runtime image/bootstrap.
10. Prove one repo read/edit/test/commit/push end-to-end.
11. Add idle suspend/restore with uncommitted-state preservation.
12. Add Local/Cloud selector.
13. Add Files/Terminal/Git/Preview GUI before full Desktop.
14. Measure actual cost per active Cloud user before setting quotas/prices.
15. Update privacy docs and marketplace submission pack before public launch.

---

# 43. Maintenance rule

Every future runtime change must answer:

```text
Does Local still work?
Does Cloud work without local setup?
Is provider-specific code isolated?
Is Git authorization scoped and short-lived?
Does uncommitted work survive normal lifecycle transitions?
Are secrets excluded from Project Brain/checkpoints/logs?
Is runtime switching reconciled safely?
Is the feature metered and cleanup-safe?
Does CI/CD build only affected product regions?
Does the user need to understand infrastructure jargon? Prefer no.
```

If an implementation violates these invariants, update this decision document explicitly before changing production behavior.
