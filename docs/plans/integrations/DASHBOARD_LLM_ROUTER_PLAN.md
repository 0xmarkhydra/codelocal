# Dashboard LLM Router

## Product decision

> Temporary bypass (2026-09-05): Pool is inert unless
> `CODELOCAL_AI_POOL_ENABLED=1`. Dashboard chat runs direct OpenCode Zen
> (`muse-spark-1.3-contributor-free` via `https://opencode.ai/zen/v1`).
> Re-enable Pool by setting `CODELOCAL_AI_POOL_ENABLED=1` with
> `CODELOCAL_AI_POOL_BASE_URL` + `CODELOCAL_AI_POOL_API_KEY`.
> While `CODELOCAL_LLM_PROVIDER=zen` with a Zen credential, the picker is
> pinned to `auto` + `muse-spark-1.3-contributor-free`.

Dashboard chat exposes a user-selectable model picker while keeping `Auto` as the default.

Built-in fallback models:

- `auto`
- `glm-5.3-flash`
- `qwen3.8-flash`
- `muse-spark-1.3-contributor-free`

The assistant identity remains **CodeLocal**. Model selection controls routing, not the product identity.

When CodeLocal Pool is configured, the backend discovers its complete active
canonical catalog from authenticated `GET /v1/models` and caches it for five
minutes. The dashboard does not apply the ShopAIKey/OpenRouter Top 20 cap to
Pool models. Without Pool, ShopAIKey discovery and the curated provider list
remain available. Media generation, audio, embedding, moderation,
transcription and reranking models are excluded because they do not implement
the dashboard chat contract.

## Auto routing

With CodeLocal Pool configured, Auto uses
`muse-spark-1.3-contributor-free` unless `CODELOCAL_AI_POOL_MODEL` contains a
different valid canonical model. Pool is the exclusive execution plane in this
mode, so provider credentials and provider-qualified IDs remain behind Pool.

Without Pool, ShopAIKey Auto prefers the configured
`CODELOCAL_SHOPAIKEY_MODEL` (default `qwen3.5-flash`). Without ShopAIKey,
generic, non-sensitive chat prefers:

1. GLM-5.3-Flash via Empero
2. Qwen3.8-Flash via Empero
3. Muse Spark 1.3 via OpenCode Zen
4. Existing configured provider as a final compatible fallback

A user-selected model is strict: CodeLocal retries that same model for a
transient failure, then asks the user to retry. It never switches a manually
selected request to another model. Only `auto` may use the fallback chain.

## Reliability

- Retry a provider once for transient network errors, 408, 425, 429, 500, 502, 503 and 504.
- Failed targets enter a 30-second cooldown.
- Never replay a stream on another provider after bytes have already been emitted to the user.
- Validate model-generated tool calls before executing them.

## Privacy boundary

Community providers must not receive:

- images or image metadata;
- workspace-bound prompts or workspace identifiers;
- obvious secrets/tokens/password material;
- CodeLocal tool results.

A deployment may explicitly opt in with `CODELOCAL_ALLOW_COMMUNITY_WORKSPACE=1`,
which permits workspace and image content on community lanes so a free model can
do project work; obvious secrets stay blocked.

If a community model requests a CodeLocal tool, the tool may execute locally, but the continuation containing the tool result is routed only through a trusted/BYOK lane. This prevents workspace or device data from being forwarded to a community provider.

## UI contract

`GET /api/v1/dashboard/models` returns the complete active CodeLocal Pool
catalog when Pool is configured, or the ShopAIKey/curated fallback catalog, and
`auto` as `default_model`.

The dashboard sends the selected model in the chat request payload. The existing workspace picker, media upload, tool-loop, access-mode and streaming UX remain unchanged.

## Verification

Required targeted checks:

- `go test ./internal/cloudserver`
- `npm run typecheck:web`
- `git diff --check`

Full-repository failures outside the dashboard/cloudserver scope should be reported separately rather than mixed into this change.
