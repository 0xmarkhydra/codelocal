# CodeLocal / codex-mcp

Remote MCP coding bridge for ChatGPT/Codex.

```text
ChatGPT / Codex
    | MCP over HTTPS + OAuth
    v
Railway gateway
    | authenticated WebSocket
    v
CodeLocal client on your machine
    | local workspace tools
    v
PROJECT_ROOT
```

The model remains the reasoning layer. CodeLocal provides local filesystem access, selective retrieval, semantic code intelligence, Git, diagnostics/tests, guarded shell/process execution, approvals, routing and observability.

## Current version

`1.5.0-beta.1`

This preview is intentionally conservative: sensitive paths remain blocked, risky shell operations require local approval, and OS-level sandboxing/production device-pairing are not claimed as complete yet.

## Major capabilities

### Workspace + retrieval

- `.gitignore`-aware listing/search
- targeted reads of ignored dependency/generated files when explicitly requested
- sensitive-path policy independent from `.gitignore`
- binary detection
- file metadata, SHA-256 hash and mtime
- line-range reads
- conflict-safe write/edit with `expectedHash`
- scoped `AGENTS.md`/repo instructions

### Dependencies

- inspect installed Node dependency metadata
- targeted dependency file reads
- dependency-only search

### Semantic code intelligence

TypeScript/JavaScript semantic index backed by the TypeScript compiler API:

- symbols
- definitions/declarations
- references
- callers/callees
- import/export graph
- TypeScript diagnostics

Other languages continue to work through filesystem/search/shell/toolchain commands; dedicated semantic backends can be added progressively.

### Tests + Git

- detect likely test/lint/typecheck/build commands
- find likely related tests
- run affected test command
- `git status`
- `git diff`
- `git log`
- `git show`
- `git blame`
- per-file history

### Runtime

- guarded local shell
- incremental process output cursors
- real-time stdout/stderr mirrored to the local terminal
- process list/stdin/kill
- local approval prompts for package changes, Git writes, migrations, network commands and recursive deletes
- hard blocks for obvious credential/system/disk escape commands unless explicitly unsafe mode is enabled

### Multi-device / multi-workspace routing

A client registers:

```text
deviceId + workspaceId + workspaceName
```

MCP sessions can use:

```text
list_devices
list_workspaces
select_workspace
workspace_info
```

If exactly one workspace is online it is selected implicitly. If more than one is online the model must select one explicitly.

## Server

Production test server currently deployed on Railway:

```text
https://codex-mcp-production.up.railway.app/mcp
```

ChatGPT -> server authentication uses OAuth.

Local client -> server authentication currently uses `DEVICE_TOKEN`. This is still preview-level device auth; production per-device pairing/rotation is a later hardening step.

## Install client

```bash
git clone https://github.com/0xmarkhydra/codex-mcp.git
cd codex-mcp
npm install
```

For an existing clone:

```bash
cd ~/Documents/codex-mcp
git pull
npm install
```

## Run client against a project

Example:

```bash
PROJECT_ROOT="$HOME/Desktop/BIDDI" \
SERVER_URL="wss://codex-mcp-production.up.railway.app/client" \
DEVICE_TOKEN="YOUR_DEVICE_TOKEN" \
CODELOCAL_ALLOW_SHELL=1 \
CODELOCAL_DEVICE_ID="macbook-pro" \
CODELOCAL_WORKSPACE_ID="biddi" \
CODELOCAL_WORKSPACE_NAME="BIDDI" \
npm run client
```

Useful optional variables:

```text
CODELOCAL_APPROVAL_MODE=prompt   # default: prompt; other supported preview values: deny, auto
CODELOCAL_ALLOW_DANGEROUS=0      # default; do not enable casually
CODELOCAL_MIRROR_PROCESS_OUTPUT=1
CODELOCAL_LOG_LEVEL=info
```

Expected startup logs are JSON structured events such as:

```text
client.started
client.connecting
client.registered
```

When ChatGPT runs a command, stdout/stderr also appears locally with a process prefix.

## ChatGPT setup

Create/connect the developer MCP app with:

```text
https://codex-mcp-production.up.railway.app/mcp
```

Authentication: OAuth.

After a tool/schema update, reconnect/refresh the MCP app so ChatGPT discovers the latest tool list.

## Recommended first prompt

```text
Use CodeLocal.
Inspect project_info and repo instructions first.
Analyze the architecture before editing.
Use semantic/reference tools and targeted reads rather than reading the entire repository.
Run appropriate diagnostics/tests, then show git diff.
Do not perform risky operations unless needed.
```

## Retrieval policy

`.gitignore` controls normal retrieval/indexing, not security.

```text
Normal source
  -> list/search normally

Ignored dependency/build/cache
  -> excluded from normal scans
  -> targeted read/search allowed when needed

Sensitive credentials/secrets
  -> blocked independently of .gitignore
```

Typical blocked sensitive paths include `.env*` (except templates/examples), `.ssh`, `.aws`, `.gnupg`, private key files and obvious credential files.

## Safety notes

The current client enforces workspace path boundaries for filesystem tools and applies command-policy checks for shell commands. A shell process is still a local OS process, so this preview should not be described as a complete native OS sandbox.

Do not expose device credentials publicly. Rotate the preview `DEVICE_TOKEN` before broader use.

## What is still not claimed as production-complete

- native OS sandbox parity across macOS/Linux/Windows
- fully persistent per-device pairing/revocation database
- true PTY resize/control parity for all interactive terminal applications
- dedicated LSP daemon integrations for every language
- durable server state across replicas/restarts

These are intentionally separated from the already-working coding loop rather than faked behind tool names.

## Coding loop target

```text
understand task
-> project_info + instructions
-> semantic symbols/references/import graph
-> targeted file/range/dependency reads
-> diagnostics/build/tests
-> edit with hash/conflict protection
-> re-run diagnostics/tests
-> git diff/history
-> explain result
```

See `ROADMAP.md` for design rationale and hardening work.
