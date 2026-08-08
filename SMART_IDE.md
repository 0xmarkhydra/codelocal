# CodeLocal Smart IDE (dev branch)

This branch experiments with IDE-grade workspace intelligence while keeping `main` stable.

## Goals

- Prefer targeted context over broad repository scans.
- Keep code intelligence fresh after edits, IDE changes and generated code.
- Support monorepos/polyglot workspaces without rebuilding one giant AST.
- Use language servers when available, with bounded text/AST fallback.
- Preserve CodeLocal safety, approval, audit and sandbox layers.

## Current intelligence pipeline

```text
Task from ChatGPT
  -> ProjectContextEngine
  -> persistent WorkspaceIntelligenceIndex
       - file metadata
       - lightweight symbols
       - imports
       - recent changes
       - dependency neighbors
       - nested manifests/workspace roots
  -> task ranking
  -> SemanticRouter
       - TypeScript multi-project compiler index
       - Dart Analyzer LSP
       - Pyright / rust-analyzer / gopls / clangd / jdtls / Kotlin / Lua / SourceKit / ZLS when installed
       - ripgrep fallback
  -> targeted files/ranges
  -> edit
  -> invalidate lazily
  -> verify / diagnostics / diff
```

## Workspace intelligence

`src/workspace-index.ts` maintains a bounded index and persists only metadata-derived information under:

```text
~/.codelocal/indexes/
```

The cache contains file paths, sizes/mtimes, lightweight symbol names, import specifiers and ranking tokens. It does **not** persist source file contents.

On refresh, unchanged files reuse the cached parsed metadata. Changed files are re-read and re-indexed. Deleted/new files are detected by the bounded workspace scan.

Important environment knobs:

```text
CODELOCAL_INDEX_MAX_FILES=12000
CODELOCAL_INDEX_MAX_DEPTH=8
CODELOCAL_INDEX_MAX_FILE_BYTES=393216
CODELOCAL_INDEX_FRESHNESS_MS=1500
```

## TypeScript / JavaScript monorepos

The dev branch no longer requires a `tsconfig.json` at `PROJECT_ROOT`. It searches nested projects and creates multiple bounded TypeScript Programs.

Limits:

```text
CODELOCAL_TS_MAX_PROJECTS=20
CODELOCAL_TS_CONFIG_DEPTH=6
CODELOCAL_TS_MAX_FILES_PER_PROJECT=4000
CODELOCAL_TS_MAX_TOTAL_FILES=12000
```

This avoids the old failure mode where a generic workspace accidentally became one enormous TypeScript Program.

## Flutter / Dart

When `dart` is available on `PATH`, `.dart` files route to:

```text
dart language-server --protocol=lsp
```

This provides file-position definition/reference/implementation/hover/diagnostics through the same semantic tools already exposed by CodeLocal.

## Task context ranking

The existing MCP tool `context_for_task` now returns ranked files rather than only plain symbol matches. Ranking considers:

- filename/path term matches;
- lightweight declared symbols;
- imported modules;
- recent file changes;
- entrypoints/manifests/tests;
- import graph neighbors;
- semantic provider results.

The gateway API does not need a new tool name, so a `dev` local client can still connect to the existing production gateway while this branch is tested.

## Safety

The smart index excludes sensitive paths using the existing CodeLocal sensitive-path policy. Existing shell policy, approvals, audit logs and macOS developer host-mode behavior remain in place.

## Test locally

```bash
cd ~/Documents/codex-mcp
git fetch origin
git switch dev
git pull origin dev
npm install
npm run typecheck
npm test

node dist/cli.js start "$HOME/Desktop/BIDDI"
```

Then from ChatGPT, start with:

```text
project_info
context_for_task("<your task>")
```

For Flutter/Dart, verify `semantic_info` reports `dart-analyzer` installed, then test `find_definition`, `find_references`, `get_hover` and `get_diagnostics` on a `.dart` file.

## Still intentionally not claimed as complete

- embedding/vector semantic search;
- full checker-identity call graph for every language;
- IDE UI/accessibility integration;
- production persistence on the Railway side;
- full Cursor-equivalent feature parity.

The dev branch is the place to iterate on these without destabilizing `main`.
