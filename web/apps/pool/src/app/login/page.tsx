"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";

export default function LoginPage() {
  const router = useRouter();
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
      if (!response.ok) throw new Error(response.status === 401 ? "Sai mật khẩu Pool." : "Không thể đăng nhập Pool.");
      router.replace("/");
      router.refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Không thể đăng nhập Pool.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="loginShell">
      <form className="loginCard" onSubmit={submit}>
        <span className="eyebrow">CodeLocal infrastructure</span>
        <h1>Pool</h1>
        <p>Website quản trị canonical model, nguồn 9Router và BYOK. API client vẫn dùng trực tiếp <code>/v1/*</code>.</p>
        <label>
          Operator password
          <input autoComplete="current-password" autoFocus onChange={(event) => setPassword(event.target.value)} required type="password" value={password} />
        </label>
        <button className="primaryButton" disabled={busy} type="submit">{busy ? "Đang đăng nhập…" : "Đăng nhập"}</button>
        {error ? <p className="errorText">{error}</p> : null}
      </form>
    </main>
  );
}
