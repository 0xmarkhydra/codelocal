"use client";

import { useEffect, useState } from "react";
import styles from "./dashboard-chat.module.css";

type ToolCall = { id: string; name: string; arguments: string; result?: string; durationMs?: number; status: "done" | "error" };
type ChatMsg = { role: "user" | "assistant"; content: string; tool_calls?: ToolCall[] };

const STORAGE_KEY = "codelocal:dashboard-chat";

export function DashboardChat() {
  const [messages, setMessages] = useState<ChatMsg[]>([]);
  const [input, setInput] = useState("");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      if (raw) setMessages(JSON.parse(raw) as ChatMsg[]);
    } catch {}
  }, []);
  useEffect(() => {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(messages.slice(-30)));
    } catch {}
  }, [messages]);

  async function send(e: React.FormEvent) {
    e.preventDefault();
    const text = input.trim();
    if (!text || loading) return;
    const next: ChatMsg[] = [...messages, { role: "user", content: text }];
    setMessages(next);
    setInput("");
    setLoading(true);
    try {
      const history = next.slice(-12).map((m) => ({ role: m.role, content: m.content }));
      const res = await fetch("/api/dashboard-chat", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ message: text, history }),
      });
      const data = (await res.json()) as { reply?: string; tool_calls?: ToolCall[]; error?: string };
      if (!res.ok) {
        setMessages((m) => [...m, { role: "assistant", content: data.error ? `Lỗi: ${data.error}` : `Lỗi ${res.status}` }]);
      } else {
        setMessages((m) => [...m, { role: "assistant", content: data.reply ?? "(no reply)", tool_calls: data.tool_calls }]);
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      setMessages((m) => [...m, { role: "assistant", content: `CodeLocal offline: ${msg}` }]);
    } finally {
      setLoading(false);
    }
  }

  function clear() {
    setMessages([]);
    try {
      localStorage.removeItem(STORAGE_KEY);
    } catch {}
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
      <div className={styles.chatMessages}>
        {messages.length === 0 ? (
          <div className={styles.msgEmpty}>
            Hỏi ngay trên dashboard — ví dụ: “liệt kê workspaces”, “máy nào đang online?”, “tìm trong Project Brain: dashboard”. Sẽ thấy 🔧 func call.
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
              <div className={`${styles.msg} ${m.role === "user" ? styles.msgUser : styles.msgAssistant}`}>{m.content}</div>
            </div>
          ))
        )}
        {loading ? <div className={`${styles.msg} ${styles.msgAssistant}`}>CodeLocal đang nghĩ… (có thể đang gọi func)</div> : null}
      </div>
      <form className={styles.chatForm} onSubmit={send}>
        <input value={input} onChange={(e) => setInput(e.target.value)} placeholder="Nhắn cho CodeLocal... (thử: liệt kê workspaces)" aria-label="Chat input" />
        <button type="submit" disabled={loading || !input.trim()}>
          Gửi
        </button>
      </form>
      <div className={styles.chatHint}>Nhánh feat/dashboard-chat · chuẩn OpenAI tool_calls · gắn CODELOCAL_LLM_API_KEY vào web/.env để dùng model free · mock vẫn hiện func call khi chưa gắn key</div>
    </section>
  );
}
