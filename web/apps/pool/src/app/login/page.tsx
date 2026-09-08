"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";
import { LanguageSelect, useTranslations } from "@codelocal/i18n/provider";

export default function LoginPage() {
  const router = useRouter();
  const { t, message } = useTranslations();
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      const response = await fetch("/api/session", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ password }),
      });
      if (!response.ok) throw new Error(response.status === 401 ? "Incorrect Pool password." : "Could not sign in to Pool.");
      router.replace("/");
      router.refresh();
    } catch (cause) {
      setError(cause instanceof Error && cause.message === "Incorrect Pool password." ? cause.message : "Could not sign in to Pool.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="loginShell">
      <form className="loginCard" onSubmit={submit}>
        <div className="loginTopbar"><h1>CodeLocal Pool</h1><LanguageSelect /></div>
        <label>
          {t("Operator password")}
          <input autoComplete="current-password" autoFocus onChange={(event) => setPassword(event.target.value)} required type="password" value={password} />
        </label>
        <button className="primaryButton" disabled={busy} type="submit">{busy ? t("Signing in…") : t("Sign in")}</button>
        {error ? <p className="errorText" role="alert">{message(error)}</p> : null}
      </form>
    </main>
  );
}
