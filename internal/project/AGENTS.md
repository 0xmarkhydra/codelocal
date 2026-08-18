# Project Intelligence Ownership

Scope: `internal/project/**`.

This package owns local project scanning, repository-aware project metadata, code graph/query/impact analysis, context construction and diagnostics.

Rules:

- Keep Cloud persistence in `internal/cloud`, MCP transport in `internal/mcpgateway`, and execution routing in `internal/localclient` / `internal/taskexecution`.
- Name files by intelligence responsibility: `project_*`, `repository_*`, `code_graph_*`, `context_*`, `diagnostics.go`.
- Do not split code-graph/query/impact into subpackages until shared public types and dependency direction are mapped; avoid Go import cycles caused by cosmetic decomposition.
- Preserve repository ownership/provenance on symbols, graph edges and verification context.
- New project intelligence should use existing repository identity and LSP abstractions rather than duplicating scanners.
- Run `go test ./internal/project ./internal/lsp` after changes and the localclient tests when repository routing metadata changes.
