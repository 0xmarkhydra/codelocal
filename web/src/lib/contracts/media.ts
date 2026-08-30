export type MediaVariant = {
  assetId: string;
  variant: "original" | "thumb" | "medium" | "large";
  contentType: string;
  size: number;
  width: number;
  height: number;
  sha256: string;
  createdAt: number;
};

export type MediaAsset = {
  id: string;
  ownerUserId: string;
  sourceSha256: string;
  sourceContentType: string;
  sourceSize: number;
  width: number;
  height: number;
  status: "processing" | "ready" | "failed" | "deleted";
  preserveOriginal: boolean;
  errorCode?: string;
  createdAt: number;
  updatedAt: number;
  deletedAt?: number;
  variants: MediaVariant[];
};

export type MediaUploadGrant = {
  required: boolean;
  url?: string;
  method?: string;
  headers?: Record<string, string[]>;
};

export type MediaAssetResponse = {
  asset: MediaAsset;
  urls: Record<string, string>;
};

export type MediaAssetPrepareResponse = MediaAssetResponse & {
  deduplicated: boolean;
  upload: MediaUploadGrant;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isNonNegativeNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value) && value >= 0;
}

function isVariant(value: unknown): value is MediaVariant {
  if (!isRecord(value)) return false;
  return (
    typeof value.assetId === "string" &&
    ["original", "thumb", "medium", "large"].includes(String(value.variant)) &&
    typeof value.contentType === "string" &&
    isNonNegativeNumber(value.size) && isNonNegativeNumber(value.width) && isNonNegativeNumber(value.height) &&
    typeof value.sha256 === "string" && isNonNegativeNumber(value.createdAt)
  );
}

function isAsset(value: unknown): value is MediaAsset {
  if (!isRecord(value) || !Array.isArray(value.variants)) return false;
  return (
    typeof value.id === "string" && value.id.startsWith("media_") &&
    typeof value.ownerUserId === "string" &&
    typeof value.sourceSha256 === "string" && value.sourceSha256.length === 64 &&
    typeof value.sourceContentType === "string" &&
    isNonNegativeNumber(value.sourceSize) && isNonNegativeNumber(value.width) && isNonNegativeNumber(value.height) &&
    ["processing", "ready", "failed", "deleted"].includes(String(value.status)) &&
    typeof value.preserveOriginal === "boolean" &&
    isNonNegativeNumber(value.createdAt) && isNonNegativeNumber(value.updatedAt) &&
    value.variants.every(isVariant)
  );
}

function isURLs(value: unknown): value is Record<string, string> {
  return isRecord(value) && Object.values(value).every((item) => typeof item === "string");
}

export function isMediaAssetResponse(value: unknown): value is MediaAssetResponse {
  return isRecord(value) && isAsset(value.asset) && isURLs(value.urls);
}

export function isMediaAssetPrepareResponse(value: unknown): value is MediaAssetPrepareResponse {
  if (!isMediaAssetResponse(value) || !isRecord(value.upload) || typeof value.deduplicated !== "boolean") return false;
  const upload = value.upload;
  return (
    typeof upload.required === "boolean" &&
    (upload.url === undefined || typeof upload.url === "string") &&
    (upload.method === undefined || typeof upload.method === "string") &&
    (upload.headers === undefined || isRecord(upload.headers))
  );
}
