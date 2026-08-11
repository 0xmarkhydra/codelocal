# CodeLocal

CodeLocal lets ChatGPT work on code that stays on your own machine.

```text
You
  ↓
ChatGPT                     AI reasoning layer
  ↓ MCP over HTTPS + OAuth
CodeLocal Cloud             Go gateway, routing, auth, multi-tenant state
  ↓ authenticated WebSocket
CodeLocal native runtime    Go binary on your computer
  ↓
Authorized workspaces       files, Git, terminal, local MCP extensions
```

**ChatGPT is the brain. CodeLocal gives it controlled hands.** You do not need to buy or configure a separate OpenAI API key for CodeLocal, and CodeLocal does not add per-token billing. Tool calls happen inside your ChatGPT conversation and follow the normal limits of your ChatGPT plan/model/workspace.

## Version

`1.5.2` — current stable native Go release.

The application runtime and Cloud gateway are implemented in Go. The npm distribution only keeps a tiny launcher that selects the correct prebuilt native binary for macOS, Linux or Windows.

## Why Go

The native runtime now uses one compiled process instead of a Node.js application tree. This reduces runtime dependency surface, avoids Node heap/watcher failure modes, improves concurrency for multiple workspaces and terminals, and makes Cloud deployment a small static-style Go service.

## Install

Requires Node.js 20+ and npm.

Install the current stable release:

```bash
npm i -g codelocal
```

Or install the beta channel:

```bash
npm i -g codelocal@beta
```

Verify the installed version:

```bash
codelocal --version
```

Authorize one or more projects:

```bash
cd /path/to/project-a
codelocal .

cd /path/to/project-b
codelocal .
```

Start one machine runtime:

```bash
codelocal
```

A single runtime can keep many workspaces authorized and activates each workspace lazily when ChatGPT needs it. You do **not** need one CodeLocal daemon per project.

Useful CLI commands:

```text
codelocal --version
codelocal status
codelocal workspaces
codelocal grant <path>
codelocal ungrant <path-or-id>
codelocal doctor <path>
codelocal stop
codelocal approvals list
codelocal mcp list
```

## Update

Update to the newest stable release:

```bash
npm i -g codelocal@latest
```

Short form (same stable channel):

```bash
npm i -g codelocal
```

Update to the newest beta release:

```bash
npm i -g codelocal@beta
```

After updating, verify and start CodeLocal again:

```bash
codelocal --version
codelocal
```

A normal npm update keeps your existing CodeLocal pairing credentials and authorized workspaces.

## Connect ChatGPT

Add the CodeLocal MCP endpoint in ChatGPT:

```text
https://codelocal.cloud/mcp
```

OAuth signs ChatGPT into your CodeLocal account. Pairing signs your local machine into the same account. ChatGPT can only route tools to devices/workspaces belonging to that account.

Typical first request:

```text
@CodeLocal inspect project_info first, understand this project, then make the requested change and run the relevant checks.
```

## Multi-workspace model

CodeLocal is intentionally designed for multiple simultaneous ChatGPT threads and projects.

```text
Machine runtime
├── Project A (sleeping/active)
├── Project B (sleeping/active)
└── Project C (sleeping/active)
```

The Cloud gateway routes by:

```text
user + device + workspace
```

MCP sessions can use `list_workspaces`, `select_workspace` and the returned `workspaceKey`. Selection is scoped to the MCP session, while passing `workspaceKey` keeps an individual call explicit and thread-safe.

## Native Go capabilities

### Files and retrieval

- workspace boundary enforcement and symlink escape protection
- `.gitignore`-aware listing/search
- sensitive-path blocking independent of `.gitignore`
- UTF-8/binary detection
- line-range and batch reads
- SHA-256 stale-write protection
- exact edits, unified patches and transactional structured edits
- scoped `AGENTS.md` discovery

### Project intelligence

- project/language/framework/manifests map
- symbols, definitions, references, hover-like structural context
- import/call graph fallback
- diagnostics and verification snapshots
- dependency inspection for Node, Go, Rust and Python
- installed formatter support for Go, Rust, Dart, Python, C/C++ and Node projects

The native structural engine always works without a language server. Dedicated LSP integration remains an incremental enhancement rather than a requirement for the coding loop.

### Terminal and Git

- concurrent non-PTY and PTY processes
- incremental output cursors
- stdin, resize, signals, cancellation and process listing
- deterministic command risk classification
- one-time ChatGPT approvals for critical actions
- workspace-scoped remembered approvals for repeatable reviewed actions
- local redacted terminal/audit history
- non-force Git stage/unstage/commit/push controls
- persistent idempotency journal for side-effecting tool calls

### Local MCP hub

CodeLocal can connect other MCP servers installed on the user's machine while keeping their configuration local-only.

```bash
codelocal mcp add <name> -- <command> [args...]
codelocal mcp add <name> --url <https://server/mcp>
codelocal mcp list
codelocal mcp search <query>
```

Secrets are referenced from environment variables; they are not copied to CodeLocal Cloud.

### Cloud

- Go HTTP/WebSocket server
- OAuth + PKCE for ChatGPT MCP authorization
- password sessions + CSRF protection
- device pairing credentials
- PostgreSQL durable account/device/workspace state
- Redis activation, rate-limit and cross-replica gateway routing
- lazy workspace activation
- horizontal gateway ownership/routing
- client-version update notices
- optional Go `pprof` endpoint behind `CODELOCAL_ENABLE_PPROF=1`

## Security model

`.gitignore` is a retrieval rule, not a security boundary. Sensitive locations such as private key material, credential stores and `.env` secrets remain blocked separately.

Terminal commands run on the host after CodeLocal policy checks. CodeLocal does not pretend that policy-only execution is an OS sandbox. Critical operations still require fresh confirmation; safe/reviewable repeat actions can be remembered only for the specific local workspace.

## Development

Requires Go 1.25+.

```bash
go test ./...
go vet ./...
go build ./cmd/...
```

Build the cross-platform npm staging package:

```bash
go run ./cmd/release
npm pack --dry-run ./.release/npm
```

Cloud container:

```bash
docker build -t codelocal-cloud .
```

The production container runs `cmd/codelocal-cloud`; Railway is configured through `railway.json`.

## Publishing a new npm release

Maintainers can publish stable or beta with one command:

```bash
npm run release:npm
```

The release helper asks for the channel:

```text
CodeLocal npm release

  1) latest (stable) [default]
  2) beta

Choose channel [1]:
```

Press **Enter** to publish `latest`, or choose `2` for `beta`.

The helper automatically:

- checks versions already published on npm
- chooses the next available version
- synchronizes `package.json`, `package-lock.json` and `internal/version/version.go`
- rebuilds `.release/npm`
- generates the correct install command in the npm README
- runs tests/vet and package validation
- publishes with the correct `latest` or `beta` npm dist-tag

For a stable release, the generated npm README contains:

```bash
npm i -g codelocal
```

For a beta release:

```bash
npm i -g codelocal@beta
```

## Release model

The public npm package remains `codelocal`. A release contains prebuilt native binaries for:

```text
darwin-arm64
darwin-x64
linux-arm64
linux-x64
windows-arm64
windows-x64
```

The launcher selects the binary matching `process.platform` and `process.arch`. This preserves the existing `npm i -g codelocal` UX while the actual CodeLocal application runs natively in Go.
