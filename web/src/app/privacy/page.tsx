import { PublicDoc } from "../public-doc";
import { getTranslations } from "@/lib/i18n/server";

const supportEmail = process.env.CODELOCAL_SUPPORT_EMAIL?.trim() || "support@codelocal.cloud";

export async function generateMetadata() {
  const t = await getTranslations();
  return { title: t("Privacy Policy"), alternates: { canonical: "/privacy" } };
}

export default async function PrivacyPage() {
  const t = await getTranslations();
  return <PublicDoc
    title={t("Privacy Policy")}
    eyebrow={t("Data handling")}
    summary={t("CodeLocal.Cloud is designed to keep raw development data and execution on the user's machine while storing only the cloud data needed for identity, routing and durable Project Brain features.")}
    sections={[
      { title: t("What stays on your machine"), body: <p>{t("Raw source code, project secrets, local credentials, terminal execution context, approval tokens and machine-specific runtime state are intended to remain on the paired computer unless you explicitly ask a connected tool to send data elsewhere.")}</p> },
      { title: t("What CodeLocal.Cloud stores"), body: <p>{t("Cloud services may store account identity, paired-device and workspace routing metadata, sanitized durable Project Brain knowledge, verified Experience or portable-safe skill metadata, consent/preferences, usage counters and security/audit metadata needed to operate the service.")}</p> },
      { title: t("Project Brain and collective features"), body: <p>{t("Durable Project Brain records are scoped to the authenticated user and logical project. Collective Intelligence is separately gated, disabled by default, and accepts only privacy-eligible structured or de-identified contributions when a user opts in. Raw cross-user project knowledge is not exposed to other users.")}</p> },
      { title: t("Retention and deletion"), body: <><p>{t("Account, workspace and Project Brain records are retained while needed to provide the service or until they are deleted, revoked or a deletion request is completed. Revoking a workspace or device does not delete files from your computer.")}</p><p>{t("To request deletion or a copy of Cloud-stored personal data, contact {email}.", { email: supportEmail })}</p></> },
      { title: t("Third parties and model providers"), body: <p>{t("CodeLocal.Cloud is model-provider neutral and does not require its own model API key for the standard MCP workflow. Content sent by an AI client to ChatGPT, Claude or another provider is governed by that product and your selected account/workspace settings.")}</p> },
    ]}
  />;
}
