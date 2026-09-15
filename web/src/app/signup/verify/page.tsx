import { AuthShell } from "../../auth-shell";
import { VerificationPanel } from "./verification-panel";
import { getTranslations } from "@/lib/i18n/server";

type Props = { searchParams: Promise<Record<string, string | string[] | undefined>> };
const get = (params: Record<string, string | string[] | undefined>, key: string) => typeof params[key] === "string" ? params[key] as string : "";

export default async function SignupVerifyPage({ searchParams }: Props) {
  const t = await getTranslations();
  const params = await searchParams;
  return (
    <AuthShell
      eyebrow={t("Email verification")}
      title={t("One final check before the account exists.")}
      description={t("CodeLocal creates the account only after the six-digit email code is verified, keeping invite-only signup and account ownership tied together.")}
      panelTitle={t("Check your email")}
      panelSubtitle={t("Enter the six-digit verification code.")}
    >
      <VerificationPanel token={get(params, "token")} error={get(params, "error")} expired={get(params, "expired") === "1"} />
    </AuthShell>
  );
}
