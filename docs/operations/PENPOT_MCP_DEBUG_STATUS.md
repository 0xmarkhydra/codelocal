# Penpot MCP Debug Status

Date: 2026-09-08

Status: **incomplete; do not treat this patch as an end-to-end or security sign-off.**

## Deployment Attempt: 2026-09-09

- User authorized coordinated deployment and use of their Railway CLI.
- Candidate commit `48de2cb` rebased onto `origin/main` at `3f61d26`, preserving current web/i18n changes. No push or npm publication.
- Set cloud adapter variables with `--skip-deploys`: SSO-only gate, private backend port 6060, private MCP port 4401 and private WS port 4402.
- Railway CLI rejected cloud upload with `UPLOAD_FAILED`: `Your trial has expired. Please select a plan to continue using Railway.`
- No new deployment was started. Frontend proxy variables, frontend/web deployments and local runtime were left unchanged. New cloud variables remain saved for the next deployment.
- Local candidate nginx image passed `nginx -t`. Rebased narrow Go tests, web typecheck and bootstrap checks passed. Frame test required a mock for the new upstream `useTranslations` hook; product code was not changed for this.
- Real hosted `execute_code` remains unverified. Activate an eligible Railway plan before resuming cloud deployment, confirming its effective private listen port, then frontend/web deployment and runtime verification.
- Final rebased checks passed: `go test -p 2 ./...`, web `npm run typecheck`, `npm run build`, `npm run lint`, `npm audit --omit=dev` (zero reported vulnerabilities), both Node regression scripts and `git diff --check`. Temporary nginx validation container removed. Local Node 23 engine warning remains; production image uses Node 22.

## Checkout And Runtime

- Original checkout: `/Users/levanmong/Documents/codex-mcp`. Existing unrelated changes were preserved.
- Patch checkout: `/private/tmp/codelocal-penpot-auth-session`, branch `fix/penpot-auth-session`.
- Base: `5bf922c0bb6810dc7d1edbb6b12b23afbc89b3fd` from `origin/main`.
- No commit, push, npm publication, or Railway deployment performed.
- Running test binary: `/private/tmp/codelocal-penpot-auth-session-verified`, PID 95001 at startup.
- The installed npm binary has NOT been replaced. Restarting via that old binary can restore the old behavior.

## Findings

1. The installed runtime reported version 1.5.61, was built from `a7c4c837e4b3ee559b7feba195f6b0a388536e93`, and lacked `pluginConfig`. It predates the managed Penpot implementation now on origin/main. This explains the observed empty catalog on that runtime.
2. Managed registration is injected into effective MCP config. It is not a user-created registry entry. The hosted default was introduced across commits including `548833b`, `9edd46b`, `e62fc70`, and `efc83aa`.
3. Penpot MCP 2.17.0 reads `req.query.userToken` when initializing a session. Existing sessions retain that original value. `PluginBridge` uses it to route to the matching browser plugin. A static overview response does not prove authenticated project access.
4. CodeLocal previously allowed a missing managed credential to initialize an unauthenticated session. Rotation under the same credential reference also reused the old session.
5. The Design frame provisioned only the first ten online workspaces. The target workspace was beyond that limit. The bootstrap also treated failed token lookups as permission to create a replacement token and did not retry on SPA navigation after SSO.
6. The bootstrap URL without a version query returns a cached 404. The actual HTML URL, `/codelocal-mcp-bootstrap.js?v=1`, returns HTTP 200 JavaScript. The unversioned 404 is NOT evidence that the injected script is missing.

## Changes

- `internal/mcphub/hub.go`: fail closed for absent/invalid managed credentials; remove process-global credential fallback; bind cached sessions to endpoint/key fingerprint; reject stale credentials before outgoing requests; disable authenticated redirects; redact credential-bearing transport errors.
- `internal/mcphub/managed_penpot.go`: retain the reserved default when a debug endpoint override is malformed.
- `internal/mcphub/penpot_session_test.go`: session rotation/revocation, stale-session guard, missing auth, global-env rejection, registration with failed backend, invalid override, and transport-error redaction.
- `web/src/app/dashboard/design/penpot-design-frame.tsx`: configure all eligible workspaces sequentially; stop old-token queues; retry unsuccessful connections when focus refreshes workspace state.
- `web/src/app/dashboard/design/penpot-design-frame.test.mjs`: deterministic effect check for 29 workspaces, concurrency, CSRF, origin/source filtering, and in-flight rotation.
- `deploy/penpot/codelocal-mcp-bootstrap.js`: do not create keys after failed lookup; retry on hash navigation, visibility, focus, and a visible-page interval.
- `deploy/penpot/bootstrap.test.mjs`: bootstrap failure and lifecycle regression checks.
- `deploy/penpot/Dockerfile` and `Dockerfile.penpot-frontend`: bump injected script cache version to 2.
- `internal/oauth/oidc.go`: reuse the existing Ed25519 signer/verifier without changing OIDC access-token claims.
- `internal/oauth/penpot.go` and `penpot_test.go`: ten-minute audience-specific grants, live security-version/device revocation, signed sessions bound to user/device/credential/workspace/native key, expiry and cross-owner tests.
- `internal/penpot/adapter.go`: verify grants, workspace authorization, current encrypted credential and native profile ownership before private MCP proxying. Protect WebSocket admission and both message directions with native credential validation; bind response IDs to tasks delivered on that socket.
- `internal/penpot/adapter_test.go`: owner/expiry, header isolation, session replay, redirects/outages, real local WebSocket traffic and forged-response tests.
- `internal/penpot/adapter_integration_test.go`: opt-in isolated local PostgreSQL test using real Store encryption, OAuth signing and MCP Hub transport against an explicitly synthetic MCP fixture.
- `internal/cloudserver/server.go` and `plugin_connection_api.go`: optional adapter routes, validate ownership before provisioning, replace native credentials with signed grants in runtime payloads; fail closed if adapter unavailable.
- `deploy/penpot/nginx-mcp-locations.conf.template`: route HTTP and WebSocket through cloud adapter, suppress credential-bearing WS logs; reject legacy unsigned SSE with 410. Hosted managed transport remains Streamable HTTP with the same four tool names and public URL. Legacy SSE clients require migration.

## Adapter Auth Flow

1. Browser completes CodeLocal SSO into its own Penpot profile.
2. Same-origin Penpot bootstrap provisions a native key; parent validates message origin/source and sends the key through authenticated, CSRF-protected workspace provisioning.
3. Cloud validates the native key through Penpot `get-profile`, checks current user email ownership, and stores it encrypted at workspace scope.
4. Existing authenticated runtime settings channel carries a ten-minute signed grant, never a browser cookie or device credential secret.
5. Hub sends that grant in `Authorization`, not a public URL query. Adapter verifies signature, issuer, audience, expiry, security version, active device credential, authorized workspace and current stored native key.
6. Adapter sets native `userToken` only on private upstream MCP requests and seals upstream session IDs. A grant for B cannot reuse A's sealed session. Native plugin WebSockets are independently authenticated and task responses are bound to their own connection.

Grants remain bearer secrets. A stolen complete grant can act as its owner until expiry/revocation; tests do not claim proof-of-possession or protection after credential theft. The native key inside a signed grant is readable, not encrypted by JWT signing; the encrypted Store and existing secret runtime channel remain its confidentiality boundaries.

## Live Evidence

After stopping the previous test runtime and starting the verified binary, MCP list/refresh returned:

```json
{
  "name": "penpot",
  "managed": true,
  "installedBy": "codelocal",
  "enabled": true,
  "transport": "http",
  "url": "https://design.codelocal.cloud/mcp/stream",
  "version": "2.17.0",
  "toolsCached": 4,
  "connected": false
}
```

Fresh discovery, `high_level_overview`, and read-only `execute_code` failed with:

```text
managed Penpot authentication is unavailable; open CodeLocal Design to connect this account
```

There is **no successful execute_code output** from this patch. Four cached tools are not proof of fresh discovery or execution.

## Verification

Passed:

- `go test ./internal/mcphub ./internal/localclient ./internal/runtime`
- `go test -race ./internal/mcphub`
- `go test -p 2 ./...`
- `go build -o /private/tmp/codelocal-penpot-auth-session-verified ./cmd/codelocal`
- `npm run typecheck` in `web/`
- `node deploy/penpot/bootstrap.test.mjs`
- `node src/app/dashboard/design/penpot-design-frame.test.mjs` in `web/`
- targeted ESLint for the Design frame and its test
- `npm run check:repo`
- `git diff --check`

Current adapter checkpoint also passed:

- `go test ./internal/oauth ./internal/penpot ./internal/mcphub ./internal/cloudserver`
- `go test -race ./internal/penpot ./internal/oauth ./internal/mcphub`
- `CODELOCAL_PENPOT_TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55439/postgres?sslmode=disable' go test -v ./internal/penpot -run TestPenpotSignedRuntimeToAdapterIntegration`
- Same local database with `go test -race -count=1 ./internal/penpot ./internal/oauth ./internal/mcphub`
- `go test -p 2 ./...` after WebSocket response-owner enforcement
- `npm run typecheck` in `web/`
- `npm run build` in `web/`, explicit subprocess exit code 0
- Both Node regression scripts above
- `git diff --check`

The PostgreSQL test returned `PASS`. Its `fixtureOwner` response is synthetic, not a real Penpot page/project result. The adapter code has not been deployed and the running old verified binary does not contain the adapter-aware bearer transport.

The first `go test ./...` attempt failed at `TestMacNativeDaemonProbeAndPersistentHandshake` with a two-second probe timeout. The isolated test and full rerun with `-p 2` passed. No native code was changed.

The local Node version was 23.3.0; npm warned that the web package requires `^22.13.0 || >=24.0.0`. Typecheck, targeted checks and production Next build passed. Deployment should use the supported Node engine.

## Security Coverage And Remaining Work

- A: PostgreSQL-backed Hub/adapter test routes A and B independently; real hosted users not tested.
- B: cross-user sealed HTTP session replay and forged WebSocket response IDs are rejected in fixtures. Stolen bearer credentials are outside this claim.
- C: missing/invalid signed HTTP auth is rejected by Hub/adapter; WS validates native credentials before upstream admission.
- D: expired/wrong-audience signed grants and native token expiry are rejected in tests; production expiry not exercised.
- E: managed registration survived real runtime replacement/restart.
- F: a backend-503 fixture preserves registration and Hub startup; no live outage was induced.

Railway read-only configuration inspection confirmed backend and frontend flags include `enable-login-with-oidc enable-oidc-registration disable-login-with-password disable-registration`. The native MCP service has no public/custom domain. Current frontend still points directly to `http://penpot-mcp.railway.internal:4401`, so the deployed system does not yet enforce this adapter.

Independent candidate review found the upstream global pending-task response-ID gap. Source inspection confirmed it; the adapter now tracks outstanding IDs per WebSocket and a real local WebSocket test rejects B's forged result for A. No upstream core fork was made.

Before sign-off: confirm production native profile JSON against this adapter, validate deployed nginx configuration, obtain approval for coordinated production deployment, configure private adapter upstream URLs and SSO-only gate, then deploy cloud/web/Penpot frontend and install the matching runtime. Prove fresh four-tool discovery, execute a read-only call against the real open Penpot file, and repeat after restart. Do not push main or publish npm as a verified fix before this evidence exists.

Latest real runtime recheck still shows managed `penpot` with four cached tools and `connected:false`. Both `high_level_overview` and `execute_code` return the missing managed-auth error above. Read-only execution request ID: `0f0f3966c8cae8fae21c509a8e3050e5`. No success claim follows from cached search results.

Remaining debt: email ownership is a projection of enforced SSO, not a persisted Penpot profile-ID mapping; five-minute grant refresh currently reconnects Hub sessions; native profile validation occurs per WebSocket message and every 30 seconds idle; unresolved WebSocket task IDs are capped at 1024 before reconnect. Native expiry/revocation on an idle WS is detected within the revalidation interval, not instantaneously.

## Release Impact

- Runtime changes require a new CodeLocal native/npm release, not only a server redeploy.
- Completed frontend patches require deployment of `penpot-frontend` and the CodeLocal web service serving `codelocal.cloud`.
- The signed-context adapter requires deployment of `CodeLocal-MCP-PROD`, the CodeLocal web service, and `penpot-frontend`, plus private upstream configuration and frontend proxy routing. Those source changes are now in this patch but are NOT deployed.
- Required cloud settings: `CODELOCAL_PENPOT_SSO_ONLY=true`, `CODELOCAL_PENPOT_BACKEND_URL=http://penpot-backend.railway.internal:6060`, and root MCP/WS URLs for the private native server. Confirm WS listen port before applying; never guess it.
- Frontend `PENPOT_MCP_URI` must become the cloud service's private root URL (currently `codelocal-mcp-dev.railway.internal`, confirm listen port). This routing update must accompany the new nginx template.
- Current changes do not require a Penpot core fork or replacing `penpotapp/mcp:2.17.0`.
- Relevant Railway project: `d5286319-d1c5-4b51-9022-ac3211ed8f2a`; environment `8e05c1f6-9e7c-4cdd-9346-63f4d3b074ec` is named `dev` but serves the inspected production domains.
