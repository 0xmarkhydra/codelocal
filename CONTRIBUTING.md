# Contributing to CodeLocal

Thanks for helping improve CodeLocal. The public repository is being progressively separated from a private Enterprise codebase, so contributions should strengthen the single-user public edition without accidentally coupling it back to private-only organization/team behavior.

This separation is a **manual extraction and cleanup process**, not an automatic mirror of the Enterprise repository. Transitional names, compatibility files and Enterprise-era metadata may therefore remain visible while individual components are reviewed and decoupled. The target date for completing the public-repository separation is **October 19, 2026**.

## Public-edition scope

The public product is optimized for one user with a durable personal Project Brain, reusable skills and controlled local execution through MCP-compatible AI clients. Team collaboration, multi-person collective learning and organization-wide intelligence are not promises of the current public edition.

Before implementing a large feature, prefer a small issue or proposal that explains the user problem, public-edition boundary, security impact and verification plan.

## Development

Canonical runtime/backend code is Go under `cmd/` and `internal/`. The browser product is Next.js + TypeScript under `web/`. `legacy/typescript-runtime/` is quarantined compatibility/history code and must not receive new production runtime behavior.

Useful checks:

```bash
go test ./cmd/... ./internal/...
go vet ./cmd/... ./internal/...
npm run check:repo
```

For web changes, also run the relevant checks under `web/`. For npm packaging changes, use the repository release preparation command instead of hand-building a package.

## Change expectations

Keep workspace/security boundaries fail-closed, do not treat `.gitignore` as a security control, preserve secret redaction, add focused regression tests for behavior changes, and update public docs when a shipped capability or limitation changes.

Do not commit credentials, real user data, generated local state or private Enterprise-only material.

## Security reports

Do not use public issues for vulnerabilities. Follow [`SECURITY.md`](./SECURITY.md) and report security-sensitive findings privately to `security@codelocal.cloud`.
