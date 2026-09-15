# Security Policy

## Reporting a vulnerability

Please do **not** report suspected vulnerabilities through a public GitHub issue, discussion, forum post or social-media thread.

Send security-sensitive reports privately to **security@codelocal.cloud**. The same private reporting address is published on the CodeLocal Security and Support pages.

A useful report includes the affected CodeLocal version, operating system, the relevant capability (MCP, workspace/filesystem, terminal, browser/computer, authentication, Project Brain, etc.), reproduction steps, expected vs actual behavior, and the security impact.

Do not include real passwords, OAuth tokens, device credentials, approval tokens, `.env` contents, unrelated private source code or third-party personal data. Replace secrets with clearly marked dummy values.

## Scope

Security reports are especially useful for issues involving workspace-boundary escape, sensitive-path disclosure, authentication/authorization bypass, cross-user data exposure, approval bypass, command-policy bypass, unsafe browser/computer control, secret persistence, Project Brain privacy boundaries, or release/update integrity.

The public edition is still being separated from a private Enterprise codebase. A source file being present in the repository does not by itself mean that surface is enabled or supported in the public product. Please include the executable path or externally reachable behavior when reporting a finding.

## Preview safety boundary

CodeLocal currently uses application-level workspace checks, command policy and approval gates. Arbitrary terminal commands are **not yet contained by a true OS-level sandbox**. Browser Automation and Computer Use are optional capabilities and should be enabled only on machines/workspaces where that trust model is acceptable.

We will keep this limitation visible in `STATUS.md` until the native sandbox strategy is implemented and validated.
