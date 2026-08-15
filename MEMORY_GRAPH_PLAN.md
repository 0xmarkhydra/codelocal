# CodeLocal Memory Graph Upgrade Plan

Status: Proposed
Date: 2026-08-15
Scope: Extend the existing Agent Memory + pgvector + Code Graph + Learned Skills architecture with conversation-aware personal/project knowledge graphs without increasing the public MCP tool surface.

## 1. Goal

Make CodeLocal progressively understand the user, projects and relationships over time instead of only recalling isolated memory summaries.

The target model is hybrid:

```text
Conversation / Tool activity / Project state
                 |
                 v
          Memory Extractor
                 |
        +--------+--------+
        |                 |
        v                 v
  Vector Memory      Knowledge Graph
  pgvector + FTS     nodes + edges + time
        |                 |
        +--------+--------+
                 |
                 v
          Hybrid Recall
                 |
                 v
             ChatGPT
                 |
                 v
          Learned Skills
```

The three systems have different responsibilities:

- Vector memory answers: "What past memory is semantically related?"
- Knowledge graph answers: "How are the user, projects, people, goals, decisions and events connected?"
- Learned Skills answers: "How should CodeLocal perform this recurring task quickly?"

## 2. Reuse existing foundations

Do not replace existing systems.

Reuse:

- `taskstate` as hot working memory.
- L0/L1/L2/L3 long-term memory model.
- PostgreSQL + full-text search.
- pgvector embeddings and hybrid ranking.
- existing sanitization/redaction path.
- user/workspace isolation.
- existing Code Graph/import/call graph for source-code intelligence.
- new local Learned Skills store for executable workflows.
- current compact MCP surface; graph/memory operations stay private/internal.

## 3. Add a separate Memory Graph

Do not mix the project source-code graph with the personal/project memory graph in one schema.

### 3.1 Node model

Initial node types:

- `user`
- `person`
- `project`
- `company`
- `goal`
- `decision`
- `preference`
- `idea`
- `constraint`
- `problem`
- `milestone`
- `event`
- `tool`
- `workspace`
- `skill`
- `technology`
- `artifact`

Suggested fields:

```text
id
user_id
workspace_id nullable
scope global|workspace
kind
canonical_name
summary
confidence
importance
valid_from
valid_to nullable
first_seen_at
last_seen_at
source_type conversation|task|tool|project|skill
source_session_id nullable
source_memory_id nullable
metadata jsonb
embedding vector(768) optional
```

### 3.2 Edge model

Initial relations:

- `OWNS`
- `WORKS_ON`
- `FOUNDED`
- `PREFERS`
- `USES`
- `DEPENDS_ON`
- `HAS_GOAL`
- `HAS_DECISION`
- `BLOCKED_BY`
- `RELATED_TO`
- `PART_OF`
- `SUCCEEDED_WITH`
- `FAILED_WITH`
- `LEARNED_SKILL`
- `SUPERSEDES`
- `MENTIONED_IN`

Suggested fields:

```text
id
user_id
workspace_id nullable
from_node_id
to_node_id
relation
confidence
importance
valid_from
valid_to nullable
first_seen_at
last_seen_at
source_session_id nullable
source_memory_id nullable
metadata jsonb
```

## 4. Conversation Memory ingestion

Current CodeLocal receives task/tool facts but not ordinary conversation facts. Add a private ingestion path for important conversational milestones.

### 4.1 What should be retained

Extract only durable/useful facts:

- goals
- preferences
- project decisions
- constraints
- important people/organizations
- milestones
- recurring problems
- useful ideas
- explicit user facts
- stable technical bindings

Do not store routine small talk by default.

### 4.2 Extractor output

Example conversation:

> "Mục tiêu của tôi là CodeLocal đạt 10.000 users cuối năm."

Extractor output:

```json
{
  "memories": [
    {
      "kind": "goal",
      "scope": "global",
      "summary": "CodeLocal target is 10,000 users by year end",
      "importance": 0.92,
      "confidence": 0.98,
      "entities": ["user", "CodeLocal"],
      "relations": [
        {"from": "user", "relation": "HAS_GOAL", "to": "CodeLocal 10k users goal"}
      ]
    }
  ]
}
```

### 4.3 Ingestion gates

Store only when one or more are true:

- user explicitly asks to remember;
- importance above threshold;
- repeated fact appears across sessions;
- fact materially affects future decisions/actions;
- project decision/milestone was confirmed;
- durable user preference is detected with high confidence.

## 5. Global user memory vs workspace memory

Current long-term memory is mainly workspace-scoped. Add explicit global scope.

### Global user scope

Examples:

- communication preferences;
- stable personal/professional profile;
- cross-project goals;
- preferred workflows;
- relationships to people/companies.

Store with:

```text
user_id = X
scope = global
workspace_id = NULL
```

### Workspace scope

Examples:

- BIDDI bundle IDs;
- FlashX architecture decisions;
- CodeLocal release choices;
- repo-specific bugs;
- project milestones.

Store with:

```text
user_id = X
scope = workspace
workspace_id = Y
```

Recall should query global + current workspace and rank workspace facts higher when task-specific.

## 6. Entity resolution and deduplication

This is critical or the graph becomes noisy.

Pipeline:

```text
new fact
  -> normalize name
  -> vector/lexical candidate search
  -> entity compatibility check
  -> merge existing node OR create new node
```

Examples that should converge:

```text
CodeLocal
codex-mcp
codelocal project
our MCP project
```

Only merge automatically above a high confidence threshold. Ambiguous entities remain separate until additional evidence arrives.

## 7. Temporal memory and contradiction handling

Do not overwrite history blindly.

Example:

```text
2026-08-01: "Prefer Node.js"
2026-08-15: "Prefer Go for CodeLocal runtime"
```

Represent changes with validity ranges and/or `SUPERSEDES` edges.

Rules:

- preserve old facts;
- mark superseded facts inactive for default recall;
- keep source/session provenance;
- latest confirmed user statement has strong temporal weight;
- contradictions reduce confidence until resolved.

## 8. Hybrid retrieval: Vector + Graph + Recency

Do not graph-traverse the entire database for every request.

Suggested retrieval:

```text
query
  -> vector/FTS seed memories (top 6-12)
  -> map seeds to graph nodes
  -> bounded 1-2 hop graph expansion
  -> add global user anchors
  -> temporal/confidence/importance rerank
  -> compact context packet
```

Ranking signals:

- semantic similarity;
- exact lexical match;
- graph distance;
- relation importance;
- current workspace match;
- recency;
- confidence;
- importance;
- repeated confirmation count.

Return a bounded set, not the entire graph.

## 9. Link memory graph to Code Graph

Keep schemas separate but allow controlled cross-links.

Examples:

```text
Memory node: "Learned Skills feature"
    -> IMPLEMENTED_IN
Code entity: internal/learnedskills/store.go

Memory node: "OAuth 400 incident"
    -> RELATED_CODE
Code symbols/files involved in OAuth path
```

This lets CodeLocal answer not only "what happened?" but also "where in the code did it happen?"

Cross-links should use stable file/symbol identifiers where available, never raw graph coordinates.

## 10. Link Memory Graph to Learned Skills

A confirmed recurring solution can create or strengthen a skill node.

Example:

```text
Project: BIDDI
   -> HAS_SKILL
Skill: open_biddi_beta
   -> USES
Bundle: vn.infivision.biddi.dev
```

The graph decides relevance; the local Learned Skills store remains the executable source of truth.

Executable commands/paths remain local. Cloud graph stores only safe metadata and references.

## 11. Privacy model

Preserve the current privacy boundary.

### Cloud allowed

- sanitized summaries;
- entity names when safe;
- relations;
- confidence/importance/time;
- embeddings from sanitized text;
- user/workspace IDs;
- safe project/file references.

### Local-only by default

- raw conversation transcript unless explicitly enabled;
- raw terminal output;
- screenshots;
- secrets/credentials;
- executable learned-skill recipe;
- sensitive machine paths where avoidable.

Every graph query and mutation must be scoped by `user_id`; workspace data additionally enforces `workspace_id`.

## 12. Server storage

Phase 1 should stay in PostgreSQL; do not introduce Neo4j or another graph database yet.

Add tables such as:

```text
memory_nodes
memory_edges
memory_node_aliases
memory_sources
```

Use recursive CTEs/bounded SQL joins for 1-2 hop traversal. This keeps deployment simple on Railway and reuses the current DB/pgvector infrastructure.

Only evaluate a dedicated graph DB later if measured graph traversal becomes a real bottleneck.

## 13. Private/internal operations

Do not add public MCP tools.

Suggested private operations:

```text
memory_ingest_conversation
memory_graph_upsert_nodes
memory_graph_upsert_edges
memory_graph_recall
memory_graph_feedback
memory_graph_forget
```

Public ChatGPT-facing behavior remains through `context` and `agent`.

`context` should receive a compact `memoryContext` containing:

- relevant user profile facts;
- workspace memories;
- graph relationships;
- recent decisions/goals;
- related learned-skill metadata.

## 14. Conversation access constraint

MCP cannot silently read the entire ChatGPT conversation.

Therefore conversation memory requires ChatGPT/orchestration to explicitly send candidate durable facts/events to the private ingestion path.

Do not make correctness depend on receiving every message. The system should be useful even when only high-value milestones are forwarded.

## 15. Rollout phases

### M0 — Schema + feature flag

- add graph schema migrations;
- `CODELOCAL_MEMORY_GRAPH_ENABLED` rollback flag;
- tests for tenant/workspace isolation;
- no behavior change yet.

### M1 — Build graph from existing memories

- backfill current L1/L2/L3 rows into event/scenario/project nodes;
- create workspace/project relations;
- preserve current vector memory IDs as provenance;
- no conversation ingestion yet.

### M2 — Hybrid graph recall

- vector/FTS seeds + bounded graph expansion;
- global + workspace scopes;
- inject compact graph facts into `context`;
- measure latency and recall quality.

### M3 — Conversation Memory

- add private milestone ingestion;
- classify goal/preference/decision/person/project/idea/constraint/problem;
- entity resolution and aliases;
- dedupe and contradiction handling.

### M4 — User Profile memory

- synthesize compact `USER` profile from high-confidence global graph nodes;
- dynamically refresh rather than keeping one giant static profile;
- allow forget/update flows.

### M5 — Graph + Learned Skills

- link skill metadata to projects/entities/problems;
- graph recall can surface the right local skill;
- confidence feedback from skill success/failure updates relationship strength.

### M6 — Graph visualization

Optional dashboard similar to the reference image:

- interactive nodes/edges;
- filters by project/person/goal/skill/time;
- inspect provenance;
- merge/delete/forget node;
- never make visualization a dependency of agent execution.

## 16. Quality gates

Before enabling by default:

- zero cross-user memory leakage tests;
- global/workspace scope tests;
- secret sanitization tests;
- entity dedupe tests;
- contradiction/temporal tests;
- graph traversal boundedness tests;
- recall latency budget;
- graceful fallback to current vector-only memory;
- `go test ./...`;
- `go vet ./...`;
- `go build ./...`;
- npm packaging validation.

## 17. Success criteria

After several weeks of use, CodeLocal should be able to answer questions such as:

- "Mục tiêu dài hạn của tôi với CodeLocal là gì?"
- "Tại sao hồi trước mình đổi runtime sang Go?"
- "Những vấn đề nào liên quan tới OAuth 400 trước đây?"
- "FlashX đang phụ thuộc những quyết định nào?"
- "Tôi thường thích anh xử lý task theo kiểu nào?"
- "Lần trước mở BIDDI Beta mình đã dùng cách nào?"

without scanning months of chat history or injecting the entire memory database into the model context.

## 18. Recommended implementation order

Highest-value order:

```text
1. schema + global scope
2. backfill existing vector memory into graph
3. hybrid vector + graph recall
4. conversation milestone ingestion
5. temporal contradiction handling
6. Learned Skills linking
7. visualization/dashboard
```

Do not build the visualization first. The graph must improve agent recall and decisions before it becomes a UI feature.

## 19. Token economy: Memory Budgeter + Context Compiler

Memory only saves model tokens when recall is aggressively bounded. Add a private Context Compiler that decides what the model actually sees for each request.

Pipeline:

```text
user request
  -> intent/task classifier
  -> vector/FTS seed recall
  -> bounded graph expansion
  -> optional Code Graph anchors
  -> optional Learned Skill candidate
  -> dedupe + contradiction filtering
  -> token budget allocation
  -> compact context packet
  -> ChatGPT
```

The compiler should allocate separate budgets, for example:

```text
user-global memory:       small fixed budget
workspace memory:         task-weighted budget
graph relationships:      1-2 hop compact facts
code context:              symbol/snippet budget
learned skill metadata:    tiny budget unless selected
raw evidence:              zero by default; fetch only on demand
```

Never inject full conversation history, full graph neighborhoods, full source files or all remembered facts by default.

### Token-saving mechanisms

1. Progressive disclosure: return summaries/IDs first, details only when the model asks.
2. Graph compression: represent repeated relationships once instead of repeating prose memories.
3. Memory consolidation: merge repeated low-level events into one stronger scenario/knowledge node.
4. Skill fast path: recurring tasks skip repeated observe/search/read/planning loops.
5. Code Graph targeting: retrieve exact symbols/neighbor files instead of repository-wide scans.
6. Temporal pruning: superseded or stale facts are excluded from normal context.
7. Confidence pruning: weak memories do not consume context unless specifically relevant.
8. Context cache: reuse stable compiled packets within the same task until source state changes.
9. Evidence-on-demand: raw logs/diffs/screenshots remain outside the model context until needed.
10. Round-trip reduction: agent executes bounded multi-step programs and Learned Skills in fewer MCP calls.

### Measurement targets

Do not claim a fixed token saving percentage before measurement. Instrument these metrics and compare against the current vector-only baseline:

```text
input_tokens_per_completed_task
model_round_trips_per_task
mcp_calls_per_task
context_bytes_per_context_call
retrieved_memory_count
retrieved_graph_node_count
retrieved_graph_edge_count
skill_hit_rate
skill_success_rate
recall_precision_proxy
completion_latency_ms
```

Initial success target: repeated tasks and mature workspaces should show a clear downward trend in context size, MCP calls and round trips while maintaining or improving task completion quality.

## 20. Brain-like reinforcement and consolidation

Treat memory strength as dynamic, not permanent.

A memory/node/edge gains strength when:

- the user explicitly confirms it;
- it is recalled and proves useful;
- the same fact is independently observed again;
- a related Learned Skill succeeds;
- it materially contributes to a verified task.

It loses strength when:

- a newer fact supersedes it;
- a recalled fact proves wrong;
- a skill using it repeatedly fails;
- it becomes stale and is never reused.

Suggested signals:

```text
confidence
importance
confirmation_count
successful_recall_count
failed_recall_count
last_confirmed_at
last_used_at
superseded_at
```

Periodically consolidate memory:

```text
many L1 events
   -> one L2 scenario
many repeated L2 scenarios
   -> one L3 durable rule/knowledge node
repeated successful procedure
   -> strengthen/create Learned Skill
```

This is the key mechanism that makes CodeLocal progressively smaller in context while retaining more useful knowledge.

## 21. Concrete implementation roadmap

### P0 — Observability baseline

Before changing recall behavior:

- record token/context proxy metrics available at the gateway;
- record MCP/tool-call counts and task duration;
- record current vector recall size/latency;
- create a repeatable benchmark set: new task, repeated task, old-project recall, cross-thread continuation.

### P1 — Graph storage + global scope

- implement `memory_nodes`, `memory_edges`, aliases and provenance;
- add `scope=global|workspace`;
- add isolation and migration tests;
- keep current vector recall unchanged behind a feature flag.

### P2 — Backfill + entity resolution

- project current L1/L2/L3 memories into graph nodes;
- resolve workspace/project/user anchors;
- dedupe aliases;
- create temporal/provenance links.

### P3 — Hybrid Recall v1

- vector/FTS top seeds;
- bounded 1-hop graph expansion first;
- rerank by workspace, recency, confidence and importance;
- return compact `memoryContext` through existing `context` tool.

### P4 — Context Compiler / token budget

- enforce hard context budgets by section;
- progressive disclosure;
- consolidation-aware retrieval;
- cache compiled context until project/task state invalidates it;
- compare token/context proxy metrics with P0 baseline.

### P5 — Conversation milestones

- add private conversation-memory ingestion;
- extract only high-value facts/events;
- support global user profile and project-specific decisions;
- contradiction and `SUPERSEDES` handling.

### P6 — Reinforcement + forgetting

- update memory confidence from recall usefulness and confirmations;
- decay stale low-value facts;
- consolidate repeated events into scenarios/knowledge;
- expose safe forget/update flows.

### P7 — Learned Skills integration

- graph links project/problem/entity to local skill metadata;
- use graph/vector recall to select candidate skill;
- successful replay reinforces relevant memory/edges;
- failed replay weakens them and falls back to normal agent reasoning.

### P8 — Code Graph bridge

- map memory nodes to stable file/symbol identifiers;
- use memory recall to seed Code Graph traversal;
- use Code Graph findings to strengthen project knowledge after verified changes.

### P9 — Visualization

Only after the intelligence path is proven:

- render Memory Graph like the reference UI;
- inspect why a fact is remembered;
- show confidence/time/source;
- merge, correct or forget nodes;
- visualize skill/project/code relationships.

## 22. Expected behavior after maturation

For a brand-new task, CodeLocal still reasons normally.

For a familiar task/project, the path becomes:

```text
request
 -> recall 3-8 high-value memories
 -> follow 1-hop relationships
 -> locate exact code/skill anchors
 -> execute bounded plan or learned skill
 -> verify
 -> reinforce useful memory
```

Instead of:

```text
request
 -> reread large history
 -> broad repository scan
 -> repeated observe/search calls
 -> rediscover prior decisions
 -> rebuild the same plan
```

The intended long-term effect is not that the model itself becomes more intelligent. CodeLocal makes the model operate with better external memory, better retrieval and reusable procedures, so the whole system behaves more intelligently and wastes less context/round trips over time.
