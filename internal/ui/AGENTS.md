# UI Package Ownership

Scope: `internal/ui/**`.

This package owns shared server-rendered presentation primitives, page shells, visual tokens/styles, icons, graph rendering helpers, and reusable HTML components.

Rules:

- Keep persistence, SQL, authentication/session decisions, tenant logic and HTTP request orchestration out of this package.
- `internal/cloudserver` owns request/data composition and passes presentation-ready data here.
- Name files by presentation responsibility: `page_shell.go`, `product_pages.go`, `neural_graph.go`, and future `tokens.go`, `icons.go`, `forms.go`, `motion.go` when extraction is justified.
- Do not add another unrelated CSS override block to a giant file when the touched responsibility can be extracted cleanly without changing rendered behavior.
- Preserve existing public rendering functions and HTML/CSS behavior during structural refactors unless the task explicitly changes UX.
- Run `go test ./internal/ui` after changes, and include `internal/cloudserver` tests when shared dashboard output changes.
