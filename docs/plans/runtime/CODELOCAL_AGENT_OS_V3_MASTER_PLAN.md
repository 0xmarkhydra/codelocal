# CodeLocal Agent OS V3 — Master Architecture & A→Z Upgrade Plan

Status: Proposed master execution blueprint
Date: 2026-08-30
Owner: CodeLocal
Branch: `plan/agent-os-v3-assimilation`
Primary target: native Go runtime + CodeLocal Cloud
Related plans:
- `docs/plans/runtime/UNIVERSAL_AGENT_RUNTIME_PLAN.md`
- Project Brain master plan

> This document is the V3 architectural umbrella for the next full CodeLocal upgrade. It extends the Universal Agent Runtime into a complete Agent OS for software engineering. Existing plans remain useful implementation context, but where architecture differs, this document is the preferred direction for new V3 work.

---

# 0. Executive decision

CodeLocal must not become another Codex, another Claude Code, another DeepSeek Harness, or a large wrapper around many CLIs.

The target is a higher layer:

> **CodeLocal is the operating system for AI software engineering.**

External models and coding agents are replaceable reasoning/execution engines. CodeLocal owns the durable project intelligence, execution boundary, agent runtime, context economy, verification truth, experience, and cross-device continuity.

The product must simultaneously achieve four outcomes:

1. Coding quality at least comparable to the best available coding-agent baseline.
2. Lower unnecessary model context and lower tokens per verified task on warm projects.
3. Reliable autonomous execution without constant user babysitting.
4. Platform capabilities beyond a coding CLI: Project Brain, Local + Cloud, Browser, Computer, Mobile, external MCP, durable task recovery, and cross-model continuity.

North Star metric:

> **Verified software task completed correctly with the least user effort and the least unnecessary model context.**

Primary engineering metrics:

```text
Verified Success Rate
Tokens / Verified Successful Task
Human Interventions / Verified Task
Time / Verified Task
Retry Waste / Task
Lost Updates = 0
Security Regressions = 0
Resume Recovery Rate
```

---

# 1. Product positioning

The intended product relationship is:

```text
                      USER / TEAM
                          |
                          v
                  CODELOCAL AGENT OS
                          |
        +-----------------+-----------------+
        |                 |                 |
        v                 v                 v
   Project Brain      Runtime Kernel    Intelligence Layer
        |                 |                 |
   Knowledge           Files             Native Agent
   Decisions           Git               Codex
   Skills              Process           Claude
   Experience          Browser           DeepSeek
   Rules               Computer          Gemini
   History             Mobile            OpenCode
                       Cloud             Future engines
                          |
                          v
                     Verification
                          |
                          v
                       Learning
```

Models change. Provider rankings change. CodeLocal must improve even when the selected model changes.

The durable moat belongs to CodeLocal:

```text
Project identity
Verified facts
Architecture knowledge
Rules
Skills
Failure/recovery experience
Routing history
Verification history
Agent execution history
Context checkpoints
User/team working conventions
```

---

# 2. Architectural DNA

V3 intentionally absorbs proven patterns from several systems without making them runtime dependencies.

## 2.1 Codex DNA

Absorb conceptually:

- tight coding loop;
- inspect → reason → edit → run → observe → repair;
- durable agent topology;
- process/sandbox discipline;
- patch-based mutation;
- verification mindset;
- resumable sessions;
- provider-specific agent roles.

Do not copy as core dependencies:

- OpenAI-specific assumptions;
- Codex UI/TUI architecture;
- Rust implementation wholesale;
- provider-owned memory as source of truth.

## 2.2 DeepSeek Harness DNA

Absorb conceptually:

- append-only session/event truth;
- model-visible projections separated from durable history;
- token meter + compaction;
- tool-result pruning;
- programmatic tool calling;
- continuable agent identity separated from process activation;
- durable mailbox;
- task DAG;
- capability seams;
- optimistic concurrency;
- session-aware capability projection.

Do not copy as core dependencies:

- everything-is-a-plugin as a universal runtime rule;
- Node runtime as CodeLocal core;
- shared-checkout multi-agent mutation;
- worker-thread containment treated as a security boundary;
- preview-grade security assumptions.

## 2.3 CodeLocal DNA

Preserve and strengthen:

- Project Brain;
- Project Identity;
- Learned Skills;
- native Go runtime;
- workspace authorization;
- transaction-safe editing;
- expected-hash stale protection;
- Local Runtime;
- Cloud Sandbox;
- Computer / Browser / Mobile;
- MCP Hub;
- approval memory;
- secret/path policy;
- model router;
- user + agent coexistence;
- compact public MCP surface.

## 2.4 Ecosystem DNA

Community ecosystems become an R&D input, not a production dependency stream.

Examples of concepts worth evaluating:

- reversible context compression and decompression;
- message/task savepoints and rewind;
- declarative permission rules;
- one-shot capability lending;
- token/cost governors;
- multi-agent dispatch tiers and escalation;
- failure attribution and rerun;
- memory evolution and branch-scoped knowledge;
- plugin validation and marketplace governance;
- remote/device/browser/vision workflows.

These are processed through the Ecosystem Assimilation Engine defined in the dedicated plan.

---

# 3. Core invariants

These are non-negotiable.

## INV-1 — Model reasons, Runtime executes

Model output is never the execution authority by itself.

## INV-2 — Project Brain != Runtime State

Long-lived knowledge is separate from task/session/process state.

## INV-3 — Durable Event Truth != Model Context

Full runtime history is preserved independently from the subset shown to the model.

## INV-4 — Provider Result != Verified Success

Only CodeLocal Verification may mark task completion as verified.

## INV-5 — User workspace is authoritative

Agents must never silently overwrite the user's current uncommitted work.

## INV-6 — Parallel agents are isolated by default

Per-task/per-agent worktrees remain the preferred mutation boundary.

## INV-7 — Optimistic concurrency everywhere

Stale file/task/patch/worktree/brain state must fail safely instead of overwriting newer state.

## INV-8 — Security decisions are monotonic

A later layer may tighten but may not silently relax a stronger prior decision.

```text
ALLOW < PROMPT < DENY
```

## INV-9 — Model-visible capabilities must be real

The model should not see tool parameters/capabilities that cannot succeed in the current session state.

## INV-10 — Public MCP remains compact

Do not expose every internal primitive as a separate tool.

## INV-11 — Context follows a budget

Do not dump the repository, raw terminal logs, or entire history into the model by default.

## INV-12 — Task state survives disconnect/restart

Closing a client or restarting a runtime should not silently erase eligible task progress.

## INV-13 — Every important action has provenance

Task, agent, process, worktree, patch, policy, verification, engine, context packet, and learned experience must be traceable.

---

# 4. Target architecture

```text
                          USER / CLIENT
                               |
                               v
                        Intent Compiler
                               |
                               v
                         PROJECT BRAIN
            Rules / Knowledge / Skills / Experience
                               |
                               v
                    CONTEXT SURFACE ENGINE
          +--------------------+--------------------+
          |                    |                    |
          v                    v                    v
      Retrieval           Token Meter          Compaction
          |                    |                    |
          +----------- Semantic Reduction ---------+
                               |
                               v
                           Task Planner
                               |
                               v
                           Task DAG
                               |
                               v
                          Agent Graph
                               |
                         Durable Mailbox
                               |
            +------------------+------------------+
            |                  |                  |
            v                  v                  v
        Native Agent          Codex          Claude/DSH/...
            |                  |                  |
            +------------------+------------------+
                               |
                               v
                       Activation Manager
                               |
                               v
                         Runtime Kernel
             +-----------------+-----------------+
             |                 |                 |
             v                 v                 v
        Policy Kernel    Worktree Manager   Process Supervisor
             |                 |                 |
             +-----------------+-----------------+
                               |
                               v
                        Tool Orchestrator
                   Native Call / Tool Program
                               |
                               v
                          Patch Engine
                               |
                               v
                       Verification Engine
                               |
                               v
                        Experience Engine
                               |
                               v
                          Project Brain
```

---

# 5. Three truth planes

A major V3 design decision is separating durable truth from what the model sees.

## 5.1 Project Brain — durable knowledge plane

Stores long-lived, sanitized, evidence-aware knowledge:

```text
architecture
rules
decisions
verified facts
skills
verified experiences
known failure/recovery patterns
project conventions
routing/verification insights
```

Brain entries should carry:

```text
source
scope
confidence
validity
lastVerifiedAt
branch/revision applicability when relevant
provenance
```

## 5.2 Runtime Event Log — durable execution plane

Append-only runtime truth:

```text
task.created
task.planned
agent.spawned
activation.started
context.compiled
worktree.created
tool.started
tool.finished
policy.evaluated
patch.proposed
patch.applied
verification.started
verification.failed
agent.retry
verification.passed
task.completed
```

## 5.3 Model Context Surface — temporary reasoning plane

The model receives only the currently useful projection:

```text
objective
mandatory rules
relevant architecture
relevant verified experience
active files/symbols
current failure evidence
current diff summary
next verification expectation
```

This surface may compact, prune, search, or expand without deleting the durable event history.

---

# 6. Runtime Event Store

## 6.1 Event contract

Conceptual shape:

```text
RuntimeEvent
  id
  seq
  timestamp
  traceId
  sessionId
  taskId
  agentId nullable
  activationId nullable
  processId nullable
  worktreeId nullable
  patchId nullable
  kind
  safePayload
  artifactRefs[]
```

Requirements:

- append-only;
- per-stream monotonic sequence;
- idempotency key support;
- snapshots for bounded replay;
- redaction before persistence where needed;
- migration/version field;
- crash-safe writes;
- deterministic projection;
- bounded retention for large raw artifacts;
- structured references to local raw logs instead of stuffing them into events.

## 6.2 Projection engine

Event log consumers must not reimplement their own ad hoc logic.

Required projections:

```text
TaskStateProjection
AgentGraphProjection
ActivationProjection
UIProgressProjection
ContextSurfaceProjection
UsageProjection
AuditProjection
RecoveryProjection
```

Projection rebuild must be tested from an empty in-memory state using only durable events + snapshots.

---

# 7. Reversible Context Ledger

Context management becomes a first-class subsystem.

## 7.1 Goals

- keep full durable evidence;
- reduce model context aggressively but safely;
- allow later search/expansion of older evidence;
- avoid repeated full-history summarization;
- support long-running tasks;
- preserve file paths, decisions, errors, verification facts, and unresolved work.

## 7.2 Context states

```text
ACTIVE
COMPACTED
ARCHIVED
EXPANDABLE
PINNED
```

## 7.3 Compression contract

A compacted range produces a checkpoint node containing:

```text
range/start
range/end
summary
keyFacts[]
fileRefs[]
decisions[]
failures[]
openQuestions[]
verificationState
originalEventRefs[]
```

Original events remain durable.

## 7.4 Operations

Internal operations:

```text
compress(range)
expand(checkpoint)
search(query, scopes)
reconstruct(window)
pin(ref)
unpin(ref)
```

These should not necessarily become public MCP tools.

---

# 8. Context Surface Engine

## 8.1 Progressive retrieval

Use levels:

```text
L0 repository/project map
L1 relevant directories
L2 symbols
L3 bounded source ranges
L4 whole files only when needed
L5 dependency/call graph neighborhood
```

## 8.2 Context packet

Conceptual packet:

```text
ContextPacket
  objective
  mandatoryRules
  relevantKnowledge
  relevantExperience
  activeSymbols
  activeFiles
  currentFailure
  currentPatchSummary
  currentVerificationState
  unresolvedQuestions
  tokenBudget
  fingerprint
```

## 8.3 Delta context

After checkpoint, prefer:

```text
SINCE CHECKPOINT
Changed:
- auth/service.go
- auth/service_test.go

Verification:
FAIL TestRefreshToken

Observation:
race around token rotation

Next:
inspect lock strategy
```

Do not replay all previous successful steps.

---

# 9. Token Meter and Token/Cost Governor

## 9.1 Measure every request

Track at least:

```text
systemTokens
brainTokens
historyTokens
codeTokens
retrievalTokens
toolObservationTokens
outputTokens
cacheReadTokens
cacheWriteTokens
estimatedCost
actualCost when provider exposes it
```

## 9.2 Primary KPI

```text
Tokens / Verified Successful Task
```

Do not optimize token count if success rate drops.

## 9.3 Governor

Task budgets may include:

```text
maxModelCalls
maxInputTokens
maxOutputTokens
maxEstimatedCost
maxRetryWaste
```

Router and scheduler use remaining budget as a control input.

## 9.4 Targets

Initial target framework:

```text
Cold CodeLocal V3 <= best baseline token/task at comparable quality
Warm CodeLocal V3 < cold CodeLocal V3
Repeated-domain warm tasks continue improving
```

Avoid hard marketing claims until benchmark evidence exists.

---

# 10. Semantic Tool Result Reduction

Raw tool output should stay local whenever possible.

Pipeline:

```text
Raw Tool Output
      |
      v
Domain Parser
      |
      v
Semantic Reducer
      |
      v
Deterministic Pruner
      |
      v
Context Surface
      |
      v
LLM Compaction only if still necessary
```

Required reducer classes:

```text
TestReducer
BuildReducer
CompilerReducer
GitDiffReducer
GitStatusReducer
LSPReducer
DependencyReducer
BrowserReducer
ComputerReducer
NetworkReducer
AgentReportReducer
```

Each reducer must preserve evidence references to the raw artifact.

---

# 11. Agent identity and activation

A durable agent is not the same thing as a live process.

```text
AgentIdentity
  id
  taskId
  role
  parentAgentId
  enginePreference
  status
  attempt
  contextFingerprint

AgentActivation
  id
  agentId
  runtimeProvider
  processId
  startedAt
  endedAt
  status
  checkpointRef
```

One agent may have multiple activations across:

- runtime restart;
- Cloud sleep/wake;
- local device reconnect;
- retry after process crash;
- engine handoff where policy allows.

---

# 12. Durable Agent Graph

Roles may include:

```text
lead
investigator
implementer
reviewer
tester
security-reviewer
specialist
```

Edges:

```text
spawn
delegate
dependency
review
retry
handoff
```

Required operations:

```text
Spawn
Children
Descendants
Join
Wait
Retry
CancelSubtree
Resume
RecoverOrphans
AggregateProgress
```

The topology must survive runtime restart.

---

# 13. Task DAG and durable mailbox

## 13.1 Task DAG

Large work becomes machine-readable dependencies instead of plan prose only.

Task node fields:

```text
id
revision
objective
ownerAgentId
status
blockedBy[]
readScopes[]
writeScopes[]
verificationPlan
budget
```

Mutations use compare-and-set expected revision.

## 13.2 Durable mailbox

Agent-to-agent communication must survive restart.

```text
MailboxMessage
  id
  senderAgentId
  recipientAgentId
  taskId
  createdAt
  payload
  status queued|delivered|acknowledged|expired
```

Do not rely on in-memory direct callbacks as the only transport.

---

# 14. Worktree Manager V2

Current source-checkout cleanliness restrictions must be removed from the target UX.

Goal:

> A user with uncommitted work can start an agent task and continue coding while agents operate safely in isolated worktrees.

Worktree model:

```text
Worktree
  id
  taskId
  ownerAgentId
  repository
  path
  baseBranch
  baseSha
  branch
  lease
  status
```

Statuses:

```text
preparing
ready
active
dirty
merge_ready
conflicted
merged
discarded
orphaned
```

Join uses three-way reconciliation:

```text
base
+
latest user state
+
agent result
```

Never assume the authoritative checkout stayed unchanged.

---

# 15. Patch Engine V2

Preserve existing strengths:

- ExpectedHash;
- sensitive-path policy;
- atomic writes;
- overlap detection;
- rollback;
- `git apply --check` style dry validation.

Add first-class `PatchSet`:

```text
PatchSet
  id
  taskId
  agentId
  worktreeId
  baseSha
  files[]
  expectedHashes
  provenance
  state
```

States:

```text
proposed
validated
stale
applied
reverted
conflicted
rejected
```

Pipeline:

```text
PatchPlan
  -> resolve base
  -> validate scope/hash
  -> dry-run
  -> policy
  -> atomic apply
  -> inspect diff
  -> diagnostics
  -> verification
```

If expected state differs from current state, return a structured stale/conflict result. Never overwrite silently.

---

# 16. Universal Savepoint Engine

V3 introduces savepoints above Git commits.

Savepoint may capture references to:

```text
workspace revision
patch state
task state
agent graph state
context checkpoint
relevant config state
verification state
```

Goals:

- message-level/task-step rewind;
- safe undo of agent-produced changes;
- recovery from bad config/plugin changes;
- offline diagnostics for broken runtime state;
- no requirement that every checkpoint be a Git commit.

Potential operations:

```text
create savepoint
list timeline
preview rewind
rewind
redo when safe
export diagnostics
safe-mode recovery
```

All secret-bearing material must remain redacted or stored in local protected storage, never exported by default.

---

# 17. Policy Kernel V2

Policy becomes a single canonical decision layer across:

```text
read
edit
run
git
network
browser
computer
mobile
secret
worktree
agent
skill
external MCP
cloud capability
```

Conceptual decision:

```text
PolicyDecision
  decision allow|prompt|deny
  reason
  ruleId
  risk
  approvalKey
  rememberable
  capabilityLease nullable
  traceId
```

## 17.1 Declarative policy DSL

Support ordered rules with project/team/user scope.

Matching dimensions may include:

```text
action/tool
agent role
parameters
path
network target
workspace
platform
environment classification
risk
```

Rules must be schema-validated, bounded, fail-loud on invalid configuration, and support dry-run auditing.

## 17.2 Capability lending

For justified one-off widening:

```text
CapabilityLease
  actor
  capability
  target
  scope
  expiresAfterAction/TTL
  justification
  provenance
```

The lease must not silently become standing session authority.

## 17.3 Secret exposure vs secret consumption

Explicit rule:

- Secret value exposure to model/log/output is blocked.
- An approved local process may internally consume a secret for a legitimate API request if the secret never becomes model-visible or log-visible.

Examples blocked:

```text
echo $TOKEN
printenv
env
console.log(process.env.TOKEN)
```

Example potentially allowed under policy:

```text
process reads TOKEN internally
-> HTTPS request to approved service
-> secret redacted from logs/model
```

---

# 18. Dynamic capability projection

Before each model request, derive the actual capability surface from:

```text
workspace permissions
policy
sandbox mode
approval mode
runtime provider
engine capability
session state
network policy
available devices
```

The model-visible schema should exclude impossible actions/parameters.

Benefits:

- fewer invalid tool calls;
- fewer retry loops;
- less token waste;
- clearer reasoning;
- lower approval noise.

Native Tool Calling and Tool Program SDK generation must share the same projected capability surface.

---

# 19. Native Coding Kernel

Canonical loop:

```text
UNDERSTAND
   -> LOCATE
   -> PLAN
   -> ACT
   -> OBSERVE
   -> VERIFY
   -> REFLECT
   -> REPAIR or COMPLETE
```

## 19.1 Understand

Start with:

```text
objective
mandatory rules
project identity
relevant architecture
verified experience
likely packages/symbols
```

## 19.2 Locate

Use semantic-first retrieval before broad search.

## 19.3 Plan

Create verification criteria before mutation.

## 19.4 Act

Use structured edits/commands with scope and provenance.

## 19.5 Observe

Reduce raw outputs into evidence-rich compact observations.

## 19.6 Verify

Independent CodeLocal verification determines whether the objective is satisfied.

## 19.7 Reflect

Classify failures and decide targeted repair/escalation.

---

# 20. Tool Program Runtime

Programmatic Tool Calling is a major V3 optimization.

Instead of repeated model round trips:

```text
model -> search -> model -> read -> model -> read -> model
```

allow a bounded runtime program:

```text
model
  -> tool program
       search()
       read()
       filter()
       parse()
       crossReference()
  -> compact evidence
  -> model
```

Security rules:

- model-written code gets capability bindings, not arbitrary host APIs;
- no implicit raw `fs`, `child_process`, `process.env`, or unrestricted network;
- capability surface is session-projected;
- cloud execution uses a real sandbox;
- local execution remains Policy Kernel controlled;
- output is bounded and semantically reduced;
- execution has time/token/result limits.

Do not necessarily expose a public `run_code` tool. Prefer internal orchestration or a compact action on existing CodeLocal tools.

---

# 21. Adaptive multi-agent scheduler

Do not spawn multiple agents for every task.

Classification:

```text
T0 trivial
T1 focused
T2 multi-file
T3 architectural
T4 parallelizable
```

Suggested behavior:

```text
T0/T1: one agent
T2: one implementer + optional reviewer
T3/T4: agent graph with explicit dependencies
```

Child brief must include:

```text
objective
scope
context limit
allowed tools
write scope
token/cost budget
termination criteria
expected report
```

Children return compact `AgentReport` objects, not full transcripts.

---

# 22. Evidence-based engine escalation

The router should prefer the cheapest viable strategy first when risk/complexity allows, then escalate based on evidence.

Example:

```text
fast/cheap attempt
  -> success -> verify
  -> failure -> classify
       -> same engine + new evidence
       -> stronger model
       -> specialist/reviewer
       -> user only when required
```

Avoid wasting top-tier model usage on mechanical work by default.

---

# 23. Engine Registry and Auto Router

Engines may include:

```text
Native
Codex
Claude
Gemini
DeepSeek Harness
OpenCode
provider APIs
future engines
```

Capabilities:

```text
interactive
nonInteractive
structuredOutput
streaming
resume
cancel
fileEditing
shellExecution
MCP
planMode
reviewMode
subagents
browser
imageInput
sandbox
costMetadata
tokenMetadata
```

Auto scoring inputs:

```text
quality
reliability
capability fit
historical success
token efficiency
latency
cost
remaining budget
isolation strength
availability
```

Do not hard-code quality assumptions permanently by provider brand. Historical verified measurements should influence routing.

Default UX remains `Auto`.

---

# 24. Verification Engine

Verification plan is created before implementation.

Ladder:

```text
syntax
format
static diagnostics
affected unit tests
package tests
integration tests
full suite when justified
browser/computer/mobile product verification when relevant
```

Completion requirements:

```text
objective satisfied
patch valid
no unresolved conflict
required verification PASS
policy satisfied
workspace reconciliation PASS
```

Provider self-report is evidence only, never final truth.

---

# 25. Failure Intelligence Engine

Upgrade failure handling from simple retry to evidence-based diagnosis.

Pipeline:

```text
Detect
  -> Attribute
  -> Collect Evidence
  -> Select Recovery
  -> Branch Rerun
  -> Evaluate Outcome
  -> Promote or Discard Recovery
```

Failure taxonomy minimum:

```text
MODEL_FAILURE
CONTEXT_FAILURE
POLICY_DENIED
APPROVAL_REQUIRED
PROCESS_FAILURE
PATCH_STALE
PATCH_CONFLICT
TEST_FAILURE
ENVIRONMENT_FAILURE
ENGINE_FAILURE
AUTH_REQUIRED
NETWORK_FAILURE
RUNTIME_LOST
USER_CANCELLED
```

Attribution should answer:

```text
What visible failure occurred?
Which earlier step likely caused it?
Which agent/tool/context decision contributed?
What evidence supports the attribution?
What bounded recovery should be tried?
Did the rerun improve the result?
```

Verified failure/recovery pairs may become Project Brain experience and regression cases.

---

# 26. Experience Engine and Project Brain evolution

Only evidence-backed outcomes promote durable knowledge.

Pipeline:

```text
Verified Outcome
  -> Reflection
  -> Experience Candidate
  -> Trust Gate
  -> Scope/Validity tagging
  -> Project Brain
```

Experience should capture:

```text
trigger/task pattern
relevant context selectors
failure signature if any
actions taken
verification evidence
risk notes
success confidence
applicable scope
```

Branch/revision-scoped knowledge is required where architecture differs across branches.

Stale evidence reduces confidence instead of remaining permanently trusted.

---

# 27. Learned Skills V2

A Skill is executable verified knowledge, not merely prompt text.

```text
Skill
  trigger
  intent
  preconditions
  contextSelectors
  workflow
  capabilityRequirements
  verification
  riskRules
  successEvidence
  provenance
```

A good skill should reduce:

```text
repository reads
model calls
retries
context tokens
human intervention
```

---

# 28. RuntimeProvider — Local and Cloud parity

Canonical interface supports:

```text
LocalRuntime
CloudSandboxRuntime
FutureRuntime
```

Same logical capabilities:

```text
files
process
git
network
browser
computer
workspace
policy
events
artifacts
```

Cloud sandbox lifecycle:

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

Provision expensive capabilities only when required.

---

# 29. Browser / Computer / Mobile advantage

This is a key area where CodeLocal can exceed coding-only agents.

Frontend example:

```text
edit
 -> run dev server
 -> inspect browser
 -> click/fill
 -> observe DOM/console/network
 -> repair
 -> verify visual + functional state
```

Mobile example:

```text
edit
 -> build
 -> launch simulator/device
 -> interact
 -> inspect state/logs/screenshots
 -> repair
 -> verify
```

All actions belong to the same task graph and verification trace.

---

# 30. Observability — Engineering Flight Recorder

A single `traceId` links:

```text
session
task
agent
activation
engine/model
context packet
tool call
process
policy
worktree
patch
verification
skill
experience
```

User-facing progress stays simple:

```text
Fix authentication
✓ Understood repository
✓ Found root cause
● Implementing
  ├─ Backend ✓
  ├─ Tests ●
  └─ Review waiting
```

Advanced view may expose:

```text
engine
model
tokens
cost
trace
worktree
events
policy
verification
```

Streaming protocol requires sequence/eventId/dedupe/reconnect cursor to prevent duplicate text after reconnect.

---

# 31. Ecosystem Assimilation Engine

A new V3 subsystem continuously turns external open-source innovation into safe CodeLocal R&D input.

High-level pipeline:

```text
GitHub / papers / ecosystems
      -> discover
      -> classify
      -> extract capability pattern
      -> provenance/license check
      -> static security gate
      -> isolated lab benchmark
      -> compare against CodeLocal
      -> disposition:
           native core
           adapter
           skill
           experiment
           reject
```

Full design lives in:

`docs/plans/runtime/ECOSYSTEM_ASSIMILATION_ENGINE_PLAN.md`

Core rule:

> **Absorb knowledge and validated patterns; do not automatically absorb dependencies.**

---

# 32. Public MCP surface

Do not create public tools for every internal primitive.

Prefer compact action-driven tools such as existing CodeLocal surfaces:

```text
Code.agent
Code.edit
Code.run/terminal
Code.read/context
Code.git
Code.workspace
Code.browser
Code.computer
```

Internal examples that should normally remain hidden:

```text
spawn_agent
join_agent
mailbox_send
worktree_create
patch_apply
context_compact
savepoint_create
capability_lease
```

The runtime chooses these primitives as part of bounded plans.

---

# 33. Benchmark suite

Create a CodeLocal SWE benchmark corpus across:

Languages/project types:

```text
Go
TypeScript
React/Next
Python
Rust
Flutter
monorepo
```

Task classes:

```text
tiny bug
multi-file bug
refactor
test generation
dependency upgrade
frontend bug
backend bug
database migration
security bug
cross-repo change
browser verification
mobile verification
```

Run modes:

```text
best external baseline
CodeLocal current
CodeLocal V3 Native
CodeLocal V3 + Codex engine
CodeLocal V3 + other engines
CodeLocal V3 Auto
```

Run both:

```text
Cold Brain
Warm Brain
```

Metrics:

```text
Verified Success Rate
tokens
time
cost
model calls
tool calls
retries
human interventions
regressions
wrong-file edits
stale overwrite attempts
resume success
browser/mobile verification success
```

Release quality gates:

- V3 verified success must not regress against selected baseline.
- stale overwrite = 0;
- security regressions = 0;
- restart/reconnect recovery must be reliable;
- token reductions must not hide quality regressions.

---

# 34. UX blind benchmark

Compare CodeLocal against leading coding-agent workflows for:

```text
trust
number of interruptions
progress clarity
approval burden
failure recovery
resume behavior
ability to keep working concurrently
final evidence quality
```

Goal:

> CodeLocal should feel like it understands and owns the engineering workflow, not merely like it has more tools.

---

# 35. Proposed package evolution

Do not big-bang rename current packages. Introduce seams incrementally.

Conceptual target:

```text
internal/runtimeevents/
internal/projection/
internal/contextsurface/
internal/contextbudget/
internal/tokenmeter/
internal/toolreducer/
internal/savepoint/
internal/agentgraph/
internal/agentactivation/
internal/mailbox/
internal/taskdag/
internal/worktree/
internal/patchengine/
internal/policy/
internal/verification/
internal/failureintel/
internal/engine/
internal/experience/
internal/ecosystem/
internal/observability/
```

Existing packages should be reused/migrated where already mature.

---

# 36. Migration and compatibility

Keep current MCP contract stable where possible.

Feature flags:

```text
agent_runtime_v2
context_surface_v2
worktree_v2
patch_engine_v2
policy_kernel_v2
tool_program_runtime
failure_intelligence
```

Each new task records:

```text
runtimeGeneration
featureSet
```

Legacy history remains readable.

Do not migrate Project Brain raw state into Runtime Event Store.

Rollback must remain possible during staged rollout.

---

# 37. A→Z execution roadmap

The upgrade must run as gated waves, not a big-bang rewrite.

## WAVE E — Ecosystem Intelligence Foundation

### E0 — Ecosystem Radar
- define source registry;
- ingest GitHub topic/repo metadata;
- dedupe candidates;
- track source commit/version.

### E1 — Capability Genome
- classify each candidate by problem/capability;
- extract architecture pattern;
- map overlap with CodeLocal;
- score novelty/maturity.

### E2 — Provenance & License
- persist source URL/SHA/license;
- classify reuse constraints;
- require attribution where applicable.

### E3 — Static Security Gate
- dependency/manifest analysis;
- dangerous install/build hooks;
- permission/capability declarations;
- secret/network/fs risk.

### E4 — Experimental Sandbox
- isolated candidate execution;
- no trusted host credentials by default;
- controlled network and filesystem.

### E5 — Benchmark Harness
- compare quality/token/latency/security;
- produce structured candidate report.

### E6 — Pattern Distiller
- convert winning ideas into ADR/capability proposals;
- do not automatically import implementation.

### E7 — Promotion Pipeline
- disposition: Native Core / Adapter / Skill / Experiment / Reject.

Gate: no ecosystem code reaches production solely because it is popular.

---

## WAVE 1 — Durable Foundation

### Phase 0 — Architecture Delta
- audit current packages vs V3;
- freeze canonical entities/state machines;
- freeze event taxonomy;
- define data migration and trace model.

### Phase 1 — Benchmark Baseline
- capture current CodeLocal and external baselines;
- establish success/token/time/cost metrics.

### Phase 2 — Runtime Event Store
- append, sequence, snapshot, replay, retention, redaction.

### Phase 3 — Projection Engine
- Task/Agent/UI/Audit/Recovery projections.

Gate: kill runtime mid-task, rebuild deterministic state from durable truth.

---

## WAVE 2 — Context & Token Economy

### Phase 4 — Context Surface Engine
- model-visible surface contract;
- progressive retrieval;
- fingerprints and deltas.

### Phase 5 — Token Meter
- per-request and per-task accounting.

### Phase 6 — Semantic Tool Reducers
- tests/build/git/LSP/browser/etc.

### Phase 7 — Reversible Context Ledger
- checkpoint/compress/search/expand.

### Phase 8 — Compaction & Budget Manager
- deterministic reduction first;
- LLM compaction last;
- task token budgets.

Gate: median input tokens decrease without verified-success regression.

---

## WAVE 3 — Durable Agent Runtime

### Phase 9 — Agent Identity + Activation
- separate durable agent from live process.

### Phase 10 — Durable Agent Graph
- spawn/join/retry/cancel/recovery.

### Phase 11 — Task DAG
- dependencies, ownership, revision CAS.

### Phase 12 — Durable Mailbox
- queued/delivered/acknowledged communication.

Gate: restart with active multi-agent graph and recover correct topology/state.

---

## WAVE 4 — Safe Mutation

### Phase 13 — Worktree Manager V2
- dirty authoritative checkout supported;
- isolated agent worktrees;
- lease/recovery/reconcile.

### Phase 14 — Patch Engine V2
- PatchSet/baseSha/provenance/stale/conflict.

### Phase 15 — Universal Savepoints
- task/message/runtime rewind and recovery.

Gate: concurrent user edit + agent edit never causes lost update.

---

## WAVE 5 — Security & Capability Intelligence

### Phase 16 — Policy Kernel V2
- unified policy decision;
- declarative rules;
- dry-run audit;
- secret exposure/consumption semantics.

### Phase 17 — Capability Lending
- one-shot leases;
- expiration and audit.

### Phase 18 — Dynamic Capability Projection
- session-aware model/tool surface.

Gate: impossible permissions are absent from model-visible schemas; security tests remain green.

---

## WAVE 6 — Coding Quality

### Phase 19 — Native Single-Agent Coding Kernel
- understand/locate/plan/act/observe/verify/reflect.

### Phase 20 — Verification Engine V2
- verification plans and evidence-based completion.

### Phase 21 — Failure Intelligence
- detect/attribute/recover/rerun/evaluate.

Gate: single-agent focused benchmark approaches or exceeds selected coding baseline.

---

## WAVE 7 — Efficiency & Parallelism

### Phase 22 — Tool Program Runtime
- bounded programmatic tool calls;
- generated session-aware capability SDK.

### Phase 23 — Adaptive Multi-Agent Scheduler
- task classification;
- child budgets;
- dependency-aware join;
- reviewer path.

### Phase 24 — Engine Router V2
- evidence-based escalation;
- quality/cost/token/reliability scoring;
- default Auto.

Gate: multi-agent only improves tasks where parallelism is actually useful; token/cost remain bounded.

---

## WAVE 8 — Learning & Platform Expansion

### Phase 25 — Experience Engine V2
- verified outcomes -> durable experience.

### Phase 26 — Learned Skills V2
- verified executable workflows.

### Phase 27 — Cloud Sandbox parity
- provider-neutral RuntimeProvider;
- sleep/checkpoint/resume.

### Phase 28 — Browser/Computer/Mobile verification
- same task graph, same policy, same verification.

Gate: warm tasks measurably improve versus cold tasks.

---

## WAVE 9 — Productization

### Phase 29 — UX V3
- clean progress views;
- advanced trace view;
- task resume/reconnect.

### Phase 30 — Observability / Flight Recorder
- full trace correlation;
- usage/cost dashboards;
- orphan process/task visibility.

### Phase 31 — Production Chaos Hardening
Test:
- runtime killed;
- provider process killed;
- websocket disconnect;
- network outage;
- auth expiry;
- disk full;
- Git conflict;
- stale patch;
- sandbox sleep;
- event duplication;
- mailbox partial delivery;
- context checkpoint corruption.

Gate: production release candidate only after recovery/security/migration suites pass.

---

# 38. Release stages

## Alpha

Required:

```text
Event Store
Projection
Context Surface
Token Meter
single-agent runtime
safe mutation baseline
```

## Beta

Required:

```text
Agent Graph
restart recovery
Worktree V2
Patch V2
Policy V2
Savepoints
```

## RC

Required:

```text
Tool Program
adaptive multi-agent
Engine Router V2
Failure Intelligence
Experience feedback
Cloud parity
```

## Stable

Required:

```text
benchmark gates
security gates
chaos gates
migration gates
UX/reconnect gates
zero-known-lost-update defects
```

---

# 39. Definition of Done

V3 is not complete until all are true:

1. Verified coding quality is at least competitive with the selected baseline.
2. Warm-project token/task is materially lower than cold-project token/task without quality loss.
3. User and agent can edit concurrently.
4. Lost update rate is zero in concurrency tests.
5. Runtime restart does not lose eligible task state.
6. Provider/model can change without losing Project Brain continuity.
7. Multi-agent topology and mailbox are durable.
8. Verification is provider-independent.
9. Security policy is unified and auditable.
10. Secret exposure and local secret consumption are correctly distinguished.
11. Model-visible capabilities are session-realistic.
12. Public MCP surface remains compact.
13. Local and Cloud share the RuntimeProvider contract.
14. Browser/Computer/Mobile verification can participate in a normal engineering task.
15. Savepoint/rewind works without requiring Git commits.
16. Failure/recovery evidence can be replayed and benchmarked.
17. Ecosystem candidates have provenance/license/security metadata before promotion.
18. Large user goals can complete without constant babysitting.

---

# 40. Flagship demonstration

User has uncommitted work and asks:

> “Authentication is broken. Find the root cause, fix backend/frontend if necessary, do not destroy what I am currently editing, test it properly, and tell me when it is actually done.”

Expected V3 flow:

```text
1. Resolve Project Brain.
2. Build bounded Context Surface.
3. Create Task + Verification Plan.
4. Classify task complexity.
5. Create Task DAG if needed.
6. Spawn isolated agents/worktrees.
7. User keeps editing authoritative checkout.
8. Agents retrieve through token-budgeted context.
9. Tool Programs batch mechanical inspection where beneficial.
10. Raw outputs are semantically reduced.
11. Failure is attributed and targeted retry occurs.
12. Client disconnect does not erase task state.
13. Runtime restart reconstructs Event/Agent/Task state.
14. User changes an overlapping file.
15. Patch detects stale base instead of overwriting.
16. Three-way reconciliation resolves latest user state + agent result.
17. Reviewer verifies diff.
18. Tests pass.
19. Browser verifies login flow when applicable.
20. Verification marks task PASS.
21. Only verified experience is promoted to Project Brain.
```

Final report example:

```text
Authentication fixed.

Changed: 5 files
Verification:
- 18 affected tests PASS
- typecheck PASS
- API auth flow PASS
- browser login flow PASS

Your uncommitted work: preserved
Runtime recovery: successful
Stale overwrite: none
Verified experience learned: 1
Token usage: recorded against benchmark
```

---

# 41. Final strategic rule

Do not compete by having the most tools or the most plugins.

Compete by making every capable model perform better inside CodeLocal than it would alone.

```text
Best model available
        |
        v
Best engine available
        |
        v
CODELOCAL
        |
        +-- durable project intelligence
        +-- safe execution
        +-- context economy
        +-- verification
        +-- failure recovery
        +-- experience
        +-- local/cloud/device capabilities
```

> **Models improve. Ecosystems innovate. CodeLocal absorbs validated engineering knowledge and becomes more capable over time.**
