"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import styles from "../auth-surface.module.css";
import { useTranslations } from "@/lib/i18n/provider";

type Context = {
  clientId: string;
  clientName: string;
  redirectUri: string;
  responseType: string;
  codeChallenge: string;
  resource: string;
  scope: string;
  state: string;
  email: string;
  csrf: string;
};

function isContext(value: unknown): value is Context {
  if (!value || typeof value !== "object") return false;
  const item = value as Record<string, unknown>;
  return ["clientId", "clientName", "redirectUri", "responseType", "codeChallenge", "resource", "scope", "state", "email", "csrf"].every((key) => typeof item[key] === "string");
}

export function AuthorizePanel({ error }: { error: string }) {
  const { t, message } = useTranslations();
  const router = useRouter();
  const [context, setContext] = useState<Context | null>(null);
  const [failure, setFailure] = useState("");

  useEffect(() => {
    const query = window.location.search;
    fetch(`/api/v1/oauth/authorize-context${query}`, { cache: "no-store", credentials: "same-origin" })
      .then(async (response) => {
        if (response.status === 401) {
          router.replace(`/login?next=${encodeURIComponent(`/authorize${query}`)}`);
          throw new Error("unauthorized");
        }
        if (!response.ok) throw new Error("Invalid OAuth authorization request.");
        return response.json() as Promise<unknown>;
      })
      .then((payload) => {
        if (!isContext(payload)) throw new Error("Invalid authorization response.");
        setContext(payload);
      })
      .catch((reason) => {
        if (reason instanceof Error && reason.message !== "unauthorized") setFailure(reason.message);
      });
  }, [router]);

  if (failure) return <div className={styles.alert}>{failure === "Invalid OAuth authorization request." || failure === "Invalid authorization response." ? t(failure) : t("CodeLocal could not be reached.")}</div>;
  if (!context) return <div className={styles.hint}>{t("Validating the OAuth request…")}</div>;

  return (
    <>
      {error ? <div className={styles.alert}>{message(error)}</div> : null}
      <div className={styles.codeBlock}>{context.clientName}<br />{context.resource}</div>
      <p className={styles.hint}>{t("Signed in as {email}. This client can access only devices and workspaces belonging to this CodeLocal account.", { email: context.email })}</p>
      <form className={styles.form} method="post" action="/authorize">
        <input type="hidden" name="client_id" value={context.clientId} />
        <input type="hidden" name="redirect_uri" value={context.redirectUri} />
        <input type="hidden" name="code_challenge" value={context.codeChallenge} />
        <input type="hidden" name="resource" value={context.resource} />
        <input type="hidden" name="scope" value={context.scope} />
        <input type="hidden" name="state" value={context.state} />
        <input type="hidden" name="csrf" value={context.csrf} />
        <input type="hidden" name="ui" value="next" />
        <button className={styles.button} type="submit">{t("Authorize MCP client")}</button>
      </form>
      <div className={styles.switcher}><Link href="/dashboard/connect">{t("Cancel and return to MCP Connections")}</Link></div>
    </>
  );
}
