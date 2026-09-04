export type ScreenshotShare = {
  id: string;
  assetId?: string;
  url: string;
  imageUrl: string;
  previewUrl: string;
  thumbnailUrl: string;
  downloadUrl: string;
  contentType: string;
  size: number;
  width: number;
  height: number;
  createdAt: number;
};

export type ScreenshotShareResource = { share: ScreenshotShare };
export type ScreenshotSharesResource = { shares: ScreenshotShare[] };

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isNonNegativeNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value) && value >= 0;
}

function isHTTPURL(value: unknown): value is string {
  if (typeof value !== "string") return false;
  try {
    const url = new URL(value);
    return url.protocol === "http:" || url.protocol === "https:";
  } catch {
    return false;
  }
}

function isScreenshotShare(value: unknown): value is ScreenshotShare {
  if (!isRecord(value)) return false;
  return (
    typeof value.id === "string" && /^[A-Za-z0-9_-]{12,64}$/.test(value.id) &&
    (value.assetId === undefined || (typeof value.assetId === "string" && value.assetId.startsWith("media_"))) &&
    isHTTPURL(value.url) && isHTTPURL(value.imageUrl) && isHTTPURL(value.previewUrl) &&
    isHTTPURL(value.thumbnailUrl) && isHTTPURL(value.downloadUrl) &&
    typeof value.contentType === "string" && value.contentType.startsWith("image/") &&
    isNonNegativeNumber(value.size) && isNonNegativeNumber(value.width) &&
    isNonNegativeNumber(value.height) && isNonNegativeNumber(value.createdAt)
  );
}

export function isScreenshotShareResource(value: unknown): value is ScreenshotShareResource {
  return isRecord(value) && isScreenshotShare(value.share);
}

export function isScreenshotSharesResource(value: unknown): value is ScreenshotSharesResource {
  return isRecord(value) && Array.isArray(value.shares) && value.shares.every(isScreenshotShare);
}
