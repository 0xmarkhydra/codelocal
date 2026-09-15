"use client";
/* eslint-disable @next/next/no-img-element -- chat images are user-provided data/blob previews and should not be optimized remotely */

import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useMemo, useRef, useState, type ChangeEvent, type ClipboardEvent, type FormEvent, type KeyboardEvent } from "react";
import { isWorkspacesResource, type WorkspacesResource } from "@/lib/contracts/resources";
import { useTranslations } from "@/lib/i18n/provider";
import type { MessageKey } from "@/lib/i18n/messages";
import { AppIcon } from "./app-icon";
import { ChatActionSummary, type ChatToolCall } from "./chat-action-summary";
import { ChatContextSheet } from "./chat-context-sheet";
import { ChatProviderManager } from "./chat-provider-manager";
import { ChatTopBar } from "./chat-top-bar";
import { ChatRichMessage } from "./chat-rich-message";
import { ChatSidebarBrand, ChatSidebarFooter } from "./chat-sidebar-footer";
import { chatViewportFrame } from "./chat-visual-viewport";
import { projectTreeOpen, threadBelongsToWorkspace, threadCreationPayload, toggleExpandedProject, workspaceIdentityKey } from "./chat-workspace-key";
import { useDashboardResource } from "./use-dashboard-resource";
import headerStyles from "./chat-header.module.css";
import mobileStyles from "./chat-mobile.module.css";
import styles from "./dashboard-chat.module.css";
import treeStyles from "./chat-project-tree.module.css";
import skillStyles from "./skill-indicator.module.css";

type ToolCall = ChatToolCall;

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
type ChatContextMode = "smart" | "off" | "aggressive";
type ChatModelOption = {
  id: string;
  label: string;
  provider: string;
  providerId?: string;
  custom?: boolean;
};

type ChatThread = {
  id: string;
  title: string;
  model: string;
  workspaceKey?: string;
  createdAt: number;
  updatedAt: number;
};

type StreamData = {
  delta?: string;
  content?: string;
  reply?: string;
  error?: string;
  threadId?: string;
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

type ChatNotice = { kind: "status" | "error"; text: string } | null;

type MediaPrepareResponse = ChatImageMeta & {
  url: string;
  upload: {
    required: boolean;
    url?: string;
    method?: string;
    headers?: Record<string, string[]>;
  };
};

const suggestions: MessageKey[] = ["Explain this codebase", "Find and fix a bug", "Add tests for recent changes"];

function modelLabel(model: string) {
  return model === "auto" ? "Auto" : model;
}

function workspaceKey(workspace: WorkspaceItem) {
  return workspaceIdentityKey(workspace);
}

function workspaceStatusLabel(workspace: WorkspaceItem): MessageKey {
  if (!workspace.runtimeOnline || workspace.status === "offline") return "Offline";
  if (workspace.status === "sleeping") return "Sleeping";
  return "Online";
}

function workspaceState(workspace: WorkspaceItem) {
  if (!workspace.runtimeOnline || workspace.status === "offline") return "offline";
  return workspace.status;
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

function friendlyChatFailure(raw: string, t: ReturnType<typeof useTranslations>["t"], translateMessage: ReturnType<typeof useTranslations>["message"]) {
  if (/model vision|chưa có model vision/i.test(raw)) {
    return t("CodeLocal does not have a vision model available for this image. Configure CODELOCAL_SHOPAIKEY_API_KEY with a vision-capable model such as gpt-* and try again.");
  }
  if (/chưa bật upload ảnh|media_upload_incomplete|media_not_configured/i.test(raw)) {
    return t("Image upload storage (S3) is not configured. The image can still be sent directly but will only be stored compactly in chat history. Retry the image if the backend reports media_upload_incomplete.");
  }
  if (/Model community miễn phí|không nhận.*ảnh|workspace/i.test(raw)) return raw;
  if (/508|tool loop|loop exceeded/i.test(raw)) {
    return t("That processing run became too long. CodeLocal kept the completed work; send “continue” to resume.");
  }
  const rateLimit = /RATE_LIMIT:(\d+)/.exec(raw);
  if (rateLimit) return t("You are sending too quickly. Try again in {seconds}s.", { seconds: rateLimit[1] });
  if (/load failed|chat_stream_incomplete|timeout|502|503|504|network|fetch/i.test(raw)) {
    return t("The connection was interrupted after several automatic retries. Completed work was preserved; send “continue” to resume.");
  }
  const known = translateMessage(raw);
  return known !== raw ? known : t("An error occurred while processing the request: {message}", { message: raw });
}
export function DashboardChat() {
  const { locale, t, message: translateMessage } = useTranslations();
  const router = useRouter();
  const searchParams = useSearchParams();
  const [messages, setMessages] = useState<ChatMsg[]>([]);
  const [input, setInput] = useState("");
  const [loading, setLoading] = useState(false);
  const [mode, setMode] = useState<ChatMode>("agent");
  const [contextMode, setContextMode] = useState<ChatContextMode>("smart");
  const [goal, setGoal] = useState("");
  const [image, setImage] = useState<PreparedChatImage | null>(null);
  const [imageUploading, setImageUploading] = useState(false);
  const [notice, setNotice] = useState<ChatNotice>(null);
  const [threads, setThreads] = useState<ChatThread[]>([]);
  const [activeThreadId, setActiveThreadId] = useState<string | null>(null);
  const [threadSearch, setThreadSearch] = useState("");
  const [expandedProjects, setExpandedProjects] = useState<Set<string>>(() => {
    const deviceId = searchParams.get("deviceId");
    const workspaceId = searchParams.get("workspaceId");
    return deviceId && workspaceId ? new Set([`${deviceId}::${workspaceId}`]) : new Set();
  });
  const [threadActionLoading, setThreadActionLoading] = useState(true);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [models, setModels] = useState<string[]>(["auto"]);
  const [modelOptions, setModelOptions] = useState<ChatModelOption[]>([{ id: "auto", label: "Auto", provider: "CodeLocal" }]);
  const [selectedModel, setSelectedModel] = useState("auto");
  const [modelPickerOpen, setModelPickerOpen] = useState(false);
  const [providerManagerOpen, setProviderManagerOpen] = useState(false);
  const [modelSearch, setModelSearch] = useState("");
  const [contextSheetOpen, setContextSheetOpen] = useState(false);
  const [threadDrawerOpen, setThreadDrawerOpen] = useState(false);
  const [showJumpLatest, setShowJumpLatest] = useState(false);
  const [selectedWorkspaceKey, setSelectedWorkspaceKey] = useState(() => {
    const deviceId = searchParams.get("deviceId");
    const workspaceId = searchParams.get("workspaceId");
    return deviceId && workspaceId ? `${deviceId}::${workspaceId}` : "auto";
  });
  const workspaces = useDashboardResource("/api/v1/workspaces", isWorkspacesResource);
  const chatFrameRef = useRef<HTMLElement>(null);
  const fileRef = useRef<HTMLInputElement>(null);
  const messagesRef = useRef<HTMLDivElement>(null);
  const endRef = useRef<HTMLDivElement>(null);
  const formRef = useRef<HTMLFormElement>(null);
  const composerRef = useRef<HTMLTextAreaElement>(null);
  const contextTriggerRef = useRef<HTMLButtonElement>(null);
  const followLatestRef = useRef(true);
  const quickMessageRef = useRef<string | null>(null);
  const streamAbortRef = useRef<AbortController | null>(null);
  const modelPickerRef = useRef<HTMLDivElement>(null);
  const mobileDrawerTriggerRef = useRef<HTMLButtonElement>(null);
  const threadDrawerTriggerRef = useRef<HTMLButtonElement>(null);
  const threadDrawerCloseRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    const stored = window.localStorage.getItem("codelocal.chat.contextMode");
    if (stored !== "smart" && stored !== "off" && stored !== "aggressive") return;
    queueMicrotask(() => setContextMode(stored));
  }, []);

  useEffect(() => {
    window.localStorage.setItem("codelocal.chat.contextMode", contextMode);
  }, [contextMode]);

  useEffect(() => {
    const frame = chatFrameRef.current;
    if (!frame) return;
    const viewport = window.visualViewport;
    let animationFrame = 0;
    const updateViewport = () => {
      window.cancelAnimationFrame(animationFrame);
      animationFrame = window.requestAnimationFrame(() => {
        const next = chatViewportFrame(viewport, window.innerHeight);
        frame.style.setProperty("--chat-viewport-height", `${next.height}px`);
        frame.style.setProperty("--chat-viewport-offset-top", `${next.offsetTop}px`);
      });
    };
    updateViewport();
    viewport?.addEventListener("resize", updateViewport);
    viewport?.addEventListener("scroll", updateViewport);
    window.addEventListener("orientationchange", updateViewport);
    return () => {
      window.cancelAnimationFrame(animationFrame);
      viewport?.removeEventListener("resize", updateViewport);
      viewport?.removeEventListener("scroll", updateViewport);
      window.removeEventListener("orientationchange", updateViewport);
      frame.style.removeProperty("--chat-viewport-height");
      frame.style.removeProperty("--chat-viewport-offset-top");
    };
  }, []);

  useEffect(() => {
    if (!threadDrawerOpen) return;
    const previousOverflow = document.body.style.overflow;
    const closeOnEscape = (event: globalThis.KeyboardEvent) => {
      if (event.key !== "Escape") return;
      setThreadDrawerOpen(false);
      mobileDrawerTriggerRef.current?.focus();
    };
    document.documentElement.dataset.chatMenuOpen = "true";
    document.body.style.overflow = "hidden";
    document.addEventListener("keydown", closeOnEscape);
    window.requestAnimationFrame(() => threadDrawerCloseRef.current?.focus());
    return () => {
      delete document.documentElement.dataset.chatMenuOpen;
      document.body.style.overflow = previousOverflow;
      document.removeEventListener("keydown", closeOnEscape);
    };
  }, [threadDrawerOpen]);

  useEffect(() => {
    const textarea = composerRef.current;
    if (!textarea) return;
    textarea.style.height = "auto";
    textarea.style.height = `${Math.min(textarea.scrollHeight, 144)}px`;
  }, [input]);

  const workspaceItems = useMemo(
    () => workspaces.state.kind === "ready" ? workspaces.state.value.items : [],
    [workspaces.state],
  );
  const selectedWorkspace = useMemo(
    () => workspaceItems.find((workspace) => workspaceKey(workspace) === selectedWorkspaceKey),
    [selectedWorkspaceKey, workspaceItems],
  );
  const activeModels = useMemo(
    () => models.filter((model) => model !== "auto"),
    [models],
  );
  const modelOptionMap = useMemo(
    () => new Map(modelOptions.map((option) => [option.id, option])),
    [modelOptions],
  );
  const labelForModel = (model: string) => modelOptionMap.get(model)?.label || modelLabel(model);
  const providerForModel = (model: string) => modelOptionMap.get(model)?.provider || "CodeLocal";
  const visibleModels = useMemo(() => {
    const query = modelSearch.trim().toLocaleLowerCase(locale);
    if (!query) return activeModels;
    return activeModels.filter((model) => {
      const option = modelOptionMap.get(model);
      return `${model} ${option?.label || modelLabel(model)} ${option?.provider || "CodeLocal"}`.toLocaleLowerCase(locale).includes(query);
    });
  }, [locale, modelSearch, activeModels, modelOptionMap]);
  const activeThread = threads.find((thread) => thread.id === activeThreadId);
  const activeThreadModel = activeThread?.model;
  const activeThreadWorkspaceKey = activeThread?.workspaceKey;
  const threadTree = useMemo(() => {
    const query = threadSearch.trim().toLocaleLowerCase(locale);
    const matches = (values: Array<string | undefined>) => !query || values.some((value) => value?.toLocaleLowerCase(locale).includes(query));
    const projects = workspaceItems.flatMap((workspace) => {
      const projectThreads = threads.filter((thread) => threadBelongsToWorkspace(thread, workspace));
      const projectMatches = matches([workspace.workspaceName, workspace.deviceName, t(workspaceStatusLabel(workspace))]);
      const visibleThreads = projectMatches ? projectThreads : projectThreads.filter((thread) => matches([thread.title]));
      return projectMatches || visibleThreads.length ? [{ key: workspaceKey(workspace), workspace, threads: visibleThreads, threadCount: projectThreads.length }] : [];
    });
    const knownKeys = new Set(workspaceItems.map(workspaceKey));
    const generalThreads = threads.filter((thread) => !thread.workspaceKey && matches([thread.title]));
    const unavailableThreads = threads.filter((thread) => thread.workspaceKey && !knownKeys.has(thread.workspaceKey) && matches([thread.title, thread.workspaceKey]));
    return { projects, generalThreads, unavailableThreads };
  }, [locale, t, threadSearch, threads, workspaceItems]);
  const activeProject = activeThreadWorkspaceKey
    ? workspaceItems.find((workspace) => workspaceKey(workspace) === activeThreadWorkspaceKey)
    : selectedWorkspace;
  const headerProject = activeProject || selectedWorkspace;
  const mobileProjectSubtitle = headerProject
    ? `${headerProject.workspaceName} · ${headerProject.deviceName} · ${t(workspaceStatusLabel(headerProject))}`
    : t("No project");
  const contextModeLabel = contextMode === "off" ? t("Off") : contextMode === "aggressive" ? t("Aggressive") : t("Smart");
  const mobileContextSummary = `${selectedWorkspace?.workspaceName || t("Auto")} · ${mode === "ask" ? "Ask" : mode === "plan" ? "Plan" : "Agent"} · ${labelForModel(selectedModel)} · ${t("Context")}: ${contextModeLabel}`;

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
          const initialWorkspaceKey = available[0].workspaceKey;
          if (initialWorkspaceKey) {
            setExpandedProjects((current) => new Set(current).add(initialWorkspaceKey));
          }
          setMessages([]);
          setHistoryLoading(true);
          setActiveThreadId(available[0].id);
        }
      })
      .catch(() => {
        if (!cancelled) setNotice({ kind: "error", text: t("Could not load conversations") });
      })
      .finally(() => {
        if (!cancelled) setThreadActionLoading(false);
      });
    return () => { cancelled = true; };
  }, [router, t]);

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
        if (!(error instanceof DOMException && error.name === "AbortError")) setNotice({ kind: "error", text: t("Could not load chat history") });
      })
      .finally(() => {
        if (!controller.signal.aborted) setHistoryLoading(false);
      });
    return () => controller.abort();
  }, [activeThreadId, router, t]);

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

  async function refreshModelCatalog(preserveSelection = true) {
    try {
      const response = await fetch("/api/v1/dashboard/models", { credentials: "include" });
      if (!response.ok) throw new Error(String(response.status));
      const data = (await response.json()) as { models?: string[]; model_options?: ChatModelOption[]; default_model?: string };
      const available = Array.isArray(data.models) && data.models.length ? data.models : ["auto"];
      const options = Array.isArray(data.model_options) && data.model_options.length
        ? data.model_options.filter((option) => option && typeof option.id === "string" && available.includes(option.id))
        : available.map((id) => ({ id, label: modelLabel(id), provider: "CodeLocal" } satisfies ChatModelOption));
      setModels(available);
      setModelOptions(options);
      setSelectedModel((current) => preserveSelection && available.includes(current)
        ? current
        : data.default_model && available.includes(data.default_model) ? data.default_model : available[0]);
    } catch {
      setModels(["auto"]);
      setModelOptions([{ id: "auto", label: "Auto", provider: "CodeLocal" }]);
      setSelectedModel("auto");
    }
  }

  useEffect(() => {
    let cancelled = false;
    queueMicrotask(() => {
      if (!cancelled) void refreshModelCatalog(false);
    });
    return () => { cancelled = true; };
  }, []);

  useEffect(() => {
    if (!followLatestRef.current) return;
    endRef.current?.scrollIntoView({ behavior: "auto", block: "end" });
  }, [messages, loading]);

  function updateScrollFollow() {
    const container = messagesRef.current;
    if (!container) return;
    const nearBottom = container.scrollHeight - container.scrollTop - container.clientHeight < 96;
    followLatestRef.current = nearBottom;
    setShowJumpLatest(!nearBottom);
  }

  function jumpToLatest() {
    followLatestRef.current = true;
    setShowJumpLatest(false);
    endRef.current?.scrollIntoView({ behavior: "auto", block: "end" });
  }

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
      throw new Error("Session expired");
    }
    const prepared = (await presign.json().catch(() => ({}))) as Partial<MediaPrepareResponse> & { error?: string; message?: string };
    if (!presign.ok || !prepared.url || !prepared.imageRef) {
      // Task 4: surface the backend media state directly. media_not_configured
      // means S3/Skill storage is off (multipart fallback still works); other
      // failures keep the explicit backend message for retry guidance.
      if (prepared.error === "media_not_configured") throw new Error("Image upload storage (S3) is not configured; the image can still be sent directly and will only be stored compactly in history");
      throw new Error(prepared.message || prepared.error || "Could not prepare image upload");
    }

    if (prepared.upload?.required) {
      if (!prepared.upload.url) throw new Error("Image upload URL is missing");
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
        if (!fallback.ok) throw new Error(fallbackData.message || "Could not upload the image to CodeLocal");
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
      setNotice({ kind: "error", text: t("Only image files are supported") });
      return;
    }
    if (file.size > 8 * 1024 * 1024) {
      setNotice({ kind: "error", text: t("Images can be up to 8 MB") });
      return;
    }

    if (image?.previewUrl.startsWith("blob:")) URL.revokeObjectURL(image.previewUrl);
    setImage({ previewUrl: URL.createObjectURL(file), file });
    setNotice(null);
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
    if (event.key !== "Enter" || event.shiftKey || event.nativeEvent.isComposing) return;
    if (window.matchMedia("(pointer: coarse)").matches || window.innerWidth <= 820) return;
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

  async function createThreadRecord(options: { workspaceKey?: string; model?: string } = {}) {
    const workspaceKey = options.workspaceKey ?? (selectedWorkspaceKey === "auto" ? "" : selectedWorkspaceKey);
    const response = await fetch("/api/v1/dashboard/chat/threads", {
      method: "POST",
      credentials: "include",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(threadCreationPayload(options.model ?? selectedModel, workspaceKey)),
    });
    if (response.status === 401) {
      router.replace("/login");
      throw new Error("Session expired");
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
    setNotice(null);
    try {
      const thread = await createThreadRecord({ workspaceKey: "" });
      setThreads((current) => [thread, ...current]);
      setSelectedWorkspaceKey("auto");
      setHistoryLoading(true);
      setActiveThreadId(thread.id);
      setMessages([]);
      setInput("");
      discardImage();
      window.requestAnimationFrame(() => composerRef.current?.focus());
    } catch {
      setNotice({ kind: "error", text: t("Could not create a new conversation") });
    } finally {
      setThreadActionLoading(false);
    }
  }

  async function newProjectThread(projectKey: string) {
    if (loading || threadActionLoading) return;
    setThreadActionLoading(true);
    setNotice(null);
    try {
      const thread = await createThreadRecord({ workspaceKey: projectKey });
      setThreads((current) => [thread, ...current]);
      setExpandedProjects((current) => new Set(current).add(projectKey));
      setSelectedWorkspaceKey(projectKey);
      setHistoryLoading(true);
      setActiveThreadId(thread.id);
      setMessages([]);
      setInput("");
      discardImage();
      setThreadDrawerOpen(false);
      window.requestAnimationFrame(() => composerRef.current?.focus());
    } catch {
      setNotice({ kind: "error", text: t("Could not create a task in this project") });
    } finally {
      setThreadActionLoading(false);
    }
  }

  async function renameThread(thread: ChatThread) {
    if (loading || threadActionLoading) return;
    const title = window.prompt(t("Conversation name"), thread.title)?.trim();
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
      setNotice({ kind: "error", text: t("Could not rename the conversation") });
    } finally {
      setThreadActionLoading(false);
    }
  }

  async function deleteThread(thread: ChatThread) {
    if (loading || threadActionLoading || !window.confirm(t("Delete “{title}”?", { title: thread.title }))) return;
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
      setNotice({ kind: "error", text: t("Could not delete the conversation") });
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
        setNotice({ kind: "error", text: t("Could not create a conversation to send the message") });
        return;
      }
    }

    const pendingImage = image;
    let sendImage: ChatImageMeta | undefined;
    let useMultipartFallback = false;
    if (pendingImage) {
      setImageUploading(true);
      setNotice({ kind: "status", text: t("Uploading image…") });
      try {
        sendImage = await uploadImage(pendingImage.file);
      } catch {
        useMultipartFallback = true;
        setNotice({ kind: "status", text: t("Sending image directly…") });
      } finally {
        setImageUploading(false);
      }
    }

    const userMessage: ChatMsg = { role: "user", content: text || t("Analyze this image"), image: pendingImage?.previewUrl };
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
      setNotice({ kind: "status", text: t(streamedContent.trim() || streamedToolCalls.length ? "Stopped responding; received content was preserved." : "Stopped before a response was received.") });
    };
    try {
      const requestId = createChatRequestId();
      const history = messages.slice(-50).map((message) => ({
      role: message.role,
      content: message.content,
      image: message.role === "user" ? message.image : undefined,
    }));
      const payload = {
        requestId,
        threadId: requestThreadId,
        message: text || t("Analyze this image"),
        history,
        model: selectedModel,
        mode,
        contextMode,
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
          setNotice({ kind: "status", text: t("Connection unstable · reconnecting automatically ({attempt}/{total})…", { attempt: attempt + 1, total: chatTransportRetryDelays.length }) });
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
            throw new Error("Session expired");
          }
          if (response.status === 429) {
            const data = (await response.json().catch(() => ({}))) as { retry_after?: number };
            throw new Error(`HTTP 429 RATE_LIMIT:${data.retry_after || 60}`);
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
            setNotice(null);
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

              if (eventName === "error") throw new Error(data.error || "The model returned a stream error");
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
          setNotice(null);
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
      const failure = friendlyChatFailure(message, t, translateMessage);
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
    setNotice({ kind: "status", text: t("Stopping response…") });
    controller.abort("user_stop");
  }

  async function clear(thread?: ChatThread) {
    const target = thread ?? activeThread;
    if (!target || !window.confirm(t("Delete all content in “{title}”?", { title: target.title }))) return;
    if (target.id === activeThreadId) {
      setMessages([]);
      setNotice(null);
    }
    try {
      const response = await fetch(`/api/v1/dashboard/chat/history?threadId=${encodeURIComponent(target.id)}`, { method: "DELETE", credentials: "include" });
      if (!response.ok) throw new Error(String(response.status));
    } catch {
      setNotice({ kind: "error", text: t("Could not delete chat history on the server") });
    }
  }

  function toggleProject(key: string) {
    setExpandedProjects((current) => toggleExpandedProject(current, key));
  }

  function selectThread(thread: ChatThread) {
    setThreadDrawerOpen(false);
    const targetWorkspaceKey = thread.workspaceKey;
    if (targetWorkspaceKey) setExpandedProjects((current) => new Set(current).add(targetWorkspaceKey));
    if (thread.id === activeThreadId) return;
    setMessages([]);
    setHistoryLoading(true);
    setActiveThreadId(thread.id);
  }

  function closeThreadMenu(event: React.MouseEvent<HTMLButtonElement>) {
    const menu = event.currentTarget.closest("details");
    if (menu instanceof HTMLDetailsElement) menu.open = false;
  }

  function ThreadRow({ thread }: { thread: ChatThread }) {
    return (
      <div className={`${styles.threadItem} ${treeStyles.threadItem} ${thread.id === activeThreadId ? styles.threadItemActive : ""}`}>
        <button className={`${styles.threadSelect} ${treeStyles.threadSelect}`} type="button" onClick={() => selectThread(thread)} disabled={loading || historyLoading} title={thread.title}>
          <AppIcon name="chat" size={15} />
          <span>{thread.title}</span>
        </button>
        <details className={treeStyles.threadOverflow}>
          <summary aria-label={t("Options for {title}", { title: thread.title })}><span aria-hidden="true">•••</span></summary>
          <div>
            <button type="button" onClick={(event) => { closeThreadMenu(event); void renameThread(thread); }}>{t("Rename")}</button>
            <button type="button" onClick={(event) => { closeThreadMenu(event); void clear(thread); }}>{t("Clear content")}</button>
            <button type="button" className={treeStyles.threadDeleteAction} onClick={(event) => { closeThreadMenu(event); void deleteThread(thread); }}>{t("Delete task")}</button>
          </div>
        </details>
      </div>
    );
  }

  return (
    <section className={styles.chatWorkspace} aria-label={t("CodeLocal chat workspace")}>
      <button className={`${styles.threadDrawerBackdrop} ${threadDrawerOpen ? styles.threadDrawerBackdropOpen : ""}`} type="button" onClick={() => { setThreadDrawerOpen(false); mobileDrawerTriggerRef.current?.focus(); }} aria-label={t("Close menu")} />
      <aside className={`${styles.threadSidebar} ${treeStyles.threadSidebar} ${threadDrawerOpen ? styles.threadSidebarOpen : ""}`} aria-label={t("Tasks and CodeLocal navigation")} aria-modal={threadDrawerOpen || undefined} role={threadDrawerOpen ? "dialog" : undefined}>
        <ChatSidebarBrand closeRef={threadDrawerCloseRef} onClose={() => { setThreadDrawerOpen(false); mobileDrawerTriggerRef.current?.focus(); }} />
        <button className={`${styles.newThreadButton} ${treeStyles.newThreadButton}`} type="button" onClick={() => { setThreadDrawerOpen(false); void newThread(); }} disabled={loading || threadActionLoading}>
          <AppIcon name="plus" size={17} />
          {t("New task")}
        </button>
        <label className={`${styles.threadSearch} ${treeStyles.threadSearch}`}>
          <AppIcon name="search" size={16} />
          <input value={threadSearch} onChange={(event) => setThreadSearch(event.target.value)} placeholder={t("Search projects and tasks")} aria-label={t("Search projects and tasks")} type="search" />
        </label>
        <div className={`${styles.threadList} ${treeStyles.threadList}`} aria-label={t("Projects and tasks")}>
          {threadTree.projects.length === 0 && threadTree.generalThreads.length === 0 && threadTree.unavailableThreads.length === 0 ? (
            <p className={styles.threadEmpty}>{t("No projects or tasks found.")}</p>
          ) : null}
          {threadTree.projects.map(({ key, workspace, threads: projectThreads, threadCount }) => {
            const open = projectTreeOpen(key, threadSearch, expandedProjects);
            const active = activeThreadWorkspaceKey === key || (!activeThreadWorkspaceKey && selectedWorkspaceKey === key);
            return (
              <section className={`${treeStyles.projectGroup} ${active ? treeStyles.projectGroupActive : ""}`} key={key}>
                <div className={treeStyles.projectHead}>
                  <button className={treeStyles.projectToggle} type="button" onClick={() => toggleProject(key)} aria-expanded={open}>
                    <span className={treeStyles.projectFolder} data-state={workspaceState(workspace)} aria-hidden="true"><AppIcon name="folder" size={17} /><i /></span>
                    <span className={treeStyles.projectCopy}>
                      <strong title={workspace.workspaceName}>{workspace.workspaceName}</strong>
                      <small title={`${workspace.deviceName} · ${t(workspaceStatusLabel(workspace))}`}>{workspace.deviceName} · {t(workspaceStatusLabel(workspace))}</small>
                    </span>
                    <span className={treeStyles.projectThreadCount} aria-label={t("{count} tasks", { count: threadCount })}>{threadCount}</span>
                    <AppIcon className={treeStyles.projectChevron} name="chevron-right" size={14} />
                  </button>
                  <button
                    className={treeStyles.projectNewThread}
                    type="button"
                    aria-label={t("Create task in {name}", { name: workspace.workspaceName })}
                    title={t("Create task in {name}", { name: workspace.workspaceName })}
                    disabled={loading || threadActionLoading}
                    onClick={() => void newProjectThread(key)}
                  >
                    <AppIcon name="plus" size={16} />
                  </button>
                </div>
                {open ? (
                  <div className={treeStyles.projectThreads}>
                    {projectThreads.length ? projectThreads.map((thread) => <ThreadRow thread={thread} key={thread.id} />) : (
                      <button className={treeStyles.emptyProjectAction} type="button" onClick={() => void newProjectThread(key)} disabled={loading || threadActionLoading}>
                        <AppIcon name="plus" size={14} /> {t("Create first task")}
                      </button>
                    )}
                  </div>
                ) : null}
              </section>
            );
          })}
          {threadTree.generalThreads.length ? (
            <section className={`${treeStyles.projectGroup} ${!activeThreadWorkspaceKey ? treeStyles.projectGroupActive : ""}`}>
              <div className={treeStyles.generalGroupHead}><AppIcon name="folder" size={15} /><span><strong>{t("General")}</strong><small>{t("No project")}</small></span><b>{threadTree.generalThreads.length}</b></div>
              <div className={treeStyles.projectThreads}>{threadTree.generalThreads.map((thread) => <ThreadRow thread={thread} key={thread.id} />)}</div>
            </section>
          ) : null}
          {threadTree.unavailableThreads.length ? (
            <section className={treeStyles.projectGroup}>
              <div className={treeStyles.generalGroupHead}><AppIcon name="folder" size={15} /><span><strong>{t("Unavailable projects")}</strong><small>{t("The workspace is no longer in the list")}</small></span><b>{threadTree.unavailableThreads.length}</b></div>
              <div className={treeStyles.projectThreads}>{threadTree.unavailableThreads.map((thread) => <ThreadRow thread={thread} key={thread.id} />)}</div>
            </section>
          ) : null}
        </div>
        <ChatSidebarFooter onNavigate={() => setThreadDrawerOpen(false)} />
      </aside>

      <section ref={chatFrameRef} className={styles.chatShell} aria-label={t("Chat with CodeLocal")} aria-hidden={contextSheetOpen || threadDrawerOpen || undefined}>
        <ChatTopBar
          drawerOpen={threadDrawerOpen}
          title={activeThread?.title || t("New task")}
          subtitle={mobileProjectSubtitle}
          disabled={loading || threadActionLoading}
          menuRef={mobileDrawerTriggerRef}
          onOpenMenu={() => setThreadDrawerOpen(true)}
          onNewThread={() => void newThread()}
        />
        <div className={styles.chatHead}>
          <div className={headerStyles.threadHeadContext}>
            <button ref={threadDrawerTriggerRef} className={styles.threadDrawerToggle} type="button" onClick={() => setThreadDrawerOpen(true)} aria-label={t("Open CodeLocal menu")} aria-expanded={threadDrawerOpen}>
              <AppIcon name="menu" size={19} />
            </button>
            <div>
              <h1 title={activeThread?.title || t("New task")}>{activeThread?.title || t("New task")}</h1>
              <span title={mobileProjectSubtitle}>{headerProject?.workspaceName || t("No project")}</span>
            </div>
          </div>
          <div className={styles.chatActions}>
            <label className={styles.contextModePicker} title={t("Context optimization")}>
              <span>{t("Context")}</span>
              <select value={contextMode} onChange={(event) => setContextMode(event.target.value as ChatContextMode)} aria-label={t("Context optimization")} disabled={loading}>
                <option value="smart">{t("Smart")}</option>
                <option value="off">{t("Off")}</option>
                <option value="aggressive">{t("Aggressive")}</option>
              </select>
            </label>
            <div className={styles.modelPickerShell} ref={modelPickerRef}>
              <button
                className={`${styles.projectPicker} ${styles.modelPicker}`}
                type="button"
                aria-label={t("Select model")}
                aria-haspopup="listbox"
                aria-expanded={modelPickerOpen}
                onClick={() => setModelPickerOpen((open) => !open)}
              >
                <span className={styles.modelPickerName}>{labelForModel(selectedModel)}</span>
                {activeModels.length > 0 ? <span className={styles.modelPickerCount}>{activeModels.length}</span> : null}
                <span className={styles.modelPickerChevron} aria-hidden="true">⌄</span>
              </button>
              {modelPickerOpen ? (
                <div className={styles.modelPickerMenu} role="dialog" aria-label={t("Find and select a model")}>
                  <label className={styles.modelSearch}>
                    <AppIcon name="search" size={15} />
                    <input
                      autoFocus
                      value={modelSearch}
                      onChange={(event) => setModelSearch(event.target.value)}
                      placeholder={t("Search GPT, Claude, Gemini...")}
                      aria-label={t("Search models")}
                    />
                  </label>
                  <div className={styles.modelPickerSummary}>{t("{count} active models", { count: activeModels.length })}</div>
                  <div className={styles.modelOptionList} role="listbox" aria-label={t("Active models")}>
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
                        <small>{t("Automatically choose the default model")}</small>
                      </button>
                    ) : null}
                    {visibleModels.map((model) => (
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
                        <span>{labelForModel(model)}</span>
                        <small>{modelOptionMap.get(model)?.custom ? `${providerForModel(model)} · ${model}` : providerForModel(model)}</small>
                      </button>
                    ))}
                    {visibleModels.length === 0 ? <p className={styles.modelEmpty}>{t("No active models found.")}</p> : null}
                  </div>
                  <button
                    className={styles.manageAIButton}
                    type="button"
                    onClick={() => {
                      setModelPickerOpen(false);
                      setModelSearch("");
                      setProviderManagerOpen(true);
                    }}
                  >
                    <AppIcon name="plus" size={15} />
                    <span><strong>{t("Add or manage AI providers")}</strong><small>{t("Use your own API keys and models")}</small></span>
                    <AppIcon name="chevron-right" size={14} />
                  </button>
                </div>
              ) : null}
            </div>
          </div>
        </div>

        <div ref={messagesRef} className={styles.chatMessages} onPaste={onPaste} onScroll={updateScrollFollow}>
          {historyLoading ? <div className={styles.historyLoading}>{t("Loading conversation…")}</div> : messages.length === 0 ? (
            <div className={styles.emptyState}>
              <span className={styles.emptyOrb} aria-hidden="true"><AppIcon name="codelocal" size={27} /></span>
              <strong>{t("Start a task")}</strong>
              <p>{t("Describe what needs to be done. CodeLocal will read the project, execute the task, and verify the result.")}</p>
              <div className={styles.suggestions}>
                {suggestions.map((suggestion) => <button key={suggestion} type="button" onClick={() => setInput(t(suggestion))}>{t(suggestion)}</button>)}
              </div>
            </div>
          ) : messages.map((message, index) => (
            <div key={`${message.role}-${index}`} className={`${styles.msgBlock} ${message.role === "user" ? styles.userBlock : styles.assistantBlock}`}>
              <div className={styles.messageBody}>
                {message.skills?.length ? (
                  <div className={skillStyles.list} aria-label={t("Skills used by CodeLocal")}>
                    {message.skills.map((skill) => (
                      <span className={skillStyles.pill} key={`${skill.id}@${skill.version}`} title={t("CodeLocal selected {name}@{version} for this task", { name: skill.name, version: skill.version })}>
                        <AppIcon name="skill" size={13} />
                        {skill.name}
                      </span>
                    ))}
                  </div>
                ) : null}
                {message.tool_calls?.length ? (
                  <ChatActionSummary actions={message.tool_calls} busy={loading} onApprove={() => submitQuickMessage("full access")} />
                ) : null}
                {message.image ? <img src={message.image} alt={t("Sent image")} className={styles.msgImage} /> : null}
                {message.content ? (
                  <div className={`${styles.msg} ${message.role === "user" ? styles.msgUser : styles.msgAssistant} ${loading && message.role === "assistant" && index === messages.length - 1 ? styles.msgStreaming : ""}`}>
                    {message.role === "assistant" ? <ChatRichMessage content={message.content} /> : message.content}
                    {loading && message.role === "assistant" && index === messages.length - 1 ? (
                      <span className={styles.streamingDots} aria-label={t("CodeLocal is still responding")}><i /><i /><i /></span>
                    ) : null}
                  </div>
                ) : loading && index === messages.length - 1 ? <div className={styles.thinking} aria-label={t("CodeLocal is responding")}><i /><i /><i /></div> : null}
              </div>
            </div>
          ))}
          <div ref={endRef} />
        </div>

        {showJumpLatest ? <button className={mobileStyles.jumpLatest} type="button" onClick={jumpToLatest}><AppIcon name="chevron-down" size={16} /> {t("Latest")}</button> : null}

        <form ref={formRef} className={styles.chatForm} onSubmit={send} onPaste={onPaste}>
          <input ref={fileRef} type="file" accept="image/*" onChange={onFile} className={styles.fileInput} />
          {image ? <div className={styles.imagePreview}><img src={image.previewUrl} alt={t("Image ready to send")} /><button type="button" onClick={discardImage} aria-label={t("Remove image")}><AppIcon name="close" size={14} /></button></div> : null}
          <textarea ref={composerRef} value={input} onChange={(event) => setInput(event.target.value)} onKeyDown={onComposerKeyDown} onPaste={onPaste} placeholder={t("Give CodeLocal a task…")} aria-label={t("Describe the task")} enterKeyHint="enter" rows={1} />
          <div className={styles.composerToolbar}>
            <div className={styles.composerOptions}>
              <button ref={contextTriggerRef} type="button" className={mobileStyles.contextTrigger} onClick={() => setContextSheetOpen(true)} aria-label={t("Add image or adjust context")} aria-haspopup="dialog" aria-expanded={contextSheetOpen} disabled={loading || imageUploading}>
                <AppIcon name="plus" size={20} />
              </button>
              <span className={mobileStyles.contextSummary} title={mobileContextSummary}>{mobileContextSummary}</span>
              <button type="button" className={styles.attachBtn} onClick={() => fileRef.current?.click()} aria-label={t("Attach image")} disabled={loading || imageUploading}>
                <AppIcon name="paperclip" size={17} />
              </button>
              <label className={styles.modePicker} title={mode === "agent" ? t("Agent can edit files and run commands") : t("{mode} uses read-only tools", { mode: mode === "ask" ? "Ask" : "Plan" })}>
                <select value={mode} onChange={(event) => setMode(event.target.value as ChatMode)} aria-label={t("Choose chat mode")} disabled={loading}>
                  <option value="ask">Ask</option>
                  <option value="plan">Plan</option>
                  <option value="agent">Agent</option>
                </select>
              </label>
              <label className={`${styles.goalField} ${goal ? styles.goalFieldActive : ""}`}>
                <AppIcon name="target" size={13} />
                <input value={goal} onChange={(event) => setGoal(event.target.value)} placeholder={t("Add goal")} aria-label={t("Goal")} maxLength={240} disabled={loading} />
                {goal ? <button className={styles.goalClear} type="button" onClick={() => setGoal("")} aria-label={t("Clear goal")} disabled={loading}><AppIcon name="close" size={11} /></button> : null}
              </label>
            </div>
            <button className={`${styles.sendBtn} ${loading ? styles.stopBtn : ""}`} type={loading ? "button" : "submit"} onClick={loading ? stopStream : undefined} disabled={loading ? false : imageUploading || historyLoading || threadActionLoading || (!input.trim() && !image)} aria-label={loading ? t("Stop response") : t("Send")} title={loading ? t("Stop response") : t("Send")}>
              <AppIcon name={loading ? "stop" : "send"} size={loading ? 16 : 18} />
            </button>
          </div>
        </form>
        {notice ? <div className={styles.chatHint} role={notice.kind === "error" ? "alert" : "status"}>{notice.text}</div> : null}
      </section>
      <ChatContextSheet
        open={contextSheetOpen}
        triggerRef={contextTriggerRef}
        imageDisabled={loading || imageUploading}
        workspaceItems={workspaceItems}
        selectedWorkspaceKey={selectedWorkspaceKey}
        mode={mode}
        models={models}
        selectedModel={selectedModel}
        contextMode={contextMode}
        goal={goal}
        modelLabel={labelForModel}
        workspaceKey={workspaceKey}
        workspaceStatusLabel={(workspace) => t(workspaceStatusLabel(workspace))}
        onClose={() => setContextSheetOpen(false)}
        onAttach={() => { setContextSheetOpen(false); window.requestAnimationFrame(() => fileRef.current?.click()); }}
        onWorkspaceChange={setSelectedWorkspaceKey}
        onModeChange={setMode}
        onModelChange={setSelectedModel}
        onContextModeChange={setContextMode}
        onManageProviders={() => { setContextSheetOpen(false); setProviderManagerOpen(true); }}
        onGoalChange={setGoal}
      />
      <ChatProviderManager
        open={providerManagerOpen}
        onClose={() => setProviderManagerOpen(false)}
        onChanged={() => void refreshModelCatalog(true)}
      />
    </section>
  );
}
