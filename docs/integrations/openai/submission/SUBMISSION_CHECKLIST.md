# CodeLocal — OpenAI Plugins Directory Submission Checklist

Status: implementation in progress  
Target: public MCP-backed plugin  
Production MCP URL: `https://codelocal.cloud/mcp`

This checklist maps the current CodeLocal repository to the OpenAI plugin submission form and review requirements.

## P0 — must be green before submission

- [x] Public MCP server architecture exists.
- [x] Compact public MCP surface is fixed at 19 tools.
- [x] Every public tool has explicit `readOnlyHint`, `destructiveHint`, and `openWorldHint` values.
- [x] Repository test locks the expected annotations for all 19 tools.
- [x] Public privacy page implemented at `/privacy`.
- [x] Public terms page implemented at `/terms`.
- [x] Public support page implemented at `/support`.
- [x] Public security page implemented at `/security`.
- [x] OpenAI domain verification endpoint implemented at `/.well-known/openai-apps-challenge`.
- [x] Reviewer sandbox binary and isolated sample repository implemented.
- [x] Dedicated reviewer Docker image definition implemented.
- [x] Five positive and three negative reviewer test cases documented.
- [x] Starter prompts documented.
- [x] Listing copy prepared.
- [x] Data-handling/reviewer explanation prepared.
- [x] Release notes prepared.
- [ ] Deploy current Cloud build to production.
- [ ] Set `OPENAI_APPS_CHALLENGE_TOKEN` to the exact portal challenge token during verification.
- [ ] Deploy reviewer sandbox as an always-on service using `deploy/docker/reviewer.Dockerfile`.
- [ ] Create a dedicated reviewer user/account with sample data only.
- [ ] Pair reviewer sandbox credentials to that reviewer account and inject them as deployment secrets.
- [ ] Confirm reviewer login requires no MFA, SMS, inaccessible email confirmation, VPN, or private network.
- [ ] Run clean-account end-to-end review flow from ChatGPT/Codex.
- [ ] Run OpenAI **Scan Tools** against production and resolve every validation error.
- [ ] Review MCP responses for auth secrets, approval tokens, debug payloads, unnecessary PII and internal-only identifiers.

## Publisher / Platform steps

These are external OpenAI Platform actions and cannot be completed from source code alone.

- [ ] Verify the individual or business identity that will publish CodeLocal.
- [ ] Ensure the submitting OpenAI organization has **Apps Management: Write**.
- [ ] Confirm public website/support/privacy/terms identity matches the verified publisher identity.
- [ ] Select the final portal category and country/region availability.

## Production URLs

- Website: `https://codelocal.cloud`
- MCP server: `https://codelocal.cloud/mcp`
- Privacy: `https://codelocal.cloud/privacy`
- Terms: `https://codelocal.cloud/terms`
- Support: `https://codelocal.cloud/support`
- Security: `https://codelocal.cloud/security`
- Domain challenge: `https://codelocal.cloud/.well-known/openai-apps-challenge`

## Release gate

Do not submit merely because the form can be filled in. Submission is ready only when:

1. production deployment and migrations are healthy;
2. reviewer sandbox stays online through a full test pass;
3. all eight reviewer cases behave as documented;
4. Scan Tools reports no blocking metadata/schema issues;
5. privacy/legal copy matches actual production behavior;
6. the npm/client release used by real users matches the production Cloud protocol and tool surface.
