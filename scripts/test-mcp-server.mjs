import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { z } from "zod";

const server = new McpServer({ name: "codelocal-test-extension", version: "1.0.0" });

server.registerTool("echo_message", {
  title: "Echo message",
  description: "Return a message unchanged for CodeLocal MCP Hub integration tests.",
  inputSchema: { message: z.string() },
}, async ({ message }) => ({
  content: [{ type: "text", text: String(message) }],
  structuredContent: { message: String(message) },
}));

await server.connect(new StdioServerTransport());
