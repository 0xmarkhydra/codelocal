# MCP Gateway Ownership

Scope: `internal/mcpgateway/**`.

This package owns the public MCP contract, compact tool surface, lifecycle/compatibility behavior, and MCP-facing orchestration adapters.

Rules:

- Treat tool names, schemas, annotations, surface hashes and compatibility behavior as public contract boundaries.
- Do not add direct OS/filesystem/browser implementation here; route execution through the owning local/runtime/automation domains.
- Keep compatibility, knowledge/context integration, lifecycle, and tool registration responsibilities explicit in filenames instead of adding catch-all operations files.
- Public schema/version changes require focused compatibility and tool-surface tests.
- Preserve old-client behavior unless a version/capability gate explicitly changes it.
- Run `go test ./internal/mcpgateway ./internal/mcphub` after changes and full CI for public tool-surface modifications.
