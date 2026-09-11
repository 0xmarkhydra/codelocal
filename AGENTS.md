# CodeLocal Repository Instructions

These instructions apply to the entire repository unless a deeper `AGENTS.md` overrides them.

## Canonical implementation

CodeLocal is a multi-surface product with explicit technology ownership. The accepted decision is documented in `docs/architecture/PRODUCT_STACK.md`.

- Backend/cloud/runtime domain: **Go**.
- Browser product under `web/`: **Next.js + TypeScript + React**.
- Mobile application under `app/`: **Flutter + Dart** for iOS and Android.
- Desktop application under `desktop/`: **Flutter + Dart** for macOS, Windows and Linux.
- CLI/local runtime: **Go**.
- Native OS bridges: platform-specific code only where required; macOS Computer Engine remains Swift/native.
- Existing `cmd/` entrypoints and `internal/` Go packages remain canonical during incremental migration; do not mass-move them for cosmetics.
- `legacy/typescript-runtime/` is the quarantined legacy TypeScript runtime. Do not add new product behavior there and do not edit it unless the task explicitly targets legacy compatibility, migration evidence, or deletion work.
- Root `package.json` orchestrates repository-wide build/release tasks; quarantined legacy TypeScript is not canonical browser code. New browser TypeScript belongs in `web/`.

## Where new code belongs

Choose ownership by product surface first, then domain.

- Browser UI/product: `web/` (Next.js App Router + TypeScript). Do not add new production browser surfaces to legacy Go-rendered UI once a Next.js route-family boundary exists.
- Mobile application: `app/` (Flutter/Dart).
- Desktop application: `desktop/` (Flutter/Dart).
- Go backend/cloud domains: existing `internal/*` packages until their safe migration into `backend/` is explicitly performed.
- Go CLI/local runtime domains: existing `cmd/codelocal` + local/runtime packages until their safe migration into `cli/` is explicitly performed.
- Native platform bridges: existing `cmd/computerhelper`, `cmd/computernative` until their safe migration into `native/` is explicitly performed.
- AI/agent orchestration: `internal/orchestration`, `internal/agentruntime`
- MCP public gateway/tool contract: `internal/mcpgateway`, `internal/mcphub`
- Project/code intelligence: `internal/project`, `internal/projectbrain`, `internal/lsp`
- Local runtime/client execution: `internal/runtime`, `internal/localclient`, `internal/taskexecution`
- Browser/computer automation implementation: `internal/automation`; presentation belongs to `web/` or Flutter clients.
- Cloud domain/storage: `internal/cloud`
- Existing Go HTTP/API transport: `internal/cloudserver`; new browser rendering belongs in `web/`.
- Authentication/security policy: `internal/webauth`, `internal/oauth`, `internal/deviceauth`, `internal/security`, `internal/webutil`
- Files/process/platform support: `internal/localfs`, `internal/process`, `internal/osutil`, `internal/runtimecontrol`
- Release-only logic: `cmd/release`, `.github/workflows`, `scripts`

Approved product-level source directories are `backend/`, `web/`, `app/`, `desktop/`, `cli/`, `native/`, and `tools/`. Create them incrementally only when the corresponding migration/implementation starts; do not mass-move existing code for symmetry.

## File placement rules

1. Keep the repository root boring. Root is reserved for project metadata, build/release manifests and the few canonical project entry documents.
2. Product plans, architecture notes, migrations and design specifications belong under `docs/`, not the repository root.
3. New tests live next to the Go package they test using `*_test.go` unless they are true cross-system fixtures/integration tests.
4. Avoid generic dumping-ground filenames such as `utils.go`, `helpers.go`, `common.go`, `misc.go`, `manager2.go`, `new.go`, or `temp.go`.
5. Name files by responsibility, not chronology. Prefer `token_family.go`, `workspace_routing.go`, `graph_query.go`.
6. Entry-point packages under `cmd/` should compose dependencies; domain logic should move into `internal/` rather than accumulating in `main.go`.
7. Do not duplicate a domain implementation across `internal/` packages. If ownership is unclear, document the boundary before adding code.

## Package size rule

A package becoming large is not itself a reason for cosmetic splitting. Split only along a stable domain boundary with explicit callers and tests.

Packages currently requiring extra care because they aggregate multiple subdomains:

- `internal/cloud`
- `internal/cloudserver`
- `internal/automation`
- `internal/mcpgateway`
- `internal/project`

For these packages, prefer extracting a cohesive subdomain over adding another unrelated file.

## UI architecture

Follow the product-wide UI direction in `docs/plans/ui/PRODUCT_UI_MASTER_PLAN.md` and `docs/plans/ui/NEURAL_CONTROL_PLANE_DASHBOARD_MASTER_PLAN.md`.

New production browser UI belongs in the Next.js application under `web/`. The existing Go-rendered UI is a migration fallback, not the destination architecture. Migrate route families incrementally and preserve auth/security behavior before cutover.

Flutter is the default application UI framework for both `app/` and `desktop/`; share Dart features where behavior is common, but allow device-class-specific shells.

Do not keep stacking unrelated CSS overrides into one giant file. Extract touched UI responsibilities incrementally into coherent tokens, components, scenes and feature modules.

## Documentation taxonomy

Target taxonomy is documented in `docs/architecture/REPOSITORY_STRUCTURE.md`.

New documents should use these categories:

- `docs/architecture/` — stable architecture and boundaries
- `docs/plans/` — implementation/master plans, grouped by domain
- `docs/guides/` — user/developer how-to guides
- `docs/operations/` — release/deploy/runbook material
- `docs/integrations/` — external platform/plugin material
- `docs/archive/` — superseded historical plans retained for traceability

Do not add new `*_PLAN.md` files to repository root.

## Change discipline

- Preserve public routes, CLI commands, MCP tool contracts and persistence formats unless the task explicitly changes them.
- Prefer small migration phases over repository-wide renames/moves mixed with behavior changes.
- Never combine a package-move refactor with unrelated security/runtime behavior changes in the same commit.
- Before moving a file, find repository references and update them atomically.
- After Go package moves, run the narrow affected tests first, then `go test ./...`.
- After release/script path changes, also run the relevant npm/release checks.

## Security/privacy boundary

Repository cleanup must never weaken CodeLocal's existing workspace authorization, approval, local-source privacy, tenant scoping, token/session or device security boundaries.
