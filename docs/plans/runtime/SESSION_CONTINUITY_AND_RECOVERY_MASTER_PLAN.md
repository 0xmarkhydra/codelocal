# CodeLocal Session Continuity & Runtime Recovery Master Plan

Status: **P0 — Accepted user feedback / implementation required**
Date: 2026-08-21
Owner: CodeLocal
Scope: MCP/connector continuity, workspace/task/repository resume, approval reuse, retry semantics, process persistence, and failure classification.

## 1. User problem

Real user workflows are frequently interrupted because CodeLocal can become disabled or lose effective runtime continuity between otherwise valid tool calls.

Observed examples:

```text
git status -> git commit -> git push
edit -> analyze -> commit -> push -> build -> run simulator
approve flutter run -> connector disabled before execution
```

Recurring symptoms:

- an authorized workspace was online, but a later tool call requires rediscovery/reselection;
- reconnect does not reliably restore the workspace/repository/task route;
- equivalent scoped actions can request approval repeatedly;
- nested repositories such as `app-dev` may work through terminal but fail through structured Git routing;
- tool/session refresh can invalidate practical continuity even though project authorization did not change;
- long-running commands such as `flutter run`, builds, tests and dev servers become difficult to recover after connector interruption;
- the assistant cannot reliably distinguish source-code failure, approval denial, repository routing failure and runtime disconnect, causing blind retries.

The product impact is severe: individual operations work, but multi-step coding workflows are unreliable.

## 2. Product requirement

Highest-priority target:

> **CodeLocal reconnects and resumes seamlessly instead of making the user restart the workflow after a connector/runtime interruption.**

A reconnect must recover the last safe execution context without weakening authorization or approval boundaries.

## 3. Core continuity contract

Keep these identities separate:

```text
MCP transport session ID
  ephemeral transport isolation only

workspaceKey
  authorized local checkout identity

projectId
  durable logical project identity

repositoryId
  stable repository ownership inside a workspace

taskId / handoffId
  CodeLocal-owned continuation identity

processId
  CodeLocal-owned running-process identity

approval action key
  stable scoped authorization decision identity
```

Do not make task continuity depend on an MCP transport session surviving.

This extends the handoff model already described in `docs/plans/integrations/MCP_TOOL_SURFACE_PLAN.md` and the repository/task model in `docs/plans/execution/MULTI_REPO_MULTI_TASK_LOCAL_EXECUTION_PLAN.md`.

## 4. Resume State

CodeLocal should maintain a compact resumable state for the active workflow.

Recommended state:

```text
ContinuationState
- userId
- projectId
- workspaceKey
- repositoryIds[]
- activeRepositoryId?
- taskId?
- handoffId?
- branch/revision fingerprints
- changedFiles[]
- lastCompletedOperation
- nextIntendedOperation?
- runningProcessIds[]
- verification snapshot/reference?
- approval scope references (never one-time approval tokens)
- updatedAt
```

Do not persist secrets, raw approval tokens, raw terminal history or full source contents in continuity state.

## 5. Reconnect behavior

When the MCP connector or local runtime reconnects:

```text
1. authenticate user/device
2. resolve the prior logical project/workspace route
3. verify workspace is still authorized
4. refresh repository registry
5. restore task/handoff binding when available
6. compare saved repository/branch state with current reality
7. restore process handles for still-running CodeLocal processes
8. restore applicable remembered approval decisions by stable scope
9. continue from the last confirmed operation boundary
10. retry only operations proven interruption-safe
```

If state diverged, return a structured divergence result instead of silently continuing against a different branch/repository.

## 6. Automatic reconnect and retry

### Safe automatic retries

Automatic reconnect + retry is appropriate for operations that are read-only or explicitly idempotent, for example:

```text
workspace info/list
project/context/read/search
lsp reads
Git status/diff/log
process list/poll
verification status reads
```

Use bounded retry with jitter/backoff and a deterministic retry budget.

### Conditional retries

Operations with an idempotency key may resume/retry only when the runtime can prove the previous execution did not complete twice.

Examples:

```text
process start
bounded task step
selected local mutations with explicit idempotency support
```

### Never blind-retry

Do not automatically replay consequential actions merely because the connection disappeared:

```text
git commit
git push
external publish/send
permission changes
destructive commands
critical computer/browser actions
```

For these, first reconcile the actual resulting state. If the action already completed, return that fact. If it did not complete, reuse the valid approval decision only when its exact scope remains applicable; otherwise request a fresh approval.

Permission/policy failures are never treated as transient runtime errors.

## 7. Approval continuity

Existing `approvalMemory` should survive tool-surface refresh and MCP transport reconnect when the underlying authorization scope remains valid.

Rules:

- remembered approval decisions are workspace-scoped;
- operation identity must be stable and based on the internal operation/action key, not volatile public tool schema/session identity;
- repository-specific Git approvals include repository identity and, where relevant, resolved remote;
- critical actions still require fresh confirmation according to current policy;
- one-time approval tokens are never persisted or reused across unrelated calls;
- reconnect must not convert a denied/expired approval into an allowed action;
- policy version/security context changes invalidate stale remembered decisions when required.

The goal is to reuse **decisions**, not replay **tokens**.

## 8. Authoritative repository routing

Terminal and structured Git APIs must resolve repositories through the same authoritative `RepositoryRegistry` / path ownership model.

Required invariants:

```text
same workspace path -> same repositoryId everywhere
terminal cwd         -> repository resolution uses registry
Git structured API  -> repository resolution uses registry
context/project      -> repository ownership uses registry
verify               -> repository ownership uses registry
```

Nested repositories such as `app-dev` must not be recognized by one surface and rejected by another.

Use deepest matching repository root for path ownership.

On reconnect, refresh registry metadata but preserve stable repository identity where lineage/remote identity is unchanged.

## 9. Long-running process persistence

All CodeLocal-started long-running execution must be owned by `internal/process` or the canonical process lifecycle layer.

Required process metadata:

```text
processId
workspaceKey
projectId
repositoryId?
taskId?
command fingerprint
cwd/repository binding
PTY/non-PTY
startedAt
lastActivityAt
status
exitCode?
```

Required behavior:

- `flutter run`, builds, tests and dev servers continue while the MCP transport reconnects;
- reconnect does not spawn a duplicate process;
- a stable `processId` remains pollable from a later MCP session when user/workspace authorization still permits it;
- process output cursors remain resumable;
- process list can recover active/orphaned CodeLocal-owned processes after gateway reconnect;
- machine-runtime restart should attempt safe process reconciliation where the OS/process model allows it, otherwise classify the handle as orphaned/lost explicitly.

## 10. Failure taxonomy

Every failed tool call should return a machine-readable failure class.

Minimum taxonomy:

```text
SOURCE_FAILURE
  command/test/build/source-code failure

POLICY_DENIED
  local policy disallowed the operation

APPROVAL_REQUIRED
  user confirmation is required

APPROVAL_EXPIRED_OR_SCOPE_CHANGED
  a prior decision cannot safely be reused

WORKSPACE_UNAVAILABLE
  workspace is sleeping/offline/not currently routable

WORKSPACE_UNAUTHORIZED
  workspace grant no longer exists

REPOSITORY_NOT_FOUND
  no authoritative repository matches the requested operation

REPOSITORY_AMBIGUOUS
  multiple valid repositories require explicit resolution

RUNTIME_DISCONNECTED
  paired local runtime connection dropped

GATEWAY_RELAY_INTERRUPTED
  Cloud/relay path failed while runtime ownership may still exist

PROCESS_NOT_FOUND
  CodeLocal process handle is absent/expired

STATE_DIVERGED
  branch/revision/repository/task state changed since the saved continuation point

TRANSIENT_INTERNAL
  bounded retry may be appropriate

INTERNAL_BUG
  invariant/implementation failure requiring diagnostics
```

Include structured fields where useful:

```text
retryable
reconnectSuggested
approvalRequired
workspaceKey
repositoryId
processId
operationId
lastConfirmedState
```

The assistant should not have to infer whether `disabled` means source failure or runtime transport failure.

## 11. Recovery state machine

Conceptual flow:

```text
TOOL CALL
  |
  +-> success
  |
  +-> approval required -> wait for approval -> execute/reconcile
  |
  +-> policy denied -> stop; never retry as transport issue
  |
  +-> runtime disconnected
        -> reconnect runtime
        -> restore workspace/task/repo state
        -> reconcile previous operation
        -> retry only if safe
  |
  +-> workspace sleeping/unavailable
        -> bounded activation
        -> restore route
        -> retry safe operation
  |
  +-> repository mismatch
        -> refresh registry
        -> resolve once
        -> return explicit ambiguity/not-found if unresolved
  |
  +-> state diverged
        -> refresh context
        -> require deliberate next action before mutation
```

## 12. Observability

Record safe aggregate events:

```text
continuity.disconnect.detected
continuity.reconnect.started
continuity.reconnect.succeeded
continuity.reconnect.failed
continuity.workspace.restored
continuity.repository.restored
continuity.task.restored
continuity.process.reattached
continuity.retry.safe
continuity.retry.blocked
continuity.state.diverged
continuity.approval.reused
continuity.approval.invalidated
```

Metrics:

```text
runtime_disconnect_rate
reconnect_success_rate
median_reconnect_ms
workspace_resume_success_rate
repository_resume_success_rate
process_reattach_success_rate
approval_reprompt_rate
safe_retry_success_rate
workflow_completion_without_manual_retry_rate
```

The most important user-level metric is not raw tool success. It is:

```text
long_workflow_completion_without_user_retry
```

Representative workflow:

```text
edit -> analyze -> commit -> push -> build -> flutter run -> poll simulator process
```

## 13. Implementation ownership

Likely canonical packages to inspect/extend:

```text
internal/runtime
internal/localclient
internal/runtimecontrol
internal/mcpgateway
internal/gateway
internal/approval
internal/idempotency
internal/process
internal/projectidentity
internal/project
internal/taskstate
internal/taskexecution
internal/security
internal/protocol
internal/cloudserver
```

Do not implement new production continuity behavior in `legacy/typescript-runtime/` except for explicit compatibility evidence/removal work.

## 14. Implementation phases

### SC0 — Failure taxonomy + diagnostics

- introduce stable structured failure classes;
- ensure runtime disconnect, approval/policy and source failure are distinguishable;
- carry operation/workspace/repository/process IDs safely through error responses;
- add regression tests for the current `disabled`/disconnect cases.

Exit: assistant can always tell why a call failed.

### SC1 — Stable continuity identities

- formalize CodeLocal continuation/task/handoff identity independent of MCP session ID;
- persist/refresh compact continuation state;
- bind workspace + repository + process references;
- preserve user/device/workspace authorization checks.

Exit: a fresh MCP transport session can recover the same safe task route.

### SC2 — Runtime reconnect coordinator

- detect runtime/relay disconnects;
- bounded reconnect;
- workspace activation/routing restore;
- request reconciliation;
- safe retry policy using operation metadata/idempotency.

Exit: read/poll workflows self-heal after transient disconnect.

### SC3 — Approval reuse correctness

- normalize stable action keys;
- verify workspace/repository/remote scope;
- separate remembered decisions from one-time tokens;
- preserve valid decisions across transport refresh;
- invalidate on security/policy scope change.

Exit: routine equivalent actions stop prompting repeatedly without weakening critical-action safety.

### SC4 — Repository registry unification

- make terminal/Git/context/verify share authoritative repository resolution;
- add nested-repo fixtures including `app-dev` style layouts;
- add `.git` file/worktree/submodule coverage;
- reconcile registry after reconnect.

Exit: a repository recognized by one CodeLocal surface is recognized consistently by all applicable surfaces.

### SC5 — Persistent process handles

- ensure all long-running starts return stable CodeLocal process IDs;
- process IDs survive MCP reconnect;
- list/poll/write/signal use the stable handle;
- reconnect reconciles running handles without duplicate starts;
- add Flutter/build/test/dev-server integration fixtures where practical.

Exit: `flutter run` remains pollable after connector reconnect.

### SC6 — End-to-end continuity tests

Required scenarios:

1. `git status -> disconnect -> status` auto-recovers.
2. approval granted -> transient disconnect before execution -> state reconciles without unnecessary re-prompt.
3. `git commit` response lost after local completion -> reconnect detects completed commit and does not duplicate it.
4. `git push` response lost -> reconcile remote/local state before any retry.
5. nested repo recognized by terminal and structured Git consistently.
6. workspace sleeps during workflow -> activation restores route.
7. MCP session is recreated -> workspace/task continuation survives.
8. policy denial is never auto-retried as a transient failure.
9. `flutter run` continues through reconnect and is pollable by process ID.
10. process start request is retried after transport loss without spawning a duplicate.
11. branch changes while disconnected -> return `STATE_DIVERGED` before mutation.
12. revoked workspace -> return `WORKSPACE_UNAUTHORIZED`, never silently route elsewhere.

## 15. Acceptance criteria

- [ ] Transient connector/runtime disconnect does not force the user to re-select the same authorized workspace manually.
- [ ] Safe read/poll operations automatically reconnect and retry within a bounded budget.
- [ ] Consequential writes are reconciled before retry, preventing duplicate commit/push/process starts.
- [ ] Valid remembered approval decisions survive transport/tool-surface refresh within their original scope.
- [ ] One-time approval tokens are never persisted across sessions.
- [ ] Nested repositories resolve consistently across terminal, Git, context and verification.
- [ ] Long-running CodeLocal processes survive MCP reconnect and remain pollable by stable process ID.
- [ ] Error responses explicitly distinguish source, approval/policy, routing and runtime transport failures.
- [ ] State divergence is surfaced before continuing mutation.
- [ ] The representative workflow `edit -> analyze -> commit -> push -> build -> run` completes without the user repeatedly saying “thử lại”.

## 16. Priority decision

This is **P0 reliability work**.

Do not judge CodeLocal coding-agent quality only by isolated tool correctness. A production coding agent must preserve continuity across the entire workflow.

The target user experience is:

```text
connector drops briefly
    -> CodeLocal reconnects
    -> restores workspace/repository/task/process state
    -> reconciles the interrupted operation
    -> safely continues
```

not:

```text
connector drops
    -> tool disabled
    -> assistant re-discovers workspace
    -> user approves again
    -> assistant retries from the beginning
```
