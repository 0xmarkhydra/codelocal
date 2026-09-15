# CodeLocal Product UI — Master Redesign Plan

Status: **Approved direction / implementation blueprint**  
Date: **2026-08-19**  
Owner: **CodeLocal**  
Scope: **All user-facing CodeLocal surfaces**  
Related: `NEURAL_CONTROL_PLANE_DASHBOARD_MASTER_PLAN.md`, `DESIGN_RECIPE_LIBRARY_MASTER_PLAN.md`, `PROJECT_BRAIN_MASTER_PLAN.md`, `UNIVERSAL_AGENT_RUNTIME_PLAN.md`

> Product goal: CodeLocal must stop looking like a collection of individually styled web pages and become one recognizable product system: a cinematic technical environment built around real project intelligence, while forms, security and operational controls remain fast, clear and trustworthy.
>
> Inspiration may come from high-craft scene-first products such as GetLayers, but CodeLocal must not clone layouts, proprietary assets, shaders, prompts or paid components. We adopt principles: composition before cards, one focal point per screen, depth, atmosphere, deliberate motion and real data-driven visual behavior.

---

# 0. Executive decision

Adopt one product-wide design language:

**CodeLocal // Living Intelligence System**

The experience should feel like:

```text
Cinematic product shell
        +
Real Project Brain / Code Graph state
        +
Calm operational controls
        +
Meaningful motion
        +
Strong typography and composition
```

Not:

```text
Generic SaaS dashboard
+ many cards
+ random gradients
+ fake particles
+ unrelated page themes
```

The redesign applies to the entire project, but **visual intensity is intentionally different by surface**.

```text
Landing / Product story              90% cinematic
Knowledge / Project Brain            85% cinematic
Code Graph                           80% cinematic
Overview / Mission Control           65% cinematic
Runtime / Projects / Skills          50% cinematic
Devices / Workspaces / Connections   35% cinematic
Account / Security / Auth            15% cinematic
Legal / Support                      10% cinematic
CLI / approvals                       5% cinematic, 95% clarity
```

This is not inconsistency. It is one system with different information-density modes.

---

# 1. Core design principles

## 1.1 Scene first, cards second

Before creating cards, define the visual scene:

```text
viewport
 -> focal point
 -> visual hierarchy
 -> depth layers
 -> primary action
 -> supporting information
 -> motion behavior
```

Cards are containers only when they improve comprehension. They are not the default page architecture.

## 1.2 One screen, one main character

Every major page must have one obvious focal point.

Examples:

```text
Landing       -> CodeLocal architecture / living system scene
Overview      -> current system state + live project intelligence
Knowledge     -> Project Brain graph
Code Graph    -> source relationship graph
Devices       -> device trust and runtime state
Workspaces    -> authorized project topology
Connect       -> AI client -> MCP -> CodeLocal -> machine flow
Account       -> identity and security status
```

Metrics and diagnostics support the focal point instead of competing with it.

## 1.3 Motion must mean something

Allowed motion:

- real runtime heartbeat;
- selected graph neighborhood highlighting;
- edge pulse for known relationships/events;
- active workspace transition;
- verified experience promotion;
- learned-skill status change;
- connection establishment;
- loading/reveal choreography;
- pointer/scroll depth that does not imply fake system activity.

Forbidden motion:

- fake AI thinking;
- random tool execution;
- fake network traffic;
- meaningless particle storms;
- constant animation that consumes CPU while idle;
- motion that makes security actions harder to understand.

## 1.4 Atmosphere, not decoration

Depth is created through controlled layers:

```text
near-black base
 -> low-frequency ambient haze
 -> structural grid / field
 -> real graph / scene
 -> content plane
 -> inspector / action plane
```

Use glow only for meaning: active, selected, trusted, live, conflict or verified.

## 1.5 Typography carries product identity

The hierarchy should do more work than borders.

Use:

- strong display typography for one key idea;
- compact technical labels for system state;
- monospace only for identifiers/code/protocol data;
- calm body copy;
- short product language;
- fewer explanatory paragraphs above the fold.

Avoid pages where every card has the same visual weight.

## 1.6 Real data is the visual asset

CodeLocal has an advantage over decorative scene libraries: its graph, relationships, runtime state, verification and learned experience are real.

Therefore:

```text
importance   -> node scale
confidence   -> clarity / opacity
freshness    -> visual recency state
active       -> controlled pulse
conflict     -> warning emphasis
verified     -> trust signal
relationship -> real edge
runtime      -> real presence
```

Do not invent fake nodes or fake activity to fill space.

---

# 2. Product-wide visual system

## 2.1 Design tokens

Create one semantic token layer instead of page-specific color literals.

Required token families:

```text
Color
  canvas
  surface
  surface-raised
  surface-sunken
  line
  line-strong
  text-primary
  text-secondary
  text-tertiary
  accent
  accent-soft
  success
  warning
  danger
  project
  repository
  memory
  skill
  experience
  workspace
  device

Typography
  display-xl
  display-lg
  heading
  subheading
  body
  body-small
  label
  mono

Spacing
  4 / 8 / 12 / 16 / 20 / 24 / 32 / 48 / 64 / 96

Radius
  control
  panel
  stage
  pill

Motion
  instant
  fast
  normal
  slow
  cinematic

Elevation
  flat
  panel
  overlay
  modal
```

No new user-facing page should introduce arbitrary primary colors outside semantic roles.

## 2.2 Surface modes

Define three official surface modes:

### `scene`

For Landing, Knowledge, Code Graph, Overview hero.

- near-black canvas;
- ambient depth;
- sparse chrome;
- real visual focal point;
- wider layouts;
- high visual hierarchy.

### `control`

For Projects, Runtime, Skills, Experiences, Devices, Workspaces, MCP Connections.

- dark technical shell;
- restrained panels;
- dense but readable state;
- clear actions;
- modest motion.

### `trust`

For Auth, Account, Security, Pairing, Legal, Support.

- calm surfaces;
- maximum contrast/readability;
- minimal animation;
- no distracting graph background;
- explicit status and consequences.

All three modes share tokens, type, icons, language and interaction primitives.

---

# 3. Architecture cleanup before more visual work

Current UI has accumulated multiple visual layers and overrides across:

```text
internal/ui/page_shell.go
internal/ui/product_pages.go
internal/ui/neural_graph.go
internal/cloudserver/dashboard_handler.go
internal/cloudserver/knowledge_dashboard.go
internal/cloudserver/code_graph_dashboard.go
internal/cloudserver/public_pages.go
internal/webauth/*
```

The redesign must not continue by appending more CSS overrides forever.

Incrementally split responsibilities:

```text
internal/ui/
  tokens.go
  icons.go
  shell.go
  components.go
  forms.go
  scenes.go
  graph.go
  motion.go
  auth.go
  public.go
```

Do not perform a giant rewrite first. Extract each piece as it is touched.

## 3.1 Canonical styling order

Target:

```text
base reset
 -> semantic tokens
 -> primitives
 -> shell mode
 -> component variants
 -> page-specific layout
 -> responsive rules
```

Remove the current pattern of:

```text
old style
 -> redesign override
 -> de-vibe override
 -> readability override
 -> dark override
```

when migration reaches a file.

## 3.2 Preserve server-rendered architecture

Do not rewrite CodeLocal into React/Next merely for visual polish.

Keep:

- Go SSR for shell and core content;
- Canvas2D for current graph scale;
- focused JavaScript for interaction;
- existing auth/CSRF/security architecture;
- existing route stability;
- existing local/cloud privacy boundary.

Introduce WebGL/Three.js only for a scene where it creates clear product value and performance remains acceptable.

---

# 4. Global shell redesign

## Goal

Every signed-in page must immediately look like the same CodeLocal product.

## Desktop shell

```text
+----------------+----------------------------------------------+
| CODELOCAL      | top context / project / system status        |
|                +----------------------------------------------+
| CORE NAV       |                                              |
|                |              PAGE FOCAL AREA                 |
| SYSTEM NAV     |                                              |
|                |                                              |
| COMMUNITY      |                                              |
|                +----------------------------------------------+
| account/status | compact contextual rail where appropriate    |
+----------------+----------------------------------------------+
```

Rules:

- sidebar should feel structural, not like a template component;
- active nav uses semantic illumination, not a giant filled pill;
- header height and hierarchy are consistent;
- page width depends on surface mode;
- graph pages can become near-edge-to-edge;
- management pages remain bounded for readability;
- contextual right rail only appears where it carries useful state.

## Mobile shell

- top bar with product identity;
- explicit menu button;
- no horizontal nav strip with ten items;
- graph controls collapse into compact tool sheet;
- inspector becomes bottom sheet / stacked panel;
- actions remain thumb-accessible;
- safe-area handling preserved;
- keyboard handling preserved on auth/forms.

---

# 5. Landing page — cinematic product story

Current goal: move from a technically correct landing page to a memorable product story.

## Hero composition

Primary concept:

```text
AI clients
    ↓ MCP
CodeLocal core
    ↓ controlled execution
Your machine / authorized project
    ↕
Project Brain / verification / experience
```

The hero should feel like a living architecture scene, not a screenshot of dashboard cards.

### Hero copy direction

Keep one hard product claim above the fold.

Example direction:

```text
CONNECT EVERY AI.
KEEP ONE PROJECT BRAIN.
```

Supporting copy explains local execution and model neutrality.

## Visual behavior

Allowed:

- subtle scene depth;
- pointer parallax;
- slow architecture orbit/rail movement;
- connection reveal;
- graph/knowledge nodes appearing as product story progresses;
- scroll-triggered transition from architecture to Project Brain;
- real product screenshots/visuals where useful.

Do not create an unrelated decorative 3D object just to look premium.

## Landing sections

Reduce generic feature-card grids.

Preferred sequence:

```text
Hero scene
 -> Why CodeLocal exists
 -> Project Brain scene
 -> Controlled local execution
 -> Real workflow
 -> Security boundary
 -> Setup
 -> CTA
```

Each section gets its own composition, not the same repeated card template.

---

# 6. Overview — Mission Control

Overview must answer in under five seconds:

```text
Is CodeLocal online?
What project/runtime is active?
What does CodeLocal currently know?
Is anything wrong?
Where should I go next?
```

## Replace metric-first hierarchy

Current metric cards become supporting status.

Primary structure:

```text
System state hero
Living Project Brain preview
Current project/runtime focus
Recent verified activity
Compact metrics
Recent workspaces
```

## Hero

Use real state:

- online runtime count;
- active project/workspace;
- MCP connected state;
- Brain health;
- client/runtime version warnings;
- stale tool-schema warning when relevant.

No fake "AI is working" state.

---

# 7. Knowledge / Project Brain — flagship visual surface

This should be one of CodeLocal's signature screens.

## Primary rule

The graph is the page.

Not:

```text
metrics
cards
cards
graph
cards
```

Instead:

```text
compact context header
compact health strip
large graph stage
inspector
optional collapsed diagnostics
```

## Visual semantics

```text
Project       violet
Repository    cyan/blue
Memory        emerald
Skill         amber
Experience    indigo/electric blue
Workspace     blue
Device        silver
Conflict      red
```

Node visual properties remain data-derived.

## Interaction

- search;
- kind filter;
- project focus;
- pan/zoom;
- select node;
- related-neighborhood emphasis;
- inspector;
- jump to Code Graph when repository/code relationship is relevant;
- collapsed diagnostics below graph;
- reduced-motion mode.

## Future realtime

When safe realtime telemetry exists:

```text
context retrieved
 -> selected Project Brain node pulses
 -> relationship path illuminates
 -> workspace activation pulses
 -> verification event reaches Experience node
```

Only real events may drive this.

---

# 8. Code Graph — living source architecture

Code Graph should feel different from Knowledge while sharing the same system.

Knowledge answers:

```text
What does CodeLocal know?
```

Code Graph answers:

```text
How is this checkout structurally connected right now?
```

## Layout

```text
context bar
  Project | Checkout | Repository | View
status / revision strip
impact strip when applicable
large graph stage
inspector
```

## Improvements

- prioritize canvas space;
- reduce chrome around selectors;
- selected causal path must be obvious;
- distinguish semantic vs fallback edges;
- display risk/impact without giant warning cards;
- keep revision/branch evidence accessible;
- retain bounded graph limits;
- preserve local-only source graph privacy.

## Motion

- semantic edges can have stronger signal;
- selected path can animate once on selection;
- event-driven pulses later;
- idle graph should settle and reduce CPU use.

---

# 9. Projects — new first-class product surface

Add a project-centric view because Project Brain is logical-project based, not folder based.

Routes:

```text
/dashboard/projects
/dashboard/projects/{projectId}
```

## Project list

Avoid CRUD table-first design.

Each project should communicate:

- identity;
- repositories;
- checkout count;
- machines;
- Brain freshness;
- skills;
- experiences;
- conflicts;
- last verified activity.

## Project detail

Use a focused mini-system view:

```text
Project core
 -> repositories
 -> workspaces
 -> devices
 -> memories
 -> skills
 -> experiences
```

The project becomes a small solar-system composition backed by real relationships.

---

# 10. Skills and Verified Experience

These are critical differentiators and should not look like database tables.

## Skills

Visualize trust lifecycle:

```text
candidate -> trusted -> portable
                 ↘ stale when context changes
```

Show evidence:

- success/failure counts;
- confidence;
- last use;
- project relationship;
- compatibility/fingerprint state;
- stale reason where safe.

## Experiences

Show the loop:

```text
Objective
 -> execution
 -> verification
 -> outcome
 -> reusable experience
```

The page should make it obvious that experience is **verified reusable knowledge**, not chat history.

---

# 11. Runtime / Devices / Workspaces

These pages need clarity more than spectacle.

## Runtime Mission Control

Create/retain a read-focused combined runtime view:

- online machines;
- runtime versions;
- protocol;
- active/sleeping workspaces;
- capabilities;
- MCP state;
- tool surface compatibility;
- recent safe activity.

## Devices

Primary visual focus: **trust relationship**.

Show:

- machine name;
- online/offline;
- last seen;
- paired date;
- credential trust state;
- security warning when risk exists;
- revoke action clearly separated.

Do not over-animate device management.

## Workspaces

Primary visual focus: **authorized project topology**.

Show:

- logical project relationship;
- device;
- active/sleeping/offline;
- last seen;
- Code Graph shortcut;
- remove access action.

Explain sleeping as efficiency, not failure.

---

# 12. MCP Connections — make the architecture understandable

This page is often the user's first operational hurdle.

It should visually explain:

```text
AI client
 -> OAuth / MCP
 -> CodeLocal Cloud
 -> paired runtime
 -> authorized project
```

## Requirements

- one primary MCP URL;
- copy action obvious;
- connection status visible;
- OAuth explained in plain language;
- compatibility guidance for ChatGPT/Codex/Claude/other MCP clients;
- stale-schema/reconnect guidance surfaced only when relevant;
- setup steps progressive, not wall-of-text;
- no user needs to understand internal gateway terminology.

A small architecture scene is appropriate here; a giant graph is not.

---

# 13. Auth / Signup / OTP / Password / Pairing

Use `trust` mode.

These flows must feel premium but extremely calm.

Pages:

```text
/login
/signup
/signup verification / OTP
/forgot-password
/reset-password
/change-password
pair approval
reauthentication / risky-session recovery
```

## Rules

- no full-screen particle system;
- no moving graph behind form fields;
- one short visual identity accent only;
- large readable inputs;
- keyboard-safe mobile behavior;
- clear progress state;
- rate-limit messages explain when user can retry;
- errors appear beside the relevant context;
- successful security events feel conclusive;
- reauthentication explains *why* without exposing risk internals.

## Signup flow visual model

```text
1 Account
2 Verify email
3 Pair machine
4 Authorize project
5 Connect AI
```

Do not show all five as a heavy wizard if a user only needs the current step; use subtle progress context.

---

# 14. Account / Security

The account page should become a trust dashboard, not a settings dump.

Sections:

```text
Identity
Password / recovery
Active browser security state
Paired devices
MCP authorizations where available
Collective Intelligence consent/preferences
Danger zone
```

Use clear language around session invalidation, password change and device revocation.

Security status should never use a fake numeric score unless the formula is deterministic and useful.

---

# 15. Invite / Admin

These remain secondary product surfaces and should inherit the system without competing with the core product.

## Invite

- one focal invite code/link;
- simple network visualization only when useful;
- privacy-safe referral list.

## Admin

Admin may be dense.

Use:

- compact operations layout;
- strong filters/search;
- system state tables where tables are actually appropriate;
- referral graph as drill-down;
- no unnecessary cinematic scene behind operational data.

---

# 16. Public Privacy / Security / Terms / Support

Current pages are visually separate from the main product.

Bring them into the same brand system using `trust` mode.

Requirements:

- shared logo/nav/footer;
- same typography and spacing tokens;
- calm dark or neutral surface;
- high readability;
- strong section anchors;
- support diagnostics snippets visually distinct;
- no marketing motion on legal content;
- print/readability must remain good.

These pages are trust assets, not secondary leftovers.

---

# 17. CLI / terminal UX

The product includes the local CLI, so the design system cannot stop at the browser.

CLI should share **language and state semantics**, not web decoration.

## Output hierarchy

```text
CodeLocal
status line
primary result
next action
technical detail when requested
```

Semantic terminal states:

```text
✓ success
● live/connected
○ sleeping/idle
! warning
× failure
→ next action
```

Rules:

- no ASCII-art overload;
- no huge banners every run;
- errors must say what happened + what to do;
- pairing flow shows current step;
- version mismatch shows update/reconnect distinction;
- approval prompts clearly name consequence;
- terminal colors respect NO_COLOR / non-TTY;
- machine-readable JSON modes remain stable.

Future native control indicator should reuse the same status language.

---

# 18. Empty, loading, error and compatibility states

A polished product is defined heavily by non-happy paths.

Create standard states for:

```text
no paired device
runtime offline
no workspace
workspace sleeping
no Project Brain data
Knowledge unavailable
Code Graph unsupported by old runtime
Code Graph query ambiguous
schema stale / reconnect needed
runtime update required
partial feature capability
rate limited
session reauthentication required
service unavailable
permission denied
```

Each state has:

```text
state icon/signal
short title
one-sentence explanation
one primary next action
optional technical detail
```

No page should expose a raw backend error as its main UX.

---

# 19. Motion system

Define motion levels.

## Level 0 — none

Auth inputs, destructive dialogs, legal, security confirmations.

## Level 1 — micro interaction

Buttons, focus, dropdowns, panel reveal, mobile sheet.

## Level 2 — product state

Runtime connection, status transition, selected graph path, health transition.

## Level 3 — cinematic composition

Landing scene, Project Brain entrance, Code Graph focus transition.

Rules:

- `prefers-reduced-motion` collapses Level 2/3 to opacity/state transitions;
- no essential information is animation-only;
- no idle infinite high-frequency loops;
- pause visual work when tab is hidden;
- avoid layout shifts;
- scene motion must be GPU/CPU budgeted.

---

# 20. Visual performance budget

Targets for normal laptop hardware:

```text
Dashboard idle CPU        near idle
Graph after settling      low continuous CPU
Input response            immediate
Navigation                no full-page jank
LCP marketing             prioritize text + core composition
Mobile graph              bounded and usable
Scene assets              lazy loaded below fold
```

Rules:

- prefer CSS/Canvas2D before WebGL for simple effects;
- use WebGL only for genuine scene complexity;
- no heavy scene on auth/legal pages;
- defer non-critical animation;
- stop graph physics after stabilization;
- cap particles/nodes/edges;
- support low-power/reduced-motion fallback.

---

# 21. Accessibility and trust requirements

Non-negotiable:

- WCAG-appropriate contrast for functional text;
- keyboard navigation;
- visible focus states;
- semantic HTML before canvas-only controls;
- graph inspector accessible independently of visual node color;
- color never the only status signal;
- reduced motion;
- mobile safe areas;
- 16px auth input text on mobile to avoid zoom issues;
- destructive actions retain explicit confirmation;
- no dark-pattern consent UI;
- collective intelligence remains opt-in where product policy requires.

---

# 22. Design asset / scene provenance

CodeLocal can learn design principles and use user-authorized recipes, but must not depend on scraping paid libraries.

Allowed:

- CodeLocal-original scenes;
- procedural scenes written for CodeLocal;
- permissive/open-source assets;
- user-provided licensed assets;
- generated assets with clear provenance;
- internal Design Recipe Library entries that comply with licensing rules.

For every imported design recipe/asset:

```text
source
license/provenance
redistribution status
runtime dependency
performance requirement
fallback behavior
```

No production page should depend on a third-party paid asset URL that CodeLocal does not control.

---

# 23. Implementation phases

## UI0 — Freeze + inventory

Target: documentation/tests only.

Tasks:

- inventory every route and UI renderer;
- snapshot critical pages before migration;
- document current functional behaviors;
- list existing CSS/theme overrides;
- identify legacy Node UI still reachable vs Go production UI;
- freeze auth/security/route contracts.

Acceptance:

- every user-facing route belongs to `scene`, `control` or `trust` mode;
- no route is omitted.

## UI1 — Design foundation

Target: shared UI package.

Tasks:

- semantic tokens;
- typography scale;
- spacing/radius/motion tokens;
- status colors;
- shared buttons/forms/badges/panels/dialogs;
- shell modes;
- standard empty/error states;
- remove duplicate overriding CSS as touched.

Acceptance:

- one component cannot look different merely because it appears on another route;
- existing tests remain green.

## UI2 — Global shell + navigation

Tasks:

- signed-in shell;
- desktop/tablet/mobile navigation;
- contextual header;
- account/status area;
- responsive behavior;
- active navigation semantics.

Acceptance:

- every dashboard route visibly belongs to one product.

## UI3 — Landing cinematic rebuild

Tasks:

- scene-first hero;
- architecture narrative;
- Project Brain section;
- local execution story;
- security section;
- setup progressive disclosure;
- scene performance fallback.

Acceptance:

- first viewport communicates differentiation without relying on feature cards.

## UI4 — Overview Mission Control

Tasks:

- graph/state-first layout;
- current runtime/project focus;
- compatibility warnings;
- recent verified activity;
- compact metrics.

Acceptance:

- core system status understandable in <5 seconds.

## UI5 — Knowledge flagship

Tasks:

- graph stage;
- project focus;
- inspector refinement;
- diagnostic collapse;
- semantic node styling;
- graph-first responsive layout.

Acceptance:

- Knowledge looks like a signature CodeLocal feature, not a widget embedded in a dashboard.

## UI6 — Code Graph flagship

Tasks:

- context controls refinement;
- impact presentation;
- causal path selection;
- inspector refinement;
- revision evidence;
- mobile controls.

Acceptance:

- source relationships are easier to understand than in the current graph.

## UI7 — Projects / Skills / Experiences / Runtime

Tasks:

- implement project-first product IA;
- add missing routes incrementally;
- build trust lifecycle for skills;
- verified experience browser;
- runtime mission control.

Acceptance:

- Project Brain value is inspectable beyond one graph screen.

## UI8 — Operational surfaces

Migrate:

- Devices;
- Workspaces;
- MCP Connections;
- Account;
- Security;
- Invite;
- Admin.

Acceptance:

- functional density preserved;
- no spectacle harms management actions.

## UI9 — Auth / onboarding / pairing

Migrate all auth and sensitive flows to `trust` mode.

Acceptance:

- mobile keyboard flow remains correct;
- errors/rate limits/reauth are clearer than today;
- no security regression.

## UI10 — Public trust pages

Migrate:

- Privacy;
- Security;
- Terms;
- Support.

Acceptance:

- consistent brand, excellent readability, no distracting motion.

## UI11 — CLI language + status system

Tasks:

- terminal state vocabulary;
- pairing flow;
- version mismatch messages;
- reconnect/update guidance;
- approval presentation;
- machine-readable output compatibility.

Acceptance:

- browser and CLI describe the same system state with the same terminology.

## UI12 — Real activity pulse

Only after safe telemetry contracts are available.

Tasks:

- authenticated SSE;
- normalized safe events;
- graph edge/node pulse;
- live activity rail;
- reconnect/idempotency;
- old runtime fallback.

Acceptance:

- every live animation has real underlying evidence.

## UI13 — Final cinematic polish

Tasks:

- transitions;
- scene depth;
- section choreography;
- pointer/scroll refinement;
- low-power fallback;
- screenshot/visual QA;
- final removal of obsolete style overrides.

Acceptance:

- high visual craft without "template AI UI" feel;
- product remains fast and understandable.

---

# 24. Recommended implementation order

Ship in safe slices:

```text
1. UI foundation + shell
2. Landing
3. Overview
4. Knowledge
5. Code Graph
-------------------------- visual identity milestone
6. Projects / Skills / Experiences / Runtime
7. Devices / Workspaces / Connect / Account
8. Auth / pairing / security
9. Invite / admin
10. Privacy / Terms / Support
11. CLI consistency
-------------------------- full product consistency milestone
12. Real realtime activity pulse
13. Final cinematic polish
```

Do not bundle all phases into one giant `main` release.

Each production release should have a narrow visual scope and an easy rollback boundary.

---

# 25. Release strategy

Because the previous large main update combined many subsystems, UI migration must be intentionally safer.

Rules:

1. Separate UI-only changes from auth/runtime protocol changes.
2. Prefer one route family per release.
3. Keep old runtime compatibility.
4. Do not require npm update for pure Cloud UI changes.
5. Use capability gating for realtime features.
6. Keep route URLs stable.
7. Use feature flags for large scene/graph transitions where rollback risk is meaningful.
8. Preserve server-side fallback when advanced visual JS fails.

Suggested rollout:

```text
internal preview
 -> admin account
 -> small user cohort
 -> broader cohort
 -> 100%
```

---

# 26. Visual QA matrix

Every migrated surface must be checked at:

```text
Desktop 1440+
Laptop 1280
Tablet / narrow desktop
Mobile 390
Mobile 320-360
Light/reduced-motion accessibility where applicable
```

State matrix:

```text
empty
normal
large data
offline
sleeping
partial capability
error
rate limited
old runtime
stale MCP schema
admin
non-admin
long email/name/identifier
```

Graph matrix:

```text
0 nodes
small graph
medium graph
near max visible nodes
selected node
ambiguous symbol
low-confidence fallback edge
conflict
multiple repositories
multiple workspaces on same project
```

---

# 27. Technical verification

For shared UI changes:

```text
git diff --check
go test ./internal/ui/...
go test ./internal/cloudserver/...
```

When auth is touched:

```text
go test ./internal/webauth/...
go test ./internal/cloudserver/...
```

When Cloud DTO/data is touched:

```text
go test ./internal/cloud/...
go test ./...
```

When legacy TypeScript UI/build paths are touched:

```text
npm run typecheck
npm test
```

Visual verification must include real browser screenshots and interaction testing before a release is merged to `main`.

---

# 28. Success metrics

The redesign is successful when users can say these things without reading docs:

```text
"I can see what CodeLocal knows about my project."
"I can see which machine/workspace is active."
"I understand the difference between Project Brain and Code Graph."
"I know whether the system is healthy."
"I know what I need to do when it is offline or outdated."
"This does not look like a generic AI-generated dashboard."
```

Operational metrics to watch:

- dashboard navigation success;
- MCP connection completion;
- pairing completion;
- auth error/retry rate;
- stale-schema reconnect resolution;
- mobile abandonment;
- UI JS errors;
- graph interaction latency;
- page load performance;
- support tickets about confusing states.

---

# 29. Non-goals

This redesign does not mean:

- clone GetLayers;
- import proprietary paid scenes;
- turn every page into Three.js;
- rewrite Go SSR into React/Next;
- hide technical truth behind marketing animation;
- fake AI activity;
- weaken security confirmations;
- upload raw source to Cloud for visualization;
- force every user to update npm for web-only visual changes;
- make a dashboard screenshot look cool at the expense of real usability.

---

# 30. Final product rule

Before approving any new screen, ask:

```text
What is the main character of this screen?
Is the visual behavior backed by real state?
Can the user understand the next action immediately?
Does this look unmistakably like CodeLocal?
Would removing half the cards make it stronger?
```

If the answer to the last question is yes, remove them.

CodeLocal should feel cinematic because **its system is alive and visible**, not because decorative effects are layered over a conventional SaaS dashboard.
