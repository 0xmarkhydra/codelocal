# CodeLocal Central AI Account Pool — Master Plan

Status: **Canonical Pool implemented — Railway deployment in progress**

Date: **2026-09-03**

Owner: **CodeLocal**

Scope: **Private canonical AI model pool controlled by the CodeLocal owner**

This document is the source of truth for CodeLocal AI Pool v1.

## 1. Product decision

CodeLocal owns a **canonical Pool API** but does **not** reimplement provider OAuth, refresh or provider-session lifecycle that 9Router already owns.

Production is split into three boundaries:

```text
CodeLocal clients / CodeLocal Cloud
             |
             | canonical model IDs + Pool API key
             v
      pool.codelocal.cloud
          Pool Web
  ├── operator login/admin UI
  └── public OpenAI-compatible /v1 gateway
             |
             | Railway private network
             v
          Pool API
  ├── canonical model registry
  ├── Active / Exhausted / Degraded state
  ├── source priority + failover
  └── encrypted BYOK source metadata
          |             |
          |             └── owner BYOK OpenAI-compatible sources
          v
        9Router
  ├── provider connections
  ├── OAuth/API credentials
  ├── refresh lifecycle
  └── account-level quota/cooldown/fallback
```

9Router remains the provider execution engine. Pool API is the canonical routing/control layer. Provider session credentials owned by 9Router are never copied into CodeLocal's database or sent to the local CodeLocal runtime.

## 2. BA — three cases

### Case A — Canonical CodeLocal Pool in front of self-hosted 9Router — SELECTED

Run 9Router as a separate upstream execution engine and place a small CodeLocal-owned canonical Pool API/Web boundary in front of it.

Advantages:

- canonical model IDs are owned by CodeLocal rather than provider/router prefixes;
- 9Router continues to own provider OAuth/session lifecycle;
- Pool API can combine 9Router with owner BYOK sources;
- Pool can expose Active/Exhausted state independently from raw provider IDs;
- source-level failover can evolve without changing the client contract;
- 9Router upgrades can add/fix provider behavior without rewriting CodeLocal Cloud.

Tradeoffs:

- 9Router remains an external infrastructure dependency;
- Pool Web is a separate administrative surface;
- Pool API, Pool Web and 9Router are hard security boundaries;
- release upgrades require compatibility and routing tests.

Decision: **Use Case A for v1.**

### Case B — Replace 9Router with native provider adapters

Move provider OAuth/session refresh/provider-specific execution entirely into CodeLocal-owned services.

Advantages: maximum control and custom scaling.

Disadvantages: duplicates substantial 9Router provider behavior, increases security surface, slows delivery, and requires continuous provider-maintenance work.

Decision: **Deferred. Reconsider only if 9Router becomes a proven product bottleneck.**

### Case C — Distributed local contributor workers

Keep provider sessions on contributor laptops and dispatch jobs back to those machines.

Advantages: provider credentials can stay on contributor machines.

Disadvantages: contributors must stay online, prompt privacy becomes harder, scheduling is more complex, and this is not the requested product.

Decision: **Rejected for this product.**

## 3. CodeLocal client contract

CodeLocal Cloud requires only:

```text
CODELOCAL_AI_POOL_BASE_URL=https://pool.codelocal.cloud/v1
CODELOCAL_AI_POOL_API_KEY=<dedicated Pool client key>
CODELOCAL_AI_POOL_MODEL=<canonical model id, for example gpt-5.6-sol>
```

Rules:

1. `CODELOCAL_AI_POOL_API_KEY` is server-only and must never be exposed to browser JavaScript.
2. `CODELOCAL_AI_POOL_MODEL` is the first Pool route for `model=auto` and is always canonical.
3. Pool model discovery comes from authenticated `GET /v1/models`.
4. `GET /v1/models` returns only Active canonical models.
5. Provider prefixes such as `cx/`, `cc/` and `gc/` stay behind Pool and are not part of the client contract.
6. A temporary Pool outage must not break the model picker or remove existing fallback providers.
7. Pool is private/trusted, not a community/free provider; existing workspace/tool context can use it.

## 4. Routing

`auto` becomes:

```text
1. CodeLocal AI Pool canonical default model, when configured and Active
2. Existing ShopAIKey route, when configured
3. Existing GLM free route for community-eligible requests
4. Existing Qwen free route for community-eligible requests
5. Existing Muse route
6. Existing generic fallback
```

The separation is:

- CodeLocal clients choose `auto` or a canonical model ID.
- Pool API maps that canonical model to eligible sources and performs source-level failover.
- 9Router, when selected as a source, decides which provider account executes the request and performs its own account-level fallback/quota/cooldown.
- BYOK OpenAI-compatible sources can sit beside 9Router with explicit priority.

Pool must not leak provider-qualified IDs into the client model contract or duplicate 9Router's provider-account scheduler.

## 5. Railway deployment boundary

Railway uses three independent service definitions:

- **9Router:** `Dockerfile.9router` + `railway.9router.json`;
- **Pool API:** `Dockerfile.pool-api` + `railway.pool-api.json`;
- **Pool Web:** `Dockerfile.pool-web` + `railway.pool-web.json`.

Production properties:

- pin an explicit 9Router image version;
- preserve 9Router persistent storage at `/app/data`;
- use Railway private networking for Pool API → 9Router;
- use Railway private networking for Pool Web → Pool API;
- expose `pool.codelocal.cloud` from Pool Web, not directly from 9Router;
- do not delete or recreate the 9Router service/volume during cutover;
- use independent `POOL_API_KEY`, `POOL_ADMIN_TOKEN`, `POOL_ENCRYPTION_KEY`, `POOL_WEB_PASSWORD` and `POOL_WEB_SESSION_SECRET` values;
- keep `POOL_ADMIN_TOKEN` server-side;
- keep 9Router provider/session credentials behind the 9Router boundary;
- verify unauthorized requests are rejected after every deployment.

Pool API environment contract:

```text
PORT=8080
POOL_API_KEY=<client key>
POOL_ADMIN_TOKEN=<separate admin key>
POOL_ENCRYPTION_KEY=<32-byte key>
POOL_9ROUTER_BASE_URL=http://<9router-private-domain>:20128/v1
POOL_9ROUTER_API_KEY=<9Router endpoint key>
POOL_9ROUTER_PRIORITY=100
DATABASE_URL=<PostgreSQL URL when durable BYOK storage is enabled>
```

Pool Web environment contract:

```text
POOL_API_BASE_URL=http://<pool-api-private-domain>:8080
POOL_ADMIN_TOKEN=<reference to Pool API admin token>
POOL_WEB_PASSWORD=<operator password>
POOL_WEB_SESSION_SECRET=<at least 32 bytes>
```

## 6. Pool Web

`pool.codelocal.cloud` is a standalone website, separate from the main CodeLocal dashboard.

Its authenticated operator UI exposes:

- canonical model Active/Exhausted/Degraded state;
- eligible source counts and source health;
- configured routing sources;
- encrypted BYOK source create/enable/disable/delete operations.

Its public gateway exposes only:

- `GET /v1/models`;
- `POST /v1/chat/completions`;
- `POST /v1/responses`.

Security boundary:

- `POOL_ADMIN_TOKEN` exists only in server-side Pool Web code;
- operator sessions are signed with HMAC-SHA256;
- session cookies are HttpOnly and SameSite=Strict, and Secure in production;
- `POOL_WEB_SESSION_SECRET` must be at least 32 bytes;
- admin mutations enforce same-origin checks;
- gateway request size and admin response size are bounded;
- only safe request/response headers are proxied.

Provider OAuth/session credentials remain inside 9Router.

## 7. 9Router upgrade strategy

Treat 9Router as an external infrastructure dependency.

Before changing the pinned version:

1. review upstream security advisories and release notes;
2. boot the candidate image against a temporary copied data volume, never the production volume first;
3. verify `/api/health`;
4. verify authenticated `/v1/models` through 9Router;
5. run one non-streaming and one streaming request;
6. verify Codex/Claude/Gemini provider connections still refresh and route;
7. verify 9Router endpoint API-key enforcement;
8. verify Pool API canonical discovery/routing against the candidate;
9. roll forward only after those checks pass.

Rollback is the previous pinned 9Router image plus the same persistent volume, subject to upstream data-migration compatibility.

## 8. Security model

The stack contains high-value credentials. Therefore:

- deploy Pool Web, Pool API and 9Router as separate service boundaries;
- never mount 9Router `/app/data` into CodeLocal Cloud or Pool Web;
- never serialize 9Router provider tokens through Pool APIs;
- use HTTPS for the public Pool domain;
- make client and admin keys independently revocable;
- encrypt owner BYOK credentials at rest in Pool API;
- rotate affected keys if exposed;
- use provider-specific kill switches inside 9Router where appropriate;
- do not treat environment flags as proof of authentication: verify with actual unauthorized requests.

## 9. Implementation status

### Phase 1 — Canonical Pool core

- [x] Add CodeLocal Pool provider integration to CodeLocal Cloud.
- [x] Put Pool first in `model=auto` when configured.
- [x] Add canonical model registry and Active/Exhausted state.
- [x] Add source-level priority/failover.
- [x] Add encrypted BYOK source persistence.
- [x] Add OpenAI-compatible `/v1/models`, `/v1/chat/completions` and `/v1/responses`.
- [x] Keep 9Router as an upstream execution source.

### Phase 2 — Standalone Pool Web

- [x] Remove Pool control plane from the main CodeLocal dashboard.
- [x] Add standalone `web/apps/pool` website.
- [x] Add operator login and secure session cookie.
- [x] Add canonical model/source status UI.
- [x] Add BYOK source management UI.
- [x] Add strict public `/v1/*` gateway allowlist.

### Phase 3 — Verification

- [x] Go formatting and targeted Pool/Cloud tests.
- [x] Full Go test/vet suite.
- [x] Main Web lint/typecheck/build/audit.
- [x] Pool Web lint/typecheck/build/audit.
- [x] `git diff --check`.
- [x] Audit reports zero production vulnerabilities for both web apps.

### Phase 4 — Railway deployment

- [ ] Land verified feature on `main`.
- [ ] Reuse/link existing 9Router service and preserve its persistent volume.
- [ ] Create/configure Pool API service.
- [ ] Create/configure Pool Web service.
- [ ] Connect Pool API to 9Router over Railway private network.
- [ ] Connect Pool Web to Pool API over Railway private network.
- [ ] Configure `pool.codelocal.cloud` after health checks pass.
- [ ] Configure CodeLocal Cloud Pool envs.
- [ ] Smoke-test health, unauthorized access, authenticated model discovery and an actual model request.

## 10. Non-goals for v1

- no public contributor marketplace;
- no local contributor daemon;
- no provider OAuth implementation duplicated inside CodeLocal Pool;
- no provider-qualified model IDs in the client contract;
- no Pool admin key in browser JavaScript;
- no claim that a provider subscription can be redistributed contrary to that provider's current terms.

## 11. Success criteria

AI Pool v1 is complete when:

1. `pool.codelocal.cloud` exposes the standalone Pool Web/API gateway;
2. CodeLocal discovers only Active canonical models without seeing provider credentials;
3. `model=auto` can use the configured canonical Pool model as its first private route;
4. Pool API can route/fail over between 9Router and owner BYOK sources;
5. exhausted upstream standard models disappear from Active discovery without leaking provider prefixes;
6. Pool outage degrades cleanly to existing CodeLocal routes;
7. Pool client endpoints reject requests without a valid client API key;
8. Pool admin endpoints reject requests without the separate admin token;
9. 9Router remains independently upgradeable with its persistent data preserved;
10. Railway health checks and a real authenticated model request pass after deployment.
