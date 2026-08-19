# CodeLocal Web

Canonical browser presentation for CodeLocal.

Stack: **Next.js App Router + TypeScript + React**. Go remains the identity, authorization, OAuth, MCP, API, security and local-runtime authority.

The product stack is documented in [`../docs/architecture/PRODUCT_STACK.md`](../docs/architecture/PRODUCT_STACK.md).

## Development

Use the repository Node version:

```bash
nvm use
npm ci
npm run dev
```

Verify with:

```bash
npm run lint
npm run typecheck
npm run build
npm audit --omit=dev
```

## Ownership boundary

Next owns browser-facing **GET/HEAD presentation**:

- landing and public docs;
- login, signup, email verification and password recovery;
- OAuth consent and device pairing approval;
- the complete dashboard: Overview, MCP Connections, Workspaces, Knowledge Graph, Code Graph, Devices, Usage, Invite, Leaderboard, Security, Account and Admin.

Go owns all trust-bearing behavior:

- browser sessions, CSRF, password hashing, reauthentication, rate limits and audit;
- OAuth client registration, authorization-code issuance and token exchange;
- device pairing and workspace authorization/revocation;
- `/api/*`, `/api/v1/*`, `/client`, `/mcp` and runtime transports;
- every POST/PUT/PATCH/DELETE mutation.

Next forms therefore POST back to the existing Go handlers. Go redirects validation, rate-limit and recovery outcomes to the canonical Next routes; there is no server-rendered Go UI fallback.

## Browser contracts

Client-neutral read contracts include:

- `GET /api/v1/dashboard/overview`
- `GET /api/v1/workspaces`
- `GET /api/v1/devices`
- `GET /api/v1/usage`
- `GET /api/v1/knowledge/graph`
- `GET /api/v1/knowledge/health`
- `GET /api/v1/code/graph`
- `GET /api/v1/account`
- `GET /api/v1/invite`
- `GET /api/v1/leaderboard`
- `GET /api/v1/admin`
- `GET /api/v1/pair/approve`
- `GET /api/v1/auth/csrf`
- `GET /api/v1/auth/signup-verification`
- `GET /api/v1/auth/password-reset`
- `GET /api/v1/oauth/authorize-context`

The DTOs intentionally exclude private credential IDs, password hashes/salts, security versions, browser session IDs, project roots, routing keys, raw graph IDs, source-memory IDs and other server-only/runtime-only details. Invite and leaderboard email addresses are masked outside the admin-only contract.

Sensitive mutations continue through Go. Device revoke uses public `deviceId` in the browser and resolves the private credential ID server-side. Workspace removal uses the existing local-runtime revocation handshake. Password changes/reset preserve CSRF, fresh-security checks, security-version/session rotation, rate limiting and audit.

## CSRF and auth presentation

The CSRF cookie remains HttpOnly. Public Next auth forms obtain the matching form token from `GET /api/v1/auth/csrf`; they never read the cookie directly or weaken its flags.

Signup verification and password-reset context APIs return only validity plus a masked email address. OAuth consent context is validated server-side against the registered redirect URI, PKCE S256 challenge and CodeLocal MCP resource before Next renders the consent form.

## Deployment

Deployment artifacts:

- `Dockerfile.web` — Next standalone production image;
- `railway.web.json` — Railway web-service config;
- `/healthz` — Next health endpoint;
- [`../docs/operations/NEXT_WEB_CUTOVER.md`](../docs/operations/NEXT_WEB_CUTOVER.md) — canary/cutover/rollback runbook.

Final topology:

```text
Internet
   |
   v
Go gateway
   |-- GET/HEAD browser presentation ----> private Next service
   |-- API / auth mutations / OAuth POST --> Go
   |-- /mcp / /client / runtime ----------> Go
```

The public domain stays on Go. Go authenticates protected presentation routes before proxying them to Next and strips Cookie, Authorization and client-IP/Railway edge headers from the Go-to-Next presentation hop. The Next service therefore does not become a second session authority.

For direct canary testing, `CODELOCAL_BACKEND_URL` lets Next rewrite the explicit `/api/v1/*` read contracts to Go. Full auth/mutation smoke tests run through the public Go gateway, which owns those POST routes. Configure `CODELOCAL_WEB_ORIGIN` on the Go service to enable the canonical Next presentation.

The legacy Go-rendered presentation has been removed. If the web presentation must be rolled back, roll back the deployed revision rather than switching to a second UI implementation.

## Product behavior

The Next presentation includes:

- real state-driven onboarding for setup-required, runtime-offline and runtime-connected accounts;
- search/pagination for devices/workspaces;
- Workspace → Code Graph deep links by public IDs;
- Knowledge Graph and Project Brain health/controls;
- Code Graph checkout/repository/view/depth/symbol controls;
- invite/referral network and 30-day usage leaderboard;
- admin user/referral/runtime overview;
- Account/Security and logout;
- public Privacy, Terms, Support and Security pages;
- responsive/mobile navigation;
- no fabricated live telemetry or invented audit/login/IP feeds.
