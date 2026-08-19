"use client";

import { useEffect, useState } from "react";
import { CSRFFormToken } from "../../csrf-form-token";
import styles from "../../auth-surface.module.css";

type Props = { token: string; error: string; expired: boolean };

export function VerificationPanel({ token, error, expired }: Props) {
  const [maskedEmail, setMaskedEmail] = useState("");
  const [valid, setValid] = useState(!expired);

  useEffect(() => {
    if (!token || expired) return;
    fetch(`/api/v1/auth/signup-verification?token=${encodeURIComponent(token)}`, { cache: "no-store", credentials: "same-origin" })
      .then(async (response) => {
        if (!response.ok) throw new Error("expired");
        return response.json() as Promise<{ valid?: unknown; emailMasked?: unknown }>;
      })
      .then((payload) => {
        setValid(payload.valid === true);
        if (typeof payload.emailMasked === "string") setMaskedEmail(payload.emailMasked);
      })
      .catch(() => setValid(false));
  }, [token, expired]);

  if (!valid || !token) {
    return (
      <div className={styles.actions}>
        <div className={styles.alert}>That verification request is no longer valid. Verification codes expire after 10 minutes.</div>
        <a className={styles.button} href="/signup">Start sign up again</a>
      </div>
    );
  }

  return (
    <>
      {error ? <div className={styles.alert}>{error}</div> : null}
      <form className={styles.form} method="post" action="/signup/verify">
        <CSRFFormToken />
        <input type="hidden" name="token" value={token} />
        <input type="hidden" name="ui" value="next" />
        <div className={styles.field}>
          <label htmlFor="code">Verification code</label>
          <input className={`${styles.input} ${styles.mono}`} id="code" name="code" type="text" inputMode="numeric" autoComplete="one-time-code" pattern="[0-9]{6}" minLength={6} maxLength={6} required />
          <span className={styles.hint}>We sent a 6-digit code{maskedEmail ? ` to ${maskedEmail}` : " to your email"}. It expires after 10 minutes.</span>
        </div>
        <button className={styles.button} type="submit">Verify email</button>
      </form>
      <div className={styles.switcher}><a href="/signup">Use a different email</a></div>
    </>
  );
}
