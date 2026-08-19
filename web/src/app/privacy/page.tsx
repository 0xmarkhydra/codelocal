import { PublicDoc } from "../public-doc";

const supportEmail = process.env.CODELOCAL_SUPPORT_EMAIL?.trim() || "support@codelocal.cloud";

export default function PrivacyPage() {
  return <PublicDoc
    title="Privacy Policy"
    eyebrow="Data handling"
    summary="CodeLocal is designed to keep raw development data and execution on the user's machine while storing only the cloud data needed for identity, routing and durable Project Brain features."
    sections={[
      { title: "What stays on your machine", body: <p>Raw source code, project secrets, local credentials, terminal execution context, approval tokens and machine-specific runtime state are intended to remain on the paired computer unless you explicitly ask a connected tool to send data elsewhere.</p> },
      { title: "What CodeLocal Cloud stores", body: <p>Cloud services may store account identity, paired-device and workspace routing metadata, sanitized durable Project Brain knowledge, verified Experience or portable-safe skill metadata, consent/preferences, usage counters and security/audit metadata needed to operate the service.</p> },
      { title: "Project Brain and collective features", body: <p>Durable Project Brain records are scoped to the authenticated user and logical project. Collective Intelligence is separately gated, disabled by default, and accepts only privacy-eligible structured or de-identified contributions when a user opts in. Raw cross-user project knowledge is not exposed to other users.</p> },
      { title: "Retention and deletion", body: <><p>Account, workspace and Project Brain records are retained while needed to provide the service or until they are deleted, revoked or a deletion request is completed. Revoking a workspace or device does not delete files from your computer.</p><p>To request deletion or a copy of Cloud-stored personal data, contact <strong>{supportEmail}</strong>.</p></> },
      { title: "Third parties and model providers", body: <p>CodeLocal is model-provider neutral and does not require its own model API key for the standard MCP workflow. Content sent by an AI client to ChatGPT, Claude or another provider is governed by that product and your selected account/workspace settings.</p> },
    ]}
  />;
}
