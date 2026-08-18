# Cloud Server Ownership

Scope: `internal/cloudserver/**`.

This package owns authenticated/public HTTP transport, request validation/composition, dashboard handlers, runtime WebSocket/media endpoints, and release-facing Cloud routes.

Rules:

- Persistence/domain decisions belong in `internal/cloud`; shared rendering primitives belong in `internal/ui`.
- Preserve public URLs and API contracts during structural refactors.
- Name user-facing files by surface, for example `dashboard_handler.go`, `knowledge_dashboard.go`, `code_graph_dashboard.go`, `usage_dashboard.go`, `public_pages.go`; name transports by protocol such as `runtime_ws.go`.
- Keep CSRF/auth/tenant checks at the existing authoritative boundaries; file moves must not bypass middleware.
- Avoid a generic catch-all UI/server file. Extract a cohesive surface only after route/caller ownership is clear.
- Run `go test ./internal/cloudserver ./internal/ui` after dashboard/presentation changes and full CI when shared Cloud contracts change.
