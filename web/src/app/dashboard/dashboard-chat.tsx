"use client";
/* eslint-disable @next/next/no-img-element -- chat images are user-provided data/blob previews and should not be optimized remotely */

import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useMemo, useRef, useState, type ChangeEvent, type ClipboardEvent, type FormEvent, type KeyboardEvent } from "react";
import { isWorkspacesResource, type WorkspacesResource } from "@/lib/contracts/resources";
import { AppIcon } from "./app-icon";
import { ChatRichMessage } from "./chat-rich-message";
import { useDashboardResource } from "./use-dashboard-resource";
import styles from "./dashboard-chat.module.css";

type ToolCall = {
  id: string;
  name: string;
  arguments: string;
  result?: string;
  durationMs?: number;
  status: "done" | "error" | "approval_required";
};

type ChatMsg = {
  role: "user" | "assistant";
  content: string;
  tool_calls?: ToolCall[];
  image?: string;
};

type StreamData = {
  delta?: string;
  content?: string;
  reply?: string;
  error?: string;
  tool_calls?: ToolCall[] | Array<{ index: number; name?: string; arguments?: string; id?: string }>;
};

type WorkspaceItem = WorkspacesResource["items"][number];

type ChatImageMeta = {
  imageRef: string;
  sha256: string;
  contentType: string;
  size: number;
};

type PreparedChatImage = {
  previewUrl: string;
  file: File;
};

type MediaPrepareResponse = ChatImageMeta & {
  url: string;
  upload: {
    required: boolean;
    url?: string;
    method?: string;
    headers?: Record<string, string[]>;
  };
};

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
    case "list_project_files": return "Đang xem mã nguồn";
    case "read_project_file": return "Đang đọc file";
    case "search_project_code": return "Đang tìm trong code";
    case "edit_project_file": return "Đang sửa code";
    case "write_project_file": return "Đang cập nhật file";
    case "apply_project_patch": return "Đang áp dụng thay đổi";
    case "run_project_command": return "Đang chạy lệnh";
    case "verify_project_changes": return "Đang kiểm tra thay đổi";
    default: return name.replaceAll("_", " ");
  }
}

function friendlyChatFailure(message: string) {
  if (/508|tool loop|loop exceeded/i.test(message)) {
    return "Luồng xử lý vừa quá dài. Thánh Gióng đã giữ lại phần đã làm; gửi “tiếp tục” để nối tiếp ngay.";
  }
  if (/timeout|429|502|503|504|network|fetch/i.test(message)) {
    return "Kết nối xử lý vừa gián đoạn. Thánh Gióng đã thử lại tự động; gửi “tiếp tục” nếu bạn muốn nối tiếp.";
  }
  return message.startsWith("Thánh Gióng") || message.startsWith("Kết nối") ? message : `Có lỗi khi xử lý yêu cầu: ${message}`;
}

export function DashboardChat() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [messages, setMessages] = useState<ChatMsg[]>([]);
  const [input, setInput] = useState("");
  const [loading, setLoading] = useState(false);
  const [image, setImage] = useState<PreparedChatImage | null>(null);
  const [imageUploading, setImageUploading] = useState(false);
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
  const quickMessageRef = useRef<string | null>(null);

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

  async function sha256Hex(file: File) {
    const digest = await crypto.subtle.digest("SHA-256", await file.arrayBuffer());
    return Array.from(new Uint8Array(digest), (value) => value.toString(16).padStart(2, "0")).join("");
  }

  async function uploadImage(file: File): Promise<ChatImageMeta> {
    const sha256 = await sha256Hex(file);
    const presign = await fetch("/api/v1/dashboard/media/presign", {
      method: "POST",
      credentials: "include",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ sha256, contentType: file.type, size: file.size }),
    });
    if (presign.status === 401) {
      router.replace("/login");
      throw new Error("Phiên đăng nhập đã hết hạn");
    }
    const prepared = (await presign.json().catch(() => ({}))) as Partial<MediaPrepareResponse> & { error?: string; message?: string };
    if (!presign.ok || !prepared.url || !prepared.imageRef) {
      if (prepared.error === "media_not_configured") throw new Error("Hệ thống chưa bật upload ảnh");
      throw new Error(prepared.message || "Không chuẩn bị được upload ảnh");
    }

    if (prepared.upload?.required) {
      if (!prepared.upload.url) throw new Error("Thiếu đường dẫn upload ảnh");
      let directUploadOK = false;
      try {
        const headers = new Headers();
        for (const [key, values] of Object.entries(prepared.upload.headers || {})) {
          const lower = key.toLowerCase();
          if (lower === "host" || lower === "content-length") continue;
          for (const value of values) headers.append(key, value);
        }
        if (!headers.has("content-type")) headers.set("content-type", file.type);
        const upload = await fetch(prepared.upload.url, {
          method: prepared.upload.method || "PUT",
          headers,
          body: file,
        });
        directUploadOK = upload.ok;
      } catch {
        directUploadOK = false;
      }

      if (!directUploadOK) {
        const fallback = await fetch("/api/v1/dashboard/media/upload", {
          method: "POST",
          credentials: "include",
          headers: {
            "content-type": file.type,
            "x-codelocal-media-sha256": sha256,
            "x-codelocal-media-size": String(file.size),
          },
          body: file,
        });
        const fallbackData = (await fallback.json().catch(() => ({}))) as { error?: string; message?: string };
        if (!fallback.ok) throw new Error(fallbackData.message || "Không tải được ảnh lên CodeLocal");
      }
    }

    return {
      imageRef: prepared.imageRef,
      sha256,
      contentType: prepared.contentType || file.type,
      size: prepared.size || file.size,
    };
  }

  function prepareImage(file: File) {
    if (!file.type.startsWith("image/")) {
      setNotice("Chỉ hỗ trợ file ảnh");
      return;
    }
    if (file.size > 8 * 1024 * 1024) {
      setNotice("Ảnh tối đa 8 MB");
      return;
    }

    if (image?.previewUrl.startsWith("blob:")) URL.revokeObjectURL(image.previewUrl);
    setImage({ previewUrl: URL.createObjectURL(file), file });
    setNotice("");
  }

  function discardImage() {
    if (image?.previewUrl.startsWith("blob:")) URL.revokeObjectURL(image.previewUrl);
    setImage(null);
    if (fileRef.current) fileRef.current.value = "";
  }

  function onFile(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    if (file) void prepareImage(file);
  }

  function onPaste(event: ClipboardEvent) {
    const fileFromClipboard = Array.from(event.clipboardData.files).find((file) => file.type.startsWith("image/"));
    const itemFromClipboard = Array.from(event.clipboardData.items).find((entry) => entry.kind === "file" && entry.type.startsWith("image/"));
    const file = fileFromClipboard ?? itemFromClipboard?.getAsFile();
    if (!file) return;
    event.preventDefault();
    event.stopPropagation();
    void prepareImage(file);
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

  function submitQuickMessage(message: string) {
    if (loading) return;
    quickMessageRef.current = message;
    formRef.current?.requestSubmit();
  }

  async function send(event: FormEvent) {
    event.preventDefault();
    const quickMessage = quickMessageRef.current;
    quickMessageRef.current = null;
    const text = (quickMessage ?? input).trim();
    if ((!text && !image) || loading || imageUploading) return;

    const pendingImage = image;
    let sendImage: ChatImageMeta | undefined;
    let useMultipartFallback = false;
    if (pendingImage) {
      setImageUploading(true);
      setNotice("Đang tải ảnh…");
      try {
        sendImage = await uploadImage(pendingImage.file);
      } catch {
        useMultipartFallback = true;
        setNotice("Đang gửi ảnh trực tiếp…");
      } finally {
        setImageUploading(false);
      }
    }

    const userMessage: ChatMsg = { role: "user", content: text || "Phân tích ảnh này", image: pendingImage?.previewUrl };
    const next = [...messages, userMessage];
    const placeholderIndex = next.length;

    setMessages([...next, { role: "assistant", content: "", tool_calls: [] }]);
    setInput("");
    setImage(null);
    setLoading(true);
    if (fileRef.current) fileRef.current.value = "";

    let streamedContent = "";
    let streamedToolCalls: ToolCall[] = [];
    try {
      const history = next.slice(-12).map((message) => ({ role: message.role, content: message.content }));
      const payload = {
        message: text || "Phân tích ảnh này",
        history,
        imageMeta: sendImage ? {
          imageRef: sendImage.imageRef,
          sha256: sendImage.sha256,
          contentType: sendImage.contentType,
          size: sendImage.size,
        } : undefined,
        workspace: selectedWorkspace ? {
          deviceId: selectedWorkspace.deviceId,
          workspaceId: selectedWorkspace.workspaceId,
          workspaceName: selectedWorkspace.workspaceName,
        } : undefined,
      };
      let requestBody: BodyInit;
      const requestHeaders: HeadersInit = { accept: "text/event-stream" };
      if (useMultipartFallback && pendingImage) {
        const form = new FormData();
        form.append("payload", JSON.stringify(payload));
        form.append("image", pendingImage.file, pendingImage.file.name || "pasted-image");
        requestBody = form;
      } else {
        requestHeaders["content-type"] = "application/json";
        requestBody = JSON.stringify(payload);
      }
      const response = await fetch("/api/v1/dashboard/chat?stream=1", {
        method: "POST",
        credentials: "include",
        headers: requestHeaders,
        body: requestBody,
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
      setNotice("");

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
      streamedContent = content;
      streamedToolCalls = toolCalls;

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
            streamedContent = content;
            updateAssistant(placeholderIndex, content, toolCalls);
            continue;
          }
          if (eventName === "replace" && typeof data.content === "string") {
            content = data.content;
            streamedContent = content;
            updateAssistant(placeholderIndex, content, toolCalls);
            continue;
          }
          if (eventName === "tool_calls" && Array.isArray(data.tool_calls)) {
            toolCalls = data.tool_calls as ToolCall[];
            streamedToolCalls = toolCalls;
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
            streamedToolCalls = toolCalls;
            updateAssistant(placeholderIndex, content, toolCalls);
            continue;
          }
          if (eventName === "done") {
            if (typeof data.reply === "string") content = data.reply;
            if (Array.isArray(data.tool_calls)) toolCalls = data.tool_calls as ToolCall[];
            streamedContent = content;
            streamedToolCalls = toolCalls;
            updateAssistant(placeholderIndex, content, toolCalls);
          }
        }
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      const failure = friendlyChatFailure(message);
      const content = streamedContent ? `${streamedContent}\n\n${failure}` : failure;
      updateAssistant(placeholderIndex, content, streamedToolCalls);
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
          <span className={styles.avatar} aria-hidden="true"><AppIcon name="thanh-giong" size={22} /></span>
          <div className={styles.nameRow}><h1>Thánh Gióng</h1><i /></div>
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
            <span className={styles.emptyOrb} aria-hidden="true"><AppIcon name="thanh-giong" size={27} /></span>
            <strong>Bạn muốn làm gì?</strong>
            <div className={styles.suggestions}>
              {suggestions.map((suggestion) => <button key={suggestion} type="button" onClick={() => setInput(suggestion)}>{suggestion}</button>)}
            </div>
          </div>
        ) : messages.map((message, index) => (
          <div key={`${message.role}-${index}`} className={`${styles.msgBlock} ${message.role === "user" ? styles.userBlock : styles.assistantBlock}`}>
            <div className={styles.messageBody}>
              {message.tool_calls?.length ? (
                <div className={styles.toolList}>
                  {message.tool_calls.map((tool) => (
                    <div key={tool.id} className={styles.toolActionRow}>
                      <details className={styles.toolPill}>
                        <summary>
                          <span className={styles.toolName}><span className={styles.toolDot} />{toolLabel(tool.name)}</span>
                          {tool.status === "error" ? <span className={styles.toolMeta}>Lỗi</span> : null}
                          {tool.status === "approval_required" ? <span className={styles.toolMeta}>Cần quyền</span> : null}
                        </summary>
                        <div className={styles.toolDetail}>
                          <code>{tool.arguments || "{}"}</code>
                          {tool.result ? <code>{tool.result.length > 800 ? `${tool.result.slice(0, 800)}…` : tool.result}</code> : null}
                        </div>
                      </details>
                      {tool.status === "approval_required" ? (
                        <button type="button" className={styles.fullAccessBtn} onClick={() => submitQuickMessage("Toàn quyền truy cập")} disabled={loading}>
                          Toàn quyền truy cập
                        </button>
                      ) : null}
                    </div>
                  ))}
                </div>
              ) : null}
              {message.image ? <img src={message.image} alt="Ảnh đã gửi" className={styles.msgImage} /> : null}
              {message.content ? (
                <div className={`${styles.msg} ${message.role === "user" ? styles.msgUser : styles.msgAssistant} ${loading && message.role === "assistant" && index === messages.length - 1 ? styles.msgStreaming : ""}`}>
                  {message.role === "assistant" ? <ChatRichMessage content={message.content} /> : message.content}
                  {loading && message.role === "assistant" && index === messages.length - 1 ? (
                    <span className={styles.streamingDots} aria-label="Thánh Gióng vẫn đang trả lời"><i /><i /><i /></span>
                  ) : null}
                </div>
              ) : loading && index === messages.length - 1 ? <div className={styles.thinking} aria-label="Thánh Gióng đang trả lời"><i /><i /><i /></div> : null}
            </div>
          </div>
        ))}
        <div ref={endRef} />
      </div>

      {image ? <div className={styles.imagePreview}><img src={image.previewUrl} alt="Ảnh chuẩn bị gửi" /><button type="button" onClick={discardImage} aria-label="Bỏ ảnh"><AppIcon name="close" size={14} /></button></div> : null}

      <form ref={formRef} className={styles.chatForm} onSubmit={send} onPaste={onPaste}>
        <input ref={fileRef} type="file" accept="image/*" onChange={onFile} className={styles.fileInput} />
        <button type="button" className={styles.attachBtn} onClick={() => fileRef.current?.click()} aria-label="Đính kèm ảnh" disabled={imageUploading}>
          <AppIcon name="paperclip" size={18} />
        </button>
        <textarea value={input} onChange={(event) => setInput(event.target.value)} onKeyDown={onComposerKeyDown} onPaste={onPaste} placeholder="Nhắn Thánh Gióng…" aria-label="Nội dung chat" rows={1} />
        <button className={styles.sendBtn} type="submit" disabled={loading || imageUploading || (!input.trim() && !image)} aria-label="Gửi">
          <AppIcon name="send" size={18} />
        </button>
      </form>
      {notice ? <div className={styles.chatHint}>{notice}</div> : null}
    </section>
  );
}
