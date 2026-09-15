import { PublicDoc } from "../public-doc";
import { getTranslations } from "@/lib/i18n/server";

const securityEmail = process.env.CODELOCAL_SECURITY_EMAIL?.trim() || "security@codelocal.cloud";

export async function generateMetadata() {
  const t = await getTranslations();
  return { title: t("Security"), alternates: { canonical: "/security" } };
}

export default async function SecurityPage() {
  const t = await getTranslations();
  return <PublicDoc
    title={t("Security")}
    eyebrow={t("Trust boundary")}
    summary={t("CodeLocal.Cloud separates AI reasoning, cloud coordination and local execution so that access remains scoped to explicitly authorized workspaces and guarded actions.")}
    sections={[
      { title: t("Execution boundary"), body: <p>{t("Filesystem, Git, terminal, browser and desktop execution occur through a paired CodeLocal.Cloud runtime. Workspaces must be explicitly authorized. The Cloud gateway routes authenticated requests but does not turn the entire computer into a remotely browsable filesystem.")}</p> },
      { title: t("Approval model"), body: <p>{t("Potentially destructive, open-world or otherwise sensitive operations are classified by policy and may require user confirmation. Approval tokens are scoped and must not be treated as reusable credentials.")}</p> },
      { title: t("MCP metadata"), body: <p>{t("The public plugin exposes a compact MCP tool surface with explicit read-only, destructive and open-world annotations. CodeLocal.Cloud treats annotations as review metadata; local security policy and user approval remain authoritative.")}</p> },
      { title: t("Report a vulnerability"), body: <p>{t("Send security reports to {email}. Include reproduction steps and impact, but never include real user credentials or unrelated private data.", { email: securityEmail })}</p> },
    ]}
  />;
}
