# CodeLocal Ecosystem Assimilation Engine — Detailed Design

Status: Proposed V3 subsystem plan
Date: 2026-08-30
Owner: CodeLocal
Parent plan: `CODELOCAL_AGENT_OS_V3_MASTER_PLAN.md`

> Goal: turn external open-source innovation into a continuous, safe, benchmark-driven R&D input for CodeLocal without turning the runtime into a dependency landfill.

---

# 0. Executive decision

CodeLocal should deliberately learn from external agent ecosystems, but it must not automatically install, execute, vendor, or merge third-party code just because it is popular or interesting.

The operating principle is:

> **Absorb knowledge, patterns, benchmarks, and proven capability ideas. Promote implementation only after provenance, license, security, compatibility, and benchmark gates pass.**

The Ecosystem Assimilation Engine exists to make this systematic.

It converts:

```text
GitHub repos
plugin ecosystems
agent frameworks
papers
benchmarks
skills
runtime experiments
```

into:

```text
Capability Candidates
Architecture Patterns
Benchmark Cases
Security Lessons
Native Core Proposals
Adapters
Skills
Rejected Ideas with evidence
```

This is not an auto-fork bot and not a plugin auto-installer.

---

# 1. Why this subsystem exists

Agent ecosystems evolve faster than a single team can manually track.

Useful innovations repeatedly appear outside the CodeLocal repository:

- context compression strategies;
- memory architectures;
- permission DSLs;
- agent orchestration patterns;
- token/cost accounting;
- undo/savepoint systems;
- multi-agent dispatch policies;
- failure attribution;
- browser/computer/mobile bridges;
- plugin validation;
- new engine adapters;
- new benchmark/evaluation techniques.

Without a structured intake pipeline, CodeLocal faces two bad choices:

1. ignore useful external innovation;
2. copy or install things reactively and accumulate architectural debt.

The Assimilation Engine creates a third option:

```text
Discover
  -> Understand
  -> Compare
  -> Prove
  -> Distill
  -> Promote safely
```

---

# 2. Core invariants

## ECO-1 — Discovery is not trust

A discovered repository has zero production authority.

## ECO-2 — Popularity is not quality

Stars, forks, downloads, or topic ranking may influence discovery priority, never promotion.

## ECO-3 — Source code is untrusted input

Reading metadata/source is allowed under bounded scanners. Executing source requires an isolated Lab stage.

## ECO-4 — License precedes code reuse

No code-level reuse decision occurs before license classification.

## ECO-5 — Security precedes execution

Install scripts, lifecycle hooks, network behavior, filesystem reach, subprocess usage, secrets access, and build behavior are inspected before dynamic evaluation.

## ECO-6 — Benchmark precedes promotion

A candidate must prove value relative to current CodeLocal behavior.

## ECO-7 — Distill patterns before importing dependencies

Default preference:

```text
idea/pattern -> native CodeLocal implementation
```

rather than:

```text
idea/pattern -> permanent third-party runtime dependency
```

## ECO-8 — Provenance survives assimilation

Every promoted idea retains source references and evidence of where the idea came from.

## ECO-9 — No silent auto-merge

The system may generate proposals, branches, ADRs, tests, and reports. Production merge remains an explicit reviewed action.

## ECO-10 — One capability architecture, not many competing subsystems

Fifteen memory plugins should yield one coherent CodeLocal memory architecture, not fifteen memory engines mounted simultaneously.

---

# 3. High-level architecture

```text
                    EXTERNAL ECOSYSTEMS

 GitHub Topics / Repos / Releases / Papers / Skills / Benchmarks
                              |
                              v
                       Ecosystem Radar
                              |
                              v
                    Candidate Normalizer
                              |
                              v
                     Capability Genome
                              |
             +----------------+----------------+
             |                |                |
             v                v                v
       Provenance Gate    License Gate    Static Security Gate
             |                |                |
             +----------------+----------------+
                              |
                              v
                        Novelty/Dedupe
                              |
                              v
                        Lab Sandbox
                              |
                              v
                      Benchmark Harness
                              |
                              v
                      Pattern Distiller
                              |
                              v
                     Promotion Decision
                +---------+---------+---------+
                |         |         |         |
                v         v         v         v
             Native     Adapter    Skill   Reject/Watch
              Core
```

---

# 4. Source registry

The Radar must use a versioned source registry rather than hard-coded crawls scattered throughout the codebase.

Conceptual model:

```text
EcosystemSource
  id
  kind
  locator
  enabled
  trustClass
  pollingPolicy
  parser
  tags[]
  lastCursor
  lastScanAt
```

Initial source classes:

```text
github_topic
github_repository
github_release_feed
github_search
awesome_list
paper_feed
benchmark_repository
skill_repository
manual_candidate
```

Initial examples may include:

```text
DeepSeek Harness + dsh-plugin ecosystem
Codex repository + related agent tooling
MCP ecosystems
Claude coding-agent ecosystem
agent debugging/evaluation projects
browser/computer/mobile agent projects
```

The exact registry should be data-driven.

---

# 5. Ecosystem Radar

## 5.1 Responsibilities

Radar discovers deltas, not full re-analysis every time.

Track:

```text
new repository
new release
important commit
new tag/topic entry
license change
manifest change
security-sensitive dependency change
benchmark claim change
archive/unarchive
maintenance inactivity
```

## 5.2 Discovery metadata

```text
SourceSnapshot
  sourceId
  candidateLocator
  discoveredAt
  sourceRevision
  sourceVersion
  title
  description
  stars/forks/downloads optional
  updatedAt
  licenseHint
  topics[]
  defaultBranch
  archived
  metadataHash
```

Popularity fields are informational only.

## 5.3 Delta policy

Re-evaluate only when meaningful fingerprints change:

```text
manifest hash
README/design hash
source tree fingerprint
release version
license hash
security-sensitive file hash
benchmark file hash
```

Avoid wasting R&D compute on cosmetic README churn.

---

# 6. Candidate normalization

Every source becomes the same canonical object.

```text
CapabilityCandidate
  id
  sourceType
  sourceUrl
  sourceRevision
  sourceVersion
  discoveredAt

  name
  summary
  problemStatement
  capabilityClass
  architecturePattern
  integrationShape

  license
  provenance
  maturity
  maintenance
  tests
  benchmarkClaims

  requiredCapabilities[]
  requestedPermissions[]
  securitySignals[]

  overlapWithCodeLocal[]
  noveltyScore
  maturityScore
  riskScore
  expectedValueScore

  disposition
  evidenceRefs[]
```

Candidates are immutable by revision. A new upstream revision creates a new analysis version.

---

# 7. Capability Genome

The Capability Genome is the normalized knowledge extracted from external systems.

## 7.1 Capability classes

Initial taxonomy:

```text
context
memory
retrieval
coding-loop
editing
patching
worktree
process
sandbox
permission
secret-security
network-policy
agent-graph
multi-agent
mailbox
workflow
routing
model-provider
token-economy
cost-governor
savepoint-undo
verification
debugging
failure-recovery
observability
browser
computer
mobile
vision
voice/media
MCP
skills
plugin-infrastructure
cloud-runtime
remote-runtime
UX
benchmark
```

Taxonomy is extensible but should remain bounded and reviewed.

## 7.2 Pattern extraction

For each candidate extract:

```text
Problem solved
Core primitive
State model
Execution model
Security assumptions
Persistence model
Concurrency model
Failure behavior
Token/cost implications
User experience
Known limitations
What is novel vs CodeLocal
```

## 7.3 Example distilled genes

The DSH ecosystem scan already demonstrates useful categories.

### Reversible context

Observed pattern:

```text
compress old surface range
retain original durable history
search compacted history
expand/decompress when needed
```

CodeLocal target:

```text
Reversible Context Ledger
```

### Savepoint/rewind

Observed pattern:

```text
message-level change tracking
snapshot timeline
rollback without depending exclusively on Git
safe-mode/offline recovery
```

CodeLocal target:

```text
Universal Savepoint Engine
```

### Auto permission

Observed pattern:

```text
workspace sandbox by default
classify risky operations
one-shot widening
avoid standing Full Access
```

CodeLocal target:

```text
Capability Lending + Policy Kernel V2
```

### Dynamic permission projection

Observed pattern:

```text
show model only escalation parameters valid for current session
```

CodeLocal target:

```text
Dynamic Capability Projection
```

### Cost/token metering

Observed pattern:

```text
input/cache/output accounting
provider/model pricing
subscription quota windows
budgets
```

CodeLocal target:

```text
Token & Cost Governor used by Router
```

### Agent dispatch tiers

Observed pattern:

```text
cheap worker first
escalate on evidence-backed failure
workspace holder/guard
orphan visibility
```

CodeLocal target:

```text
Evidence-Based Engine Escalation
```

### Agent debugging

Observed pattern:

```text
Detect -> Attribute -> Recover -> Rerun -> Evaluate
```

CodeLocal target:

```text
Failure Intelligence Engine
```

### Memory evolution

Observed pattern:

```text
project/global memory tracks
branch-aware memory
confirmation/trust before durable promotion
cross-device continuity
```

CodeLocal target:

```text
Evidence/Scope-aware Project Brain evolution
```

---

# 8. Overlap and novelty analysis

Before any implementation proposal, compare candidate primitives to existing CodeLocal packages and plans.

Output:

```text
OverlapReport
  exactOverlap[]
  partialOverlap[]
  missingCapabilities[]
  candidateAdvantages[]
  CodeLocalAdvantages[]
  migrationRisk
  duplicateArchitectureRisk
```

Disposition rules:

```text
Already better in CodeLocal -> reject/learn test cases only
Same concept, better implementation idea -> native enhancement proposal
Complementary capability -> adapter/skill/core candidate
Competing architecture -> benchmark before decision
Pure UI/theme/cosmetic -> low priority unless UX goal requires it
```

---

# 9. Provenance model

Every candidate and distilled pattern keeps provenance.

```text
Provenance
  sourceUrl
  sourceRevision
  sourceVersion
  authors/project
  licenseId
  licenseTextHash
  filesReferenced[]
  extractedAt
  extractorVersion
```

If actual source code is adapted or copied, record:

```text
codeReuse = true
sourceRanges/files
requiredNotices
modificationNotes
```

If only an architectural idea is learned:

```text
codeReuse = false
patternReference = source/document
```

This distinction is important for both licensing and engineering clarity.

---

# 10. License gate

The engine does not make legal conclusions beyond configured policy, but it must prevent accidental reuse without classification.

Initial classes:

```text
permissive
weak-copyleft
strong-copyleft
source-available/custom
unknown
no-license
```

Policy examples:

```text
MIT/Apache/BSD-like:
  architecture learning allowed;
  code reuse only with required notices/provenance.

GPL/AGPL-like:
  architecture learning/reimplementation may be evaluated;
  direct code reuse requires explicit legal/product decision.

Unknown/no-license:
  no direct code reuse;
  architecture observation only under project policy.
```

A candidate with `unknown` license cannot become `Native Core` through copied implementation.

---

# 11. Static Security Gate

Treat repository source, manifests, install steps, build scripts, and documentation as untrusted.

## 11.1 Static checks

Inspect:

```text
package/install lifecycle scripts
shell scripts
postinstall/preinstall
build hooks
binary downloads
curl|bash patterns
PowerShell remote execution
Git hooks
subprocess APIs
raw filesystem access
network clients
credential reads
process.env usage
secret files
SSH/keychain access
home-directory writes
system-directory writes
Docker privileged flags
browser extension permissions
native addons
unsigned binaries
self-update behavior
telemetry
```

## 11.2 Dependency checks

Collect:

```text
direct dependency count
transitive dependency count
native dependencies
install scripts
known vulnerable versions when data available
unpinned Git dependencies
registry provenance
lockfile state
```

## 11.3 Capability declaration

Produce a capability request summary:

```text
filesystem.read
filesystem.write
network.outbound
process.spawn
env.read
secret.consume
browser.control
device.control
container.privileged
```

Candidates that cannot be meaningfully sandboxed receive a higher risk score.

---

# 12. Lab Sandbox

Dynamic evaluation must happen outside trusted developer state.

## 12.1 Lab principles

- disposable environment;
- no real production credentials;
- no inherited shell secrets;
- no access to user home by default;
- dedicated synthetic repositories;
- controlled network;
- bounded CPU/memory/time;
- file/network/process audit;
- explicit artifact export only.

## 12.2 Candidate profiles

```text
STATIC_ONLY
BUILD_ONLY
UNIT_TEST
INTEGRATION_TEST
RUNTIME_EXPERIMENT
BENCHMARK
```

Higher-risk profiles require stronger isolation.

## 12.3 Network

Default:

```text
deny-all
```

Then allow exact registries/endpoints needed for the experiment.

## 12.4 Secrets

Use synthetic test secrets to detect exfiltration behavior.

Real provider credentials are not injected into a third-party candidate until an explicit reviewed integration phase.

---

# 13. Candidate benchmark harness

A candidate must solve a real CodeLocal problem better than current behavior.

## 13.1 Benchmark dimensions

```text
Verified success
Tokens
Cost
Latency
Model calls
Tool calls
Retries
Human intervention
Memory footprint
CPU
Disk
Security events
Recovery behavior
Compatibility
```

## 13.2 Benchmark comparison

```text
Baseline A: current CodeLocal
Baseline B: proposed native V3 subsystem if available
Candidate implementation
Distilled/native prototype
```

This distinguishes:

- candidate is useful as-is;
- candidate idea is useful but CodeLocal-native implementation is better;
- candidate does not improve measurable behavior.

## 13.3 Reproducibility

Every benchmark stores:

```text
candidate revision
CodeLocal revision
runtime image/version
model/provider
benchmark dataset version
policy profile
seed when applicable
raw artifacts refs
summary metrics
```

---

# 14. Scoring model

Do not use a single popularity score.

Conceptual score:

```text
AssimilationScore =
    ExpectedValue
  * EvidenceStrength
  * Maturity
  * Compatibility
  * Maintainability
  * SecurityConfidence
  * LicenseCompatibility
  * Novelty
```

Each factor is normalized separately.

Suggested qualitative states:

```text
HIGH
MEDIUM
LOW
BLOCKED
UNKNOWN
```

A `BLOCKED` security or license gate prevents promotion regardless of popularity.

---

# 15. Promotion dispositions

Exactly one primary disposition per candidate revision.

## 15.1 NATIVE_CORE

Use when:

- capability is strategically central;
- it should share CodeLocal state/security/runtime semantics;
- long-term ownership matters;
- external dependency would weaken invariants.

Examples:

```text
Context Ledger
Policy Kernel
Failure Intelligence
Savepoint Engine
```

## 15.2 ADAPTER

Use when:

- third-party engine/service owns domain-specific behavior;
- integration boundary is stable;
- CodeLocal should not reimplement provider internals.

Examples:

```text
Codex engine adapter
Claude engine adapter
specialized cloud runtime provider
```

## 15.3 SKILL

Use when:

- value is mostly workflow/knowledge;
- no new privileged runtime primitive is needed;
- capability can be expressed as verified CodeLocal actions.

## 15.4 LAB_EXPERIMENT

Use when promising but immature.

## 15.5 WATCH

Use when useful but current timing/value is insufficient.

## 15.6 REJECT

Use when:

- duplicate of stronger CodeLocal behavior;
- unacceptable risk;
- incompatible license for intended reuse;
- no measurable value;
- abandoned or broken architecture;
- complexity exceeds benefit.

Rejected candidates retain evidence so future scans do not repeat the same analysis without reason.

---

# 16. Pattern Distiller

Winning ideas should become provider-neutral design artifacts before implementation.

Output:

```text
PatternProposal
  problem
  currentCodeLocalState
  externalPattern
  distilledInvariant
  proposedCodeLocalDesign
  stateModel
  API/internal contracts
  security implications
  migration implications
  benchmark plan
  provenance
```

The Distiller should explicitly answer:

```text
What is the smallest primitive worth absorbing?
What should remain external?
What assumptions do we reject?
How does this fit existing CodeLocal packages?
How will we prove it improved the product?
```

---

# 17. Proposal generation

The system may create a proposal branch or issue containing:

```text
ADR
candidate report
architecture delta
benchmark evidence
security report
license/provenance report
implementation checklist
```

It must not directly merge production code.

Suggested branch namespace:

```text
research/ecosystem-<candidate>
experiment/<capability>
feat/<promoted-capability>
```

---

# 18. Ecosystem Watch automation

The discovery loop should support periodic monitoring.

The watch job should report only meaningful changes:

```text
new candidate with high novelty
important security architecture
new context/memory technique
new agent orchestration pattern
new token/cost optimization
new failure/debugging method
new browser/computer/mobile capability
new adapter/runtime innovation
```

Notifications should contain:

```text
repository/project
what changed
why it matters
CodeLocal overlap
recommended disposition
whether benchmark is needed
```

Avoid notification spam for themes, cosmetic forks, and low-value churn.

---

# 19. Candidate report template

Every serious candidate should produce:

```markdown
# Candidate: <name>

## Source
URL / revision / version / license

## Problem solved

## Architecture

## Novel capability

## Overlap with CodeLocal

## Security assumptions

## Runtime permissions

## Token/cost implications

## Maturity/tests

## Benchmark claims

## Lab results

## Recommended disposition
Native Core / Adapter / Skill / Lab / Watch / Reject

## Proposed CodeLocal primitive

## Verification plan

## Provenance
```

---

# 20. Security threat model

## Threat: malicious repository

Mitigation:

- static-only first;
- no install/build before gate;
- sandbox execution;
- no secrets;
- controlled network.

## Threat: supply-chain takeover after initial approval

Mitigation:

- pin source revision;
- immutable analysis per revision;
- re-run gates on revision/dependency changes;
- never blindly float `latest` in trusted production integrations.

## Threat: prompt injection in README/source comments

Mitigation:

- treat repository text as untrusted evidence;
- no repository text may authorize actions;
- candidate analysis policy is system-owned;
- tool execution decisions remain Policy Kernel controlled.

## Threat: license drift

Mitigation:

- fingerprint license per candidate revision;
- re-gate on change.

## Threat: benchmark gaming

Mitigation:

- CodeLocal-owned benchmark corpus;
- independent verification;
- candidate cannot define final success criteria alone.

## Threat: ecosystem spam

Mitigation:

- source ranking;
- dedupe;
- novelty threshold;
- low-priority queue;
- no auto-execution.

---

# 21. Data retention

Keep durable:

```text
candidate metadata
source revision
provenance
license classification
security report
benchmark summary
promotion decision
architecture proposal
```

Raw source copies should follow bounded cache/retention policy rather than becoming permanent project data unless needed for a reviewed experiment.

---

# 22. Observability

Every assimilation run gets a trace:

```text
assimilationTraceId
sourceId
candidateId
candidateRevision
gate transitions
security findings
benchmark runs
disposition
review decisions
```

This lets the team answer:

> Why did we absorb this idea?

or:

> Why did we reject it six months ago?

without redoing research from scratch.

---

# 23. APIs/internal contracts

Do not expose a huge public MCP surface.

Potential internal Go interfaces:

```go
type SourceAdapter interface {
    Discover(ctx context.Context, cursor Cursor) ([]SourceSnapshot, Cursor, error)
}

type CandidateAnalyzer interface {
    Analyze(ctx context.Context, snapshot SourceSnapshot) (CapabilityCandidate, error)
}

type Gate interface {
    Evaluate(ctx context.Context, candidate CapabilityCandidate) (GateResult, error)
}

type Benchmarker interface {
    Run(ctx context.Context, candidate CapabilityCandidate, suite BenchmarkSuite) (BenchmarkResult, error)
}

type Distiller interface {
    Distill(ctx context.Context, candidate CapabilityCandidate, evidence EvidenceBundle) (PatternProposal, error)
}
```

Implementation may evolve, but the separation of discovery/analyze/gate/benchmark/distill/promotion should remain.

---

# 24. Package proposal

Conceptual incremental layout:

```text
internal/ecosystem/
  source/
  radar/
  candidate/
  genome/
  provenance/
  license/
  security/
  lab/
  benchmark/
  distill/
  promotion/
  store/
```

Do not add all packages at once if the first implementation can live in fewer cohesive packages.

---

# 25. Initial implementation phases

## E0 — Source Registry + Radar

Deliver:

- source registry schema;
- GitHub topic/repository adapter;
- delta fingerprinting;
- candidate queue;
- no code execution.

Exit criteria:

- can discover new/changed DSH ecosystem candidates deterministically;
- repeated scan with no changes does not duplicate work.

## E1 — Candidate/Genome Schema

Deliver:

- `CapabilityCandidate`;
- capability taxonomy;
- overlap placeholders;
- provenance.

Exit criteria:

- candidate report can be generated without executing upstream code.

## E2 — License/Provenance Gate

Deliver:

- license fingerprint/classification;
- reuse-policy decision;
- source SHA persistence.

Exit criteria:

- no candidate can enter dynamic Lab without provenance metadata.

## E3 — Static Security Gate

Deliver:

- manifest/build/install/script scanners;
- dependency/capability summary;
- risk score;
- hard blocks for dangerous unknowns.

Exit criteria:

- known malicious fixtures fail before execution.

## E4 — Lab Sandbox

Deliver:

- disposable runtime provider;
- no real secrets;
- default deny network;
- process/file/network audit.

Exit criteria:

- malicious test fixture cannot read host secrets or write outside sandbox.

## E5 — Benchmark Harness

Deliver:

- candidate experiment contract;
- baseline comparison;
- reproducible artifacts.

Exit criteria:

- at least one context candidate and one policy/runtime candidate compared against CodeLocal baseline.

## E6 — Pattern Distiller

Deliver:

- proposal schema;
- provenance links;
- CodeLocal architecture delta;
- generated ADR/report.

Exit criteria:

- candidate can produce a reviewed native-core proposal without copying upstream code.

## E7 — Promotion Workflow

Deliver:

- disposition state machine;
- review gate;
- branch/issue proposal generation;
- no auto-merge.

Exit criteria:

- complete Discover → Gate → Benchmark → Distill → Promote/Reject lifecycle demonstrated.

---

# 26. First candidate backlog

Use the current ecosystem research as initial test cases.

## Candidate family A — Context

Evaluate patterns for:

- reversible compression;
- checkpoint search;
- context decompression;
- model-driven vs runtime-driven compaction;
- token-pressure nudging;
- hybrid retrieval over compacted history.

Expected output:

```text
Reversible Context Ledger V3 proposal
```

## Candidate family B — Savepoints

Evaluate:

- message-level rollback;
- workspace snapshotting;
- crash recovery;
- safe mode;
- snapshot GC;
- secret-safe export.

Expected output:

```text
Universal Savepoint Engine proposal
```

## Candidate family C — Permissions

Evaluate:

- ordered rule DSL;
- project hierarchy;
- network policy;
- auto classification;
- one-shot widening;
- session-aware tool schemas.

Expected output:

```text
Policy Kernel V2 + Capability Lending + Dynamic Projection
```

## Candidate family D — Agent orchestration

Evaluate:

- cheap/pro tiers;
- escalation after failure;
- workspace locks;
- orphan workers;
- external-agent dispatch;
- durable team communication.

Expected output:

```text
Adaptive Agent Scheduler + Engine Router V2
```

## Candidate family E — Debugging

Evaluate:

- trajectory schema;
- root-cause attribution;
- counterfactual rerun;
- recovery strategy evaluation;
- error corpus generation.

Expected output:

```text
Failure Intelligence Engine
```

## Candidate family F — Memory

Evaluate:

- project/global tracks;
- branch-scoped knowledge;
- user-confirmed memory;
- automated logs;
- cross-device sync;
- stale-memory handling.

Expected output:

```text
Project Brain evidence/scope evolution
```

---

# 27. What must NOT happen

Do not:

- add hundreds of DSH plugins as CodeLocal dependencies;
- execute arbitrary GitHub repositories on the user's machine;
- copy code without license/provenance review;
- create a second Project Brain because a plugin has its own memory system;
- create a second Policy Kernel because a plugin has its own permission engine;
- create public MCP tools for every ecosystem feature;
- let popularity override security/benchmark gates;
- allow repository README instructions to grant permissions;
- auto-push/auto-merge generated improvements;
- trust upstream benchmark claims without CodeLocal verification.

---

# 28. Definition of Done

The Ecosystem Assimilation Engine is production-ready when:

1. New ecosystem candidates can be discovered incrementally.
2. Every candidate has immutable provenance and source revision.
3. License classification occurs before implementation reuse.
4. Static security scan occurs before execution.
5. Dynamic evaluation runs in an isolated Lab.
6. Real user/provider secrets are absent by default.
7. Candidate behavior can be benchmarked against CodeLocal baseline.
8. Architecture patterns can be distilled without copying source.
9. Promotion has explicit dispositions and review gates.
10. Rejected candidates retain evidence.
11. Revision changes trigger re-evaluation only when meaningful.
12. No candidate can silently enter production.
13. Assimilation reports are traceable from source to final CodeLocal capability.
14. The system can show at least one validated external idea improving a CodeLocal metric without weakening security.

---

# 29. Strategic outcome

The intended flywheel is:

```text
Open-source ecosystem invents something useful
             |
             v
      CodeLocal discovers it
             |
             v
      extracts the primitive
             |
             v
      checks license/security
             |
             v
        benchmarks it
             |
             v
     improves/distills it
             |
             v
 integrates the winning pattern
             |
             v
 every CodeLocal-compatible model benefits
```

The goal is not to own the largest plugin collection.

The goal is to own the strongest **validated capability architecture**.

> **The ecosystem becomes CodeLocal's external R&D surface; CodeLocal remains the trusted runtime, memory, security, and verification authority.**
