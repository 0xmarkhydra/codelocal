# CodeLocal Central Codex Account Pool — Master Plan

Status: **Proposed — ready for implementation spike**  
Date: **2026-09-02**  
Owner: **CodeLocal**  
Scope: **Private, single-owner provider account pool used only by the owner's CodeLocal account**

This document is the source of truth for building a private central AI account pool that can hold and operate many owner-controlled Codex/OpenAI provider accounts, then expose one stable internal API to CodeLocal.

The initial target is Codex, but the architecture must remain provider-neutral so Claude, Gemini, OpenCode-compatible providers, NVIDIA endpoints and other providers can be added later without changing CodeLocal's core execution/runtime model.

---

# 1. Product decision

Build a **separate central Pool service** instead of putting hundreds or thousands of provider credentials directly inside the CodeLocal MCP gateway.

Target shape:

```text
Owner-controlled provider accounts
  ├── Codex #0001
  ├── Codex #0002
  ├── ...
  └── Codex #1000
           |
           v
     CodeLocal Pool
     ├── Credential Vault
     ├── Provider Adapters
     ├── Account Registry
     ├── Health / Quota State
     ├── Scheduler
     ├── Cooldown / Circuit Breaker
     ├── Token Refresh
     ├── Usage Accounting
     └── OpenAI-compatible Gateway
           |
           v
     ONE INTERNAL API
           |
           v
      CodeLocal Cloud
           |
           v
      owner's CodeLocal sessions
```

CodeLocal must never need to know which physical provider account handled a request.

The CodeLocal integration contract should look like a normal provider:

```text
CODELOCAL_AI_POOL_BASE_URL=https://pool.codelocal.cloud/v1
CODELOCAL_AI_POOL_SERVICE_KEY=<internal service credential>
```

The pool chooses provider/account, performs refresh/failover and streams the result back.

---

# 2. Important scope boundary

This plan is intentionally scoped to:

- accounts controlled by the same owner;
- private/internal usage;
- no contributor marketplace;
- no public resale of account access;
- no external user depositing credentials into the pool;
- no requirement for contributor machines to stay online;
- no provider credential sent to the CodeLocal native runtime;
- no credential exposed to ChatGPT, Codex, Claude or MCP tool payloads.

The system must still have a provider-policy kill switch. Ownership of an account does not automatically imply every provider permits every form of automated multiplexing. Each provider adapter must therefore be independently disableable without affecting CodeLocal itself.

---

# 3. BA — three implementation cases

## Case A — Run 9Router as the pool server

```text
CodeLocal
   |
   v
9Router deployment
   |
   +-- Codex #1
   +-- Codex #2
   +-- ...
   +-- Codex #N
```

### Advantages

- fastest way to validate Codex multi-account behavior;
- existing provider/OAuth/token-refresh logic can be exercised quickly;
- OpenAI-compatible routing behavior already exists;
- useful as a reference implementation and protocol oracle.

### Problems

- CodeLocal Cloud is currently Go-centric while 9Router brings its own application/runtime architecture;
- CodeLocal would inherit 9Router data shapes and lifecycle assumptions;
- scaling from tens to ~1000 accounts requires stronger distributed locking, scheduling and observability than a normal personal router;
- authentication, tenancy and credential-vault guarantees must remain controlled by CodeLocal;
- difficult to evolve CodeLocal's `auto` routing without coupling product logic to an external project's internal model.

### Decision

**Use only as a spike/reference. Do not make a stock 9Router deployment the permanent control plane.**

---

## Case B — Separate CodeLocal Pool service, reuse/adapt 9Router provider behavior

```text
                       CodeLocal Cloud
                             |
                    internal service auth
                             |
                             v
                  pool.codelocal.cloud
                             |
          +------------------+------------------+
          |                  |                  |
          v                  v                  v
      Scheduler           Vault           Provider layer
                                                  |
                                      behavior/adapters informed
                                         by 9Router patterns
                                                  |
                                   +--------------+--------------+
                                   |                             |
                                 Codex                        future
```

### Advantages

- keeps CodeLocal core clean;
- one independent failure domain;
- pool can scale independently from MCP/WebSocket traffic;
- provider-specific token/refresh behavior can evolve without touching local execution;
- supports ~1000 accounts with purpose-built leases, health and queueing;
- future `model=auto` can choose provider + model + account;
- provider engine can later be rewritten incrementally without changing the CodeLocal API contract.

### Cost

- requires building a real pool control plane;
- requires secure credential migration/import flow;
- requires dedicated storage and operations.

### Decision

**RECOMMENDED. Build this architecture.**

---

## Case C — Implement the complete pool natively inside CodeLocal Cloud

```text
CodeLocal Cloud
  ├── MCP gateway
  ├── WebSocket routing
  ├── Project Brain
  ├── provider vault
  ├── OAuth
  ├── 1000-account scheduler
  └── AI response proxy
```

### Advantages

- one deployment stack;
- maximum reuse of existing Go/PostgreSQL/Redis infrastructure;
- no extra network hop.

### Problems

- credential failures/provider outages become coupled to the main CodeLocal control plane;
- AI streaming load competes with MCP/runtime routing;
- provider-specific complexity pollutes the core gateway;
- harder to isolate secrets and blast radius;
- future provider experiments require redeploying the main service.

### Decision

**Do not use this as the final architecture.** Shared libraries are acceptable; shared process ownership is not.

---

# 4. Recommended architecture

Choose **Case B**.

```text
                         Internet
                            |
                +-----------+-----------+
                |                       |
                v                       v
         codelocal.cloud        pool.codelocal.cloud
         CodeLocal Cloud          Pool Control Plane
                |                       |
                | internal mTLS/service key
                +---------------------->|
                                        |
                         +--------------+--------------+
                         |              |              |
                         v              v              v
                    API Gateway      Scheduler      Admin API
                         |              |              |
                         +-------+------+--------------+
                                 |
                                 v
                         Provider Executor
                                 |
                  +--------------+--------------+
                  |              |              |
                  v              v              v
               Codex A        Codex B        Codex N
```

The pool is a **control plane + provider execution gateway**, not a local runtime.

CodeLocal local/native clients continue doing what they already do: project authorization, file access, terminal, browser/computer and local MCP execution. They must not receive provider refresh credentials.

---

# 5. Core services

## 5.1 Account Registry

Stores account metadata and operational state, never plaintext credentials in normal application columns.

Minimum fields:

```text
ProviderAccount
  id
  provider
  label
  ownerId
  credentialRef
  status
  healthScore
  priority
  weight
  maxConcurrency
  activeLeases
  cooldownUntil
  rateLimitedUntil
  lastSuccessAt
  lastFailureAt
  lastErrorCode
  quotaSnapshot
  quotaUpdatedAt
  createdAt
  updatedAt
```

Status values:

```text
PENDING
HEALTHY
DEGRADED
RATE_LIMITED
COOLDOWN
AUTH_REQUIRED
DISABLED
DEAD
```

Do not use email as the primary account identity. Provider identifiers can change and emails may not uniquely encode entitlement state.

---

## 5.2 Credential Vault

Credential storage is a separate security boundary.

Requirements:

- encrypt credentials at rest using envelope encryption;
- master key comes from production secret/KMS infrastructure, not the database;
- each account record has an independently encrypted data key or equivalent envelope;
- never log access tokens, refresh tokens, cookies or authorization codes;
- never return raw credentials from normal admin APIs;
- audit credential create/rotate/revoke operations;
- support provider-specific credential schema without exposing it to CodeLocal;
- support token rotation where a refresh operation returns a new refresh token;
- atomic compare-and-swap/transaction on credential version.

Conceptual data:

```text
ProviderCredential
  credentialRef
  provider
  ciphertext
  keyVersion
  credentialVersion
  expiresAt
  refreshedAt
  createdAt
  updatedAt
```

---

## 5.3 Provider Adapter

Stable internal interface:

```text
Authenticate / Connect
RefreshCredential
ValidateCredential
ListModels
ExecuteResponse
ReadQuota / InferQuota
ClassifyError
```

Provider-specific code must map raw upstream errors into normalized pool errors:

```text
AUTH_EXPIRED
RATE_LIMITED
QUOTA_EXHAUSTED
TRANSIENT_UPSTREAM
MODEL_UNAVAILABLE
BAD_REQUEST
POLICY_BLOCKED
UNKNOWN
```

Initial adapter: `codex`.

Future adapters must not change scheduler or CodeLocal API contracts.

---

## 5.4 Scheduler

The scheduler is the main feature that distinguishes a real 1000-account pool from a personal router.

Candidate selection:

```text
1. provider supports requested model/capability
2. status allows traffic
3. cooldownUntil <= now
4. rateLimitedUntil <= now
5. activeLeases < maxConcurrency
6. credential valid or refreshable
7. quota is not known-exhausted
8. account passes health threshold
```

Recommended scoring:

```text
score =
    priorityWeight
  * healthFactor
  * quotaFactor
  * latencyFactor
  * concurrencyFactor
  * recencyDistributionFactor
```

Do not implement naive round-robin as the only strategy. It wastes requests on known-bad/rate-limited accounts and creates synchronized refresh pressure.

Use Redis for short-lived distributed leases and hot operational state. PostgreSQL remains the durable source of truth.

Lease key example:

```text
pool:lease:<providerAccountId>:<requestId>
```

Lease requirements:

- TTL;
- release on stream completion;
- release on cancellation;
- recover automatically after worker crash;
- do not exceed per-account concurrency.

---

## 5.5 Refresh Coordinator

Only one worker may refresh the same credential version at a time.

Bad behavior:

```text
worker A -> refresh token R1 -> gets R2
worker B -> refresh token R1 -> fails/revokes chain
```

Required behavior:

```text
request sees expiring token
        |
        v
acquire refresh lock(accountId, credentialVersion)
        |
        +-- lock lost -> wait/re-read credential
        |
        v
refresh once
        |
        v
persist access token + rotated refresh token atomically
        |
        v
release lock
```

Redis may coordinate the lock, but the persisted credential version must protect correctness if a lock expires.

---

## 5.6 Health / Circuit Breaker

Every account maintains rolling health state.

Example rules:

```text
success                  -> health increases slowly
429/rate-limit           -> temporary cooldown
quota exhausted          -> long cooldown until reset estimate
401 after refresh        -> AUTH_REQUIRED
5xx/timeouts             -> degrade + short retry/circuit break
repeated fatal auth      -> DISABLED/DEAD
```

An account should automatically return from `COOLDOWN` to probe mode when the cooldown expires.

Do not require an operator to manually revive temporary rate limits.

---

## 5.7 Usage Accounting

Track both request-level and account-level usage.

```text
PoolRequest
  id
  ownerId
  requestedModel
  resolvedProvider
  resolvedModel
  providerAccountId
  startedAt
  firstByteAt
  completedAt
  status
  inputTokens
  outputTokens
  cachedTokens
  retryCount
  errorClass
```

Do not store full prompts/responses by default.

Usage logging must be useful for:

- health decisions;
- quota prediction;
- debugging;
- capacity planning;
- detecting one account being over-selected.

---

# 6. API contract exposed to CodeLocal

Start with a narrow internal API rather than exposing the entire pool admin surface.

Minimum:

```text
GET  /health
GET  /v1/models
POST /v1/responses
```

Optional compatibility endpoint later:

```text
POST /v1/chat/completions
```

CodeLocal authenticates using an internal service credential. Pool admin credentials are different and must never be accepted by the runtime-facing API.

Request metadata should allow CodeLocal to pass non-secret routing hints:

```text
model=auto | explicit-model
purpose=coding | reasoning | fast | cheap
requestId
ownerId
workspaceId (optional for metrics only)
```

The pool must not need a workspace path or local device secret.

---

# 7. `model=auto` evolution

Current CodeLocal product direction already favors `auto` as the default model experience.

Pool routing can make `auto` meaningful at three levels:

```text
AUTO
  -> provider
      -> model
          -> account
```

Phase 1:

```text
auto -> Codex -> best healthy Codex account
```

Phase 2:

```text
coding/reasoning task
  -> capability policy
  -> provider/model
  -> healthy account
```

Phase 3:

```text
quality/cost/latency policy
  -> provider/model/account
  -> adaptive historical scoring
```

Do not put these routing rules into the browser UI. The server is authoritative.

---

# 8. Admin / owner UI

This is an owner-only operational dashboard for the initial version.

Minimum pages:

## Accounts

```text
Codex Pool                                      842 / 1000 healthy

#001  HEALTHY       quota 82%   active 1   latency 1.8s
#002  COOLDOWN      reset 14m   active 0
#003  AUTH_REQUIRED              active 0
...
```

Actions:

```text
Add account
Reconnect
Validate
Pause
Resume
Disable
Remove
Bulk import/connect helper (later)
```

## Pool health

```text
healthy accounts
available concurrency
requests/min
success rate
p50 / p95 first-token latency
rate limits
refresh failures
account exhaustion distribution
```

## Request explorer

Metadata only by default; no prompt contents.

---

# 9. Account connection and onboarding

For the first spike, use a deliberately controlled owner flow.

```text
Admin -> Add Codex Account
      -> provider authorization/connect flow
      -> callback/import into Pool
      -> immediate credential validation
      -> model/capability probe
      -> save encrypted credential
      -> account becomes HEALTHY
```

Do not build a public self-service contributor onboarding flow.

For large-scale onboarding (~1000 accounts), design a separate bulk operations helper only after one-account connection is stable. Bulk tooling must reuse the same validated credential-storage path rather than writing directly to database tables.

---

# 10. Failure and retry policy

A single CodeLocal request may try more than one account only when the failure class is safe to retry.

Safe examples:

```text
RATE_LIMITED
QUOTA_EXHAUSTED before useful response
TRANSIENT_UPSTREAM before useful response
connection failure before stream starts
```

Do not transparently replay a request after meaningful streamed output unless the protocol supports deterministic continuation. Otherwise CodeLocal may receive duplicated tool calls/text.

Recommended request path:

```text
request
  |
  v
select account A
  |
  v
execute
  |
  +-- success -> stream + complete
  |
  +-- safe failure -> mark A/cooldown -> select B -> retry
  |
  +-- unsafe/partial failure -> return normalized error
```

Default retry budget should be small (for example 2 alternate accounts), configurable by provider/error class.

1000 accounts must not mean 1000 retries.

---

# 11. Security boundaries

Hard requirements:

- CodeLocal native runtime never receives provider refresh credentials;
- browser never receives decrypted provider credentials;
- normal application logs must redact `Authorization`, cookies, OAuth codes and credential payloads;
- database dumps alone must not be sufficient to recover provider credentials;
- Pool service key must be rotatable;
- Admin auth and request-gateway auth are separate;
- callback state/PKCE/nonces are validated where applicable;
- credential reads are minimal and audited;
- no plaintext export endpoint in production;
- internal debugging tools return credential fingerprints/versions, not secrets;
- account deletion revokes or deletes stored credentials where the provider flow permits.

Threat model must include:

```text
DB leak
Redis leak
application log leak
admin session theft
SSRF toward metadata/internal services
malicious upstream response
credential refresh race
accidental secret rendering in UI
```

---

# 12. Capacity model for ~1000 accounts

Do not keep 1000 active refresh timers or 1000 permanent connections.

Use lazy/event-driven state:

```text
PostgreSQL -> durable account state
Redis      -> hot availability / leases / cooldown
workers    -> stateless request execution
refresh    -> on-demand + bounded proactive refresh queue
```

Recommended worker behavior:

- stateless horizontal replicas;
- bounded concurrency per worker;
- account-level distributed concurrency limit;
- no in-memory account authority;
- graceful stream drain during deploy;
- cancellation propagates upstream.

The pool should scale by request concurrency, not account count.

---

# 13. Database / Redis separation

## PostgreSQL — durable

```text
provider_accounts
provider_credentials_encrypted
provider_credential_versions
pool_requests
account_usage_rollups
account_health_events
admin_audit_events
```

## Redis — ephemeral/hot

```text
account leases
refresh locks
cooldowns cache
rate-limit windows
health score cache
routing candidate cache
request ownership
```

Never make Redis the only copy of a credential.

---

# 14. Observability

Metrics:

```text
pool_requests_total
pool_requests_inflight
pool_request_duration_seconds
pool_first_token_seconds
pool_retries_total
pool_account_selection_total
pool_account_rate_limited_total
pool_account_auth_failure_total
pool_refresh_total
pool_refresh_failure_total
pool_accounts_by_status
pool_available_concurrency
```

Logs correlate by `requestId` and `providerAccountId`, but never contain provider secret material.

Add dashboards/alerts for:

- sudden global auth failure;
- >X% accounts rate limited;
- refresh failure spike;
- pool success rate drop;
- no healthy accounts for requested model;
- repeated selection imbalance;
- Redis/PostgreSQL degradation.

---

# 15. Repository/service boundary

Long-term recommendation:

```text
CodeLocal repository
  -> provider-neutral pool client
  -> config / health integration
  -> owner UI link/embed if desired

CodeLocal Pool service
  -> provider credentials
  -> scheduler
  -> provider adapters
  -> AI streaming gateway
```

Do not copy the entire 9Router application into the CodeLocal web/backend tree.

For the spike, 9Router may be deployed/read as a behavior reference to validate:

- Codex connect behavior;
- credential refresh lifecycle;
- request format translation;
- model mapping;
- failure semantics.

The production service should preserve a stable internal CodeLocal API contract regardless of whether an adapter implementation is initially ported, wrapped or rewritten.

---

# 16. Implementation phases

## Phase 0 — 9Router compatibility spike

Goal: prove the provider flow before building the full pool.

- [ ] Pin the exact 9Router source/release/commit used for research.
- [ ] Document Codex connect/auth artifacts required at runtime.
- [ ] Document refresh-token rotation behavior.
- [ ] Capture `/v1/responses` request/stream/error behavior.
- [ ] Test 2–5 owner-controlled accounts.
- [ ] Verify account fallback behavior.
- [ ] Identify which code/patterns can be safely adapted and which must be rewritten.
- [ ] Produce a compatibility test fixture independent of 9Router.

Exit criteria:

```text
We can connect multiple test accounts,
execute Codex requests server-side,
refresh credentials,
and classify rate-limit/auth failures reliably.
```

---

## Phase 1 — Pool MVP

- [ ] Create separate Pool service skeleton.
- [ ] Add PostgreSQL migrations for account registry/vault metadata.
- [ ] Add encrypted credential vault.
- [ ] Add Codex provider adapter.
- [ ] Add owner-only Add/Reconnect/Disable account flow.
- [ ] Add `/v1/responses` streaming endpoint.
- [ ] Add service-to-service auth from CodeLocal Cloud.
- [ ] Add simple healthy-account scheduler.
- [ ] Add account concurrency lease.
- [ ] Add normalized errors.
- [ ] Add request/usage metadata.
- [ ] Integrate CodeLocal `auto` -> Pool.

Exit criteria:

```text
CodeLocal can call one internal endpoint
and transparently use a small multi-account Codex pool
without a local provider credential.
```

---

## Phase 2 — Production scheduler

- [ ] Health scores.
- [ ] Cooldown/circuit breaker.
- [ ] quota snapshots/estimation.
- [ ] refresh coordinator and credential versioning.
- [ ] weighted selection.
- [ ] safe retry budget.
- [ ] cancellation propagation.
- [ ] metrics and operational dashboard.
- [ ] owner account management UI.
- [ ] load test with synthetic 1000-account registry.

Exit criteria:

```text
The pool remains stable under concurrent load,
does not over-select unhealthy accounts,
and survives worker restarts without losing account correctness.
```

---

## Phase 3 — Scale to large owner pool

- [ ] Efficient bulk account operations.
- [ ] background validation queue.
- [ ] lazy proactive refresh queue.
- [ ] account grouping/tags.
- [ ] per-model capacity forecast.
- [ ] operational alerts.
- [ ] backup/restore procedure for encrypted credential records.
- [ ] disaster recovery test.
- [ ] credential/key rotation runbook.

Exit criteria:

```text
~1000 registered accounts can be operated without
1000 permanent workers/connections and without manual per-account monitoring.
```

---

## Phase 4 — Provider-neutral Auto Router

Only after Codex pool stability:

- [ ] add provider capability registry;
- [ ] add additional provider adapters;
- [ ] choose provider + model + account from `auto`;
- [ ] configurable quality/latency/cost policies;
- [ ] provider-specific kill switches;
- [ ] fallback policy that never silently changes task semantics.

---

# 17. Testing strategy

Unit tests:

- scheduler candidate filtering;
- scoring;
- retry classification;
- credential version CAS;
- refresh lock races;
- cooldown transitions;
- secret redaction.

Integration tests:

- two workers competing for the same account;
- credential rotation while requests are active;
- Redis restart;
- PostgreSQL reconnect;
- upstream 401/429/5xx;
- stream cancellation;
- partial stream failure;
- all accounts exhausted;
- one model unavailable on subset of accounts.

Scale tests should generate 1000 synthetic account records and realistic concurrent request patterns. Do not require 1000 live provider accounts to validate scheduler correctness.

---

# 18. Rollout

Recommended rollout order:

```text
1 account
  -> 5 accounts
    -> 20 accounts
      -> 100 synthetic/20 live
        -> 1000 synthetic
          -> expand live pool gradually
```

Feature flags:

```text
CODELOCAL_AI_POOL_ENABLED
CODELOCAL_AI_POOL_CODEX_ENABLED
CODELOCAL_AI_POOL_AUTO_ROUTING_ENABLED
CODELOCAL_AI_POOL_FALLBACK_ENABLED
```

CodeLocal must retain an emergency fallback/provider path during early rollout so a Pool outage does not block all development work.

---

# 19. Definition of done

The first production version is done when:

- [ ] CodeLocal calls one Pool endpoint only;
- [ ] provider credentials exist only in the encrypted Pool vault;
- [ ] owner can connect/reconnect/disable accounts from an admin flow;
- [ ] requests are assigned only to healthy eligible accounts;
- [ ] per-account concurrency is enforced across replicas;
- [ ] refresh-token rotation is atomic;
- [ ] 401/429/quota/5xx are classified and handled correctly;
- [ ] safe failover works without duplicate streamed output;
- [ ] pool metrics expose capacity and failure state;
- [ ] no provider credential appears in browser/runtime/log telemetry;
- [ ] a 1000-account synthetic load test passes;
- [ ] provider adapter can be disabled independently;
- [ ] CodeLocal core remains usable if Pool is disabled.

---

# 20. Immediate implementation order

When implementation starts, use this exact sequence:

```text
1. Freeze 9Router research commit/release
2. Build compatibility tests for Codex request/auth/refresh behavior
3. Create CodeLocal Pool service boundary
4. Implement Vault + ProviderAccount schema
5. Implement Codex adapter
6. Implement single-account /v1/responses
7. Add multi-account leases + scheduler
8. Add refresh coordinator
9. Add error classification + cooldown + retry
10. Connect CodeLocal model=auto to Pool behind feature flag
11. Add owner dashboard
12. Load/chaos/security test
13. Gradually increase account count
14. Add other providers only after Codex pool is stable
```

Do not begin with the 1000-account UI, bulk importer or advanced AI routing. The hard correctness problems are credential lifecycle, request leases, refresh races, safe retry and streaming behavior; solve those first.

---

# Final recommendation

**Build Case B: a separate private CodeLocal Pool service.**

Use 9Router as a reference/spike for Codex provider behavior, but keep CodeLocal ownership of:

- credential security;
- distributed scheduling;
- health/cooldown;
- concurrency control;
- usage accounting;
- service authentication;
- the stable internal API contract.

This gives CodeLocal one clean AI endpoint today while preserving the option to support many providers and thousands of owner-controlled accounts later without coupling provider internals to the MCP/local-runtime architecture.
