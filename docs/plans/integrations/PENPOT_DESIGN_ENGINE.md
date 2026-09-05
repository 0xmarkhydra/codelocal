# Penpot Design Engine — CodeLocal

## Goal

Expose design as a first-class CodeLocal surface without cloning Figma. Penpot remains the design runtime; CodeLocal owns discovery, auth boundary, project context and the agent workflow.

## Product cases

1. **Hosted bridge (MVP fallback)** — Dashboard > Design opens Penpot Cloud through `CODELOCAL_DESIGN_URL`. This is immediately usable and requires no Penpot infrastructure.
2. **Self-hosted DEV (target)** — deploy Penpot frontend/backend/exporter/MCP plus Postgres and Valkey on Railway. Point `CODELOCAL_DESIGN_URL` at the Railway frontend domain.
3. **Agentic design (next)** — configure `CODELOCAL_PENPOT_MCP_URL` server-side, then let CodeLocal agents inspect/edit the active Penpot file and sync design tokens/components into project code.

## Why the dashboard does not iframe Penpot by default

Penpot frontend images send frame-protection headers. Cross-origin iframe embedding is therefore not a stable integration contract. The dashboard treats Penpot as a first-class external design surface and opens it in a dedicated tab. A future same-origin reverse proxy may enable an embedded experience after explicit security review.

## Railway DEV service map

- `penpot-frontend` — `penpotapp/frontend:2.16`
- `penpot-backend` — `penpotapp/backend:2.16`
- `penpot-exporter` — `penpotapp/exporter:2.16`
- `penpot-mcp` — `penpotapp/mcp:2.16`
- `penpot-postgres` — PostgreSQL 15 with persistent storage
- `penpot-valkey` — Valkey 8.1

Required production-grade properties: HTTPS public URI, persistent PostgreSQL + assets/object storage, a strong `PENPOT_SECRET_KEY`, private service networking and server-side-only MCP credentials.

## CodeLocal config

- `CODELOCAL_DESIGN_URL` — public Penpot URL shown by Dashboard > Design.
- `CODELOCAL_PENPOT_MCP_URL` — server-only MCP stream URL. Never render this value to the browser because it may contain a `userToken`.

## Done for MVP

- Add Dashboard > Design navigation and route title.
- Add responsive design workspace/launcher page.
- Default to Penpot Cloud so the page is usable even before self-hosting finishes.
- Add server-side config boundary for a self-hosted Penpot URL and MCP endpoint.

## Next integration

- Provision Railway DEV Penpot stack with persistence.
- Set `CODELOCAL_DESIGN_URL` to the generated HTTPS domain.
- Enable Penpot `enable-mcp enable-access-tokens` flags.
- Create a dedicated CodeLocal MCP connection flow with per-user credentials, revocation and audit events.
