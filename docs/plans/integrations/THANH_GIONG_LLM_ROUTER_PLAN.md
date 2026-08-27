# Thánh Gióng LLM Router

## Product decision

Dashboard chat exposes a user-selectable model picker while keeping `Auto` as the default.

Selectable models:

- `auto`
- `glm-5.3-flash`
- `qwen3.8-flash`
- `muse-spark-1.2-contributor-free`

The assistant identity remains **Thánh Gióng**. Model selection controls routing, not the product identity.

## Auto routing

For generic, non-sensitive chat, Auto prefers:

1. GLM-5.3-Flash via Empero
2. Qwen3.8-Flash via Empero
3. Muse Spark 1.2 via OpenCode Zen
4. Existing configured provider as a final compatible fallback

A user-selected model is tried first, then the remaining compatible fallbacks.

## Reliability

- Retry a provider once for transient network errors, 408, 429, 502, 503 and 504.
- Failed targets enter a 30-second cooldown.
- Never replay a stream on another provider after bytes have already been emitted to the user.
- Validate model-generated tool calls before executing them.

## Privacy boundary

Community providers must not receive:

- images or image metadata;
- workspace-bound prompts or workspace identifiers;
- obvious secrets/tokens/password material;
- CodeLocal tool results.

If a community model requests a CodeLocal tool, the tool may execute locally, but the continuation containing the tool result is routed only through a trusted/BYOK lane. This prevents workspace or device data from being forwarded to a community provider.

## UI contract

`GET /api/v1/dashboard/models` returns only the curated model list and `auto` as `default_model`.

The dashboard sends the selected model in the chat request payload. The existing workspace picker, media upload, tool-loop, access-mode and streaming UX remain unchanged.

## Verification

Required targeted checks:

- `go test ./internal/cloudserver`
- `npm run typecheck:web`
- `git diff --check`

Full-repository failures outside the dashboard/cloudserver scope should be reported separately rather than mixed into this change.
