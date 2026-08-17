# CodeLocal Task-to-Agent Workflow Plan

Status: Proposed
Date: 2026-08-17
Target branch: `dev`
Scope: Add a human-readable work orchestration layer on top of CodeLocal's existing project identity, durable memory, knowledge graph, learned skills, agent loop, task execution isolation, MCP hub, approvals, verification and dashboard. Local Git worktrees are the first execution backend; OpenSandbox must plug into the same private execution contract later without changing the public MCP/task model.

## 1. Goal

Make CodeLocal's existing intelligence visible and operable as a simple workflow:

```text
Knowledge / Memory / Project State
              |
              v
            Task
              |
      +-------+-------+
      |               |
      v               v
   Human           AI Agent
                      |
              Task Execution Env
               /              \
      local worktree        safe sandbox
                              (future)
                      |
                    verify
                      |
                    review
                      |
                     PR
```

The UX should let a normal user understand, in a few seconds:

- what work exists,
- who or which agent owns it,
- what is currently running,
- what is waiting for human review,
- which branch/worktree contains the work,
- whether verification passed,
- and what knowledge CodeLocal learned from the result.

This is inspired by the strongest part of Clauge's UX — card -> agent -> worktree -> review — but it must be implemented using CodeLocal's existing architecture rather than cloning Clauge or creating a second source of truth.

## 2. Non-goals

Do not turn CodeLocal into a full IDE or developer super-app.

Do not add unrelated modules such as:

- SQL/NoSQL GUI,
- REST/Postman clone,
- SSH client UI,
- generic file explorer,
- embedded editor replacement.

Do not duplicate project memory into a separate task database if the same fact already belongs in durable memory or the knowledge graph.

Do not expose dozens of new public MCP tools. Prefer the existing compact tool surface and internal operations.

Do not let an AI silently commit, push, create a PR, approve a review, or bypass local approval policy.

## 3. Core design principle

### 3.1 One brain, multiple projections

CodeLocal already has several strong primitives:

- logical project identity across checkouts/devices,
- repository-set and Git-lineage matching,
- durable global/project/workspace memory,
- memory graph nodes and edges,
- learned skills with verification and confidence,
- task working memory and agent lifecycle,
- semantic-first context retrieval,
- MCP hub,
- filesystem/shell/git/LSP,
- browser/computer automation,
- local approval memory and deterministic policy,
- worktree isolation foundations,
- dashboard knowledge graph.

The new board must be a projection over these systems, not a replacement.

```text
                  CodeLocal Brain
                       |
       +---------------+---------------+
       |               |               |
       v               v               v
 Knowledge Graph   Task Board      Review Inbox
       |               |               |
       +---------------+---------------+
                       |
                  Agent Runtime
```

### 3.2 Task is an orchestration object

A task represents work to be performed, not long-term knowledge.

Long-lived decisions, facts and lessons continue to live in memory/graph.

A task may reference those memories, but should not copy them unnecessarily.

## 4. Proposed task model

Introduce an internal task entity with a stable ID.

Suggested fields:

```text
Task
- id
- projectId
- workspaceId?                 checkout/runtime binding when active
- repositoryIds[]              repositories touched by the task
- title
- description
- status                       backlog | todo | in_progress | review | done | blocked
- priority                     p0 | p1 | p2 | p3
- source                       user | memory | issue | agent | automation
- sourceRef?                   issue/memory/event reference
- ownerType                    none | user | agent
- ownerId?                     logical agent/persona/session identity
- activeSessionId?
- executionEnvironmentId?      stable task execution binding
- executionProvider?           local_worktree | opensandbox | future provider
- executionMode?               trusted_local | isolated
- worktreeId?                  local_worktree compatibility/detail only
- worktreePath?                local-only provider detail, never cloud-sanitized
- branchName?
- reviewState                  none | pending | approved | changes_requested
- verificationState            unknown | running | passed | failed
- verificationSummary?
- prUrl?
- issueUrl?
- createdAt
- updatedAt
- startedAt?
- completedAt?
- createdBy
- updatedBy
```

Important separation:

- `projectId` is logical and cross-device.
- `workspaceId` is a concrete authorized checkout/runtime.
- provider-specific local paths such as `worktreePath` are local-only.
- task identity and task lifecycle must not depend on a specific execution provider.
- OpenSandbox provider/session identifiers are execution metadata, not task identity.
- cloud/dashboard synchronization must never leak local paths, secrets, approval tokens or learned-skill recipe internals.

## 5. Task graph integration

Every task should project into the knowledge graph as a node when graph storage is enabled.

Suggested node kind:

```text
kind = task
```

Suggested relationships:

```text
User        -[CREATED]-> Task
Project     -[HAS_TASK]-> Task
Repository  -[AFFECTED_BY]-> Task
Task        -[USES_MEMORY]-> Memory
Task        -[OWNED_BY]-> AgentSession
Task        -[PRODUCED]-> Artifact/File/PR
Task        -[LEARNED]-> Skill
Task        -[DEPENDS_ON]-> Task
Task        -[BLOCKED_BY]-> Task
Task        -[DERIVED_FROM]-> Issue/Memory/Event
```

The board UI then becomes a filtered visualization of `task` nodes plus task runtime state.

This keeps the Knowledge Graph as the durable relationship model while the board provides a simpler operational view.

## 6. Board projection

Default columns:

```text
Backlog
Todo
In Progress
Review
Done
Blocked
```

The status field is canonical; board columns are not separate workflow truth.

Moving a card means changing `Task.status`.

Rules:

- Backlog -> Todo: work accepted/planned.
- Todo -> In Progress: task has an active owner/session or human explicitly starts it.
- In Progress -> Review: implementation is complete enough to evaluate.
- Review -> Done: human approval or explicit completion policy passes.
- Any active state -> Blocked: dependency or runtime error requires intervention.
- Done should be reversible by an explicit user action.

Do not infer `Done` merely because an agent process exits successfully.

## 7. Single-owner task locking

Borrow Clauge's strongest concurrency idea: one active work stream per task.

Add an ownership lease:

```text
TaskLease
- taskId
- ownerSessionId
- ownerAgentId?
- deviceId
- workspaceId
- acquiredAt
- heartbeatAt
- expiresAt
```

Behavior:

1. Agent claims task.
2. Cloud/local state verifies there is no live conflicting lease.
3. Agent session becomes active owner.
4. Other agents may read/comment but cannot perform task-driving mutations.
5. Lease is released on completion, explicit handoff, cancellation or timeout.
6. A stale lease can be recovered safely after checking local runtime state.

A manual terminal/code session must be represented the same way as an AI agent session so the UI can show that the task is already active elsewhere.

## 8. Task -> agent flow

Primary UX:

```text
User opens task
    |
    +-- Assign to ChatGPT
    +-- Assign to Codex
    +-- Assign to Claude
    +-- Assign to Gemini
    +-- Continue myself
```

Internally:

```text
assign
  -> resolve logical project
  -> select/activate authorized workspace
  -> claim task
  -> create/resume agent session
  -> hydrate context
  -> create isolated worktree when code mutation is needed
  -> execute bounded agent loop
  -> verify
  -> move to Review or Blocked
```

Agent selection must use the MCP hub/provider abstraction rather than baking a provider directly into task storage.

## 9. Context hydration

When an agent starts/resumes a task, CodeLocal should build a bounded context packet from:

1. task title + description,
2. project identity,
3. relevant repository map,
4. semantic/LSP context,
5. related graph nodes,
6. durable project memories,
7. previous task checkpoints,
8. prior review feedback,
9. relevant learned skill,
10. current git/worktree state.

Pseudo-flow:

```text
task
  -> context_for_task(task objective)
  -> hybrid memory recall
  -> graph neighborhood
  -> learned_skill_match
  -> repository/LSP evidence
  -> bounded prompt/context packet
```

Do not dump all project history into the model. Preserve the existing semantic-first retrieval strategy.

## 10. Task execution environment lifecycle

A task owns an execution bundle rather than directly owning a Git worktree. The bundle is provider-agnostic and contains one or more repository execution bindings.

Initial provider:

```text
local_worktree
```

Future isolated provider:

```text
opensandbox
```

Private contract:

```text
TaskExecutionBundle
- taskId
- projectId
- workspaceId
- provider
- mode
- repositoryBindings[]
- checkpointId?
- state

RepositoryExecutionBinding
- repositoryId
- sourceRevision
- branchName?
- providerBindingId
- localPath?          local-only
- sandboxPath?        provider-private
```

For `local_worktree`, create an isolated worktree only when mutation is required.

Suggested local state:

```text
.codelocal/
  worktrees/
    <task-id>/
      <repository-id>/
```

User-facing UI should not need to expose implementation paths.

Suggested branch naming:

```text
codelocal/task/<short-task-id>-<slug>
```

Lifecycle:

```text
Task claimed
   |
mutation/execution needed?
   |
  yes
   v
Execution Router
   |
   +-- trusted bounded work -> local_worktree
   |
   +-- untrusted/high-risk work -> opensandbox (future)
   |
create/reuse execution bundle
   |
agent edits / executes
   |
verify provider result
   |
Review
   |
commit / push / PR or apply ChangeSet only after governed approval
```

Requirements:

- authoritative/main checkout remains clean by default,
- execution-provider internals are ignored from CodeLocal project indexing,
- recursive `.codelocal/worktrees` must never leak into search/context,
- task resumes should reuse a compatible execution binding when safe,
- provider changes must not change logical task identity or Project Brain identity,
- failed/cancelled execution environments are retained until explicit cleanup or a safe retention policy,
- cleanup must never delete an unmerged branch or unreviewed ChangeSet without explicit confirmation,
- OpenSandbox integration must stay behind the private execution contract defined by `OPENSANDBOX_SAFE_EXECUTION_MASTER_PLAN.md`; it must not add a parallel public MCP tool surface.

## 11. Verification gate

A task must not move automatically from In Progress to Review until verification has fresh evidence.

Reuse existing project-aware verification planning.

Store only a compact result on the task:

```text
verificationState
verificationSummary
verificationRunId
verifiedAt
```

Detailed logs stay in existing task/session/audit storage.

Suggested policy:

- code changed -> require `verify_changes`,
- run recognized project checks when relevant,
- diagnostic regression => task remains In Progress or becomes Blocked,
- verification pass => eligible for Review,
- missing verification => visibly marked, never silently treated as passed.

## 12. Human Review Inbox

Add one dashboard view that aggregates all pending human decisions.

Examples:

- task implementation ready for review,
- failed verification needing judgment,
- requested git commit,
- requested push,
- requested PR creation,
- approval-required shell/network/computer action,
- agent asks for clarification,
- conflicting task ownership,
- blocked dependency.

Review item:

```text
ReviewItem
- id
- taskId
- projectId
- type
- summary
- evidenceRefs[]
- diffSummary?
- verificationSummary?
- requestedAction
- createdAt
- status
```

Actions must map back to existing approval/policy systems. The Review Inbox is a UI/control-plane projection, not a bypass.

## 13. Review workflow

Suggested state machine:

```text
In Progress
    |
verification passed
    v
Review
    |
    +-- Approve ----------> Done / ready to ship
    |
    +-- Request changes --> In Progress
    |
    +-- Reassign ---------> Todo / In Progress with new owner
```

Review feedback becomes both:

- task activity for immediate continuation,
- and, when durable/important, sanitized project memory after completion.

Do not store every comment as long-term memory.

## 14. Task activity timeline

Each task should have an append-only activity stream for explainability.

Examples:

```text
10:04 User created task
10:05 Assigned to Codex
10:05 Context hydrated: 6 files, 3 memories, 1 learned skill
10:06 Worktree created
10:11 4 files changed
10:12 Verification failed: typecheck
10:15 Agent repaired regression
10:17 Verification passed
10:17 Moved to Review
10:23 User requested changes
10:31 Verification passed
10:32 User approved
```

Activity is not the same as memory. It is operational/audit history.

## 15. Learned Skills integration

Task execution is a strong source of verified learned skills.

After a task completes:

```text
Task execution trace
      |
      v
verified reusable sequence?
      |
     yes
      v
learned_skill_record
```

Keep current safeguards:

- never learn from unverified execution,
- never persist approval tokens,
- unsafe/state-changing steps must still obey runtime policy,
- confidence grows through successful reuse,
- failures reduce confidence,
- recipes remain local when they contain local execution detail.

Board UI can show a subtle label such as:

```text
Known workflow
```

when a matching trusted skill exists, but should not imply autonomous permission.

## 16. Memory integration after task completion

When a task is completed, run a small durable-memory extraction step.

Potential outputs:

- architectural decision,
- discovered project constraint,
- verified implementation pattern,
- known failure mode,
- important file/module relationship,
- user preference relevant to the project.

Do not persist:

- transient logs,
- raw diffs,
- secrets,
- approval tokens,
- temporary file paths,
- low-value chatter.

The task node remains linked to the resulting memories so future agents can understand why knowledge exists.

## 17. MCP/public tool surface

Avoid adding one MCP tool per task operation.

Preferred option: extend the compact existing surfaces internally.

Possible mapping:

```text
workspace
  action: tasks

agent
  objective + bounded steps can claim/start/resume/update task

git
  existing explicit commit/push operations remain authoritative

verify
  existing verification operations remain authoritative
```

If a new public surface is unavoidable, add at most one compact `task` tool with an action discriminator rather than many tools:

```text
task(action=list|read|create|update|assign|claim|release|review|activity)
```

Do not implement this until token/schema cost is measured against extending `workspace`/`agent`.

## 18. Internal operations

Suggested private runtime operations:

```text
task_list
task_read
task_create
task_update
task_claim
task_release
task_activity_append
task_review_submit
task_review_resolve
task_worktree_attach
task_verification_update
task_session_attach
```

These should be private routing operations first. Public MCP exposure should be decided later based on UX and schema-size measurements.

## 19. Storage strategy

### Cloud-safe task metadata

Store logical task metadata centrally so the same project/task is visible across devices.

Safe to sync:

- task ID/title/description/status,
- logical project/repository IDs,
- timestamps,
- owner/provider labels,
- verification state/summary,
- PR/issue URL,
- sanitized activity summaries.

### Local-only execution metadata

Keep on device:

- absolute paths,
- worktree path,
- shell command internals when sensitive,
- approval tokens,
- credentials,
- learned-skill steps that depend on local environment,
- raw browser/computer handles,
- local process IDs.

The cloud task record can reference a local execution binding by opaque ID without receiving the sensitive payload.

## 20. Multi-device behavior

Because CodeLocal already resolves logical projects across repository sets and Git lineage, tasks should attach to `projectId`, not a filesystem path.

Example:

```text
Mac A: /Users/a/Projects/BIDDI
Mac B: /work/BIDDI-copy

           same logical projectId
                    |
                    v
                same tasks
```

When a user opens a task on another machine:

1. locate an authorized workspace bound to the same project,
2. verify required repository coverage,
3. fetch/sync logical task state,
4. create a new local execution binding if needed,
5. never assume the old machine's absolute worktree path exists.

An active task lease on another machine must be visible before attempting ownership takeover.

## 21. Multi-repository projects

A task can touch one or more repositories under a logical project.

Do not reduce project identity to a single git root.

Add explicit `repositoryIds[]` when known. If unknown at creation, let semantic context infer candidates and ask for confirmation only when ambiguity affects a write.

For code execution:

- one task may require multiple repositories,
- create one provider-agnostic task execution bundle with one repository binding per affected repository,
- with `local_worktree`, each affected repository gets its own isolated worktree,
- with `opensandbox`, the same logical bundle may be reconstructed inside one sandbox session when policy/capability allows, while preserving per-repository identity and provenance,
- Git operations always route to the owning repository rather than the logical workspace root,
- verification runs from the correct repository execution root,
- task UI groups changed repositories under one task,
- two concurrent tasks touching the same logical project must never share a mutable execution binding by default.

## 22. Dashboard UX

Add three primary views under the existing dashboard:

### 22.1 Work

Simple Kanban projection.

Card shows only high-signal information:

```text
Title
Project
Status
Owner/agent
Verification badge
PR badge
Last activity
```

### 22.2 Review

Human Review Inbox ordered by urgency and age.

### 22.3 Knowledge

Keep the existing Knowledge Graph, but clicking a task node should open the associated task/workflow view.

The three views should feel like different lenses over the same system, not three disconnected products.

## 23. Task detail UI

Task drawer/page:

```text
[Title]                     [Status]
Project / repositories

Description

Owner
Agent session
Branch / worktree status
Verification
PR / issue

Activity timeline

Relevant context
- linked memories
- related files/symbols
- learned workflow

Actions
[Assign] [Start/Resume] [Review] [Request changes]
[Commit] [Push] [Open PR]  <-- explicit approval path
```

Avoid exposing raw implementation detail unless the user opens an advanced/debug section.

## 24. Agent personas

Do not copy Clauge's persona system as the main abstraction.

CodeLocal's core abstraction should remain provider/session/capability based.

Optional display personas can be added later as presentation metadata:

```text
name
role
provider preference
system guidance
avatar
```

but task ownership must resolve to a real agent/session identity underneath.

## 25. Failure and recovery rules

### Agent crash

- preserve task state,
- preserve worktree,
- mark session interrupted,
- allow resume with fresh context.

### Runtime offline

- keep task visible in cloud dashboard,
- show execution device offline,
- do not transfer ownership automatically.

### Verification failure

- record evidence,
- keep task In Progress or Blocked,
- do not open review automatically.

### Stale lease

- verify runtime/device heartbeat,
- allow explicit takeover,
- preserve prior worktree/activity.

### Conflicting edits

- use optimistic version/update timestamps,
- reject stale task mutations,
- refresh state before retry.

### Branch/worktree missing

- never silently recreate over unknown git state,
- inspect repository status and history,
- offer safe recovery path.

## 26. Security model

The new workflow must inherit CodeLocal's current conservative security model.

Never auto-confirm:

- git commit,
- git push,
- PR creation,
- destructive filesystem operations,
- open-world network actions,
- physical/native input,
- credential access,
- remembered approvals outside their exact scope.

Task assignment is not permission escalation.

A learned skill is not permission escalation.

A trusted agent/persona is not permission escalation.

## 27. Implementation phases

### Phase 0 — Schema and invariants

Deliverables:

- internal task model,
- task status state machine,
- project/repository relationships,
- ownership lease model,
- task activity log,
- local/cloud privacy boundary,
- tests for cross-device logical project binding.

Exit criteria:

- tasks persist against logical project IDs,
- stale updates are rejected,
- no local path leaks to cloud task payloads.

### Phase 1 — Task CRUD + board read model

Deliverables:

- internal task repository/store,
- list/read/create/update operations,
- status transitions,
- board projection API,
- dashboard Work page,
- basic task detail drawer.

Exit criteria:

- user can create/move tasks,
- refresh/device reconnect preserves state,
- board is generated from canonical task status.

### Phase 2 — Ownership + agent sessions

Deliverables:

- claim/release lease,
- attach task to agent/manual session,
- visible active owner,
- takeover/handoff flow,
- session resume.

Exit criteria:

- two agents cannot drive the same task concurrently,
- stale lease recovery is deterministic,
- manual and AI sessions share the same ownership rules.

### Phase 3 — Context hydration

Deliverables:

- task-aware `context_for_task`,
- graph neighborhood injection,
- durable memory recall,
- learned-skill match,
- prior review feedback injection,
- context-budget tests.

Exit criteria:

- task start/resume gets bounded relevant context without broad scans,
- token/schema regression is measured and acceptable.

### Phase 4 — Task execution environments

Current implementation scope: **local worktree only**. OpenSandbox implementation is explicitly deferred to a later milestone.

Deliverables:

- provider-agnostic `TaskExecutionBundle` and `RepositoryExecutionBinding`,
- private Execution Router contract,
- `local_worktree` provider creation/reuse,
- multi-repo execution bundle support,
- per-task mutable-isolation rules so concurrent tasks do not share writable bindings,
- branch naming,
- provider-safe cleanup policy,
- UI execution/branch state,
- compatibility seam for a future isolated provider without implementing it now.

Exit criteria:

- agent code changes never dirty the user's authoritative/main checkout by default,
- two concurrent mutating tasks in the same workspace/project are isolated from each other,
- one multi-repo task preserves repository identity across all execution bindings,
- recursive CodeLocal worktrees remain excluded from indexing,
- adding OpenSandbox later does not require changing task identity, Project Brain identity, public MCP task semantics, or review/verification state machines.

### Phase 5 — Verification + Review Inbox

Deliverables:

- task verification state,
- verification evidence link,
- Review status gate,
- Review Inbox,
- approve/request-changes flows,
- approval integration.

Exit criteria:

- unverified code cannot appear as safely completed,
- human can see every pending decision in one view.

### Phase 6 — Git shipping flow

Deliverables:

- explicit commit request from task,
- explicit push request,
- explicit PR creation,
- PR URL/task linkage,
- activity events for each operation.

Exit criteria:

- no autonomous git write escapes existing approval policy,
- user can trace task -> branch -> commit -> PR.

### Phase 7 — Memory/skill feedback loop

Deliverables:

- post-completion memory extraction,
- task-to-memory graph links,
- verified task recipe -> learned skill candidate,
- learned-skill success/failure feedback,
- board indicators for known workflows.

Exit criteria:

- repeated tasks become faster without bypassing safety,
- durable memory stays high-signal rather than becoming a task log dump.

### Phase 8 — Knowledge Graph integration

Deliverables:

- task nodes/edges in Knowledge Graph,
- click-through graph <-> task,
- filters for active/completed/blocked tasks,
- relation inspector.

Exit criteria:

- Work, Review and Knowledge views resolve to the same logical project/task identities.

## 28. Suggested package/module layout

Exact names should follow existing repository conventions after implementation context review.

Conceptually:

```text
internal/
  tasks/
    models.go
    store.go
    transitions.go
    leases.go
    activity.go
    privacy.go

  orchestration/
    task_context.go
    task_execution.go
    task_verify.go
    task_handoff.go

  memory/
    task_graph.go

  cloud/
    tasks.go

  cloudserver/
    work_dashboard.go
    review_dashboard.go

  mcpgateway/
    task_operations.go
    task_context.go
```

Prefer small focused files and reuse existing stores/services instead of introducing parallel abstractions.

## 29. Testing plan

### Unit

- state transition table,
- lease acquisition/release/expiry,
- optimistic concurrency,
- privacy sanitizer,
- task graph projection,
- task-memory extraction filters,
- learned-skill eligibility,
- worktree naming/lookup.

### Integration

- create -> assign -> claim -> worktree -> verify -> review,
- request changes -> resume same task,
- provider handoff,
- runtime disconnect/reconnect,
- stale lease takeover,
- multi-device same project,
- multi-repo task,
- git write approval paths.

### Regression

- public MCP schema size,
- context token budget,
- dashboard load time with large task counts,
- graph query performance,
- no `.codelocal/worktrees` indexing,
- no sensitive local metadata in cloud payloads.

## 30. Metrics

Track product value, not just feature usage.

Suggested metrics:

- task time-to-first-action,
- task completion time,
- review wait time,
- agent resume success rate,
- verification pass rate,
- average context bytes/tokens per task,
- learned-skill reuse rate,
- rework rate after human review,
- ownership conflict rate,
- task recovery rate after interrupted runtime,
- percentage of tasks completed without dirtying main checkout.

Do not collect raw source code or sensitive prompt contents for product analytics.

## 31. UX acceptance criteria

A new user should be able to answer these questions without understanding MCP, worktrees or memory internals:

1. What is being worked on?
2. Who is doing it?
3. Is it still running?
4. Did checks pass?
5. What needs my approval?
6. Where is the resulting PR?
7. What did CodeLocal learn from this work?

If the UI requires the user to understand internal runtime terminology to answer these, the design is not ready.

## 32. Architectural acceptance criteria

The implementation is ready only if all are true:

- logical project identity remains the root cross-device identity,
- board/task state does not fork from Knowledge/Memory truth,
- task ownership prevents conflicting active agents,
- mutable code work is isolated by default,
- verification is a gate, not decoration,
- existing approval policy remains authoritative,
- public MCP tool count/schema does not balloon,
- learned skills only come from verified reusable execution,
- sensitive local execution data stays local,
- task history can survive agent/provider/device changes,
- users can review and continue work from a simple UI.

## 33. Recommended implementation order

Build in this order:

```text
1. Task model + state machine
2. Board read model + dashboard
3. Ownership lease
4. Task/session linkage
5. Task-aware context
6. Worktree binding
7. Verification gate
8. Review Inbox
9. Commit/push/PR flow
10. Memory + Learned Skill feedback loop
11. Knowledge Graph task projection
12. Multi-device/multi-repo hardening
```

The key rule is to make each phase usable and testable without requiring all later phases.

## 34. Final product direction

Do not position this as "CodeLocal has a Kanban board".

The intended product is:

```text
CodeLocal = persistent brain + safe runtime + orchestration control plane
            shared by ChatGPT, Codex, Claude, Gemini and other agents.
```

The board is simply the human-facing work projection of that control plane.

The desired end state:

```text
                    USER
                     |
             Work / Review UI
                     |
               CodeLocal Brain
        +------------+-------------+
        |            |             |
      Memory       Graph        Skills
        |            |             |
        +------------+-------------+
                     |
               Orchestrator
        +------------+------------+
        |            |            |
     ChatGPT        Codex        Claude ...
        |            |            |
        +------ isolated work -----+
                     |
                  Verify
                     |
                  Review
                     |
                  Commit/PR
                     |
             durable learning
```

This keeps CodeLocal focused on the layer that can become a durable moat: continuity, shared knowledge, safe execution and cross-agent orchestration — while borrowing the clearest workflow UX pattern from Clauge.