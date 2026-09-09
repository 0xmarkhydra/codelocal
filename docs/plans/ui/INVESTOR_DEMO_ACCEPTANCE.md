# CodeLocal Demo Acceptance

Date: 2026-09-09
Status: source checks passed for the UI release; production acceptance remains open.

## Product decisions

- Primary audience for this release: developers using an AI client on their own project.
- Primary value: authorized local execution with reusable project knowledge and verifiable results.
- Demonstrate one real workflow before showcasing secondary surfaces.
- Supported interface languages: English, Vietnamese, Simplified Chinese, Hindi.
- Hindi is one supported language of India, not a label for all Indian languages.
- Keep user content, source code, names, paths and identifiers unchanged.
- Prefer saved language choice, then supported browser language, then English.
- Keep dashboard content headers removed. Place essential state and actions inside the working layout.
- Do not introduce a new wizard, route family or translation dependency for this release.
- Existing Next.js/Go architecture takes precedence over older Go SSR guidance in the UI master plan.

## Evidence and release blockers

| Priority | Item | Evidence | Acceptance |
| --- | --- | --- | --- |
| P0 | Production language selection | Public `/login` returned `lang="en"` for each supported preference, including explicit language request headers. This does not establish the deployed revision. | Confirm deployed Go and web revisions; changing language persists across refresh and navigation through the public Go gateway. |
| P0 | Go presentation locale transport | `web_frontend_proxy.go` removes all cookies; Next reads the language preference from a cookie. Source fix forwards only allowlisted locale values as `Accept-Language`. | Proxy test covers all four locales and invalid preferences; session cookies and Authorization remain stripped. Repeat against deployed gateway. |
| P0 | First successful task | UI/unit checks are not authenticated end-to-end proof. | Sign in, pair a machine, authorize a workspace, connect an AI client, execute one approved bounded task and inspect its real result. |
| P1 | Responsive and accessible operation | Source layout checks passed; browser visual/interaction acceptance not recorded. | Desktop and narrow mobile layouts work in all four locales, at 200% text enlargement, with keyboard focus and no clipped primary actions. |
| P1 | Translation quality | Dictionary completeness and placeholders checked; native-language review not recorded. | Native review of Chinese/Hindi critical flows; legal translations reviewed before claims of legal equivalence. |

## Screen-specific follow-up

These are scoped recommendations, not claims that implementation is complete.

| Surface | Decision | Acceptance |
| --- | --- | --- |
| Overview / Projects | Do not call workspace counts unique project counts without deduplication by logical project identity. | Labels match the API entities actually counted; multiple checkouts do not imply multiple logical projects. |
| Connect / Devices | Make the first missing setup step reachable from the empty state using existing routes. Current connection guide lists MCP steps but not runtime installation/pairing. | A new user can reach installation and pairing instructions without guessing where to go. |
| Connect | Treat static OAuth capability separately from verified client connection. | Never present a configuration badge as proof of a connected client or successful task. |
| Knowledge / Code Graph | Show graph purpose, selection details and actionable empty/offline states; do not add decorative activity. | User can distinguish stored project knowledge from current checkout structure. |
| Skills / Plugins | Keep saved-but-refresh-failed feedback explicit. | Mutation failure or stale state never looks like confirmed success. |
| Blog editors | Preserve drafts across language changes. Existing unload/link warning does not block same-document back/forward navigation. | Include back/forward draft-loss cases in acceptance before relying on the warning as complete protection. |
| Account / Security | Preserve explicit destructive confirmation and authorization. | Revoking access is never described as deleting local source files. |
| Public pages | Keep positioning, setup, trust and legal content readable. | Describe shipped behavior, not roadmap capabilities, as available now. |

## Demo sequence

1. Show a real authorized workspace and its connected machine.
2. Run one bounded task from an AI client with normal approval policy.
3. Show the execution result and verification evidence.
4. Show relevant project knowledge or code relationships.
5. Switch interface language without changing project content.

Use a dedicated demo project without credentials or customer data. Do not stage fake
telemetry or bypass approvals to make the demonstration look smoother.

## Deferred

- More languages, automatic user-content translation and locale-specific SEO URLs.
- New dashboard features, decorative animation and further global redesign.
- Analytics infrastructure solely for this demo. First record successful task completion,
  setup failure points and time to first verified result through observed pilot sessions.

Release verdict must distinguish source readiness from production acceptance.
An HTTP 200 response or a passing build alone is not production sign-off.
