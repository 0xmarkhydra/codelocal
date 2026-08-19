# Next.js Web Cutover Runbook

Status: **complete browser-presentation parity implemented; cutover remains feature-flagged until the Railway DEV canary passes.**

## Final topology

Keep the existing Go cloud service as the public gateway. Do not move the CodeLocal public domain directly to Next.

```text
Internet / Railway edge
          |
          v
   Go cloud gateway
   |-- browser GET/HEAD presentation ----> Next.js web service (private Railway network)
   |-- auth/OAuth/pairing mutations ------> Go
   |-- /api/* and /api/v1/* --------------> Go
   |-- /client and /mcp -------------------> Go
   `-- local-runtime transports -----------> Go
```

Next is the canonical browser UI. Go remains the identity, authorization, security, MCP and runtime authority.

## Next-owned browser presentation

When `CODELOCAL_WEB_CUTOVER=1`, Go proxies GET/HEAD for the complete user-facing browser surface to Next:

Public/account presentation:

- `/`
- `/login`
- `/register`
- `/signup`
- `/signup/verify`
- `/forgot-password`
- `/reset-password`
- `/privacy`
- `/terms`
- `/support`
- `/security`
- `/healthz`
- `/_next/*`, `/favicon.ico`, `/codelocal-icon.png`

Fresh-security protected presentation:

- `/authorize`
- `/pair/approve`

Authenticated dashboard presentation:

- `/dashboard`
- `/dashboard/connect`
- `/dashboard/workspaces`
- `/dashboard/knowledge`
- `/dashboard/knowledge-preview`
- `/dashboard/code-graph`
- `/dashboard/code-graph-preview`
- `/dashboard/devices`
- `/dashboard/usage`
- `/dashboard/invite`
- `/dashboard/leaderboard`
- `/dashboard/security`
- `/dashboard/account`
- `/dashboard/account-preview`
- `/dashboard/admin` (admin identity required)

Unknown browser paths, protocol endpoints and every mutation remain on Go.

## Go-owned trust surface

The cutover must never proxy these through Next as application authority:

- POST/PUT/PATCH/DELETE requests;
- login/signup/verification/password-reset form processing;
- logout and password change;
- OAuth client registration, authorization POST and token endpoint;
- device pairing POST and workspace/device mutations;
- `/api/*`, `/api/v1/*`;
- `/.well-known/*` protocol metadata;
- `/client`, `/mcp`, runtime WebSocket/API routes;
- health/debug/internal protocol endpoints that are not browser presentation.

Next forms submit to the existing Go handlers. Forms include `ui=next` only to make Go redirect validation/rate-limit/recovery outcomes back to Next; it does not change security policy.

## Security boundary

For authenticated Next pages, Go resolves the existing browser identity before the presentation hop. `/authorize` and `/pair/approve` also require a fresh security context. `/dashboard/admin` additionally requires the configured admin identity.

Before Go proxies a presentation request to Next it strips:

- `Cookie`;
- `Authorization`;
- `X-Real-IP`;
- `X-Forwarded-For`;
- Railway edge/request identifiers.

The presentation service therefore does not receive browser session secrets or the original client-IP security signal.

Public Next auth forms obtain their CSRF form token from `GET /api/v1/auth/csrf`; the matching CSRF cookie remains HttpOnly. Signup/reset context APIs expose only validity and masked email addresses. OAuth context is validated by Go against the registered redirect URI, PKCE S256 challenge and MCP resource before consent is rendered.

## Railway DEV web service

Create a second service in the existing `dev` environment from `0xmarkhydra/codex-mcp`, branch `dev`:

```text
Service: CodeLocal-Web-DEV
Config as Code: /railway.web.json
Dockerfile: Dockerfile.web
Health: /healthz
Port: 3000
```

Keep the repository root as build root because `Dockerfile.web` copies `web/`.

Set on the web service:

```text
CODELOCAL_BACKEND_URL=http://${{CodeLocal-MCP-DEV.RAILWAY_PRIVATE_DOMAIN}}:${{CodeLocal-MCP-DEV.PORT}}
```

Use Railway service-reference variables rather than copying a private hostname manually where possible.

Give the web service a temporary Railway public domain for canary checks. Do not move `codelocal.cloud` or any production domain.

## DEV canary checks

Before enabling the Go presentation cutover:

1. `GET <web-canary>/healthz` returns healthy.
2. `/`, `/login`, `/signup`, `/forgot-password`, `/privacy`, `/terms`, `/support`, `/security` render the Next design.
3. Unauthenticated `/api/v1/account` through the canary remains 401/no-store rather than fabricating an account.
4. Invalid/expired signup/reset context fails closed.
5. No Next page receives a raw project root, credential ID, password hash/salt, browser session ID or raw graph identity.

Authenticated canary checks with a test account:

1. login/logout remain Go session operations;
2. signup → email OTP → verify creates the account only after OTP success;
3. forgot/reset password preserves rate limits and revokes existing sessions after success;
4. OAuth consent validates client/redirect/PKCE/resource and POSTs to Go;
5. pair approval requires authenticated fresh security context and POSTs to Go;
6. MCP Connections shows the current public origin plus `/mcp`;
7. Workspaces/Devices preserve search, 12-item paging and mutation semantics;
8. Workspace → Code Graph opens the matching public device/workspace context;
9. Knowledge Graph, Code Graph and Project Brain controls fail closed when backend/runtime is unavailable;
10. Invite, Leaderboard and Admin render the Go-backed client-neutral DTOs;
11. `/dashboard/admin` is forbidden for non-admin users;
12. mobile navigation and reduced-motion layouts remain usable.

## Enable DEV cutover

After the web service is healthy, set on **CodeLocal-MCP-DEV only**:

```text
CODELOCAL_WEB_ORIGIN=http://${{CodeLocal-Web-DEV.RAILWAY_PRIVATE_DOMAIN}}:${{CodeLocal-Web-DEV.PORT}}
CODELOCAL_WEB_CUTOVER=1
```

Redeploy/restart the Go DEV service if Railway does not automatically redeploy after variable changes.

Then verify on the existing public DEV gateway:

```text
https://codelocal-mcp.up.railway.app/
```

Expected:

- entire browser UI is Next;
- `/health` is still Go health;
- `/healthz` reaches Next through Go;
- `/api/*`, `/mcp`, `/client`, OAuth POST/token and mutations still terminate on Go;
- protected pages redirect to the Next login presentation when unauthenticated;
- no legacy Go HTML appears in normal browser flows.

## Rollback

If any DEV smoke check fails:

```text
CODELOCAL_WEB_CUTOVER=0
```

or remove the variable, then redeploy/restart the Go service. The legacy Go-rendered presentation remains compiled specifically for this rollback window. Auth, database, MCP, pairing and runtime state do not need to be rolled back.

Do not delete the legacy Go presentation until the Next UI has been proven in production and rollback confidence is high.

## Production promotion

Only after DEV passes:

1. merge the verified dev line according to the normal release process;
2. create `CodeLocal-Web-PROD` from `main` with the same web config;
3. set its private Go backend reference to `CodeLocal-MCP-PROD`;
4. canary the web service;
5. set `CODELOCAL_WEB_ORIGIN` on Go PROD;
6. enable `CODELOCAL_WEB_CUTOVER=1` only after smoke tests pass;
7. keep public MCP/OAuth/client URLs unchanged.

This runbook does not authorize a production cutover automatically.

## Repository gates

Every cutover candidate must pass:

```bash
npm run ci
git diff --check
docker build -t codelocal-cloud:cutover .
docker build -f Dockerfile.web \
  --build-arg CODELOCAL_BACKEND_URL=http://backend.railway.internal:3333 \
  -t codelocal-web:cutover .
```

GitHub CI builds both Docker images and the cross-platform npm staging package.
