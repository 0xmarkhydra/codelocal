# Legacy TypeScript Runtime Instructions

These instructions apply to `legacy/typescript-runtime/**` and override the repository root instructions for this subtree.

## Status

This tree is quarantined compatibility/history code. The shipping CodeLocal runtime is the native Go implementation under `cmd/` and `internal/`.

## Rules

- Do not add new product/runtime behavior here.
- Do not use this tree as the implementation target for a new feature just because it is named `src/`.
- Modify files here only for an explicitly requested legacy compatibility fix, retirement evidence, or deletion/migration task.
- Do not wire root build/test/release scripts back to legacy entrypoints.
- Do not move or delete this tree until the physical deletion gate in `README.md` is satisfied with external evidence.
- New production equivalents belong in the owning Go domain under `internal/` or an executable composition under `cmd/`.
