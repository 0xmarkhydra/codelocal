# CodeLocal Neural Control Plane — Dashboard Master Plan

Status: **Proposed implementation blueprint**  
Date: **2026-08-17**  
Owner: **CodeLocal**  
Primary target: **CodeLocal Cloud web dashboard first; native/npm runtime only when realtime execution telemetry needs new events**  
Depends on: [`PROJECT_BRAIN_MASTER_PLAN.md`](../intelligence/PROJECT_BRAIN_MASTER_PLAN.md), [`KNOWLEDGE_V2_COLLECTIVE_QUALITY_MASTER_PLAN.md`](../intelligence/KNOWLEDGE_V2_COLLECTIVE_QUALITY_MASTER_PLAN.md), [`UNIVERSAL_AGENT_RUNTIME_PLAN.md`](../runtime/UNIVERSAL_AGENT_RUNTIME_PLAN.md)
Product-wide umbrella: [`PRODUCT_UI_MASTER_PLAN.md`](./PRODUCT_UI_MASTER_PLAN.md)

> Product goal: when a user opens the CodeLocal dashboard, it should feel like looking into a living AI operating system rather than a conventional SaaS admin panel. The visual impact must come from real Project Brain, runtime, skill, verification and relationship data — not decorative fake telemetry.

---

# 0. Executive decision

Build the experience in two layers:

```text
V1 — WEB / CLOUD FIRST
  dramatic dark control plane
  living Project Brain graph
  project drill-down
  learned skill / experience views
  brain health
  device/runtime/tool/usage state

V2 — NATIVE RUNTIME + CLOUD TELEMETRY
  realtime agent activity stream
  context -> tool -> edit -> verify -> experience -> skill events
  animated graph pulses based on real execution
```

The first major release should be **web-only wherever possible**. Existing CodeLocal Cloud data is already rich enough to ship the core experience without requiring every user to update `codelocal` from npm.

Only add a runtime/npm release when the UI needs event detail the current runtime does not already send.

---

# 1. Product identity

Working product name for this dashboard architecture:

**CodeLocal // Neural Control Plane**

Internal design language:

- Mission Control
- Living Project Brain
- Neural Graph
- Runtime Pulse
- Verified Experience
- Intelligence, not admin chrome

The dashboard should answer these questions immediately:

1. What projects does CodeLocal understand?
2. How are projects, repositories, workspaces and machines related?
3. What has CodeLocal remembered?
4. What skills has it learned and how trustworthy are they?
5. What verified experiences can it reuse?
6. Which AI/runtime/workspace is active now?
7. Is the Project Brain healthy?
8. What is happening right now?

---

# 2. Current architecture we must reuse

Do not rewrite the product into React/Next merely for this redesign.

Current production UI path is already viable:

```text
Go Cloud server
  -> internal/cloudserver/dashboard_handler.go
  -> internal/cloudserver/knowledge_dashboard.go
  -> internal/ui/product_pages.go
  -> server-rendered HTML + CSS + focused client-side JavaScript
```

Current dashboard routes already include:

```text
/dashboard
/dashboard/workspaces
/dashboard/knowledge
/dashboard/devices
/dashboard/connect
/dashboard/invite
/dashboard/admin
```

Current API/health surfaces already expose useful dashboard data:

```text
GET /api/status
GET /health
```

The design should preserve:

- existing web auth
- CSRF protection
- current Go Cloud deployment shape
- explicit workspace/device authorization semantics
- tenant scoping
- server-side rendering for core navigation and shell
- current local-source-code privacy boundary

---

# 3. Existing data that makes the redesign possible

## 3.1 Knowledge Graph is already real

`internal/cloud/knowledge.go` already builds an account-scoped graph through `KnowledgeGraph(...)`.

Existing node families include:

```text
user
project
repository
workspace
device
skill
portable skill
knowledge source
knowledge revision / canonical knowledge
conflict
verified experience
native project/repository memory
legacy memory graph nodes
```

Existing relationships include, among others:

```text
WORKS_ON
CONTAINS_REPO
HAS_CHECKOUT
HOSTS
HAS_SKILL
USES_SKILL
HAS_PORTABLE_SKILL
HAS_KNOWLEDGE_SOURCE
HAS_CONFLICT
HAS_VERIFIED_EXPERIENCE
HAS_MEMORY
```

The graph already includes useful visual signals:

```text
confidence
importance
scope
lastSeenAt
kind
summary
```

This means the UI can encode semantic meaning directly:

- node radius from importance
- opacity/clarity from confidence
- grouping from node kind
- freshness from lastSeenAt
- conflict emphasis from kind/status

No fake graph data should ever be generated solely to make the dashboard look fuller.

## 3.2 Knowledge Graph UI already exists

`internal/cloudserver/knowledge_dashboard.go` currently contains:

- Canvas2D graph rendering
- force-style physics
- zoom
- pan
- drag nodes
- search
- kind filter
- node inspector
- relationship inspection
- confidence and importance meters
- mobile fallback layout

This becomes the visual foundation for the new overview instead of being thrown away.

## 3.3 Logical project / cross-device identity already exists

Current Project Brain and workspace sync architecture distinguishes:

```text
Logical Project
  -> Repository set
  -> Workspace checkout(s)
  -> Device(s)
```

The redesign must make this visually obvious so a user can see that two workspaces on different machines may belong to one logical project.

## 3.4 Learned Skills already expose trust signals

The learned-skill model already includes:

```text
intent
task kind
status
confidence
success count
failure count
last used
context fingerprint
repository IDs
rules hash
dependency hash
branch policy
required capabilities
workflow file hashes
portable ID
```

Statuses currently include:

```text
candidate
trusted
imported
stale
```

The dashboard should visualize these as a real trust lifecycle, not as a generic list.

## 3.5 Verified Experience already exists

Knowledge Graph already projects verified experiences connected to projects and repositories.

The dashboard should expose this product loop clearly:

```text
Request
 -> Context
 -> Execute
 -> Verify
 -> Verified Experience
 -> Reuse / Project Brain
```

## 3.6 Brain health already exists

Existing server status surfaces already calculate or expose:

```text
Durable Learning / outbox health
Canonical Graph Freshness
Canonical Embedding Freshness
Semantic Canary metrics
Schema migration status
Memory/vector/graph availability
```

These should be combined into one understandable Project Brain Health view.

## 3.7 Runtime / device / workspace state already exists

Existing dashboard and `/api/status` already provide:

```text
paired devices
runtime online/offline
workspace status: active / sleeping / device offline
workspace/device last seen
protocol/runtime data where available
MCP usage estimates
```

No npm update is needed to redesign these views.

---

# 4. Experience architecture

Desktop layout target:

```text
+-------------+-------------------------------------------+----------------+
|             |                                           |                |
| NAVIGATION  |            LIVING PROJECT BRAIN           |  LIVE SYSTEM   |
|             |                                           |                |
| Overview    |                graph canvas               |  runtime       |
| Brain       |                                           |  workspace     |
| Projects    |                                           |  AI/MCP        |
| Skills      |                                           |  tool surface  |
| Experience  |                                           |  brain health  |
| Runtime     |                                           |                |
| Security    |                                           |                |
|             |                                           |                |
+-------------+-------------------------------------------+----------------+
|                       INTELLIGENCE / ACTIVITY PULSE                       |
+----------------------------------------------------------------------------+
```

The center is primary. Cards and tables become drill-down surfaces, not the first impression.

---

# 5. Visual language

Target: **high-end technical operating system**, not gamer UI and not generic AI-gradient SaaS.

Base palette direction:

```text
background      near-black / graphite
surface         deep blue-black
primary text    near-white
secondary text  cool gray
CodeLocal       electric blue
project         violet
repository      cyan
memory          emerald
skill           amber
experience      electric blue / indigo
workspace       blue
machine/device  silver
global/user      warm gold
conflict        red
```

Rules:

1. Dark mode is the default Neural Control Plane experience.
2. Glow must have semantic meaning.
3. Do not add animated particles that do not correspond to real state or events.
4. High-confidence knowledge looks sharper; low-confidence knowledge is visually weaker.
5. High-importance nodes are larger, not merely brighter.
6. Conflicts are visible without turning the entire interface red.
7. Motion must stop or reduce under `prefers-reduced-motion`.
8. UI must remain legible on a normal laptop, not only on 4K screenshots.

---

# 6. Navigation model

Proposed control-plane navigation:

```text
CORE
  Overview
  Project Brain
  Projects
  Skills
  Experiences

SYSTEM
  Runtime
  Devices
  Workspaces
  MCP Connections
  Security

COMMUNITY
  Invite

ADMIN
  Administration
```

Existing URLs should remain stable where practical.

Recommended route evolution:

```text
/dashboard                    -> Neural Overview
/dashboard/knowledge          -> Project Brain
/dashboard/projects           -> new
/dashboard/skills             -> new
/dashboard/experiences        -> new
/dashboard/runtime            -> new combined runtime view
/dashboard/devices            -> retained
/dashboard/workspaces         -> retained
/dashboard/connect            -> retained
/dashboard/security           -> new dashboard security view if needed
```

Do not break old bookmarked routes unnecessarily.

---

# 7. Phase NCP0 — Freeze current data contracts

**Target:** Web / Cloud only  
**npm/native update:** No

Before changing visual presentation, document and test the existing graph/runtime data contracts.

Tasks:

- Add/confirm Go tests for graph tenant scoping.
- Add/confirm graph node/edge field stability.
- Confirm `/api/status` does not expose secrets or device credential hashes.
- Define stable DTOs for future JSON dashboard APIs.
- Keep `KnowledgeGraph` derivation rebuildable and separate from presentation state.
- Do not encode visual colors or coordinates into durable knowledge storage.

Acceptance criteria:

- No cross-user nodes or edges can appear.
- No source code, secrets, approval tokens or raw local learned-skill bindings are added to Cloud merely for visualization.
- Existing `/dashboard/knowledge` still works before visual refactor begins.

Likely files:

```text
internal/cloud/knowledge.go
internal/cloud/canonical_graph_test.go
internal/cloudserver/knowledge_dashboard.go
internal/cloudserver/server.go
```

---

# 8. Phase NCP1 — Neural Shell

**Target:** Web / Cloud only  
**npm/native update:** No

Replace the current restrained light dashboard shell with the dark Neural Control Plane shell.

Tasks:

- Introduce semantic dashboard design tokens.
- Replace current light-first `iosDashboardTheme` override on dashboard pages.
- Keep landing page styling independent unless intentionally aligned later.
- Add left navigation groups described above.
- Add compact top system status.
- Add right-side `Live System` rail on desktop.
- Add responsive tablet/mobile behavior.
- Add reduced-motion support.
- Keep auth/login views conservative and readable; do not force the cinematic shell onto authentication.

Initial Live System rail uses current data only:

```text
runtime status
active workspace count
sleeping workspace count
device count
MCP/tool status when available
brain health summary
```

Acceptance criteria:

- User immediately perceives a technical control plane, not a generic CRUD dashboard.
- Current auth, CSRF and destructive-action confirmation flows remain unchanged.
- Mobile navigation remains usable.
- No npm update is required.

Primary files:

```text
internal/ui/product_pages.go
internal/ui/main_port_test.go
internal/cloudserver/dashboard_handler.go
```

---

# 9. Phase NCP2 — Living Project Brain as the Overview hero

**Target:** Web / Cloud only  
**npm/native update:** No

Move the Knowledge Graph from a secondary page into the primary visual identity of CodeLocal.

Tasks:

- Extract reusable graph CSS/JS structure from `knowledge_dashboard.go` instead of duplicating logic.
- Render a compact or full-screen graph in `/dashboard`.
- Keep `/dashboard/knowledge` as the dedicated deep-inspection view.
- Cluster graph layout around logical projects.
- Keep user/global node near the global center.
- Visually anchor repositories to projects.
- Visually anchor workspaces to projects and devices.
- Separate durable intelligence from runtime topology while still showing relationships.
- Make selected node causal neighborhood easy to follow.
- Add project-focus zoom mode.

Semantic rendering rules:

```text
radius      <- importance
opacity     <- confidence
outline     <- selected / conflict / active
pulse       <- only real live event later
label       <- selected, high-importance, project/user, zoom-dependent
```

Graph grouping must support at minimum:

```text
project
repository
workspace
device
memory
skill
experience
knowledge source
canonical knowledge
conflict
```

Acceptance criteria:

- Existing graph data is reused.
- A user can visually understand `Project != Workspace`.
- Multiple device workspaces can visibly converge on one logical project.
- Graph remains interactive at current dashboard node limits.
- No decorative fake nodes are introduced.

Primary files:

```text
internal/cloudserver/knowledge_dashboard.go
internal/cloudserver/dashboard_handler.go
internal/ui/product_pages.go
internal/cloud/knowledge.go        # only if projection metadata genuinely needs extension
```

---

# 10. Phase NCP3 — Project drill-down / project solar system

**Target:** Web / Cloud only  
**npm/native update:** No

New route:

```text
/dashboard/projects
/dashboard/projects/{projectId}
```

Project overview should show:

```text
Project
  repositories
  workspaces/checkouts
  devices
  durable memories
  learned skills
  portable skills
  verified experiences
  knowledge sources
  conflicts
  freshness / health
```

Project detail should support a focused graph where the selected project is the visual center.

Recommended project summary metrics:

```text
repositories
active/sleeping checkouts
trusted/candidate skills
verified experiences
knowledge sources
open conflicts
last activity
brain freshness
```

Acceptance criteria:

- Project ID is resolved from Cloud logical identity, not local path.
- Same logical project across machines appears as one project.
- Repository-set identity is understandable from UI.
- User can jump from project -> repository/workspace/skill/experience node.

Likely backend additions:

```text
GET /api/projects
GET /api/projects/{projectId}/brain
```

These APIs are optional if first implementation stays SSR, but stable JSON routes are recommended for the richer graph UI.

Primary files:

```text
internal/cloud/knowledge.go
internal/cloud/store.go
internal/cloudserver/server.go
internal/cloudserver/dashboard_handler.go
new internal/cloudserver/projects_dashboard.go
```

---

# 11. Phase NCP4 — Learned Skills Intelligence

**Target:** Web / Cloud only for metadata view  
**npm/native update:** No for initial release

New route:

```text
/dashboard/skills
```

The page must make skill learning tangible.

Summary:

```text
Trusted
Candidate
Imported
Stale
Portable / cross-device corroborated
Degraded
```

Each skill card/row should show only Cloud-safe metadata:

```text
intent
status
confidence
success count
failure count
step count
last used
project/workspace relationship
portable/corroboration state
```

Project Brain graph should be able to focus on a skill and show its safe relationship context.

Important privacy rule:

- Do not sync or display machine-specific replay arguments or local-sensitive step payloads merely to make this page richer.
- Cloud metadata remains metadata.
- Portable-safe semantic definitions remain subject to existing Project Brain rules.

Acceptance criteria:

- A user can explain why a skill is trusted/candidate/stale.
- Skill confidence is not represented as a magic AI score without supporting success/failure/status information.
- Stale skills visibly explain that context changed when safe metadata exists.

Primary data source:

```text
codelocal_learned_skill_metadata
portable skill aggregation already used by KnowledgeGraph
```

Likely files:

```text
internal/cloud/knowledge.go
internal/cloud/store.go
internal/cloudserver/server.go
new internal/cloudserver/skills_dashboard.go
```

---

# 12. Phase NCP5 — Verified Experience Intelligence

**Target:** Web / Cloud only  
**npm/native update:** No for existing experience data

New route:

```text
/dashboard/experiences
```

Expose the core reuse loop visually:

```text
objective
 -> task kind
 -> project / repository
 -> outcome
 -> verification summary
 -> created at
 -> reusable Project Brain relationship
```

Experience detail should never pretend CodeLocal independently reasoned if the AI client/model performed the reasoning. Product language should remain accurate:

**AI reasons; CodeLocal preserves project context, controls execution, verifies evidence and stores reusable experience.**

Acceptance criteria:

- Recent verified experiences can be browsed by project.
- Failed/unverified operational noise is not presented as trusted experience.
- Experience graph connections remain tied to real project/repository relationships.

Primary data source:

```text
codelocal_experiences
```

Likely files:

```text
internal/cloud/knowledge.go
internal/cloud/store.go
new internal/cloudserver/experiences_dashboard.go
internal/cloudserver/server.go
```

---

# 13. Phase NCP6 — Project Brain Health score

**Target:** Web / Cloud only  
**npm/native update:** No

Combine existing diagnostics into an understandable health model.

Inputs already available:

```text
Durable Learning health
Canonical Graph Freshness
Canonical Embedding Freshness
Semantic Canary metrics
Memory graph/vector availability
Open knowledge conflicts
Schema readiness where user-relevant
```

The UI may show a normalized score such as `98 / 100`, but only if the formula is explicit and deterministic.

Suggested components:

```text
Knowledge        Fresh / stale / unavailable
Learning         Healthy / delayed / degraded
Semantic         Ready / disabled / degraded
Graph            Current / rebuilding / stale
Conflicts        0 / N open
Fallback         Deterministic recall available
```

Do not make semantic embeddings appear mandatory. The current architecture intentionally supports deterministic fallback.

Acceptance criteria:

- Health status is actionable.
- Disabled optional features are not automatically treated as failure.
- Open conflicts reduce health in a visible, explainable way.
- A user can drill into the relevant health detail.

Likely files:

```text
internal/cloudserver/knowledge_dashboard.go
internal/cloudserver/server.go
existing health-card helpers
```

---

# 14. Phase NCP7 — Runtime Mission Control

**Target:** Web / Cloud only initially  
**npm/native update:** No for current runtime state

New route:

```text
/dashboard/runtime
```

Combine system views that are currently fragmented.

Display:

```text
paired devices
online runtimes
active/sleeping/offline workspaces
client version when synchronized
protocol version
capabilities when Cloud knows them
last seen
MCP usage estimates
tool surface metadata where safe/available
```

Do not expose secret local machine state.

Keep `Devices` and `Workspaces` drill-down routes because they contain management actions such as revoke/remove access.

Acceptance criteria:

- Runtime Mission Control is read-focused.
- Destructive management stays behind existing confirmation and CSRF flows.
- Sleeping is explained as an efficiency state, not an error.

Primary files:

```text
internal/cloudserver/dashboard_handler.go
internal/cloudserver/server.go
internal/ui/product_pages.go
```

---

# 15. Phase NCP8 — Activity timeline using existing Cloud events

**Target:** Web / Cloud first  
**npm/native update:** Maybe not

Before changing the runtime, inspect whether current audit/gateway/usage/event data is sufficient to produce a recent activity strip.

Initial activity examples can include only events already persisted safely:

```text
runtime connected/disconnected
workspace synced/activated/revoked
MCP use activity
device paired/revoked
knowledge sync
verified experience created
skill metadata updated if available
```

New route/API recommendation:

```text
GET /api/activity/recent
```

Overview bottom rail:

```text
05:42  workspace activated
05:43  Project Brain synced
05:44  verified experience added
05:44  learned skill confidence updated
```

No claim such as `ChatGPT is editing file X` should be shown unless that event is actually known.

Acceptance criteria:

- Timeline is based on persisted/auditable state.
- No raw source code or command contents are exposed by default.
- No npm update is required if existing events are sufficient.

---

# 16. Phase NCP9 — Realtime Agent Pulse

**Target:** Cloud + native/npm runtime  
**npm/native update:** **Yes, likely required**

This is the first phase intentionally allowed to require a new `codelocal` runtime release.

Goal: animate the UI only when real work is occurring.

Desired high-level safe event model:

```text
agent.request.started
context.resolved
context.retrieved
workspace.activated
tool.started
tool.completed
verification.started
verification.completed
experience.promoted
skill.updated
agent.request.completed
```

Do not stream raw source text or secret command payloads to the dashboard.

A telemetry event should prefer fields like:

```text
eventId
timestamp
userId
projectId
repositoryId?
workspaceId?
deviceId?
sessionId?
requestId?
eventType
toolCategory?
status
durationMs?
counts / safe metrics
```

Cloud transport recommendation:

```text
runtime -> existing authenticated CodeLocal channel
Cloud -> normalized safe activity event
web dashboard -> SSE
```

New web endpoint recommendation:

```text
GET /api/activity/stream
Content-Type: text/event-stream
```

Use SSE for browser delivery unless a strong need for bidirectional dashboard communication appears.

Visual behavior:

- pulse relevant nodes
- animate only the involved edges
- update activity rail
- update runtime state
- update brain health/skill state when event completes

Example real sequence:

```text
AI client
  -> Project Brain context
  -> workspace
  -> tool execution
  -> verification
  -> verified experience
  -> learned skill update
```

Acceptance criteria:

- Every animation corresponds to a real normalized event.
- UI safely survives dropped SSE connections and reconnects.
- Event order is resilient to duplicate delivery using event IDs.
- No raw source/secret payload is introduced.
- Old runtime clients remain supported; they simply show less live telemetry.

Likely runtime files:

```text
internal/runtime/*
internal/localclient/*
internal/orchestration/*
internal/learnedskills/*
internal/taskstate/*
internal/usage/*
```

Likely Cloud files:

```text
internal/cloud/*
internal/cloudserver/server.go
new internal/cloudserver/activity.go
```

This phase must be version/capability gated.

---

# 17. Phase NCP10 — Cinematic polish without fake behavior

**Target:** Mostly Web / Cloud  
**npm/native update:** No unless additional event types are required

Allowed polish:

- semantic node glow
- smooth project-focus transition
- graph neighborhood fade
- relationship path highlight
- subtle active-runtime pulse
- event-driven edge pulse
- keyboard navigation
- command-palette style graph search
- reduced-motion mode
- responsive mobile inspector

Forbidden polish:

- random network traffic particles
- fake tool execution logs
- fake AI thinking progress
- fake model connections
- fake runtime activity
- arbitrary cyberpunk noise that harms readability

The product should feel powerful because the system has real state, not because the UI imitates a sci-fi movie.

---

# 18. API plan

Do not add all routes up front. Add only when the SSR page needs a reusable or realtime JSON contract.

Recommended eventual surface:

```text
GET /api/brain/graph
GET /api/brain/status
GET /api/projects
GET /api/projects/{projectId}/brain
GET /api/projects/{projectId}/skills
GET /api/projects/{projectId}/experiences
GET /api/activity/recent
GET /api/activity/stream        # SSE, later
```

Every route must be:

- authenticated
- tenant scoped
- bounded by explicit limits
- sanitized
- free of local secret/source payload by default

---

# 19. Rendering strategy

## V1

Keep Canvas2D.

Current graph dashboard limit is small enough that Canvas2D is appropriate and already implemented.

Required refactor:

```text
Graph data
 -> reusable graph model
 -> reusable Canvas renderer
 -> Overview compact mode
 -> Project Brain full mode
 -> Project focus mode
```

Do not introduce a heavyweight graph dependency until performance data proves it is needed.

## Future threshold

Consider WebGL only when real production graphs regularly exceed the comfortable Canvas2D interaction budget.

Before migrating renderer:

- benchmark node count
- benchmark edge count
- benchmark mobile behavior
- benchmark interaction latency
- preserve semantic rendering contract independently of rendering engine

---

# 20. Security and privacy constraints

These are non-negotiable.

1. Dashboard redesign must not expand what Cloud stores merely for visuals.
2. Raw source code remains local unless an existing explicit product flow says otherwise.
3. Secrets remain local/protected.
4. Local machine-specific learned-skill bindings remain local.
5. Device credentials/hashes are never exposed to dashboard JS.
6. Approval tokens are never exposed.
7. All project/graph queries remain user scoped.
8. Project identifiers must not permit cross-tenant enumeration.
9. SSE/activity streams must be authenticated and tenant scoped.
10. Realtime activity defaults to metadata, not content.

---

# 21. Compatibility strategy

The dashboard must tolerate mixed runtime versions.

Example:

```text
Runtime 1.5.15
  current metadata support
  no fine-grained activity pulse

Future runtime 1.6.x+
  normalized realtime telemetry capability
  graph pulse enabled
```

Capability rule:

```text
if runtime advertises realtimeActivityV1
  enable fine-grained pulse
else
  render normal runtime state + Cloud activity only
```

Do not force an npm upgrade merely to access the dashboard.

---

# 22. Rollout order

Recommended shipping sequence:

```text
NCP0  Freeze/test contracts
NCP1  Neural Shell
NCP2  Living Brain on Overview
NCP3  Projects
NCP4  Skills
NCP5  Experiences
NCP6  Brain Health
NCP7  Runtime Mission Control
NCP8  Existing-event Activity Timeline
----------------------------------------- web-only milestone
NCP9  Realtime Agent Pulse
NCP10 Cinematic polish
```

The line above is the intentional release boundary:

**Ship a visually strong web product before asking users to update npm.**

---

# 23. Suggested implementation file split

Avoid continuing to grow `internal/ui/product_pages.go` and `knowledge_dashboard.go` indefinitely.

When implementation starts, split by responsibility.

Suggested direction:

```text
internal/ui/
  dashboard_shell.go
  dashboard_theme.go
  dashboard_icons.go
  dashboard_components.go

internal/cloudserver/
  overview_dashboard.go
  knowledge_dashboard.go
  projects_dashboard.go
  skills_dashboard.go
  experiences_dashboard.go
  runtime_dashboard.go
  brain_status.go
  activity.go

internal/cloud/
  knowledge.go
  project_dashboard.go
  skill_dashboard.go
  experience_dashboard.go
  activity.go
```

This is a direction, not a requirement to perform a giant refactor before visual work.

Apply extraction incrementally as touched code grows.

---

# 24. Acceptance criteria for the web-only milestone

The web-only milestone is complete when all are true:

- [ ] Dashboard defaults to the Neural Control Plane visual system.
- [ ] Overview is graph-first rather than metric-card-first.
- [ ] Graph uses real Project Brain data.
- [ ] Project/repository/workspace/device relationships are visually understandable.
- [ ] User can focus a project and inspect its related intelligence.
- [ ] Learned Skills page exposes trust lifecycle and evidence counts.
- [ ] Experiences page exposes verified reusable outcomes.
- [ ] Brain Health combines current durable-learning/graph/semantic/conflict state.
- [ ] Runtime Mission Control shows devices/workspaces/runtime state without destructive shortcuts.
- [ ] Current dashboard management actions still work.
- [ ] Mobile remains usable.
- [ ] Reduced-motion users do not receive unnecessary animation.
- [ ] No npm update is required.
- [ ] No new privacy boundary is crossed.

---

# 25. Acceptance criteria for realtime milestone

Realtime Agent Pulse is complete when:

- [ ] Native runtime advertises the new telemetry capability.
- [ ] Old runtimes remain compatible.
- [ ] Cloud normalizes safe event metadata.
- [ ] Browser receives events over authenticated SSE.
- [ ] Graph animations correspond only to real events.
- [ ] Activity timeline can reconnect without corrupting state.
- [ ] Duplicate events are idempotently handled.
- [ ] No source/secret payload is required for the visual effect.
- [ ] Verification and skill/experience transitions can visibly update after completion.

---

# 26. Verification plan

For web-only implementation phases, every meaningful change should run the narrowest relevant checks first:

```text
git diff --check
go test ./internal/ui/...
go test ./internal/cloudserver/...
go test ./internal/cloud/...
```

Then widen when shared data contracts change:

```text
go test ./...
npm run typecheck
npm run test
```

Visual verification should include:

```text
Desktop wide
Desktop laptop width
Tablet rail width
Mobile portrait
Empty graph
Small graph
Large graph near current limit
Offline runtime
Multiple workspaces
Multiple devices for one logical project
Conflict nodes
No semantic provider
Degraded brain health
Admin and non-admin account
```

Performance checks:

- graph should remain interactive at current production limits
- no continuous animation loop should consume high CPU when graph is idle
- stop/reduce simulation after stabilization
- do not rerender entire graph for unrelated sidebar changes
- SSE reconnect backoff must be bounded

---

# 27. Non-goals

This project does **not** mean:

- replacing the AI model with a CodeLocal brain
- moving local source code to Cloud
- rewriting the whole product in React/Next
- exposing every local tool schema in the UI
- inventing an autonomous-agent progress indicator that the runtime cannot prove
- making embeddings mandatory
- replacing Project Brain architecture
- changing MCP public tool surface purely for dashboard visuals
- requiring all users to update npm for the first redesign release

---

# 28. Product language rules

Prefer:

```text
Project Brain
Verified Experience
Learned Skill
Logical Project
Workspace checkout
Local runtime
Controlled execution
Brain Health
Live activity
```

Avoid misleading claims such as:

```text
CodeLocal thinks
CodeLocal understands everything
CodeLocal autonomously solved
AI is currently editing X
```

unless the underlying event/data proves the statement.

Canonical positioning remains:

> **The AI client/model reasons. CodeLocal provides shared project intelligence, controlled local execution, verification and reusable experience across AI clients.**

---

# 29. Final design principle

The UI should not be designed as a prettier dashboard.

It should be designed as a **window into CodeLocal's real system state**.

The strongest screen is not one with the most effects. It is one where a user can literally see:

```text
AI client
   -> Project Brain
   -> Project / Repository
   -> Memory / Learned Skill
   -> Local Workspace / Device
   -> Tool Execution
   -> Verification
   -> Verified Experience
   -> Project Brain
```

When that loop is visible and backed by real data, CodeLocal stops looking like an MCP connector and starts looking like the durable intelligence/control layer around every AI coding client.
