# CodeLocal AI Pool — 9Router Railway Runbook

Date: 2026-09-02

Target domain: `pool.codelocal.cloud`

Deployment: dedicated Railway service

Runtime: pinned 9Router container

## 1. What this service owns

AI Pool is the central provider-account control plane. Connect Codex, Claude, Gemini and API-key providers directly in its 9Router dashboard.

CodeLocal Cloud does **not** store those upstream credentials. It only stores a dedicated Pool endpoint API key and calls the Pool's OpenAI-compatible `/v1` endpoint.

## 2. Railway service

Use:

```text
Dockerfile.pool
railway.pool.json
```

The Dockerfile pins the exact 9Router image version. Do not deploy `latest` to production.

Attach a persistent Railway volume at:

```text
/app/data
```

Losing this volume can lose the self-hosted router's persisted provider configuration/session state.

## 3. Pool environment

Configure strong values in the Pool Railway service, not in source control:

```text
PORT=20128
HOSTNAME=0.0.0.0
NODE_ENV=production
DATA_DIR=/app/data

JWT_SECRET=<strong random value>
INITIAL_PASSWORD=<strong one-time/admin password>
API_KEY_SECRET=<strong random value>
MACHINE_ID_SALT=<strong random value>

ENABLE_REQUEST_LOGS=false
AUTH_COOKIE_SECURE=true
```

`REQUIRE_API_KEY=true` may be set as defense-in-depth/documentation, but deployment must not rely on that environment value alone. The authoritative verification is the runtime dashboard setting plus a real unauthenticated request test.

Keep optional 9Router Cloud Sync disabled for this private self-hosted service unless a later architecture decision explicitly enables it.

## 4. Domain

Map the Railway service to:

```text
pool.codelocal.cloud
```

Production CodeLocal must use HTTPS:

```text
https://pool.codelocal.cloud/v1
```

## 5. First boot

After health is green:

1. Open `https://pool.codelocal.cloud`.
2. Sign in with the initial/admin credential.
3. Change/secure the admin credential if the 9Router version provides that workflow.
4. Connect the owner-controlled Codex/OpenAI accounts.
5. Connect the owner-controlled Claude/Anthropic accounts.
6. Connect Gemini/Google and any API-key providers needed by CodeLocal.
7. Confirm account health/quota in 9Router before exposing the gateway to CodeLocal.

Do not paste provider secrets into the CodeLocal Dashboard.

## 6. Build the CodeLocal default route

Create a 9Router Combo for CodeLocal `auto` routing.

Example intent only:

```text
CodeLocal Auto Combo
  1. preferred Claude/Codex account/model
  2. next healthy account/model
  3. next fallback
  ...
```

Use the exact Combo/model ID displayed by the deployed 9Router instance. Do not guess or hardcode an ID from documentation.

The Combo should own account-level fallback. CodeLocal should not duplicate its account scheduler.

## 7. Require an endpoint API key

In the 9Router dashboard:

```text
Endpoint
  → Require API Key = ON
```

Then create a dedicated key for CodeLocal under the router's API-key management UI.

Rules:

- do not reuse the Pool admin password;
- do not use a provider API key as the CodeLocal Pool key;
- do not expose the key to the browser;
- rotate/revoke it independently if leaked.

### Mandatory enforcement test

Before CodeLocal is configured, test both paths from outside the Pool service:

```text
GET /v1/models without Authorization  -> must be rejected
GET /v1/models with CodeLocal key      -> must succeed
```

A deployment is not production-ready until this test passes.

## 8. Configure CodeLocal Cloud

Set these only on the CodeLocal Cloud service:

```text
CODELOCAL_AI_POOL_BASE_URL=https://pool.codelocal.cloud/v1
CODELOCAL_AI_POOL_API_KEY=<dedicated 9Router endpoint key>
CODELOCAL_AI_POOL_MODEL=<exact Pool Combo/model ID>
CODELOCAL_AI_POOL_DASHBOARD_URL=https://pool.codelocal.cloud
```

Restart/redeploy CodeLocal Cloud after the variables are applied.

Expected behavior:

- `/api/v1/pool` shows configured and available;
- Dashboard → AI Pool lists models from authenticated `/v1/models`;
- configured default Combo/model becomes the first `model=auto` route;
- provider-qualified Pool model IDs route back through Pool;
- existing provider routes remain available if Pool is down.

## 9. Smoke test

Run in this order:

### Pool health

```text
GET https://pool.codelocal.cloud/api/health
```

### Endpoint auth

Verify unauthenticated `/v1/models` is rejected.

### Model catalog

Use the dedicated CodeLocal endpoint key against:

```text
GET https://pool.codelocal.cloud/v1/models
```

Confirm the production Combo/default model appears.

### Chat

Send one minimal non-streaming request through the Combo and confirm a provider account is selected successfully.

Then send one streaming request and confirm the stream completes cleanly.

### CodeLocal

Open:

```text
/dashboard/pool
```

Confirm:

- Gateway = Online
- Models discovered > 0
- Auto route = expected Combo/model

Finally send a CodeLocal chat request with model `Auto` and verify Pool is the first route.

## 10. Upgrade procedure

Never change the 9Router production image tag without a compatibility pass.

1. Read current upstream release notes/security advisories.
2. Change `NINEROUTER_VERSION` only in a test branch/deployment first.
3. Test against a disposable or copied Pool data volume.
4. Verify health, login, endpoint-key enforcement, `/v1/models`, provider connections, Combo routing and streaming.
5. Back up production persistence as appropriate for the installed version.
6. Deploy the pinned candidate version.
7. Repeat the smoke test.

If a regression occurs, restore the previous image version only when its persistence format remains compatible with the current data volume.

## 11. Security checklist

Before production:

- [ ] Pool runs as a separate Railway service.
- [ ] 9Router version is explicitly pinned.
- [ ] `/app/data` uses a persistent volume.
- [ ] HTTPS domain is configured.
- [ ] Strong admin/runtime secrets are configured.
- [ ] Request logs are disabled unless intentionally needed.
- [ ] Pool Cloud Sync is disabled.
- [ ] Endpoint → Require API Key is ON.
- [ ] Unauthenticated `/v1/models` is verified rejected.
- [ ] Dedicated CodeLocal endpoint key exists.
- [ ] CodeLocal Cloud has the Pool key only server-side.
- [ ] Provider OAuth/API credentials exist only inside the Pool boundary.
- [ ] Default Combo/model is configured and smoke-tested.
