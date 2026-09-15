import Link from "next/link";
import { AuthShell } from "../auth-shell";
import { CSRFFormToken } from "../csrf-form-token";
import styles from "../auth-surface.module.css";
import { getTranslations } from "@/lib/i18n/server";

type Props = { searchParams: Promise<Record<string, string | string[] | undefined>> };

function value(params: Record<string, string | string[] | undefined>, key: string, fallback = "") {
  const raw = params[key];
  return typeof raw === "string" ? raw : fallback;
}

export default async function LoginPage({ searchParams }: Props) {
  const t = await getTranslations();
  const params = await searchParams;
  const next = value(params, "next", "/dashboard");
  const error = value(params, "error");
  return (
    <AuthShell
      eyebrow="Private control plane"
      title="Your AI tools, one trusted local runtime."
      description="Sign in once to manage the machines, workspaces and Project Brain context you explicitly authorize for MCP-compatible AI clients."
      panelTitle={t("Welcome back")}
      panelSubtitle={t("Sign in to your CodeLocal.Cloud account.")}
    >
      {error ? <div className={styles.alert}>{t.message(error)}</div> : null}
      <form className={styles.form} method="post" action="/login">
        <CSRFFormToken />
        <input type="hidden" name="next" value={next} />
        <input type="hidden" name="ui" value="next" />
        <div className={styles.field}>
          <label htmlFor="email">{t("Email")}</label>
          <input className={styles.input} id="email" name="email" type="email" autoComplete="email" inputMode="email" maxLength={254} required />
        </div>
        <div className={styles.field}>
          <label htmlFor="password">{t("Password")}</label>
          <input className={styles.input} id="password" name="password" type="password" autoComplete="current-password" minLength={10} maxLength={256} required />
        </div>
        <button className={styles.button} type="submit">{t("Sign in")}</button>
      </form>
      <div className={styles.switcher}>
        {t("New to CodeLocal.Cloud?")} <Link href={`/signup?next=${encodeURIComponent(next)}`}>{t("Create an account")}</Link> · <Link href="/forgot-password">{t("Forgot password?")}</Link>
      </div>
    </AuthShell>
  );
}
