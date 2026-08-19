"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { CSRFFormToken } from "../csrf-form-token";
import styles from "../auth-surface.module.css";

type Props = { token: string; error: string; expired: boolean; done: boolean };

export function ResetPanel({ token, error, expired, done }: Props) {
  const [maskedEmail, setMaskedEmail] = useState("");
  const [valid, setValid] = useState(!expired);

  useEffect(() => {
    if (!token || expired || done) return;
    fetch(`/api/v1/auth/password-reset?token=${encodeURIComponent(token)}`, { cache: "no-store", credentials: "same-origin" })
      .then(async (response) => {
        if (!response.ok) throw new Error("expired");
        return response.json() as Promise<{ valid?: unknown; emailMasked?: unknown }>;
      })
      .then((payload) => {
        setValid(payload.valid === true);
        if (typeof payload.emailMasked === "string") setMaskedEmail(payload.emailMasked);
      })
      .catch(() => setValid(false));
  }, [token, expired, done]);

  if (done) {
    return (
      <div className={styles.actions}>
        <div className={styles.success}>Your password has been updated. Existing signed-in sessions were revoked.</div>
        <Link className={styles.button} href="/login">Sign in</Link>
      </div>
    );
  }

  if (!valid || !token) {
    return (
      <div className={styles.actions}>
        <div className={styles.alert}>That password reset request is no longer valid.</div>
        <Link className={styles.button} href="/forgot-password">Request a new reset</Link>
        <Link className={styles.secondaryButton} href="/login">Back to sign in</Link>
      </div>
    );
  }

  return (
    <>
      {error ? <div className={styles.alert}>{error}</div> : null}
      <form className={styles.form} method="post" action="/reset-password">
        <CSRFFormToken />
        <input type="hidden" name="token" value={token} />
        <input type="hidden" name="ui" value="next" />
        <div className={styles.field}>
          <label htmlFor="code">Reset code</label>
          <input className={`${styles.input} ${styles.mono}`} id="code" name="code" type="text" inputMode="numeric" autoComplete="one-time-code" pattern="[0-9]{6}" minLength={6} maxLength={6} required />
          <span className={styles.hint}>We sent a six-digit code{maskedEmail ? ` to ${maskedEmail}` : " to your email"}.</span>
        </div>
        <div className={styles.field}>
          <label htmlFor="password">New password</label>
          <input className={styles.input} id="password" name="password" type="password" autoComplete="new-password" minLength={10} maxLength={256} required />
        </div>
        <div className={styles.field}>
          <label htmlFor="confirmPassword">Confirm new password</label>
          <input className={styles.input} id="confirmPassword" name="confirmPassword" type="password" autoComplete="new-password" minLength={10} maxLength={256} required />
        </div>
        <button className={styles.button} type="submit">Reset password</button>
      </form>
    </>
  );
}
