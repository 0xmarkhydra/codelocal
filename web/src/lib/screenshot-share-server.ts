import "server-only";

import { isScreenshotShareResource, type ScreenshotShare } from "@/lib/contracts/shots";
import { backendURL } from "@/lib/server/backend";

export async function getScreenshotShareForRender(shareID: string): Promise<ScreenshotShare | undefined> {
  if (!/^[A-Za-z0-9_-]{12,64}$/.test(shareID)) return undefined;
  try {
    const response = await fetch(backendURL(`/api/v1/public/shots/${encodeURIComponent(shareID)}`), {
      method: "GET",
      cache: "no-store",
      headers: { Accept: "application/json" },
    });
    if (!response.ok) return undefined;
    const body: unknown = await response.json();
    return isScreenshotShareResource(body) ? body.share : undefined;
  } catch {
    return undefined;
  }
}
