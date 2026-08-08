# CodeLocal MCP Hub MVP

CodeLocal can host other MCP servers locally while exposing only a small, stable tool surface to ChatGPT.

The architecture deliberately keeps ChatGPT as the only planner:

```text
ChatGPT
   |
   | CodeLocal MCP
   v
mcp_list / mcp_search_tools / mcp_tool_info / mcp_call
   |
   v
CodeLocal local MCP Hub
   |-- stdio MCP A
   |-- stdio MCP B
   `-- Streamable HTTP MCP C
```

Installed MCP tools are **not** individually registered with ChatGPT. A user may install hundreds of extension tools without expanding the ChatGPT-facing tool catalog by hundreds of schemas.

## Start a project

From a project directory:

```bash
codelocal .
```

This is equivalent to `codelocal start .`.

## Install a local stdio MCP

The shortest form mirrors how MCP clients normally configure command-based servers:

```bash
codelocal mcp add my-mcp -- npx -y @company/my-mcp
```

Workspace scope is the default. Use `--global` to make the server available in every CodeLocal workspace:

```bash
codelocal mcp add my-mcp --global -- npx -y @company/my-mcp
```

Equivalent explicit form:

```bash
codelocal mcp add my-mcp \
  --stdio npx \
  --arg -y \
  --arg @company/my-mcp
```

CodeLocal saves the configuration and immediately probes the server with MCP `initialize` + `tools/list`. Use `--no-probe` when the environment is not ready yet.

## Install a Streamable HTTP MCP

```bash
codelocal mcp add remote-demo --url https://example.com/mcp
```

Plain HTTP is rejected except for loopback development endpoints. For a local MCP server this is allowed:

```bash
codelocal mcp add local-demo --url http://127.0.0.1:3001/mcp
```

### Bearer token without storing the token

```bash
export MY_MCP_TOKEN='...'
codelocal mcp add remote-demo \
  --url https://example.com/mcp \
  --bearer-env MY_MCP_TOKEN
```

The registry stores only the string `MY_MCP_TOKEN`. The token value itself is read from the environment at connection time and is not persisted by CodeLocal.

Generic header references are also supported:

```bash
codelocal mcp add remote-demo \
  --url https://example.com/mcp \
  --header-env X-API-Key=MY_MCP_API_KEY
```

OAuth login for third-party remote MCPs is intentionally not implemented in this MVP. The first public release should add a dedicated OAuth credential provider instead of storing refresh/access tokens in the registry.

## Environment variables for stdio MCPs

Pass environment variables by reference:

```bash
export GITHUB_TOKEN='...'
codelocal mcp add example --env API_TOKEN=GITHUB_TOKEN -- node ./server.mjs
```

CodeLocal stores:

```text
API_TOKEN -> GITHUB_TOKEN
```

It does **not** store the current value of `GITHUB_TOKEN`.

## Inspect installed MCPs

```bash
codelocal mcp list
codelocal mcp list --json
codelocal mcp info my-mcp
```

Refresh/probe one server and cache its tool catalog:

```bash
codelocal mcp probe my-mcp
```

Search the local tool catalog from the CLI:

```bash
codelocal mcp search "create github issue"
codelocal mcp search "read figma node" --server figma
```

Remove a server:

```bash
codelocal mcp remove my-mcp
codelocal mcp remove my-mcp --global
```

## What ChatGPT sees

Only four MCP Hub tools are exposed by CodeLocal:

### `mcp_list`

Lists installed MCP servers visible to the selected workspace.

### `mcp_search_tools`

Searches CodeLocal's cached local catalog and returns a small ranked set of matching tools. It does not dump every installed tool schema into ChatGPT context.

### `mcp_tool_info`

Returns the exact schema/metadata of one discovered tool before ChatGPT calls it.

### `mcp_call`

Calls one installed MCP tool using `{ server, tool, arguments }`.

A typical flow is:

```text
User: "Create a GitHub issue for this bug"

ChatGPT -> mcp_search_tools("create github issue")
CodeLocal -> github.create_issue + a few close matches

ChatGPT -> mcp_tool_info(github, create_issue)
CodeLocal -> exact input schema

ChatGPT -> mcp_call(github, create_issue, {...})
CodeLocal -> local approval -> installed GitHub MCP -> result
```

## Approval and trust model

Every external `mcp_call` is approval-gated by default.

CodeLocal intentionally does **not** treat an MCP server's `readOnlyHint` annotation as a security boundary. Third-party tool annotations are advisory metadata, not proof that a tool is safe.

The local terminal prompt supports:

```text
Allow once
Allow matched tool rule for this CodeLocal session
Deny
```

The session rule is scoped to the concrete tool:

```text
mcp:<server>:<tool>
```

so allowing one tool does not silently allow every tool from that MCP server.

## Local state

MCP Hub state lives under:

```text
~/.codelocal/mcp/registry.json
~/.codelocal/mcp/catalog.json
```

or under `$CODELOCAL_STATE_DIR/mcp` during tests/custom deployments.

Directories are created private (`0700`) and state files are written with private permissions (`0600`) on POSIX systems. Writes use a temporary file followed by rename.

The catalog stores tool metadata such as names, descriptions and JSON schemas. It does not contain the source code of the current project.

## Result forwarding

When an installed MCP returns normal MCP content, CodeLocal forwards supported nested content back through the main MCP response instead of converting everything into a JSON string.

The MVP preserves:

- text content;
- image content;
- audio content;
- embedded resource content;
- `structuredContent`;
- `isError`.

This is important for extensions that return screenshots or other model-visible artifacts.

## MVP test sequence

```bash
cd ~/Documents/codex-mcp
git fetch origin
git switch dev-mcp-hub-mvp
git pull origin dev-mcp-hub-mvp
npm install
npm run typecheck
npm test

node -p "require('./package.json').version"
# 1.3.0-dev.0
```

Test the CLI with the bundled fixture:

```bash
codelocal mcp add local-echo -- node "$PWD/scripts/test-mcp-server.mjs"
codelocal mcp list
codelocal mcp search "echo message"
codelocal mcp info local-echo
codelocal mcp probe local-echo
```

Then run CodeLocal from any project:

```bash
cd /path/to/project
codelocal .
```

From ChatGPT, test:

```text
Use mcp_list and tell me which MCP extensions are installed.
```

then:

```text
Search installed MCP tools for "echo message". Inspect the best matching schema, then call it with the text "hello from ChatGPT".
```

The terminal should request approval before the actual extension tool call.

## Explicit MVP limits

Not yet included:

- OAuth flow for third-party remote MCPs;
- website marketplace/install sync;
- cloud account-backed MCP installation records;
- persistent OS Keychain-backed third-party MCP credentials;
- resource/prompt routing through the Hub;
- cancellation propagation into an in-flight child MCP call;
- semantic embeddings for tool search (current ranking is deterministic lexical/schema ranking);
- signed publisher manifests or marketplace trust badges.

These are intentionally deferred until the local CLI + routing model is validated end-to-end.
