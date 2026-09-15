import Link from "next/link";
import { AuthShell } from "../auth-shell";
import { CSRFFormToken } from "../csrf-form-token";
import styles from "../auth-surface.module.css";
import { getTranslations } from "@/lib/i18n/server";

type Props = { searchParams: Promise<Record<string, string | string[] | undefined>> };
const get = (params: Record<string, string | string[] | undefined>, key: string) => typeof params[key] === "string" ? params[key] as string : "";

export default async function ForgotPasswordPage({ searchParams }: Props) {
  const t = await getTranslations();
  const params = await searchParams;
  const sent = get(params, "sent") === "1";
  const error = get(params, "error");
  return (
    <AuthShell
      eyebrow={t("Account recovery")}
      title={t("Recover access without weakening the trust boundary.")}
      description={t("Password reset codes expire quickly and completion revokes existing signed-in sessions. CodeLocal never exposes recovery state to advertisers or third parties.")}
      panelTitle={t(sent ? "Check your email" : "Forgot your password?")}
      panelSubtitle={t(sent ? "If the account exists, reset instructions have been sent." : "Enter the email used for your CodeLocal account.")}
    >
      {sent ? (
        <div className={styles.actions}>
          <div className={styles.success}>{t("Open the email from CodeLocal. The reset code expires after 10 minutes.")}</div>
          <Link className={styles.secondaryButton} href="/login">{t("Back to sign in")}</Link>
        </div>
      ) : (
        <>
          {error ? <div className={styles.alert}>{t.message(error)}</div> : null}
          <form className={styles.form} method="post" action="/forgot-password">
            <CSRFFormToken />
            <input type="hidden" name="ui" value="next" />
            <div className={styles.field}>
              <label htmlFor="email">{t("Email")}</label>
              <input className={styles.input} id="email" name="email" type="email" autoComplete="email" inputMode="email" maxLength={254} required />
            </div>
            <button className={styles.button} type="submit">{t("Send reset instructions")}</button>
          </form>
          <div className={styles.switcher}><Link href="/login">{t("Back to sign in")}</Link></div>
        </>
      )}
    </AuthShell>
  );
}
