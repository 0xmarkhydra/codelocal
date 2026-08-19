import Link from "next/link";
import { AuthShell } from "../auth-shell";
import { CSRFFormToken } from "../csrf-form-token";
import styles from "../auth-surface.module.css";

type Props = { searchParams: Promise<Record<string, string | string[] | undefined>> };
const get = (params: Record<string, string | string[] | undefined>, key: string) => typeof params[key] === "string" ? params[key] as string : "";

export default async function ForgotPasswordPage({ searchParams }: Props) {
  const params = await searchParams;
  const sent = get(params, "sent") === "1";
  const error = get(params, "error");
  return (
    <AuthShell
      eyebrow="Account recovery"
      title="Recover access without weakening the trust boundary."
      description="Password reset codes expire quickly and completion revokes existing signed-in sessions. CodeLocal never exposes recovery state to advertisers or third parties."
      panelTitle={sent ? "Check your email" : "Forgot your password?"}
      panelSubtitle={sent ? "If the account exists, reset instructions have been sent." : "Enter the email used for your CodeLocal account."}
    >
      {sent ? (
        <div className={styles.actions}>
          <div className={styles.success}>Open the email from CodeLocal. The reset code expires after 10 minutes.</div>
          <Link className={styles.secondaryButton} href="/login">Back to sign in</Link>
        </div>
      ) : (
        <>
          {error ? <div className={styles.alert}>{error}</div> : null}
          <form className={styles.form} method="post" action="/forgot-password">
            <CSRFFormToken />
            <input type="hidden" name="ui" value="next" />
            <div className={styles.field}>
              <label htmlFor="email">Email</label>
              <input className={styles.input} id="email" name="email" type="email" autoComplete="email" inputMode="email" maxLength={254} required />
            </div>
            <button className={styles.button} type="submit">Send reset instructions</button>
          </form>
          <div className={styles.switcher}><Link href="/login">Back to sign in</Link></div>
        </>
      )}
    </AuthShell>
  );
}
