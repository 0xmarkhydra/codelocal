# CodeLocal Universal Agent Runtime — Master Implementation Plan

Status: Proposed master plan / execution blueprint
Date: 2026-08-15
Owner: CodeLocal
Depends on: `PROJECT_BRAIN_MASTER_PLAN.md`
Primary implementation target: native Go runtime + CodeLocal Cloud

> This document defines how CodeLocal becomes the single user-facing runtime for many coding agents/CLIs while keeping one Project Brain, one security boundary, one project identity, one verification layer, and one durable experience history.

---

# 0. Executive decision

CodeLocal should support a **Universal Agent Runtime**.

The user-facing promise is:

```text
One CodeLocal interface
        ↓
Many coding agents
        ↓
One Project Brain
        ↓
One local security/execution boundary
```

The user should not need to build a separate memory/rules/workflow setup for Codex CLI, Claude Code, Cosine, Gemini CLI, Cursor-compatible agents, or future coding agents.

Those products become replaceable **agent engines** behind CodeLocal.

The durable asset belongs to CodeLocal:

```text
Project identity
Rules
Memories
Decisions
Experiences
Skills
Routing history
Verification history
User/team preferences
```

The external coding agent contributes reasoning and provider-specific capabilities for the current task.

The long-term positioning is:

> **CodeLocal is not another coding agent. CodeLocal is the operating layer for coding agents.**

---

# 1. Product goal

A developer installs/starts CodeLocal and can issue one command:

```bash
codelocal agent "fix the login bug"
```

CodeLocal then:

```text
1. identifies the logical project
2. resolves applicable Project Brain context
3. discovers available agent engines on this machine
4. chooses an engine or honors the user's explicit choice
5. starts/resumes the provider session through an adapter
6. streams provider output into one canonical event model
7. keeps execution under CodeLocal policy/approval boundaries
8. independently verifies the resulting workspace state
9. records a provider-neutral Experience
10. reinforces routing/memory/skills from verified outcomes
```

The user may explicitly choose:

```bash
codelocal agent --engine codex "fix the login bug"
codelocal agent --engine claude "review payment"
codelocal agent --engine cosine "refactor auth"
```

or let CodeLocal choose:

```bash
codelocal agent --engine auto "fix the login bug"
```

The Project Brain remains the same across all choices.

---

# 2. What this subsystem is NOT

Do not turn CodeLocal into a thin shell alias layer.

This is insufficient:

```go
exec.Command("codex", prompt)
```

The Universal Agent Runtime must provide:

- provider discovery;
- capability negotiation;
- session lifecycle;
- structured event normalization;
- context handoff;
- security mediation;
- cancellation/recovery;
- independent verification;
- provider-neutral Experience learning;
- routing intelligence;
- cross-device continuity metadata.

Also do not reimplement Codex/Claude reasoning internally merely to make adapters look uniform.

CodeLocal orchestrates and governs; the selected agent reasons.

---

# 3. Core architecture

```text
                         USER / CHATGPT / CLI
                                  |
                                  v
                           CodeLocal Command
                                  |
                                  v
                           Project Identity
                                  |
                                  v
                            Project Brain
                                  |
                     Knowledge Resolver + Context Compiler
                                  |
                                  v
                        Universal Agent Runtime
                                  |
                     +------------+-------------+
                     |            |             |
                     v            v             v
                  Codex         Claude        Cosine
                  Adapter       Adapter        Adapter
                     |            |             |
                     +------------+-------------+
                                  |
                    Canonical Agent Event Stream
                                  |
                                  v
                         CodeLocal Runtime
                      Files / Git / Process / PTY
                    Browser / Computer / Local MCP
                                  |
                                  v
                        Security + Approval
                                  |
                                  v
                            Verification
                                  |
                                  v
                             Experience
                                  |
                                  v
                            Project Brain
```

A provider must never become the authoritative source of project memory, permissions, or task-completion truth.

---

# 4. Core terminology

## Agent Engine

An external coding agent implementation, for example a CLI, SDK, ACP server, JSON-RPC application server, MCP-capable runtime, or provider API.

## Agent Adapter

CodeLocal implementation translating one provider into the canonical Universal Agent Runtime contract.

## Agent Session

A running or resumable conversation/task instance owned by a provider engine but tracked by CodeLocal.

## Agent Request

Provider-neutral task envelope created by CodeLocal.

## Agent Event

Provider-neutral structured output emitted while a session runs.

## Agent Result

Final provider outcome. It is **not** equivalent to verified task success.

## Verification Result

CodeLocal's independent assessment of workspace state after an agent finishes or checkpoints.

## Routing Decision

Decision choosing which engine/strategy to use for the task.

---

# 5. Existing CodeLocal foundations to reuse

Do not build parallel infrastructure where current native components already solve the problem.

Reuse:

```text
internal/process/         process + PTY lifecycle
internal/security/        command/action policy
internal/approval/        approval broker/memory
internal/orchestration/   capability-aware planning + verification model
internal/taskstate/       hot agent/task state
internal/project/         project map/context
internal/projectidentity/ logical project/repository identity
internal/memory/          durable memory + graph
internal/learnedskills/   reusable verified workflows
internal/mcphub/          external local MCP integration
internal/runtime/         workspace workers/runtime lifecycle
internal/history/         terminal/process history
internal/idempotency/     side-effect retry safety
```

Project Brain remains the higher-level durable knowledge layer.

The Universal Agent Runtime should fit into this architecture rather than replace it.

---

# 6. Proposed package structure

Initial native Go layout:

```text
internal/agentruntime/
    runtime.go
    registry.go
    adapter.go
    capability.go
    request.go
    event.go
    session.go
    router.go
    routing_score.go
    context.go
    verification.go
    recovery.go
    experience.go
    config.go

internal/agentruntime/adapters/
    codex/
    claude/
    cosine/
    gemini/
    acp/
    genericpty/
```

Provider-specific code must not leak throughout the rest of CodeLocal.

The rest of CodeLocal should depend on canonical interfaces.

---

# 7. Agent Adapter contract

Initial conceptual interface:

```go
type Adapter interface {
    ID() string
    DisplayName() string

    Probe(ctx context.Context) ProbeResult
    Capabilities(ctx context.Context) Capabilities
    AuthStatus(ctx context.Context) AuthStatus

    Start(ctx context.Context, req Request) (Session, error)
    Resume(ctx context.Context, session Session, req Request) error
    Cancel(ctx context.Context, session Session) error

    Events(session Session) <-chan Event
    Wait(ctx context.Context, session Session) (Result, error)
}
```

This may later split into smaller transport/session/parser interfaces.

Provider adapters are responsible for:

- locating the installed executable/runtime;
- determining version;
- validating minimum supported versions;
- constructing provider-safe invocation arguments;
- parsing structured output;
- mapping provider session IDs;
- exposing provider capabilities;
- detecting auth/not-authenticated states;
- translating cancellation;
- redacting unsafe diagnostic details.

Adapters are **not** responsible for:

- selecting Project Brain rules;
- overriding CodeLocal security policy;
- declaring task verification success;
- persisting provider secrets to Cloud;
- deciding cross-provider routing policy globally.

---

# 8. Adapter transport tiers

Providers differ substantially. Use tiers rather than pretending every CLI has the same interface.

## Tier A — Native structured protocol

Best tier.

Examples of possible transports:

```text
JSON-RPC
ACP
structured SDK
provider application server
streaming JSON protocol
```

Capabilities:

- structured events;
- reliable session IDs;
- cancellation;
- resumability where supported;
- lower parser fragility;
- provider tool/activity metadata.

## Tier B — Structured CLI/headless output

CLI launches as child process but emits machine-readable JSON/JSONL/events.

Capabilities:

- good automation support;
- process lifecycle through CodeLocal;
- structured parser;
- partial resumability depending on provider.

## Tier C — Interactive PTY compatibility

For agents lacking stable machine-readable protocols.

```text
CodeLocal PTY
    ↓
provider CLI/TUI
```

Mark as degraded:

```text
structuredEvents = partial
reliableToolEvents = false
resume = provider-dependent
```

Do not build routing decisions that assume Tier C output is as reliable as Tier A.

## Tier D — Direct API/SDK provider

Optional later path if a provider exposes a better programmatic SDK/API than its CLI.

This should still implement the same Adapter contract.

---

# 9. Capability model

Every engine advertises canonical capabilities.

Suggested initial structure:

```text
Capabilities
  interactive
  nonInteractive
  structuredOutput
  streaming
  resume
  cancel
  providerTools
  fileEditing
  shellExecution
  mcp
  browser
  imageInput
  subagents
  planMode
  reviewMode
  sandboxMode
  modelSelection
  costMetadata
  tokenUsageMetadata
  maxContextHint
```

Also include transport confidence:

```text
transportTier
parserConfidence
versionSupported
```

Routing must use actual discovered capabilities rather than provider name assumptions.

---

# 10. Engine discovery

CodeLocal should automatically detect installed engines.

Examples of probe signals:

```text
binary exists on PATH
known app installation path
version command succeeds
structured/headless mode supported by that version
provider-specific local socket/server available
```

Cache probe results with short TTL and executable fingerprint.

Invalidate when:

- executable path changes;
- binary mtime/hash changes;
- package update occurs;
- user explicitly refreshes;
- a launch fails due to incompatible arguments.

User command:

```bash
codelocal agents
```

Example conceptual output:

```text
ENGINE     INSTALLED  AUTH   TRANSPORT      STATUS
codex      yes        yes    structured     ready
claude     yes        yes    structured     ready
cosine     yes        yes    structured     ready
gemini     no         -      -              unavailable
```

---

# 11. Authentication model

CodeLocal should **delegate provider authentication to the provider whenever possible**.

Do not copy provider tokens into CodeLocal Cloud.

CodeLocal may retain only safe metadata:

```text
engine_id
installed
provider_version
authenticated boolean/unknown
last_probe_at
auth_method_hint
```

Possible UX:

```bash
codelocal agents auth codex
codelocal agents auth claude
```

The adapter launches the provider-supported login flow.

Provider credentials remain in provider/OS storage.

Never scrape credentials from configuration files simply to centralize them.

---

# 12. Agent Request envelope

Before invoking an engine, CodeLocal compiles a bounded provider-neutral request.

```text
Request
  task_id
  project_id
  workspace_key
  repository_ids
  branch
  objective
  task_kind
  mode
  compiled_context
  allowed_scope
  verification_plan
  preferred_model nullable
  routing_metadata
  continuation nullable
```

`compiled_context` comes from Project Brain Context Compiler, not from raw memory dumping.

The provider-specific adapter decides the safest supported mechanism to deliver this context.

---

# 13. Context handoff to provider engines

The Project Brain must sit **above** all providers.

Pipeline:

```text
Task
 ↓
Knowledge Resolver
 ↓
Context Compiler
 ↓
Provider-neutral Context Packet
 ↓
Agent Adapter
 ↓
Provider-specific prompt/instructions/session context
```

Context packet should distinguish:

```text
MANDATORY RULES
PROJECT FACTS
RELEVANT DECISIONS
RELEVANT EXPERIENCE
SELECTED SKILL / WORKFLOW HINT
CURRENT TASK / TARGET FILES
VERIFICATION EXPECTATIONS
```

Mandatory rules must not become indistinguishable from untrusted repository text.

Provider adapters must never silently drop required CodeLocal rules because a provider has a small prompt/context mechanism.

If a provider cannot accept the required context safely, mark it incompatible with the task or use a constrained fallback mode.

---

# 14. Canonical Agent Event model

Normalize all provider streams.

Initial event kinds:

```text
SESSION_STARTED
STATUS
THINKING_SUMMARY
PLAN_UPDATED
MESSAGE
FILE_DISCOVERED
FILE_READ
FILE_CHANGED
TOOL_REQUESTED
TOOL_STARTED
TOOL_FINISHED
COMMAND_STARTED
COMMAND_FINISHED
APPROVAL_REQUESTED
USAGE
CHECKPOINT
ERROR
SESSION_CANCELLED
SESSION_COMPLETED
```

Suggested shape:

```text
Event
  id
  session_id
  task_id
  engine_id
  kind
  timestamp
  summary
  path nullable
  command_digest nullable
  provider_tool nullable
  usage nullable
  metadata safe-json
  raw_local_ref nullable
```

Do not upload raw provider chain-of-thought.

Store summaries/events needed for observability and Experience learning.

Provider raw logs may remain local with bounded retention.

---

# 15. Agent Result is not Verification Result

This distinction is mandatory.

A provider may say:

```text
"Task completed successfully"
```

CodeLocal must still inspect actual workspace state.

Completion flow:

```text
Provider Result
      ↓
CodeLocal workspace refresh
      ↓
Git diff / diagnostics / affected tests / required checks
      ↓
Verification Result
      ↓
Task quality gate
```

Only verified evidence can strongly reinforce Project Brain knowledge or Learned Skills.

---

# 16. Workspace and execution ownership

Every agent session belongs to exactly one authorized CodeLocal workspace.

Provider process must run with:

```text
workspace root
CodeLocal runtime owner
CodeLocal process/session ID
CodeLocal cancellation lifecycle
CodeLocal audit metadata
```

Avoid letting provider processes operate from arbitrary current working directories.

Provider adapters must not bypass workspace boundary checks simply because the provider CLI itself has broad filesystem access.

Long-term target: provider execution should inherit the strongest practical CodeLocal sandbox/containment available on the OS.

---

# 17. Security and approval architecture

Universal Agent Runtime increases the attack surface because external coding agents may independently decide to execute commands or access files.

Core invariant:

> Provider permissions can be stricter than CodeLocal, but never weaker than CodeLocal's required security boundary.

Preferred architecture:

```text
Provider request/action
       ↓
Provider-native permission layer (when useful)
       ↓
CodeLocal security classification
       ↓
CodeLocal approval broker
       ↓
Allowed execution
```

Never automatically invoke provider flags equivalent to unrestricted dangerous execution as the default integration strategy.

Never persist provider approval bypass tokens/options as learned skills.

High-risk actions remain governed by existing CodeLocal approval policy.

---

# 18. Provider subprocess containment challenge

Some CLIs may execute shell/file actions internally instead of asking CodeLocal for each action.

This creates an important implementation distinction.

## Preferred mode — mediated tools

Provider uses CodeLocal/MCP/structured tools for filesystem/shell operations.

Then CodeLocal can approve every action directly.

## Constrained subprocess mode

Provider runs as a subprocess but is sandboxed/contained so its autonomous filesystem/process/network scope cannot exceed allowed policy.

## Weak fallback mode

If neither mediated tools nor reliable sandboxing is available, explicitly classify engine execution as weaker isolation and require stronger user approval/configuration.

Do not claim full CodeLocal action-level enforcement when the provider process can bypass it internally.

This must be transparent in `codelocal agents doctor`.

---

# 19. Process/PTy integration

Use `internal/process` as the lifecycle owner where possible.

Agent process records should extend normal process metadata with:

```text
agent_session_id
engine_id
provider_session_id
transport_tier
task_id
project_id
workspace_key
started_at
last_event_at
status
exit_code
```

Requirements:

- cancellation propagates to provider;
- process output is bounded;
- PTY resize supported for interactive fallback;
- disconnect/reconnect does not accidentally duplicate provider execution;
- idempotency keys prevent accidental duplicate starts;
- terminal history can distinguish agent subprocesses from ordinary commands.

---

# 20. Session model

CodeLocal tracks canonical session identity independently from provider session IDs.

```text
AgentSession
  id                    CodeLocal ID
  engine_id
  provider_session_id   nullable
  user_id
  project_id
  workspace_key
  task_id
  status
  transport_tier
  context_fingerprint
  started_at
  updated_at
  completed_at nullable
```

Possible statuses:

```text
starting
running
waiting_approval
waiting_user
verifying
completed
failed
cancelled
orphaned
```

Provider session IDs are adapter metadata, not global identifiers.

---

# 21. Resume and continuation

A user should be able to continue work without memorizing provider-specific resume commands.

```bash
codelocal agent resume <session>
```

CodeLocal decides whether:

```text
same provider session can resume
      OR
provider lacks resume → create a new provider session using compact handoff context
```

For cross-provider handoff:

```bash
codelocal agent resume <session> --engine claude
```

CodeLocal compiles:

```text
original objective
verified/observed progress
current Git diff
remaining blockers
applicable rules
relevant prior messages summarized
```

Do not blindly forward a full provider transcript to another provider.

---

# 22. Universal Agent Router

The router chooses an engine when `--engine auto` is used.

Initial router should be deterministic/heuristic before becoming learned.

Signals:

```text
engine availability
auth status
transport quality
required capabilities
task kind
project/language fit
user preference
provider mode availability
recent provider failures
historical verified success
latency
usage/cost metadata when available
```

Suggested conceptual score:

```text
score =
  capability_fit
+ task_fit
+ project_fit
+ historical_verified_success
+ user_preference
+ transport_reliability
- failure_penalty
- latency_penalty
- cost_penalty
```

Security compatibility is a gate, not a weighted preference.

An engine incapable of meeting required security/context constraints should be excluded regardless of score.

---

# 23. Routing learning

Routing should improve from verified experience.

Store provider-neutral aggregates such as:

```text
engine_id
task_kind
project_id optional
language/framework optional
attempt_count
verified_success_count
verified_failure_count
median_completion_ms
median_tool_roundtrips
verification_regression_rate
cost/usage estimate optional
```

Do not optimize solely for provider self-reported completion.

Use **CodeLocal verification outcome** as the primary success signal.

Avoid premature overfitting to one project/user with tiny samples.

Use Bayesian/smoothed priors or minimum sample thresholds before strong automatic routing changes.

---

# 24. Routing modes

User-facing modes:

```text
manual
  explicit engine chosen by user

auto
  router selects one engine

review
  primary engine performs task; second engine reviews

compare
  two or more agents independently propose/inspect; CodeLocal compares results

ensemble
  specialized agents receive sub-tasks; use only for large work
```

Default should remain `manual` or conservative `auto`, not expensive multi-agent execution.

---

# 25. Multi-agent orchestration

Multi-agent is valuable but dangerous for cost, concurrency, and conflicting edits.

## Safe first strategy — sequential review

```text
Engine A implements
      ↓
CodeLocal captures diff
      ↓
Engine B reviews read-only
      ↓
CodeLocal/model applies selected findings
      ↓
Verification
```

## Parallel research strategy

Agents inspect independently but do not mutate the same workspace.

Use isolated worktrees/snapshots if parallel mutation is required later.

## Specialist strategy

Example:

```text
Planner      → Engine A
Implementation → Engine B
Review       → Engine C
Verification → CodeLocal deterministic
```

Do not allow two independent agents to edit the same working tree concurrently without isolation and reconciliation.

---

# 26. Worktree/sandbox strategy for parallel agents

For multi-agent mutation, use disposable Git worktrees or equivalent isolated workspace copies.

```text
main authorized checkout
    |
    +-- agent-worktree-A
    +-- agent-worktree-B
```

Each agent receives the same base commit/context fingerprint.

After completion:

```text
diffs
  ↓
compare
  ↓
select/merge/cherry-pick under CodeLocal control
  ↓
verify in authoritative workspace
```

Never merge competing agent changes automatically without deterministic conflict checks and user/agent review policy.

---

# 27. Project Brain integration

Every agent engine consumes the same canonical Project Brain.

The Project Brain should record:

```text
which engine worked on the task
which rules/context packet were supplied
which skill was selected
provider-neutral outcome
verification evidence summary
routing decision/reason
latency/usage metadata
```

It should not need to retain full raw provider transcripts.

This creates cross-provider continuity:

```text
Claude solves incident
      ↓
Experience stored in Project Brain
      ↓
Codex handles related incident next month
      ↓
Codex receives relevant experience
```

---

# 28. Learned Skills integration

A Learned Skill belongs to the logical user/project workflow, not to one provider by default.

Skill model should distinguish:

```text
provider_neutral skill
provider_specific skill
```

Provider-neutral example:

```text
run affected tests
inspect diff
deploy configured dev service
health check
```

Provider-specific skill should only exist when workflow semantics genuinely depend on provider features.

Skill Context Fingerprint must include agent capability requirements when relevant.

Example:

```text
required_agent_capabilities:
  structuredOutput=true
  resume=true
```

Do not replay a skill on an incompatible provider merely because task intent matches.

---

# 29. MCP Hub integration

Agent engines may support MCP themselves.

CodeLocal should eventually be able to expose selected local MCP capabilities to an agent through a controlled bridge.

Principle:

```text
Provider agent
   ↓
CodeLocal-managed MCP view
   ↓
allowed local MCP extensions
```

Do not expose every installed MCP server/tool blindly.

Project/user policy decides the allowed capability set.

Avoid recursive loops where CodeLocal invokes an agent that invokes CodeLocal that recursively invokes another agent without explicit orchestration boundaries.

Track origin/session chain IDs to detect recursion.

---

# 30. Provider model selection

Some engines support multiple models.

Canonical request may include:

```text
preferred_model
reasoning_profile
```

but adapters decide how/if those map to provider options.

Router should initially select **engine**, not aggressively optimize provider model choice.

Add model-level routing only after engine-level benchmarks are stable.

---

# 31. Context isolation between providers

Provider-specific transient state must not pollute canonical memory automatically.

Examples:

- Claude internal scratch context;
- Codex provider-specific session metadata;
- CLI-generated cache;
- local temporary prompts.

Only validated durable facts/experiences pass through Project Brain ingestion gates.

Provider session history is evidence, not automatically truth.

---

# 32. CLI UX

Initial commands:

```bash
codelocal agents
codelocal agents doctor
codelocal agents refresh
codelocal agents auth <engine>

codelocal agent "task"
codelocal agent --engine codex "task"
codelocal agent --engine auto "task"
codelocal agent --mode review "task"
codelocal agent status
codelocal agent sessions
codelocal agent resume <session-id>
codelocal agent cancel <session-id>
```

Optional later:

```bash
codelocal agent compare codex claude -- "task"
codelocal agent route explain "task"
```

Keep command names simple enough that users never need provider-specific CLI syntax for normal CodeLocal workflows.

---

# 33. Configuration model

Global user config outside repositories:

```text
~/.codelocal/agents.json
```

Example conceptual config:

```json
{
  "defaultEngine": "auto",
  "engines": {
    "codex": { "enabled": true },
    "claude": { "enabled": true },
    "cosine": { "enabled": false }
  },
  "routing": {
    "allowLearnedRouting": true,
    "maxParallelAgents": 2
  }
}
```

Project-level non-secret preferences may live in `.codelocal/project.json` or future canonical project config.

Provider credentials never belong in these files.

---

# 34. Installation management

Long-term UX may offer:

```bash
codelocal agents install codex
codelocal agents install claude
```

But CodeLocal should prefer invoking/documenting the provider's official installer rather than bundling all providers into the CodeLocal npm package.

Reasons:

- licensing;
- provider update cadence;
- security boundaries;
- binary size;
- platform differences;
- package-signing responsibility.

Install/update actions require explicit approval because they modify the system/global package environment and may use the network.

---

# 35. Provider version compatibility

Adapters must define supported version ranges/capability probes.

Do not assume provider CLI flags/protocols remain stable.

Each adapter tracks:

```text
adapter_version
provider_version
minimum_supported_version
capability_probe_result
last_compatibility_check
```

When provider updates unexpectedly:

```text
structured probe fails
→ mark adapter degraded/incompatible
→ do not guess command flags
→ recommend adapter/runtime update
```

Adapters require fixture/integration tests against representative provider versions where feasible.

---

# 36. Generic agent adapter manifest

Later, support adding simple external CLI agents without compiling CodeLocal.

Example conceptual manifest:

```yaml
id: my-agent
transport: jsonl-command
binary: my-agent
versionCommand: ["--version"]
start:
  args: ["run", "--json"]
capabilities:
  structuredOutput: true
  resume: false
```

Security constraints:

- manifest cannot grant permissions;
- manifest cannot weaken sandbox/policy;
- executable must still pass local policy;
- remote manifests require signatures/trust mechanism;
- arbitrary script hooks are not silently trusted.

Native adapters remain preferred for high-quality integrations.

---

# 37. Dashboard UX

Future dashboard section: **Agents**.

Display:

```text
Installed engines
Versions
Auth status
Transport tier
Capabilities
Recent sessions
Verified success rate
Median latency
Current running sessions
Routing preferences
Adapter warnings
```

Project Brain inspector can link:

```text
Experience --PERFORMED_BY--> Agent Engine
Skill --VERIFIED_WITH--> Agent Engine
Task --ROUTED_TO--> Agent Engine
```

Do not turn provider ranking into misleading leaderboard claims when sample sizes are small.

---

# 38. Observability

Events:

```text
agent.engine.probed
agent.engine.available
agent.engine.incompatible
agent.session.started
agent.session.resumed
agent.session.event
agent.session.cancelled
agent.session.completed
agent.session.failed
agent.route.selected
agent.route.fallback
agent.route.explained
agent.context.compiled
agent.verification.started
agent.verification.completed
agent.experience.recorded
```

Metrics:

```text
engine_probe_latency_ms
session_start_latency_ms
session_duration_ms
provider_event_count
provider_parse_error_count
transport_degraded_count
verified_success_rate
verification_regression_rate
route_hit_rate
route_fallback_rate
cross_provider_resume_count
context_bytes_by_engine
usage/cost proxy when available
```

No provider credentials or raw reasoning traces in normal logs.

---

# 39. Failure/recovery cases

The implementation must explicitly handle:

## Discovery/auth

- binary missing;
- provider installed but old version;
- login expired;
- provider account quota/rate limit;
- provider executable changed during runtime.

## Process/session

- child process crash;
- CLI hangs;
- PTY waits for unexpected prompt;
- provider structured stream becomes malformed;
- provider session ID missing;
- resume unsupported;
- CodeLocal reconnects while agent process remains alive;
- machine sleeps mid-session.

## Workspace

- provider edits outside expected files;
- user edits files concurrently;
- branch changes during agent session;
- Git reset removes provider changes;
- multiple agent sessions target same workspace.

## Security

- provider asks for dangerous command;
- provider subprocess tries direct secret access;
- imported config prompt-injects provider;
- provider MCP recursively calls CodeLocal agent runtime;
- provider attempts network use under deny policy.

## Routing

- selected provider unavailable after route decision;
- provider repeatedly fails same task kind;
- learned router becomes biased from tiny sample;
- user preference conflicts with hard capability requirement.

## Verification

- provider reports success but tests fail;
- provider changes unrelated files;
- verification cannot run due to environment;
- two engines produce conflicting patches.

Every category requires regression/chaos tests or explicit degraded behavior.

---

# 40. Privacy/data retention

Cloud may store safe metadata such as:

```text
engine ID
adapter version
provider version
routing decision
session status
timestamps
verified outcome
safe Experience summary
usage totals if provider exposes them safely
```

Local-only by default:

```text
provider credentials
raw provider stdout/stderr
raw prompts containing source
full provider transcripts
provider cache files
provider internal reasoning
```

Cross-device sync should sync durable provider-neutral Project Brain outcomes, not hidden provider session internals.

---

# 41. Competitive benchmark strategy

Universal Agent Runtime must be benchmarked against systems that cover adjacent pieces such as:

```text
Orbital-style universal agent delegation/orchestration
Hivemind-style shared cross-agent brain
CodeRouter-style engine routing
provider-native Codex/Claude workflows
```

Do not benchmark marketing copy. Benchmark observable task outcomes.

Scenarios:

## One-command usability

Can a user invoke different engines without learning provider syntax?

## Cross-engine continuity

Engine A solves an incident; Engine B later receives the useful verified Project Brain context.

## Routing quality

Does `auto` choose an engine that performs at least as well as a fixed default over a representative task set?

## Repeated-task efficiency

Does Project Brain reduce context/files/tool loops regardless of selected provider?

## Provider failure fallback

If selected engine is unavailable/rate-limited, can CodeLocal safely choose a fallback without losing task state?

## Rule compliance

Do mandatory CodeLocal rules remain consistent across providers?

## Verification independence

Does CodeLocal catch false provider completion claims?

## Security

Can a provider or imported instruction bypass CodeLocal policy? It must not.

---

# 42. Benchmark metrics

Measure at minimum:

```text
verified_task_success_rate
first_pass_verification_rate
median_time_to_verified_completion
context_bytes_to_provider
CodeLocal MCP/tool calls
provider subprocess events
files unnecessarily touched
unrelated_diff_rate
rule_violation_rate
agent_route_accuracy_proxy
fallback_success_rate
cross_engine_handoff_success_rate
session_resume_success_rate
security_policy_violation_count
```

When provider usage metadata is available:

```text
input tokens
output tokens
cost estimate
```

Do not claim a cost/token advantage without controlled measurement.

---

# 43. Rollout roadmap

## UAR0 — Architecture freeze + provider capability survey

Tasks:

- finalize canonical adapter/session/event contracts;
- inspect current Codex/Claude/Cosine/Gemini integration surfaces;
- map process/PTy/security constraints;
- define test fixtures/mocks;
- capture baseline provider-native workflows.

Exit gate:

- contracts documented;
- no provider-specific assumptions leak into core design.

## UAR1 — Runtime core + fake adapter

Implement:

```text
internal/agentruntime core
registry
session manager
event stream
request/result models
fake deterministic adapter for tests
```

Exit gate:

- one fake session supports start/event/cancel/result without external CLI dependencies.

## UAR2 — Agent discovery + doctor

Implement:

- engine registry;
- binary/version probes;
- auth-status hints;
- capability matrix;
- cache/invalidation;
- `codelocal agents`;
- `codelocal agents doctor`.

Exit gate:

- CodeLocal can accurately explain available/degraded engines without running a coding task.

## UAR3 — First production adapter: Codex or best structured candidate

Choose the provider offering the most stable structured integration after UAR0 survey.

Implement:

- structured start;
- event parsing;
- cancellation;
- bounded output;
- workspace/session mapping;
- provider-version tests.

Exit gate:

```bash
codelocal agent --engine <first> "read/review task"
```

runs through canonical runtime.

Start with read/review workflows before mutation if security mediation is incomplete.

## UAR4 — Project Brain context injection

Implement:

- Resolver/Context Compiler handoff;
- mandatory rule packet;
- task/context fingerprints;
- provider-specific safe delivery;
- context-size instrumentation.

Exit gate:

- the same canonical rule/context test passes through the first provider adapter.

## UAR5 — Independent verification + Experience

Implement:

- workspace refresh after provider task;
- `verify_changes` integration;
- provider-neutral Experience records;
- provider outcome vs verified outcome separation.

Exit gate:

- false provider success is detected;
- only verified results reinforce Project Brain strongly.

## UAR6 — Second/third provider adapters

Implement native adapters for additional major engines.

Exit gate:

- same task contract can run on at least three engines with canonical events/results.

## UAR7 — Manual universal CLI UX

Implement stable commands:

```text
codelocal agent --engine ...
codelocal agent sessions
resume
cancel
```

Exit gate:

- user no longer needs provider-specific invocation syntax for supported workflows.

## UAR8 — Auto Router v1

Implement deterministic scoring based on:

- capabilities;
- task type;
- transport quality;
- user preference;
- availability;
- initial historical verified outcomes.

Add route explanation.

Exit gate:

```bash
codelocal agent --engine auto ...
```

always has an inspectable routing reason and safe fallback behavior.

## UAR9 — Learned routing

Add verified outcome aggregates and conservative learning.

Requirements:

- minimum sample thresholds;
- smoothed success rates;
- decay/recovery from historical failures;
- per-project + global priors;
- user override always available.

Exit gate:

- benchmark demonstrates learned routing is no worse than deterministic baseline before default enablement.

## UAR10 — Cross-provider handoff/resume

Implement provider-neutral checkpoint summaries.

Exit gate:

- task started with Engine A can continue with Engine B without replaying full history or losing project rules/state.

## UAR11 — Sequential multi-agent review

Implement safe read-only reviewer role and isolated comparison flows.

Exit gate:

- secondary agent review improves measured quality on selected complex tasks without touching authoritative workspace concurrently.

## UAR12 — Parallel isolated agents

Implement Git worktree/snapshot isolation.

Exit gate:

- two agents can mutate independent worktrees and CodeLocal safely compare/reconcile results.

## UAR13 — Provider adapter ecosystem

Implement signed/declarative adapter manifests for simple engines plus native adapters for high-value integrations.

Exit gate:

- adding a simple compatible CLI does not require changes to Universal Agent Runtime core.

---

# 44. Recommended implementation order

```text
UAR0 capability survey/contracts
  ↓
UAR1 runtime core + fake adapter
  ↓
UAR2 discovery/doctor
  ↓
UAR3 first structured provider
  ↓
UAR4 Project Brain context injection
  ↓
UAR5 independent verification + Experience
  ↓
UAR6 additional providers
  ↓
UAR7 stable manual UX
  ↓
UAR8 deterministic auto-router
  ↓
UAR9 learned routing
  ↓
UAR10 cross-provider handoff
  ↓
UAR11 sequential review
  ↓
UAR12 parallel isolated agents
  ↓
UAR13 adapter ecosystem
```

Do not start with multi-agent parallelism or learned routing.

First prove one provider through a clean canonical contract, Project Brain context, security, and independent verification.

---

# 45. First concrete implementation slice

The smallest useful implementation should be:

```text
1. internal/agentruntime canonical interfaces/models
2. fake adapter + lifecycle tests
3. provider registry/probe model
4. `codelocal agents` + doctor output
5. one structured provider adapter
6. `codelocal agent --engine X <task>`
7. Project Brain compiled context passed to provider
8. canonical event streaming
9. cancellation
10. post-run CodeLocal verification
11. Experience record with engine ID + verified outcome
```

Explicitly postpone:

```text
auto routing
learned routing
cross-provider resume
parallel agents
provider installation management
generic plugin ecosystem
```

until the first provider path is production-quality.

---

# 46. Acceptance tests for first slice

Required scenarios:

```text
provider missing
provider installed/authenticated
unsupported provider version
start successful session
stream canonical events
cancel running session
provider exits non-zero
provider says success but verification fails
provider modifies unexpected file
CodeLocal rule reaches provider context
user explicitly selects engine
workspace escape attempt is blocked/contained
CodeLocal restart does not duplicate completed task
raw provider secret-like output is not uploaded
```

Use a fake adapter for deterministic unit tests and an optional integration suite for installed real providers.

---

# 47. Security gates before mutation default

Do not enable full autonomous mutation through a provider by default until one of these is true:

```text
A. provider actions are mediated through CodeLocal-controlled tools
OR
B. provider subprocess runs inside a validated containment/sandbox boundary
```

If neither is achieved:

- support read/review mode;
- require explicit user opt-in for provider-direct mutation;
- clearly display weaker isolation status.

This is a production gate, not a documentation warning.

---

# 48. Decision log — 2026-08-15

## UARD1 — One Project Brain, many engines

Reason: durable project intelligence must not fragment across providers.

## UARD2 — Agent Result is not task truth

Reason: provider self-report must be independently verified by CodeLocal.

## UARD3 — Provider credentials remain provider-owned/local

Reason: reduce credential exposure and preserve official auth lifecycle.

## UARD4 — Structured integrations outrank PTY scraping

Reason: reliable event/session contracts are required for production orchestration.

## UARD5 — CodeLocal security policy cannot be weakened by an engine

Reason: external agents are untrusted execution participants relative to the local security boundary.

## UARD6 — Routing learns from verified outcomes

Reason: provider completion text is a weak signal; workspace verification is stronger.

## UARD7 — Multi-agent mutation requires isolation

Reason: concurrent edits in one checkout create non-deterministic corruption/conflict risk.

## UARD8 — Adapters hide provider-specific syntax from core

Reason: provider CLIs/protocols evolve independently and must not contaminate CodeLocal architecture.

## UARD9 — Manual routing precedes auto routing

Reason: first prove adapter correctness before adding algorithmic engine selection.

## UARD10 — Universal Agent Runtime is a separate subsystem from Project Brain

Reason: Project Brain owns durable intelligence; Agent Runtime owns provider execution/orchestration. They integrate through bounded contracts.

---

# 49. What NOT to do

Avoid these shortcuts:

- create one giant `switch engine { ... }` inside CLI code;
- parse colored TUI output when structured protocol exists;
- silently pass `--dangerous`/permission-bypass flags;
- store provider API keys/tokens in CodeLocal Cloud;
- treat provider success text as verified completion;
- let each provider maintain unrelated canonical project memory;
- forward full raw history between providers;
- run several mutating agents against one working tree;
- learn routing from one or two anecdotal successes;
- expose every installed MCP tool to every provider;
- claim action-level CodeLocal security if provider subprocess can bypass it;
- bundle every provider binary inside CodeLocal distribution;
- hard-code volatile provider flags without version probes;
- add auto-routing before manual provider execution is reliable.

---

# 50. Competitive product target

The intended user experience should combine strengths that currently tend to exist separately:

```text
Universal agent invocation
        +
Shared durable Project Brain
        +
Cross-device project identity
        +
Deterministic rules
        +
Provider-neutral learned experience
        +
Independent verification
        +
Local execution/security control
        +
Learned routing
```

The point is not to say CodeLocal "contains Codex/Claude".

The point is:

> The user owns one durable engineering environment, while coding agents are interchangeable workers inside it.

---

# 51. Definition of success

Universal Agent Runtime is successful when a developer can work for months with this mental model:

```text
I use CodeLocal.
CodeLocal knows my projects.
CodeLocal knows my rules/history/workflows.
For each task it can use whichever coding engine I choose or whichever is best.
Changing engines does not reset the relationship.
```

A mature flow should look like:

```text
codelocal agent "fix payment retry bug"
          ↓
Project Brain identifies prior payment incident
          ↓
Resolver loads mandatory payment rules
          ↓
Router selects an eligible engine
          ↓
Adapter runs provider safely
          ↓
Canonical events stream to CodeLocal
          ↓
Provider edits workspace
          ↓
CodeLocal independently verifies tests/diff
          ↓
Experience records what actually worked
          ↓
Future provider gets the same durable learning
```

The result should be a system where changing from Codex to Claude to another future agent changes the worker, **not the accumulated engineering brain**.

---

# 52. Maintenance rule

When adding or materially changing an agent provider:

1. update its adapter capability contract;
2. record supported provider versions/transports;
3. add fixture/integration tests;
4. update security assumptions;
5. update benchmark results;
6. add a dated Decision Log entry if architecture changes;
7. never silently weaken verification or permission gates to gain compatibility.

Do not allow provider-specific behavior to become undocumented tribal knowledge in adapter code.
