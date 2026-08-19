import { PublicDoc } from "../public-doc";

export default function TermsPage() {
  return <PublicDoc
    title="Terms of Use"
    eyebrow="Service terms"
    summary="These terms describe the responsibilities and boundaries for using CodeLocal with authorized projects, local tools and AI agents."
    sections={[
      { title: "Authorized use", body: <p>You may use CodeLocal only with computers, repositories, accounts and services you own or are authorized to access. You are responsible for the instructions you give an AI model and for reviewing consequential changes before they are applied.</p> },
      { title: "Local actions and approvals", body: <p>CodeLocal provides permission and approval controls for sensitive operations, but no software safeguard replaces user judgment. Destructive, external or irreversible operations may require explicit confirmation and can still have consequences after approval.</p> },
      { title: "Your content", body: <p>You retain your rights in source code and project content. You grant CodeLocal only the permissions reasonably necessary to provide the requested service, including storing sanitized durable Project Brain records when those features are used.</p> },
      { title: "Service availability", body: <p>CodeLocal is provided on an as-available basis. Features may change as security, platform and MCP requirements evolve. Beta features may be modified, disabled or withdrawn without the stability guarantees of a stable release.</p> },
      { title: "Acceptable use", body: <p>Do not use CodeLocal to violate law, third-party rights, provider policies, service rules or access controls. Do not use it to introduce malware, steal credentials, bypass authorization or conceal harmful activity.</p> },
    ]}
  />;
}
