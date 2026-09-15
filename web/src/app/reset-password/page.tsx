import { AuthShell } from "../auth-shell";
import { ResetPanel } from "./reset-panel";
import { getTranslations } from "@/lib/i18n/server";

type Props = { searchParams: Promise<Record<string, string | string[] | undefined>> };
const get = (params: Record<string, string | string[] | undefined>, key: string) => typeof params[key] === "string" ? params[key] as string : "";

export default async function ResetPasswordPage({ searchParams }: Props) {
  const t = await getTranslations();
  const params = await searchParams;
  const done = get(params, "done") === "1";
  return (
    <AuthShell
      eyebrow={t("Secure recovery")}
      title={t("Rotate the password. Revoke the old trust.")}
      description={t("A completed reset updates the account security version and invalidates existing browser sessions before the account can be used again.")}
      panelTitle={t(done ? "Password reset" : "Choose a new password")}
      panelSubtitle={t(done ? "Your account is ready for a fresh sign-in." : "The reset code expires after 10 minutes.")}
    >
      <ResetPanel token={get(params, "token")} error={get(params, "error")} expired={get(params, "expired") === "1"} done={done} />
    </AuthShell>
  );
}
