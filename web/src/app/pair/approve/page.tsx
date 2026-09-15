import { AuthShell } from "../../auth-shell";
import { PairPanel } from "./pair-panel";
import { getTranslations } from "@/lib/i18n/server";

type Props = { searchParams: Promise<Record<string, string | string[] | undefined>> };
const get = (params: Record<string, string | string[] | undefined>, key: string) => typeof params[key] === "string" ? params[key] as string : "";

export default async function PairApprovePage({ searchParams }: Props) {
  const t = await getTranslations();
  const params = await searchParams;
  const approved = get(params, "approved") === "1";
  return <AuthShell
    eyebrow={t("Device trust")}
    title={t("Pair a machine without widening the account boundary.")}
    description={t("This approval binds one local runtime to your CodeLocal account. Project folders are still authorized separately, and revoking the device does not delete local files.")}
    panelTitle={t(approved ? "Device approved" : "Approve device")}
    panelSubtitle={t(approved ? "The local runtime can finish claiming its credential." : "Review the machine identity before granting account access.")}
  >
    <PairPanel pairingId={get(params, "pairingId")} approved={approved} error={get(params, "error")} />
  </AuthShell>;
}
