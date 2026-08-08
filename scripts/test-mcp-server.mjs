import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { z } from "zod";

const variant = process.argv[2] ?? "default";
const toolName = variant === "global" ? "global_echo" : variant === "workspace" ? "workspace_echo" : "echo_message";
const server = new McpServer({ name: `codelocal-test-extension-${variant}`, version: "1.0.0" });

server.registerTool(toolName, {
  title: variant === "default" ? "Echo message" : `${variant} echo`,
  description: `Return a message unchanged from the ${variant} CodeLocal MCP Hub fixture.`,
  inputSchema: { message: z.string() },
}, async ({ message }) => ({
  content: [{ type: "text", text: String(message) }],
  structuredContent: { message: String(message), variant },
}));

await server.connect(new StdioServerTransport());
