# CodeLocal Agent OS V3 — A→Z Execution Checklist

Status: Execution companion to `CODELOCAL_AGENT_OS_V3_MASTER_PLAN.md`
Date: 2026-08-30
Owner: CodeLocal

> This file is the implementation control board. Each phase has dependencies, primary code areas, deliverables, tests, benchmark gates, and explicit exit criteria. Do not skip gates merely to move faster.

---

# 0. Global execution rules

- Never implement V3 as a big-bang rewrite.
- Keep `main` releasable.
- Use dedicated feature branches per phase or tightly-coupled slice.
- Prefer migration of existing mature packages over duplicate subsystems.
- Preserve the compact public MCP surface.
- Every phase must have a measurable reason: quality, reliability, token, latency, cost, security, or UX.
- Every runtime-affecting phase must include recovery and backward-compatibility tests.
- Every mutation subsystem must include stale/concurrency tests.
- Every security phase must include fail-closed tests.
- Every optimization phase must include verified-success regression checks.
- External ecosystem ideas are inputs, not authorities.

Suggested branch prefixes:

```text
plan/
research/
experiment/
feat/
fix/
```

Suggested feature flags:

```text
agent_runtime_v2
context_surface_v2
runtime_events_v2
worktree_v2
patch_engine_v2
policy_kernel_v2
tool_program_runtime
failure_intelligence
ecosystem_assimilation
```

---

# A. PREPARATION — freeze baseline before changing core

## A1. Repository architecture audit

Dependencies: none

Inspect and document current ownership for:

```text
internal/agentruntime/
internal/orchestration/
internal/taskexecution/
internal/taskstate/
internal/security/
internal/approval/
internal/editing/
internal/process/
internal/runtime/
internal/project/
internal/projectidentity/
internal/memory/
internal/learnedskills/
internal/mcphub/
internal/history/
internal/idempotency/
```

Deliverables:

- current package map;
- current state owners;
- duplicated responsibilities;
- migration targets;
- APIs that must remain backward compatible;
- dead/legacy pathways that V3 can eventually remove.

Exit criteria:

- every V3 primitive has a known current owner or clearly identified gap.

## A2. Baseline test inventory

Record:

```text
unit test commands
integration test commands
CLI smoke tests
MCP contract tests
security smoke tests
runtime reconnect tests
browser/computer tests
```

Exit criteria:

- baseline suite is runnable and result stored before architectural edits.

## A3. Benchmark corpus baseline

Create initial representative tasks for:

```text
Go
TypeScript/Next
Python
Rust
Flutter or mobile project
```

At minimum include:

```text
tiny bug
multi-file bug
refactor
test failure
frontend flow
backend flow
```

Capture:

```text
success
time
tokens if available
model calls
tool calls
retries
human intervention
```

Exit criteria:

- current CodeLocal has a repeatable benchmark baseline.

---

# B. ECOSYSTEM ASSIMILATION FOUNDATION

Parent: `ECOSYSTEM_ASSIMILATION_ENGINE_PLAN.md`

## B0. Source Registry

Primary target:

```text
internal/ecosystem/
```

Deliver:

- source registry structs;
- source versioning;
- enabled/disabled sources;
- cursor state;
- no dynamic execution.

Tests:

- registry parse;
- duplicate source IDs rejected;
- bad source config fails loud;
- disabled source is not polled.

Exit:

- source registry deterministic and versioned.

## B1. GitHub Radar adapter

Initial support:

```text
topic discovery
repository snapshots
release/version deltas
```

Tests:

- first scan creates candidates;
- identical second scan creates no duplicate work;
- changed revision creates new candidate revision;
- archived repository state recorded.

Exit:

- can track the DSH ecosystem incrementally.

## B2. CapabilityCandidate + Genome

Deliver canonical schema and taxonomy.

Tests:

- serialization/versioning;
- unknown capability class handled safely;
- immutable source revision;
- overlap fields can reference existing CodeLocal capability IDs.

Exit:

- one report shape for all external sources.

## B3. Provenance + License Gate

Deliver:

```text
source URL
revision
version
license hash
license class
reuse status
```

Tests:

- no-license => direct reuse blocked;
- license change => candidate re-gated;
- provenance missing => Lab blocked.

Exit:

- no candidate can lose source lineage.

## B4. Static Security Gate

Deliver scanners for:

```text
install scripts
build hooks
process spawn
network
filesystem/home access
secret/env access
binary download
privileged container usage
```

Use fixtures containing malicious patterns.

Exit:

- known-danger fixtures cannot reach dynamic Lab.

## B5. Lab Sandbox

Prefer provider-neutral RuntimeProvider contract where possible.

Security defaults:

```text
no host secrets
no user home
network deny-all
synthetic repo
bounded CPU/RAM/time
full process/file/network audit
```

Exit:

- malicious fixture cannot escape test sandbox.

## B6. Candidate Benchmarks

Start with at least:

```text
context candidate
permission/runtime candidate
```

Exit:

- baseline vs candidate vs distilled prototype can be compared reproducibly.

## B7. Pattern Distiller + Promotion state machine

Disposition enum:

```text
NATIVE_CORE
ADAPTER
SKILL
LAB_EXPERIMENT
WATCH
REJECT
```

Exit:

- end-to-end candidate produces a reviewed architecture proposal without auto-merging code.

---

# C. DURABLE EVENT FOUNDATION

## C0. Canonical entity contracts

Freeze versioned V3 structs for:

```text
Session
Task
AgentIdentity
AgentActivation
AgentEdge
RuntimeEvent
TaskNode
MailboxMessage
Worktree
PatchSet
PolicyDecision
ContextPacket
VerificationResult
Savepoint
```

Do not overfit provider-specific fields.

Exit:

- schema review completed before persistence implementation.

## C1. Runtime Event Store

Requirements:

```text
append-only
sequence
idempotency
snapshot
replay
redaction
schema version
crash safety
```

Tests:

- duplicate append key is idempotent;
- monotonic sequence;
- corrupted tail handled explicitly;
- replay yields expected state;
- secret-like payload is redacted according to policy.

Exit:

- durable event truth works independently from current hot state.

## C2. Projection Engine

Implement:

```text
TaskStateProjection
AgentGraphProjection
ActivationProjection
UIProgressProjection
AuditProjection
RecoveryProjection
UsageProjection
```

Tests:

- rebuild from zero using events;
- snapshot + tail equals full replay;
- duplicate/reordered invalid inputs fail predictably.

Exit:

- hot state may be reconstructed after restart.

## C3. Restart recovery smoke test

Scenario:

```text
start task
append several state changes
simulate runtime crash
restart
reconstruct
continue
```

Exit:

- no duplicated side effect and correct task state restored.

---

# D. CONTEXT SURFACE & TOKEN ECONOMY

## D0. Context Surface contract

Inputs:

```text
Task objective
Project Brain
Project map
symbol graph
recent runtime events
verification state
```

Output:

```text
ContextPacket
```

Tests:

- mandatory rules cannot be dropped;
- untrusted repository text remains clearly separated;
- deterministic fingerprint for identical packet.

Exit:

- model-visible context has one canonical assembly path.

## D1. Progressive retrieval

Order:

```text
project map
relevant directories
symbols
ranges
whole files
call/import graph
```

Measure repeated-file-read rate.

Exit:

- broad repo dump is no longer default retrieval path.

## D2. Token Meter

Track request composition.

Tests:

- counts aggregate per request/task/agent/engine;
- provider missing metadata uses marked estimate, not fake precision;
- cache tokens represented separately when available.

Exit:

- `tokens / verified task` can be computed.

## D3. Semantic Tool Reducers

Implement incrementally:

1. tests/build/compiler;
2. Git diff/status;
3. LSP/diagnostics;
4. browser/computer;
5. agent reports.

Tests:

- critical failure lines retained;
- file/line evidence retained;
- raw artifact remains referenceable;
- reducer output bounded.

Exit:

- raw tool logs no longer routinely enter model context.

## D4. Reversible Context Ledger

Implement:

```text
checkpoint
compress
search
expand
pin
```

Tests:

- compressed history remains durable;
- search can find original hidden evidence;
- expand reconstructs source references;
- checkpoint never claims details absent from source evidence.

Exit:

- long-running session can reduce context without destructive loss.

## D5. Budget Manager

Inputs:

```text
model context limit
task complexity
phase
remaining task budget
retrieval confidence
```

Actions:

```text
reduce
prune
compact
expand only when needed
```

Exit benchmark:

- median input token use decreases with no verified-success regression.

---

# E. DURABLE AGENT RUNTIME

## E0. AgentIdentity

Create durable identity independent of process.

Exit:

- agent survives activation restart in persisted state.

## E1. Activation Manager

Implement lifecycle:

```text
starting
running
waiting
checkpointed
stopped
failed
cancelled
```

Exit:

- same AgentIdentity may have multiple sequential activations safely.

## E2. Durable Agent Graph

Operations:

```text
spawn
children
descendants
join
wait
retry
cancel subtree
recover orphan
```

Tests:

- graph survives restart;
- cycle rejected;
- child status aggregation deterministic.

Exit:

- graph topology is durable source-backed state.

## E3. Task DAG

Add dependency nodes and optimistic revision.

Tests:

- stale revision rejected;
- blocked node cannot run;
- completed dependency unblocks child;
- owner change requires expected revision.

Exit:

- plan can become executable dependency graph.

## E4. Durable Mailbox

Statuses:

```text
queued
delivered
acknowledged
expired
```

Tests:

- crash between queued/delivered recovers;
- message not delivered twice as new work;
- acknowledgement idempotent.

Exit:

- agent communication is not RAM-only.

---

# F. SAFE CONCURRENT MUTATION

## F0. Worktree Manager V2 design migration

Current known problem:

- source checkout cleanliness restriction blocks normal user + agent coexistence.

Target:

- authoritative checkout may be dirty;
- agent work happens in isolated worktree.

Deliver:

```text
worktree lease
owner agent
task base SHA
lifecycle
recovery
cleanup
```

Tests:

- dirty source accepted;
- agent cannot mutate authoritative checkout through normal V2 path;
- orphan worktree discovered after restart.

Exit:

- user can keep coding while task runs.

## F1. Three-way reconciliation

Inputs:

```text
base
latest authoritative user state
agent result
```

Tests:

- no-overlap auto-join;
- overlapping clean merge;
- true conflict produces structured conflict;
- no silent overwrite.

Exit:

- concurrency suite proves lost-update rate zero.

## F2. PatchSet V2

Add:

```text
baseSha
expectedHashes
provenance
state
```

Tests:

- stale base => `STALE`;
- expected hash mismatch => no write;
- atomic multi-file rollback;
- policy rejection => no mutation.

Exit:

- all V2 agent mutations are provenance-bearing and stale-safe.

## F3. Universal Savepoints

Start with task/message-level file state references + runtime checkpoint references.

Tests:

- rewind only agent-produced changes;
- preserve unrelated user changes;
- secret material excluded/redacted from export;
- invalid/corrupt savepoint fails safe.

Exit:

- user can undo agent step without requiring Git commit.

---

# G. POLICY KERNEL V2

## G0. Canonical PolicyDecision

Every privileged subsystem maps to one decision type.

Exit:

- edit/run/git/network/browser/computer/mobile/external MCP can share policy semantics.

## G1. Declarative rule DSL

Start bounded:

```text
action
tool/command
agent role
path
network target
workspace
risk
```

Features:

```text
ordered rules
allow/prompt/deny
schema validation
dry run
audit
hot reload only if safe
```

Tests:

- bad rule file retains previous valid config or fails closed according to mode;
- rule evaluation deterministic;
- deny cannot be relaxed by a later lower-authority layer.

Exit:

- policy rules are auditable and testable without model calls.

## G2. Capability Lending

Implement one-shot leases.

Tests:

- lease valid only actor/action/target scope;
- lease expires after use/TTL;
- child agent cannot inherit wider capability unless explicitly allowed;
- lease never becomes remembered global approval automatically.

Exit:

- normal work remains sandboxed without forcing persistent Full Access.

## G3. Secret policy formalization

Create explicit tests for:

Allowed candidate:

```text
approved process internally consumes token for approved HTTPS request
secret value never appears in model/log
```

Blocked:

```text
echo $TOKEN
env
printenv
read credential file and return contents
```

Exit:

- exposure and internal consumption are no longer conflated.

## G4. Dynamic Capability Projection

Before model request, project actual available tool schemas.

Tests:

- all-access session does not advertise impossible escalation fields;
- restricted session advertises only viable widening;
- approval=never hides unusable approval/escalation options;
- Native and Tool Program SDK surfaces match.

Exit:

- invalid permission-induced retry loops removed in regression fixtures.

---

# H. NATIVE CODING QUALITY

## H0. Verification-plan-first workflow

Task creation emits expected verification plan before mutation.

Exit:

- coding loop always knows what evidence is required for success.

## H1. UNDERSTAND/LOCATE

Use context + semantic/LSP retrieval.

Measure:

```text
files opened
repeated reads
irrelevant context ratio
```

Exit:

- focused bugs locate correct symbols with bounded context.

## H2. PLAN/ACT

Plan must include:

```text
root cause hypothesis
files/symbols
mutation boundaries
verification
fallback
```

Exit:

- no blind repo-wide edits for focused tasks.

## H3. OBSERVE/VERIFY

Feed reduced structured observations.

Verification ladder:

```text
syntax
format
static
unit
package
integration
full/product checks when needed
```

Exit:

- `done` cannot be emitted as verified without evidence.

## H4. REFLECT/REPAIR

Targeted retries only.

Track repeated failure signature.

Exit:

- same failed action is not blindly repeated indefinitely.

## H5. Single-agent benchmark gate

Compare focused coding corpus against selected external baseline.

Exit target:

- competitive verified success;
- no security regression;
- reasonable token/task.

Do not proceed to aggressive multi-agent optimization if single-agent kernel is weak.

---

# I. FAILURE INTELLIGENCE

## I0. Failure taxonomy

Implement structured reason codes.

Exit:

- retries no longer depend on parsing arbitrary error strings only.

## I1. Detect + Evidence bundle

Capture:

```text
visible failure
preceding decisions
context packet fingerprint
tool calls
patch state
verification failures
agent ownership
```

Exit:

- failure report is reproducible.

## I2. Attribution

Start deterministic/heuristic before expensive LLM judge.

Possible methods:

```text
last causal mutation
failure signature mapping
dependency chain
binary search across event ranges later
```

Exit:

- common failure classes identify likely responsible step/agent.

## I3. Recovery strategy

Strategies:

```text
refresh context
re-read stale file
rebase patch
change test focus
stronger model
specialist reviewer
restart activation
switch runtime provider
```

Exit:

- recovery selected from failure class and evidence.

## I4. Branch rerun

Use savepoint/checkpoint to run bounded alternative recovery.

Exit:

- system can compare before/after verification rather than trusting recovery proposal.

## I5. Error corpus

Promote verified failure/recovery pairs into regression fixtures.

Exit:

- repeated failure classes become cheaper to diagnose over time.

---

# J. TOOL PROGRAM RUNTIME

## J0. Capability SDK projection

Generate bounded callable SDK from current projected capabilities.

Exit:

- Tool Program can call only session-valid operations.

## J1. Program sandbox

Block implicit access to:

```text
raw filesystem
raw environment
raw child process
raw network
```

unless exposed through explicit bindings.

Exit:

- model code cannot bypass Policy Kernel.

## J2. Resource limits

Bound:

```text
time
memory
operations
output bytes
nested tool calls
```

Exit:

- runaway program terminates safely.

## J3. Benchmark

Compare repeated native tool loops vs Tool Program for:

```text
search/read/filter
repo metadata aggregation
log analysis
```

Exit:

- promote only workflows with measurable token/latency win and no quality loss.

---

# K. ADAPTIVE MULTI-AGENT

## K0. Task complexity classifier

Output T0–T4 plus confidence.

Exit:

- trivial/focused tasks do not spawn teams by default.

## K1. Child brief contract

Mandatory:

```text
objective
scope
read/write limits
context budget
tool budget
verification expectation
termination criteria
```

Exit:

- no vague “explore the repo” child jobs in normal scheduler path.

## K2. Parallel scheduler

Use Task DAG + Agent Graph + Worktree leases.

Exit:

- parallel work only when dependencies/scopes allow it.

## K3. Join/review path

Pipeline:

```text
children complete
dependency check
conflict check
review
join candidate
verification
reconcile
```

Exit:

- multi-agent result cannot bypass final verification.

## K4. Multi-agent benchmark gate

Compare one-agent vs team mode on same tasks.

Exit:

- team mode enabled only for classes where it improves success/time enough to justify extra tokens/cost.

---

# L. ENGINE ROUTER V2

## L0. Engine capability registry

Probe real capabilities/version/auth/isolation.

Exit:

- routing never assumes capabilities only from provider name.

## L1. Historical verified metrics

Store by engine/task class/repo language where statistically useful:

```text
success
tokens
cost
latency
retries
```

Exit:

- routing can learn from verified outcomes.

## L2. Auto scoring

Inputs:

```text
quality
reliability
capability fit
historical success
token efficiency
cost
latency
remaining budget
isolation
availability
```

Exit:

- score explanation is auditable.

## L3. Evidence-based escalation

Policy example:

```text
cheap viable engine
 -> failure with evidence
 -> stronger engine
 -> reviewer/specialist
```

Exit:

- escalation is bounded by attempts/token/cost and does not loop forever.

---

# M. EXPERIENCE + PROJECT BRAIN V2

## M0. Evidence-backed ExperienceCandidate

No task failure or unverified model assertion promotes directly.

Exit:

- durable learning requires verification evidence.

## M1. Scope and validity

Add support for:

```text
global/project/workspace
branch/revision relevance
confidence
lastVerifiedAt
```

Exit:

- old architecture fact can become stale instead of poisoning context forever.

## M2. Failure/recovery learning

Promote:

```text
failure signature
root cause evidence
successful recovery
verification
```

Exit:

- future matching tasks can retrieve prior proven recovery.

## M3. Learned Skills V2

Skill contract includes:

```text
trigger
preconditions
context selectors
workflow
capabilities
verification
risk
success evidence
```

Exit:

- skill reuse reduces reads/calls/tokens in benchmark without lowering quality.

---

# N. LOCAL/CLOUD RUNTIME PARITY

## N0. RuntimeProvider contract audit

Map current Local Runtime and Cloud Sandbox requirements onto one provider-neutral interface.

Exit:

- agent logic does not hard-code local-only process/filesystem assumptions unnecessarily.

## N1. Cloud lifecycle

Implement/verify:

```text
cold
provisioning
ready
active
idle
checkpointed
sleeping
terminated
```

Exit:

- idle sleep does not destroy resumable task state.

## N2. Capability on demand

Provision browser/GUI/mobile only when task requires them.

Exit:

- compute cost is not paid for unused capabilities.

## N3. Cross-runtime resume

When allowed and technically valid, task/agent state can be reconstructed after provider restart.

Exit:

- durable task truth remains independent of one machine process lifetime.

---

# O. BROWSER / COMPUTER / MOBILE VERIFICATION

## O0. Browser verification adapter

Convert browser evidence to canonical VerificationResult inputs.

Tests:

```text
DOM state
console errors
network failures
form interactions
screenshot references
```

Exit:

- frontend task can prove behavior, not just compile.

## O1. Computer verification

Use semantic accessibility-first interactions.

Exit:

- native desktop workflow can be included in task verification trace.

## O2. Mobile verification

Integrate device/simulator execution without bloating public MCP surface.

Exit:

- mobile build + launch + interaction + evidence can occur inside same task graph.

---

# P. OBSERVABILITY / FLIGHT RECORDER

## P0. Trace propagation

One traceId across:

```text
task
agent
activation
engine
context
tool
process
policy
worktree
patch
verification
experience
```

Exit:

- support/admin can reconstruct a task journey.

## P1. Stream sequencing

Add:

```text
eventId
sequence
dedupeKey
reconnectCursor
```

Exit:

- reconnect does not repeat streamed text/events.

## P2. Usage/cost dashboard

Expose developer/admin metrics without cluttering normal chat.

Exit:

- benchmark and production metrics use the same telemetry contract.

## P3. Orphan visibility

Detect orphan:

```text
process
activation
worktree
mailbox job
cloud sandbox
```

Exit:

- orphan work never silently disappears.

---

# Q. UX V3

## Q0. User view

Keep simple:

```text
Task
current phase
subtasks
changes
verification
blocked approval only when necessary
```

Exit:

- no raw architecture noise in default flow.

## Q1. Advanced view

Optional:

```text
engine/model
tokens/cost
trace
events
worktrees
policy
verification details
```

Exit:

- experts can inspect without making basic UX overwhelming.

## Q2. Reconnect

User leaves and returns.

Expected:

```text
same task
current progress
no duplicated stream
no lost task state
```

Exit:

- reconnect UX passes automated and manual smoke tests.

---

# R. SECURITY HARDENING

Run after each major wave, not only at end.

Required suites:

```text
path escape
secret exposure
secret internal consumption
command redaction
network denial
capability lease expiry
agent child escalation
sandbox bypass
symlink/hardlink edge cases
malicious tool program
malicious ecosystem candidate
stale policy config
```

Exit:

- zero unresolved critical regressions.

---

# S. CHAOS / RECOVERY HARDENING

Inject:

```text
runtime kill
agent process kill
provider auth expiry
websocket drop
network outage
disk full
partial event append
corrupt checkpoint
Git conflict
stale patch
sandbox sleep
mailbox delivery crash
```

For each verify:

```text
state reconstruction
no duplicate side effect
clear user status
bounded retry
correct terminal state
```

Exit:

- recovery suite stable enough for RC.

---

# T. MIGRATION

## T0. Feature flags

V1 and V2/V3 paths coexist during rollout.

Exit:

- rollback possible without data loss.

## T1. Runtime generation

Every task records runtime generation/features.

Exit:

- support can explain which path executed a task.

## T2. Legacy history

Remain readable, do not force Project Brain into Event Store.

Exit:

- existing users/projects continue to load.

---

# U. RELEASE GATES

## Alpha

Must have:

```text
Event Store
Projection
Context Surface
Token Meter
single-agent kernel
baseline safe mutation
```

## Beta

Must have:

```text
Agent Graph
restart recovery
Worktree V2
Patch V2
Policy V2
Savepoints
```

## RC

Must have:

```text
Tool Program
adaptive multi-agent
Router V2
Failure Intelligence
Experience learning
Cloud parity
```

## Stable

Must pass:

```text
benchmark
security
chaos
migration
reconnect
lost-update zero gate
```

---

# V. FINAL FLAGSHIP ACCEPTANCE TEST

Setup:

- repository has uncommitted user changes;
- auth bug spans backend and frontend;
- test failure can be reproduced;
- one overlapping file will be changed by user during task;
- runtime will be intentionally restarted mid-task;
- browser verification is required.

User asks:

> Fix authentication end-to-end. Find the root cause, split work if useful, do not destroy what I am currently editing, test everything, and report only when it is actually verified.

Pass conditions:

1. Context Surface uses Project Brain without repo dump.
2. Verification plan exists before mutation.
3. Appropriate task/agent topology chosen.
4. Agent worktree isolation active.
5. User keeps editing authoritative checkout.
6. Raw tool output is semantically reduced.
7. Runtime restart recovers task.
8. Agent graph/mailbox state survives.
9. Overlapping user edit causes stale/conflict detection, never overwrite.
10. Reconciliation uses latest user state.
11. Tests pass.
12. Browser flow passes.
13. Final VerificationResult is PASS.
14. Only verified experience promotes to Brain.
15. Token/cost/trace metrics recorded.
16. No secret or approval boundary regression.

---

# W. STOP CONDITIONS

Pause a phase and fix architecture if any occurs:

```text
verified success materially drops
lost update observed
security boundary weakens
resume duplicates side effects
context optimization hides required evidence
multi-agent costs more without outcome benefit
third-party candidate executes before gates
public MCP surface explodes
provider-specific assumptions leak into core
```

Do not continue stacking features on a broken foundation.

---

# X. MEASUREMENT BOARD

Track continuously:

```text
VerifiedSuccessRate
TokensPerVerifiedTask
CostPerVerifiedTask
TimePerVerifiedTask
ModelCallsPerTask
ToolCallsPerTask
RetryWaste
DuplicateContextRatio
ContextCacheHitRate
BrainReuseRate
HumanInterventions
ResumeSuccessRate
LostUpdateCount
SecurityRegressionCount
BrowserVerificationRate
MobileVerificationRate
```

A phase is successful only when the targeted metric improves or a required invariant becomes enforceable.

---

# Y. DELIVERY REPORT FORMAT

Every completed phase should report:

```markdown
## Phase

### Branch / commits

### Changed packages/files

### New contracts

### Migrations/flags

### Tests run

### Benchmark delta

### Security impact

### Known limitations

### Rollback path

### Next phase
```

This format becomes the handoff contract across long upgrade work.

---

# Z. FINAL DONE STATE

CodeLocal V3 is done only when it behaves as an Agent OS rather than a tool wrapper:

```text
User objective
   -> durable task
   -> bounded context
   -> adaptive reasoning engine
   -> safe execution
   -> concurrent isolated mutation
   -> evidence-based recovery
   -> independent verification
   -> durable experience
   -> better next task
```

Final bar:

- competitive coding quality;
- lower waste on warm projects;
- reliable unattended task continuation;
- zero silent lost updates;
- unified security;
- model/provider replaceability;
- local/cloud/device continuity;
- ecosystem learning without dependency chaos.

> **The product is complete when a better model or a better open-source idea makes CodeLocal stronger without forcing the user to rebuild their workflow or trust boundary.**
