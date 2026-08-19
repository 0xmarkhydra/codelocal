"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { isAccountResource } from "@/lib/contracts/account";
import { DashboardResourceFeedback } from "../dashboard-resource-feedback";
import { formatDashboardTime } from "../dashboard-format";
import styles from "../dashboard.module.css";
import { useDashboardResource } from "../use-dashboard-resource";

export function LiveAccount() {
  const searchParams = useSearchParams();
  const { state, retry } = useDashboardResource("/api/v1/account", isAccountResource);
  const ok = searchParams.get("ok");
  const error = searchParams.get("error");

  if (state.kind !== "ready") {
    return (
      <DashboardResourceFeedback
        label="Account"
        {...(state.kind === "error"
          ? { kind: "error" as const, message: state.message, onRetry: retry }
          : { kind: state.kind })}
      />
    );
  }

  const account = state.value;
  return (
    <>
      {(ok || error) && (
        <section className={error ? styles.accountFlashError : styles.accountFlashSuccess} role="status">
          {error || ok}
        </section>
      )}

      {account.requiresReauthentication && (
        <section className={styles.accountRisk} role="alert">
          <span className={styles.eyebrow}>Fresh security context required</span>
          <h2>This browser session observed a security-signal change.</h2>
          <p>Sensitive changes will require you to sign in again. This is enforced by the Go backend, not by the Next.js page.</p>
        </section>
      )}

      <section className={styles.accountGrid}>
        <article className={styles.panel}>
          <div className={styles.panelHead}>
            <div>
              <span className={styles.eyebrow}>Identity</span>
              <h3>Account information</h3>
            </div>
            {account.isAdmin && <span className={styles.badge}>ADMIN</span>}
          </div>
          <dl className={styles.accountDetails}>
            <div><dt>Email</dt><dd>{account.email}</dd></div>
            <div><dt>User ID</dt><dd>{account.userId}</dd></div>
            <div><dt>Referral code</dt><dd>{account.referralCode || "—"}</dd></div>
            <div><dt>Invited by</dt><dd>{account.invitedBy}</dd></div>
            <div><dt>Member since</dt><dd>{formatDashboardTime(account.createdAt)}</dd></div>
            <div><dt>Password updated</dt><dd>{account.passwordChangedAt > 0 ? formatDashboardTime(account.passwordChangedAt) : "Not changed since account creation"}</dd></div>
          </dl>
        </article>

        <article className={styles.panel}>
          <div className={styles.panelHead}>
            <div>
              <span className={styles.eyebrow}>Password security</span>
              <h3>Change password</h3>
            </div>
          </div>
          <p className={styles.accountSecurityCopy}>
            The form posts directly to the existing Go security handler. A successful change rotates this browser session and revokes other signed-in web sessions.
          </p>
          <form className={styles.accountForm} method="post" action="/account/password">
            <input type="hidden" name="csrf" value={account.csrf} />
            <input type="hidden" name="next" value="/dashboard/account-preview" />
            <label>
              <span>Current password</span>
              <input type="password" name="currentPassword" autoComplete="current-password" maxLength={256} required />
            </label>
            <label>
              <span>New password</span>
              <input type="password" name="password" autoComplete="new-password" minLength={10} maxLength={256} required />
            </label>
            <label>
              <span>Confirm new password</span>
              <input type="password" name="confirmPassword" autoComplete="new-password" minLength={10} maxLength={256} required />
            </label>
            <button className={styles.liveAction} type="submit">Update password</button>
          </form>
          <p className={styles.accountHelp}>
            Locked out? <Link href="/forgot-password">Use the verified-email reset flow</Link>. Reset emails remain limited by the existing Go rate limits.
          </p>
        </article>
      </section>
    </>
  );
}
