# codex-mcp

Minimal filesystem MCP server for testing whether ChatGPT can access and modify files inside one explicitly allowed local project directory.

## Tools

- `project_info` — show the configured allowed root and limits
- `list_files` — list the project tree
- `read_file` — read a UTF-8 text file
- `write_file` — create or overwrite a UTF-8 text file
- `edit_file` — exact text replacement with ambiguity protection

No shell/terminal execution is included in this first test version.

## Security model

The server only accepts paths inside `PROJECT_ROOT`.

It rejects:

- absolute file paths supplied to tools
- `../` traversal outside the project
- existing symlinks that resolve outside the project
- writes whose parent directory resolves outside the project

`read_file`/`edit_file` are limited to 1 MiB per file and `list_files` is capped at 5,000 entries.

> Important: the MCP tools can write files. Do not expose this endpoint publicly without understanding the risk. For the initial test, keep the server bound to `127.0.0.1` and use a secure tunnel supported by your ChatGPT setup.

## Requirements

- Node.js 20+
- npm

## Install

```bash
git clone https://github.com/0xmarkhydra/codex-mcp.git
cd codex-mcp
npm install
```

## Run against a test folder

Create a harmless folder first:

```bash
mkdir -p ~/mcp-test-project
printf 'hello from local project\n' > ~/mcp-test-project/hello.txt
```

Then run the MCP server:

```bash
PROJECT_ROOT="$HOME/mcp-test-project" npm start
```

Expected output:

```text
codex-mcp listening on http://127.0.0.1:3333/mcp
PROJECT_ROOT=/Users/you/mcp-test-project
```

Health check:

```bash
curl http://127.0.0.1:3333/
```

## Expose it to ChatGPT

ChatGPT does not directly connect to a localhost MCP endpoint. Use OpenAI Secure MCP Tunnel when available for your account/workspace, or another HTTPS tunnel for an isolated test environment.

Your MCP URL should ultimately look like:

```text
https://YOUR-TUNNEL-HOST/mcp
```

In ChatGPT Developer Mode, create a custom app/MCP connection, enter that MCP endpoint, and scan the tools.

The tool scan should discover:

```text
project_info
list_files
read_file
write_file
edit_file
```

## Suggested first ChatGPT test

Ask ChatGPT:

```text
Use codex-mcp.
1. List the files in the allowed project.
2. Read hello.txt.
3. Create chatgpt-test.txt with the content: Hello from ChatGPT MCP
4. Read chatgpt-test.txt back to me.
5. Edit it so the content becomes: Edited successfully by ChatGPT MCP
6. Read it again.
```

Then verify locally:

```bash
cat ~/mcp-test-project/chatgpt-test.txt
```

Expected final content:

```text
Edited successfully by ChatGPT MCP
```

## Point it at a real project later

After the isolated test succeeds:

```bash
PROJECT_ROOT="/Users/yourname/Projects/your-project" npm start
```

Only that directory is exposed through the MCP filesystem tools.

## Environment variables

| Variable | Default | Description |
| --- | --- | --- |
| `PROJECT_ROOT` | required | Absolute path to the only allowed project directory |
| `HOST` | `127.0.0.1` | HTTP bind host |
| `PORT` | `3333` | HTTP port |

## Next step after the filesystem test

Once ChatGPT can reliably list/read/write/edit files, a later version can add deliberately restricted tools such as code search, git diff/status, tests, lint/typecheck, and selected commands. Do not add unrestricted shell execution to an internet-exposed MCP server.
