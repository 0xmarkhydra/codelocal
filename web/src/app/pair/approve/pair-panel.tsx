"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import styles from "../../auth-surface.module.css";
import { useTranslations } from "@/lib/i18n/provider";
import { formatDashboardTime } from "../../dashboard/dashboard-format";

type Account = { email: string; csrf: string };
type Pairing = { pairingId: string; code: string; deviceId: string; deviceName: string; expiresAt: number };

export function PairPanel({ pairingId, approved, error }: { pairingId: string; approved: boolean; error: string }) {
  const { locale, t, message } = useTranslations();
  const router = useRouter();
  const [account, setAccount] = useState<Account | null>(null);
  const [pairing, setPairing] = useState<Pairing | null>(null);
  const [unavailable, setUnavailable] = useState(false);

  useEffect(() => {
    if (approved) return;
    Promise.all([
      fetch("/api/v1/account", { cache: "no-store", credentials: "same-origin" }),
      fetch(`/api/v1/pair/approve?pairingId=${encodeURIComponent(pairingId)}`, { cache: "no-store", credentials: "same-origin" }),
    ]).then(async ([accountResponse, pairingResponse]) => {
      if (accountResponse.status === 401 || pairingResponse.status === 401) {
        router.replace(`/login?next=${encodeURIComponent(`/pair/approve?pairingId=${pairingId}`)}`);
        return;
      }
      if (!accountResponse.ok || !pairingResponse.ok) throw new Error("unavailable");
      const accountPayload = await accountResponse.json() as Record<string, unknown>;
      const pairingPayload = await pairingResponse.json() as Record<string, unknown>;
      if (typeof accountPayload.email !== "string" || typeof accountPayload.csrf !== "string" || typeof pairingPayload.pairingId !== "string" || typeof pairingPayload.code !== "string" || typeof pairingPayload.deviceId !== "string" || typeof pairingPayload.deviceName !== "string" || typeof pairingPayload.expiresAt !== "number") throw new Error("invalid");
      setAccount({ email: accountPayload.email, csrf: accountPayload.csrf });
      setPairing(pairingPayload as Pairing);
    }).catch(() => setUnavailable(true));
  }, [pairingId, approved, router]);

  if (approved) return <div className={styles.actions}><div className={styles.success}>{t("Device approved. Return to your terminal; CodeLocal.Cloud will claim its credential automatically.")}</div><Link className={styles.button} href="/dashboard/devices">{t("View devices")}</Link></div>;
  if (unavailable) return <div className={styles.actions}><div className={styles.alert}>{t("This pairing request is invalid, expired, or temporarily unavailable.")}</div><Link className={styles.secondaryButton} href="/dashboard/devices">{t("Open devices")}</Link></div>;
  if (!account || !pairing) return <div className={styles.hint}>{t("Loading pairing request…")}</div>;

  return <>
    {error ? <div className={styles.alert}>{message(error)}</div> : null}
    <div className={styles.codeBlock}>{pairing.deviceName}<br />{t("Device ID")}: {pairing.deviceId}</div>
    <form className={styles.form} method="post" action="/pair/approve">
      <input type="hidden" name="csrf" value={account.csrf} />
      <input type="hidden" name="pairingId" value={pairing.pairingId} />
      <input type="hidden" name="code" value={pairing.code} />
      <input type="hidden" name="ui" value="next" />
      <button className={styles.button} type="submit">{t("Approve device")}</button>
    </form>
    <form className={styles.form} method="post" action="/logout">
      <input type="hidden" name="csrf" value={account.csrf} />
      <input type="hidden" name="next" value={`/pair/approve?pairingId=${pairing.pairingId}`} />
      <button className={styles.secondaryButton} type="submit">{t("Use another account")}</button>
    </form>
    <div className={styles.hint}>{t("Signed in as {email}. Approval expires at {time}.", { email: account.email, time: formatDashboardTime(pairing.expiresAt, locale) })}</div>
  </>;
}
