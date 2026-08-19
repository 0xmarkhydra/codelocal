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
- `GET /api/v1/code/graph` — bounded local-runtime Code Graph using public workspace identifiers, repository-relative paths and response-local graph IDs.

The device/workspace DTOs are typed and runtime-validated in `src/lib/contracts/resources.ts`; usage is typed and validated in `src/lib/contracts/usage.ts`; the graph contract is validated in `src/lib/contracts/knowledge.ts`. Resource DTOs are intentionally narrower than internal Go structs and exclude credential identifiers, public keys, secret hashes, project roots, capabilities, routing keys and infrastructure diagnostics. The graph contract additionally excludes source-memory IDs, raw graph IDs and repository remotes. Usage does not infer USD cost or provider billing.

Browser-session-bound reads use same-origin `/api/v1/...` requests. `next.config.ts` rewrites those requests to the Go service and falls back unmatched routes to Go during incremental migration. This lets the browser send the existing HttpOnly session cookie and User-Agent naturally instead of having a Server Component replay session secrets.

Deployment requirements:

- `CODELOCAL_BACKEND_URL` must point directly at the Go service, not the public Next.js origin;
- the trusted edge/proxy must preserve the real browser User-Agent and trustworthy client-IP chain while overwriting or rejecting spoofed forwarding headers;
- unported login/signup/account routes continue to resolve to the Go application through the fallback rewrite.

## Migration state

Current slice provides:

- Next.js project foundation;
- product design tokens/reset;
- landing page;
- dashboard shell;
- versioned Go overview/workspaces/devices/usage read contracts;
- same-origin browser session bridge through explicit Next rewrites (implemented, pending real-edge cookie/User-Agent/client-IP parity verification before cutover);
- real Overview, Workspaces, Devices and Usage rendering only when authenticated backend data validates against runtime TypeScript contracts;
- Knowledge Graph preview with privacy-minimized graph DTO, aggregate Project Brain health and CSRF-protected collective settings that still persist through the existing Go mutation endpoint;
- Code Graph preview with checkout/repository/view/depth/symbol controls over a bounded local-runtime graph; project roots, routing keys, repository IDs and source hashes stay out of the browser contract;
- shared dashboard layout/navigation with legacy-route fallback to Go for unported route families;
- no production route cutover yet; Knowledge and Code Graph remain on Go routes until real-edge auth/security-signal parity is verified;
- no fake live telemetry.

The existing Go-rendered web UI remains the production compatibility fallback until individual Next.js route families pass behavior/security parity and have a rollback path.
