# CodeLocal Project Brain — Master Architecture & Execution Plan

Status: Master blueprint / source of truth for the next intelligence layer
Date: 2026-08-15
Owner: CodeLocal
Primary implementation target: native Go runtime + CodeLocal Cloud

> This document exists so the architectural intent is not lost across chats, machines, branches, or future refactors. When implementation details conflict with this document, either update this document with a dated decision or explicitly document why the implementation diverged.
>
> The provider-neutral multi-agent execution layer is specified separately in [`UNIVERSAL_AGENT_RUNTIME_PLAN.md`](./UNIVERSAL_AGENT_RUNTIME_PLAN.md). Project Brain owns durable intelligence; Universal Agent Runtime owns execution/routing across Codex, Claude, Cosine and future coding engines.
>
> The reusable design-intelligence layer is specified separately in [`DESIGN_RECIPE_LIBRARY_MASTER_PLAN.md`](./DESIGN_RECIPE_LIBRARY_MASTER_PLAN.md). Design Recipes preserve legally usable Composition + Style + Assets + Motion/3D specifications; Learned Skills preserve verified adaptation procedures; Project Brain owns durable metadata, provenance, and reusable verified knowledge without treating unlicensed third-party raw content as canonical cloud knowledge.

---

# 0. Product thesis

CodeLocal should not compete with Codex, Claude Code, Cursor, Copilot, Windsurf, or another coding agent by trying to own the best foundation model.

CodeLocal should become the **persistent project intelligence layer that survives the model, editor, machine, and conversation**.

The long-term product idea is:

```text
AI model/editor = replaceable reasoning surface
CodeLocal       = durable project brain + controlled execution layer
```

The user may work today with ChatGPT, tomorrow with Claude, later with another coding model. The user's accumulated project knowledge, coding rules, team conventions, learned workflows, decisions, experiences, and relationships should remain available through CodeLocal.

The competitive target is not merely:

```text
"Can CodeLocal edit code as well as Codex CLI?"
```

It is:

```text
"Does the coding system get materially better the longer the user and AI work together?"
```

CodeLocal should eventually know:

- what this project is;
- which repositories and modules belong to it;
- how the company expects code to be written;
- which project/directory rules apply to the current task;
- what architectural decisions were made and why;
- what the user prefers;
- what has failed before;
- what workflows repeatedly succeeded;
- which learned skill is safe to reuse now;
- which knowledge is stale, superseded, conflicting, or uncertain;
- what minimum context the model needs for the current task.

The core loop becomes:

```text
Observe -> Understand -> Resolve -> Compile Context -> Act -> Verify -> Learn
```

---

# 1. Non-negotiable principles

## 1.1 Server-first durable knowledge

The durable memory of the relationship between user, AI, and project belongs in CodeLocal Cloud storage.

Changing machine, reinstalling CodeLocal, cloning the project elsewhere, or switching AI products must not erase useful project knowledge.

The local runtime is primarily:

- observer;
- execution engine;
- code intelligence provider;
- local security boundary;
- local cache;
- provider of fresh project state.

The Cloud is primarily:

- durable knowledge source of truth;
- project/repository identity graph;
- knowledge revisions/provenance;
- cross-device synchronization point;
- memory/vector/graph retrieval layer;
- rules/skills metadata and portable definitions;
- long-term experience store.

## 1.2 Source code and secrets are not the product database

Server-first knowledge does **not** mean uploading the user's repository wholesale.

Cloud may store sanitized/canonical knowledge extracted from project sources, such as:

```text
"payment writes require a transaction"
"project uses pnpm"
"production deploys from release branch"
"controllers remain thin"
```

Cloud should not need raw application source code to retain these facts.

Never treat regex redaction alone as a complete security boundary.

Secrets, credentials, approval tokens, private keys, auth cookies, raw sensitive command output, and machine-specific secure bindings remain local.

## 1.3 Deterministic before probabilistic

If CodeLocal can determine something exactly using project identity, Git, path scope, metadata, parser rules, or policy, do not ask an LLM.

Preferred intelligence pyramid:

```text
~90% deterministic metadata/parsing/policy
 ~9% lexical/vector/heuristic ranking
 ~1% LLM interpretation for ambiguity
```

LLM reasoning should be used for ambiguous extraction, conflict explanation, experience consolidation, or genuinely semantic decisions — not basic file discovery.

## 1.4 Rules are not skills, skills are not memories

Keep semantics separate:

- **Rule**: what must or should be followed.
- **Skill**: how to perform a recurring task.
- **Memory**: a durable fact or decision.
- **Experience**: something that happened during real work.
- **Preference**: user/team preference that influences choices.
- **Source**: where knowledge came from.
- **Knowledge Graph**: relationships among these entities.

Do not flatten all of them into generic prompt text.

## 1.5 Security policy outranks learned behavior

A learned skill can never override CodeLocal security policy, organization policy, or a stronger applicable rule.

If a skill was learned under an old project state, it must be revalidated when its context fingerprint changes.

## 1.6 Knowledge must be correctable

CodeLocal must support:

```text
ACTIVE
STALE
CONFLICTED
SUPERSEDED
REVOKED
```

A memory existing in the database does not automatically make it current truth.

History should be preserved while normal recall uses only currently valid knowledge.

## 1.7 Retrieval must be bounded

More memory must not mean larger prompts forever.

CodeLocal must compile a tiny task-specific packet rather than dumping the database into the model.

The system is successful when long-lived projects require **less repeated context**, not more.

## 1.8 One canonical durable truth

Canonical durable knowledge is represented by the PostgreSQL knowledge object + revision + provenance model.

The following are derived/rebuildable indexes, not independent sources of truth:

- embeddings/vector index;
- semantic Knowledge Graph nodes/edges;
- compiled context caches;
- local Code Graph/indexes.

If a derived index is corrupt or stale, CodeLocal must be able to delete and rebuild it from canonical state without losing durable knowledge.

## 1.9 Experience-first automatic learning

Operational activity such as edits, verification progress, terminal progress, retries, and transient failures belongs to task/audit/history storage.

A verified task may automatically create a verified **Experience**. It must not directly create a canonical fact, decision, memory, or Skill merely because an operation succeeded. Promotion from Experience to durable knowledge/Skill passes through explicit deterministic/semantic promotion rules.

## 1.10 Durable asynchronous learning boundary

Work that is acknowledged as durable but executes outside the interactive response path must cross a transactional durable outbox/queue boundary and be processed by idempotent at-least-once workers.

A naked in-process goroutine is acceptable for best-effort telemetry, but is not a durability mechanism for canonical knowledge, Experience promotion, embeddings, or graph projections.

---

# 2. Current foundations to preserve

The current repository already contains useful foundations. Do not rebuild them unnecessarily.

Primary components:

```text
internal/projectidentity/       logical project/repository identity
internal/memory/                durable memory, ranking, graph foundations
internal/learnedskills/         learned recipe store and confidence lifecycle
internal/cloud/                 server-side project/knowledge persistence
internal/cloudserver/           dashboard + Knowledge Graph UI
internal/localclient/           native project/tool runtime
internal/project/               project context/code intelligence
internal/mcpgateway/            context + bounded agent orchestration
internal/security/              local policy enforcement
internal/taskstate/             hot task state
```

Legacy TypeScript code such as `src/workspace-index.ts` can provide useful prior concepts, but new production-critical Project Brain behavior should live in the current native Go architecture unless a deliberate migration decision says otherwise.

Existing strengths already implemented or partially implemented:

- logical project identity using repository sets;
- multi-repository discovery;
- project/workspace memory scopes;
- PostgreSQL memory + pgvector/FTS path;
- Memory Graph tables/relationships;
- Knowledge Graph dashboard;
- user isolation by `user_id`;
- learned-skill matching, recording, feedback, trust confidence;
- scoped `AGENTS.md` reading;
- semantic-first code context retrieval;
- local security/approval boundaries;
- compact public MCP tool surface.

This plan extends these systems instead of replacing them.

---

# 3. Target architecture

```text
                          USER / AI REQUEST
                                |
                                v
                        Task Classification
                                |
                                v
+----------------------+  PROJECT BRAIN  +----------------------+
|                      |                 |                      |
| Project Identity     |                 | Current Project State|
| User Identity        |                 | Git/branch/files     |
| Repository/Module    |                 | dependencies/tests   |
|                      |                 |                      |
+-----------+----------+                 +----------+-----------+
            |                                       |
            +-------------------+-------------------+
                                |
                                v
                     Knowledge Resolver
                                |
          +---------------------+---------------------+
          |                     |                     |
          v                     v                     v
     Effective Rules      Relevant Memory      Eligible Skills
          |                     |                     |
          +---------------------+---------------------+
                                |
                                v
                        Context Compiler
                                |
                                v
                      ChatGPT / Claude / AI
                                |
                                v
                       CodeLocal Execution
                                |
                                v
                          Verification
                                |
                                v
                           Experience
                                |
                                v
                    Knowledge Reinforcement
```

Supporting ingestion path:

```text
Git + filesystem watcher + AI editor configs + conversations + tool outcomes
                                |
                                v
                    Project Knowledge Discovery
                                |
                                v
                       Canonical Knowledge Model
                                |
                                v
                    Hash / Revision / Provenance
                                |
                                v
                         CodeLocal Cloud DB
```

---

# 4. Canonical Knowledge Model

All editor/provider-specific formats should normalize into one internal model.

## 4.1 Knowledge Source

A source is evidence, not yet necessarily a rule or memory.

Examples:

- `AGENTS.md`;
- `CLAUDE.md`;
- `.cursor/rules/backend.mdc`;
- `.github/copilot-instructions.md`;
- `.github/instructions/*.instructions.md`;
- `.codelocal/rules/*.md`;
- `.codelocal/skills/*/SKILL.md`;
- Git history;
- package manifests;
- deployment config;
- confirmed user statement;
- verified task outcome.

Suggested model:

```text
knowledge_source
  id
  user_id
  project_id
  repository_id nullable
  module_id nullable
  provider
  source_type
  canonical_path nullable
  content_hash nullable
  semantic_hash nullable
  manifest_hash nullable
  git_commit nullable
  git_blob_oid nullable
  branch nullable
  adapter_version
  parser_version
  first_seen_at
  last_seen_at
  valid_from
  valid_to nullable
  active
  metadata jsonb
```

## 4.2 Canonical Rule

```text
rule
  id
  user_id
  project_id nullable
  repository_id nullable
  module_id nullable
  scope
  topic
  normalized_statement
  strength required|preferred|advisory
  authority
  apply_to []
  languages []
  frameworks []
  task_kinds []
  branch_constraint nullable
  confidence
  valid_from
  valid_to nullable
  status active|stale|conflicted|superseded|revoked
```

A rule may have multiple source links.

Do not duplicate the same logical rule simply because Claude, Cursor, and AGENTS all contain it.

## 4.3 Memory / Knowledge Claim

A claim is a fact believed useful for later tasks.

Examples:

```text
"The CodeLocal production dashboard is implemented in Go."
"BIDDI consists of multiple repositories."
"This project deploys to Railway from dev."
```

Suggested fields:

```text
claim
  id
  kind
  stable_key
  cardinality scalar|set
  qualifier nullable
  normalized_statement
  scope global|project|repository|module
  confidence
  importance
  authority
  confirmation_count
  successful_recall_count
  failed_recall_count
  valid_from
  valid_to nullable
  status
```

For `scalar` claims, a changed value creates a new revision of the same canonical identity. For `set` claims, simultaneous independent members require distinct qualifiers/entity keys so unrelated constraints, people, dependencies, or relationships are not merged merely because they share a predicate.

## 4.4 Experience

Experience captures what actually happened during work.

```text
experience
  id
  user_id
  project_id
  repository_id nullable
  workspace_id nullable
  device_id nullable
  task_kind
  intent
  branch
  start_revision
  end_revision
  outcome success|partial|failed|cancelled
  summary
  root_cause nullable
  files []
  checks []
  skill_id nullable
  started_at
  completed_at
```

Experience is the raw material for future memories and skills.

## 4.5 Skill

Cloud should become the portable source of truth for **sanitized abstract skill definitions and metadata**.

Device-local state keeps sensitive bindings and approval state.

A cloud skill must never contain credentials or approval tokens.

Suggested model:

```text
skill
  id
  user_id
  project_id nullable
  repository_id nullable
  module_id nullable
  name
  intent
  task_kind
  version
  status candidate|trusted|stale|revoked
  confidence
  success_count
  failure_count
  abstract_steps jsonb
  preconditions jsonb
  context_fingerprint jsonb
  created_at
  updated_at
  last_used_at
```

Local bindings may resolve abstract operations such as:

```text
"run project test command"
"deploy configured Railway service"
```

to machine-specific paths/processes without persisting secrets to Cloud.

## 4.6 Relationships

Important graph relations include:

```text
OWNS
WORKS_ON
CONTAINS_REPO
CONTAINS_MODULE
HAS_RULE
HAS_SKILL
HAS_MEMORY
HAS_EXPERIENCE
DERIVED_FROM
SUPPORTED_BY
CONFLICTS_WITH
SUPERSEDES
APPLIES_TO
DEPENDS_ON
IMPLEMENTED_IN
FAILED_WITH
SUCCEEDED_WITH
OBSERVED_ON
MIGRATED_FROM
ALIAS_OF
```

---

# 5. Project and repository identity

Project continuity across devices depends on identity quality.

## 5.1 Repository identity

Current remote/lineage discovery remains useful.

Repository identity should support aliases because real repositories move:

```text
GitHub org rename
GitHub -> GitLab migration
remote URL change
fork becomes canonical repo
local-only repo later gains a remote
```

Add a repository alias/history layer instead of creating permanently disconnected knowledge islands.

## 5.2 Logical Project identity

A project may contain:

- one Git repository;
- many Git repositories;
- monorepo packages;
- local folders without Git;
- Git submodules;
- multiple checkouts/worktrees on different devices.

`.codelocal/project.json` should become an optional **strong project-identity marker** when present, but it is not trusted blindly. CodeLocal must validate the marker against the authenticated tenant plus repository/alias evidence so copied templates or cloned starter folders cannot silently merge unrelated projects.

Recommended committed marker:

```json
{
  "schemaVersion": 1,
  "projectId": "stable-project-id",
  "name": "BIDDI"
}
```

Marker schema versions are explicit. A marker with an unsupported/missing schema version or unsafe payload is rejected locally and revalidated by Cloud. A valid marker can create a new logical project only when the incoming repository set is not already bound elsewhere; once the project exists, marker continuation requires compatible direct repository or project-local lineage evidence. Competing repository evidence always wins over the marker and forces a fail-closed repository-set fallback.

Portable project configuration is intentionally commit-able:

```text
.codelocal/project.json
.codelocal/quality.json
.codelocal/rules/**
.codelocal/skills/**
```

Runtime-only state such as `.codelocal/worktrees/**` remains ignored and must never enter project indexing/context.

When no marker exists, CodeLocal derives project identity from a stable repository set and records repository aliases/history as evidence changes. Remote renames/migrations must not permanently split one logical repository into disconnected knowledge islands.

## 5.3 Module / Project Area identity

Repository scope is too coarse for monorepos.

Introduce module/project-area identity:

```text
project
  -> repository
       -> module/path scope
```

Examples:

```text
apps/web
apps/api
packages/payment
packages/shared-ui
```

Rules, skills, memories, and dependencies may target a module.

---

# 6. Project Knowledge Discovery

This subsystem discovers AI/editor/project conventions without repeatedly scanning the whole repository.

## 6.1 Adapter registry

Create a versioned adapter registry.

Initial providers should include at least:

```text
generic
  AGENTS.md

claude
  CLAUDE.md
  CLAUDE.local.md
  .claude/**

cursor
  .cursor/rules/**

copilot
  .github/copilot-instructions.md
  .github/instructions/**/*.instructions.md

codelocal
  .codelocal/**
```

Future adapters can cover Windsurf, Cline/Roo, Continue, Aider conventions, and other ecosystems without changing the canonical model.

Provider format changes must be represented by `adapter_version` and `parser_version`.

## 6.2 Discovery must reuse existing indexing/watching

Do not create a second recursive repository scanner.

The project/file index already observes repository structure. Project Knowledge Discovery subscribes to matching paths/events.

Fast paths:

- Git tracked filename list;
- known root configuration files;
- current workspace index;
- filesystem watcher events.

## 6.3 Nested instruction files

Instruction applicability may be hierarchical.

Example:

```text
AGENTS.md
backend/AGENTS.md
backend/payment/AGENTS.md
```

Editing `backend/payment/service.go` should resolve all applicable ancestors in precedence order, not load every instruction file in the project.

---

# 7. Incremental fingerprinting and hashing

Hashing answers whether source state changed; it does not decide whether the knowledge is correct or applicable.

## 7.1 Three-level change detection

### Level A — metadata fast gate

```text
size
mtime
known Git state
```

If metadata and Git state are unchanged, avoid reading the file.

### Level B — exact source fingerprint

Use content hash or Git blob OID as truth.

For tracked clean files:

```text
Git blob OID = content identity
```

For modified/untracked files:

```text
SHA-256/BLAKE3(file content)
```

### Level C — semantic fingerprint

Normalize irrelevant formatting/frontmatter noise and calculate semantic hash.

If exact content changed but semantic hash did not:

```text
record source revision
skip expensive re-extraction/embedding when safe
```

Never replace exact content identity with semantic hash.

## 7.2 Parser fingerprint

A source may need reprocessing even when bytes did not change because CodeLocal's parser improved.

Effective extraction fingerprint:

```text
hash(
  content_hash
  + adapter_version
  + parser_version
  + semantic_normalizer_version
)
```

## 7.3 Manifest root hash

For all discovered AI/project knowledge sources:

```text
sort(canonical_path + source_hash + parser_fingerprint)
-> manifest_root_hash
```

If local and Cloud manifest root hashes match, stop immediately.

No parse, embedding, or upload is needed.

## 7.4 Event-driven updates

Normal operation:

```text
watcher event
-> mark one source dirty
-> debounce
-> hash one source
-> manifest delta
-> parse changed source only
-> sync delta
```

## 7.5 Reconciliation events

Filesystem watchers are a fast path, not the correctness source.

Perform lightweight reconciliation when:

- runtime starts;
- laptop wakes from sleep;
- watcher reports overflow;
- Git HEAD changes;
- branch checkout/reset occurs;
- reconnect follows a long offline period;
- adapter/parser version changes.

Avoid periodic full scans of the entire repository.

---

# 8. Content-addressed Cloud synchronization

Do not upload the same source repeatedly from multiple machines.

Protocol concept:

```text
Client -> Cloud
projectId
manifestRootHash
entries[path, sourceHash, parserFingerprint]

Cloud -> Client
missingHashes[]
changedMappings[]
conflicts[]
```

Only sources unknown to Cloud or needing a new parse revision are sent.

For identical committed config on Machine A and Machine B, Git blob identity should dedupe naturally.

---

# 9. Multi-device conflict handling

Never use silent last-write-wins for knowledge sources.

Each update includes:

```text
source_id
base_revision
base_hash
new_hash
device_id
project_id
branch/commit when available
```

Example:

```text
Cloud: A1
Machine A: A1 -> A2
Machine B: A1 -> B2
```

After A2 is accepted, B2 becomes an explicit conflict because its base is stale.

Resolution strategies:

1. Git-backed source: use commit ancestry when possible.
2. Different branches/worktrees: preserve branch-scoped revisions.
3. Local-only source: preserve both revisions; do not overwrite silently.
4. User-confirmed policy source: higher-authority revision may supersede lower-authority source.

---

# 10. Provenance and authority

Every meaningful knowledge item needs provenance.

Recommended authority order:

```text
CodeLocal security policy
  > explicit organization policy
  > explicit user confirmation
  > committed project rule
  > repository/module rule
  > verified execution outcome
  > repeated observation
  > imported editor preference
  > AI inference
```

Authority is separate from confidence.

A statement may be highly confidently extracted from a low-authority source but still lose a conflict against an organization rule.

Each claim/rule should be able to answer:

```text
Where did this come from?
When was it last confirmed?
Which revision introduced it?
What superseded it?
Why is CodeLocal applying it now?
```

---

# 11. Canonical deduplication and contradiction detection

## 11.1 Deduplication

Same logical rule appearing in multiple files should converge into one canonical rule with many supporting sources.

Example:

```text
AGENTS.md
CLAUDE.md
.cursor/rules/backend.mdc
```

all state:

```text
controllers contain no business logic
```

Result:

```text
one canonical rule
three provenance sources
stronger confidence
```

## 11.2 Conflict

If another source states the opposite, do not merge them.

Create:

```text
rule A --CONFLICTS_WITH--> rule B
```

Resolver decides effective behavior using authority, scope, specificity, validity, and explicit policy.

Unresolved high-authority conflicts should be visible in UI and may require user confirmation.

---

# 12. Rule precedence and applicability

Rules must resolve deterministically before being sent to the AI.

Default precedence:

```text
1. CodeLocal security/runtime policy
2. Organization/company mandatory rules
3. Project mandatory rules
4. Repository mandatory rules
5. Module/directory mandatory rules
6. User preferences/local overrides
7. Learned skills
8. AI inference
```

Within the same authority tier, prefer:

```text
more specific scope
newer valid revision
explicit applyTo match
explicit user confirmation
higher confidence
```

Applicability signals:

```text
project identity
repository identity
module/path match
branch/commit validity
language
framework
task kind
dependency presence
source status
semantic relevance
recency
confidence
authority
```

Hard path-scoped rules do not require vector similarity to apply.

---

# 13. Knowledge Resolver

The Resolver is the heart of Project Brain.

Input:

```text
user
project
repositories
module/path
branch/commit
task intent
task kind
language/framework
dependency/project state
```

Output:

```text
effective_rules[]
relevant_memories[]
eligible_skills[]
active_decisions[]
conflicts[]
stale_items[]
explanations/provenance IDs
```

The Resolver should answer:

```text
What is true now?
What is mandatory now?
What is useful now?
What workflow is safe to reuse now?
```

It should not return the entire Knowledge Graph.

---

# 14. Context Compiler

The Context Compiler turns resolved knowledge into a minimal model packet.

Example output:

```text
PROJECT CONTEXT

Must follow:
- Controllers remain thin.
- Payment writes require transactions.
- External API calls require timeout handling.

Known project facts:
- Payment provider is Stripe.
- Relevant tests are under internal/payment.

Verified workflow available:
- create-payment-api@4 (confidence 0.94)
```

The model should not need to know whether those facts originated from Cursor, Claude, AGENTS, a previous conversation, or a verified execution unless provenance is relevant to the task.

## 14.1 Separate context lanes

### Mandatory lane

- security policy;
- applicable hard rules;
- active organization/project constraints.

Loaded deterministically.

### Contextual lane

- memories;
- preferences;
- prior decisions;
- experience summaries.

Loaded through bounded hybrid retrieval.

### Skill lane

- selected skill metadata/preconditions;
- abstract steps only when eligible.

### Code lane

- exact symbols/ranges from semantic retrieval.

## 14.2 Token budgets

Enforce per-lane budgets instead of one global unbounded prompt.

Suggested initial policy:

```text
mandatory rules: never silently drop applicable hard rules
user/project facts: small fixed budget
memory: top 3-8 compact claims
relationships: bounded 1-hop evidence
skills: zero unless candidate exists
code snippets: task-driven, symbol/range bounded
raw evidence: zero unless explicitly fetched
```

## 14.3 Cache compiled context

Cache a compiled packet only while its dependency fingerprint remains valid.

Invalidate on:

- rule manifest change;
- relevant file/symbol change;
- branch/project context change;
- selected skill fingerprint change;
- explicit user correction.

## 14.4 Compiler v3 deterministic compaction

Before budget selection, the compiler performs conservative exact-normalized rule compaction across providers/authority layers:

```text
lowercase
+ collapse whitespace
+ ignore trailing . ; :
```

It does **not** embedding-merge or semantically paraphrase rules. Opposite or merely similar rules stay separate. When exact-normalized duplicates exist, the highest-authority/specific representative is retained and `required` propagates if any duplicate is mandatory.

The packet reports:

```text
inputRules / candidateRules / duplicateRules
inputChars / deduplicatedChars / usedChars
deduplicationRatio
```

This prevents repeated AGENTS/Claude/Copilot copies of one mandatory rule from consuming the budget multiple times or causing a false `mandatoryOverflow`. The savings are exposed through existing context-budget/benchmark output; no extra MCP tool or database write is required.

---

# 15. Learned Skills v2

Current learned skills are useful but too workspace-local and too dependent on intent/confidence alone.

## 15.1 Cloud portability

Move sanitized abstract skill definitions and metadata to Cloud so the same logical project can reuse them across devices.

Portable skill identity describes the workflow, while project scope is stored separately. The same semantic workflow therefore keeps one `portableSkillId` when repository/project aliases are canonicalized, but it never escapes its authenticated tenant/project scope.

Portable v1 is intentionally narrow: only semantic browser/computer steps that can be stripped of ephemeral machine identifiers are eligible. Raw terminal/shell commands remain local until a separate command-template/local-binding model can prove they contain no secrets, absolute paths, host-specific executables, or unsafe arguments.

Local remains responsible for:

- device-specific paths;
- local executable bindings;
- approval state;
- secrets;
- machine-only credentials;
- ephemeral window/element IDs and coordinates;
- final replay trust on this machine.

Cloud portable definitions are revalidated on ingestion and again before serving. Cross-device imports enter local state as `imported`, not `candidate` or `trusted`. A successful verified execution on the receiving machine is required before the local recipe can become a normal candidate.

Multiple devices may contribute evidence for the same portable workflow. Aggregation is contributor-limited by independent device, not workspace count, and reports `single_source | collecting | corroborated | degraded | stale`. `corroborated` is recommendation evidence only; it never grants local replay permission or bypasses context fingerprint/security checks.

Contributor evidence decays without deleting provenance. By default, a contribution not refreshed for 180 days stops participating in corroboration (`CODELOCAL_PORTABLE_SKILL_EVIDENCE_MAX_AGE_DAYS`). The historical row remains available for audit. `stale` and `degraded` Cloud suggestions are not imported into a new active workspace; they must be refreshed by newer verified evidence first.

## 15.2 Context fingerprint

Every skill must record the project context under which it was verified.

Example:

```text
project_id
repository_ids
module_id
rules_manifest_hash
dependency_manifest_hash
important_file_hashes
branch policy
adapter/parser versions
required capabilities
```

## 15.3 Replay modes

```text
EXACT MATCH
  -> fast replay if safety policy allows

COMPATIBLE CHANGE
  -> lightweight revalidation / model confirmation

MATERIAL CHANGE
  -> do not replay; plan normally
```

## 15.4 Trust lifecycle

```text
candidate
-> repeated verified success
-> trusted
-> context changes / failures
-> stale
-> revalidated candidate/trusted
```

A trusted skill is not permanently trusted independent of project state.

## 15.5 Skill safety

Skill steps should use structured CodeLocal operations wherever possible.

Never persist approval tokens.

Never allow imported repository skills to grant themselves permissions.

Imported skills start untrusted regardless of source branding.

---

# 16. Experience Learning Engine

CodeLocal should learn primarily from verified experience, not model speculation.

After meaningful tasks:

```text
request
-> plan
-> actions
-> verification
-> outcome
-> experience record
```

Experience extraction should capture:

- intent;
- task kind;
- relevant files/symbols;
- rules used;
- chosen approach;
- failed approaches when useful;
- root cause;
- verification evidence summary;
- final outcome;
- whether a skill was used.

Repeated successful experiences may produce:

```text
experience -> memory claim
experience -> stronger relationship
experience -> candidate skill
```

Repeated failures weaken memories/skills or create warnings.

---

# 17. Memory lifecycle and reinforcement

Suggested lifecycle:

```text
OBSERVED
-> CONFIRMED
-> ACTIVE
-> STALE
-> SUPERSEDED
```

Strength increases when:

- user explicitly confirms;
- committed project source agrees;
- independent sources agree;
- repeated verified tasks use it successfully;
- a related trusted skill succeeds.

Strength decreases when:

- user corrects it;
- newer source contradicts it;
- retrieval leads to failed decisions;
- relevant project state changes;
- it remains stale and unused.

Do not physically delete history simply because it became stale; remove it from normal recall and preserve provenance.

---

# 18. Branch, worktree, and temporal correctness

Logical project identity should remain branch-independent for continuity, but knowledge applicability must not be branch-blind.

Every source-derived knowledge item should retain sufficient revision context:

```text
repository_id
git_commit
branch when useful
source_revision
valid_from
valid_to
```

Examples:

- `dev` deploy rule differs from `release`;
- two worktrees are active simultaneously;
- old architecture decision applies only before a migration commit.

Project-level durable facts may remain branch-independent when explicitly classified as such.

---

# 19. Generated/config churn handling

Some tool-generated files change timestamps, comments, ordering, or generated metadata without changing meaning.

Use exact content hash to retain source history, but semantic normalization to avoid unnecessary:

- embeddings;
- canonical re-extraction;
- graph churn;
- context invalidation when meaning did not change.

Never let semantic normalization hide a security-significant change.

---

# 20. Privacy and data classification

Classify every source before Cloud ingestion.

Suggested classes:

```text
PUBLIC_PROJECT
TEAM_PROJECT
PRIVATE_PROJECT
LOCAL_PRIVATE
SENSITIVE
SECRET
```

Behavior:

- project rules normally may sync after sanitization;
- `.local` configs default to more restrictive handling;
- secret-class content never syncs;
- local paths should be normalized/abstracted where possible;
- raw terminal output/screenshots remain local unless explicitly designed otherwise.

The Cloud should store extracted knowledge when safe rather than blindly storing raw local config text.

All reads/writes remain tenant-scoped by authenticated `user_id`; organization/team scopes must add explicit membership checks.

Never accept a frontend-supplied arbitrary user ID as authorization.

---

# 21. Knowledge Graph UX evolution

The dashboard should evolve from visualization into a control surface for Project Brain.

Target graph entities:

```text
User
Project
Repository
Module
Device
Workspace
Rule
Skill
Memory
Decision
Experience
Dependency
Artifact
```

Inspector should eventually show:

```text
Why this exists
Source(s)
Authority
Confidence
Current status
Applies to
Last confirmed
Related experiences
Superseded by
Conflicts with
```

User actions should eventually include:

- correct a fact;
- mark a rule authoritative;
- resolve conflict;
- forget/revoke knowledge;
- merge aliases;
- inspect source history;
- revalidate a skill.

Visualization must remain optional to agent execution.

---

# 22. AI/editor compatibility strategy

CodeLocal should become the interoperability layer rather than asking users to rewrite their existing AI setup.

Goal:

```text
existing Claude/Cursor/Copilot/etc config
           ↓
CodeLocal adapters
           ↓
canonical Project Brain
           ↓
any supported AI client
```

Users should be able to adopt CodeLocal without deleting their existing editor-specific rules.

## Import behavior

- discover known formats;
- preserve original provenance;
- normalize semantics;
- never mutate foreign config by default;
- do not execute imported hooks/skills automatically merely because they exist.

## Export behavior (later)

Optional future capability:

```text
canonical CodeLocal rules
-> generate editor-specific representation
```

This would make CodeLocal the source of truth while editors become views/adapters.

---

# 23. Smart YAGNI / reuse-first coding policy

CodeLocal should reduce unnecessary code generation using deterministic project intelligence before asking the model to invent a solution.

Before substantial new implementation, the agent/context planner should prefer checking:

```text
1. Is the requested feature actually needed for the stated goal?
2. Does equivalent code/symbol/component already exist?
3. Does the language standard library provide it?
4. Does the platform/native API provide it?
5. Is an already-installed dependency suitable?
6. Is there a simpler existing project pattern?
7. Only then create the smallest necessary code.
```

This is not a universal ban on abstraction. Architecture quality and maintainability still matter.

The policy should primarily prevent redundant invention, duplicate components, and unnecessary dependencies.

---

# 24. Failure and edge-case checklist

The implementation is incomplete until these cases are handled deliberately.

## Identity

- repository remote rename/migration;
- repo without remote;
- shallow clone without complete lineage;
- multiple repos with same display name;
- Git submodules;
- nested repos;
- monorepo modules;
- multiple worktrees;
- same project on two devices;
- project marker conflicts with derived identity.

## Source discovery

- nested AGENTS files;
- editor convention changes;
- unknown/new editor format;
- symlinks;
- generated config;
- ignored config that is still semantically relevant;
- huge config file;
- binary/malformed config;
- watcher missed event;
- atomic rename save behavior.

## Hashing

- same mtime/size but changed bytes;
- file changed then restored to same content;
- parser version changed while content hash stayed equal;
- semantic hash equal but security-significant metadata changed.

## Synchronization

- two devices edit same source offline;
- different branches edit same rule;
- source deleted on one device and modified on another;
- device reconnects after months with stale base revision;
- server receives duplicate updates after retry.

## Knowledge quality

- duplicate rule from several editors;
- contradictory rules;
- stale memory;
- user correction;
- AI inference conflicting with committed policy;
- imported rule is malicious prompt injection;
- source deleted but historical memory remains;
- project renamed/restructured.

## Skills

- skill references deleted file;
- dependency/version changed;
- branch policy changed;
- rule changed;
- skill worked on Machine A but requires unavailable capability on Machine B;
- skill failure after becoming trusted;
- imported skill tries to execute unsafe command.

## Retrieval

- hard rule omitted by semantic ranking;
- global preference incorrectly overrides project rule;
- stale branch memory outranks current branch fact;
- too much context destroys token savings;
- graph expansion pulls irrelevant neighboring projects;
- ambiguous entity alias joins unrelated things.

## Privacy

- secret embedded inside normal Markdown;
- credential-like value inside skill config;
- local override contains internal host/path;
- cross-user graph query;
- organization member loses access but cached context persists.

Every category requires explicit regression tests before stable rollout.

---

# 25. Implementation roadmap

## PB0 — Freeze invariants and benchmark baseline

Goal: know whether Project Brain actually improves the product.

Build baseline benchmark scenarios:

```text
new repository task
repeated task same thread
repeated task new thread
same project different device
old project resumed after days
monorepo module task
rule conflict task
branch/worktree task
skill replay task
```

Measure:

```text
input/context bytes per completed task
model round trips
MCP calls
repository files/ranges read
completion latency
first-pass verification success
regression rate
memory precision
rule application accuracy
skill hit rate
skill success rate
cross-device continuity success
```

Exit gate:

- repeatable benchmark harness exists;
- current CodeLocal baseline captured.

## PB1 — Canonical schema + provenance

Implement canonical sources/rules/claims/experiences/revisions.

Tasks:

- DB migrations;
- canonical IDs;
- provenance joins;
- validity/status fields;
- source revision model;
- tenant isolation tests;
- migration/backfill from existing memory graph where safe.

Exit gate:

- every canonical knowledge item can explain its source and validity.

## PB2 — Project Knowledge Discovery v1

Implement adapters for:

- AGENTS;
- Claude;
- Cursor;
- Copilot;
- CodeLocal native format.

Tasks:

- adapter registry;
- nested scope resolution;
- parser versions;
- source classification;
- no automatic execution of imported hooks.

Exit gate:

- representative fixtures normalize into one canonical schema.

## PB3 — Incremental manifest + content-addressed sync

Tasks:

- metadata fast gate;
- Git blob identity;
- exact content hash;
- semantic hash;
- extraction fingerprint;
- manifest root hash;
- watcher integration;
- reconciliation triggers;
- Cloud missing-hash handshake;
- idempotent source update API.

Exit gate:

- unchanged project startup performs near-zero source reprocessing;
- one changed rule reparses only that source.

## PB4 — Cross-device source revision/conflict engine

Tasks:

- base revision protocol;
- optimistic concurrency;
- branch/worktree revision awareness;
- Git ancestry resolution;
- conflict persistence;
- source tombstones;
- long-offline-device handling.

Exit gate:

- no silent knowledge loss under concurrent device updates.

## PB5 — Rule Resolver v1

Tasks:

- authority model;
- precedence engine;
- path/module applicability;
- branch validity;
- duplicate canonicalization;
- contradiction edges;
- effective-rule output;
- explanation/provenance IDs.

Exit gate:

- deterministic tests cover nested, cross-provider, and conflicting rules.

## PB6 — Context Compiler v1

Tasks:

- mandatory/contextual/skill/code lanes;
- hard token budgets;
- dedupe;
- stale/superseded filtering;
- progressive disclosure;
- compiled packet cache + invalidation;
- instrumentation.

Exit gate:

- mature/repeated tasks use less context than PB0 baseline without worse verification quality.

## PB7 — Learned Skills v2 / Cloud portability

Tasks:

- abstract cloud skill schema;
- migrate local metadata;
- skill context fingerprints;
- compatibility/revalidation states;
- local secret/capability bindings;
- cross-device skill availability;
- skill invalidation from rule/dependency changes;
- skill run history.

Exit gate:

- Machine B can safely reuse a verified project skill learned on Machine A when fingerprints/capabilities match.

## PB8 — Experience Learning Engine

Tasks:

- verified task outcome ingestion;
- experience summaries;
- claim reinforcement;
- failure weakening;
- candidate-skill generation;
- decision extraction;
- evidence references.

Exit gate:

- repeated verified work improves recall/skill selection automatically.

## PB9 — Memory lifecycle + correction

Tasks:

- observed/confirmed/active/stale/superseded states;
- authority-aware user correction;
- temporal contradiction handling;
- confidence reinforcement/decay;
- alias merge;
- safe forget/revoke paths.

Exit gate:

- the system can recover cleanly from an old wrong memory.

## PB10 — Knowledge Graph control plane

Tasks:

- Rule/Skill/Experience/Module nodes;
- source provenance inspector;
- conflicts view;
- stale/revalidation indicators;
- correct/forget/merge controls;
- project/account filters;
- large graph virtualization/clustering.

Exit gate:

- user can understand and correct why Project Brain believes something.

## PB11 — Organization/team knowledge

Only after single-user isolation is strong.

Tasks:

- organizations;
- membership/roles;
- organization rules;
- project sharing;
- private-user overlay;
- audit log;
- access revocation/cache invalidation.

Exit gate:

- company coding standards apply consistently without exposing private user memory.

## PB12 — Adapter ecosystem + import/export

Tasks:

- server-managed signed adapter manifests;
- more editor conventions;
- plugin SDK for parsers where safe;
- optional canonical->editor export;
- version compatibility testing.

Exit gate:

- CodeLocal can sit above multiple coding environments without forcing migration.

## PB13 — Universal Agent Runtime / coding-engine router

Detailed source of truth: [`UNIVERSAL_AGENT_RUNTIME_PLAN.md`](./UNIVERSAL_AGENT_RUNTIME_PLAN.md).

Goal:

```text
One CodeLocal interface
        ↓
Codex / Claude / Cosine / Gemini / future coding agents
        ↓
One Project Brain + one verification/security layer
```

Tasks:

- canonical Agent Adapter/session/event contracts;
- engine discovery, version and capability probing;
- provider-owned authentication handoff;
- structured integrations first, PTY fallback only when necessary;
- Project Brain Context Compiler handoff to every provider;
- provider-neutral process/session lifecycle;
- independent CodeLocal verification after provider execution;
- provider-neutral Experience recording;
- deterministic engine selection before learned routing;
- conservative routing learned from verified outcomes;
- cross-provider task handoff/resume;
- isolated worktrees for parallel mutating agents;
- explicit security classification when a provider subprocess cannot be fully mediated.

Exit gate:

- a user can perform supported coding workflows through `codelocal agent` without learning provider-specific invocation syntax;
- changing agent engines does not reset project rules, memory, decisions, experiences or skills;
- provider self-reported completion never bypasses CodeLocal verification;
- no supported agent can weaken CodeLocal's required local security/approval boundary.

---

# 26. Definition of "smarter than current coding agents"

Do not claim superiority based on a demo or model benchmark alone.

CodeLocal should earn the claim through measurable system behavior.

## Dimension A — Long-term project continuity

Test:

```text
work on Project A on Machine A
switch conversation/model
work on Machine B
resume weeks later
```

Target:

- project recognized correctly;
- active decisions/rules restored;
- prior successful workflows reusable;
- no user re-teaching required for known durable facts.

## Dimension B — Cross-editor portability

Existing Claude/Cursor/Copilot project knowledge should be consumable by CodeLocal and available to another supported AI client through the canonical brain.

## Dimension C — Repeated-task efficiency

On mature projects, repeated tasks should show lower:

- input context;
- file reads;
- model round trips;
- MCP calls;
- time-to-first-correct-change.

## Dimension D — Correct rule enforcement

Company/project/module rules must apply reliably even when semantic retrieval would not rank them highly.

## Dimension E — Safe learned execution

A verified skill should accelerate recurring work while being invalidated when its assumptions change.

## Dimension F — Explainable memory

The user can inspect:

```text
what CodeLocal knows
why it believes it
where it came from
whether it is current
how to correct it
```

## Dimension G — Model independence

The same Project Brain should improve ChatGPT, Claude, or a future compatible model without rebuilding project knowledge from scratch.

---

# 27. Competitive strategy versus Codex / Claude Code

CodeLocal should not attempt to beat foundation-model vendors at foundation-model training.

The moat should be the **external intelligence substrate around the model**.

Target advantages:

```text
1. Cross-model memory
2. Cross-editor rule/skill ingestion
3. Cross-device logical project continuity
4. Server-first durable project experience
5. Explainable provenance graph
6. Deterministic rule resolution
7. Context compilation/token reduction
8. Verified workflow learning
9. Local controlled execution
10. User-owned long-term project brain
```

Where Codex/Claude may have stronger model-native reasoning, CodeLocal should make any capable model operate with a better persistent environment.

The ambition is:

```text
strong model + CodeLocal Project Brain
>
same model without durable project intelligence
```

Then validate against competing coding systems on real repositories and repeated-work scenarios.

---

# 28. Benchmark suite for competitive validation

Create a private/public benchmark repository set with scripted scenarios.

Categories:

## Fresh task quality

Can the system solve a new issue with minimal unnecessary reading?

## Repeated issue

Does the second similar task require less context and fewer calls?

## Historical decision recall

Can it correctly answer why an architectural choice exists and find related code?

## Cross-device continuation

Can another machine continue without re-teaching project workflow?

## Rule compliance

Does generated code follow project/company standards automatically?

## Rule conflict

Does the resolver choose correctly and explain override?

## Branch correctness

Does knowledge from another worktree/branch stay out when invalid?

## Skill invalidation

Does a changed dependency/rule prevent unsafe replay?

## Token efficiency

Compare context size and tool round-trips per verified completion.

## Safety

Can malicious imported AI config persuade the system to bypass policy? It must not.

Run the same scenario against available competitor tools when legally and operationally feasible, recording observable behavior rather than marketing claims.

---

# 29. Performance targets

Initial engineering targets; revise from measured baseline.

```text
Unchanged AI-config startup:
  no content reparse
  no embedding
  no Cloud source upload

Single config-file change:
  only changed source parsed/synced

Resolver latency:
  target p95 low enough to remain hidden inside normal context retrieval

Context Compiler:
  bounded packet independent of total knowledge DB size

Graph traversal:
  bounded hops/results; never unbounded traversal per request

Skill replay:
  fewer model/tool round trips than normal planning for eligible tasks
```

Do not optimize percentages before instrumentation identifies the real bottleneck.

---

# 30. Security threat model additions

Project Brain introduces new attack surfaces.

Explicit threats:

- malicious repository instructions;
- poisoned imported skill;
- cross-user knowledge leakage;
- organization privilege leakage;
- stale rule used after access revocation;
- prompt injection hidden in Markdown/config;
- secret accidentally ingested as knowledge source;
- hostile source attempting to override security policy;
- replay skill attempting broader side effects than originally verified;
- tampered adapter manifest.

Required protections:

- local policy always authoritative for execution;
- adapter manifests signed/versioned;
- imported executable behavior untrusted by default;
- strict tenant/project scope in DB queries;
- source classification before ingestion;
- secret scanners + structured parsers where possible;
- no approval token persistence;
- context compiler labels untrusted descriptive content separately from mandatory trusted rules;
- audit every authority-changing action.

---

# 31. Observability

Every Project Brain decision should be diagnosable without logging secrets.

Metrics/events:

```text
knowledge.discovery.started/completed
knowledge.source.changed
knowledge.source.deduped
knowledge.source.conflict
knowledge.sync.noop
knowledge.sync.delta
knowledge.rule.resolved
knowledge.rule.overridden
knowledge.context.compiled
knowledge.context.cache_hit
knowledge.skill.matched
knowledge.skill.invalidated
knowledge.skill.revalidated
knowledge.experience.ingested
knowledge.claim.reinforced
knowledge.claim.superseded
```

Useful metrics:

```text
config_sources_discovered
config_sources_reparsed
manifest_noop_rate
bytes_uploaded_for_knowledge_sync
canonical_rule_count
rule_conflict_count
context_compiled_tokens_proxy
memory_precision_feedback
skill_hit_rate
skill_success_rate
skill_invalidation_rate
cross_device_reuse_rate
```

---

# 32. Migration strategy

Do not perform a flag-day rewrite.

Migration order:

```text
existing memories
   -> attach canonical provenance gradually
existing graph
   -> keep rendering while new entities are introduced
existing learned skills
   -> continue local replay
   -> add Cloud metadata/abstract definition
   -> later migrate to Skill v2 fingerprints
existing AGENTS support
   -> retain behavior
   -> add canonical discovery/resolution under feature flag
```

Feature flags should allow fallback to current vector-only / local-skill behavior while Project Brain matures.

---

# 33. Native CodeLocal format

CodeLocal should support its own clean portable structure while still importing other ecosystems.

Recommended future structure:

```text
project/
├── AGENTS.md
├── .codelocal/
│   ├── project.json
│   ├── rules/
│   │   ├── architecture.md
│   │   ├── code-style.md
│   │   ├── testing.md
│   │   └── security.md
│   ├── skills/
│   │   └── deploy/
│   │       └── SKILL.md
│   ├── commands/
│   └── agents/
└── .codelocal.local/
    └── machine/user overrides (gitignored)
```

Do not require this format for adoption. Existing editor conventions remain valid knowledge sources.

---

# 34. Decision log — 2026-08-15

These are intentional decisions and should not be accidentally reversed later.

## D1 — Durable knowledge is Cloud-first

Reason: the user's accumulated experience with AI must survive devices, editors, conversations, and reinstallations.

## D2 — Raw source code is not required as Cloud memory

Reason: retain useful knowledge while preserving local-code privacy and reducing storage/security exposure.

## D3 — Project identity is logical, not machine-path based

Reason: `/Users/A/project` and `D:\project` can be the same logical project.

## D4 — Branch-independent identity, branch-aware applicability

Reason: continuity belongs to the project, but some rules/skills/facts are revision-specific.

## D5 — Hashing is change detection, not truth resolution

Reason: content hashes tell us what changed; Resolver/provenance decide what is valid and authoritative.

## D6 — Watchers are optimization, manifests are correctness

Reason: watchers can miss events; occasional reconciliation must repair drift.

## D7 — Multiple editor formats normalize into one canonical model

Reason: CodeLocal should own durable intelligence rather than become another incompatible rule format.

## D8 — Hard rules are deterministic

Reason: mandatory policy must not disappear because vector similarity ranked it low.

## D9 — Learned skills require context fingerprints

Reason: a workflow verified yesterday may be unsafe after project state changes.

## D10 — Experience is a first-class entity

Reason: verified real work is better training signal for project behavior than isolated AI inference.

## D11 — Context must shrink as knowledge grows

Reason: Project Brain exists to remove repeated rediscovery, not to create giant prompts.

## D12 — Competitive advantage is system intelligence, not model ownership

Reason: CodeLocal can improve whichever strong model the user chooses.

## D13 — Coding agents are interchangeable workers behind CodeLocal

Reason: durable project identity, rules, memory, experiences, skills, verification and user continuity must survive a change from Codex to Claude, Cosine or a future agent. Universal Agent Runtime is therefore a first-class subsystem, while provider-specific engines remain replaceable adapters.

Detailed design: [`UNIVERSAL_AGENT_RUNTIME_PLAN.md`](./UNIVERSAL_AGENT_RUNTIME_PLAN.md).

---

# 35. What NOT to do

Avoid these shortcuts:

- scan the full repository on a timer;
- upload every project file to Cloud "just in case";
- store one giant project prompt;
- inject all memories into every request;
- trust imported skills automatically;
- resolve conflicting mandatory rules with pure embedding similarity;
- let learned skills override security policy;
- key project memory by local absolute path;
- use last-write-wins for offline multi-device conflicts;
- delete old historical knowledge when it becomes superseded;
- equate `mtime + size` with content truth;
- make Knowledge Graph UI a dependency of execution;
- claim token savings or superiority without measured benchmarks.

---

# 36. First concrete implementation slice

The next implementation should be deliberately narrow and prove the architecture.

Recommended slice:

```text
1. Add canonical knowledge_sources + rule schema.
2. Implement AGENTS + Claude + Cursor + Copilot adapters.
3. Add content/extraction fingerprints and manifest root.
4. Sync changed source metadata/content safely to Cloud.
5. Canonicalize/dedupe rules.
6. Implement deterministic Rule Resolver.
7. Feed compact effectiveRules into existing context pipeline.
8. Instrument context bytes/rule hit accuracy.
```

Do **not** start by implementing every editor, full organization sharing, or complex UI.

Prove that one real coding task:

```text
user request
-> relevant rules discovered automatically
-> no unchanged source rescanned
-> effective rules resolved correctly
-> compact context injected
-> code follows rules
-> verification passes
```

Then add Skill v2 and Experience learning.

---

# 37. Recommended implementation order after the first slice

```text
PB0 benchmark baseline
  ↓
PB1 canonical schema/provenance
  ↓
PB2 discovery adapters
  ↓
PB3 incremental manifest/sync
  ↓
PB5 Rule Resolver
  ↓
PB6 Context Compiler
  ↓
PB4 multi-device conflicts
  ↓
PB7 Skill v2
  ↓
PB8 Experience Learning
  ↓
PB9 memory lifecycle/correction
  ↓
PB10 Knowledge Graph control plane
  ↓
PB11 organization/team knowledge
  ↓
PB12 adapter ecosystem/export
  ↓
PB13 Universal Agent Runtime / coding-engine router
```

PB4 may move earlier if cross-device conflicts become common during development. PB13 should begin its UAR0/UAR1 architecture work once PB1-PB6 interfaces are stable enough to supply canonical project identity, rules and compiled context; full learned routing should wait for verified Experience data.

## Canonical semantic index rollout boundary

Canonical embeddings are a **derived/rebuildable Project Brain index**, never a second durable truth. The implemented v38/v39 path follows these rules:

- v38 core migration stores projection state without requiring pgvector; when the feature is explicitly enabled, an on-demand derived vector schema stores embeddings by tenant/project/revision/provider/model/model-version with explicit dimension and content hash;
- provider/model version changes create a new cohort instead of mutating canonical knowledge;
- provider calls are batched outside DB transactions, then the canonical source snapshot is revalidated before commit;
- active private Knowledge V2 revisions are the only embedding source; secret-like content is skipped;
- freshness is reported separately as `current | stale | missing | empty` and does not degrade deterministic canonical truth;
- semantic query shadowing is separately opt-in and aggregate-only metrics are stored by v39;
- a semantic Hybrid canary exists but defaults OFF; it may enter model context only after base Knowledge V2 readiness, current vector freshness, and semantic-shadow readiness all report `ready`; any gate/error/timeout falls back to deterministic Hybrid unchanged;
- v40 records aggregate canary outcomes only, with bounded asynchronous metric writes so observability never adds a synchronous DB write to the prompt path; no query/task/summary/repository/path/symbol content is stored;
- the live semantic lane is additionally protected by a bounded in-memory circuit breaker: per-project aggregate metrics are refreshed asynchronously into a 30-second cache, cache miss/staleness fails closed to deterministic Hybrid, bad error/timeout/slow/readiness-block rates block semantic attempts after enough samples, and a single five-minute half-open probe permits automatic recovery without changing user configuration.

This preserves the retrieval hierarchy: deterministic canonical retrieval is safe without vectors; semantic vectors are optional acceleration/ranking evidence that can be deleted and rebuilt.

---

# 38. Final product vision

A mature CodeLocal session should feel like working with an engineer who has been on the project for years.

The user should be able to say:

```text
"Fix this issue."
```

and CodeLocal already understands, without unnecessary re-teaching:

```text
which project this is
which repo/module matters
which coding standards apply
what similar incidents happened before
what decisions must be preserved
what code paths are relevant
whether a verified workflow already exists
what changed since that workflow was learned
what minimal context the AI needs
how to verify the result
```

After successful work, the system should retain only the durable value of the experience and become better prepared for the next task.

The desired long-term behavior is:

```text
More shared experience
      ↓
Better project understanding
      ↓
Less rediscovery
      ↓
Smaller prompts
      ↓
Fewer tool/model round trips
      ↓
Safer reuse of verified workflows
      ↓
Faster, more consistent engineering
```

That is the Project Brain moat.

---

# 39. Maintenance rule for this document

Whenever a major Project Brain architecture decision changes:

1. update the relevant section;
2. add a dated entry to the Decision Log;
3. state the migration impact;
4. identify old behavior that becomes deprecated;
5. add/update acceptance tests before calling the migration complete.

Do not allow the architecture to exist only in chat history.
