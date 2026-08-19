import Link from "next/link";
import { AuthShell } from "../auth-shell";
import { CSRFFormToken } from "../csrf-form-token";
import styles from "../auth-surface.module.css";

type Props = { searchParams: Promise<Record<string, string | string[] | undefined>> };
const get = (params: Record<string, string | string[] | undefined>, key: string, fallback = "") => typeof params[key] === "string" ? params[key] as string : fallback;

export default async function SignupPage({ searchParams }: Props) {
  const params = await searchParams;
  const next = get(params, "next", "/dashboard");
  const referral = get(params, "ref");
  const error = get(params, "error");
  return (
    <AuthShell
      eyebrow="Invite-only access"
      title="Give your AI client a controlled way into your real projects."
      description="Create one CodeLocal identity, pair the machines you trust, and authorize project folders independently. Local source stays local."
      panelTitle="Create your account"
      panelSubtitle="A valid member invite code is required."
      highlights={["Email verification before account creation", "Per-device and per-workspace authorization", "OAuth-ready for compatible MCP clients"]}
    >
      {error ? <div className={styles.alert}>{error}</div> : null}
      <form className={styles.form} method="post" action="/signup">
        <CSRFFormToken />
        <input type="hidden" name="next" value={next} />
        <input type="hidden" name="ui" value="next" />
        <div className={styles.field}>
          <label htmlFor="email">Email</label>
          <input className={styles.input} id="email" name="email" type="email" autoComplete="email" inputMode="email" maxLength={254} required />
        </div>
        <div className={styles.field}>
          <label htmlFor="password">Password</label>
          <input className={styles.input} id="password" name="password" type="password" autoComplete="new-password" minLength={10} maxLength={256} required />
        </div>
        <div className={styles.field}>
          <label htmlFor="referralCode">Referral code</label>
          <input className={`${styles.input} ${styles.mono}`} id="referralCode" name="referralCode" defaultValue={referral} minLength={4} maxLength={6} pattern="[A-Za-z0-9]+" autoComplete="off" required />
          <span className={styles.hint}>Enter the invite code shared by an existing CodeLocal member.</span>
        </div>
        <button className={styles.button} type="submit">Create account</button>
      </form>
      <div className={styles.switcher}>Already have an account? <Link href={`/login?next=${encodeURIComponent(next)}`}>Sign in</Link></div>
    </AuthShell>
  );
}
