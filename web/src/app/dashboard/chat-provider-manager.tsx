"use client";

import { useEffect, useRef, useState, type FormEvent } from "react";
import { AppIcon } from "./app-icon";
import { useTranslations } from "@/lib/i18n/provider";
import styles from "./chat-provider-manager.module.css";

type AIProvider = {
  id: string;
  name: string;
  baseUrl: string;
  protocol: string;
  models: string[];
  enabled: boolean;
  hasCredential: boolean;
  lastTestStatus?: string;
  lastTestMessage?: string;
  lastTestAt?: number;
};

type ProviderPreset = "openai" | "openrouter" | "custom";

type ChatProviderManagerProps = {
  open: boolean;
  onClose: () => void;
  onChanged: () => void;
};

const presets: Record<Exclude<ProviderPreset, "custom">, { name: string; baseUrl: string }> = {
  openai: { name: "OpenAI", baseUrl: "https://api.openai.com/v1" },
  openrouter: { name: "OpenRouter", baseUrl: "https://openrouter.ai/api/v1" },
};

function parseManualModels(value: string) {
  return value.split(/[\n,]/).map((item) => item.trim()).filter(Boolean).slice(0, 200);
}

export function ChatProviderManager({ open, onClose, onChanged }: ChatProviderManagerProps) {
  const { t } = useTranslations();
  const closeRef = useRef<HTMLButtonElement>(null);
  const [providers, setProviders] = useState<AIProvider[]>([]);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [testingId, setTestingId] = useState("");
  const [deletingId, setDeletingId] = useState("");
  const [preset, setPreset] = useState<ProviderPreset>("openai");
  const [name, setName] = useState(presets.openai.name);
  const [baseUrl, setBaseUrl] = useState(presets.openai.baseUrl);
  const [apiKey, setApiKey] = useState("");
  const [manualModels, setManualModels] = useState("");
  const [notice, setNotice] = useState<{ kind: "ok" | "error"; text: string } | null>(null);
  const [rotateId, setRotateId] = useState("");
  const [rotateKey, setRotateKey] = useState("");

  async function loadProviders() {
    setLoading(true);
    try {
      const response = await fetch("/api/v1/dashboard/ai/providers", { credentials: "include" });
      if (!response.ok) throw new Error(String(response.status));
      const data = (await response.json()) as { providers?: AIProvider[] };
      setProviders(Array.isArray(data.providers) ? data.providers : []);
    } catch {
      setNotice({ kind: "error", text: t("Could not load AI providers.") });
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    if (!open) return;
    const previousOverflow = document.body.style.overflow;
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    const loadFrame = window.requestAnimationFrame(() => {
      void loadProviders();
      closeRef.current?.focus();
    });
    document.body.style.overflow = "hidden";
    document.addEventListener("keydown", closeOnEscape);
    return () => {
      window.cancelAnimationFrame(loadFrame);
      document.body.style.overflow = previousOverflow;
      document.removeEventListener("keydown", closeOnEscape);
    };
  }, [open]); // eslint-disable-line react-hooks/exhaustive-deps

  if (!open) return null;

  function choosePreset(next: ProviderPreset) {
    setPreset(next);
    setNotice(null);
    if (next === "custom") {
      setName("");
      setBaseUrl("");
      return;
    }
    setName(presets[next].name);
    setBaseUrl(presets[next].baseUrl);
  }

  async function testProvider(id: string, quiet = false) {
    setTestingId(id);
    try {
      const response = await fetch(`/api/v1/dashboard/ai/providers/${encodeURIComponent(id)}/test`, { method: "POST", credentials: "include" });
      const data = (await response.json().catch(() => ({}))) as { ok?: boolean; error?: string; models?: string[] };
      if (!response.ok || !data.ok) throw new Error(data.error || "test_failed");
      if (!quiet) setNotice({ kind: "ok", text: t("Connected. Models are ready to use in Chat.") });
      await loadProviders();
      onChanged();
      return true;
    } catch {
      if (!quiet) setNotice({ kind: "error", text: t("Provider saved, but CodeLocal could not verify the connection. Check the URL/key or add models manually.") });
      return false;
    } finally {
      setTestingId("");
    }
  }

  async function submitProvider(event: FormEvent) {
    event.preventDefault();
    if (!name.trim() || !baseUrl.trim() || !apiKey.trim()) return;
    setSaving(true);
    setNotice(null);
    try {
      const response = await fetch("/api/v1/dashboard/ai/providers", {
        method: "POST",
        credentials: "include",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          name: name.trim(),
          baseUrl: baseUrl.trim(),
          apiKey: apiKey.trim(),
          protocol: "openai_compatible",
          models: parseManualModels(manualModels),
        }),
      });
      const data = (await response.json().catch(() => ({}))) as { provider?: AIProvider; error?: string };
      if (!response.ok || !data.provider) throw new Error(data.error || "save_failed");
      setApiKey("");
      const verified = await testProvider(data.provider.id, true);
      setNotice({
        kind: verified ? "ok" : "error",
        text: verified ? t("AI provider added. Its models are available in Chat now.") : t("Provider saved, but CodeLocal could not verify the connection. Check the URL/key or add models manually."),
      });
      await loadProviders();
      onChanged();
    } catch (error) {
      const text = error instanceof Error && error.message && error.message !== "save_failed" ? error.message : t("Could not save this AI provider.");
      setNotice({ kind: "error", text });
    } finally {
      setSaving(false);
    }
  }

  async function deleteProvider(provider: AIProvider) {
    if (!window.confirm(t("Remove {name}? The encrypted key and provider settings will be deleted.", { name: provider.name }))) return;
    setDeletingId(provider.id);
    try {
      const response = await fetch(`/api/v1/dashboard/ai/providers/${encodeURIComponent(provider.id)}`, { method: "DELETE", credentials: "include" });
      if (!response.ok && response.status !== 204) throw new Error(String(response.status));
      setProviders((current) => current.filter((item) => item.id !== provider.id));
      setNotice({ kind: "ok", text: t("AI provider removed.") });
      onChanged();
    } catch {
      setNotice({ kind: "error", text: t("Could not remove this AI provider.") });
    } finally {
      setDeletingId("");
    }
  }

  async function rotateProviderKey(provider: AIProvider) {
    if (!rotateKey.trim()) return;
    setSaving(true);
    try {
      const response = await fetch(`/api/v1/dashboard/ai/providers/${encodeURIComponent(provider.id)}`, {
        method: "PATCH",
        credentials: "include",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ apiKey: rotateKey.trim() }),
      });
      if (!response.ok) throw new Error(String(response.status));
      setRotateKey("");
      setRotateId("");
      const verified = await testProvider(provider.id, true);
      setNotice({ kind: verified ? "ok" : "error", text: verified ? t("API key replaced and verified.") : t("API key replaced, but the connection test failed.") });
      await loadProviders();
      onChanged();
    } catch {
      setNotice({ kind: "error", text: t("Could not replace the API key.") });
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className={styles.layer}>
      <button className={styles.backdrop} type="button" onClick={onClose} aria-label={t("Close AI providers")} />
      <section className={styles.dialog} role="dialog" aria-modal="true" aria-labelledby="ai-provider-title">
        <header className={styles.head}>
          <div>
            <span className={styles.eyebrow}>{t("Your AI")}</span>
            <h2 id="ai-provider-title">{t("AI providers")}</h2>
            <p>{t("Add a key once. CodeLocal keeps it encrypted and your models appear in Chat automatically.")}</p>
          </div>
          <button ref={closeRef} className={styles.close} type="button" onClick={onClose} aria-label={t("Close AI providers")}><AppIcon name="close" size={18} /></button>
        </header>

        <div className={styles.body}>
          {notice ? <div className={`${styles.notice} ${notice.kind === "error" ? styles.noticeError : ""}`} role={notice.kind === "error" ? "alert" : "status"}>{notice.text}</div> : null}

          <form className={styles.addCard} onSubmit={submitProvider}>
            <div className={styles.cardTitle}>
              <span><AppIcon name="plus" size={16} /></span>
              <div><strong>{t("Add AI provider")}</strong><small>{t("OpenAI-compatible providers work immediately.")}</small></div>
            </div>
            <div className={styles.presets}>
              {(["openai", "openrouter", "custom"] as const).map((value) => <button key={value} type="button" className={preset === value ? styles.presetActive : ""} onClick={() => choosePreset(value)}>{value === "openai" ? "OpenAI" : value === "openrouter" ? "OpenRouter" : t("Custom")}</button>)}
            </div>
            <div className={styles.grid}>
              <label><span>{t("Name")}</span><input value={name} onChange={(event) => setName(event.target.value)} placeholder="My AI" maxLength={80} required /></label>
              <label><span>{t("Base URL")}</span><input value={baseUrl} onChange={(event) => setBaseUrl(event.target.value)} placeholder="https://api.example.com/v1" inputMode="url" required /></label>
              <label className={styles.keyField}><span>{t("API key")}</span><input value={apiKey} onChange={(event) => setApiKey(event.target.value)} placeholder="••••••••••••••••" type="password" autoComplete="off" required /><small><AppIcon name="shield" size={12} /> {t("Encrypted before storage. Never returned to the browser.")}</small></label>
            </div>
            <details className={styles.advanced}>
              <summary>{t("Advanced · manual models")}</summary>
              <label><span>{t("Models (optional)")}</span><textarea value={manualModels} onChange={(event) => setManualModels(event.target.value)} placeholder="gpt-5.6-sol, model-name" rows={2} /><small>{t("Leave empty and CodeLocal will detect models automatically.")}</small></label>
            </details>
            <button className={styles.primary} type="submit" disabled={saving || !name.trim() || !baseUrl.trim() || !apiKey.trim()}>{saving ? t("Saving…") : t("Save & detect models")}</button>
          </form>

          <section className={styles.savedSection}>
            <div className={styles.sectionHead}><div><strong>{t("Saved providers")}</strong><small>{t("Available on every signed-in device")}</small></div><span>{providers.length}</span></div>
            {loading ? <div className={styles.empty}>{t("Loading…")}</div> : providers.length === 0 ? <div className={styles.empty}>{t("No custom AI providers yet.")}</div> : (
              <div className={styles.providerList}>
                {providers.map((provider) => (
                  <article className={styles.providerCard} key={provider.id}>
                    <div className={styles.providerMain}>
                      <span className={`${styles.statusDot} ${provider.lastTestStatus === "ok" ? styles.statusOK : provider.lastTestStatus === "error" ? styles.statusError : ""}`} />
                      <div><strong>{provider.name}</strong><small>{provider.baseUrl}</small></div>
                      <span className={styles.modelCount}>{t("{count} models", { count: provider.models?.length || 0 })}</span>
                    </div>
                    <div className={styles.providerMeta}><span><AppIcon name="shield" size={12} /> {t("Key encrypted")}</span>{provider.lastTestMessage ? <span>{provider.lastTestMessage}</span> : <span>{t("Not tested yet")}</span>}</div>
                    {rotateId === provider.id ? (
                      <div className={styles.rotateRow}>
                        <input type="password" value={rotateKey} onChange={(event) => setRotateKey(event.target.value)} placeholder={t("New API key")} autoComplete="off" autoFocus />
                        <button type="button" disabled={saving || !rotateKey.trim()} onClick={() => void rotateProviderKey(provider)}>{t("Replace")}</button>
                        <button type="button" onClick={() => { setRotateId(""); setRotateKey(""); }}>{t("Cancel")}</button>
                      </div>
                    ) : null}
                    <div className={styles.actions}>
                      <button type="button" disabled={testingId === provider.id} onClick={() => void testProvider(provider.id)}>{testingId === provider.id ? t("Testing…") : t("Test")}</button>
                      <button type="button" onClick={() => { setRotateId(provider.id); setRotateKey(""); }}>{t("Replace key")}</button>
                      <button type="button" className={styles.danger} disabled={deletingId === provider.id} onClick={() => void deleteProvider(provider)}>{t("Remove")}</button>
                    </div>
                  </article>
                ))}
              </div>
            )}
          </section>
        </div>
      </section>
    </div>
  );
}
