# CodeLocal Agent OS V3 — Architecture Delta Audit

Status: Phase A implementation audit
Date: 2026-08-30
Baseline: `main@151279f58a3e76664d2335e94da3173fce3fd787`
Implementation branch: `feat/agent-os-v3-foundation`
Parent plan: `CODELOCAL_AGENT_OS_V3_MASTER_PLAN.md`

## Executive finding

CodeLocal already contains the correct V3 seeds. V3 must be an incremental evolution, not a rewrite.

The strongest existing primitives are:

- `internal/agentruntime`: provider-neutral adapter/session/event contracts, transport and isolation tiers, mutation isolation checks, runtime tests.
- `internal/taskexecution`: durable local task bundle store, leases, per-task Git worktrees, cleanup safety.
- `internal/editing`: expected-hash optimistic file edits, validation-before-write, atomic writes, rollback-on-failure, patch path validation and `git apply --check`.
- `internal/security`: mature risk/approval/network/path/secret policy foundation and tests.
- `internal/state`: private state directories, atomic JSON writes, append-only JSONL helper.

The highest-value missing primitives are durable runtime truth, context projection, token economics, durable agent identity/activation, CAS revisions, dirty-checkout-safe worktrees, PatchSet provenance, unified monotonic policy decisions, verification truth, and failure intelligence.

## Non-negotiable migration rule

Do not create a second implementation beside an existing subsystem merely to match the V3 document naming.

Prefer:

```text
existing primitive
      ↓
add missing invariant
      ↓
add durable contract
      ↓
add migration/compatibility
      ↓
benchmark
```

Only introduce a new package when the responsibility is genuinely absent or when keeping it inside an existing package would mix durable truth with execution/provider concerns.

## Current-to-target matrix

| V3 target | Current implementation | Delta | Migration decision |
|---|---|---|---|
| Universal Agent Runtime | `internal/agentruntime` | No durable session event truth, activation split, router evidence history | Evolve existing package |
| Durable Runtime Event Log | no canonical runtime event store | Missing | Add `internal/runtimeevents` |
| Context Surface Engine | bounded `ContextPacket` only | Missing projections, pressure, reversible surfaces | Add separate context subsystem after event store |
| Token Meter | provider usage events only | Missing canonical accounting/budgets | Add after projections |
| Agent Identity vs Activation | provider Session only | Missing durable identity/lifecycle split | Extend agent runtime after event truth |
| Agent Graph | orchestration/runtime foundations | Not durable/recoverable enough for V3 | Add graph store using durable events |
| Task DAG + CAS | task execution bundles | No revision/CAS/dependency DAG | Extend task state/execution, do not duplicate |
| Durable Mailbox | none canonical | Missing | Add beside agent graph |
| Worktree V2 | `internal/taskexecution/local_worktree.go` | Source checkout must be clean; no reconciliation model | Evolve current provider |
| Patch V2 | `internal/editing` | No PatchSet/baseSha/provenance/status | Wrap/evolve editing engine |
| Optimistic Concurrency Everywhere | `ExpectedHash`, leases | Not generalized to tasks/brain/worktrees | Add revision contracts incrementally |
| Policy Kernel V2 | `internal/security` | Decisions not yet one canonical monotonic contract across all capabilities | Refactor in place |
| Secret consumption policy | sensitive env/path/redaction foundations | Must explicitly separate exposure from approved internal consumption | Add explicit decision tests before runtime rollout |
| Coding Kernel | external agent runtime + orchestration | No one canonical understand/locate/plan/act/observe/verify/reflect loop | Build after context/token/runtime foundations |
| Tool Program Runtime | none canonical | Missing | Add only after Policy V2 |
| Verification Engine | orchestration/verification concepts | Needs provider-independent durable evidence truth | Consolidate, do not trust provider completion |
| Failure Intelligence | errors/retries distributed | Missing causal attribution and branch rerun | Add after verification/event truth |
| Experience → Brain | Project Brain/Learned Skills foundations | Needs verified-evidence promotion gate | Evolve existing Brain/skills |
| RuntimeProvider Local/Cloud | local/cloud/runtime foundations | Needs one canonical capability contract | Unify after core semantics stabilize |
| Browser/Computer/Mobile verification | existing capabilities | Not yet one task verification graph | Integrate late, do not add public tools |
| Ecosystem Assimilation | skills/plugin research foundations | Missing governed R&D pipeline | Implement as separate lab/control plane |

## Critical current constraints discovered

### 1. Dirty checkout blocks task worktrees

`sourceRevision()` currently runs `git status --porcelain` and returns `ErrDirtyRepository` when the authoritative checkout contains changes.

V3 invariant:

> The user's authoritative checkout may be dirty. Agent isolation must not require the user to commit, stash, or stop editing.

Target migration:

1. snapshot authoritative `HEAD` as base revision without rejecting dirty state;
2. record a lightweight user-change fingerprint at task start;
3. create agent worktree from committed base;
4. never mirror uncommitted user changes into an agent worktree implicitly;
5. reconcile agent output against latest authoritative state with 3-way/base-aware logic;
6. stale/conflict is explicit and never resolves by overwriting user work.

This must not be implemented until PatchSet/base provenance and reconciliation tests are available.

> Resolution status (2026-09-04): implemented on main. Dirty checkouts are
> snapshotted (`snapshotSourceChanges`) instead of rejected; provenance is
> recorded (`SourceRevision`, `SourceDirty`, `SourceStatusFingerprint`);
> `ErrDirtyRepository` is retained only as a compatibility symbol.

### 2. Task execution leases exist but task mutations are not CAS-versioned

`taskexecution.Bundle` already has a lease and atomic JSON persistence. This is valuable and should stay.

Missing:

- `Revision uint64`;
- expected-revision mutation API;
- lease generation/token to prevent an expired owner from committing a stale mutation;
- durable transition validation;
- dependency DAG ownership.

> Resolution status (2026-09-04): implemented. `Bundle` carries `Revision`,
> mutations go through the expected-revision API with transition validation,
> leases are fenced by generation, and task dependencies live in `TaskDAG`.

Target invariant:

```text
mutation(expectedRevision=N)
  succeeds -> revision=N+1
  stale    -> explicit conflict
```

### 3. Editing already has the correct lost-update primitive

`editing.FileEdit.ExpectedHash` is exactly the V3 direction. It validates all edits before writing, rejects overlaps, writes atomically, and rolls back already-written files if a later write fails.

Do not replace this engine.

Patch V2 should add a durable `PatchSet` envelope around it:

```text
PatchSet
  id
  taskId
  agentId
  activationId
  worktreeId
  baseSha
  expectedHashes
  files
  provenance
  state
  verificationRefs
```

Existing `ExpectedHash` remains the file-level concurrency guard.

### 4. Agent runtime already has provider-neutral contracts

`agentruntime` already has:

- transport tiers;
- isolation levels;
- read/review/mutate modes;
- session status;
- canonical event kinds;
- adapter capability discovery;
- mutation isolation validation.

Do not rename or move it merely for V3 aesthetics.

Missing durable layer:

```text
AgentIdentity
  1 → N AgentActivation

Session/Event truth
  survives process death
```

The existing provider `Session` becomes a live activation/runtime handle, not durable identity.

### 5. Durable event truth is the first missing foundation

`internal/state.AppendJSONL` already supplies a private append-only storage primitive, but no canonical runtime-event store owns sequencing, idempotency, snapshots, replay, corruption handling, or model-surface separation.

Therefore the first V3 code package is `internal/runtimeevents`.

It must provide:

- append-only ordered task events;
- monotonic sequence numbers;
- idempotency-key deduplication;
- private on-disk storage;
- replay from a sequence;
- durable snapshots/checkpoints;
- strict corruption detection;
- no raw chain-of-thought requirement;
- payloads suitable for later redaction/projection.

## Phase ordering after audit

The audit confirms the V3 ordering from the master plan:

```text
A. architecture delta + benchmark contracts
B. ecosystem assimilation foundation (independent R&D lane)
C. durable event truth
D. context projection + token economy
E. durable agent identity/activation/graph/mailbox
F. worktree + patch concurrency
G. policy kernel
H. coding kernel + verification
I. failure intelligence
J. tool program runtime
K. adaptive multi-agent
L. evidence-based router
M. verified experience/brain learning
N/O. runtime providers + browser/computer/mobile verification
P/Q. observability + UX
R/S. security + chaos hardening
T/U/V. migration + release + flagship acceptance
```

The critical dependency chain is C → D → E → F/G → H. Multi-agent work before this chain is prohibited.

## Baseline test gates

Before changing production behavior, preserve these gates:

- `go test ./...`
- `go vet ./...`
- `go build ./cmd/...`
- existing agentruntime tests remain green;
- existing taskexecution tests remain green;
- existing security tests remain green;
- existing editing behavior remains backwards compatible.

GitHub PR CI runs these gates on Ubuntu, macOS and Windows.

## Phase A exit criteria

Phase A is complete when:

1. V3 master plan and execution checklist exist.
2. Architecture delta is tied to concrete current packages and files.
3. Existing primitives to preserve are explicit.
4. Missing primitives and implementation ordering are explicit.
5. The implementation branch is isolated from `main`.
6. The first production package can be built without a big-bang migration.
7. No existing runtime behavior has changed yet.

## First implementation slice

Start with `internal/runtimeevents` only.

Do not wire it into agent execution in the same commit.

Reason: the event format/store must prove ordering, replay, idempotency and corruption behavior independently before it becomes authoritative runtime truth.

After CI passes, wire one shadow-write path behind a feature flag/runtime generation and compare reconstructed state against existing task state before making it authoritative.
