"use client";

import { useEffect, useRef, useState } from "react";
import styles from "./dashboard-chat.module.css";

type ToolCall = { id: string; name: string; arguments: string; result?: string; durationMs?: number; status: "done" | "error" };
type ChatMsg = { role: "user" | "assistant"; content: string; tool_calls?: ToolCall[]; image?: string };

export function DashboardChat() {
  const [messages, setMessages] = useState<ChatMsg[]>([]);
  const [input, setInput] = useState("");
  const [loading, setLoading] = useState(false);
  const [image, setImage] = useState<string | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    fetch("/api/v1/dashboard/chat/history", { credentials: "include" })
      .then((r) => {
        if (r.status === 401) {
          window.location.href = "/login";
          return null;
        }
        return r.ok ? r.json() : null;
      })
      .then((data: { messages?: Array<{ role: string; content: string; tool_calls?: string | ToolCall[]; image?: string }> } | null) => {
        if (!data?.messages) return;
        const mapped: ChatMsg[] = data.messages.map((m) => {
          let tcs: ToolCall[] | undefined;
          if (typeof m.tool_calls === "string") {
            try {
              tcs = JSON.parse(m.tool_calls as unknown as string) as ToolCall[];
            } catch {
              tcs = undefined;
            }
          } else if (Array.isArray(m.tool_calls)) tcs = m.tool_calls as ToolCall[];
          return { role: m.role as ChatMsg["role"], content: m.content, tool_calls: tcs, image: (m as unknown as { image?: string }).image };
        });
        if (mapped.length) setMessages(mapped.slice(-50));
      })
      .catch(() => {});
  }, []);

  function onFile(e: React.ChangeEvent<HTMLInputElement>) {
    const f = e.target.files?.[0];
    if (!f) return;
    if (f.size > 8 * 1024 * 1024) {
      alert("Ảnh quá lớn (>8MB)");
      return;
    }
    const reader = new FileReader();
    reader.onload = () => setImage(reader.result as string);
    reader.readAsDataURL(f);
  }

  function onPaste(e: React.ClipboardEvent) {
    const item = Array.from(e.clipboardData.items).find((it) => it.type.startsWith("image/"));
    if (!item) return;
    const f = item.getAsFile();
    if (!f) return;
    e.preventDefault();
    const reader = new FileReader();
    reader.onload = () => setImage(reader.result as string);
    reader.readAsDataURL(f);
  }

  async function send(e: React.FormEvent) {
    e.preventDefault();
    const text = input.trim();
    if ((!text && !image) || loading) return;
    const userMsg: ChatMsg = { role: "user", content: text || "(ảnh)", image: image || undefined };
    const next: ChatMsg[] = [...messages, userMsg];
    setMessages(next);
    const sendImage = image;
    setInput("");
    setImage(null);
    if (fileRef.current) fileRef.current.value = "";
    setLoading(true);
    const placeholderIdx = next.length;
    setMessages((m) => [...m, { role: "assistant", content: "", tool_calls: [] }]);
    try {
      const history = next.slice(-12).map((m) => ({ role: m.role, content: m.content, image: m.image }));
      const res = await fetch("/api/v1/dashboard/chat?stream=1", {
        method: "POST",
        headers: { "content-type": "application/json", accept: "text/event-stream" },
        body: JSON.stringify({ message: text || "Phân tích ảnh này", history, image: sendImage }),
      });
      if (res.status === 401) {
        window.location.href = "/login";
        throw new Error("Unauthorized - redirecting to login");
      }
      if (res.status === 429) {
        const data = (await res.json().catch(() => ({ error: "rate_limited" }))) as { error?: string; retry_after?: number };
        throw new Error(data.error ? `Rate limited, thử lại sau ${data.retry_after || 60}s` : "Rate limited");
      }
      if (!res.ok || !res.body) {
        const data = (await res.json().catch(() => ({ error: `HTTP ${res.status}` }))) as { reply?: string; tool_calls?: ToolCall[]; error?: string };
        throw new Error(data.error || `HTTP ${res.status}`);
      }
      const contentType = res.headers.get("content-type") || "";
      if (!contentType.includes("text/event-stream")) {
        const data = (await res.json()) as { reply?: string; tool_calls?: ToolCall[]; error?: string };
        if (data.error) throw new Error(data.error);
        setMessages((m) => {
          const copy = [...m];
          copy[placeholderIdx] = { role: "assistant", content: data.reply ?? "", tool_calls: data.tool_calls };
          return copy;
        });
        return;
      }
      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buf = "";
      let acc = "";
      let toolCalls: ToolCall[] = [];
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buf += decoder.decode(value, { stream: true });
        const parts = buf.split("\n\n");
        buf = parts.pop() || "";
        for (const part of parts) {
          const lines = part.split("\n");
          let event = "message";
          let dataStr = "";
          for (const ln of lines) {
            if (ln.startsWith("event:")) event = ln.slice(6).trim();
            else if (ln.startsWith("data:")) dataStr += ln.slice(5).trim();
          }
          if (!dataStr) continue;
          try {
            const data = JSON.parse(dataStr);
            if (event === "delta" && typeof data.delta === "string") {
              acc += data.delta;
              setMessages((m) => {
                const copy = [...m];
                copy[placeholderIdx] = { role: "assistant", content: acc, tool_calls: [...toolCalls] };
                return copy;
              });
            } else if (event === "tool_calls" && Array.isArray(data.tool_calls)) {
              toolCalls = data.tool_calls as ToolCall[];
              setMessages((m) => {
                const copy = [...m];
                copy[placeholderIdx] = { role: "assistant", content: acc, tool_calls: [...toolCalls] };
                return copy;
              });
            } else if (event === "tool_delta" && data.tool_calls) {
              const deltas = data.tool_calls as Array<{ index: number; name?: string; arguments?: string; id?: string }>;
              for (const d of deltas) {
                if (!toolCalls[d.index]) toolCalls[d.index] = { id: d.id || `tool_${d.index}`, name: d.name || toolCalls[d.index]?.name || "", arguments: "", status: "done" };
                if (d.name) toolCalls[d.index].name = d.name;
                if (d.arguments) toolCalls[d.index].arguments += d.arguments;
              }
              setMessages((m) => {
                const copy = [...m];
                copy[placeholderIdx] = { role: "assistant", content: acc, tool_calls: [...toolCalls] };
                return copy;
              });
            } else if (event === "done" && data.reply) {
              acc = data.reply;
              if (data.tool_calls) toolCalls = data.tool_calls;
              setMessages((m) => {
                const copy = [...m];
                copy[placeholderIdx] = { role: "assistant", content: acc, tool_calls: [...toolCalls] };
                return copy;
              });
            } else if (event === "done") {
              setMessages((m) => {
                const copy = [...m];
                copy[placeholderIdx] = { role: "assistant", content: acc || copy[placeholderIdx].content, tool_calls: [...toolCalls] };
                return copy;
              });
            } else if (event === "error") {
              throw new Error(data.error || "stream error");
            }
          } catch {}
        }
      }
      setMessages((m) => {
        const copy = [...m];
        if (!copy[placeholderIdx].content && acc) copy[placeholderIdx].content = acc;
        return copy;
      });
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      setMessages((m) => {
        const copy = [...m];
        copy[placeholderIdx] = { role: "assistant", content: `Lỗi: ${msg}` };
        return copy;
      });
    } finally {
      setLoading(false);
    }
  }

  function clear() {
    setMessages([]);
    fetch("/api/v1/dashboard/chat/history", { method: "DELETE", credentials: "include" }).catch(() => {});
  }

  return (
    <section className={styles.chatShell} aria-label="Chat với CodeLocal">
      <div className={styles.chatHead}>
        <div>
          <h3>Chat với CodeLocal</h3>
          <span>dashboard · codelocal · không cần ChatGPT · hiện logic func call</span>
        </div>
        <button onClick={clear} className={styles.clearBtn} type="button" aria-label="Xóa lịch sử">
          Xóa
        </button>
      </div>
      <div className={styles.chatMessages} onPaste={onPaste}>
        {messages.length === 0 ? (
          <div className={styles.msgEmpty}>
            Hỏi ngay trên dashboard — ví dụ: “liệt kê workspaces”, “máy nào đang online?”, “tìm trong Project Brain: dashboard”. Kéo ảnh vào, dán Ctrl+V, hoặc bấm 📎 để upload — sẽ thấy 🔧 func call.
          </div>
        ) : (
          messages.map((m, i) => (
            <div key={i} className={styles.msgBlock}>
              {m.tool_calls && m.tool_calls.length > 0 ? (
                <div className={styles.toolList}>
                  {m.tool_calls.map((tc) => (
                    <details key={tc.id} className={styles.toolPill} open={m.tool_calls!.length === 1}>
                      <summary>
                        <span>🔧 {tc.name}</span>
                        <span className={styles.toolMeta}>
                          {tc.durationMs ? `${tc.durationMs}ms` : ""} · {tc.status}
                        </span>
                      </summary>
                      <div className={styles.toolDetail}>
                        <div>
                          <strong>args:</strong> <code>{tc.arguments}</code>
                        </div>
                        <div>
                          <strong>result:</strong> <code>{tc.result ? (tc.result.length > 800 ? tc.result.slice(0, 800) + "…" : tc.result) : "—"}</code>
                        </div>
                      </div>
                    </details>
                  ))}
                </div>
              ) : null}
              {m.image ? <img src={m.image} alt="upload" className={styles.msgImage} /> : null}
              <div className={`${styles.msg} ${m.role === "user" ? styles.msgUser : styles.msgAssistant}`}>{m.content}</div>
            </div>
          ))
        )}
        {loading ? <div className={`${styles.msg} ${styles.msgAssistant}`}>CodeLocal đang nghĩ… (có thể đang gọi func)</div> : null}
      </div>
      {image ? (
        <div className={styles.imagePreview}>
          <img src={image} alt="preview" />
          <button type="button" onClick={() => setImage(null)} aria-label="Xóa ảnh">
            ✕
          </button>
        </div>
      ) : null}
      <form className={styles.chatForm} onSubmit={send}>
        <input ref={fileRef} type="file" accept="image/*" onChange={onFile} style={{ display: "none" }} />
        <button type="button" className={styles.attachBtn} onClick={() => fileRef.current?.click()} aria-label="Upload ảnh">
          📎
        </button>
        <input value={input} onChange={(e) => setInput(e.target.value)} placeholder="Nhắn cho CodeLocal... dán ảnh Ctrl+V hoặc 📎" aria-label="Chat input" />
        <button type="submit" disabled={loading || (!input.trim() && !image)}>
          Gửi
        </button>
      </form>
      <div className={styles.chatHint}>Nhánh main đã có chat · Go backend stream SSE · history backend · gắn CODELOCAL_LLM_API_KEY vào Go env để dùng model free · hỗ trợ upload/paste ảnh</div>
    </section>
  );
}
