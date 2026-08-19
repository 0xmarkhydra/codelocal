# Cloud Server Ownership

Scope: `internal/cloudserver/**`.

This package owns authenticated/public HTTP transport, request validation/composition, browser-facing API contracts, the Go-to-Next presentation edge, runtime WebSocket/media endpoints, and release-facing Cloud routes.

Rules:

- Persistence/domain decisions belong in `internal/cloud`; browser presentation belongs in `web/`.
- Preserve public URLs and API contracts during structural refactors.
- Keep this package transport-focused: prefer files named by API/protocol responsibility such as `dashboard_api.go`, `code_graph_api.go`, `runtime_ws.go`, and `release_email.go`.
- Do not add server-rendered product HTML back into Go. Next.js is the canonical browser UI.
- Keep CSRF/auth/tenant checks at the existing authoritative Go boundaries; file moves must not bypass middleware.
- Run `go test ./internal/cloudserver` after transport/API changes and full CI when shared Cloud contracts change.
