function isRecord(value: unknown): value is Record<string, any> {
  return !!value && typeof value === "object" && !Array.isArray(value);
}

function validContentItem(item: unknown) {
  if (!isRecord(item) || typeof item.type !== "string") return false;
  if (item.type === "text") return typeof item.text === "string";
  if (item.type === "image" || item.type === "audio") return typeof item.data === "string" && typeof item.mimeType === "string";
  if (item.type === "resource") return isRecord(item.resource);
  return false;
}

export function bridgeMcpToolResult(value: unknown) {
  const wrapper = isRecord(value) ? value : null;
  const nested = isRecord(wrapper?.result) ? wrapper.result : null;
  if (!nested) return null;

  const content = Array.isArray(nested.content) ? nested.content.filter(validContentItem) : [];
  const structuredContent = isRecord(nested.structuredContent) ? nested.structuredContent : null;
  if (!content.length && !structuredContent) return null;

  const result: Record<string, unknown> = {
    content: content.length ? content : [{ type: "text", text: "Installed MCP returned structured content." }],
  };
  if (structuredContent) result.structuredContent = structuredContent;
  if (nested.isError === true) result.isError = true;
  return result;
}
