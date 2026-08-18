# Cloud Domain Ownership

Scope: `internal/cloud/**`.

This package owns CodeLocal Cloud domain state and persistence. It must not own HTTP rendering or browser UI.

Rules:

- Keep HTTP handlers, route parsing and HTML in `internal/cloudserver` / `internal/ui`.
- Group new files by stable subdomain responsibility: auth/session/security store; device/workspace/runtime state; Project Brain/knowledge/canonical graph; semantic retrieval/embeddings; learning/outbox/promotion/experience/skills; organization/usage/admin.
- Do not create generic store/helper files that mix unrelated domains.
- Extract a subpackage only when callers, dependency direction and tests demonstrate a stable boundary; avoid cosmetic moves that introduce Go import cycles.
- Preserve tenant scoping, migration compatibility and persistence formats during structural work.
- Run the narrow affected cloud tests first, then `go test ./internal/cloud` and full CI when schemas or shared contracts change.
