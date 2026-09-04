"use client";

import {
  isMediaAssetPrepareResponse,
  isMediaAssetResponse,
  type MediaAsset,
  type MediaUploadGrant,
} from "@/lib/contracts/media";

const SUPPORTED_MEDIA_TYPES = new Set(["image/jpeg", "image/png", "image/webp"]);
const MAX_MEDIA_BYTES = 25 * 1024 * 1024;

function bytesToHex(buffer: ArrayBuffer) {
  return Array.from(new Uint8Array(buffer), (byte) => byte.toString(16).padStart(2, "0")).join("");
}

async function fileSHA256(file: File) {
  if (!globalThis.crypto?.subtle) throw new Error("This browser cannot hash media securely.");
  const digest = await globalThis.crypto.subtle.digest("SHA-256", await file.arrayBuffer());
  return bytesToHex(digest);
}

function uploadHeaders(input?: Record<string, string[]>) {
  const headers = new Headers();
  if (!input) return headers;
  for (const [name, values] of Object.entries(input)) {
    for (const value of values) headers.append(name, value);
  }
  return headers;
}

async function readJSON(response: Response): Promise<unknown> {
  return response.json().catch(() => null);
}

async function directUpload(file: File, upload: MediaUploadGrant) {
  if (!upload.url) return false;
  try {
    const response = await fetch(upload.url, {
      method: upload.method || "PUT",
      headers: uploadHeaders(upload.headers),
      body: file,
    });
    return response.ok;
  } catch {
    return false;
  }
}

async function proxyUpload(file: File, assetID: string, csrf: string) {
  const response = await fetch(`/api/v1/media/assets/${encodeURIComponent(assetID)}/upload`, {
    method: "POST",
    credentials: "same-origin",
    headers: {
      Accept: "application/json",
      "Content-Type": file.type,
      "X-CSRF-Token": csrf,
    },
    body: file,
  });
  if (!response.ok) throw new Error(`Image upload failed (${response.status}).`);
}

type MediaVariantName = "original" | "thumb" | "medium" | "large";

export function privateMediaVariantURL(assetID: string, variant: MediaVariantName = "medium") {
  return `/api/v1/media/assets/${encodeURIComponent(assetID)}/variants/${variant}`;
}

export function publicMediaVariantURL(assetID: string, variant: MediaVariantName = "large") {
  return `/api/v1/public/media/${encodeURIComponent(assetID)}/${variant}`;
}

export async function uploadMediaAsset(file: File, csrf: string, options: { preserveOriginal?: boolean } = {}): Promise<MediaAsset> {
  if (!SUPPORTED_MEDIA_TYPES.has(file.type)) throw new Error("Use a JPEG, PNG, or WebP image.");
  if (file.size <= 0 || file.size > MAX_MEDIA_BYTES) throw new Error("Image must be smaller than 25 MB.");

  const sha256 = await fileSHA256(file);
  const prepare = await fetch("/api/v1/media/assets/prepare", {
    method: "POST",
    credentials: "same-origin",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
      "X-CSRF-Token": csrf,
    },
    body: JSON.stringify({ sha256, contentType: file.type, size: file.size, preserveOriginal: Boolean(options.preserveOriginal) }),
  });
  const preparedBody = await readJSON(prepare);
  if (!prepare.ok || !isMediaAssetPrepareResponse(preparedBody)) {
    throw new Error(`Unable to prepare image upload (${prepare.status}).`);
  }

  if (preparedBody.asset.status === "ready") return preparedBody.asset;

  if (preparedBody.upload.required) {
    const uploadedDirectly = await directUpload(file, preparedBody.upload);
    if (!uploadedDirectly) {
      await proxyUpload(file, preparedBody.asset.id, csrf);
    }
  }

  const finalize = await fetch(`/api/v1/media/assets/${encodeURIComponent(preparedBody.asset.id)}/finalize`, {
    method: "POST",
    credentials: "same-origin",
    headers: { Accept: "application/json", "X-CSRF-Token": csrf },
  });
  const finalizedBody = await readJSON(finalize);
  if (!finalize.ok || !isMediaAssetResponse(finalizedBody) || finalizedBody.asset.status !== "ready") {
    throw new Error(`Image processing failed (${finalize.status}).`);
  }
  return finalizedBody.asset;
}
