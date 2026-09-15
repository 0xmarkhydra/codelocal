# CodeLocal

CodeLocal is a universal MCP connection layer for AI coding. It connects ChatGPT, Codex, Claude and other MCP-compatible AI clients and agents to one durable Project Brain plus controlled access to project folders you explicitly authorize on your own machine.

> [!IMPORTANT]
> **Public repository status:** This public repository is currently being separated from CodeLocal's private Enterprise codebase. That extraction is being performed **manually**: components are reviewed, decoupled, cleaned up and moved into the public edition one by one rather than exported automatically from the Enterprise tree.
>
> **Why not copy the Enterprise source directly?** The Enterprise edition is designed for organizations that need to use AI inside security-sensitive business environments. Its codebase contains deployment assumptions, internal integrations, organization-wide controls, security hardening paths and implementation details that are specific to Enterprise operation. Publishing that codebase wholesale could expose attack surfaces, defensive assumptions or integration paths that attackers could study and then apply against Enterprise customers.
>
> For that reason, the public edition is not produced by simply copying the Enterprise repository and deleting a few private files. Public components are deliberately decoupled and, where necessary, re-implemented so that useful functionality can be opened without carrying over Enterprise-specific security architecture, customer-dependent behavior or internal operational coupling. This does not replace normal secure engineering: the public edition must still be secure and reviewable on its own.
>
> Because this is a manual separation process, reviewers may temporarily encounter historical names, compatibility files, source-only modules, Enterprise-era metadata or documentation that has not yet been fully normalized. Their presence does not by itself mean that those paths are part of the runtime shipped in the public npm package.
>
> The target date for completing this public-repository separation and cleanup is **October 19, 2026**. Until then, treat the shipping/runtime documentation and release builder as the source of truth when repository history or transitional structure appears inconsistent.

## What we are building

We are **not** trying to rebuild DeepSeek Harness, Codex, Claude Desktop, or another AI coding client.

The public edition of CodeLocal is intentionally optimized for **one person**. It gives an individual a durable personal Project Brain, reusable skills, project context and controlled execution across compatible AI clients without forcing that person to rebuild their working context in every new chat or tool.

Our broader Enterprise vision goes further: an **intelligent company** in which multiple people and AI agents can contribute specialized skills, experience and verified knowledge to a shared organizational intelligence.

That multi-user model — including team collaboration, collective learning from many people, shared organizational intelligence and company-wide skill coordination — belongs to the private Enterprise codebase and is **not part of the current public edition**.

For the public edition:

- **One person owns the intelligence.** Knowledge, experience and project context are optimized around an individual user.
- **Skills are reusable.** The user can teach the system workflows and capabilities that can be applied again across their own projects.
- **Work can become durable personal knowledge.** Verified decisions, project context and useful experience do not need to disappear when a chat ends.
- **AI clients are interfaces, not the product itself.** ChatGPT, Codex, Claude and other compatible agents can connect to the same personal intelligence and authorized projects.

The long-term Enterprise direction is to let people become specialized workers who can teach, package and sell skills to an organization's intelligence — but the public repository today focuses on making that model powerful and practical for **a single user first**.

## Build it with us

> **“Every child needs a village to grow.”**
>
> CodeLocal is still growing. If you find a bug, disagree with a design decision, have an idea, or can make any part of the project better, please contribute code and share feedback in the [CodeLocal Forums](https://codelocal.cloud/forums). We want the public edition to be shaped in the open by the people who actually use it.

`codelocal.cloud` should be treated as the CodeLocal team's current public/reference deployment, not as the only way this project should exist. We encourage developers to **fork the repository, study it, modify it, and build their own deployment** for their own needs and environments.

Please do not wait for CodeLocal to become “finished” before participating. Review the code, open issues, propose safer designs, send pull requests, challenge assumptions, and tell us what does not work. **Help us raise this project together.**

Website: [https://codelocal.cloud](https://codelocal.cloud/)

User documentation:

- [Start here](./docs/README.md)
- [User Guide](./docs/guides/USER_GUIDE.md)
- [How CodeLocal Works](./docs/guides/HOW_CODELOCAL_WORKS.md)
- [Security & Privacy](./docs/architecture/SECURITY_AND_PRIVACY.md)
- [Project Brain Evidence](./docs/architecture/PROJECT_BRAIN_EVIDENCE.md)
- [Security Policy](./SECURITY.md)
- [Contributing](./CONTRIBUTING.md)
- [Product Stack](./docs/architecture/PRODUCT_STACK.md)
- [Beta Channel](./docs/operations/BETA_CHANNEL.md)
- [OpenAI Plugin Submission Pack](./docs/integrations/openai/submission/README.md)

```text
You
  ↓
MCP-compatible AI clients    ChatGPT · Codex · Claude · other agents
  ↕ MCP over HTTPS + OAuth
CodeLocal Project Brain      durable rules, decisions, memory, Experience, skills
  ↓
CodeLocal Cloud              identity, routing, auth, sanitized durable project knowledge
  ↓ authenticated runtime channel
CodeLocal native runtime     controlled execution on your computer
  ↓
Authorized workspaces        files, Git, terminal and local MCP extensions
  ↓
Verification                 evidence → verified Experience → safe learning
```

**The AI client/model reasons; CodeLocal provides the shared MCP layer, preserves project intelligence and controls execution.** You can switch among MCP-compatible AI clients without rebuilding CodeLocal workspace authorization or Project Brain. For the ChatGPT/Codex plugin workflow, CodeLocal does not require a separate OpenAI API key and does not add per-token billing to those tool calls.

## Version

Use `codelocal --version` for the installed runtime. Stable installs use `codelocal@latest`; reviewer and canary builds use the npm beta channel until promoted.

The canonical backend, Cloud gateway, CLI and local runtime are implemented in Go. The browser product is migrating to Next.js + TypeScript, while Flutter + Dart is the accepted application stack for future mobile and desktop clients; OS-specific bridges remain native only where required. See [Product Stack](./docs/architecture/PRODUCT_STACK.md). The npm distribution only keeps a tiny launcher that selects the correct prebuilt native binary for macOS, Linux or Windows.

## Repository map

| Path | Ownership |
| --- | --- |
| `cmd/`, `internal/` | Canonical Go backend, CLI and local runtime |
| `web/` | Next.js browser product |
| `deploy/` | Dockerfiles, Railway configs and deployment assets |
| `tools/`, `scripts/` | Build, release and developer tooling |
| `docs/` | Architecture, guides, operations and plans |
| `legacy/typescript-runtime/` | Quarantined compatibility code; not shipped runtime |

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
npm i -g codelocal
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

On first start, CodeLocal explains and asks separately for two optional capabilities:

- **Browser Automation** gives connected MCP clients the compact `browser` tool for opening, inspecting, clicking, typing and taking screenshots in an isolated website session. Enabling it downloads one managed Chromium browser once; choosing No does not affect Coding.
- **Computer Use** gives connected MCP clients the compact `computer` tool for inspecting and controlling desktop apps through a bundled native helper. It does not download a browser, remains opt-in, requires the operating system's screen/accessibility permissions, and still applies action-level approvals.

Run `codelocal setup` to change either choice later. Run `codelocal doctor` at any time to see whether the browser, native helper and required capabilities are ready.

A single runtime can keep many workspaces authorized and activates each workspace lazily when an MCP session needs it. You do **not** need one CodeLocal daemon per project or per AI client.

Useful CLI commands:

```text
codelocal --version
codelocal setup
codelocal status
codelocal workspaces
codelocal grant <path>
codelocal ungrant <path-or-id>
codelocal doctor <path>
codelocal stop
codelocal reset --all
codelocal uninstall --all
codelocal approvals list
codelocal mcp list
```

Reset all local state while keeping the CLI installed:

```bash
codelocal reset --all
```

This stops the runtime and removes the local login, workspace grants, approvals, indexes, history, managed browser runtime and capability choices. The next `codelocal` run starts first-time setup again. Use `--yes` only for intentional non-interactive cleanup.

Remove both local state and the globally installed npm package:

```bash
codelocal uninstall --all
```

Operating-system privacy permissions such as macOS Accessibility and Screen Recording remain controlled by the OS and are not silently changed.

## Update

When you run `codelocal`, the CLI performs a short cached check against the configured npm release channel. If a newer CodeLocal runtime/tool release is available, it prints the installed/latest versions and the exact npm update command before starting. The result is cached privately under CodeLocal state so normal startup does not repeatedly wait on npm, and offline/update-check failures never stop the runtime. Set `CODELOCAL_UPDATE_CHECK=0` to disable this notice.

CodeLocal fingerprints the public MCP tool surface. If an already-open MCP session calls an old granular tool, CodeLocal translates the call when possible and returns `CODELOCAL_TOOL_SCHEMA_STALE`; if a client calls a tool name that does not exist in the current surface, the gateway returns `CODELOCAL_TOOL_SCHEMA_MISMATCH`. In either case, refresh or reconnect CodeLocal in that AI client so it scans the current tool/action schema. Reconnecting does not remove the local device pairing or workspace grants. Runtime capability mismatches are reported separately and include the installed client version plus the npm update command when a newer client is available.

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

## Connect an MCP-compatible AI client

CodeLocal exposes one remote MCP endpoint:

```text
https://codelocal.cloud/mcp
```

For ChatGPT and Codex, use the CodeLocal Plugin from the OpenAI directory when available; reviewer/developer flows can use the endpoint directly. Other MCP-compatible AI clients and coding agents can add the same remote endpoint when they support authenticated remote MCP.

OAuth signs the AI client into your CodeLocal account. Pairing signs your local machine into the same account. A connected client can only route tools to devices/workspaces belonging to that account.

Typical first request:

```text
@CodeLocal inspect project_info first, understand this project, then make the requested change and run the relevant checks.
```

## Multi-workspace model

CodeLocal is intentionally designed for multiple simultaneous MCP sessions, AI clients and projects.

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

MCP sessions use `workspace(action=list)`, `workspace(action=select)` and the returned `workspaceKey`. Selection is scoped to the MCP session, while passing `workspaceKey` keeps an individual call explicit and thread-safe.

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

### Compact MCP tool surface

The Cloud gateway exposes exactly 19 domain tools: `device`, `workspace`,
`project`, `context`, `agent`, `read`, `search`, `dependency`, `lsp`, `edit`,
`verify`, `git`, `terminal`, `process`, `approvals`, `security`, `mcp`, `browser`
and `computer`.

The 98 granular runtime commands are not advertised as top-level tools and cannot be
re-enabled by configuration. Their granular command names remain only as the
private Cloud-to-native runtime protocol, so capability is preserved without
adding model-facing schema or tool-selection cost.

Performance-sensitive defaults are intentionally bounded:

- `CODELOCAL_READ_CONCURRENCY=6` controls parallel workers inside `read_files`;
- `CODELOCAL_USAGE_LOCAL_QUEUE_SIZE=8192` controls the non-blocking usage ingress;
- `CODELOCAL_USAGE_STREAM_MAXLEN=1000000` bounds the approximate Redis Stream length.

Usage writes never block an MCP response. They flow through the local queue and
Redis Stream to a single leased, idempotent PostgreSQL batch consumer. File and search reads can fan out; edits, Git mutations and process writes remain ordered.

Validate and build the cross-platform npm CLI staging package without publishing:

```bash
npm run release:npm:prepare
```

This npm-specific gate checks repository structure and Go code, builds `.release/npm`, and runs `npm pack --dry-run`. It intentionally does not install, lint, typecheck, build, or audit the web applications. To run the full repository gate including web checks, use `npm run release:prepare`.

Inspect an already generated package with:

```bash
npm pack --dry-run ./.release/npm
```

Cloud container:

```bash
docker build -t codelocal-cloud .
```

The production container runs `cmd/codelocal-cloud`; Railway is configured through `deploy/railway/backend.json`.

## Publishing a new npm release

The canonical maintainer command—and the default meaning of publishing/releasing npm in this repository—is:

```bash
npm run release:npm
```

This command publishes the CLI package only. It does not install, lint, typecheck, build, or audit either web application. Use `npm run release:npm:prepare` only when explicitly validating/building the npm package without publishing; use `npm run release:prepare` only when explicitly requesting the full repository gate including web.

The release helper asks for the channel:

```text
CodeLocal npm release

  1) latest (stable) [default]
  2) beta

Choose channel [1]:
```

Press **Enter** or choose `1` for the production `latest` release. Choose `2` only for `beta`.
Do not run a standalone `npm publish` for the normal release flow.

The helper automatically:

- checks versions already published on npm
- chooses the next available version
- synchronizes `package.json`, `package-lock.json` and `internal/version/version.go`
- rebuilds `.release/npm`
- generates the correct install command in the npm README
- runs tests/vet and package validation
- publishes with the correct `latest` or `beta` npm dist-tag
- verifies that npm exposes the published version and selected dist-tag

See the [npm release operations runbook](docs/operations/NPM_RELEASE.md) for validation-only commands, post-release verification, and failure handling.

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

## License

CodeLocal is licensed under the [Apache License 2.0](./LICENSE). Unless a file or directory states otherwise, source code in this repository may be used, modified, and distributed under the terms of that license. Third-party components retain their respective licenses.
