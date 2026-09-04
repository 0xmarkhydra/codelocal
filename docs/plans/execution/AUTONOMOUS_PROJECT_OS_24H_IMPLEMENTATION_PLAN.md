# CodeLocal Autonomous Project OS — 24H Vertical Slice Implementation Plan

Status: **Execution blueprint / P0**
Date: **2026-09-05**
Target: **A usable end-to-end vertical slice within one 24-hour build window**
Branch baseline: `main`

Related:
- `docs/plans/product/AUTONOMOUS_PROJECT_OS_MASTER_PLAN.md`
- `docs/plans/ui/EXECUTIVE_CODEX_COMPANY_OS_UX_MASTER_PLAN.md`

---

# 0. Delivery decision

The 24-hour target is **not** the full long-term Autonomous Company OS.

The target is one thin but real end-to-end slice proving the product model:

```text
Global Executive Chat / Project Chat
  -> Goal detection
  -> Plan proposal
  -> User approval
  -> Chief creates Task Graph
  -> Agent executes isolated work
  -> Review / Tester verification
  -> Tester DONE
  -> Goal/Task state visible in Work + Chat + Company
  -> Needs You only for unresolved material decisions
```

If this path works reliably, later Docs impact, Design, Feedback, Media and non-software capability packs can reuse the same kernel.

Do not spend the first 24h building decorative Company animations while task/state semantics are fake.

---

# 1. P0 scope — must ship in the vertical slice

## 1.1 Global Executive Home shell

Implement a simple account-level Home in the Next.js web product:

```text
Global chat input
Needs You summary
Project cards / health
Recent outcomes
```

Use real available project/workspace/runtime state. Do not fabricate agent counts or activity that backend cannot prove.

Global Chat may initially use a minimal cross-project resolver:

- read/query across all authorized logical projects;
- for mutation, resolve a single target project before execution;
- if several targets are equally plausible, ask one concise question.

## 1.2 Project Chat remains Codex-like

Preserve existing Dashboard Chat behavior and progressively adapt the shell:

```text
thread rail
center chat
optional context rail
project selector
```

Direct coding behavior must remain available.

## 1.3 Durable Goal / Plan / Task core

Add minimum durable entities required for P0:

```text
Goal
Plan
Task
TaskDependency
TaskEvent
TesterVerdict
```

Minimum fields should include project/tenant scope, status, timestamps and revision/CAS where mutable concurrency matters.

Do not expose internal numeric IDs directly in normal UX.

## 1.4 Plan approval gate

For broad autonomous work:

```text
GoalDraft
  -> PlanProposed
  -> WaitingApproval
  -> PlanApproved
  -> Executing
```

No autonomous code mutation before `PlanApproved` for this P0 goal path.

Direct explicit coding commands continue through the existing direct agent path without requiring Goal ceremony.

## 1.5 Chief P0 planner

Chief P0 may be intentionally simple:

```text
approved plan
  -> structured task decomposition
  -> dependency graph
  -> capability/agent assignment
```

Reuse existing `internal/orchestration` and `internal/agentruntime` abstractions. Do not create a second agent loop.

## 1.6 Task execution binding

Coding tasks must reuse existing `internal/taskexecution` isolation.

Required invariant:

```text
Task ID -> one active execution bundle/worktree unless explicitly forked/recovered
```

The existing worktree provider must remain the execution mechanism rather than creating a new parallel worktree implementation.

## 1.7 Tester DONE authority

P0 must model verification explicitly.

Task completion flow:

```text
implementation result
  -> reviewer/verification
  -> tester evaluates acceptance criteria
  -> TesterVerdict(DONE | NOT_DONE)
```

Only `DONE` transitions the task to Done.

Goal Done requires all required task acceptance criteria and Tester verdicts satisfied.

## 1.8 Work view

Add minimal `Work` UI:

```text
Goals
Running
Testing
Done
Needs attention
```

A full Linear clone is not required in P0.

## 1.9 Company view

Add a truthful lightweight Company view:

```text
Current Goal
Chief status
active tasks / workers
waiting tasks
Tester status
Needs You
```

Do not require graph animation for P0.

## 1.10 Needs You

Persist or derive P0 human decisions from Task/Goal state.

Only show:

```text
plan approval
material ambiguity
policy-required approval
repeated unresolved blocker
```

Do not surface routine recoverable technical failures immediately.

---

# 2. Explicitly deferred from the 24h P0

The contracts may be documented and UI placeholders may exist, but the following should not block the first vertical slice:

```text
full Feedback Widget SDK
feedback clustering ML/scoring
full Docs -> impact engine
Design editor/prototype engine
Media/video pipeline UI
content publishing pipeline
cross-project Chief-of-Chiefs scheduler
advanced cost governor
agent reputation ranking
full Branch/Attempt DAG UI
realtime animated Company graph
external customer resolution notifications
fully autonomous production release policy editor
```

These are P1/P2 after the P0 kernel is proven.

---

# 3. Repository ownership

Respect current CodeLocal boundaries.

## 3.1 Durable state

Use existing Cloud ownership:

```text
internal/cloud
```

Add cohesive Project OS persistence/domain files rather than unrelated dumping-ground files.

Suggested responsibility split:

```text
internal/cloud/project_goals.go
internal/cloud/project_plans.go
internal/cloud/project_tasks.go
internal/cloud/project_task_events.go
```

Exact names may change if an existing cohesive file is a better owner.

## 3.2 HTTP/API transport

Use:

```text
internal/cloudserver
```

Transport only; business orchestration should not accumulate in handler functions.

Suggested API surface for P0:

```text
GET  /api/v1/project-os/summary
GET  /api/v1/projects/{projectId}/work
GET  /api/v1/projects/{projectId}/company
POST /api/v1/projects/{projectId}/goals
POST /api/v1/projects/{projectId}/plans/{planId}/approve
POST /api/v1/tasks/{taskId}/decision
```

Reuse existing authenticated browser/session boundaries and tenant scoping.

Route names are recommendations; preserve existing route conventions if implementation evidence suggests a better compatible shape.

## 3.3 AI orchestration

Use:

```text
internal/orchestration
internal/agentruntime
```

Chief should be a domain coordinator around existing runtime/planner capabilities, not a duplicate model-provider implementation.

## 3.4 Local task execution

Use:

```text
internal/taskexecution
internal/localclient
internal/runtime
```

P0 coding tasks bind to existing isolated execution/worktree machinery.

## 3.5 Browser product

New production UI goes under:

```text
web/
```

Use Next.js + TypeScript + React according to repository architecture.

Keep server components by default and add client boundaries only for actual interactivity.

Do not recreate backend authorization truth in TypeScript.

---

# 4. Minimal durable data model

The exact SQL/migration shape must follow existing Cloud migration conventions.

Conceptual P0 model:

```text
ProjectGoal
  id
  user_id / tenant scope
  project_id
  title
  objective
  status
  active_plan_id?
  created_by_actor
  created_at
  updated_at
  revision

ProjectPlan
  id
  goal_id
  project_id
  summary
  flow_json
  assumptions_json
  acceptance_criteria_json
  status
  approved_by?
  approved_at?
  revision

ProjectTask
  id
  goal_id
  plan_id
  project_id
  title
  description
  status
  task_kind
  required_capabilities_json
  assigned_agent_role?
  execution_task_id?
  priority
  created_at
  updated_at
  revision

ProjectTaskDependency
  project_id
  task_id
  depends_on_task_id

ProjectTaskEvent
  id
  project_id
  goal_id?
  task_id?
  event_type
  actor_type
  actor_id?
  safe_payload_json
  created_at

TesterVerdict
  id
  project_id
  task_id
  verdict
  evidence_json
  created_at
```

All reads/writes are tenant/project scoped.

Do not store raw secrets or unnecessary local source content in event payloads.

---

# 5. State machine

## 5.1 Goal

```text
DRAFT
PLAN_PROPOSED
WAITING_APPROVAL
APPROVED
EXECUTING
VERIFYING
DONE
BLOCKED
CANCELLED
```

## 5.2 Task

```text
PLANNED
READY
ASSIGNED
RUNNING
REVIEWING
TESTING
DONE
BLOCKED
WAITING_HUMAN
WAITING_EXTERNAL
RETRYING
STALE
FAILED
CANCELLED
```

Transitions must be explicit and validated server-side.

Do not allow client UI to arbitrarily declare Tester DONE without the authorized verification path.

---

# 6. Chat intent routing for P0

The Chat request should be classified into one of four high-level behaviors:

```text
ASK
DIRECT_WORK
GOAL_WORK
STATUS / CONTROL
```

## 6.1 ASK

Read/analyze only.

## 6.2 DIRECT_WORK

Examples:

```text
"Fix đoạn này."
"Đổi timeout thành 10s."
```

Use existing direct coding agent behavior.

## 6.3 GOAL_WORK

Examples:

```text
"Làm payment mới cho BIDDI."
"Làm onboarding V2."
```

Create Goal -> propose Plan -> await user approval -> autonomous task execution.

## 6.4 STATUS / CONTROL

Examples:

```text
"Payment đến đâu rồi?"
"Dừng hướng này."
"Ưu tiên task này."
```

Resolve to durable Project OS state.

P0 does not require perfect ML classification. A structured planner response with deterministic safety rules is sufficient.

---

# 7. Chief P0 execution algorithm

After Plan approval:

```text
1. Load approved Plan + Project Brain context.
2. Produce bounded structured task graph.
3. Validate no duplicate IDs/dependency cycle.
4. Persist tasks/dependencies transactionally.
5. Mark dependency-free tasks READY.
6. Scheduler claims READY tasks subject to concurrency/policy.
7. Bind coding tasks to taskexecution isolated bundle.
8. Run assigned agent.
9. Persist safe execution events.
10. On implementation completion -> REVIEWING/TESTING.
11. Tester evaluates acceptance criteria/evidence.
12. If NOT_DONE -> create repair/retry path.
13. If DONE -> task DONE and unblock dependents.
14. When required tasks DONE -> Goal DONE.
```

P0 scheduler may process a small bounded number of tasks concurrently; correctness is more important than maximizing concurrency.

---

# 8. Recovery policy P0

Minimum autonomous recovery sequence:

```text
execution failed
  -> classify retryability
  -> one safe retry where appropriate
  -> alternate strategy/agent invocation if planner supports it
  -> create repair subtask or return to implementing task
  -> WAITING_HUMAN only when unresolved/material
```

Hard constraints:

- no infinite retry;
- no duplicate irreversible side effects;
- reuse request/idempotency identifiers where current runtime supports them;
- permission failures are not silently retried;
- security boundaries remain authoritative.

---

# 9. UI implementation P0

## 9.1 Global Home

Target components:

```text
ExecutiveHero
GlobalCommandComposer
NeedsYouPanel
ProjectPulseList
RecentOutcomeList
```

Data must come from APIs or clearly labeled empty states.

## 9.2 Project Chat

Preserve current chat implementation and add:

```text
project context header
thread rail polish
plan proposal card
approved plan state
active task summary
optional right context rail
```

Do not rewrite chat persistence in the same slice unless required by Task OS integration.

## 9.3 Work

P0 is read-focused with minimal controls:

```text
Goal list
Task status groups
Task detail
manual priority / decision where safe
```

## 9.4 Company

P0 uses simple structured visualization:

```text
Chief
Current Goal
Active workers/tasks
Dependency relation
Tester
Needs You
```

No fake realtime animation.

---

# 10. 24-hour build order

This is an execution sequence, not a promise that every long-term feature fits the window.

## Block A — Contract + persistence

- finalize P0 entity/state contracts;
- add migrations;
- add tenant-scoped store methods;
- add state transition tests;
- add event append/read primitives.

Exit gate:

```text
Goal -> Plan -> Task data can be persisted/read safely.
Invalid cross-tenant access fails.
Invalid task state transitions fail.
```

## Block B — Plan approval + Chief decomposition

- connect broad Chat request to Goal/Plan proposal;
- render plan card response contract;
- approval mutation;
- Chief structured task decomposition;
- dependency validation/dedupe.

Exit gate:

```text
A real user prompt can become a durable approved Plan and Task Graph.
```

## Block C — Execution + Tester

- bind READY coding task to existing taskexecution;
- execute through existing agent runtime;
- persist task events;
- route result to review/testing;
- implement Tester verdict;
- unblock dependency chain;
- close Goal only on Tester DONE.

Exit gate:

```text
One approved Goal reaches DONE end-to-end without manual task management.
```

## Block D — Web UX

- Executive Home shell;
- Project Chat plan/task state;
- Work page;
- Company P0 page;
- Needs You panel;
- responsive/basic accessibility.

Exit gate:

```text
User can understand Goal -> Plan -> Running -> Testing -> Done without reading technical logs.
```

## Block E — hardening

- recovery/no-double-side-effect checks;
- restart/resume behavior where existing infrastructure permits;
- tenant/auth tests;
- direct coding regression check;
- browser visual verification;
- narrow tests then relevant full checks.

---

# 11. P0 API/contract tests

Required test cases:

## Tenant/project scope

- user A cannot read/write user B Goal/Plan/Task;
- task cannot be attached to another tenant's project;
- global summary only includes authorized projects.

## Planning

- broad goal produces Plan proposal but does not execute before approval;
- direct coding request does not unnecessarily create Goal ceremony;
- plan approval is idempotent / revision safe;
- plan edit invalidates stale proposed execution state.

## Task graph

- dependency cycle rejected;
- duplicate task/dependency rejected or normalized;
- dependent task cannot become READY prematurely;
- task ownership/execution bundle is stable across continuation.

## Tester

- implementing agent cannot mark task DONE directly;
- Tester NOT_DONE returns task to repair path;
- Tester DONE unblocks dependent tasks;
- Goal only completes after all required tasks are Tester DONE.

## Recovery

- retry budget bounded;
- permission failure does not auto-retry;
- duplicated delivery/event does not duplicate side effect;
- stale execution after Plan/requirement revision is rejected or marked stale.

## UI/API

- Global Home handles zero projects;
- multiple projects render without workspace internals;
- Needs You zero state is positive/clear;
- project Chat remains usable without right rail;
- old/direct Chat path remains functional.

---

# 12. Verification commands

Use repository-defined checks and narrow tests first.

For Go changes:

```text
gofmt on touched files
go test ./internal/cloud/...
go test ./internal/orchestration/...
go test ./internal/agentruntime/...
go test ./internal/taskexecution/...
go test ./internal/cloudserver/...
```

Then when shared behavior is touched:

```text
go test ./...
```

For web changes:

```text
npm run typecheck:web
npm run build:web
```

Repository sanity:

```text
git diff --check
```

Visual validation:

- desktop wide;
- laptop width;
- mobile;
- zero projects;
- several projects;
- goal waiting approval;
- task running;
- tester failed;
- tester done;
- Needs You zero/non-zero;
- direct coding thread.

---

# 13. P1 immediately after the vertical slice

Once P0 is stable, extend the same event/task kernel rather than adding parallel systems.

Priority P1:

1. **Docs -> RequirementChanged -> impact analysis -> stale/replan.**
2. **Feedback domain + embeddable widget ingestion.**
3. **Feedback clustering/dedupe -> bug/feature signal.**
4. **Project policy for dev/main/staging/prod autonomy.**
5. **Agent Registry + capability/skill matching.**
6. **Global Chief-of-Chiefs for cross-project priorities.**
7. **Attempt/fork DAG and compare/select.**

---

# 14. P2 expansion

After P1:

```text
Design first-class objects
visual verification
Media/video project capability pack
Content publishing capability pack
Analytics feedback loop
Budget governor
Agent reputation/evidence routing
Realtime Company visualization
Customer resolution notifications
```

---

# 15. Guardrails

Do not weaken existing CodeLocal guarantees while rushing the 24h slice.

Non-negotiable:

- preserve auth and tenant scope;
- preserve MCP public contracts unless explicitly versioned;
- preserve direct coding behavior;
- do not leak local secrets/source into Cloud events;
- do not fake agent/system activity;
- do not let UI become authorization truth;
- do not bypass approval/security policy;
- do not create a second worktree implementation;
- do not create a second AI runtime/provider stack;
- do not combine unrelated repository cleanup with this feature.

---

# 16. Definition of 24H MVP complete

The 24-hour vertical slice is complete only when this scenario works:

```text
1. User opens CodeLocal.
2. User sees Global Executive Home and can enter a project.
3. User opens BIDDI Chat.
4. User says: "Làm payment mới cho BIDDI."
5. CodeLocal proposes a clear Plan and user flow.
6. No implementation mutation occurs yet.
7. User approves the Plan.
8. Chief creates a durable Task Graph.
9. At least one coding task executes through existing isolated taskexecution.
10. Work/Company reflect real task state.
11. Implementation result reaches Tester.
12. Tester may reject and send work back.
13. Tester eventually declares DONE based on evidence/acceptance criteria.
14. Task becomes DONE.
15. Goal becomes DONE only after required tasks satisfy Tester DONE.
16. User can still open another thread and use CodeLocal directly like a coding agent.
17. Global Home accurately summarizes project state and real Needs You items.
```

Anything less is a UI prototype, not the P0 Autonomous Project OS vertical slice.