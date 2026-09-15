# Dashboard Density & Navigation Refinement

Status: **Approved for implementation**  
Date: **2026-08-21**  
Scope: signed-in CodeLocal dashboard, starting with MCP Connections and the global shell.

## User feedback being solved

New users currently see too many navigation choices and too much explanatory copy at once. Large page headers consume valuable space, while secondary product areas compete visually with the core workflow. The previous redesign also removed an obvious install/download affordance.

The desired first impression is: **calm, compact, understandable in seconds**.

## 5-second product questions

The dashboard shell must make these answers obvious without documentation:

1. Where am I?
2. How do I connect an AI client?
3. Where are my workspaces and devices?
4. Where can I install CodeLocal?
5. Where do advanced intelligence/settings features live if I need them?

## Information architecture

### Always visible

- Home
- Connections
- Workspaces
- Devices

These are the primary operational journey and should remain visible without opening a menu.

### Progressive disclosure

**Intelligence**
- Knowledge
- Code Graph

**More**
- Usage
- Invite
- User network (admin only)

The active secondary group opens automatically. Inactive groups remain collapsed to reduce cognitive load.

### Account

Account, Security and Sign out move into the compact account control at the bottom of the sidebar. They do not need to compete with primary navigation.

### Install/download affordance

A persistent download/install icon remains beside the CodeLocal brand in the dashboard shell. It links to the public setup flow and uses a native tooltip/accessible label so the icon does not require permanent explanatory copy.

## Density rules

### Header budget

A standard control page header should normally contain only:

- page title;
- at most one short supporting sentence;
- compact contextual action(s).

Target desktop height: roughly 58–80 px before content begins. Avoid marketing-style hero headers inside operational pages.

### Copy budget

Above the fold, prefer:

- labels over paragraphs;
- one sentence over three;
- status badges over explanatory prose;
- tooltips/popovers for protocol/security detail.

If information is not needed to make the next decision, hide it behind an `i` affordance or a detailed guide.

### Progressive help

Technical explanations should be available through:

- hover title/tooltip;
- click/tap info disclosure;
- dedicated guide for longer instructions.

Help must remain keyboard accessible and must not require hover on touch devices.

## MCP Connections target state

The page has one primary job: provide the MCP endpoint and make the connection sequence understandable.

Primary content:

- `MCP server URL`
- endpoint
- `Copy URL`
- `OAuth ready` status

Secondary compact flow:

1. Add server
2. Choose OAuth
3. Sign in
4. Approve

OAuth/security internals remain available through info disclosure rather than permanent paragraphs.

## Visual rules

- Reduce sidebar width and vertical spacing.
- Use small consistent line icons to improve scanning.
- Keep active navigation restrained: subtle surface + thin accent indicator.
- Use fewer large cards and less decorative empty space.
- Reserve glow for active/trusted state.
- Keep typography compact on operational surfaces.
- Do not fake telemetry or activity to make empty space feel occupied.

## Responsive behavior

Desktop:
- compact fixed sidebar;
- four primary items visible;
- secondary groups collapsed;
- account control pinned to bottom.

Mobile:
- brand/download row first;
- primary routes remain horizontally accessible;
- secondary groups open as compact menus;
- account becomes a compact avatar menu;
- no ten-item horizontal navigation strip.

## Acceptance criteria

- A new user is not presented with more than four primary navigation choices at once.
- Install/download is visible from every signed-in dashboard route.
- MCP Connections header is materially shorter than the previous version.
- MCP Connections has no long explanatory paragraph above the endpoint.
- OAuth and connection-help detail is available on click/tap through an info control.
- Account/Security/Sign out remain discoverable without occupying primary navigation.
- Active advanced routes automatically reveal the corresponding collapsed group.
- Desktop and mobile navigation remain keyboard accessible.
- `npm run lint`, `npm run typecheck`, and `npm run build` pass before merge.

## Product rule

**Show the action and current state first. Explain the system only when the user asks for the explanation.**
