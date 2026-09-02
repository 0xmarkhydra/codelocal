# CodeLocal Central AI Account Pool — Master Plan

Status: **Selected architecture — implementation in progress**

Date: **2026-09-02**

Owner: **CodeLocal**

Scope: **Private central provider-account pool controlled by the CodeLocal owner**

This document is the source of truth for CodeLocal AI Pool v1.

## 1. Product decision

CodeLocal will **not reimplement provider OAuth, refresh, quota, account rotation or combo fallback in Go for v1**.

The production shape is a dedicated self-hosted 9Router deployment at `pool.codelocal.cloud`. Provider accounts are connected directly to that website. CodeLocal Cloud treats the Pool as one private OpenAI-compatible provider.

```text
Owner-controlled accounts
  ├── Codex / OpenAI accounts
  ├── Claude / Anthropic accounts
  ├── Gemini / Google accounts
  ├── API-key providers
  └── other 9Router-supported providers
             |
             v
      pool.codelocal.cloud
           9Router
  ├── provider connections
  ├── OAuth/API credentials
  ├── refresh lifecycle
  ├── quota / cooldown
  ├── combos / fallback
  └── OpenAI-compatible /v1 gateway
             |
             | dedicated endpoint API key
             v
        CodeLocal Cloud
             |
       model=auto / selected model
             |
             v
      CodeLocal chat/agent runtime
```

Provider credentials are not copied into CodeLocal's database and are never sent to the local CodeLocal runtime.

## 2. BA — three cases

### Case A — Self-host 9Router as the Pool control plane — SELECTED

Use the upstream 9Router runtime as a separate deployment and add only a thin CodeLocal integration layer.

Advantages:

- fastest path to a working central account pool;
- provider OAuth/API-key management already exists;
- account refresh, quota and fallback behavior remain inside the router that owns those provider connections;
- CodeLocal stays provider-agnostic;
- a 9Router upgrade can add/fix provider behavior without rewriting CodeLocal Cloud.

Tradeoffs:

- CodeLocal depends on a third-party runtime and its persistence format;
- release upgrades require compatibility tests;
- the Pool dashboard is a separate administrative surface;
- CodeLocal must treat the Pool as a hard security boundary because provider credentials live there.

Decision: **Use Case A for v1.**

### Case B — Build a separate native CodeLocal Pool service

Reimplement credential vault, provider adapters, refresh, scheduler and fallback in CodeLocal-owned Go services.

Advantages: maximum control and custom scaling.

Disadvantages: duplicates a large amount of existing 9Router behavior, increases security surface, slows delivery, and requires continuous provider-maintenance work.

Decision: **Deferred. Reconsider only if 9Router becomes a proven product bottleneck.**

### Case C — Distributed local contributor workers

Keep provider sessions on contributor laptops and dispatch jobs back to those machines.

Advantages: provider credential can stay on the contributor machine.

Disadvantages: contributor must stay online, prompt privacy becomes harder, scheduling is more complex, and this is not the requested product.

Decision: **Rejected for this product.**

## 3. CodeLocal integration contract

CodeLocal Cloud requires only:

```text
CODELOCAL_AI_POOL_BASE_URL=https://pool.codelocal.cloud/v1
CODELOCAL_AI_POOL_API_KEY=<dedicated 9Router endpoint key>
CODELOCAL_AI_POOL_MODEL=<exact 9Router model or combo id>
CODELOCAL_AI_POOL_DASHBOARD_URL=https://pool.codelocal.cloud
```

Rules:

1. `CODELOCAL_AI_POOL_API_KEY` is server-only and must never be exposed to Next.js.
2. `CODELOCAL_AI_POOL_MODEL` is the first Pool route for `model=auto`.
3. Pool model discovery comes from authenticated `GET /v1/models`.
4. Explicit provider-qualified model IDs such as `cc/...` and combo IDs route through the Pool.
5. A temporary Pool outage must not break the model picker or remove existing fallback providers.
6. A Pool route is private/trusted, not a community/free provider; existing workspace/tool context can use it.

## 4. Routing

`auto` becomes:

```text
1. CodeLocal AI Pool default model/combo, when configured
2. Existing ShopAIKey route, when configured
3. Existing GLM free route for community-eligible requests
4. Existing Qwen free route for community-eligible requests
5. Existing Muse route
6. Existing generic fallback
```

The important separation is:

- CodeLocal decides whether to call the Pool and which top-level model/combo ID to request.
- 9Router decides which provider account handles that request and performs account-level fallback/quota/cooldown.

CodeLocal must not duplicate 9Router's account scheduler.

## 5. Pool deployment boundary

Pool is deployed as a dedicated Railway service using `Dockerfile.pool` and `railway.pool.json`.

Required production properties:

- pin an explicit 9Router image version;
- mount persistent storage at `/app/data`;
- expose a dedicated `pool.codelocal.cloud` domain;
- use strong `JWT_SECRET`, `INITIAL_PASSWORD`, `API_KEY_SECRET` and `MACHINE_ID_SALT` values;
- keep request logging off unless explicitly required for debugging;
- keep 9Router Cloud Sync disabled for the private self-hosted Pool;
- require endpoint API-key authentication in the 9Router dashboard;
- create a dedicated endpoint key only for CodeLocal Cloud;
- do not reuse the Pool admin password as the API key.

See `docs/operations/AI_POOL_9ROUTER.md`.

## 6. Dashboard

`/dashboard/pool` is a CodeLocal status/integration page, not a second credential manager.

It exposes only:

- configured / available / routing-ready state;
- discovered model count;
- default model/combo ID;
- bounded model list;
- link to the separate Pool dashboard;
- setup/security boundary documentation.

It never returns:

- provider OAuth tokens;
- provider API keys;
- Pool endpoint API key;
- 9Router admin credentials;
- raw upstream error bodies.

Provider connections are managed on `pool.codelocal.cloud` itself.

## 7. Upgrade strategy

Treat 9Router as an external infrastructure dependency.

Before changing the pinned version:

1. review upstream security advisories and release notes;
2. boot the candidate image against a temporary copied data volume, never the production volume first;
3. verify `/api/health`;
4. verify authenticated `/v1/models`;
5. run one non-streaming and one streaming chat request through the configured Combo;
6. verify Codex/Claude/Gemini provider connections still refresh and route;
7. verify endpoint API-key enforcement from an unauthenticated client;
8. roll forward only after those checks pass.

Rollback is the previous pinned image plus the same persistent volume, subject to upstream data-migration compatibility.

## 8. Security model

The Pool contains high-value provider credentials. Therefore:

- deploy it separately from the main CodeLocal Cloud service;
- never mount its `/app/data` volume into CodeLocal Cloud;
- never proxy or serialize provider tokens through CodeLocal APIs;
- use HTTPS for the public Pool domain;
- make the CodeLocal endpoint key revocable independently;
- rotate the CodeLocal endpoint key if exposed;
- back up the encrypted/provider data according to upstream persistence semantics;
- use provider-specific kill switches by disabling connections/combos in the Pool dashboard;
- do not treat environment flags as proof of endpoint-key enforcement: verify enforcement with an actual unauthorized request after each deployment.

## 9. Implementation phases

### Phase 1 — Foundation

- [x] Create a clean feature branch from `origin/main`.
- [x] Add thin AI Pool config/target adapter to CodeLocal Cloud.
- [x] Put Pool first in `model=auto` when a default model/combo is configured.
- [x] Add authenticated `/v1/models` discovery with bounded caching.
- [x] Add authenticated CodeLocal `/api/v1/pool` status resource.
- [x] Add `Dockerfile.pool` and Railway service definition.

### Phase 2 — Browser integration

- [x] Add Dashboard → AI Pool navigation.
- [x] Add live Pool status page.
- [x] Show discovered models and current auto default.
- [x] Link to the Pool dashboard without exposing its API key.

### Phase 3 — Verification

- [ ] Go formatting and package tests.
- [ ] Full Go test suite.
- [ ] Web lint/typecheck/build/audit.
- [ ] `git diff --check`.
- [ ] Review final diff for secrets and old distributed-worker code.
- [ ] Commit and push only `feat/codelocal-ai-pool-9router`.

### Phase 4 — Deployment

- [ ] Create Railway Pool service from `railway.pool.json`.
- [ ] Attach persistent `/app/data` volume.
- [ ] Configure `pool.codelocal.cloud`.
- [ ] Add production secrets.
- [ ] Connect owner-controlled provider accounts in 9Router.
- [ ] Create the production Combo/default model.
- [ ] Enable Endpoint → Require API Key and create the CodeLocal endpoint key.
- [ ] Configure CodeLocal Cloud Pool envs.
- [ ] Smoke-test `/api/health`, `/v1/models`, chat streaming and CodeLocal `auto`.

## 10. Non-goals for v1

- no public contributor marketplace;
- no local contributor daemon;
- no credential upload through CodeLocal Dashboard;
- no CodeLocal-owned provider OAuth implementation;
- no reimplementation of 9Router's scheduler/combos/quota system;
- no Pool API key in browser JavaScript;
- no claim that a provider subscription can be redistributed contrary to that provider's current terms.

## 11. Success criteria

AI Pool v1 is complete when:

1. provider accounts can be administered centrally at `pool.codelocal.cloud`;
2. CodeLocal can discover Pool models without seeing provider credentials;
3. `model=auto` can use a configured 9Router Combo as its first private route;
4. an explicit Pool model can be selected and routed without ShopAIKey hijacking it;
5. Pool outage degrades cleanly to existing CodeLocal routes;
6. the Pool endpoint rejects requests without a valid endpoint API key;
7. the pinned deployment can be upgraded/rolled back with a documented procedure.
