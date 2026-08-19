import { redirect } from "next/navigation";

type Props = { searchParams: Promise<Record<string, string | string[] | undefined>> };

export default async function RegisterPage({ searchParams }: Props) {
  const params = await searchParams;
  const next = typeof params.next === "string" ? params.next : "/dashboard";
  const ref = typeof params.ref === "string" ? params.ref : "";
  const query = new URLSearchParams({ next });
  if (ref) query.set("ref", ref);
  redirect(`/signup?${query.toString()}`);
}
