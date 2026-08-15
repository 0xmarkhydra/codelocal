# CodeLocal Knowledge V2 + Collective Intelligence + Code Quality Policy — Master Implementation Plan

Status: **Proposed for management approval**  
Date: **2026-08-15**  
Target: **CodeLocal 1.5.16+**  
Public MCP surface: **must remain 19 tools**  
Release status: **1.5.16 is the Memory Safety hotfix and remains paused only until the Memory Safety release gate passes; Knowledge V2 Core, Code Quality, and Collective Intelligence ship on separate trains**

> This document is the execution plan for the next CodeLocal intelligence hardening cycle. It complements, but does not replace, `PROJECT_BRAIN_MASTER_PLAN.md` and `UNIVERSAL_AGENT_RUNTIME_PLAN.md`. Those documents remain the architectural sources of truth for Project Brain and agent execution respectively.

---

## 1. Executive summary

CodeLocal already has the foundations of a durable Project Brain: project identity, scoped rules, long-term memory, pgvector recall, graph projection, verified Experience, Learned Skills, cross-device knowledge sync, and a compact 19-tool MCP surface.

The current memory implementation has exposed a structural weakness: operational task events such as `Files edited for task: ...` and `Verification evidence refreshed: ...` are being promoted into durable long-term memory. These records are embedded, projected into the graph, and later recalled back into AI context. Near-duplicates therefore accumulate and amplify noise, token use, and graph clutter.

The proposed fix is not a similarity-based patch. It is a controlled redesign of the intelligence storage boundary around one invariant:

> **Operational Event != Durable Knowledge.**

The program introduces five coordinated capabilities:

1. **Knowledge V2** — canonical structured knowledge with stable identity, revisions, provenance, temporal validity, deterministic promotion rules, and bounded hybrid recall.
2. **Knowledge Health Monitor** — continuous structural/behavioral health checks, automatic quarantine/suppression for safe cases, anomaly detection, and release/canary gates.
3. **Four-plane intelligence storage** — operational activity, rebuildable local code intelligence, durable private knowledge, and verified Experience/Skills are separated physically and logically, then unified only at retrieval time.
4. **Collective Intelligence** — privacy-preserving aggregate reusable patterns learned from eligible verified outcomes across users, without exposing raw cross-tenant project knowledge.
5. **Code Quality Policy Engine** — enforceable project coding conventions such as function/file size, nesting, complexity, and orchestrator-only functions; deterministic local checks first, semantic AI review only for ambiguous cases.

The target user effect is simple:

- fewer repeated instructions;
- cleaner and smaller AI context;
- faster subsequent tasks;
- automatic detection of knowledge degradation;
- consistent coding conventions across ChatGPT, Codex, Claude, and future agents;
- better recommendations from verified aggregate engineering experience;
- no need for users to manually inspect the Brain for structural corruption.

---

# 2. Current confirmed problem

## 2.1 Root cause

`internal/mcpgateway/task_memory.go::longTermMemoryInput()` currently turns several operational outcomes into long-term memory inputs, including:

- successful edits: `Files edited for task: ...`;
- verification progress: `Verification evidence refreshed; more checks may still be required ...`;
- tool errors;
- some ready-state task events.

These inputs are sent asynchronously to `Memory.Ingest`, which embeds the summary and projects qualifying workspace/project records into the memory graph.

The current identity path is execution-oriented: session/workspace or project/repository, operation ID, and exact summary contribute to an idempotency hash. SQL `ON CONFLICT(id)` therefore deduplicates only exact identities, not semantically equivalent durable facts. Different sessions, operation variants, or slightly different summaries can create new records.

The observed failure chain is:

```text
Operational tool event
        -> durable memory row
        -> embedding
        -> graph node / edges
        -> hybrid recall candidate
        -> AI context
        -> more noise and token cost
```

This is a model-boundary problem, not only a deduplication problem.

## 2.2 Release decision

`1.5.16` is intentionally narrowed to a **Memory Safety hotfix**. It MUST NOT be published until operational-event durable ingestion is stopped, known legacy operational noise is blocked from normal recall, targeted regression tests pass, and the safety/token smoke metrics are acceptable. Full Knowledge V2 schema/revision work, Code Quality Policy, and Collective Intelligence are not blockers for this hotfix and move to later release trains.

This prevents an architecture rewrite from delaying the production fix for a confirmed data-quality bug.

---

# 3. Non-negotiable architecture principles

1. **Operational events never become canonical long-term knowledge directly.**
2. **The only canonical durable truth is PostgreSQL Knowledge Object + Revision + Provenance.** JSONB is payload; embeddings and Knowledge Graph edges are rebuildable derived indexes, never independent sources of truth.
3. **Stable semantic identity + revision history replaces append-only near-duplicate facts, with explicit scalar/set cardinality and qualifiers where one stable key may have multiple simultaneous values.**
4. **Verified task completion produces Experience first.** Automatic facts/decisions/skills may only be promoted later through an explicit promotion gate; tool success never writes canonical knowledge directly.
5. **Any automatic durable learning accepted after the interactive response must cross a durable outbox/queue boundary with idempotent at-least-once consumers.** Naked goroutines are best-effort only and must not be the durability mechanism for canonical knowledge.
6. **Automatic consolidation, embedding, graph projection, health auditing, and collective aggregation stay outside the interactive hot path.**
7. **Local rebuildable Code Graph and cloud durable Knowledge Graph remain separate systems.**
8. **Deterministic code/metadata analysis is preferred over LLM calls. AI is used only for semantic ambiguity.**
9. **Private project/user knowledge is tenant scoped. Collective Intelligence receives only eligible, de-identified, aggregated patterns.**
10. **Knowledge Health may quarantine/suppress automatically, but must not silently delete or rewrite canonical user/project facts. Security/scope invariants use hard circuit breakers rather than a compensating aggregate health score.**
11. **Coding rules that can be checked deterministically are enforced by CodeLocal, not trusted to the model prompt.**
12. **No increase to the 19-tool public MCP surface. Internal implementation may add private operations/workers only.**

---

# 4. Target architecture

```text
                 USER / AGENT / REPOSITORY
                           |
                           v
                 +-------------------+
                 | Typed Ingestion DAG|
                 +---------+---------+
                           |
              +------------+------------+
              |                         |
              v                         v
     OPERATIONAL ACTIVITY       KNOWLEDGE CANDIDATES
     task/audit/history          explicit / verified
     TTL, no embedding                   |
              |                         v
              |                Promotion / Consolidation
              |                         |
              |                         v
              |                Canonical Knowledge V2
              |                         |
              |          +--------------+--------------+
              |          |              |              |
              |          v              v              v
              |       JSONB          pgvector       semantic graph
              |          |              |              |
              |          +--------------+--------------+
              |                         |
              |                         v
              |               Knowledge Health Monitor
              |                         |
              +-------------------------+
                                        |
                         +--------------+--------------+
                         |                             |
                         v                             v
                 Local Code Graph              Verified Experience
                 rebuildable/git               + Learned Skills
                         |                             |
                         +--------------+--------------+
                                        |
                                  Context Router
                                        |
                                  Recall Guard
                                        |
                                        v
                                    AI / Agent
```

A separate optional path derives privacy-safe cross-user patterns:

```text
Verified private outcomes
       -> eligibility
       -> de-identification
       -> cohort aggregation
       -> Collective Health
       -> Collective Pattern Store
       -> Recommendation Router
```

---

# 5. Four-plane storage model

## Plane A — Activity / Telemetry

Purpose: debugging, progress, audit, recent task state.

Examples:

- file edited;
- verification refreshed;
- test executed;
- terminal command completed;
- agent iteration;
- transient tool error.

Properties:

- local/task/audit store or bounded server telemetry;
- retention/TTL, e.g. 7-30 days by policy;
- **no durable embedding**;
- **no canonical graph projection**;
- **not returned by normal long-term recall**.

## Plane B — Local Code Intelligence

Purpose: answer repository/code questions cheaply and accurately.

Contains:

- files/modules;
- symbols;
- imports;
- call graph;
- implementations/references;
- Git state/commit identity;
- optional communities/process flows;
- quality-policy structural metrics.

Properties:

- stored locally;
- rebuildable from repository + Git identity;
- incrementally updated on file/Git changes;
- raw source code does not need to be uploaded to cloud by default;
- inspired by the useful deterministic/indexing patterns of GitNexus, without treating the code graph as user long-term memory.

## Plane C — Private Durable Knowledge

Purpose: knowledge that should survive model, chat, machine, checkout, and time.

Allowed canonical types include:

- `decision`;
- `project_fact`;
- `constraint`;
- `goal`;
- `preference`;
- `problem`;
- `milestone`;
- `architecture`;
- `relationship`;
- `person` / `company` where explicitly appropriate;
- `verified_experience` references.

Properties:

- PostgreSQL source of truth;
- stable identity;
- revisions;
- provenance;
- temporal validity;
- lifecycle/status;
- optional embedding;
- semantic graph projection only after promotion.

## Plane D — Experience / Learned Skills

Purpose: preserve verified execution knowledge rather than raw event history.

An Experience should record:

```text
problem/context
 -> attempted approach
 -> verified outcome
 -> evidence
 -> reusable lesson
```

Learned Skills are derived reusable workflows with capability fingerprints, verification evidence, and lifecycle/feedback.

Experience/Skill data may contribute to Collective Intelligence only after privacy eligibility checks.

---

# 6. Knowledge V2 canonical model

## 6.1 Logical object

```json
{
  "schemaVersion": 2,
  "knowledgeId": "knw_...",
  "stableKey": "project.architecture.project-brain",
  "type": "architecture",
  "scope": {
    "projectId": "prj_...",
    "repositoryId": null,
    "branch": null
  },
  "subject": {
    "type": "system",
    "id": "project-brain"
  },
  "predicate": "uses_architecture",
  "object": {
    "type": "architecture",
    "value": "server_first"
  },
  "summary": "Project Brain uses server-first architecture.",
  "status": "active",
  "confidence": 0.98,
  "importance": 0.95,
  "revision": 4,
  "validFrom": 1786800000000,
  "validUntil": null,
  "createdAt": 1786800000000,
  "updatedAt": 1786810000000
}
```

`summary` remains useful for display, lexical retrieval, and embedding, but MUST NOT be the canonical semantic identity.

## 6.2 Stable identity

Primary identity:

```text
tenant
+ semantic scope
+ knowledge type
+ stableKey
+ cardinality
+ qualifier/entityKey when cardinality=set
```

Examples:

```text
project.current_release                 scalar
project.database.strategy               scalar
project.architecture.project-brain      scalar
project.security.execution-boundary     scalar
problem.memory-pollution                scalar
user.preference.answer-length           scalar
project.constraint::<constraint-key>    set member
```

For `scalar` knowledge, changing the value creates a new revision of the same canonical object rather than a sibling fact. For `set` knowledge, independent simultaneous members MUST have distinct qualifiers/entity keys so unrelated constraints, dependencies, people, or relationships are never merged merely because they share a predicate.

Portable project identity is part of this invariant: `.codelocal/project.json`, `.codelocal/rules/**`, and `.codelocal/skills/**` must be commit-able when the project chooses to use them. Runtime-only workspace state such as `.codelocal/worktrees/**` remains ignored. A project marker is strong evidence, but must still be validated against tenant/repository evidence so copied templates cannot silently merge unrelated projects.

## 6.3 Temporal model

A mutable fact can be represented as:

```text
PostgreSQL   validFrom=T1 validUntil=T2
CockroachDB  validFrom=T2 validUntil=null
```

This preserves history without allowing two stale active facts to compete in recall.

## 6.4 Provenance

Private knowledge must be traceable to its origin, e.g.:

- conversation;
- explicit user memory;
- project file/rule;
- verified Experience;
- migration source;
- agent-generated candidate with verification evidence.

Provenance does not grant execution permission and must remain tenant scoped.

---

# 7. Database migration sequence and implemented rollout

The rollout is deliberately split so durability, staging, canonical truth, approval, health, and source provenance can be deployed and rolled back independently. Existing legacy memory remains readable; no first-rollout migration hard-deletes legacy rows.

## 7.1 Migration v26 — durable outbox — IMPLEMENTED

`codelocal_durable_outbox` provides tenant-scoped dedupe, `FOR UPDATE SKIP LOCKED` leasing, bounded retry/backoff, dead-letter state, retention, and at-least-once processing. Verified Experience and explicit-memory promotion work cross this durable boundary instead of relying on naked in-process goroutines.

## 7.2 Migration v27 — promotion candidate staging — IMPLEMENTED

Introduces:

```text
codelocal_knowledge_promotion_candidates
codelocal_knowledge_promotion_evidence
```

A processed verified Experience may create a **pending candidate only** when an explicit structured promotion hint passes deterministic schema/scope/privacy/confidence gates. Candidate rows represent semantic proposal revisions: candidate identity includes the canonical identity plus semantic fingerprint, while scalar value changes retain one later KnowledgeObject identity. Cross-candidate conflicts are consolidated by the deterministic approval layer rather than silently overwritten during upsert.

No v27 staging path writes Learned Skills, embeddings, or graph projections.

## 7.3 Migration v28 — canonical Knowledge V2 core — IMPLEMENTED

Introduces the only canonical durable truth:

```text
codelocal_knowledge_objects
codelocal_knowledge_revisions
codelocal_knowledge_provenance
```

Canonical promotion is transactional and idempotent. Scalar changes create a new revision of one KnowledgeObject; set members require a qualifier. Temporal validity closes the previous revision before activating the next. Revoked/conflicted canonical objects fail closed. Vector, graph, cache, and Skill state remain downstream/derived and are intentionally absent from v28.

## 7.4 Migration v29 — deterministic approval/consolidation — IMPLEMENTED

Adds candidate evaluation state:

```text
approval_reason
evidence_count
independent_task_count
evaluated_at
approved_at
```

Verified-Experience auto-approval requires deterministic corroboration from distinct task IDs; retries of one task do not count as independent evidence. Pending/approved sibling semantic proposals are conflict-checked. Already-promoted history does not block a later scalar revision.

## 7.5 Migration v30 — Knowledge Health/circuit breaker — IMPLEMENTED

Introduces `codelocal_knowledge_health` as derived/rebuildable health state. It persists counts and reason codes only, never raw knowledge payloads.

Hard breakers mark health `critical` and disable automatic promotion when the monitor detects:

- canonical object/revision/provenance integrity violations;
- operational-noise leakage into canonical knowledge;
- probable secret-bearing canonical payloads;
- privacy-boundary violations;
- an incomplete/truncated integrity scan.

Promotion conflicts and stale pending candidates are `degraded` signals, not automatic hard stops. A bounded periodic worker scans recently active projects so structural problems are detected without waiting for a human dashboard review.

## 7.6 Migration v31 — source-aware promotion/provenance — IMPLEMENTED

Introduces `codelocal_knowledge_promotion_sources` and generalizes canonical provenance beyond Experience-only evidence.

Supported durable source types in this phase:

```text
verified_experience
explicit_user_memory
```

Explicit project memory is eligible only when it is project-scoped, has a user-supplied stable key, uses a defined safe kind, and passes confidence/importance/privacy checks. It never creates a fake Experience and free text is not reclassified into architecture/decision semantics by inference. Mutable memory uses a revision token so a stale outbox event cannot overwrite the current durable memory value.

The initial safe explicit-memory kinds are:

```text
decision
project_fact
constraint
goal
milestone
problem
```

`preference`, `user_fact`, `idea`, `person`, and `company` remain legacy-only until their canonical identity/cardinality model is explicitly defined.

## 7.7 K11 — bounded reconciliation/backfill — IMPLEMENTED, no schema migration required

The existing Knowledge Health lifecycle performs bounded reconciliation of safe project-scoped explicit memories. This recovers:

- eligible memories created before the V2 producer existed;
- eligible memories whose earlier promotion-outbox enqueue failed;
- a mutable keyed memory after its current value/confidence/importance changes.

Reconciliation requires exactly one `memory-key:` symbol, reads only the current durable memory row, and is idempotent. The query excludes a memory once a source-aware candidate already represents its current summary/confidence/importance, allowing each bounded sweep to progress through the backlog without a separate cursor table.

Maintenance order is deliberately:

```text
Health
 -> bounded explicit-memory reconciliation
 -> Health again
 -> reevaluate older pending candidates
```

A critical health result skips reconciliation/promotion entirely. After reconciliation, health is checked again before older pending candidates may proceed.

## 7.8 Migration v32 / K12 — project and repository identity hardening — IMPLEMENTED

Introduces tenant-scoped `codelocal_repository_aliases` so repository identity survives remote renames/migrations without treating Git lineage as a globally unique repository identifier.

Identity precedence is deliberately conservative:

```text
exact known remote alias
 -> canonical repository_id
 -> selected logical project
 -> unique lineage alias inside that project only
```

Lineage never overrides a direct remote alias and never globally merges fork-like repositories. Duplicate incoming lineage in one multi-repo snapshot fails closed for lineage rescue.

`.codelocal/project.json` v1 is also enforced as a strong but non-authoritative marker:

```json
{
  "schemaVersion": 1,
  "projectId": "stable-project-id",
  "name": "BIDDI"
}
```

Invalid/unsupported marker payloads are rejected locally and revalidated by Cloud. An unversioned marker from an older runtime may only continue an already-existing project when repository/lineage evidence agrees; it cannot create a new project. Competing direct repository evidence rejects the marker instead of merging projects. The project binding returns an observed→canonical repository ID map, and Knowledge Manifest/Delta sync uses that same map so remote migration cannot split repository-scoped knowledge after project resolution.

## 7.9 Migration v33 / K13 — Shadow Readiness Gate — IMPLEMENTED

Introduces `codelocal_knowledge_shadow_metrics` as derived/rebuildable rollout evidence. It stores aggregate counts/latency only:

```text
sample/success/error counts
legacy/canonical non-empty sample counts
legacy/canonical hit totals
high-confidence canonical hit total
slow sample count
duration totals / last / max
first / last sample timestamp
```

It never persists knowledge summary, stable key, subject/object, repository ID, or branch name. Shadow failures are counted as well as successes, and metric writes run after the asynchronous shadow recall so they do not add interactive response latency. Aggregate rows expire through bounded retention.

Per-project readiness is deterministic:

```text
collecting | ready | blocked
```

A `critical` Knowledge Health state or disabled auto-promotion always forces `blocked`. Missing/degraded health, insufficient/freshness-lapsed samples, or too few canonical observations remain `collecting`. Excessive shadow errors, excessive slow samples, or a low high-confidence canonical-hit ratio force `blocked`. Only a healthy project that satisfies every configured threshold becomes `ready`.

`ready` is evidence for a future rollout decision only; K13 does **not** enable hybrid/live recall.

## 7.10 Current read rollout — shadow only

Canonical V2 retrieval is implemented with tenant/project/repository/branch/temporal bounds and deterministic ranking, but the gateway remains in asynchronous `shadow` mode by default:

```text
CODELOCAL_KNOWLEDGE_V2_READ_MODE=shadow
```

Shadow reads never change legacy vector+graph recall output and do not add V2 query latency to the interactive path. Only compact count/latency metrics are logged; no knowledge summary/stable key/repository/branch payload is emitted in shadow telemetry.

## 7.11 Deterministic Code Quality Policy Engine — IMPLEMENTED FOUNDATION

The first production foundation is implemented as a zero/near-zero-token local structural analyzer. It adds no public MCP tools and is emitted from the existing `verify.changes` result as `qualityPolicy`.

Portable project configuration is optional at:

```text
.codelocal/quality.json
```

Schema v1 is language-neutral. The first analyzer is Go-native and uses `go/parser`, AST structure, and token positions; future TypeScript/JavaScript/Rust analyzers must reuse the same policy/result model rather than inventing another quality system.

Example strict project policy — these values are examples, **not universal CodeLocal defaults**:

```json
{
  "schemaVersion": 1,
  "languages": {
    "go": {
      "maxFileLines": 400,
      "maxFunctionLines": 10,
      "maxNestingDepth": 3,
      "maxCyclomaticComplexity": 12,
      "maxParameters": 5,
      "singleResponsibility": true,
      "severity": {
        "maxFileLines": "advisory",
        "maxFunctionLines": "blocking"
      },
      "orchestrators": {
        "enabled": true,
        "namePatterns": ["Switch*", "Route*", "Dispatch*"],
        "explicitFunctions": ["HandleWorkflow"],
        "maxOwnedStatements": 2,
        "severity": "advisory"
      }
    }
  },
  "exemptions": {
    "generated": true,
    "tests": true,
    "migrations": true,
    "paths": ["dsl/**"]
  }
}
```

Foundation invariants:

- executable LOC means token-bearing source lines; blank and comment-only lines do not count;
- generated code, tests, migrations and configured DSL/framework paths can be exempted;
- default thresholds are deliberately broad and advisory so upgrading CodeLocal does not suddenly break existing repositories;
- a project can explicitly disable a threshold with `0`;
- only a rule explicitly configured as `blocking` may prevent the agent quality gate from finalizing;
- `singleResponsibility` is initially a conservative structural proxy, not an LLM claim about business semantics, and remains advisory unless the project explicitly opts into blocking;
- orchestrator detection combines configured explicit roles/name patterns with AST behavior. Direct calls/control flow are coordination; owned transformations/state mutations count against `maxOwnedStatements`;
- quality findings are a separate structured channel and never inflate compiler/LSP diagnostic regression;
- `.codelocal/quality.json` is portable project configuration, while `.codelocal/worktrees/**` remains runtime-only and excluded from verification/indexing.

The current safe defaults for Go are advisory (`800` executable lines/file, `80`/function, nesting `6`, cyclomatic complexity `20`, parameters `8`). The user examples `400` lines/file and `10` lines/function belong in project policy, not hard-coded global behavior.

## 7.12 Context Compiler v3 deterministic compaction — IMPLEMENTED

Before rule-budget selection, the Context Compiler performs conservative exact-normalized deduplication across provider/authority sources. It collapses whitespace/case/trailing punctuation only; no embeddings or LLM are used and merely similar/opposite statements remain separate.

The highest-authority/specific representative survives while mandatory strength propagates across exact duplicates. This prevents duplicate AGENTS/Claude/Copilot copies from consuming rule budget multiple times or creating false mandatory overflow.

The existing context packet/benchmark path now exposes:

```text
inputRules
candidateRules
duplicateRules
inputChars
deduplicatedChars
usedChars
deduplicationRatio
```

No extra MCP tool or Cloud write is added. A regression fixture with 40 identical rules reduces 15,160 raw rule characters to one 379-character candidate before budget selection, while separate tests prove that opposite/merely similar rules are not merged.

## 7.13 Migration v34 — Learned Skills v2 portable project contributions — IMPLEMENTED FOUNDATION

Introduces tenant/project-scoped `codelocal_project_learned_skill_contributions` while retaining existing workspace-local learned-skill storage and metadata.

Portable v1 deliberately permits only sanitized semantic browser/computer workflow steps. Raw terminal/shell commands, approval tokens, secrets, absolute paths, coordinates, ephemeral window/element IDs and other machine bindings remain local. Cloud revalidates every portable payload and supplies canonical project scope; client payload cannot choose an arbitrary destination project.

Portable workflow identity is independent from project alias identity, while project scope remains part of the database key. Contributions retain per-device/workspace provenance instead of using cross-device last-write-wins.

Machine B receives an eligible Machine A workflow only as local status `imported`:

```text
Cloud portable workflow
 -> local imported suggestion
 -> context compatibility check
 -> no autonomous replay
 -> successful local verified execution
 -> local candidate
 -> repeated local verified success
 -> local trusted
```

Project-level evidence is aggregated with **one contribution per independent device** so multiple workspaces on one machine cannot manufacture corroboration. Aggregate health states are:

```text
single_source
collecting
corroborated
degraded
stale
```

`corroborated` is recommendation evidence only. It never grants local replay permission, never bypasses context fingerprints, and never overrides CodeLocal security/approval policy. Knowledge Graph/dashboard projection uses aggregate metadata only and never selects portable recipe steps/context into the graph payload.

Contributor evidence decays without deleting history. Default corroboration freshness is 180 days (`CODELOCAL_PORTABLE_SKILL_EVIDENCE_MAX_AGE_DAYS`). Older contribution rows remain as provenance but stop counting toward current aggregate health. A project portable workflow with no fresh contributors becomes `stale`; both `stale` and `degraded` Cloud suggestions are withheld from new local imports until newer verified evidence refreshes the workflow.

## 7.14 Guarded Knowledge V2 hybrid read — IMPLEMENTED, DEFAULT OFF

`CODELOCAL_KNOWLEDGE_V2_READ_MODE` now accepts `off | shadow | hybrid`, while `shadow` remains the default.

Hybrid is an explicit canary path, not an automatic promotion from shadow. Each live recall still performs the legacy vector/graph path first. Canonical Knowledge may supplement that result only when the same project's `KnowledgeV2Readiness` returns `ready`, which already requires healthy Knowledge Health plus sufficient fresh shadow samples, bounded error/slow rates and strong canonical confidence.

Initial live safeguards:

```text
shared readiness + canonical timeout: 220 ms
minimum canonical confidence: 0.95
maximum canonical supplements: 2
final memory packet limit: unchanged (normally 6)
exact-normalized duplicate summaries: suppressed
legacy relevance ordering: retained first
canonical graph projection: not used in live recall yet
```

Any missing project scope, readiness error, non-ready state, timeout, canonical query error, empty/low-confidence result or duplicate-only result fails soft to the legacy records. Shadow remains asynchronous and does not run in hybrid mode. This gives CodeLocal a deployable hybrid implementation without changing the production default before real readiness data approves a project.

## 7.15 Migration v35 — Privacy-first Collective Intelligence foundation — IMPLEMENTED, DEFAULT OFF

Collective learning now has an explicit storage/privacy boundary that is separate from private Project Brain knowledge.

Two independent gates are required:

```text
server rollout flag
AND
per-user preference
```

Both contribution and suggestion rollout flags default OFF. User preferences also default OFF. Runtime/device credentials cannot opt the user in; settings are exposed only through the authenticated web session API/UI with CSRF protection.

A verified Experience is reduced deterministically to a structured fingerprint containing only whitelisted/bucketed fields such as task kind, verification-check categories, file/symbol count buckets, execution-tool category, quality-score bucket, whether a learned skill was involved and whether a diff was observed. The collective fingerprint never includes:

```text
objective / task free-form text
root cause / verification summary
raw code
conversation text
project or repository identity
workspace or device identity
file paths or symbols
branch names
skill IDs
rules/context hashes
```

Idempotency uses a hashed event token. Each user has at most one aggregate row per pattern, preventing high-volume users from manufacturing contributor count. Recommendation success rate is averaged per user rather than weighted by raw sample volume.

Recommendation serving requires:

```text
user suggestion opt-in
server suggestion rollout enabled
fresh aggregate evidence
minimum independent cohort (default 20, hard privacy floor 5)
requesting user excluded from the cohort itself
```

The Knowledge dashboard exposes the opt-in controls and aggregate recommendation cards. Disabling contribution transactionally deletes that user's collective event ledger and pattern aggregates, withdrawing them from future cohorts. Suggestions currently remain dashboard-only and are **not injected into model context** until ranking/provenance/relevance gates are mature.

## 7.16 Migration v36 — Canonical Knowledge Graph projection — IMPLEMENTED

Canonical Knowledge V2 now has a dedicated tenant/project-scoped graph projection, but the graph remains explicitly **derived and rebuildable** rather than a second source of truth.

The projection is rebuilt from canonical Knowledge Objects + Revisions after the Knowledge Health maintenance sweep has rechecked both explicit-memory reconciliation and pending promotion. If the post-promotion health breaker becomes critical, projection is skipped.

Projected relationships are deliberately deterministic:

```text
Project -> Knowledge        HAS_CANONICAL_KNOWLEDGE
Repository -> Knowledge     SCOPES_KNOWLEDGE
Knowledge -> Revision       HAS_REVISION
Revision -> next revision   SUPERSEDED_BY
```

Only `private_project` canonical objects in active/stale/conflicted lifecycle participate. Secret-like stable keys are omitted from labels and secret-like historical summaries are redacted before projection. A configurable project cap (`CODELOCAL_KNOWLEDGE_GRAPH_MAX_REVISIONS_PER_PROJECT`, default 2000) prevents oversized rebuilds; exceeding the cap leaves the previous projection intact rather than publishing a partial graph.

Rebuild is transactional per project: old derived nodes/edges are deleted and the complete replacement is inserted in one transaction. Losing these tables therefore loses no durable knowledge; they can be recreated from canonical state. Dashboard graph reads the derived tables under the same tenant boundary and only renders edges whose endpoints are present in the bounded node packet.

## 7.17 Migration v37 — Canonical Graph projection freshness — IMPLEMENTED

The derived graph now records projection state in the same transaction as each successful project rebuild. State contains only counts/timestamps and projection sizes — never knowledge summaries, stable keys, paths or other content.

A separate aggregate freshness check compares current eligible canonical object/revision counts + latest source timestamp against the last projected state and reports:

```text
current
stale
missing
empty
```

`empty` is healthy only when canonical source and projection are both empty. If canonical knowledge is revoked/removed while old graph nodes still exist, freshness becomes `stale` until the next rebuild removes them. Dashboard, `/api/status` and `/health` expose only aggregate freshness counts/lag.

Graph freshness is intentionally **not** folded into canonical Knowledge Health/Hybrid readiness yet. The graph is a derived dashboard/index surface and Hybrid recall does not consume it; a stale graph therefore must not incorrectly downgrade otherwise healthy canonical truth. If future live retrieval depends on the canonical graph, that retrieval mode must add its own freshness gate first.

## 7.18 Still intentionally deferred

The following remain separate later rollout tracks:

```text
canonical embedding/vector projection
Learned Skill derivation from consolidated Experience
semantic/LLM extraction from free-form task text
Collective recommendation injection into model context
```

All future migrations must preserve:

- tenant scope in every read/write path;
- closed lifecycle/status sets;
- unique canonical identity and revision semantics;
- provenance/auditability;
- feature-gated reversible rollout;
- no silent legacy deletion or last-write-wins conflict resolution.

---

# 8. Typed ingestion DAG

The ingestion pipeline should use explicit phases and dependencies rather than one large memory function.

```text
collect
 -> classify
 -> normalize
 -> validate scope/privacy
 -> derive stable identity
 -> dedupe/conflict check
 -> promote/reject
 -> persist revision
 -> enqueue embedding
 -> enqueue graph projection
 -> health update
```

Each phase exposes structured output and metrics. Failures do not silently fall through to text-memory creation.

Patterns to borrow conceptually from existing systems:

- GitNexus: explicit indexing phases, incremental Git-aware work, deterministic repository analysis, evaluation harness;
- Graphiti: temporal validity, provenance, incremental facts, hybrid graph/vector retrieval;
- GraphRAG: knowledge-model abstraction separate from persistence and local/global retrieval modes;
- Mem0: token/latency benchmarking and entity linking, while avoiding append-only duplicate accumulation.

No external project becomes a required runtime dependency solely because its design pattern is reused.

---

# 9. Promotion and consolidation

## 9.1 Deterministic rejection

These MUST NOT enter durable knowledge automatically:

```text
edit progress
verification progress
test execution
terminal progress
context refresh
agent iteration
tool call noise
transient tool failure
```

## 9.2 Promotion candidates

Candidates may originate from:

- explicit user `remember`;
- project/rule/config sources;
- verified task completion;
- verified Experience;
- deliberate architecture/decision extraction;
- deterministic lifecycle milestones.

## 9.3 Promotion gate

Candidate must pass:

```text
schema valid
scope valid
allowed type
not operational noise
stable identity resolved
privacy classification safe
conflict state resolved or marked
confidence/importance policy
```

## 9.4 Dedupe hierarchy

1. exact `stableKey` / canonical identity;
2. deterministic semantic fingerprint: type + scope + normalized subject + predicate;
3. vector similarity as a duplicate candidate signal only.

Vector similarity MUST NOT silently merge two high-confidence canonical facts.

---

# 10. Async write architecture and hot-path performance

## 10.1 Auto-generated knowledge

Interactive path:

```text
tool/action completes
 -> task/audit/history update
 -> verified outcome may persist Experience
 -> return result
```

A verified task does **not** directly become a fact, decision, memory, or skill. Experience is the automatic learning boundary.

Promotion path:

```text
verified Experience / explicit candidate
 -> durable outbox row committed transactionally
 -> return / continue without waiting for semantic work
```

Durable workers consume the outbox with idempotent at-least-once semantics:

```text
claim job
 -> classify
 -> promote/consolidate
 -> canonical DB transaction
 -> enqueue/refresh embedding projection
 -> enqueue/refresh graph projection
 -> health audit
 -> mark job complete
```

Worker crashes/restarts may cause safe retries, never silent loss or duplicate canonical objects.

## 10.2 Explicit remember

Required durability boundary:

```text
validate
 -> canonical write transaction OR durable outbox accept
 -> ACK to caller
```

The caller must never receive a durability ACK based only on an in-process goroutine. Embedding, graph projection, consolidation, and health work remain asynchronous and rebuildable.

## 10.3 Recall hot path

Recall is allowed to block because context is required for correctness, but MUST be bounded:

```text
query
 -> tenant/project/repo/branch filter
 -> lifecycle/health filter
 -> stable/exact candidates
 -> vector candidates
 -> graph boost where useful
 -> RRF/MMR-like diversity
 -> bounded ContextPacket
```

No health scan, migration, community rebuilding, or LLM consolidation runs synchronously in recall.

---

# 11. Retrieval router

Not every question queries every intelligence store.

Examples:

### Code-specific

```text
"Where is this function called?"
```

Use:

```text
local Code Graph
+ exact repository rules
+ at most a few relevant durable decisions
```

### Project-global

```text
"What is the current Project Brain architecture?"
```

Use:

```text
canonical architecture/decision knowledge
+ project-level relationships/summaries
```

### User preference

```text
"How does this user prefer releases handled?"
```

Use:

```text
user/project preference knowledge only
```

Routing is a major token-control mechanism.

---

# 12. Knowledge Health Monitor

Knowledge Health is a first-class subsystem, not dashboard decoration.

## 12.1 Three layers

### Write-time guard

Before persistence:

- schema validation;
- operational-noise rejection;
- stable-key collision check;
- scope/branch validation;
- privacy eligibility;
- basic conflict detection.

### Continuous auditor

Background metrics include:

```text
duplicateRatio
operationalNoiseRatio
schemaViolationRate
stableKeyCollisionRate
conflictRatio
orphanNodeRatio
orphanEdgeRatio
crossScopeLeakage
knowledgeGrowthRate
knowledgeAmplificationRatio
promotionRate
embeddingFailureRate
embeddingVersionDrift
recallRedundancy
tokensPerRecall
recallLatencyP50/P95
migrationHealth
```

### Recall-time guard

Before context reaches the AI:

- exclude quarantined/revoked/superseded entries;
- enforce branch/project/repository compatibility;
- suppress near-duplicate candidates;
- enforce diversity and token budget;
- reject operational noise even if legacy rows remain.

## 12.2 Hard safety circuit breakers + health states

Safety invariants are evaluated **before** any aggregate 0-100 health score. They cannot be compensated by healthy unrelated metrics.

Immediate `critical` examples:

```text
crossTenantLeakage > 0
unauthorized organization/project access > 0
secret-class content promoted or embedded > 0
branch/repository hard-scope violation > 0
canonical revision integrity failure > 0
```

Only after hard invariants pass is the UX/operational health score calculated:

```text
90-100  healthy
75-89   degraded
50-74   unhealthy
<50     critical
```

Suggested runtime behavior:

- `healthy`: normal recall/promotion;
- `degraded`: stricter promotion and smaller recall;
- `unhealthy`: disable auto-promotion, quarantine suspicious candidates;
- `critical`: canonical read-only mode for automatic mutation; explicit trusted writes only.

The dashboard score is explanatory; safety circuit breakers are authoritative.

## 12.3 Safe self-healing

May happen automatically:

- quarantine deterministic operational noise;
- suppress weaker near-duplicate recall candidates;
- exclude superseded facts;
- regenerate stale embeddings;
- disable orphan graph edges;
- stop automatic promotion on anomaly spikes.

Must NOT happen automatically:

- delete canonical fact history;
- resolve two high-confidence conflicting decisions;
- rewrite user preference;
- merge ambiguous high-confidence knowledge.

Those become `needs_review`.

---

# 13. Safe legacy cleanup

Cleanup is staged:

```text
scan
 -> classify
 -> dry-run report
 -> quarantine
 -> exclude from recall
 -> detach graph projection
 -> remove/rebuild embeddings as appropriate
 -> retention period
 -> later deletion only after verification
```

Initial deterministic legacy-noise signatures include records with task/event characteristics and known prefixes such as:

```text
Files edited for task:
Verification evidence refreshed;
```

The classifier MUST preserve legitimate decisions, milestones, project facts, constraints, goals, preferences, problems, and verified scenarios.

Migration stores old->new identifiers for audit/rollback.

---

# 14. Local Code Graph strategy

Code intelligence and durable knowledge MUST NOT share one physical graph.

Local Code Graph is:

- repository-derived;
- commit/file-hash aware;
- rebuildable;
- optimized for symbol/import/call/process relationships;
- invalidated incrementally;
- disposable without losing Project Brain knowledge.

Durable Knowledge Graph is:

- small relative to code graph;
- semantic;
- temporal;
- revisioned;
- provenance-aware;
- multi-device/project aware;
- cloud durable.

Cross-links may use stable repository/symbol identifiers, but deleting/rebuilding the Code Graph must never delete canonical knowledge.

---

# 15. Collective Intelligence

## 15.1 Goal

Use aggregate verified engineering outcomes to improve suggestions for other users without moving raw project/user knowledge across tenants.

Principle:

> Private Brain understands a specific user/project. Collective Intelligence understands patterns that are commonly effective, but does not expose who contributed them.

## 15.2 V1 strategy

Do NOT fine-tune a shared model on raw user knowledge in V1.

Use explainable structured pattern mining and ranking first:

```text
Verified Experience
 -> eligibility
 -> de-identification
 -> normalization
 -> pattern fingerprint
 -> contribution limiting
 -> minimum cohort threshold
 -> aggregate outcome statistics
 -> collective health
 -> publish recommendation pattern
```

## 15.3 Eligible data

Eligible by policy:

- verified problem/solution/outcome classes;
- generic architecture patterns;
- generic testing/verification strategy;
- reusable tool/workflow combinations;
- generic performance/failure-resolution patterns.

Never contribute directly:

- raw source code;
- secrets/credentials;
- raw conversation;
- private URLs;
- absolute paths;
- user/company/customer identifiers;
- project names;
- unredacted natural-language summaries that can identify a tenant;
- user personal preference unless explicitly designed for a private-only model.

## 15.4 Consent

Cross-user learning is **private by default** and requires explicit opt-in.

Organization/Enterprise settings should distinguish:

```text
cross-project learning inside organization
cross-organization collective learning
```

Cross-organization contribution defaults OFF for enterprise unless configured otherwise.

## 15.5 Anti-leak and anti-manipulation

- minimum independent-contributor cohort before publication;
- one contributor/account cannot create artificial vote volume by repeating the same pattern;
- contributor diversity influences confidence;
- rare-pattern leakage checks;
- no private provenance exposed in recommendation serving;
- abuse/anomaly detection;
- collective recommendation never overrides local security or organization policy.

## 15.6 Collective storage

Separate security domain:

```text
collective_candidates
collective_patterns
collective_pattern_stats
collective_embeddings
collective_relations
recommendation_impressions
recommendation_outcomes
```

No serving query may join these tables back to private raw project knowledge to explain another tenant's suggestion.

## 15.7 Ranking

Example score inputs:

```text
context match
verified success rate
contributor diversity
confidence
freshness
security/health status
local project compatibility
user feedback
```

Popularity alone never implies correctness.

## 15.8 Feedback loop

Strong positive evidence:

```text
suggestion applied
 -> CodeLocal verification passes
 -> problem resolved
```

Strong negative evidence:

```text
suggestion applied
 -> verification fails / regression
 -> reverted or abandoned
```

Simple dismissal is weak evidence, not proof a pattern is wrong.

Later phases may consider differential privacy/federated learning after the structured system has enough users and a measurable privacy model.

---

# 16. Code Quality Policy Engine

## 16.1 Current state

Project Brain already resolves scoped rules and authority and compiles required/relevant instructions into the AI context. It can stop mutation when required rules cannot fit the mandatory context budget.

However, rules such as these are currently primarily guidance, not deterministic invariants:

```text
function <= 10 lines
file <= 400 lines
one responsibility per function
Switch*/Route*/Dispatch* functions only orchestrate/delegate
max nesting / complexity / parameters
```

## 16.2 Goal

Convert enforceable coding conventions into structured executable policies.

Example:

```json
{
  "schemaVersion": 1,
  "scope": "project",
  "rules": {
    "maxFunctionLines": 10,
    "maxFileLines": 400,
    "maxNestingDepth": 3,
    "maxCyclomaticComplexity": 6,
    "maxParameters": 5,
    "singleResponsibility": true,
    "orchestratorFunctions": {
      "patterns": ["Switch*", "Route*", "Dispatch*"],
      "allowBusinessLogic": false
    }
  }
}
```

## 16.3 Deterministic-first checks

Target: **90-95% of coding-policy checks use zero LLM tokens**.

Deterministic examples:

- function LOC;
- file LOC;
- nesting depth;
- parameter count;
- cyclomatic complexity;
- number/type of direct calls;
- forbidden DB/network calls from orchestrators;
- orchestrator branch/delegate/return shape;
- direct import/dependency restrictions.

## 16.4 Ambiguous semantic checks

`single responsibility` cannot be decided safely only from LOC.

Use local heuristics first:

```text
number of domains touched
number of external side-effect categories
mixed persistence/network/business-calculation behavior
branch/complexity score
call-graph spread
```

Only suspicious cases may trigger bounded semantic review on the changed symbol, never a full-repo LLM review by default.

## 16.5 Violation packets

Return compact structured evidence to the agent:

```json
{
  "rule": "max_function_lines",
  "path": "internal/payment/service.go",
  "symbol": "ProcessPayment",
  "actual": 27,
  "allowed": 10
}
```

This avoids repeatedly injecting large natural-language rule documents into model context.

## 16.6 Relationship to Project Brain rules

```text
Knowledge: what is true / what was decided
Rules: what the agent is expected to follow
Quality Policies: deterministic proof that produced code follows enforceable rules
```

Organization policy outranks project/repository/user policy according to the existing authority hierarchy. Quality policy can constrain code generation but can never grant execution/security permissions.

---

# 17. Token and latency budgets

The system MUST be optimized for the user-facing path, not only backend correctness.

Hard targets for the first production rollout:

```text
automatic durable write blocking latency: approximately 0
operational event embeddings: 0
operational graph nodes: 0
knowledge context token reduction: >= 50% vs polluted baseline
duplicate canonical knowledge: < 2%
operational noise in normal recall: 0%
cross-tenant leakage: 0
cross-project/repo/branch incompatible recall: 0
recall p95 regression from health/policy changes: <= 30 ms target
semantic quality-policy LLM calls: <= 5-10% of policy checks
```

Performance measurements:

```text
context_build_latency_p50/p95
db_recall_latency_p50/p95
vector_recall_latency_p50/p95
graph_boost_latency_p50/p95
tokens_per_context
knowledge_objects_per_completed_task
embedding_calls_per_task
quality_policy_checks_per_task
semantic_policy_review_rate
```

A regression beyond agreed thresholds blocks rollout.

---

# 18. Dashboard / UX

Separate concepts in UI:

```text
Knowledge
Entities
Relationships
Decisions
Problems
Milestones
Experiences
Skills
Health
Activity
Collective Suggestions (when enabled)
Quality Policies
```

`Activity` is not labeled `Memory`.

Example Brain Health card:

```text
Brain Health              94/100
Duplicate ratio            0.7%
Operational noise          0
Active conflicts           2
Quarantined               37
Avg recall tokens         620
Knowledge created today     8
```

Quality Policy card:

```text
Policy Health              PASS
Changed functions checked  14
Deterministic violations    2
Semantic reviews            0
```

Collective suggestion UX should be explainable:

```text
Suggested pattern: Transactional Outbox
Reason: strong match with current architecture and verified aggregate outcomes
Actions: Apply / Explain / Dismiss
```

Never expose another tenant's project identity as justification.

---

# 19. Feature flags and rollback

Minimum flags:

```text
CODELOCAL_KNOWLEDGE_V2
CODELOCAL_KNOWLEDGE_V2_WRITE
CODELOCAL_KNOWLEDGE_V2_READ
CODELOCAL_KNOWLEDGE_HEALTH
CODELOCAL_KNOWLEDGE_AUTO_PROMOTION
CODELOCAL_CODE_QUALITY_POLICY
CODELOCAL_COLLECTIVE_INTELLIGENCE
CODELOCAL_COLLECTIVE_CONTRIBUTION
```

Rollout rules:

- migrations are additive;
- reads can fall back to legacy during transition;
- auto-promotion can be disabled independently;
- collective contribution and collective serving are separate switches;
- quality policy can begin report-only before enforcement;
- kill switch must not require DB downgrade.

---

# 20. Implementation module boundaries

Proposed new/updated Go package structure:

```text
internal/knowledgebase/
  model.go
  identity.go
  validator.go
  promotion.go
  consolidator.go
  recall.go
  health.go
  quarantine.go

internal/codequality/
  policy.go
  resolver.go
  analyzer.go
  metrics.go
  orchestrator.go
  semantic_review.go

internal/collective/
  eligibility.go
  deidentify.go
  fingerprint.go
  aggregate.go
  ranking.go
  health.go

internal/cloud/
  knowledge_v2_store.go
  knowledge_v2_migration.go
  collective_store.go

internal/runtime/
  knowledge_workers.go
  health_workers.go
  collective_workers.go

internal/mcpgateway/
  task_memory.go        # remove operational durable ingestion, adapters only
  context/recall path  # consume V2 packet without adding public tools
```

Existing `internal/memory` remains as legacy compatibility during migration and should shrink over time rather than become the home for every new subsystem.

---

# 21. Phased execution plan

## Release trains

The implementation is deliberately split so the confirmed pollution bug can ship without waiting for the full intelligence program.

### CodeLocal 1.5.16 — Memory Safety Hotfix

Required scope only:

```text
K0 baseline fixture
K1 Stop Pollution
minimal legacy recall guard/quarantine for known operational noise
targeted token/latency safety smoke
```

No Knowledge V2 schema migration, Code Quality enforcement, or Collective Intelligence is required to unblock `1.5.16`.

### CodeLocal 1.5.17+ — Knowledge V2 Core

Primary scope:

```text
K2 -> K7
```

This train introduces canonical objects/revisions/provenance, durable outbox workers, stable identity/cardinality, cross-device/project identity hardening, bounded retrieval, health, and safe legacy migration.

### Later independent trains

```text
Code Quality Policy: K8 + its own benchmark/enforcement rollout
Collective Intelligence: CI0 -> CI4 after private Knowledge V2 is healthy
```

## K0 — Baseline and freeze

Deliverables:

- record current memory/graph/recall baseline;
- confirm release freeze;
- create fixtures containing known polluted memories;
- document rollback flags.

Gate K0:

- repeatable baseline report exists;
- no destructive migration executed.

## K1 — Stop Pollution

Deliverables:

- remove automatic durable ingestion for edit/verify/progress/error operational events;
- preserve taskstate/audit/history;
- preserve verified Experience separately;
- add regression tests.

Gate K1:

```text
100 edit events -> 0 durable knowledge
100 verify-progress events -> 0 durable knowledge
operational event -> 0 embedding -> 0 graph node
```

## K2 — Knowledge V2 schema + migration v26

Deliverables:

- canonical Go model;
- additive v26 tables/indexes;
- tenant/project/repository/branch constraints;
- revision/provenance model;
- feature flags.

Gate K2:

- fresh DB migration passes;
- historical fixture migration passes;
- tenant-scope tests pass;
- rollback flags work.

## K3 — Stable identity + revisions + temporal validity

Deliverables:

- stable-key resolver with `scalar|set` cardinality and qualifiers/entity keys;
- canonical upsert/revision semantics;
- repository alias/history and project-marker validation for cross-device continuity;
- portable `.codelocal/project.json` / rules / skills with runtime-only state excluded;
- lifecycle/conflict handling;
- explicit memory compatibility adapter.

Gate K3:

```text
same stable fact written 100 times -> 1 canonical object
mutable value change -> revision, not sibling duplicate
machine A + machine B -> same project identity when evidence matches
```

## K4 — Promotion / consolidation / durable async write

Deliverables:

- typed ingestion DAG;
- deterministic operational classifier;
- Experience-first promotion whitelist;
- transactional durable outbox;
- idempotent at-least-once workers with retry/backoff/dead-letter visibility;
- semantic consolidation only for ambiguity.

Gate K4:

- automatic semantic work does not block tool response on embedding/graph/consolidation;
- accepted durable jobs survive process restart and retry without duplicate canonical objects;
- candidate/promotion/outbox metrics available;
- malformed/unsafe candidate fails closed.

## K5 — Local Code Graph separation + Retrieval Router

Deliverables:

- explicit code-vs-knowledge graph boundary;
- Git/hash incremental code graph lifecycle;
- retrieval intent/router;
- bounded exact/vector/graph fusion and diversity.

Gate K5:

- code query does not require broad durable-memory recall;
- rebuilding local code index does not affect canonical knowledge;
- retrieval context budget is deterministic.

## K6 — Knowledge Health Monitor

Deliverables:

- health metrics;
- anomaly detection;
- quarantine/suppression;
- health score;
- background audit worker;
- release/canary gate hooks.

Gate K6:

- injected duplicate/noise spike is detected automatically;
- auto-promotion stops at unsafe threshold;
- canonical user facts are not auto-deleted.

## K7 — Legacy cleanup and dual read

Deliverables:

- dry-run classifier/report;
- quarantine known noise;
- migrate legitimate knowledge;
- V2-primary + legacy fallback;
- old->new mapping.

Gate K7:

- known noise is absent from recall;
- valid legacy decision/milestone fixtures survive;
- rollback to legacy read remains possible.

## K8 — Code Quality Policy Engine

Deliverables:

- structured policy model;
- deterministic analyzer for LOC/nesting/complexity/params;
- orchestrator-only checks;
- compact violation packet;
- report-only mode;
- optional bounded semantic reviewer for responsibility ambiguity.

Gate K8:

```text
max function/file policies enforced in fixtures
orchestrator DB/network/business logic violation detected
>=90% policy checks deterministic in benchmark fixture
no extra MCP tools
```

## K9 — Performance/token benchmark

Deliverables:

- before/after context benchmark;
- recall p50/p95;
- token accounting;
- DB/embedding/graph call counts;
- noise/duplicate metrics.

Gate K9:

- mandatory performance budgets in section 17 pass.

### RELEASE GO/NO-GO

`1.5.16` may proceed to canary after the **Memory Safety Hotfix gate** passes: K1, deterministic legacy operational-noise recall suppression/quarantine, targeted regression tests, and the safety/token smoke budget. K2-K9 are not blanket blockers for this hotfix.

`1.5.17+` Knowledge V2 Core proceeds only after its applicable K2-K7 gates and benchmark budgets pass. Code Quality and Collective Intelligence remain independent rollout tracks.

## CI0 — Privacy/consent model

- contribution policy;
- opt-in UX;
- enterprise defaults;
- privacy classifications;
- delete/withdraw design.

## CI1 — Eligibility + de-identification

- generic structured payload only;
- identifiers/code/secrets/path stripping;
- tests against cross-tenant leakage.

## CI2 — Pattern fingerprint + cohort aggregation

- stable pattern identity;
- independent contributor limiting;
- minimum cohort threshold;
- confidence/outcome stats.

## CI3 — Collective Pattern Store + Recommendation Router

- separate storage domain;
- contextual ranking;
- explanation without contributor identity.

## CI4 — Outcome feedback + Collective Health

- apply/verify outcome recording;
- manipulation/anomaly detection;
- freshness/security suppression.

## CI5 — Advanced privacy research

Only after scale and measurement justify it:

- differential privacy;
- secure aggregation;
- federated learning/ranking.

---

# 22. Test matrix

Mandatory integration scenarios:

```text
A. 100 edits + 100 verifies + 100 terminal events
   -> 0 canonical knowledge from operational activity

B. same project fact across reconnects/devices
   -> one canonical object

C. mutable fact changes value
   -> revision history, one active current fact

D. branch-A-only fact
   -> not recalled into incompatible branch B

E. repository-A convention
   -> not leaked into repository B

F. 10,000 legacy operational-noise rows
   -> 0 normal recall results and 0 active semantic graph nodes after quarantine

G. health duplicate/promotion spike
   -> degraded/unhealthy state and automatic promotion suppression

H. function 27 lines with max=10
   -> deterministic violation

I. file 487 lines with max=400
   -> deterministic violation

J. SwitchPayment directly writes DB when orchestrator policy forbids it
   -> deterministic/structural violation

K. private Experience containing repo/company/path identifiers
   -> collective de-identification prevents identifiable publication

L. collective pattern below cohort threshold
   -> not served globally
```

Security tests:

- every private query tenant scoped;
- collective tables cannot be used to fetch raw contributor knowledge;
- organization/project rules cannot grant execution permission;
- knowledge/quality subsystem cannot bypass approval policy;
- feature flags fail closed for mutation.

---

# 23. Management KPIs

Success should be visible in product and infrastructure metrics.

User-facing:

- reduced repeated project explanation;
- lower tokens per context;
- faster subsequent repository tasks;
- fewer convention regressions;
- useful recommendations with explainable reasons;
- no need for manual memory cleanup.

System:

```text
duplicate durable knowledge < 2%
operational noise in recall = 0%
operational knowledge-graph nodes = 0%
cross-tenant leak = 0
context token reduction >= 50%
auto-write hot-path blocking ~= 0
policy deterministic check rate >= 90%
knowledge-health anomaly detection before user-visible pollution
```

Collective Intelligence (later rollout):

```text
recommendation acceptance
verified-success after recommendation
rollback/failure rate
contributor diversity
privacy/rare-pattern violations = 0
```

---

# 24. Main risks and mitigations

## Risk: overengineering before stopping the current bug

Mitigation: K1 Stop Pollution ships first and is independently testable.

## Risk: V2 migration corrupts legitimate knowledge

Mitigation: additive schema, dual read, quarantine before deletion, old->new mapping, staging historical migration test.

## Risk: health monitor adds latency

Mitigation: health computation background; recall consumes cached health state only.

## Risk: semantic consolidation increases token cost

Mitigation: deterministic-first classification; batch async semantic work; LLM only on ambiguity.

## Risk: Code Quality becomes overly rigid

Mitigation: policy scope/authority, report-only rollout, project-configurable thresholds, semantic rules do not become hard blockers without sufficient confidence/policy.

## Risk: collective learning leaks private information

Mitigation: opt-in, eligibility allowlist, structural de-identification, cohort thresholds, separate storage domain, no raw provenance in serving, leakage fixtures, enterprise cross-org OFF by default.

## Risk: popularity reinforces bad engineering patterns

Mitigation: rank verified outcomes, context compatibility, failure/rollback rate, freshness, security and diversity—not raw popularity alone.

---

# 25. Decisions requested from management

Approval is requested for the following product/architecture decisions:

1. Approve the invariant **Operational Event != Durable Knowledge**.
2. Approve PostgreSQL + typed columns + JSONB as canonical Knowledge V2 storage.
3. Approve stable semantic identity + revisions + provenance + temporal validity.
4. Approve local rebuildable Code Graph and cloud durable Knowledge Graph as separate planes.
5. Approve Knowledge Health Monitor as a release-blocking/self-healing subsystem.
6. Approve Experience-first automatic learning plus a transactional durable outbox for any later promotion/consolidation/embedding/graph work outside the response hot path.
7. Approve Code Quality Policy Engine with deterministic-first enforcement and bounded semantic fallback on its own rollout train.
8. Approve Collective Intelligence as opt-in, de-identified, aggregate verified pattern learning; no raw cross-tenant knowledge sharing, on its own rollout train.
9. Approve keeping the public MCP surface fixed at 19 tools while these systems remain internal.
10. Approve narrowing `1.5.16` to the Memory Safety Hotfix gate and moving Knowledge V2 Core to `1.5.17+` rather than blocking the hotfix on K1-K9.

---

# 26. Recommended approval scope

Recommended immediate approval/shipping scope:

```text
1.5.16 Memory Safety Hotfix
= K0 baseline + K1 Stop Pollution + minimal legacy recall guard/quarantine + safety/token smoke
```

Recommended next core architecture scope:

```text
1.5.17+ Knowledge V2 Core
= K2 -> K7
```

Recommended independent follow-up tracks:

```text
Code Quality Policy = K8 + dedicated enforcement benchmark
Collective Intelligence = CI0 -> CI4
```

This ordering fixes the current production defect first, then introduces canonical durable knowledge and health without coupling release timing to coding-style enforcement or cross-user learning. Collective Intelligence begins only after Knowledge V2/Health supplies clean, verified, privacy-classified source data; otherwise current noise would be amplified globally.

---

# 27. Final target state

After this program:

```text
CodeLocal does not remember everything.
CodeLocal remembers what is durable.

CodeLocal does not ask AI to inspect what machines can prove.
CodeLocal uses deterministic analyzers first.

CodeLocal does not require users to watch memory quality manually.
CodeLocal measures and protects its own Brain.

CodeLocal does not copy one user's project knowledge to another.
CodeLocal learns privacy-safe aggregate engineering patterns.

CodeLocal does not only tell agents which coding conventions exist.
CodeLocal can prove whether the generated code satisfies enforceable conventions.
```

This is the intended intelligence flywheel:

```text
clean private knowledge
 -> better context
 -> better execution
 -> verified Experience
 -> safer reusable Skills
 -> privacy-safe collective patterns
 -> better suggestions
 -> better verified outcomes
 -> healthier Project Brain
```
