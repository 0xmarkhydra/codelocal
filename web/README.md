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

The first client-neutral read contract is `GET /api/v1/dashboard/overview`. Its TypeScript shape is documented in `src/lib/contracts/dashboard.ts`. The DTO is intentionally narrower than internal Go device/workspace structs and excludes credentials, public keys, project roots, capabilities and infrastructure diagnostics.

## Migration state

Current slice provides:

- Next.js project foundation;
- product design tokens/reset;
- landing page;
- dashboard shell;
- server-only Go backend URL boundary;
- no production route cutover;
- no fake live telemetry.

The existing Go-rendered web UI remains the production compatibility fallback until individual Next.js route families pass behavior/security parity and have a rollback path.
