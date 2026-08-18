# Automation Domain Ownership

Scope: `internal/automation/**`.

This package owns browser/computer automation models, scheduling, semantic actions, scene/state handling, verification and automation safety policy integration.

Rules:

- Native executable/platform helper implementation belongs under `cmd/computerhelper` or `cmd/computernative`, not here.
- Keep browser, computer control, scene/state, input safety and scheduling responsibilities explicit in filenames; do not create a generic automation helper bucket.
- Physical-input fallback, focus behavior and sensitive-input handling must remain governed by the existing approval/security policy.
- Prefer background semantic control over new foreground/physical behavior unless the product contract explicitly requires otherwise.
- Extract subpackages only after controller/model dependency direction is proven and tests can move with the boundary.
- Run `go test ./internal/automation ./cmd/computerhelper` after structural or behavior changes.
