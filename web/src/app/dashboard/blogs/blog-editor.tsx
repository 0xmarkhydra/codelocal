"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { isAccountResource } from "@/lib/contracts/account";
import { isBlogPostResource, isBlogSeriesCollectionResource } from "@/lib/contracts/blog";
import { privateMediaVariantURL, uploadMediaAsset } from "@/lib/media-upload";
import { useDashboardResource } from "../use-dashboard-resource";
import dashboard from "../dashboard.module.css";
import styles from "./blog-editor.module.css";

type EditorImageBlock = {
  type: "image";
  assetId: string;
  width: number;
  height: number;
  alt: string;
  caption: string;
  variant: "large";
};

function recordBlock(block: unknown): Record<string, unknown> | undefined {
  return typeof block === "object" && block !== null && !Array.isArray(block)
    ? block as Record<string, unknown>
    : undefined;
}

function blockText(content: unknown[]) {
  return content.map((block) => {
    const item = recordBlock(block);
    if (!item || item.type === "image") return "";
    if (typeof item.text === "string") return item.text;
    if (Array.isArray(item.items)) return item.items.filter((value): value is string => typeof value === "string").join("\n");
    if (typeof item.code === "string") return item.code;
    return "";
  }).filter(Boolean).join("\n\n");
}

function imageBlocks(content: unknown[]): EditorImageBlock[] {
  return content.flatMap((block) => {
    const item = recordBlock(block);
    if (!item || item.type !== "image" || typeof item.assetId !== "string" || !item.assetId.startsWith("media_")) return [];
    return [{
      type: "image" as const,
      assetId: item.assetId,
      width: typeof item.width === "number" && item.width > 0 ? item.width : 1600,
      height: typeof item.height === "number" && item.height > 0 ? item.height : 900,
      alt: typeof item.alt === "string" ? item.alt : "",
      caption: typeof item.caption === "string" ? item.caption : "",
      variant: "large" as const,
    }];
  });
}

function hasStructuredBlocks(content: unknown[]) {
  return content.some((block) => {
    const item = recordBlock(block);
    return item && item.type !== "paragraph" && item.type !== "image";
  });
}

function contentFromText(value: string) {
  return value.trim()
    ? value.split(/\n\s*\n/).map((text) => ({ type: "paragraph", text: text.trim() })).filter((block) => block.text)
    : [];
}

function consumeImage(images: EditorImageBlock[], assetID: string) {
  const index = images.findIndex((image) => image.assetId === assetID);
  if (index < 0) return undefined;
  const [image] = images.splice(index, 1);
  return image;
}

function sourceImageWithEdits(source: Record<string, unknown>, image: EditorImageBlock) {
  return {
    ...source,
    assetId: image.assetId,
    width: image.width,
    height: image.height,
    alt: image.alt,
    caption: image.caption,
  };
}

function structuredContentWithImages(source: unknown[], images: EditorImageBlock[]) {
  const pending = [...images];
  const output: unknown[] = [];
  for (const block of source) {
    const item = recordBlock(block);
    if (!item || item.type !== "image" || typeof item.assetId !== "string") {
      output.push(block);
      continue;
    }
    const replacement = consumeImage(pending, item.assetId);
    if (replacement) output.push(sourceImageWithEdits(item, replacement));
  }
  output.push(...pending);
  return output;
}

function paragraphContentWithImages(source: unknown[], body: string, images: EditorImageBlock[]) {
  const paragraphs = contentFromText(body);
  const pendingImages = [...images];
  const output: unknown[] = [];
  let paragraphIndex = 0;

  for (const block of source) {
    const item = recordBlock(block);
    if (item?.type === "image" && typeof item.assetId === "string") {
      const replacement = consumeImage(pendingImages, item.assetId);
      if (replacement) output.push(sourceImageWithEdits(item, replacement));
      continue;
    }
    if (item?.type === "paragraph") {
      if (paragraphIndex < paragraphs.length) output.push(paragraphs[paragraphIndex++]);
      continue;
    }
    output.push(block);
  }

  output.push(...paragraphs.slice(paragraphIndex));
  output.push(...pendingImages);
  return output;
}

type SaveState = "idle" | "saving" | "saved" | "error";

export function BlogEditor({ postID }: { postID: string }) {
  const account = useDashboardResource("/api/v1/account", isAccountResource);
  const resource = useDashboardResource(`/api/v1/blog/posts/${encodeURIComponent(postID)}`, isBlogPostResource);
  const seriesResource = useDashboardResource("/api/v1/blog/series", isBlogSeriesCollectionResource);
  const [title, setTitle] = useState("");
  const [slug, setSlug] = useState("");
  const [excerpt, setExcerpt] = useState("");
  const [category, setCategory] = useState("");
  const [tags, setTags] = useState("");
  const [visibility, setVisibility] = useState("public");
  const [seriesID, setSeriesID] = useState("");
  const [seriesPart, setSeriesPart] = useState(0);
  const [body, setBody] = useState("");
  const [sourceContent, setSourceContent] = useState<unknown[]>([]);
  const [coverAssetID, setCoverAssetID] = useState("");
  const [images, setImages] = useState<EditorImageBlock[]>([]);
  const [hydratedID, setHydratedID] = useState("");
  const [dirty, setDirty] = useState(false);
  const [saveState, setSaveState] = useState<SaveState>("idle");
  const [mediaBusy, setMediaBusy] = useState<"cover" | "image" | "">("");
  const [actionError, setActionError] = useState("");
  const editVersion = useRef(0);
  const saveQueue = useRef<Promise<boolean>>(Promise.resolve(true));

  const post = resource.state.kind === "ready" ? resource.state.value.post : undefined;
  const series = seriesResource.state.kind === "ready" ? seriesResource.state.value.series.filter((item) => item.status !== "archived") : [];
  const structured = useMemo(() => hasStructuredBlocks(sourceContent), [sourceContent]);

  useEffect(() => {
    if (!post || hydratedID === post.id) return;
    // eslint-disable-next-line react-hooks/set-state-in-effect -- hydrate editable form state when the async post resource changes
    setTitle(post.title);
    setSlug(post.slug);
    setExcerpt(post.excerpt);
    setCategory(post.category ?? "");
    setTags(post.tags.join(", "));
    setVisibility(post.visibility);
    setSeriesID(post.seriesId ?? "");
    setSeriesPart(post.seriesPart ?? 0);
    setBody(blockText(post.content));
    setSourceContent(post.content);
    setCoverAssetID(post.coverAssetId ?? "");
    setImages(imageBlocks(post.content));
    setHydratedID(post.id);
    editVersion.current = 0;
    saveQueue.current = Promise.resolve(true);
    setDirty(false);
    setSaveState("idle");
  }, [hydratedID, post]);

  const payload = useMemo(() => ({
    title,
    slug,
    excerpt,
    category,
    tags: tags.split(",").map((tag) => tag.trim()).filter(Boolean),
    visibility,
    seriesId: seriesID,
    seriesPart: seriesID ? Math.max(1, seriesPart || 1) : 0,
    coverAssetId: coverAssetID,
    content: structured
      ? structuredContentWithImages(sourceContent, images)
      : paragraphContentWithImages(sourceContent, body, images),
  }), [body, category, coverAssetID, excerpt, images, seriesID, seriesPart, slug, sourceContent, structured, tags, title, visibility]);

  const save = useCallback(() => {
    if (!post || !dirty || account.state.kind !== "ready") return Promise.resolve(true);
    const snapshot = payload;
    const version = editVersion.current;
    const csrf = account.state.value.csrf;

    const run = async () => {
      setSaveState("saving");
      setActionError("");
      try {
        const response = await fetch(`/api/v1/blog/posts/${encodeURIComponent(post.id)}`, {
          method: "PATCH",
          credentials: "same-origin",
          headers: {
            Accept: "application/json",
            "Content-Type": "application/json",
            "X-CSRF-Token": csrf,
          },
          body: JSON.stringify(snapshot),
        });
        const responseBody: unknown = await response.json().catch(() => null);
        if (!response.ok || !isBlogPostResource(responseBody)) {
          if (editVersion.current === version) {
            setSaveState("error");
            setActionError(response.status === 409 ? "That URL slug or series part is already in use." : `Save failed (${response.status}).`);
          }
          return false;
        }
        if (editVersion.current === version) {
          setSourceContent(responseBody.post.content);
          setDirty(false);
          setSaveState("saved");
          return true;
        }
        setDirty(true);
        setSaveState("idle");
        return false;
      } catch (error) {
        if (editVersion.current === version) {
          setSaveState("error");
          setActionError(error instanceof Error ? error.message : "Save failed.");
        }
        return false;
      }
    };

    const queued = saveQueue.current.then(run, run);
    saveQueue.current = queued.then(() => true, () => false);
    return queued;
  }, [account.state, dirty, payload, post]);

  useEffect(() => {
    if (!dirty || hydratedID !== post?.id || mediaBusy) return;
    const timer = window.setTimeout(() => { void save(); }, 1200);
    return () => window.clearTimeout(timer);
  }, [dirty, hydratedID, mediaBusy, post?.id, save]);

  function markDirty() {
    editVersion.current += 1;
    setDirty(true);
    setSaveState("idle");
  }

  function change(setter: (value: string) => void, value: string) {
    setter(value);
    markDirty();
  }

  function changeSeries(value: string) {
    setSeriesID(value);
    setSeriesPart(value ? Math.max(1, seriesPart || 1) : 0);
    markDirty();
  }

  async function upload(file: File, kind: "cover" | "image") {
    if (account.state.kind !== "ready") return;
    setMediaBusy(kind);
    setActionError("");
    try {
      const asset = await uploadMediaAsset(file, account.state.value.csrf);
      if (kind === "cover") {
        setCoverAssetID(asset.id);
      } else {
        setImages((current) => [...current, {
          type: "image",
          assetId: asset.id,
          width: asset.width,
          height: asset.height,
          alt: "",
          caption: "",
          variant: "large",
        }]);
      }
      markDirty();
    } catch (error) {
      setActionError(error instanceof Error ? error.message : "Image upload failed.");
    } finally {
      setMediaBusy("");
    }
  }

  function updateImage(index: number, patch: Partial<Pick<EditorImageBlock, "alt" | "caption">>) {
    setImages((current) => current.map((image, imageIndex) => imageIndex === index ? { ...image, ...patch } : image));
    markDirty();
  }

  function removeImage(index: number) {
    setImages((current) => current.filter((_, imageIndex) => imageIndex !== index));
    markDirty();
  }

  async function setPublished(published: boolean) {
    if (!post || account.state.kind !== "ready") return;
    if (!(await save())) return;
    setActionError("");
    try {
      const response = await fetch(`/api/v1/blog/posts/${encodeURIComponent(post.id)}/${published ? "publish" : "unpublish"}`, {
        method: "POST",
        credentials: "same-origin",
        headers: { Accept: "application/json", "X-CSRF-Token": account.state.value.csrf },
      });
      if (!response.ok) {
        setActionError(`Unable to ${published ? "publish" : "unpublish"} (${response.status}).`);
        return;
      }
      resource.retry();
    } catch (error) {
      setActionError(error instanceof Error ? error.message : "Publishing action failed.");
    }
  }

  if (resource.state.kind === "loading") {
    return <section className={dashboard.content}><div className={styles.state}>Loading editor…</div></section>;
  }
  if (resource.state.kind !== "ready" || !post) {
    const message = resource.state.kind === "error" ? resource.state.message : "This post is not available.";
    return <section className={dashboard.content}><div className={styles.state}>{message}</div></section>;
  }

  return (
    <section className={`${dashboard.content} ${dashboard.editorContent}`}>
      <div className={styles.editor}>
        <header className={styles.topbar}>
          <Link href="/dashboard/blogs">← Blogs</Link>
          <span className={styles.saveState} aria-live="polite">
            {mediaBusy ? "Processing image…" : saveState === "saving" ? "Saving…" : saveState === "saved" ? "Saved" : saveState === "error" ? "Save failed" : dirty ? "Unsaved" : ""}
          </span>
          <div className={styles.actions}>
            {post.status === "published" && <Link href={`/blogs/${post.slug}`}>View</Link>}
            <button type="button" onClick={() => void save()} disabled={!dirty || saveState === "saving" || Boolean(mediaBusy)}>Save</button>
            <button className={styles.publish} type="button" onClick={() => void setPublished(post.status !== "published")} disabled={Boolean(mediaBusy)}>
              {post.status === "published" ? "Unpublish" : "Publish"}
            </button>
          </div>
        </header>

        <main className={styles.canvas}>
          {coverAssetID && (
            <div className={styles.coverPreview}>
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img src={privateMediaVariantURL(coverAssetID, "large")} alt="Post cover preview" />
            </div>
          )}
          <input className={styles.title} maxLength={200} aria-label="Title" value={title} onChange={(event) => change(setTitle, event.target.value)} placeholder="Post title" />
          <textarea className={styles.excerpt} maxLength={700} aria-label="Excerpt" value={excerpt} onChange={(event) => change(setExcerpt, event.target.value)} placeholder="Short description for previews and SEO" rows={3} />
          <textarea
            className={styles.body}
            aria-label="Post body"
            value={body}
            onChange={structured ? undefined : (event) => change(setBody, event.target.value)}
            readOnly={structured}
            placeholder="Start writing…"
            rows={18}
          />
          {structured && <p>Structured blocks are preserved exactly. Edit headings, lists, code and callouts with the Blog tool; metadata and images remain editable here.</p>}

          {images.length > 0 && (
            <section className={styles.inlineImages} aria-label="Article images">
              {images.map((image, index) => (
                <article className={styles.inlineImage} key={`${image.assetId}-${index}`}>
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img src={privateMediaVariantURL(image.assetId, "medium")} alt={image.alt || "Article image preview"} />
                  <div>
                    <input value={image.alt} maxLength={300} onChange={(event) => updateImage(index, { alt: event.target.value })} placeholder="Alt text for accessibility" />
                    <input value={image.caption} maxLength={500} onChange={(event) => updateImage(index, { caption: event.target.value })} placeholder="Caption (optional)" />
                    <button type="button" onClick={() => removeImage(index)}>Remove image</button>
                  </div>
                </article>
              ))}
            </section>
          )}
        </main>

        <aside className={styles.settings}>
          <h2>Post settings</h2>
          <label>URL slug<input value={slug} onChange={(event) => change(setSlug, event.target.value)} /></label>
          <label>Category<input maxLength={80} value={category} onChange={(event) => change(setCategory, event.target.value)} /></label>
          <label>Tags<input value={tags} onChange={(event) => change(setTags, event.target.value)} placeholder="AI, MCP, Agents" /></label>
          <label>Visibility
            <select value={visibility} onChange={(event) => change(setVisibility, event.target.value)}>
              <option value="public">Public</option>
              <option value="unlisted">Unlisted</option>
              <option value="private">Private</option>
            </select>
          </label>
          <label>Series
            <select value={seriesID} onChange={(event) => changeSeries(event.target.value)}>
              <option value="">No series</option>
              {series.map((item) => <option key={item.id} value={item.id}>{item.title}</option>)}
            </select>
          </label>
          {seriesID && (
            <label>Part number
              <input
                type="number"
                min={1}
                max={999}
                value={Math.max(1, seriesPart || 1)}
                onChange={(event) => { setSeriesPart(Math.max(1, Number.parseInt(event.target.value || "1", 10))); markDirty(); }}
              />
            </label>
          )}

          <div className={styles.mediaPanel}>
            <strong>Media</strong>
            <span>JPEG, PNG or WebP. CodeLocal stores optimized durable WebP variants.</span>
            <label className={styles.uploadButton}>
              {mediaBusy === "cover" ? "Processing cover…" : coverAssetID ? "Replace cover" : "Upload cover"}
              <input type="file" accept="image/jpeg,image/png,image/webp" disabled={Boolean(mediaBusy)} onChange={(event) => {
                const file = event.currentTarget.files?.[0];
                event.currentTarget.value = "";
                if (file) void upload(file, "cover");
              }} />
            </label>
            {coverAssetID && <button type="button" className={styles.removeMedia} onClick={() => { setCoverAssetID(""); markDirty(); }}>Remove cover</button>}
            <label className={styles.uploadButton}>
              {mediaBusy === "image" ? "Processing image…" : "Add article image"}
              <input type="file" accept="image/jpeg,image/png,image/webp" disabled={Boolean(mediaBusy)} onChange={(event) => {
                const file = event.currentTarget.files?.[0];
                event.currentTarget.value = "";
                if (file) void upload(file, "image");
              }} />
            </label>
          </div>
          {seriesResource.state.kind === "error" && <p className={styles.error} role="alert">Series unavailable: {seriesResource.state.message}</p>}
          {actionError && <p className={styles.error} role="alert">{actionError}</p>}
        </aside>
      </div>
    </section>
  );
}
