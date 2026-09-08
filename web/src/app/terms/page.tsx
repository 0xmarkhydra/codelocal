import { PublicDoc } from "../public-doc";
import { getTranslations } from "@/lib/i18n/server";

export async function generateMetadata() {
  const t = await getTranslations();
  return { title: t("Terms of Use"), alternates: { canonical: "/terms" } };
}

export default async function TermsPage() {
  const t = await getTranslations();
  return <PublicDoc
    title={t("Terms of Use")}
    eyebrow={t("Service terms")}
    summary={t("These terms describe the responsibilities and boundaries for using CodeLocal with authorized projects, local tools and AI agents.")}
    sections={[
      { title: t("Authorized use"), body: <p>{t("You may use CodeLocal only with computers, repositories, accounts and services you own or are authorized to access. You are responsible for the instructions you give an AI model and for reviewing consequential changes before they are applied.")}</p> },
      { title: t("Local actions and approvals"), body: <p>{t("CodeLocal provides permission and approval controls for sensitive operations, but no software safeguard replaces user judgment. Destructive, external or irreversible operations may require explicit confirmation and can still have consequences after approval.")}</p> },
      { title: t("Your content"), body: <p>{t("You retain your rights in source code and project content. You grant CodeLocal only the permissions reasonably necessary to provide the requested service, including storing sanitized durable Project Brain records when those features are used.")}</p> },
      { title: t("Service availability"), body: <p>{t("CodeLocal is provided on an as-available basis. Features may change as security, platform and MCP requirements evolve. Beta features may be modified, disabled or withdrawn without the stability guarantees of a stable release.")}</p> },
      { title: t("Acceptable use"), body: <p>{t("Do not use CodeLocal to violate law, third-party rights, provider policies, service rules or access controls. Do not use it to introduce malware, steal credentials, bypass authorization or conceal harmful activity.")}</p> },
    ]}
  />;
}
