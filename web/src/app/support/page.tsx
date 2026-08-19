import { PublicDoc } from "../public-doc";

const supportEmail = process.env.CODELOCAL_SUPPORT_EMAIL?.trim() || "support@codelocal.cloud";
const securityEmail = process.env.CODELOCAL_SECURITY_EMAIL?.trim() || "security@codelocal.cloud";

export default function SupportPage() {
  return <PublicDoc
    title="Support"
    eyebrow="Help & diagnostics"
    summary="Get help with installation, pairing, workspace access, plugin connections, update mismatches and Project Brain behavior."
    sections={[
      { title: "Contact", body: <p>Email <strong>{supportEmail}</strong>. Include the CodeLocal version, protocol version, runtime status and a correlation/request ID when available. Do not send passwords, OAuth tokens, approval tokens, .env files or private source code.</p> },
      { title: "First checks", body: <ul><li>Run <code>codelocal --version</code> and confirm the expected release channel.</li><li>Run <code>codelocal</code> and confirm the paired runtime is online.</li><li>Run <code>codelocal .</code> only inside a project you intend to authorize.</li><li>If an AI client shows an old tool surface, reconnect or refresh CodeLocal in that client after updating the runtime.</li></ul> },
      { title: "Security or privacy issue", body: <p>For suspected vulnerabilities or security-sensitive reports, use <strong>{securityEmail}</strong> instead of public issue trackers.</p> },
    ]}
  />;
}
