"use client";
/* eslint-disable @next/next/no-img-element -- chat images are user-provided data/blob previews and should not be optimized remotely */

import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useMemo, useRef, useState, type ChangeEvent, type ClipboardEvent, type FormEvent, type KeyboardEvent } from "react";
import { isWorkspacesResource, type WorkspacesResource } from "@/lib/contracts/resources";
import { AppIcon } from "./app-icon";
import { ChatRichMessage } from "./chat-rich-message";
import { useDashboardResource } from "./use-dashboard-resource";
import styles from "./dashboard-chat.module.css";
import skillStyles from "./skill-indicator.module.css";

type ToolCall = {
  id: string;
  name: string;
  arguments: string;
  result?: string;
  durationMs?: number;
  status: "running" | "done" | "error" | "approval_required";
};

type SkillBadge = {
  id: string;
  name: string;
  version: string;
};

type ChatMsg = {
  role: "user" | "assistant";
  content: string;
  tool_calls?: ToolCall[];
  image?: string;
  skills?: SkillBadge[];
};

type ChatMode = "ask" | "plan" | "agent";

type ChatThread = {
  id: string;
  title: string;
  model: string;
  workspaceKey?: string;
  createdAt: number;
  updatedAt: number;
};

type ChatTokenUsage = {
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
};

type StreamData = {
  delta?: string;
  content?: string;
  reply?: string;
  error?: string;
  threadId?: string;
  tool_calls?: ToolCall[] | Array<{ index: number; name?: string; arguments?: string; id?: string }>;
  usage?: ChatTokenUsage;
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

function modelLabel(model: string) {
  switch (model) {
    case "auto": return "Auto";
    case "glm-5.3-flash": return "GLM-5.3-Flash";
    case "qwen3.8-flash": return "Qwen3.8-Flash";
    case "muse-spark-1.3-contributor-free": return "Muse Spark 1.3 Contributor Free";
    case "muse-spark-1.2-contributor-free": return "Muse Spark 1.2";
    default: return model;
  }
}

function modelStrengthScore(model: string) {
  const id = model.toLowerCase();
  let score = 500;

  if (id.includes("gpt-5.6")) score = 1100;
  else if (id.includes("claude-opus")) score = 1080;
  else if (id.includes("gpt-5.4")) score = 1040;
  else if (id.includes("gemini") && id.includes("pro")) score = 1010;
  else if (id.includes("claude-sonnet")) score = 990;
  else if (id.includes("kimi")) score = 930;
  else if (id.includes("qwen")) score = 900;
  else if (id.includes("glm")) score = 880;
  else if (id.includes("deepseek")) score = 860;
  else if (id.includes("minimax")) score = 820;
  else if (id.includes("gemini")) score = 800;
  else if (id.includes("gpt")) score = 790;
  else if (id.includes("claude")) score = 780;

  if (id.includes("sol")) score += 35;
  if (id.includes("opus")) score += 30;
  if (id.includes("reasoning") || id.includes("thinking")) score += 24;
  if (id.includes("pro")) score += 18;
  if (id.includes("high")) score += 12;
  if (id.includes("agent")) score += 5;
  if (id.includes("flash")) score -= 28;
  if (id.includes("low")) score -= 45;
  if (id.includes("mini")) score -= 70;
  if (id.includes("lite")) score -= 90;
  if (id.includes("nano")) score -= 110;

  return score;
}

function compareModelsByStrength(left: string, right: string) {
  const scoreDelta = modelStrengthScore(right) - modelStrengthScore(left);
  return scoreDelta || left.localeCompare(right, "en", { numeric: true, sensitivity: "base" });
}

function workspaceKey(workspace: WorkspaceItem) {
  return `${workspace.deviceId}::${workspace.workspaceId}`;
}

function toolLabel(name: string, status: ToolCall["status"]) {
  const labels: Record<string, [string, string, string]> = {
    list_workspaces: ["Đang xem workspaces", "Đã xem workspaces", "Xem workspaces"],
    list_devices: ["Đang kiểm tra thiết bị", "Đã kiểm tra thiết bị", "Kiểm tra thiết bị"],
    search_project_brain: ["Đang truy vấn Brain", "Đã truy vấn Brain", "Truy vấn Brain"],
    get_workspace_detail: ["Đang đọc dự án", "Đã đọc dự án", "Đọc dự án"],
    list_project_files: ["Đang xem mã nguồn", "Đã xem mã nguồn", "Xem mã nguồn"],
    read_project_file: ["Đang đọc file", "Đã đọc file", "Đọc file"],
    search_project_code: ["Đang tìm trong code", "Đã tìm trong code", "Tìm trong code"],
    edit_project_file: ["Đang sửa code", "Đã sửa code", "Sửa code"],
    write_project_file: ["Đang cập nhật file", "Đã cập nhật file", "Cập nhật file"],
    apply_project_patch: ["Đang áp dụng thay đổi", "Đã áp dụng thay đổi", "Áp dụng thay đổi"],
    run_project_command: ["Đang chạy lệnh", "Đã chạy lệnh", "Chạy lệnh"],
    poll_project_command: ["Đang chờ lệnh", "Lệnh đã kết thúc", "Chờ lệnh"],
    verify_project_changes: ["Đang kiểm tra thay đổi", "Đã kiểm tra thay đổi", "Kiểm tra thay đổi"],
  };
  const label = labels[name];
  if (!label) return name.replaceAll("_", " ");
  if (status === "running") return label[0];
  if (status === "done") return label[1];
  return label[2];
}

function parseSkillBadges(value: unknown): SkillBadge[] {
  if (!Array.isArray(value)) return [];
  return value.flatMap((item) => {
    if (!item || typeof item !== "object") return [];
    const record = item as Record<string, unknown>;
    const id = typeof record.id === "string" ? record.id.trim() : "";
    const name = typeof record.name === "string" ? record.name.trim() : "";
    const version = typeof record.version === "string" ? record.version.trim() : "";
    return id && name && version ? [{ id, name, version }] : [];
  }).slice(0, 3);
}

function parseSkillHeader(raw: string | null): SkillBadge[] {
  if (!raw) return [];
  try {
    return parseSkillBadges(JSON.parse(raw) as unknown);
  } catch {
    return [];
  }
}

function parseHistorySkills(raw: string | SkillBadge[] | undefined): SkillBadge[] {
  if (Array.isArray(raw)) return parseSkillBadges(raw);
  return parseSkillHeader(raw ?? null);
}

function historyMessages(value: unknown): ChatMsg[] {
  if (!value || typeof value !== "object") return [];
  const records = (value as { messages?: unknown }).messages;
  if (!Array.isArray(records)) return [];
  return records.flatMap((entry) => {
    if (!entry || typeof entry !== "object") return [];
    const message = entry as { role?: unknown; content?: unknown; tool_calls?: string | ToolCall[]; skills?: string | SkillBadge[]; image?: unknown };
    if ((message.role !== "user" && message.role !== "assistant") || typeof message.content !== "string") return [];
    let toolCalls: ToolCall[] | undefined;
    if (typeof message.tool_calls === "string") {
      try {
        const parsed = JSON.parse(message.tool_calls) as unknown;
        if (Array.isArray(parsed)) toolCalls = parsed as ToolCall[];
      } catch {
        toolCalls = undefined;
      }
    } else if (Array.isArray(message.tool_calls)) {
      toolCalls = message.tool_calls;
    }
    const skills = parseHistorySkills(message.skills);
    return [{
      role: message.role,
      content: message.content,
      tool_calls: toolCalls,
      skills: skills.length ? skills : undefined,
      image: typeof message.image === "string" ? message.image : undefined,
    } satisfies ChatMsg];
  }).slice(-50);
}

function threadGroupLabel(updatedAt: number) {
  const startToday = new Date();
  startToday.setHours(0, 0, 0, 0);
  const age = startToday.getTime() - updatedAt;
  if (age <= 0) return "Hôm nay";
  if (age < 24 * 60 * 60 * 1000) return "Hôm qua";
  if (age < 7 * 24 * 60 * 60 * 1000) return "7 ngày qua";
  return "Cũ hơn";
}

const chatTransportRetryDelays = [350, 900, 1800];

function createChatRequestId() {
  if (typeof crypto.randomUUID === "function") return crypto.randomUUID();
  return `chat-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 12)}`;
}

function isRetryableChatTransportError(error: unknown) {
  const message = error instanceof Error ? error.message : String(error);
  return /load failed|failed to fetch|fetch failed|network request failed|network error|body stream|connection.*lost|terminated|chat_stream_incomplete|HTTP (408|425|429|500|502|503|504)/i.test(message);
}

function waitForChatRetry(delayMs: number, signal: AbortSignal) {
  return new Promise<void>((resolve, reject) => {
    if (signal.aborted) {
      reject(new DOMException("Aborted", "AbortError"));
      return;
    }
    const timer = window.setTimeout(() => {
      signal.removeEventListener("abort", onAbort);
      resolve();
    }, delayMs);
    const onAbort = () => {
      window.clearTimeout(timer);
      reject(new DOMException("Aborted", "AbortError"));
    };
    signal.addEventListener("abort", onAbort, { once: true });
  });
}

function friendlyChatFailure(message: string) {
  if (/508|tool loop|loop exceeded/i.test(message)) {
    return "Luồng xử lý vừa quá dài. Thánh Gióng đã giữ lại phần đã làm; gửi “tiếp tục” để nối tiếp ngay.";
  }
  if (/load failed|chat_stream_incomplete|timeout|429|502|503|504|network|fetch/i.test(message)) {
    return "Kết nối vừa gián đoạn sau nhiều lần tự nối lại. Phần đã hoàn thành vẫn được giữ nguyên; gửi “tiếp tục” để nối tiếp.";
  }
  return message.startsWith("Thánh Gióng") || message.startsWith("Kết nối") ? message : `Có lỗi khi xử lý yêu cầu: ${message}`;
}

export function DashboardChat() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [messages, setMessages] = useState<ChatMsg[]>([]);
  const [input, setInput] = useState("");
  const [loading, setLoading] = useState(false);
  const [mode, setMode] = useState<ChatMode>("agent");
  const [goal, setGoal] = useState("");
  const [image, setImage] = useState<PreparedChatImage | null>(null);
  const [imageUploading, setImageUploading] = useState(false);
  const [notice, setNotice] = useState("");
  const [tokenUsage, setTokenUsage] = useState<ChatTokenUsage | null>(null);
  const [threads, setThreads] = useState<ChatThread[]>([]);
  const [activeThreadId, setActiveThreadId] = useState<string | null>(null);
  const [threadSearch, setThreadSearch] = useState("");
  const [threadActionLoading, setThreadActionLoading] = useState(true);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [models, setModels] = useState<string[]>(["auto"]);
  const [selectedModel, setSelectedModel] = useState("auto");
  const [modelPickerOpen, setModelPickerOpen] = useState(false);
  const [modelSearch, setModelSearch] = useState("");
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
  const streamAbortRef = useRef<AbortController | null>(null);
  const modelPickerRef = useRef<HTMLDivElement>(null);

  const workspaceItems = useMemo(
    () => workspaces.state.kind === "ready" ? workspaces.state.value.items : [],
    [workspaces.state],
  );
  const selectedWorkspace = useMemo(
    () => workspaceItems.find((workspace) => workspaceKey(workspace) === selectedWorkspaceKey),
    [selectedWorkspaceKey, workspaceItems],
  );
  const poolModels = useMemo(
    () => models.filter((model) => model !== "auto").sort(compareModelsByStrength),
    [models],
  );
  const visiblePoolModels = useMemo(() => {
    const query = modelSearch.trim().toLocaleLowerCase("vi");
    if (!query) return poolModels;
    return poolModels.filter((model) => `${model} ${modelLabel(model)}`.toLocaleLowerCase("vi").includes(query));
  }, [modelSearch, poolModels]);
  const threadGroups = useMemo(() => {
    const query = threadSearch.trim().toLocaleLowerCase("vi");
    const visible = query ? threads.filter((thread) => thread.title.toLocaleLowerCase("vi").includes(query)) : threads;
    const labels = ["Hôm nay", "Hôm qua", "7 ngày qua", "Cũ hơn"];
    return labels.map((label) => ({ label, threads: visible.filter((thread) => threadGroupLabel(thread.updatedAt) === label) })).filter((group) => group.threads.length > 0);
  }, [threadSearch, threads]);
  const activeThreadModel = threads.find((thread) => thread.id === activeThreadId)?.model;
  const activeThreadWorkspaceKey = threads.find((thread) => thread.id === activeThreadId)?.workspaceKey;

  useEffect(() => {
    let cancelled = false;
    fetch("/api/v1/dashboard/chat/threads", { credentials: "include" })
      .then(async (response) => {
        if (response.status === 401) {
          router.replace("/login");
          return null;
        }
        if (!response.ok) throw new Error(`threads ${response.status}`);
        return response.json() as Promise<{ threads?: ChatThread[] }>;
      })
      .then(async (data) => {
        if (cancelled || !data) return;
        let available = Array.isArray(data.threads) ? data.threads : [];
        if (available.length === 0) {
          const response = await fetch("/api/v1/dashboard/chat/threads", {
            method: "POST",
            credentials: "include",
            headers: { "content-type": "application/json" },
            body: JSON.stringify({ model: "auto" }),
          });
          if (!response.ok) throw new Error(`create thread ${response.status}`);
          const created = (await response.json()) as { thread?: ChatThread };
          available = created.thread ? [created.thread] : [];
        }
        if (cancelled) return;
        setThreads(available);
        if (available[0]) {
          setMessages([]);
          setHistoryLoading(true);
          setActiveThreadId(available[0].id);
        }
      })
      .catch(() => {
        if (!cancelled) setNotice("Không tải được danh sách cuộc trò chuyện");
      })
      .finally(() => {
        if (!cancelled) setThreadActionLoading(false);
      });
    return () => { cancelled = true; };
  }, [router]);

  useEffect(() => {
    if (!activeThreadId) {
      return;
    }
    const controller = new AbortController();
    fetch(`/api/v1/dashboard/chat/history?threadId=${encodeURIComponent(activeThreadId)}`, { credentials: "include", signal: controller.signal })
      .then(async (response) => {
        if (response.status === 401) {
          router.replace("/login");
          return null;
        }
        if (!response.ok) throw new Error(`history ${response.status}`);
        return response.json() as Promise<unknown>;
      })
      .then((data) => {
        if (data) setMessages(historyMessages(data));
      })
      .catch((error: unknown) => {
        if (!(error instanceof DOMException && error.name === "AbortError")) setNotice("Không tải được lịch sử chat");
      })
      .finally(() => {
        if (!controller.signal.aborted) setHistoryLoading(false);
      });
    return () => controller.abort();
  }, [activeThreadId, router]);

  useEffect(() => {
    if (!activeThreadId) return;
    let cancelled = false;
    queueMicrotask(() => {
      if (cancelled) return;
      setSelectedModel(activeThreadModel && models.includes(activeThreadModel) ? activeThreadModel : "auto");
      const storedWorkspace = activeThreadWorkspaceKey || "auto";
      setSelectedWorkspaceKey(storedWorkspace === "auto" || workspaceItems.some((workspace) => workspaceKey(workspace) === storedWorkspace) ? storedWorkspace : "auto");
    });
    return () => { cancelled = true; };
  }, [activeThreadId, activeThreadModel, activeThreadWorkspaceKey, models, workspaceItems]);

  useEffect(() => {
    fetch("/api/v1/dashboard/models", { credentials: "include" })
      .then(async (response) => {
        if (!response.ok) throw new Error(String(response.status));
        return response.json() as Promise<{ models?: string[]; default_model?: string }>;
      })
      .then((data) => {
        const available = Array.isArray(data.models) && data.models.length ? data.models : ["auto"];
        setModels(available);
        setSelectedModel(data.default_model && available.includes(data.default_model) ? data.default_model : available[0]);
      })
      .catch(() => {
        setModels(["auto"]);
        setSelectedModel("auto");
      });
  }, []);

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: "smooth", block: "end" });
  }, [messages, loading]);

  useEffect(() => {
    if (!modelPickerOpen) return;
    const closeOnOutsideClick = (event: PointerEvent) => {
      if (modelPickerRef.current && !modelPickerRef.current.contains(event.target as Node)) {
        setModelPickerOpen(false);
        setModelSearch("");
      }
    };
    const closeOnEscape = (event: globalThis.KeyboardEvent) => {
      if (event.key === "Escape") {
        setModelPickerOpen(false);
        setModelSearch("");
      }
    };
    document.addEventListener("pointerdown", closeOnOutsideClick);
    document.addEventListener("keydown", closeOnEscape);
    return () => {
      document.removeEventListener("pointerdown", closeOnOutsideClick);
      document.removeEventListener("keydown", closeOnEscape);
    };
  }, [modelPickerOpen]);

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

  function updateAssistant(index: number, content: string, toolCalls: ToolCall[], skills?: SkillBadge[]) {
    setMessages((current) => {
      const copy = [...current];
      const preservedSkills = skills ?? copy[index]?.skills;
      copy[index] = {
        role: "assistant",
        content,
        tool_calls: [...toolCalls],
        skills: preservedSkills?.length ? [...preservedSkills] : undefined,
      };
      return copy;
    });
  }

  function finishStoppedAssistant(index: number, content: string, toolCalls: ToolCall[]) {
    setMessages((current) => {
      const copy = [...current];
      if (!content.trim() && toolCalls.length === 0) {
        copy.splice(index, 1);
        return copy;
      }
      const preservedSkills = copy[index]?.skills;
      copy[index] = {
        role: "assistant",
        content,
        tool_calls: [...toolCalls],
        skills: preservedSkills?.length ? [...preservedSkills] : undefined,
      };
      return copy;
    });
  }

  function submitQuickMessage(message: string) {
    if (loading || historyLoading || threadActionLoading) return;
    quickMessageRef.current = message;
    formRef.current?.requestSubmit();
  }

  async function createThreadRecord() {
    const response = await fetch("/api/v1/dashboard/chat/threads", {
      method: "POST",
      credentials: "include",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({
        model: selectedModel,
        workspaceKey: selectedWorkspaceKey === "auto" ? "" : selectedWorkspaceKey,
      }),
    });
    if (response.status === 401) {
      router.replace("/login");
      throw new Error("Phiên đăng nhập đã hết hạn");
    }
    if (!response.ok) throw new Error(`create thread ${response.status}`);
    const data = (await response.json()) as { thread?: ChatThread };
    if (!data.thread) throw new Error("missing thread");
    return data.thread;
  }

  async function refreshThreads(preferredThreadId?: string) {
    const response = await fetch("/api/v1/dashboard/chat/threads", { credentials: "include" });
    if (!response.ok) return;
    const data = (await response.json()) as { threads?: ChatThread[] };
    if (!Array.isArray(data.threads)) return;
    setThreads(data.threads);
    if (preferredThreadId && preferredThreadId !== activeThreadId && data.threads.some((thread) => thread.id === preferredThreadId)) {
      setMessages([]);
      setHistoryLoading(true);
      setActiveThreadId(preferredThreadId);
    }
  }

  async function newThread() {
    if (loading || threadActionLoading) return;
    setThreadActionLoading(true);
    setNotice("");
    try {
      const thread = await createThreadRecord();
      setThreads((current) => [thread, ...current]);
      setHistoryLoading(true);
      setActiveThreadId(thread.id);
      setMessages([]);
      setInput("");
      discardImage();
    } catch {
      setNotice("Không thể tạo cuộc trò chuyện mới");
    } finally {
      setThreadActionLoading(false);
    }
  }

  async function renameThread(thread: ChatThread) {
    if (loading || threadActionLoading) return;
    const title = window.prompt("Tên cuộc trò chuyện", thread.title)?.trim();
    if (!title || title === thread.title) return;
    setThreadActionLoading(true);
    try {
      const response = await fetch(`/api/v1/dashboard/chat/threads/${encodeURIComponent(thread.id)}`, {
        method: "PATCH",
        credentials: "include",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ title }),
      });
      if (!response.ok) throw new Error(String(response.status));
      const data = (await response.json()) as { thread?: ChatThread };
      setThreads((current) => {
        const updated = data.thread || { ...thread, title, updatedAt: Date.now() };
        return [updated, ...current.filter((item) => item.id !== thread.id)];
      });
    } catch {
      setNotice("Không thể đổi tên cuộc trò chuyện");
    } finally {
      setThreadActionLoading(false);
    }
  }

  async function deleteThread(thread: ChatThread) {
    if (loading || threadActionLoading || !window.confirm(`Xóa “${thread.title}”?`)) return;
    setThreadActionLoading(true);
    try {
      const response = await fetch(`/api/v1/dashboard/chat/threads/${encodeURIComponent(thread.id)}`, { method: "DELETE", credentials: "include" });
      if (!response.ok) throw new Error(String(response.status));
      let remaining = threads.filter((item) => item.id !== thread.id);
      if (remaining.length === 0) {
        const replacement = await createThreadRecord();
        remaining = [replacement];
      }
      setThreads(remaining);
      if (activeThreadId === thread.id) {
        setHistoryLoading(true);
        setActiveThreadId(remaining[0].id);
        setMessages([]);
      }
    } catch {
      setNotice("Không thể xóa cuộc trò chuyện");
    } finally {
      setThreadActionLoading(false);
    }
  }

  async function send(event: FormEvent) {
    event.preventDefault();
    const quickMessage = quickMessageRef.current;
    quickMessageRef.current = null;
    const text = (quickMessage ?? input).trim();
    if ((!text && !image) || loading || imageUploading || historyLoading || threadActionLoading) return;
    let requestThreadId = activeThreadId;
    if (!requestThreadId) {
      try {
        const thread = await createThreadRecord();
        requestThreadId = thread.id;
        setThreads((current) => [thread, ...current]);
        setHistoryLoading(true);
        setActiveThreadId(thread.id);
      } catch {
        setNotice("Không thể tạo cuộc trò chuyện để gửi tin nhắn");
        return;
      }
    }

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
    const controller = new AbortController();
    streamAbortRef.current = controller;
    const finishStoppedResponse = () => {
      finishStoppedAssistant(placeholderIndex, streamedContent, streamedToolCalls);
      setNotice(streamedContent.trim() || streamedToolCalls.length ? "Đã dừng trả lời; phần đã nhận vẫn được giữ lại." : "Đã dừng trước khi có phản hồi.");
    };
    try {
      const requestId = createChatRequestId();
      const history = messages.slice(-12).map((message) => ({ role: message.role, content: message.content }));
      const payload = {
        requestId,
        threadId: requestThreadId,
        message: text || "Phân tích ảnh này",
        history,
        model: selectedModel,
        mode,
        goal: goal.trim() || undefined,
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
      let activeSkills: SkillBadge[] = [];
      let completed = false;
      let lastError: unknown;

      for (let attempt = 0; attempt < chatTransportRetryDelays.length; attempt += 1) {
        if (attempt > 0) {
          setNotice(`Kết nối chập chờn · đang tự nối lại (${attempt + 1}/${chatTransportRetryDelays.length})…`);
          await waitForChatRetry(chatTransportRetryDelays[attempt - 1], controller.signal);
        }
        try {
          let requestBody: BodyInit;
          const requestHeaders: HeadersInit = {
            accept: "text/event-stream",
            "x-codelocal-request-id": requestId,
          };
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
            signal: controller.signal,
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
            const prefix = data.error ? `${data.error} · ` : "";
            throw new Error(`${prefix}HTTP ${response.status}`);
          }

          const responseSkills = parseSkillHeader(response.headers.get("x-codelocal-skills"));
          if (responseSkills.length) activeSkills = responseSkills;
          if (activeSkills.length) updateAssistant(placeholderIndex, streamedContent, streamedToolCalls, activeSkills);

          if (!(response.headers.get("content-type") || "").includes("text/event-stream")) {
            const data = (await response.json()) as { reply?: string; tool_calls?: ToolCall[]; error?: string; threadId?: string };
            if (data.error) throw new Error(data.error);
            if (data.threadId) requestThreadId = data.threadId;
            streamedContent = data.reply || "";
            streamedToolCalls = data.tool_calls || [];
            updateAssistant(placeholderIndex, streamedContent, streamedToolCalls, activeSkills);
            completed = true;
            setNotice("");
            break;
          }

          const reader = response.body.getReader();
          const decoder = new TextDecoder();
          let buffer = "";
          let content = "";
          let toolCalls: ToolCall[] = [];
          let sawDone = false;
          streamedContent = content;
          streamedToolCalls = toolCalls;
          if (attempt > 0) updateAssistant(placeholderIndex, "", [], activeSkills);

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
                updateAssistant(placeholderIndex, content, toolCalls, activeSkills);
                continue;
              }
              if (eventName === "replace" && typeof data.content === "string") {
                content = data.content;
                streamedContent = content;
                updateAssistant(placeholderIndex, content, toolCalls, activeSkills);
                continue;
              }
              if (eventName === "tool_calls" && Array.isArray(data.tool_calls)) {
                toolCalls = data.tool_calls as ToolCall[];
                streamedToolCalls = toolCalls;
                updateAssistant(placeholderIndex, content, toolCalls, activeSkills);
                continue;
              }
              if (eventName === "tool_delta" && Array.isArray(data.tool_calls)) {
                const deltas = data.tool_calls as Array<{ index: number; name?: string; arguments?: string; id?: string }>;
                for (const delta of deltas) {
                  const existing = toolCalls[delta.index] || { id: delta.id || `tool_${delta.index}`, name: "", arguments: "", status: "running" as const };
                  toolCalls[delta.index] = {
                    ...existing,
                    id: delta.id || existing.id,
                    name: delta.name || existing.name,
                    arguments: delta.arguments ?? existing.arguments,
                  };
                }
                streamedToolCalls = toolCalls;
                updateAssistant(placeholderIndex, content, toolCalls, activeSkills);
                continue;
              }
              if (eventName === "done") {
                sawDone = true;
                if (typeof data.threadId === "string" && data.threadId) requestThreadId = data.threadId;
                if (typeof data.reply === "string") content = data.reply;
                if (Array.isArray(data.tool_calls)) toolCalls = data.tool_calls as ToolCall[];
                if (data.usage) setTokenUsage(data.usage);
                streamedContent = content;
                streamedToolCalls = toolCalls;
                updateAssistant(placeholderIndex, content, toolCalls, activeSkills);
              }
            }
          }
          if (controller.signal.aborted) {
            finishStoppedResponse();
            return;
          }
          if (!sawDone) throw new Error("chat_stream_incomplete");
          completed = true;
          setNotice("");
          break;
        } catch (attemptError) {
          if (controller.signal.aborted) {
            finishStoppedResponse();
            return;
          }
          lastError = attemptError;
          if (!isRetryableChatTransportError(attemptError) || attempt === chatTransportRetryDelays.length - 1) {
            throw attemptError;
          }
        }
      }
      if (!completed && lastError) throw lastError;
    } catch (error) {
      if (controller.signal.aborted) {
        finishStoppedResponse();
        return;
      }
      const message = error instanceof Error ? error.message : String(error);
      const failure = friendlyChatFailure(message);
      const content = streamedContent ? `${streamedContent}\n\n${failure}` : failure;
      updateAssistant(placeholderIndex, content, streamedToolCalls);
    } finally {
      if (streamAbortRef.current === controller) streamAbortRef.current = null;
      setLoading(false);
      if (requestThreadId) void refreshThreads(requestThreadId);
    }
  }

  function stopStream() {
    const controller = streamAbortRef.current;
    if (!controller || controller.signal.aborted) return;
    setNotice("Đang dừng trả lời…");
    controller.abort("user_stop");
  }

  async function clear() {
    if (!activeThreadId) return;
    setMessages([]);
    setNotice("");
    try {
      const response = await fetch(`/api/v1/dashboard/chat/history?threadId=${encodeURIComponent(activeThreadId)}`, { method: "DELETE", credentials: "include" });
      if (!response.ok) throw new Error(String(response.status));
    } catch {
      setNotice("Không thể xóa lịch sử trên server");
    }
  }

  return (
    <section className={styles.chatWorkspace} aria-label="Không gian trò chuyện Thánh Gióng">
      <aside className={styles.threadSidebar} aria-label="Các cuộc trò chuyện">
        <div className={styles.threadSidebarHead}>
          <div>
            <span>Lịch sử</span>
            <strong>Cuộc trò chuyện</strong>
          </div>
          <span className={styles.threadCount}>{threads.length}</span>
        </div>
        <button className={styles.newThreadButton} type="button" onClick={() => void newThread()} disabled={loading || threadActionLoading}>
          <AppIcon name="plus" size={16} />
          Cuộc trò chuyện mới
        </button>
        <label className={styles.threadSearch}>
          <AppIcon name="search" size={15} />
          <input value={threadSearch} onChange={(event) => setThreadSearch(event.target.value)} placeholder="Tìm cuộc trò chuyện" aria-label="Tìm cuộc trò chuyện" />
        </label>
        <div className={styles.threadList}>
          {threadGroups.length === 0 ? <p className={styles.threadEmpty}>Không tìm thấy cuộc trò chuyện.</p> : threadGroups.map((group) => (
            <section className={styles.threadGroup} key={group.label}>
              <h2>{group.label}</h2>
              {group.threads.map((thread) => (
                <div className={`${styles.threadItem} ${thread.id === activeThreadId ? styles.threadItemActive : ""}`} key={thread.id}>
                  <button className={styles.threadSelect} type="button" onClick={() => {
                    if (thread.id === activeThreadId) return;
                    setMessages([]);
                    setHistoryLoading(true);
                    setActiveThreadId(thread.id);
                  }} disabled={loading || historyLoading} title={thread.title}>
                    <AppIcon name="chat" size={15} />
                    <span>{thread.title}</span>
                  </button>
                  <div className={styles.threadItemActions}>
                    <button type="button" onClick={() => void renameThread(thread)} aria-label={`Đổi tên ${thread.title}`} title="Đổi tên"><AppIcon name="edit" size={13} /></button>
                    <button type="button" onClick={() => void deleteThread(thread)} aria-label={`Xóa ${thread.title}`} title="Xóa"><AppIcon name="trash" size={13} /></button>
                  </div>
                </div>
              ))}
            </section>
          ))}
        </div>
        <div className={styles.threadSidebarFoot}>
          Lưu theo tài khoản CodeLocal
        </div>
      </aside>

      <section className={styles.chatShell} aria-label="Chat với Thánh Gióng">
        <div className={styles.chatHead}>
          <div className={styles.brandBlock}>
            <span className={styles.avatar} aria-hidden="true"><AppIcon name="thanh-giong" size={22} /></span>
            <div className={styles.nameRow}><h1>Thánh Gióng</h1><i /></div>
          </div>
          <div className={styles.chatActions}>
            {tokenUsage ? (
              <span className={styles.tokenUsage} title={`${tokenUsage.inputTokens.toLocaleString()} input · ${tokenUsage.outputTokens.toLocaleString()} output`}>
                {tokenUsage.totalTokens.toLocaleString()} tokens
              </span>
            ) : null}
            <div className={styles.modelPickerShell} ref={modelPickerRef}>
              <button
                className={`${styles.projectPicker} ${styles.modelPicker}`}
                type="button"
                aria-label="Chọn model"
                aria-haspopup="listbox"
                aria-expanded={modelPickerOpen}
                onClick={() => setModelPickerOpen((open) => !open)}
              >
                <span className={styles.modelPickerName}>{modelLabel(selectedModel)}</span>
                <span className={styles.modelPickerCount}>{poolModels.length}</span>
                <span className={styles.modelPickerChevron} aria-hidden="true">⌄</span>
              </button>
              {modelPickerOpen ? (
                <div className={styles.modelPickerMenu} role="dialog" aria-label="Tìm và chọn model">
                  <label className={styles.modelSearch}>
                    <AppIcon name="search" size={15} />
                    <input
                      autoFocus
                      value={modelSearch}
                      onChange={(event) => setModelSearch(event.target.value)}
                      placeholder="Tìm GPT, Claude, Gemini..."
                      aria-label="Tìm model"
                    />
                  </label>
                  <div className={styles.modelPickerSummary}>CodeLocal Pool · {poolModels.length} Active · mạnh → nhẹ</div>
                  <div className={styles.modelOptionList} role="listbox" aria-label="Model đang hoạt động">
                    {!modelSearch.trim() ? (
                      <button
                        className={`${styles.modelOption} ${selectedModel === "auto" ? styles.modelOptionActive : ""}`}
                        type="button"
                        role="option"
                        aria-selected={selectedModel === "auto"}
                        onClick={() => {
                          setSelectedModel("auto");
                          setModelPickerOpen(false);
                          setModelSearch("");
                        }}
                      >
                        <span>Auto</span>
                        <small>Pool tự chọn model mặc định</small>
                      </button>
                    ) : null}
                    {visiblePoolModels.map((model) => (
                      <button
                        className={`${styles.modelOption} ${selectedModel === model ? styles.modelOptionActive : ""}`}
                        type="button"
                        role="option"
                        aria-selected={selectedModel === model}
                        key={model}
                        onClick={() => {
                          setSelectedModel(model);
                          setModelPickerOpen(false);
                          setModelSearch("");
                        }}
                      >
                        <span>{modelLabel(model)}</span>
                        {modelLabel(model) !== model ? <small>{model}</small> : null}
                      </button>
                    ))}
                    {visiblePoolModels.length === 0 ? <p className={styles.modelEmpty}>Không tìm thấy model Active.</p> : null}
                  </div>
                </div>
              ) : null}
            </div>
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
          {historyLoading ? <div className={styles.historyLoading}>Đang tải cuộc trò chuyện…</div> : messages.length === 0 ? (
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
                {message.skills?.length ? (
                  <div className={skillStyles.list} aria-label="Skills đang được CodeLocal sử dụng">
                    {message.skills.map((skill) => (
                      <span className={skillStyles.pill} key={`${skill.id}@${skill.version}`} title={`CodeLocal tự chọn ${skill.name}@${skill.version} cho task này`}>
                        <AppIcon name="skill" size={13} />
                        {skill.name}
                      </span>
                    ))}
                  </div>
                ) : null}
                {message.tool_calls?.length ? (
                  <div className={styles.toolList}>
                    {message.tool_calls.map((tool) => (
                      <div key={tool.id} className={styles.toolActionRow}>
                        <details className={styles.toolPill}>
                          <summary>
                            <span className={styles.toolName}><span className={styles.toolDot} />{toolLabel(tool.name, tool.status)}</span>
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
          <textarea value={input} onChange={(event) => setInput(event.target.value)} onKeyDown={onComposerKeyDown} onPaste={onPaste} placeholder="Nhắn Thánh Gióng…" aria-label="Nội dung chat" rows={1} />
          <div className={styles.composerToolbar}>
            <div className={styles.composerOptions}>
              <button type="button" className={styles.attachBtn} onClick={() => fileRef.current?.click()} aria-label="Đính kèm ảnh" disabled={loading || imageUploading}>
                <AppIcon name="paperclip" size={17} />
              </button>
              <label className={styles.modePicker} title={mode === "agent" ? "Agent có thể chỉnh sửa và chạy lệnh" : `${mode === "ask" ? "Ask" : "Plan"} chỉ dùng công cụ đọc`}>
                <select value={mode} onChange={(event) => setMode(event.target.value as ChatMode)} aria-label="Chọn chế độ chat" disabled={loading}>
                  <option value="ask">Ask</option>
                  <option value="plan">Plan</option>
                  <option value="agent">Agent</option>
                </select>
              </label>
              <label className={`${styles.goalField} ${goal ? styles.goalFieldActive : ""}`}>
                <AppIcon name="target" size={13} />
                <input value={goal} onChange={(event) => setGoal(event.target.value)} placeholder="Thêm mục tiêu" aria-label="Mục tiêu" maxLength={240} disabled={loading} />
                {goal ? <button className={styles.goalClear} type="button" onClick={() => setGoal("")} aria-label="Xóa mục tiêu" disabled={loading}><AppIcon name="close" size={11} /></button> : null}
              </label>
            </div>
            <button className={`${styles.sendBtn} ${loading ? styles.stopBtn : ""}`} type={loading ? "button" : "submit"} onClick={loading ? stopStream : undefined} disabled={loading ? false : imageUploading || historyLoading || threadActionLoading || (!input.trim() && !image)} aria-label={loading ? "Dừng trả lời" : "Gửi"} title={loading ? "Dừng trả lời" : "Gửi"}>
              <AppIcon name={loading ? "stop" : "send"} size={loading ? 16 : 18} />
            </button>
          </div>
        </form>
        {notice ? <div className={styles.chatHint}>{notice}</div> : null}
      </section>
    </section>
  );
}
