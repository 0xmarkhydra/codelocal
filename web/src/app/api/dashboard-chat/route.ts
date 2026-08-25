import { NextResponse } from "next/server";

type ChatHistoryItem = { role: "user" | "assistant" | "tool"; content: string; tool_call_id?: string; name?: string };
type ToolCall = { id: string; name: string; arguments: string; result?: string; durationMs?: number; status: "done" | "error" };

const CODELOCAL_TOOLS = [
  {
    type: "function" as const,
    function: {
      name: "list_workspaces",
      description: "List authorized CodeLocal workspaces for current user",
      parameters: { type: "object", properties: { status: { type: "string", enum: ["all", "active", "offline"] } }, required: [] },
    },
  },
  {
    type: "function" as const,
    function: {
      name: "list_devices",
      description: "List paired devices and online status",
      parameters: { type: "object", properties: {}, required: [] },
    },
  },
  {
    type: "function" as const,
    function: {
      name: "search_project_brain",
      description: "Search Project Brain memory/graph for a query",
      parameters: { type: "object", properties: { query: { type: "string" } }, required: ["query"] },
    },
  },
  {
    type: "function" as const,
    function: {
      name: "get_workspace_detail",
      description: "Get detail of a workspace by name or id",
      parameters: { type: "object", properties: { workspace: { type: "string" } }, required: ["workspace"] },
    },
  },
] as const;

async function execTool(name: string, args: Record<string, unknown>): Promise<string> {
  const start = Date.now();
  try {
    // Mock executors — replace with internal Go calls (projectbrain, mcpgateway) when ready
    if (name === "list_workspaces") {
      // In prod: call internal/projectbrain or codelocal API. Mock: return 5 recent from status
      return JSON.stringify({
        total: 26,
        recent: [
          { name: "codex-mcp", id: "codex-mcp-e759036dfa", status: "active" },
          { name: "X.com", id: "X.com-73873cb5ef", status: "active" },
          { name: "BIDDI", status: "offline" },
        ],
        filter: (args.status as string) || "all",
        durationMs: Date.now() - start,
      });
    }
    if (name === "list_devices") {
      return JSON.stringify({ paired: 1, online: 1, devices: [{ id: "MacBook-Pro-cua-Le-Van-Mong.local", online: true }] });
    }
    if (name === "search_project_brain") {
      return JSON.stringify({ query: args.query, hits: [{ path: "web/src/app/dashboard", score: 0.92 }], note: "mock - will query Project Brain index" });
    }
    if (name === "get_workspace_detail") {
      return JSON.stringify({ workspace: args.workspace, path: `/Users/levanmong/Documents/${args.workspace}`, status: "active", note: "mock" });
    }
    return JSON.stringify({ error: `unknown tool ${name}` });
  } catch (e) {
    return JSON.stringify({ error: String(e) });
  }
}

function mockToolCallsForMessage(msg: string): { reply: string; tool_calls: ToolCall[] } | null {
  const lower = msg.toLowerCase();
  if (lower.includes("workspace")) {
    return {
      reply: "Đây là workspaces của bạn (mock tool call):",
      tool_calls: [
        {
          id: "mock_1",
          name: "list_workspaces",
          arguments: JSON.stringify({ status: "all" }),
          result: JSON.stringify({ total: 26, sample: ["codex-mcp", "X.com", "BIDDI"] }),
          durationMs: 42,
          status: "done",
        },
      ],
    };
  }
  if (lower.includes("device") || lower.includes("máy")) {
    return {
      reply: "Thiết bị đã pair:",
      tool_calls: [
        {
          id: "mock_2",
          name: "list_devices",
          arguments: "{}",
          result: JSON.stringify({ paired: 1, online: 1 }),
          durationMs: 18,
          status: "done",
        },
      ],
    };
  }
  if (lower.includes("brain") || lower.includes("memory")) {
    return {
      reply: "Kết quả Project Brain:",
      tool_calls: [
        {
          id: "mock_3",
          name: "search_project_brain",
          arguments: JSON.stringify({ query: msg }),
          result: JSON.stringify({ hits: 3 }),
          durationMs: 55,
          status: "done",
        },
      ],
    };
  }
  return null;
}

export async function POST(req: Request) {
  let body: { message?: string; history?: ChatHistoryItem[] };
  try {
    body = (await req.json()) as typeof body;
  } catch {
    return NextResponse.json({ error: "invalid json" }, { status: 400 });
  }
  const message = body.message?.trim();
  const history = Array.isArray(body.history) ? body.history.slice(-12) : [];
  if (!message) return NextResponse.json({ error: "missing message" }, { status: 400 });

  const apiKey = process.env.CODELOCAL_LLM_API_KEY || process.env.OPENAI_API_KEY || "";
  const baseUrl = (process.env.CODELOCAL_LLM_BASE_URL || "https://api.openai.com/v1").replace(/\/$/, "");
  const model = process.env.CODELOCAL_LLM_MODEL || "gpt-4o-mini";

  // No API key -> mock with tool_calls demo so UI still shows func logic without ChatGPT
  if (!apiKey) {
    const mocked = mockToolCallsForMessage(message);
    if (mocked) return NextResponse.json({ ...mocked, mock: true, model });
    return NextResponse.json({
      reply: `CodeLocal (mock - chưa gắn API key): đã nhận "${message}". Gắn CODELOCAL_LLM_API_KEY + MODEL=${model} vào web/.env để dùng model free qua codelocal.`,
      mock: true,
      model,
      tool_calls: [] as ToolCall[],
    });
  }

  const system = "You are CodeLocal assistant on codelocal.cloud/dashboard. Answer concisely in Vietnamese when user speaks Vietnamese. Use tools when user asks about workspaces/devices/Project Brain. No need to go to ChatGPT.";
  const messages: Array<{ role: string; content: string; tool_call_id?: string; name?: string; tool_calls?: unknown }> = [
    { role: "system", content: system },
    ...history.map((m) => ({ role: m.role, content: m.content, tool_call_id: m.tool_call_id, name: m.name })),
    { role: "user", content: message },
  ];

  try {
    // First LLM call with tools
    const first = await fetch(`${baseUrl}/chat/completions`, {
      method: "POST",
      headers: { "content-type": "application/json", authorization: `Bearer ${apiKey}` },
      body: JSON.stringify({ model, messages, tools: CODELOCAL_TOOLS, tool_choice: "auto", temperature: 0.7 }),
    });
    if (!first.ok) {
      const t = await first.text();
      return NextResponse.json({ error: `upstream ${first.status}: ${t}` }, { status: 502 });
    }
    const data = (await first.json()) as {
      choices?: Array<{ message?: { content?: string; tool_calls?: Array<{ id: string; function: { name: string; arguments: string } }> } }>;
    };
    const msg = data.choices?.[0]?.message;
    const toolCallsRaw = msg?.tool_calls || [];

    if (toolCallsRaw.length === 0) {
      return NextResponse.json({ reply: msg?.content?.trim() || "(empty)", model, tool_calls: [] });
    }

    // Execute tools
    const toolResults: ToolCall[] = [];
    for (const tc of toolCallsRaw) {
      const name = tc.function.name;
      const argsStr = tc.function.arguments || "{}";
      let args: Record<string, unknown> = {};
      try {
        args = JSON.parse(argsStr) as Record<string, unknown>;
      } catch {
        args = {};
      }
      const t0 = Date.now();
      const result = await execTool(name, args);
      toolResults.push({ id: tc.id, name, arguments: argsStr, result, durationMs: Date.now() - t0, status: "done" });
    }

    // Second LLM call with tool results
    const followMessages = [
      ...messages,
      { role: "assistant", content: msg?.content || "", tool_calls: toolCallsRaw },
      ...toolResults.map((tr) => ({ role: "tool" as const, content: tr.result || "", tool_call_id: tr.id, name: tr.name })),
    ];
    const second = await fetch(`${baseUrl}/chat/completions`, {
      method: "POST",
      headers: { "content-type": "application/json", authorization: `Bearer ${apiKey}` },
      body: JSON.stringify({ model, messages: followMessages, temperature: 0.7 }),
    });
    if (!second.ok) {
      const t = await second.text();
      return NextResponse.json({ error: `upstream2 ${second.status}: ${t}`, tool_calls: toolResults }, { status: 502 });
    }
    const data2 = (await second.json()) as { choices?: Array<{ message?: { content?: string } }> };
    const reply = data2.choices?.[0]?.message?.content?.trim() || "(empty after tools)";
    return NextResponse.json({ reply, model, tool_calls: toolResults });
  } catch (e) {
    const m = e instanceof Error ? e.message : String(e);
    return NextResponse.json({ error: m }, { status: 500 });
  }
}
