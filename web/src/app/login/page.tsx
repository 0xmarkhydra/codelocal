import Link from "next/link";
import { AuthShell } from "../auth-shell";
import { CSRFFormToken } from "../csrf-form-token";
import styles from "../auth-surface.module.css";

type Props = { searchParams: Promise<Record<string, string | string[] | undefined>> };

function value(params: Record<string, string | string[] | undefined>, key: string, fallback = "") {
  const raw = params[key];
  return typeof raw === "string" ? raw : fallback;
}

export default async function LoginPage({ searchParams }: Props) {
  const params = await searchParams;
  const next = value(params, "next", "/dashboard");
  const error = value(params, "error");
  return (
    <AuthShell
      eyebrow="Private control plane"
      title="Your AI tools, one trusted local runtime."
      description="Sign in once to manage the machines, workspaces and Project Brain context you explicitly authorize for MCP-compatible AI clients."
      panelTitle="Welcome back"
      panelSubtitle="Sign in to your CodeLocal account."
    >
      {error ? <div className={styles.alert}>{error}</div> : null}
      <form className={styles.form} method="post" action="/login">
        <CSRFFormToken />
        <input type="hidden" name="next" value={next} />
        <input type="hidden" name="ui" value="next" />
        <div className={styles.field}>
          <label htmlFor="email">Email</label>
          <input className={styles.input} id="email" name="email" type="email" autoComplete="email" inputMode="email" maxLength={254} required />
        </div>
        <div className={styles.field}>
          <label htmlFor="password">Password</label>
          <input className={styles.input} id="password" name="password" type="password" autoComplete="current-password" minLength={10} maxLength={256} required />
        </div>
        <button className={styles.button} type="submit">Sign in</button>
      </form>
      <div className={styles.switcher}>
        New to CodeLocal? <Link href={`/signup?next=${encodeURIComponent(next)}`}>Create an account</Link> · <Link href="/forgot-password">Forgot password?</Link>
      </div>
    </AuthShell>
  );
}
