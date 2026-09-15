import { PublicDoc } from "../public-doc";
import { getTranslations } from "@/lib/i18n/server";

const supportEmail = process.env.CODELOCAL_SUPPORT_EMAIL?.trim() || "support@codelocal.cloud";
const securityEmail = process.env.CODELOCAL_SECURITY_EMAIL?.trim() || "security@codelocal.cloud";

export async function generateMetadata() {
  const t = await getTranslations();
  return { title: t("Support"), alternates: { canonical: "/support" } };
}

export default async function SupportPage() {
  const t = await getTranslations();
  return <PublicDoc
    title={t("Support")}
    eyebrow={t("Help & diagnostics")}
    summary={t("Get help with installation, pairing, workspace access, plugin connections, update mismatches and Project Brain behavior.")}
    sections={[
      { title: t("Contact"), body: <p>{t("Email {email}. Include the CodeLocal.Cloud version, protocol version, runtime status and a correlation/request ID when available. Do not send passwords, OAuth tokens, approval tokens, .env files or private source code.", { email: supportEmail })}</p> },
      { title: t("First checks"), body: <ul><li><code>codelocal --version</code>: {t("Check the version and confirm the expected release channel.")}</li><li><code>codelocal</code>: {t("Start the runtime and confirm the paired runtime is online.")}</li><li><code>codelocal .</code>: {t("Run only inside a project you intend to authorize.")}</li><li>{t("If an AI client shows an old tool surface, reconnect or refresh CodeLocal.Cloud in that client after updating the runtime.")}</li></ul> },
      { title: t("Security or privacy issue"), body: <p>{t("For suspected vulnerabilities or security-sensitive reports, use {email} instead of public issue trackers.", { email: securityEmail })}</p> },
    ]}
  />;
}
