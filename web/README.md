# CodeLocal Web

Canonical browser product for CodeLocal.

Stack: **Next.js App Router + TypeScript + React**.

The accepted product technology ownership is documented in [`../docs/architecture/PRODUCT_STACK.md`](../docs/architecture/PRODUCT_STACK.md).

## Development

Use the repository Node version:

```bash
nvm use
```

Install and run:

```bash
npm ci
npm run dev
```

The development server starts on the normal Next.js development port unless overridden.

## Verification

```bash
npm run lint
npm run typecheck
npm run build
npm audit --omit=dev
```

## Backend boundary

The browser product does **not** replace the Go backend.

Server-only backend configuration:

```text
CODELOCAL_BACKEND_URL=http://127.0.0.1:3333
```

Copy `.env.example` to an untracked local environment file when needed. Never expose privileged backend configuration through `NEXT_PUBLIC_*` variables.

`src/lib/server/backend.ts` is the initial server-only HTTP boundary. Do not turn it into a generic authenticated proxy. Existing Go session, CSRF, rate-limit, authorization and tenant behavior must be migrated route by route with parity tests.

The client-neutral read contracts currently include:

- `GET /api/v1/dashboard/overview` — compact overview DTO, typed in `src/lib/contracts/dashboard.ts`;
- `GET /api/v1/workspaces` — authorized workspace display/status DTO;
- `GET /api/v1/devices` — paired device display/status DTO;
- `GET /api/v1/usage` — 24h/30d/all-time MCP tool payload estimates, explicitly separated from provider billing;
- `GET /api/v1/knowledge/graph` — bounded Project Brain graph with response-local node/edge IDs and privacy-minimized display fields;
- `GET /api/v1/knowledge/health` — aggregate Project Brain pipeline/index/canary health, collective rollout/preferences and the CSRF token used by the existing Go preference mutation;
- `GET /api/v1/code/graph` — bounded local-runtime Code Graph using public workspace identifiers, repository-relative paths and response-local graph IDs;
- `GET /api/v1/account` — account display state plus CSRF/fresh-security status, excluding password hashes, salts, security version and session ID.

The device/workspace DTOs are typed and runtime-validated in `src/lib/contracts/resources.ts`; usage is typed and validated in `src/lib/contracts/usage.ts`; the graph contract is validated in `src/lib/contracts/knowledge.ts`. Resource DTOs are intentionally narrower than internal Go structs and exclude credential identifiers, public keys, secret hashes, project roots, capabilities, routing keys and infrastructure diagnostics. The graph contract additionally excludes source-memory IDs, raw graph IDs and repository remotes. Usage does not infer USD cost or provider billing.

Sensitive resource mutations now use public identifiers only:

- `POST /api/v1/devices/{deviceId}/revoke` resolves the private credential ID server-side before calling the shared Go revoke/audit path;
- `POST /api/v1/workspaces/{deviceId}/{workspaceId}/remove` uses the existing local-runtime revocation handshake and audit path.

Both mutations require the current CSRF token and a fresh Go security context. Next.js does not decide whether the session is trustworthy.

Browser-session-bound reads use same-origin `/api/v1/...` requests. For a **direct Next canary**, `next.config.ts` rewrites those requests to Go so the existing browser session can be tested without duplicating auth. `src/proxy.ts` normalizes Railway forwarding headers on that canary path.

The production topology is stricter: the existing Go gateway keeps `codelocal.cloud`, authenticates protected dashboard requests, and proxies **presentation routes only** to the private Next service. Browser APIs, auth, OAuth, pairing, `/client`, `/mcp` and mutations continue to hit Go directly. Before Go sends a dashboard request to Next it strips Cookie, Authorization and client-IP/Railway edge headers, so the presentation service does not receive the browser session secret.

Deployment artifacts:

- `Dockerfile.web` builds a Next standalone image;
- `railway.web.json` is the service-specific Railway config;
- `/healthz` is the web-service health check;
- [`../docs/operations/NEXT_WEB_CUTOVER.md`](../docs/operations/NEXT_WEB_CUTOVER.md) is the canary, cutover and rollback runbook.

`CODELOCAL_BACKEND_URL` is still required for direct-canary rewrites. Production Go-to-Next cutover instead uses `CODELOCAL_WEB_ORIGIN` plus the explicit `CODELOCAL_WEB_CUTOVER` feature flag.

## Migration state

Current slice provides:

- Next.js project foundation;
- product design tokens/reset;
- landing page;
- dashboard shell;
- versioned Go overview/workspaces/devices/usage/account read contracts;
- canonical Next routes for Overview, Workspaces, Devices, Usage, Knowledge Graph, Code Graph, Security and Account;
- privacy-minimized Knowledge Graph plus aggregate Project Brain health and CSRF-protected collective settings;
- bounded local-runtime Code Graph with checkout/repository/view/depth/symbol controls and impact evidence;
- Account password changes through the existing Go security handler, including CSRF, fresh-security checks, rate limiting, password verification, security-version rotation and session replacement;
- public-ID device revoke and workspace removal mutations that resolve sensitive credentials only server-side;
- Security presentation without exposing or fabricating audit/IP/login history;
- direct-canary rewrite support plus a production Go presentation proxy that keeps MCP/CLI/WebSocket transports off Next;
- explicit cutover/rollback flags and deployable Next standalone image;
- no fake live telemetry.

Feature parity is implemented. The existing Go-rendered UI remains compiled as the instant rollback path until the Railway canary and authenticated cutover checklist in `NEXT_WEB_CUTOVER.md` pass.
