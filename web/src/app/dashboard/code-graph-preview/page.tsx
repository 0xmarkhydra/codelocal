import { redirect } from "next/navigation";

type Props = { searchParams: Promise<Record<string, string | string[] | undefined>> };

export default async function CodeGraphPreviewPage({ searchParams }: Props) {
  const params = await searchParams;
  const query = new URLSearchParams();
  for (const [key, raw] of Object.entries(params)) {
    if (typeof raw === "string") query.set(key, raw);
  }
  redirect(`/dashboard/code-graph${query.size ? `?${query.toString()}` : ""}`);
}
