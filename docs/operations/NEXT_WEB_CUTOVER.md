# Next.js Web Cutover Runbook

Status: **feature parity implemented; production cutover is feature-flagged and must pass canary deployment gates.**

## Production topology

Keep the existing Go cloud service as the public gateway for `codelocal.cloud`.

```text
Internet / Railway public edge
          |
          v
   Go cloud gateway
   ├─ auth / OAuth / pairing
   ├─ /api/* and /api/v1/*
   ├─ /client transport
   ├─ /mcp transport
   ├─ /dashboard/admin
   └─ selected web pages ──────> Next.js web service (Railway private network)
```

The Next service is the canonical browser presentation layer. Go remains the network, identity, authorization, MCP and local-runtime authority.

Do **not** move the production domain directly from Go to Next. The Go service owns long-lived/runtime routes such as `/client` and `/mcp`; making Next the catch-all public proxy would unnecessarily put those transports behind a framework proxy.

## Route ownership after cutover

Go proxies these presentation routes to Next when `CODELOCAL_WEB_CUTOVER=1`:

- `/`
- `/dashboard`
- `/dashboard/*`, except `/dashboard/admin` and descendants
- `/_next/*`
- `/favicon.ico`
- `/codelocal-icon.png`
- `/healthz`

Go continues to serve all other routes directly, including:

- login/signup/password reset and reauthentication;
- OAuth and MCP authorization;
- pairing and client-runtime endpoints;
- `/api/*` and `/api/v1/*`;
- `/client` and `/mcp`;
- `/dashboard/admin`;
- legal/support/security public documents.

This also means browser API calls from Next pages go directly to the public Go service on the same origin. They do not need to pass through the Next server in production.

## Security boundary

For protected dashboard pages, Go runs the existing `WebAuth.Require` check **before** proxying presentation to Next.

The Go presentation proxy then strips:

- `Cookie`;
- `Authorization`;
- `X-Real-IP`;
- `X-Forwarded-For`;
- Railway edge/request identifiers.

Next therefore renders the page without receiving the browser session secret or client-IP security signal. Sensitive mutations continue to hit Go directly and retain CSRF, fresh-security, rate-limit and audit behavior.

The old Go-rendered dashboard remains compiled. Rollback is an environment change, not a code revert.

## Railway web service

The repository includes:

- `Dockerfile.web` — production Next standalone image;
- `railway.web.json` — service-specific build/health configuration;
- `/healthz` — Next service health check;
- `web/src/proxy.ts` — forwarding-header hardening for direct canary traffic.

Create a second Railway service from the same GitHub repository. Configure its **Config as Code file** to:

```text
/railway.web.json
```

Keep the repository root as the build root because `Dockerfile.web` copies from `web/`.

Set the web service variable:

```text
CODELOCAL_BACKEND_URL=http://${{<GO_SERVICE>.RAILWAY_PRIVATE_DOMAIN}}:${{<GO_SERVICE>.PORT}}
```

Replace `<GO_SERVICE>` with the actual Railway service reference. The variable is required during the Docker build because Next compiles external rewrite destinations into the production build; `Dockerfile.web` declares it as a build `ARG` and Railway also supplies service variables at runtime.

During canary testing, give the Next service its own temporary public Railway domain. Do not move `codelocal.cloud` yet.

## Edge canary probe

The Go service contains a disabled-by-default probe at:

```text
GET /internal/web-edge-probe
```

It returns `404` unless `CODELOCAL_EDGE_PROBE_TOKEN` is configured and the exact bearer token is supplied. The probe returns only HMAC digests of client IP and User-Agent, never the raw client IP.

Set the same temporary token in the Go deployment and local shell, then run:

```bash
CODELOCAL_WEB_URL=https://<next-canary-domain> \
CODELOCAL_BACKEND_PUBLIC_URL=https://<go-public-domain> \
CODELOCAL_EDGE_PROBE_TOKEN='<temporary-secret>' \
npm run verify:web-edge
```

The smoke check verifies:

1. Next `/healthz` works;
2. `/api/v1/account` reaches Go through the direct canary rewrite and remains unauthenticated/no-store without a session;
3. direct-Go and Next-proxied client-IP HMACs match;
4. direct-Go and Next-proxied User-Agent HMACs match;
5. HTTPS/Railway edge markers survive the canary path.

Remove `CODELOCAL_EDGE_PROBE_TOKEN` after canary verification unless another controlled deployment check is planned.

## Authenticated canary checks

Before production cutover, verify with a test account and disposable device/workspace:

1. Login and logout through the canary domain.
2. Overview, Workspaces, Devices and Usage show only backend-validated state.
3. Knowledge Graph and Code Graph render bounded data and fail closed when the backend/local runtime is unavailable.
4. Password change requires current password + CSRF + fresh security context and rotates other browser sessions.
5. Device revoke uses only public `deviceId` in the browser request; private credential ID never appears in browser JSON/HTML.
6. Workspace removal is disabled while the runtime is offline and succeeds through the existing local revocation handshake when online.
7. Security page does not fabricate an audit/login/IP feed.
8. Reduced-motion and mobile layouts remain usable.

## Enable production presentation proxy

After the web canary passes, configure the **Go service**:

```text
CODELOCAL_WEB_ORIGIN=http://${{<WEB_SERVICE>.RAILWAY_PRIVATE_DOMAIN}}:${{<WEB_SERVICE>.PORT}}
CODELOCAL_WEB_CUTOVER=1
```

Do not change `PUBLIC_BASE_URL`, MCP/OAuth endpoints or client gateway URLs as part of this cutover.

Verify after deployment:

- `/health` still reports Go health;
- `/healthz` reports Next health through Go;
- `/mcp` and `/client` still terminate on Go;
- `/dashboard/admin` is still Go;
- `/dashboard`, Knowledge, Code Graph, Security and Account are Next pages;
- API and mutation behavior remains Go-owned.

## Rollback

Set:

```text
CODELOCAL_WEB_CUTOVER=0
```

or remove it and redeploy the Go service. The gateway immediately returns to the existing Go-rendered web UI without changing auth, database, MCP, pairing or runtime state.

If the Next service itself is unhealthy, keep cutover disabled until `/healthz` is stable. Do not make the Go transport surface depend on Next availability.

## Repository gates

Before every web cutover candidate:

```bash
npm run ci
git diff --check
docker build -t codelocal-cloud:cutover .
docker build -f Dockerfile.web \
  --build-arg CODELOCAL_BACKEND_URL=http://backend.railway.internal:3333 \
  -t codelocal-web:cutover .
```

CI builds both Docker images so an application build can be green while a production container build is not.
