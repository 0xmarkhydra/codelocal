# CodeLocal Agent Memory Plan

Status: production integration implemented; validation in progress
Base: `feat/smart-computer-runtime` @ `dacb0fe`
Branch: `feat/agent-memory`

## Goal

Turn CodeLocal from session-only working memory into a persistent project memory system without changing the user experience (`npm install -g codelocal && codelocal`).

The design borrows the useful ideas from TencentDB-Agent-Memory — hierarchical memory, context offloading, progressive disclosure and traceable evidence — but keeps CodeLocal's existing privacy boundary and Go/native runtime architecture.

## Architecture

```text
ChatGPT
   |
   v
CodeLocal MCP
   |
   +--> Local hot memory
   |      - current task state
   |      - branch
   |      - touched files
   |      - recent checks/errors
   |
   +--> Local evidence
   |      - raw tool/process output
   |      - source/diff/screenshot references
   |      - never uploaded by default
   |
   `--> CodeLocal Cloud long-term memory
          - PostgreSQL metadata/full-text
          - pgvector semantic index when available
          - sanitized summaries only
          - user/workspace isolation
          - embedding provider abstraction
```

## Memory hierarchy

- L0 Evidence: raw local evidence references. Local-only by default.
- L1 Event: sanitized facts such as "OAuth integration test failed with redirect mismatch".
- L2 Scenario: task/incident summary with root cause, files, resolution and verification.
- L3 Workspace knowledge: reusable architecture conventions, workflows and recurring solutions.

`taskstate` remains the hot working-memory layer. The new memory service is warm/cold long-term memory.

## Privacy contract

The Cloud memory path MUST NOT upload by default:

- source file contents;
- raw git diffs;
- screenshots;
- raw terminal output;
- raw commands containing secrets;
- `.env`, credentials, private keys or tokens.

Cloud memory stores sanitized summaries, identifiers/metadata that pass policy, and embeddings derived from sanitized text.

Every Cloud memory row is scoped by `user_id` and `workspace_id`.

## Retrieval

Hybrid ranking:

1. hard filter by user/workspace;
2. PostgreSQL full-text score;
3. pgvector cosine similarity when vector capability is available;
4. recency;
5. memory confidence/importance;
6. optional file/symbol overlap from the current task;
7. merge into a bounded result set.

Progressive disclosure: recall returns compact summaries and memory IDs. Raw evidence stays local and is only fetched separately when needed.

## Embedding provider

Interface first. Initial provider: Gemini embedding using server environment credentials.

Expected configuration:

- memory is enabled by default; set `CODELOCAL_MEMORY_ENABLED=0` for an immediate rollback
- `CODELOCAL_EMBEDDING_PROVIDER=gemini`
- `GEMINI_API_KEY=...` (optional; without it memory remains lexical/full-text)
- optional `CODELOCAL_EMBEDDING_MODEL=...`; default is `gemini-embedding-2`
- Gemini output is fixed to 768 dimensions so pgvector HNSW indexing stays within its `vector` dimension limit

No key, quota failure or provider outage MUST NOT break CodeLocal. In those cases memory falls back to PostgreSQL full-text/metadata and vector indexing can be backfilled later.

Provider interface must allow adding OpenRouter, Voyage, OpenAI or local embeddings without changing storage/retrieval contracts.

## M1 — Storage foundation

- Add memory tables to CloudStore.
- Detect/install pgvector safely when permitted; do not assume extension exists.
- Keep a capability flag for vector availability.
- Add migration-safe indexes and retention-ready metadata.
- Tests for schema behavior where possible without requiring a live DB.

## M2 — Embedding abstraction

- Define `EmbeddingProvider` contract.
- Implement Gemini provider with timeouts, batch support, bounded input and explicit model/dimension metadata.
- Never log API keys or raw request headers.
- Fail open to lexical memory when embeddings fail.

## M3 — Memory service

- Sanitize memory text before persistence/embedding.
- Ingest L1/L2/L3 records with dedupe/idempotency keys.
- Hybrid recall API.
- Compact result shape suitable for MCP context injection.
- Confidence, importance, created/last-used timestamps.

## M4 — Smart-runtime integration

- Project current `taskstate` into safe L1/L2 memory records at useful boundaries, not every tool call. Implemented for successful edits, verified tasks and stable error events.
- Recall relevant workspace memories during `context`/task setup. Implemented with bounded hybrid recall and compact structured content.
- Do not increase the public MCP tool count unless progressive inspection truly requires it.
- Keep ChatGPT as planner; memory is context, not an autonomous planner.

## M5 — Reliability and operations

- Unit tests for sanitization/ranking/provider failures.
- `go test ./...`, `go vet ./...`, `go build ./...`.
- npm release cross-build validation.
- Metrics: ingest count, recall count, embedding failures, vector fallback rate; no source/secrets in logs.
- Feature flag and graceful rollback path.

## Initial acceptance criteria

1. Existing CodeLocal behavior is unchanged with memory disabled.
2. Memory enabled without Gemini key still works lexically.
3. Gemini failure never blocks an MCP/code operation.
4. User A cannot retrieve User B memory; workspace filters are mandatory.
5. Secret-like strings are redacted before Cloud persistence and embedding.
6. pgvector absence is a degraded mode, not startup failure.
7. Recall returns bounded compact summaries, not raw evidence.
8. Existing smart-runtime tests/build/package remain green.
