# CodeLocal

**One Project Brain. Any AI client. Your machine.**

[![npm](https://img.shields.io/npm/v/codelocal?label=npm)](https://www.npmjs.com/package/codelocal)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](./LICENSE)
[![Go](https://img.shields.io/badge/runtime-Go-00ADD8.svg)](https://go.dev/)
[![MCP](https://img.shields.io/badge/protocol-MCP-5A45FF.svg)](https://modelcontextprotocol.io/)

CodeLocal is a universal MCP layer for AI coding. It connects ChatGPT, Codex, Claude and other MCP-compatible clients to one durable **Project Brain** plus controlled access to the project folders you explicitly authorize on your own machine.

The model reasons. **CodeLocal remembers, routes and executes.**

## Why CodeLocal

Most AI coding tools treat each chat, client or machine as a fresh start. CodeLocal is designed around the opposite idea: your project intelligence should survive the interface you happen to use today.

- **One Project Brain** — preserve verified decisions, project context, rules, Experience and reusable skills.
- **Any compatible AI client** — use ChatGPT, Codex, Claude or another MCP-capable agent against the same project intelligence.
- **Controlled local execution** — files, Git, terminal and optional automation run against workspaces you explicitly authorize.
- **Multi-workspace by design** — one local runtime can keep many projects available and activate them lazily.
- **Local-first source access** — repository source and local indexes stay on your machine by default; Cloud is used for identity, routing and sanitized durable continuity.

We are **not** trying to rebuild Codex, Claude Desktop, DeepSeek Harness, Cursor or another AI coding client. AI clients are interfaces. CodeLocal is the shared intelligence and execution layer behind them.

## How it works

```text
You
  ↓
MCP-compatible AI clients    ChatGPT · Codex · Claude · other agents
  ↕ MCP over HTTPS + OAuth
CodeLocal Project Brain      durable rules · decisions · Experience · skills
  ↓
CodeLocal Cloud              identity · routing · auth · sanitized continuity
  ↓ authenticated runtime channel
CodeLocal native runtime     controlled execution on your computer
  ↓
Authorized workspaces        files · Git · terminal · local MCP extensions
  ↓
Verification                 evidence → verified Experience → safer reuse
```

**The AI client/model reasons; CodeLocal provides the shared MCP layer, preserves project intelligence and controls execution.**

## Quick start

Requires Node.js 20+ and npm.

Install the current stable release:

```bash
npm i -g codelocal@latest
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

A single runtime can keep many workspaces authorized. You do **not** need one CodeLocal daemon per project or per AI client.

Useful commands:

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

## Connect an MCP-compatible AI client

The current hosted MCP endpoint is:

```text
https://codelocal.cloud/mcp
```

For ChatGPT and Codex, use the CodeLocal Plugin from the OpenAI directory when available; reviewer/developer flows can use the endpoint directly. Other MCP-compatible clients can add the same remote endpoint when they support authenticated remote MCP.

OAuth signs the AI client into your CodeLocal account. Pairing signs your local machine into the same account. A connected client can only route tools to devices and workspaces belonging to that account.

Typical first request:

```text
@CodeLocal inspect project_info first, understand this project,
then make the requested change and run the relevant checks.
```

## Core capabilities

### Project Brain

- durable single-user project knowledge;
- verified Experience promotion and reuse;
- reusable learned skills;
- project rules, decisions and constraints;
- continuity across compatible clients and paired machines when Cloud continuity is available.

See [Project Brain Evidence](./docs/architecture/PROJECT_BRAIN_EVIDENCE.md).

### Files and code intelligence

- workspace boundary enforcement and symlink escape protection;
- `.gitignore`-aware listing and search;
- sensitive-path blocking independent of `.gitignore`;
- UTF-8/binary detection and ranged reads;
- SHA-256 stale-write protection;
- exact edits, patches and transactional structured edits;
- project/language/framework mapping;
- symbols, definitions, references, diagnostics and structural call/import context;
- dependency inspection for Node, Go, Rust and Python.

The native structural engine works without requiring a language server. Dedicated LSP integrations add depth where available.

### Terminal and Git

- guarded host command execution;
- concurrent process management and PTY support where available;
- incremental stdout/stderr, stdin, resize, signals and cancellation;
- deterministic command risk classification;
- approval gates for sensitive operations;
- local redacted terminal/audit history;
- non-force Git stage, commit and push controls;
- idempotency protection for side-effecting tool calls.

### Local MCP hub

CodeLocal can connect other MCP servers installed on the user's machine while keeping their configuration local.

```bash
codelocal mcp add <name> -- <command> [args...]
codelocal mcp add <name> --url <https://server/mcp>
codelocal mcp list
codelocal mcp search <query>
```

Secrets should be referenced from environment variables instead of being copied into CodeLocal Cloud.

### Optional automation

On first start, CodeLocal can enable two optional capabilities separately:

- **Browser Automation** — isolated Chromium sessions for opening, inspecting, clicking, typing and screenshots.
- **Computer Use** — native desktop control through operating-system accessibility/screen permissions.

Run `codelocal setup` to change these choices later and `codelocal doctor` to inspect readiness.

## Security model

`.gitignore` is a retrieval rule, not a security boundary. Sensitive locations such as private keys, credential stores and `.env` secrets are handled separately.

Terminal commands execute on the user's host after CodeLocal policy and approval checks. CodeLocal does **not** claim that command policy alone is an OS sandbox. Browser Automation and Computer Use are optional and should be enabled only where that trust model is acceptable.

Raw repository source, local indexes, secrets and machine bindings remain local by default. The supported remote MCP flow, account/device routing and cross-device Project Brain continuity currently depend on CodeLocal Cloud.

Read the full [Security & Privacy](./docs/architecture/SECURITY_AND_PRIVACY.md), [Security Policy](./SECURITY.md) and [Implementation Status](./STATUS.md).

## Build it with us

> **“It takes a village to raise a child. Open source is no different.”**

CodeLocal is still growing. If you find a bug, disagree with a design decision, have an idea, or can make any part of the project better, contribute code and share feedback in the [CodeLocal Forums](https://codelocal.cloud/forums).

`codelocal.cloud` is the CodeLocal team's current **reference deployment**. The source is open for you to study, fork, modify and experiment with. Experimental self-hosting is welcome, but a self-hosted public gateway is **not yet an officially supported deployment target**.

Do not wait for CodeLocal to become “finished” before participating. Review the code, open issues, propose safer designs, send pull requests, challenge assumptions and tell us what does not work. **Help us grow CodeLocal together.**

See [Contributing](./CONTRIBUTING.md).

## Public edition and Enterprise

The public edition is intentionally optimized for **one person**: one user's Project Brain, reusable skills, project context and controlled execution across compatible AI clients.

The private Enterprise edition serves organizations that need AI inside security-sensitive business environments and can include organization-wide controls, private integrations, customer-specific deployment assumptions and proprietary infrastructure coupling.

For that reason, the public repository is **not** produced by copying the Enterprise source tree and deleting a few private files. Components are reviewed, decoupled, cleaned and, where necessary, re-implemented before they are opened. This allows useful functionality to be published without carrying over Enterprise-only operational context or customer-specific security architecture.

Publishing an Enterprise tree wholesale can also reveal internal integration paths, deployment assumptions and defensive architecture that could unnecessarily expand the information available to attackers studying Enterprise environments. This is not a substitute for secure engineering: both editions must remain secure and reviewable on their own merits.

The separation is being performed **manually**. During this transition, reviewers may still encounter historical names, compatibility files, source-only modules or Enterprise-era metadata that are not part of the shipped public npm runtime.

**Target completion date for the current public-repository separation and cleanup: October 19, 2026.**

Until then, use `STATUS.md`, the release builder and canonical architecture documentation as the source of truth for what actually ships in the public edition.

## Repository map

| Path | Ownership |
| --- | --- |
| `cmd/`, `internal/` | Canonical Go backend, CLI and local runtime |
| `web/` | Next.js browser product |
| `deploy/` | Dockerfiles, Railway configs and deployment assets |
| `tools/`, `scripts/` | Build, release and developer tooling |
| `docs/` | Architecture, guides, operations and plans |
| `legacy/typescript-runtime/` | Quarantined compatibility/history code; not the shipped runtime |

The canonical backend, Cloud gateway, CLI and local runtime are implemented in Go. The browser product uses Next.js + TypeScript. See [Product Stack](./docs/architecture/PRODUCT_STACK.md).

## Development

Requires Go 1.25+.

```bash
go test ./...
go vet ./...
go build ./cmd/...
```

Validate and build the npm CLI staging package without publishing:

```bash
npm run release:npm:prepare
```

For the full repository gate, use:

```bash
npm run release:prepare
```

Publishing and release details live in the [npm release operations runbook](./docs/operations/NPM_RELEASE.md). Keep release mechanics there rather than duplicating fast-changing implementation details in this README.

## Update

Stable:

```bash
npm i -g codelocal@latest
```

Beta:

```bash
npm i -g codelocal@beta
```

A normal npm update keeps existing pairing credentials and authorized workspaces. If an already-open MCP client reports a tool/schema mismatch after an update, refresh or reconnect CodeLocal in that client so it scans the current tool surface.

## Documentation

- [Start here](./docs/README.md)
- [User Guide](./docs/guides/USER_GUIDE.md)
- [How CodeLocal Works](./docs/guides/HOW_CODELOCAL_WORKS.md)
- [Security & Privacy](./docs/architecture/SECURITY_AND_PRIVACY.md)
- [Project Brain Evidence](./docs/architecture/PROJECT_BRAIN_EVIDENCE.md)
- [Product Stack](./docs/architecture/PRODUCT_STACK.md)
- [Implementation Status](./STATUS.md)
- [Security Policy](./SECURITY.md)
- [Contributing](./CONTRIBUTING.md)
- [Beta Channel](./docs/operations/BETA_CHANNEL.md)
- [OpenAI Plugin Submission Pack](./docs/integrations/openai/submission/README.md)

Website: [https://codelocal.cloud](https://codelocal.cloud/)

## License

CodeLocal is licensed under the [Apache License 2.0](./LICENSE). Unless a file or directory states otherwise, source code in this repository may be used, modified and distributed under the terms of that license. Third-party components retain their respective licenses.
