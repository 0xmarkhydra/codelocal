"use client";

import { useEffect, useRef, useState, type ChangeEvent, type ClipboardEvent, type FormEvent } from "react";
import styles from "./dashboard-chat.module.css";

type ToolCall = {
  id: string;
  name: string;
  arguments: string;
  result?: string;
  durationMs?: number;
  status: "done" | "error";
};

type ChatMsg = {
  role: "user" | "assistant";
  content: string;
  tool_calls?: ToolCall[];
  image?: string;
};

type StreamData = {
  delta?: string;
  reply?: string;
  error?: string;
  tool_calls?: ToolCall[] | Array<{ index: number; name?: string; arguments?: string; id?: string }>;
};

const suggestions = ["Liệt kê workspaces", "Máy nào đang online?", "Tìm trong Project Brain"];

export function DashboardChat() {
  const [messages, setMessages] = useState<ChatMsg[]>([]);
  const [input, setInput] = useState("");
  const [loading, setLoading] = useState(false);
  const [image, setImage] = useState<string | null>(null);
  const [models, setModels] = useState<string[]>([]);
  const [selectedModel, setSelectedModel] = useState("");
  const [notice, setNotice] = useState("");
  const fileRef = useRef<HTMLInputElement>(null);
  const endRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    fetch("/api/v1/dashboard/chat/history", { credentials: "include" })
      .then(async (response) => {
        if (response.status === 401) {
          window.location.href = "/login";
          return null;
        }
        if (!response.ok) throw new Error(`history ${response.status}`);
        return response.json();
      })
      .then((data: { messages?: Array<{ role: string; content: string; tool_calls?: string | ToolCall[]; image?: string }> } | null) => {
        if (!data?.messages) return;
        const mapped: ChatMsg[] = data.messages.map((message) => {
          let toolCalls: ToolCall[] | undefined;
          if (typeof message.tool_calls === "string") {
            try {
              toolCalls = JSON.parse(message.tool_calls) as ToolCall[];
            } catch {
              toolCalls = undefined;
            }
          } else if (Array.isArray(message.tool_calls)) {
            toolCalls = message.tool_calls;
          }
          return {
            role: message.role as ChatMsg["role"],
            content: message.content,
            tool_calls: toolCalls,
            image: message.image,
          };
        });
        setMessages(mapped.slice(-50));
      })
      .catch(() => setNotice("Không tải được lịch sử chat"));
  }, []);

  useEffect(() => {
    fetch("/api/v1/dashboard/models", { credentials: "include" })
      .then(async (response) => {
        if (!response.ok) throw new Error(`models ${response.status}`);
        return response.json();
      })
      .then((data: { models?: string[]; default_model?: string }) => {
        const nextModels = Array.isArray(data.models) ? data.models : [];
        setModels(nextModels);
        setSelectedModel((current) => current || data.default_model || nextModels[0] || "");
      })
      .catch(() => setNotice((current) => current || "Không tải được danh sách model"));
  }, []);

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: "smooth", block: "end" });
  }, [messages, loading]);

  const SUPPORTED_MIME = new Set(["image/jpeg", "image/png", "image/webp"]);
  const MAX_BYTES = 8 * 1024 * 1024;

  function readImage(file: File) {
    const mime = (file.type || "").toLowerCase();
    // Bê logic KidGPT (clipboard_image_paste_web.dart): chỉ nhận PNG/JPG/WebP, báo lỗi rõ ràng
    if (mime && mime.startsWith("image/") && !SUPPORTED_MIME.has(mime)) {
      setNotice(`Ảnh dán vào cần là PNG, JPG hoặc WebP (bạn dán ${mime})`);
      return;
    }
    if (file.size > MAX_BYTES) {
      setNotice(`Ảnh tối đa 8 MB (ảnh của bạn ${(file.size / 1024 / 1024).toFixed(1)} MB)`);
      return;
    }
    const reader = new FileReader();
    reader.onload = () => {
      setImage(reader.result as string);
      setNotice("");
    };
    reader.onerror = () => setNotice("Không đọc được ảnh trong clipboard");
    reader.readAsDataURL(file);
  }

  function onFile(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    if (file) readImage(file);
  }

  function extractImageFromClipboard(clipboardData: DataTransfer | null): File | null {
    if (!clipboardData) return null;
    // Bê y nguyên KidGPT: duyệt DataTransferItem kind=='file' && type.startsWith('image/')
    for (const item of Array.from(clipboardData.items)) {
      if (item.kind === "file" && item.type.startsWith("image/")) {
        const f = item.getAsFile();
        if (f) return f;
      }
    }
    const firstFile = clipboardData.files?.[0];
    if (firstFile && firstFile.type.startsWith("image/")) return firstFile;
    const anyImage = Array.from(clipboardData.items).find((entry) => entry.type.startsWith("image/"));
    return anyImage?.getAsFile() ?? null;
  }

  function onPaste(event: ClipboardEvent) {
    const file = extractImageFromClipboard(event.clipboardData);
    if (!file) return;
    event.preventDefault();
    readImage(file);
  }

  // Fix bug hiện tại: onPaste chỉ gắn ở chatMessages nên dán khi focus trong input không được.
  // Bê logic KidGPT document.addEventListener('paste') sang React.
  useEffect(() => {
    function onDocumentPaste(event: globalThis.ClipboardEvent) {
      const dt = event.clipboardData as unknown as DataTransfer | null;
      const file = extractImageFromClipboard(dt);
      if (!file) return;
      // Chỉ xử lý khi focus trong chat để tránh bắt paste全局
      const ae = document.activeElement as HTMLElement | null;
      const shell = document.querySelector(`.${styles.chatShell}`);
      const insideShell = !!shell?.contains(ae);
      const isTextField = !!ae && (ae.tagName === "INPUT" || ae.tagName === "TEXTAREA" || ae.isContentEditable);
      if (!insideShell && !isTextField) return;
      event.preventDefault();
      readImage(file);
    }
    document.addEventListener("paste", onDocumentPaste as unknown as EventListener);
    return () => document.removeEventListener("paste", onDocumentPaste as unknown as EventListener);
  }, []);

  function updateAssistant(index: number, content: string, toolCalls: ToolCall[]) {
    setMessages((current) => {
      const copy = [...current];
      copy[index] = { role: "assistant", content, tool_calls: [...toolCalls] };
      return copy;
    });
  }

  async function send(event: FormEvent) {
    event.preventDefault();
    const text = input.trim();
    if ((!text && !image) || loading) return;

    const userMessage: ChatMsg = { role: "user", content: text || "Phân tích ảnh này", image: image || undefined };
    const next = [...messages, userMessage];
    const sendImage = image;
    const placeholderIndex = next.length;

    setMessages([...next, { role: "assistant", content: "", tool_calls: [] }]);
    setInput("");
    setImage(null);
    setNotice("");
    setLoading(true);
    if (fileRef.current) fileRef.current.value = "";

    try {
      const history = next.slice(-12).map((message) => ({ role: message.role, content: message.content, image: message.image }));
      const response = await fetch("/api/v1/dashboard/chat?stream=1", {
        method: "POST",
        credentials: "include",
        headers: { "content-type": "application/json", accept: "text/event-stream" },
        body: JSON.stringify({ message: text || "Phân tích ảnh này", history, image: sendImage, model: selectedModel || undefined }),
      });

      if (response.status === 401) {
        window.location.href = "/login";
        throw new Error("Phiên đăng nhập đã hết hạn");
      }
      if (response.status === 429) {
        const data = (await response.json().catch(() => ({}))) as { retry_after?: number };
        throw new Error(`Gửi quá nhanh, thử lại sau ${data.retry_after || 60}s`);
      }
      if (!response.ok || !response.body) {
        const data = (await response.json().catch(() => ({}))) as { error?: string };
        throw new Error(data.error || `HTTP ${response.status}`);
      }

      if (!(response.headers.get("content-type") || "").includes("text/event-stream")) {
        const data = (await response.json()) as { reply?: string; tool_calls?: ToolCall[]; error?: string };
        if (data.error) throw new Error(data.error);
        updateAssistant(placeholderIndex, data.reply || "", data.tool_calls || []);
        return;
      }

      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";
      let content = "";
      let toolCalls: ToolCall[] = [];

      while (true) {
        const chunk = await reader.read();
        if (chunk.done) break;
        buffer += decoder.decode(chunk.value, { stream: true });
        const frames = buffer.split("\n\n");
        buffer = frames.pop() || "";

        for (const frame of frames) {
          let eventName = "message";
          let dataText = "";
          for (const line of frame.split("\n")) {
            if (line.startsWith("event:")) eventName = line.slice(6).trim();
            if (line.startsWith("data:")) dataText += line.slice(5).trim();
          }
          if (!dataText) continue;

          let data: StreamData;
          try {
            data = JSON.parse(dataText) as StreamData;
          } catch {
            continue;
          }

          if (eventName === "error") {
            throw new Error(data.error || "Model trả về lỗi stream");
          }
          if (eventName === "delta" && typeof data.delta === "string") {
            content += data.delta;
            updateAssistant(placeholderIndex, content, toolCalls);
            continue;
          }
          if (eventName === "tool_calls" && Array.isArray(data.tool_calls)) {
            toolCalls = data.tool_calls as ToolCall[];
            updateAssistant(placeholderIndex, content, toolCalls);
            continue;
          }
          if (eventName === "tool_delta" && Array.isArray(data.tool_calls)) {
            const deltas = data.tool_calls as Array<{ index: number; name?: string; arguments?: string; id?: string }>;
            for (const delta of deltas) {
              const existing = toolCalls[delta.index] || { id: delta.id || `tool_${delta.index}`, name: "", arguments: "", status: "done" as const };
              toolCalls[delta.index] = {
                ...existing,
                id: delta.id || existing.id,
                name: delta.name || existing.name,
                arguments: delta.arguments ?? existing.arguments,
              };
            }
            updateAssistant(placeholderIndex, content, toolCalls);
            continue;
          }
          if (eventName === "done") {
            if (typeof data.reply === "string") content = data.reply;
            if (Array.isArray(data.tool_calls)) toolCalls = data.tool_calls as ToolCall[];
            updateAssistant(placeholderIndex, content, toolCalls);
          }
        }
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      updateAssistant(placeholderIndex, `Không thể trả lời: ${message}`, []);
    } finally {
      setLoading(false);
    }
  }

  async function clear() {
    setMessages([]);
    setNotice("");
    try {
      const response = await fetch("/api/v1/dashboard/chat/history", { method: "DELETE", credentials: "include" });
      if (!response.ok) throw new Error(String(response.status));
    } catch {
      setNotice("Không thể xóa lịch sử trên server");
    }
  }

  return (
    <section className={styles.chatShell} aria-label="Chat với CodeLocal">
      <div className={styles.chatHead}>
        <div className={styles.brandBlock}>
          <span className={styles.brandMark} aria-hidden="true" />
          <div>
            <h3>CodeLocal</h3>
            <span>Project Brain · local tools</span>
          </div>
        </div>
        <div className={styles.chatActions}>
          <select
            className={styles.modelSelect}
            value={selectedModel}
            onChange={(event) => setSelectedModel(event.target.value)}
            aria-label="Chọn model"
            disabled={!models.length && !selectedModel}
          >
            {!selectedModel && !models.length ? <option value="">Đang tải model…</option> : null}
            {selectedModel && !models.includes(selectedModel) ? <option value={selectedModel}>{selectedModel}</option> : null}
            {models.map((model) => (
              <option key={model} value={model}>{model}</option>
            ))}
          </select>
          <button onClick={clear} className={styles.clearBtn} type="button" aria-label="Xóa lịch sử" title="Xóa lịch sử">
            <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 7h16M9 7V4h6v3m-8 0 1 13h8l1-13M10 11v5m4-5v5" /></svg>
          </button>
        </div>
      </div>

      <div className={styles.chatMessages} onPaste={onPaste}>
        {messages.length === 0 ? (
          <div className={styles.emptyState}>
            <div className={styles.emptyMark}><span /></div>
            <strong>Hỏi CodeLocal</strong>
            <p>Workspace, thiết bị và Project Brain trong một cuộc trò chuyện.</p>
            <div className={styles.suggestions}>
              {suggestions.map((suggestion) => (
                <button key={suggestion} type="button" onClick={() => setInput(suggestion)}>{suggestion}</button>
              ))}
            </div>
          </div>
        ) : (
          messages.map((message, index) => (
            <div key={`${message.role}-${index}`} className={styles.msgBlock}>
              {message.tool_calls?.length ? (
                <div className={styles.toolList}>
                  {message.tool_calls.map((tool) => (
                    <details key={tool.id} className={styles.toolPill}>
                      <summary>
                        <span className={styles.toolName}><span className={styles.toolDot} />{tool.name}</span>
                        <span className={styles.toolMeta}>{tool.durationMs ? `${tool.durationMs}ms` : tool.status}</span>
                      </summary>
                      <div className={styles.toolDetail}>
                        <code>{tool.arguments || "{}"}</code>
                        {tool.result ? <code>{tool.result.length > 800 ? `${tool.result.slice(0, 800)}…` : tool.result}</code> : null}
                      </div>
                    </details>
                  ))}
                </div>
              ) : null}
              {message.image ? <img src={message.image} alt="Ảnh đã gửi" className={styles.msgImage} /> : null}
              {message.content ? (
                <div className={`${styles.msg} ${message.role === "user" ? styles.msgUser : styles.msgAssistant}`}>{message.content}</div>
              ) : loading && index === messages.length - 1 ? (
                <div className={styles.thinking} aria-label="CodeLocal đang trả lời"><i /><i /><i /></div>
              ) : null}
            </div>
          ))
        )}
        <div ref={endRef} />
      </div>

      {image ? (
        <div className={styles.imagePreview}>
          <img src={image} alt="Ảnh chuẩn bị gửi" />
          <button type="button" onClick={() => setImage(null)} aria-label="Bỏ ảnh">×</button>
        </div>
      ) : null}

      <form className={styles.chatForm} onSubmit={send}>
        <input ref={fileRef} type="file" accept="image/*" onChange={onFile} className={styles.fileInput} />
        <button type="button" className={styles.attachBtn} onClick={() => fileRef.current?.click()} aria-label="Đính kèm ảnh">
          <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M8 12.5 14.7 5.8a3 3 0 0 1 4.2 4.2l-8.2 8.2a5 5 0 0 1-7.1-7.1l8-8" /></svg>
        </button>
        <input value={input} onChange={(event) => setInput(event.target.value)} placeholder="Nhắn cho CodeLocal" aria-label="Nội dung chat" autoComplete="off" />
        <button className={styles.sendBtn} type="submit" disabled={loading || (!input.trim() && !image)} aria-label="Gửi">
          <svg viewBox="0 0 24 24" aria-hidden="true"><path d="m5 12 7-7 7 7M12 5v14" /></svg>
        </button>
      </form>

      <div className={styles.chatHint}>{notice || "Enter để gửi · Dán ảnh trực tiếp"}</div>
    </section>
  );
}
