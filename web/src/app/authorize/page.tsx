import { AuthShell } from "../auth-shell";
import { AuthorizePanel } from "./authorize-panel";
import { getTranslations } from "@/lib/i18n/server";

type Props = { searchParams: Promise<Record<string, string | string[] | undefined>> };

export default async function AuthorizePage({ searchParams }: Props) {
  const t = await getTranslations();
  const params = await searchParams;
  const error = typeof params.error === "string" ? params.error : "";
  return (
    <AuthShell
      eyebrow={t("OAuth consent")}
      title={t("Approve one MCP client, not your whole machine.")}
      description={t("CodeLocal binds this authorization to the registered redirect URI, PKCE challenge and your authenticated account before issuing an OAuth code.")}
      panelTitle={t("Authorize MCP client")}
      panelSubtitle={t("Review the client and CodeLocal resource before continuing.")}
      highlights={[t("PKCE S256 required"), t("Exact registered redirect URI"), t("Account-scoped devices and workspaces only")]}
    >
      <AuthorizePanel error={error} />
    </AuthShell>
  );
}
