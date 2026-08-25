"use client";
/* eslint-disable @next/next/no-img-element -- chat images are user-provided data/blob previews and should not be optimized remotely */

import Image from "next/image";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useMemo, useRef, useState, type ChangeEvent, type ClipboardEvent, type FormEvent, type KeyboardEvent } from "react";
import { isWorkspacesResource, type WorkspacesResource } from "@/lib/contracts/resources";
import { AppIcon } from "./app-icon";
import { useDashboardResource } from "./use-dashboard-resource";
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

type WorkspaceItem = WorkspacesResource["items"][number];

const suggestions = ["Tóm tắt dự án hiện tại", "Tìm file liên quan", "Kiểm tra workspace đang online"];

function workspaceKey(workspace: WorkspaceItem) {
  return `${workspace.deviceId}::${workspace.workspaceId}`;
}

function toolLabel(name: string) {
  switch (name) {
    case "list_workspaces": return "Đang xem workspaces";
    case "list_devices": return "Đang kiểm tra thiết bị";
    case "search_project_brain": return "Đang truy vấn Brain";
    case "get_workspace_detail": return "Đang đọc dự án";
    default: return name.replaceAll("_", " ");
  }
}

export function DashboardChat() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [messages, setMessages] = useState<ChatMsg[]>([]);
  const [input, setInput] = useState("");
  const [loading, setLoading] = useState(false);
  const [image, setImage] = useState<string | null>(null);
  const [notice, setNotice] = useState("");
  const [selectedWorkspaceKey, setSelectedWorkspaceKey] = useState(() => {
    const deviceId = searchParams.get("deviceId");
    const workspaceId = searchParams.get("workspaceId");
    return deviceId && workspaceId ? `${deviceId}::${workspaceId}` : "auto";
  });
  const workspaces = useDashboardResource("/api/v1/workspaces", isWorkspacesResource);
  const fileRef = useRef<HTMLInputElement>(null);
  const endRef = useRef<HTMLDivElement>(null);
  const formRef = useRef<HTMLFormElement>(null);

  const workspaceItems = useMemo(
    () => workspaces.state.kind === "ready" ? workspaces.state.value.items : [],
    [workspaces.state],
  );
  const selectedWorkspace = useMemo(
    () => workspaceItems.find((workspace) => workspaceKey(workspace) === selectedWorkspaceKey),
    [selectedWorkspaceKey, workspaceItems],
  );

  useEffect(() => {
    fetch("/api/v1/dashboard/chat/history", { credentials: "include" })
      .then(async (response) => {
        if (response.status === 401) {
          router.replace("/login");
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
  }, [router]);

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: "smooth", block: "end" });
  }, [messages, loading]);

  function readImage(file: File) {
    if (file.size > 8 * 1024 * 1024) {
      setNotice("Ảnh tối đa 8 MB");
      return;
    }
    const reader = new FileReader();
    reader.onload = () => {
      setImage(reader.result as string);
      setNotice("");
    };
    reader.readAsDataURL(file);
  }

  function onFile(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    if (file) readImage(file);
  }

  function onPaste(event: ClipboardEvent) {
    const item = Array.from(event.clipboardData.items).find((entry) => entry.type.startsWith("image/"));
    const file = item?.getAsFile();
    if (!file) return;
    event.preventDefault();
    readImage(file);
  }

  function onComposerKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (event.key !== "Enter" || event.shiftKey) return;
    event.preventDefault();
    formRef.current?.requestSubmit();
  }

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
        body: JSON.stringify({
          message: text || "Phân tích ảnh này",
          history,
          image: sendImage,
          workspace: selectedWorkspace ? {
            deviceId: selectedWorkspace.deviceId,
            workspaceId: selectedWorkspace.workspaceId,
            workspaceName: selectedWorkspace.workspaceName,
          } : undefined,
        }),
      });

      if (response.status === 401) {
        router.replace("/login");
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

          if (eventName === "error") throw new Error(data.error || "Model trả về lỗi stream");
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
    <section className={styles.chatShell} aria-label="Chat với Thánh Gióng">
      <div className={styles.chatHead}>
        <div className={styles.brandBlock}>
          <span className={styles.avatar} aria-hidden="true"><Image src="/thanh-giong-mark.svg" alt="" width={44} height={44} priority /></span>
          <div>
            <div className={styles.nameRow}><h1>Thánh Gióng</h1><i /></div>
            <span>Trợ lý AI của CodeLocal</span>
          </div>
        </div>
        <div className={styles.chatActions}>
          <label className={styles.projectPicker}>
            <span className={styles.projectPickerIcon} aria-hidden="true">
              <AppIcon name="folder" size={17} />
            </span>
            <select value={selectedWorkspaceKey} onChange={(event) => setSelectedWorkspaceKey(event.target.value)} aria-label="Chọn dự án">
              <option value="auto">Dự án: Auto</option>
              {workspaceItems.map((workspace) => <option key={workspaceKey(workspace)} value={workspaceKey(workspace)}>Dự án: {workspace.workspaceName}</option>)}
            </select>
          </label>
          <button onClick={clear} className={styles.clearBtn} type="button" aria-label="Xóa lịch sử" title="Xóa lịch sử">
            <AppIcon name="trash" size={18} />
          </button>
        </div>
      </div>

      <div className={styles.chatMessages} onPaste={onPaste}>
        {messages.length === 0 ? (
          <div className={styles.emptyState}>
            <span className={styles.emptyOrb} aria-hidden="true"><Image src="/thanh-giong-mark.svg" alt="" width={58} height={58} /></span>
            <strong>Bạn muốn làm gì?</strong>
            <p>{selectedWorkspace ? `Đang làm việc với ${selectedWorkspace.workspaceName}.` : "Auto sẽ tự chọn dự án phù hợp."}</p>
            <div className={styles.suggestions}>
              {suggestions.map((suggestion) => <button key={suggestion} type="button" onClick={() => setInput(suggestion)}>{suggestion}</button>)}
            </div>
          </div>
        ) : messages.map((message, index) => (
          <div key={`${message.role}-${index}`} className={`${styles.msgBlock} ${message.role === "user" ? styles.userBlock : styles.assistantBlock}`}>
            {message.role === "assistant" ? <span className={styles.messageAvatar} aria-hidden="true"><Image src="/thanh-giong-mark.svg" alt="" width={32} height={32} /></span> : null}
            <div className={styles.messageBody}>
              {message.tool_calls?.length ? (
                <div className={styles.toolList}>
                  {message.tool_calls.map((tool) => (
                    <details key={tool.id} className={styles.toolPill}>
                      <summary>
                        <span className={styles.toolName}><span className={styles.toolDot} />{toolLabel(tool.name)}</span>
                        <span className={styles.toolMeta}>{tool.status === "done" ? "Hoàn tất" : "Lỗi"}</span>
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
              {message.content ? <div className={`${styles.msg} ${message.role === "user" ? styles.msgUser : styles.msgAssistant}`}>{message.content}</div> : loading && index === messages.length - 1 ? <div className={styles.thinking} aria-label="Thánh Gióng đang trả lời"><i /><i /><i /></div> : null}
            </div>
          </div>
        ))}
        <div ref={endRef} />
      </div>

      {image ? <div className={styles.imagePreview}><img src={image} alt="Ảnh chuẩn bị gửi" /><button type="button" onClick={() => setImage(null)} aria-label="Bỏ ảnh"><AppIcon name="close" size={14} /></button></div> : null}

      <form ref={formRef} className={styles.chatForm} onSubmit={send}>
        <input ref={fileRef} type="file" accept="image/*" onChange={onFile} className={styles.fileInput} />
        <button type="button" className={styles.attachBtn} onClick={() => fileRef.current?.click()} aria-label="Đính kèm ảnh">
          <AppIcon name="paperclip" size={18} />
        </button>
        <textarea value={input} onChange={(event) => setInput(event.target.value)} onKeyDown={onComposerKeyDown} placeholder="Nhắn Thánh Gióng…" aria-label="Nội dung chat" rows={1} />
        <button className={styles.sendBtn} type="submit" disabled={loading || (!input.trim() && !image)} aria-label="Gửi">
          <AppIcon name="send" size={18} />
        </button>
      </form>
      <div className={styles.chatHint}>{notice || "Enter để gửi · Shift+Enter để xuống dòng"}</div>
    </section>
  );
}
