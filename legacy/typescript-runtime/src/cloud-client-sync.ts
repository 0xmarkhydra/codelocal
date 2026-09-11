// MCP configuration is intentionally local-only.
// This compatibility shim remains so older compiled entrypoints fail closed
// instead of attempting to fetch MCP metadata from CodeLocal Cloud.
export async function syncCloudMcpBeforeClient() {
  return { skipped: true, reason: "MCP configuration is local-only" };
}
