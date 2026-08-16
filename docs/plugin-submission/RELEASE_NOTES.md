# CodeLocal — Plugin Submission Release Notes

## Initial public submission candidate

CodeLocal is being prepared as an MCP-backed public plugin for ChatGPT and Codex.

### What this submission provides

- A compact 19-tool MCP surface for project inspection, semantic context, file editing, verification, Git, guarded terminal/process control, local MCP extensions, browser automation and desktop automation.
- Explicit workspace authorization and paired-device routing.
- Local approval controls for sensitive actions.
- Project Brain for durable project rules, decisions, facts, verified Experience and reusable learned workflows.
- Cross-device/logical-project continuity using repository/project identity evidence.
- Deterministic-first context compilation with optional guarded semantic retrieval and fallback behavior.
- Public privacy, terms, support and security pages.
- Dedicated OpenAI reviewer sandbox with deterministic sample data.

### Safety and review hardening in this candidate

- All 19 public tools have explicit `readOnlyHint`, `destructiveHint` and `openWorldHint` values.
- Annotation expectations are locked by repository tests.
- `agent` and `computer` use conservative open-world annotations because they can coordinate or perform actions that may affect external/public state.
- Force push remains blocked by local policy.
- Path escape outside an authorized workspace remains blocked.
- Browser, computer, terminal and extension calls remain subject to applicable approval/policy controls.
- Domain verification is supported at `/.well-known/openai-apps-challenge` using a deployment secret.

### Rollout notes

The public plugin submission should use the production universal MCP URL `https://codelocal.cloud/mcp`. Reviewer credentials and the reviewer runtime must be provisioned separately and kept out of source control. Beta client releases should remain on the npm beta channel until production Cloud, reviewer smoke tests and OpenAI Scan Tools pass.
