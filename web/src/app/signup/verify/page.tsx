import { AuthShell } from "../../auth-shell";
import { VerificationPanel } from "./verification-panel";

type Props = { searchParams: Promise<Record<string, string | string[] | undefined>> };
const get = (params: Record<string, string | string[] | undefined>, key: string) => typeof params[key] === "string" ? params[key] as string : "";

export default async function SignupVerifyPage({ searchParams }: Props) {
  const params = await searchParams;
  return (
    <AuthShell
      eyebrow="Email verification"
      title="One final check before the account exists."
      description="CodeLocal creates the account only after the six-digit email code is verified, keeping invite-only signup and account ownership tied together."
      panelTitle="Check your email"
      panelSubtitle="Enter the six-digit verification code."
    >
      <VerificationPanel token={get(params, "token")} error={get(params, "error")} expired={get(params, "expired") === "1"} />
    </AuthShell>
  );
}
