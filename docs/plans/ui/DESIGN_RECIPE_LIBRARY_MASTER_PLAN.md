# CodeLocal Design Recipe Library — Master Plan

Status: Proposed / durable product-development plan
Date: 2026-08-19
Owner: CodeLocal
Related: `PROJECT_BRAIN_MASTER_PLAN.md`, `UNIVERSAL_AGENT_RUNTIME_PLAN.md`, `NEURAL_CONTROL_PLANE_DASHBOARD_MASTER_PLAN.md`, `PRODUCT_UI_MASTER_PLAN.md`

> This document exists so the design-recipe idea is not lost across chats, branches, agents, or future refactors.
>
> The goal is not to clone or redistribute a proprietary design library. The goal is to let CodeLocal preserve, search, adapt, execute, verify, and improve design recipes that the user is legally allowed to use: user-authored recipes, user-provided prompts/specifications, open-source recipes, licensed assets, and CodeLocal-original recipes.

---

# 0. Product thesis

A high-quality design system for coding agents should not be reduced to a giant prompt saying "make it beautiful".

The useful reusable unit is a **design recipe**: a structured, executable specification that can describe the visual composition, style, motion, shaders, interaction, assets, environment assumptions, and verification criteria needed to reproduce a design reliably.

The working model is:

```text
User brief
   ↓
Recipe search / retrieval
   ↓
Composition + Style + Assets + Motion/Scene
   ↓
Adapt to target codebase
   ↓
Build / preview
   ↓
Visual + technical verification
   ↓
Iterate
   ↓
Persist state + verified recipe experience
```

The durable value belongs in CodeLocal rather than in one model session.

---

# 1. Why this idea matters

A generic design prompt usually leaves too much unspecified: layout hierarchy, spacing and typography, exact animation timing, 3D geometry, shader logic, post-processing, camera behavior, interaction equations, responsive behavior, required assets, framework-specific integration, and acceptance criteria.

By contrast, a detailed recipe may include exact values and executable behavior such as:

```text
framework/runtime version
geometry parameters
vertex + fragment shaders
postprocessing chain
camera constants
scroll and pointer equations
palette/font tokens
asset paths
animation timing
resize behavior
verification expectations
```

A recipe of this quality can often be implemented by different agents without depending on the original design service at execution time.

---

# 2. Core design model

CodeLocal should treat a page/scene as composable layers rather than one undifferentiated prompt.

```text
DesignRecipe
├── Brief
├── Composition
├── Style
├── Assets
├── Motion
├── Scene / 3D / Shader (optional)
├── Runtime constraints
├── Integration instructions
├── Verification criteria
└── Provenance + license metadata
```

## 2.1 Composition

The structural skeleton: hero arrangement, navigation placement, section hierarchy, card/grid geometry, media placement, responsive transformations, and loader/gate/reveal choreography.

The agent should choose a composition before falling back to a generic centered stack.

## 2.2 Style

Shared visual system: palette roles, typography system, spacing scale, radius/border treatment, shadows/glow, background language, visual density, and contrast rules.

One coherent style should flow through all sections unless the recipe explicitly defines surface variants.

## 2.3 Assets

May include images, video, GLB/3D models, textures, fonts, icons, procedural assets, and generated assets.

Asset records must carry provenance/license information and must never silently hotlink private or paid third-party buckets.

## 2.4 Motion / scene

Optional advanced layer: timeline/reveal choreography, scroll transforms, pointer interaction, particles, Three.js scenes, shaders, bloom/post-processing, camera motion, and WebGL/WebGPU runtime assumptions.

---

# 3. Recipe storage format

Initial local portable layout:

```text
.codelocal/design-recipes/
  tunnel/
    RECIPE.md
    recipe.json
    preview.png
    assets/
  neural-hero/
    RECIPE.md
    recipe.json
    preview.png
    assets/
```

A recipe should have a machine-readable manifest plus human-readable implementation instructions.

Example manifest shape:

```json
{
  "id": "tunnel",
  "version": 1,
  "title": "Procedural Tunnel",
  "kind": "scene",
  "stack": ["three.js", "webgl"],
  "roles": ["hero-background", "immersive-scene"],
  "styleTags": ["dark", "cyan", "violet", "technical"],
  "assets": [],
  "procedural": true,
  "license": {
    "source": "user-provided",
    "redistributable": false
  },
  "verification": {
    "requiresPreview": true,
    "requiresInteractionCheck": true
  }
}
```

The schema should evolve without forcing the full recipe body into every model context.

---

# 4. Retrieval flow

Do not strict-filter recipes only by tags.

Use a layered selection strategy:

```text
user language
  ↓
lexical + semantic retrieval
  ↓
metadata constraints
  ↓
preview/description judgment
  ↓
rank top candidates
```

Retrieval should understand intents such as "dark scary AI dashboard", "neural graph hero", "minimal Apple-like landing", "wormhole background", "3D scene with mouse steering", or "reuse this layout but with our brand".

The result should return a small candidate set with compact summaries rather than dumping every recipe into context.

---

# 5. Adaptation flow

Before applying a recipe, CodeLocal must inspect the target project.

```text
read project conventions
→ detect framework/runtime
→ detect existing design system
→ detect reusable components
→ choose integration target
→ materialize only required recipe parts
```

Target adapters should progressively support plain HTML/CSS/JS, React, Next.js, Vite, and other frontend stacks through portable source translation.

Rules:

1. Preserve the recipe's defining composition/motion unless the user asks to simplify.
2. Adapt implementation to the project's conventions rather than forcing a foreign scaffold.
3. Reuse existing project tokens/components when doing so does not destroy the intended visual identity.
4. Keep heavy binary assets local to the project/runtime; do not leave fragile third-party hotlinks.

---

# 6. Build state

Long builds drift unless the agent remembers what was selected.

Persist current design state, initially locally:

```text
.codelocal/design-state.json
```

Suggested fields:

```text
selectedRecipe
selectedComposition
selectedStyle
selectedPalette
selectedTypography
selectedAssets
completedSections
pendingSections
adaptationDecisions
verificationResults
```

The agent reads this state before modifying the next section and updates it after a verified step.

Later, sanitized durable decisions may be promoted into Project Brain when appropriate.

---

# 7. Preview and verification loop

A design recipe is not complete when code merely compiles.

Required loop:

```text
materialize
→ start preview/dev server
→ capture screenshot/scene
→ inspect console/runtime errors
→ compare against recipe criteria
→ inspect responsive states
→ inspect motion/interaction where applicable
→ iterate
```

Technical verification should cover typecheck/build, missing assets, imports, shader compilation, console errors, runtime performance, and responsive overflow.

Visual verification should cover composition hierarchy, coherent palette/type tokens, required reveal/motion behavior, scene/camera interaction, and protection against generic fallback UI replacing the defining design idea.

Later phases may use screenshot embeddings or visual-diff scoring, but deterministic checks remain first.

---

# 8. Relationship to Project Brain and Learned Skills

Design Recipes and Learned Skills overlap but are not identical.

```text
Design Recipe = reusable design artifact/specification
Learned Skill = verified procedure for performing a recurring task
Experience    = what happened during an actual build
```

Example:

```text
Recipe: "Procedural Tunnel"
Skill:  "Integrate a Three.js procedural hero into an existing Next.js page"
Experience: "Recipe X integrated into project Y; shader compile issue fixed by Z"
```

Project Brain should index recipe metadata/provenance and verified adaptation knowledge without storing unlicensed raw third-party content in canonical cloud knowledge.

---

# 9. Provenance, licensing, and safety boundary

This is non-negotiable.

CodeLocal must NOT become a mechanism for scraping, cloning, bypassing payment, or redistributing proprietary design libraries.

Allowed sources:

- original CodeLocal-created recipes;
- user-authored recipes;
- user-provided material the user is allowed to use;
- permissively licensed open-source material;
- properly licensed commercial material stored/used under its license;
- procedural recipes that contain no third-party asset dependency.

Every imported recipe should record source/provenance and redistribution status.

If provenance is unknown:

```text
usable locally with caution
≠ automatically redistributable
≠ eligible for public/team marketplace
```

Never strip license notices or provenance metadata during materialization.

---

# 10. Product surface

Potential future user experience:

```text
Design Library
├── Recipes
├── Compositions
├── Styles
├── Scenes
├── Motion
└── Assets
```

Agent-facing flow should remain compact:

```text
search recipes
→ inspect candidate
→ select/adapt
→ materialize
→ preview
→ verify
```

Do not explode the public MCP surface into dozens of tiny design tools. Prefer one compact domain capability or internal orchestration behind existing CodeLocal agent/context surfaces.

---

# 11. Implementation phases

## DR0 — Documentation + schema spike

- finalize recipe terminology;
- define `recipe.json` schema;
- define provenance/license fields;
- add one user-provided procedural example as a private/local test fixture only if licensing permits;
- keep all work local-first.

## DR1 — Local recipe discovery

- discover `.codelocal/design-recipes/**/recipe.json`;
- validate schema;
- index metadata;
- bounded lexical search;
- expose compact recipe summaries to agent context.

## DR2 — Semantic retrieval + Project Brain metadata

- semantic retrieval over sanitized recipe descriptions;
- project/team/global scopes where policy allows;
- provenance-aware ranking;
- no raw proprietary payload uploaded by default.

## DR3 — Composition / Style separation

- explicit composition records;
- palette/type/style tokens;
- reusable section roles;
- recipe inheritance/overrides;
- deterministic merge rules.

## DR4 — Materialization adapters

- HTML first;
- React/Vite;
- Next.js;
- asset copy/rewrite;
- framework-aware integration.

## DR5 — Preview + visual verification

- launch detected dev server;
- browser screenshot capture;
- runtime/console inspection;
- responsive checkpoints;
- interaction verification;
- visual acceptance rubric.

## DR6 — Iterative design agent

- read design state before each section;
- perform bounded edit/preview/verify cycles;
- stop on quality gate or repair budget;
- preserve user control over destructive/expensive actions.

## DR7 — Learned adaptation skills

- promote repeated verified adaptations into Skills;
- fingerprint stack/framework/context;
- invalidate stale skills when environment changes;
- keep recipe and skill semantics separate.

## DR8 — Optional curated/open library

Only after provenance/licensing controls are mature:

- CodeLocal-original recipes;
- open-source community recipes;
- explicit licenses;
- moderation/review;
- versioning and trust signals.

---

# 12. First concrete experiment

Use one fully procedural, user-provided design specification as the architecture test because it avoids binary-asset licensing complexity.

The experiment should prove:

```text
recipe import
→ schema extraction
→ local index
→ target-project detection
→ implementation
→ dev preview
→ interaction check
→ screenshot verification
→ saved design state
```

Success is not "the model generated something cool".

Success is:

1. a second agent can discover the same recipe;
2. it receives only bounded context;
3. it reproduces the defining design behavior reliably;
4. it adapts to a different target stack;
5. CodeLocal can verify the result;
6. the recipe remains reusable across conversations/models.

---

# 13. Strategic outcome

If implemented well, CodeLocal gains a durable **design intelligence layer**:

```text
Models come and go
Design recipes persist
Verified adaptation skills improve
Project style remains coherent
```

This complements the existing Project Brain thesis: the model is replaceable, while the user's accumulated project/design knowledge remains durable and reusable through CodeLocal.
