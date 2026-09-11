# CodeLocal Smart MCP Intelligence (dev branch)

`dev` experiments with IDE-grade local code intelligence while `main` stays stable.

## Non-negotiable architecture

CodeLocal is **not an autonomous coding agent**. ChatGPT remains the planner/reasoning layer and decides which MCP tool to call next.

```text
User
  -> ChatGPT (reason / plan / decide)
       -> MCP read/intelligence call
            -> CodeLocal may refresh index, query LSP, traverse graph and read bounded context
            -> structured result back to ChatGPT
       -> ChatGPT decides whether another read, edit, command, Git action or rollback is needed
       -> explicit MCP side-effect call
            -> policy / approval / audit
            -> local execution
            -> result back to ChatGPT
```

Read-only intelligence may perform internal deterministic work inside one MCP call. There are **no autonomous hidden side effects**.

Every edit, delete, shell command, dependency install, Git write, developer-host command or rollback must originate from an explicit MCP request and retain its request ID through policy, approval, audit and execution.

## v1.2 intelligence pipeline

```text
ChatGPT calls context_for_task(task)
  -> native recursive workspace events
       - macOS FSEvents
       - Linux inotify
       - Windows native watcher
       - bounded Chokidar fallback
  -> persistent WorkspaceIntelligenceIndex
       - file metadata
       - lightweight symbols
       - imports
       - local package identities
       - recent changes
       - dependency / reverse-dependency neighbors
       - nested manifests / workspace roots
  -> task ranking
  -> SemanticRouter
       - bounded TypeScript multi-project compiler index
       - nearest-project-root LSP routing
       - Dart Analyzer / Pyright / rust-analyzer / gopls / clangd / jdtls / Kotlin / Lua / SourceKit / ZLS when installed
       - circuit breaker for broken server/root pairs
       - ripgrep fallback
  -> diagnostics + graph edges
  -> bounded source snippets around relevant symbols/errors
  -> Context Packet returned to ChatGPT
```

`context_for_task` is deliberately read-only. It does not edit files, run tests or start Flutter on its own.

## Native workspace events

`legacy/typescript-runtime/src/client-entry-v2.ts` installs `legacy/typescript-runtime/src/native-watcher.ts` before `client-v2` loads. Existing Chokidar consumers transparently use `@parcel/watcher` when available.

The preferred native backends are:

```text
macOS   -> fs-events
Linux   -> inotify
Windows -> windows
```

The watcher is recursive and ignores generated/vendor directories. If the native watcher cannot start, CodeLocal falls back to a bounded Chokidar watch instead of recursively polling the whole repository.

The persistent index remains a second freshness layer: the next task-context refresh compares file size/mtime and re-indexes changed files, including deep files missed by a fallback watcher.

## Workspace intelligence

`legacy/typescript-runtime/src/workspace-index.ts` persists metadata-derived information under:

```text
~/.codelocal/indexes/
```

It stores paths, size/mtime, lightweight symbols, imports, package names and ranking tokens. It does **not** persist source file contents.

The index understands local Flutter package imports such as:

```dart
import 'package:biddi_mobile/features/profile.dart';
```

and maps them back to the package's local `pubspec.yaml -> lib/...` source when that package is inside the workspace.

Important limits:

```text
CODELOCAL_INDEX_MAX_FILES=12000
CODELOCAL_INDEX_MAX_DEPTH=8
CODELOCAL_INDEX_MAX_FILE_BYTES=393216
CODELOCAL_INDEX_FRESHNESS_MS=1500
```

## Per-root language intelligence

Language servers are selected by file and nearest project marker rather than forcing one LSP root for the whole repository.

Examples:

```text
apps/mobile/lib/a.dart
  -> nearest pubspec.yaml
  -> Dart Analyzer rooted at apps/mobile

services/api/main.py
  -> nearest pyproject.toml
  -> Pyright rooted at services/api

crates/payments/src/lib.rs
  -> nearest Cargo.toml
  -> rust-analyzer rooted at crates/payments
```

Clients are reused by `(server, projectRoot)`. Failed pairs are temporarily circuit-broken instead of respawning in a loop. LSP document state is reset after a server restart so a new server never receives `didChange` before `didOpen`.

Supported position intelligence includes definition, references, implementations, hover, diagnostics and LSP call hierarchy where the server supports it.

## TypeScript / JavaScript monorepos

The TypeScript index searches bounded nested `tsconfig.json` / `jsconfig.json` projects instead of building one enormous program from a generic root.

```text
CODELOCAL_TS_MAX_PROJECTS=20
CODELOCAL_TS_CONFIG_DEPTH=6
CODELOCAL_TS_MAX_FILES_PER_PROJECT=4000
CODELOCAL_TS_MAX_TOTAL_FILES=12000
```

## Context Packet

A single `context_for_task` call now returns a bounded packet containing, when available:

- project/workspace roots and commands;
- ranked files with scores and reasons;
- semantic symbols;
- diagnostics;
- relevant import/dependency graph edges;
- recent-change information;
- source snippets centered around the relevant symbol or diagnostic;
- explicit context budget usage.

Defaults:

```text
CODELOCAL_CONTEXT_MAX_CHARS=48000
CODELOCAL_CONTEXT_SNIPPET_CHARS=7000
CODELOCAL_CONTEXT_FILES=8
```

ChatGPT should use this packet before broad scans, then request exact definitions/references/larger ranges only when the packet is insufficient.

## Side effects remain separate MCP calls

Recommended reasoning flow:

```text
context_for_task
  -> optional semantic/read calls
  -> ChatGPT decides change
  -> snapshot_diagnostics
  -> apply_edits / edit_file / apply_patch
  -> verify_changes
  -> ChatGPT decides done / further patch / explicit rollback when available
```

Long-running commands such as `flutter run` remain explicit process MCP calls. macOS Flutter/Xcode developer-host execution still goes through the existing policy and approval layer.

## Test locally

```bash
cd ~/Documents/codex-mcp
git fetch origin
git switch dev
git pull origin dev
npm install
npm run typecheck
npm test

node -p "require('./package.json').version"
node dist/cli.js start "$HOME/Desktop/BIDDI"
```

Expected development version:

```text
1.2.0-dev.0
```

Then ask ChatGPT to begin a coding task using `context_for_task` and inspect whether the returned packet contains the expected Flutter/Dart files, symbols, diagnostics and graph neighbors.

## Next slices after v1.2 core validation

- selective content-addressed workspace snapshots and restore;
- per-file mutation locks / compare-and-swap transactions;
- context epoch/delta for session workspace state;
- richer permission rules by action + resource + workspace/session;
- optional embedding retrieval as a supplement, never a replacement for LSP/graph/lexical evidence;
- high-level MCP metadata/annotations once the experimental client behavior is validated against the production gateway.

These remain MCP capabilities. ChatGPT continues to orchestrate them.
