<!-- BEGIN:nextjs-agent-rules -->
# This is NOT the Next.js you know

This version has breaking changes — APIs, conventions, and file structure may all differ from your training data. Read the relevant guide in `node_modules/next/dist/docs/` before writing any code. Heed deprecation notices.
<!-- END:nextjs-agent-rules -->

# CodeLocal Web Instructions

Scope: `web/**`.

CodeLocal Web is the canonical browser product and uses Next.js App Router + TypeScript + React.

## Architecture

- Keep pages/layouts as Server Components by default. Add `"use client"` only at the smallest interactive boundary.
- The Go backend remains authoritative for identity, authorization, tenant scope, billing/entitlements, Project Brain and durable state.
- Treat the Go backend as an external HTTP API with Zero Trust semantics. Do not import Go persistence/business logic into the web app or recreate authorization truth in TypeScript.
- Put server-only backend access behind `src/lib/server/` and use `import "server-only"` for modules that touch private environment variables or privileged APIs.
- Never expose backend credentials through `NEXT_PUBLIC_*` variables.
- Route Handlers/Server Actions are public/reachable boundaries; authenticate, authorize and validate again at every mutation boundary.
- Existing Go-rendered routes remain compatibility fallback until a Next.js route family has security/behavior parity and an explicit cutover/rollback plan.

## UI

- Follow `docs/plans/ui/PRODUCT_UI_MASTER_PLAN.md` and `docs/plans/ui/NEURAL_CONTROL_PLANE_DASHBOARD_MASTER_PLAN.md`.
- Use global CSS only for tokens/reset/foundation; prefer CSS Modules or coherent feature styles for route/component-specific styling.
- Do not use fake telemetry, fake terminal output, fake AI thinking, fake online counts or decorative data presented as real system state.
- Motion/glow must correspond to real state when connected to production data. Static previews must be clearly presented as product structure, not live activity.
- Accessibility and `prefers-reduced-motion` are required for animated features.

## Verification

For web changes run:

```text
npm run lint
npm run typecheck
npm run build
npm audit --omit=dev
```

Use the repository root CI after changes that also touch shared/release/backend boundaries.
