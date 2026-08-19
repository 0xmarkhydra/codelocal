import { PublicDoc } from "../public-doc";

const securityEmail = process.env.CODELOCAL_SECURITY_EMAIL?.trim() || "security@codelocal.cloud";

export default function SecurityPage() {
  return <PublicDoc
    title="Security"
    eyebrow="Trust boundary"
    summary="CodeLocal separates AI reasoning, cloud coordination and local execution so that access remains scoped to explicitly authorized workspaces and guarded actions."
    sections={[
      { title: "Execution boundary", body: <p>Filesystem, Git, terminal, browser and desktop execution occur through a paired CodeLocal runtime. Workspaces must be explicitly authorized. The Cloud gateway routes authenticated requests but does not turn the entire computer into a remotely browsable filesystem.</p> },
      { title: "Approval model", body: <p>Potentially destructive, open-world or otherwise sensitive operations are classified by policy and may require user confirmation. Approval tokens are scoped and must not be treated as reusable credentials.</p> },
      { title: "MCP metadata", body: <p>The public plugin exposes a compact MCP tool surface with explicit read-only, destructive and open-world annotations. CodeLocal treats annotations as review metadata; local security policy and user approval remain authoritative.</p> },
      { title: "Report a vulnerability", body: <p>Send security reports to <strong>{securityEmail}</strong>. Include reproduction steps and impact, but never include real user credentials or unrelated private data.</p> },
    ]}
  />;
}
