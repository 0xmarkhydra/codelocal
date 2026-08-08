export function bridgeMcpToolResult(value: unknown) {
  const wrapper = value && typeof value === "object" ? value as Record<string, any> : null;
  const nested = wrapper?.result && typeof wrapper.result === "object" ? wrapper.result as Record<string, any> : null;
  if (!nested || !Array.isArray(nested.content)) return null;

  const allowedTypes = new Set(["text", "image", "audio", "resource"]);
  const content = nested.content.filter((item: unknown) => {
    return !!item && typeof item === "object" && allowedTypes.has(String((item as Record<string, unknown>).type ?? ""));
  });

  if (!content.length && nested.structuredContent == null) return null;
  const result: Record<string, unknown> = {
    content: content.length ? content : [{ type: "text", text: "Installed MCP returned structured content." }],
  };
  if (nested.structuredContent && typeof nested.structuredContent === "object") result.structuredContent = nested.structuredContent;
  if (nested.isError === true) result.isError = true;
  return result;
}
