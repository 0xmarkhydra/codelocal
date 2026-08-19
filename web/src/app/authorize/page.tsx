import { AuthShell } from "../auth-shell";
import { AuthorizePanel } from "./authorize-panel";

type Props = { searchParams: Promise<Record<string, string | string[] | undefined>> };

export default async function AuthorizePage({ searchParams }: Props) {
  const params = await searchParams;
  const error = typeof params.error === "string" ? params.error : "";
  return (
    <AuthShell
      eyebrow="OAuth consent"
      title="Approve one MCP client, not your whole machine."
      description="CodeLocal binds this authorization to the registered redirect URI, PKCE challenge and your authenticated account before issuing an OAuth code."
      panelTitle="Authorize MCP client"
      panelSubtitle="Review the client and CodeLocal resource before continuing."
      highlights={["PKCE S256 required", "Exact registered redirect URI", "Account-scoped devices and workspaces only"]}
    >
      <AuthorizePanel error={error} />
    </AuthShell>
  );
}
