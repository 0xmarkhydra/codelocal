# CodeLocal UX Master Plan — Executive Home + Codex Workspace + Company OS

Status: **Accepted UX direction**
Date: **2026-09-05**
Owner: **CodeLocal**

Depends on:
- `docs/plans/product/AUTONOMOUS_PROJECT_OS_MASTER_PLAN.md`
- `docs/plans/ui/PRODUCT_UI_MASTER_PLAN.md`
- `docs/plans/ui/NEURAL_CONTROL_PLANE_DASHBOARD_MASTER_PLAN.md`

Authority note: **this document supersedes the navigation/information-architecture assumptions in the older UI plans where they conflict.** The existing scene-first, cinematic, real-data, accessibility and meaningful-motion principles remain valid.

---

# 0. UX decision

CodeLocal must visibly communicate two truths at the same time:

1. **It is as easy to use directly as a modern AI coding workspace such as Codex.**
2. **It has a much larger autonomous operating system behind the chat.**

Do not merge both truths into one overloaded dashboard.

The UI is therefore organized as three depth levels:

```text
LEVEL 1 — EXECUTIVE
  Global Chat
  Needs You
  Company Pulse
  Project Health

LEVEL 2 — PROJECT / MANAGEMENT
  Chat
  Work
  Company
  Project domains: Docs / Design / Feedback / Knowledge / Code / Media

LEVEL 3 — TECHNICAL
  Diff
  Branch
  Worktree
  Tests
  Runtime
  Agent attempts
  Event log
  Verification evidence
```

The deeper the user goes, the more technical detail appears.

---

# 1. Primary UX principle

The user should feel:

> **“I am giving orders, not operating a management tool.”**

The largest, clearest interaction on the global home is always the chat input.

Do not make Tasks, charts, agents or telemetry visually dominate the first screen.

Canonical order of importance:

```text
1. Ask / command
2. Needs You
3. Company / project health
4. Recent outcomes
5. Detailed operations
```

---

# 2. Global Home — CEO surface

When the user has several projects, the signed-in default surface should be an account-level executive home.

Example desktop composition:

```text
┌───────────────────────────────────────────────────────────────┐
│ CodeLocal                                      ● All healthy  │
│                                                               │
│ Good morning.                                                  │
│ Your company is running.                                      │
│                                                               │
│            What do you want done?                             │
│                                                               │
│ ┌───────────────────────────────────────────────────────────┐ │
│ │ Ask anything across all projects...                     ↑ │ │
│ └───────────────────────────────────────────────────────────┘ │
│                                                               │
│ [What needs me?] [What got done?] [What's at risk?]          │
│                                                               │
├───────────────────────────────────────┬───────────────────────┤
│ COMPANY PULSE                         │ NEEDS YOU             │
│                                       │                       │
│ 4 Projects       12 AI Workers        │ 2 decisions           │
│ 7 Running        18 Done today        │                       │
│                                       │ BIDDI                 │
│ ● BIDDI       Running normally        │ Recurring booking     │
│ ● CodeLocal   Shipping release        │ [Review]              │
│ ○ EKIP AI     Healthy                 │                       │
│ ● Content     Publishing              │ CodeLocal             │
│                                       │ Production release    │
│                                       │ [Approve]             │
├───────────────────────────────────────┴───────────────────────┤
│ RECENT OUTCOMES                                               │
│ ✓ BIDDI payment regression fixed and verified                │
│ ✓ CodeLocal release passed Tester                            │
│ ✓ 2 customer feedback clusters resolved                      │
└───────────────────────────────────────────────────────────────┘
```

## 2.1 Home should not look like Jira

Do not lead with:

```text
issues
pull requests
commits
runtime metrics
device counts
token counts
```

Those may exist deeper in the product.

At CEO level, summarize outcomes and required decisions.

## 2.2 Dynamic executive prompt chips

Prompt chips should be derived from real state.

Examples:

```text
Có gì cần tôi?
Hôm nay làm được gì?
Có gì đang trễ?
Tự xử lý mọi thứ có thể.
Có feedback nào đáng lo?
Có nên release BIDDI?
```

Do not show irrelevant fixed suggestions forever.

---

# 3. Global navigation

Keep global navigation short.

Recommended global shell:

```text
Home
Projects
Needs You
Activity
```

Optional secondary areas based on account capability:

```text
Integrations
Automation
Settings
```

Do not make Code/Brain/Agents/Skills global top-level concepts unless the user explicitly enters a project or advanced system view.

---

# 4. Project switcher

Project context must always be visually obvious.

Header / sidebar control:

```text
[BIDDI ▾]
```

Dropdown:

```text
Projects

BIDDI        ● 3 working   1 needs you
CodeLocal    ● 5 working   0 needs you
EKIP AI      ○ Healthy
AI Content   ● 2 working   1 needs you

+ New project
```

Rules:

- switching projects should preserve the same mental model;
- project status is concise;
- user never needs to understand local workspace IDs;
- if a command explicitly names another project, Chat may route there without requiring manual switching.

---

# 5. Project default — Codex-like Workspace

Inside a project, default to Chat.

The experience should feel immediately familiar to users of modern coding agents.

Desktop layout:

```text
┌────────────────┬────────────────────────────────┬──────────────┐
│ BIDDI ▾        │                                │ Context      │
│                │        CHAT / WORKSPACE        │              │
│ + New thread   │                                │ TASK-182     │
│                │ You                            │ ● Running    │
│ Today          │ Làm payment flow mới           │              │
│ Payment V2 ●   │                                │ Backend AI   │
│ Login bug   ✓  │ CodeLocal                      │              │
│ Booking UX  !  │ Tôi đã lên plan...             │ Progress 65% │
│                │                                │              │
│ Yesterday      │ ✓ Plan approved                │ Tester wait  │
│ Refactor API   │ ● Backend working              │              │
│                │ ○ Tester waiting               │ View task    │
│                │                                │ View code    │
│                ├────────────────────────────────┤ Open company │
│                │ Ask CodeLocal...               │              │
└────────────────┴────────────────────────────────┴──────────────┘
```

## 5.1 Thread sidebar

Use a lightweight Codex-like thread history.

```text
+ New thread

Today
  Payment V2       ●
  Login bug        ✓
  Booking UX       !

Yesterday
  Refactor backend
```

Tiny status decoration is acceptable:

```text
● work active
✓ completed
! needs user
```

Do not turn the thread list into a task board.

## 5.2 Chat controls everything

From Project Chat the user may:

```text
ask questions
create goals
approve plans
inspect tasks
edit docs
request code changes
trigger direct coding
request design work
review feedback
request video/content output
change priorities
ask project status
```

The system infers intent rather than requiring a permanent mode selector.

Optional explicit modes may exist as advanced overrides, but `Auto` is the default.

---

# 6. Plan approval inside Chat

A broad request should become a rich plan object in the conversation.

Example:

```text
Payment Flow V2 — Proposed Plan

Goal
Support MoMo + ZaloPay safely across web/mobile.

User flow
User -> Backend -> Payment Gateway -> Webhook -> BIDDI

Scope
✓ MoMo integration
✓ ZaloPay integration
✓ Webhook verification
✓ transaction state
✓ monitoring / retry
✓ web/mobile UI

Acceptance criteria
...

Likely work
Backend / Mobile / QA / Release

[Approve plan & start]
[Edit plan]
[Ask a question]
```

The user approves outcome and flow, not task implementation details.

After approval:

```text
✓ Plan approved
● 3 agents working
○ Tester waiting

No action required from you.

[View Work] [Open Company]
```

---

# 7. Right Context Rail

The right rail is optional and collapsible.

It should show context relevant to the current conversation, not generic dashboard noise.

For active task:

```text
TASK-182
Fix payment callback

● Running

Agent
Backend Engineer

Current
Implement webhook

Progress
■■■■■■■░░ 72%

Verification
3 / 5 passed

Related
Docs 2
Files 5
Tasks 2

[Open in Work]
[Open Code]
```

For a direct coding thread it may instead show:

```text
Repository
Branch
Changed files
Tests
Current diff
```

For an executive question it may remain hidden.

---

# 8. Project navigation

Primary project navigation should stay small:

```text
Chat
Work
Company
Project
```

Plus the globally visible `Needs You` indicator.

This is intentionally simpler than exposing every domain in the sidebar.

---

# 9. Work — management surface

`Work` is the structured view of Goals, Plans and Tasks.

It may borrow useful interaction patterns from Linear, but must not become the product identity.

Recommended tabs:

```text
Board
List
Goals
```

Optional advanced filters:

```text
Cycles
Agent
Human assignee
Priority
Status
```

Board example:

```text
BACKLOG          RUNNING          TESTING          DONE

TASK-190         TASK-182         TASK-184         TASK-170
Mobile UI        Backend API      Payment QA       Login API
                 Backend Agent    Tester Agent
```

Rules:

- Chief can create/move/update tasks automatically;
- humans can intervene directly when desired;
- changing task state manually is reflected in the same canonical Task OS;
- Work is an inspection/control surface, not a requirement for autonomy.

---

# 10. Company — autonomous system visualized

`Company` is the signature “wow” view.

Do not put this full visualization into normal Chat.

Company should communicate that a real system is coordinating work.

Example:

```text
BIDDI / Company                             ● Running

Chief
Payment V2 is operating normally.
3 agents working. 0 decisions required.

                    CURRENT GOAL
                    Payment V2
                        │
                       Chief
                        │
              ┌─────────┴──────────┐
              │                    │
        Backend Agent        Mobile Agent
          ● Working            ● Working
              │                    │
              └─────────┬──────────┘
                        ↓
                    Tester
                    Waiting
                        ↓
                     Release
```

## 10.1 Company modes

Keep only three views:

```text
Overview
Workforce
Operations
```

### Overview

Goal, active workforce, dependency flow, Needs You and project health.

### Workforce

Show AI worker capabilities and evidence:

```text
Backend Engineer
Status: Working
Current: TASK-182
Successful tasks: 147
Quality evidence: 96%
Trusted skills: 23
Recent learning: payment webhook retry pattern
```

Do not invent a magic score without evidence.

### Operations

Advanced view:

```text
task dependencies
attempts
retries
scheduler decisions
events
runtime state
verification evidence
```

This is not the default Company screen.

## 10.2 Motion

Company motion must represent real state.

Allowed examples:

```text
TaskCompleted -> edge pulse to Tester
RequirementChanged -> affected task highlight
TestFailed -> task returns to implementing agent
DeploymentReady -> release node activates
```

No fake “AI thinking” animations.

---

# 11. Project — first-class project domains

`Project` is a container for the actual project assets and operating context.

Suggested nested navigation:

```text
Overview
Docs
Design
Feedback
Knowledge
Code
Media
Members
Environments
Integrations
Settings
```

Only relevant domains are shown.

## 11.1 Docs

Docs experience:

```text
┌──────────────────────┬────────────────────────────────────┐
│ Documents            │ Payment V2                         │
│                      │                                    │
│ Product Requirements │ ## Requirement                     │
│ Payment V2           │ ...                                │
│ Authentication       │                                    │
│ Booking Flow         │ Impact                             │
│ Decisions            │ 3 tasks affected                   │
│ API Reference        │ TASK-182 / TASK-201 / TASK-209    │
│                      │                                    │
│                      │ [Review affected work]             │
└──────────────────────┴────────────────────────────────────┘
```

A requirement edit should show impact after the system processes `RequirementChanged`.

## 11.2 Design

Design is first-class, not hidden in Code.

Views may include:

```text
Flows
Screens
Components
Prototypes
Assets
Decisions
```

Links:

```text
Requirement
  -> Design
  -> Task
  -> Code
  -> Tester visual check
```

## 11.3 Feedback

Feedback surface:

```text
Inbox
Clusters
Bugs
Feature Requests
Resolved
Insights
```

Cluster example:

```text
MoMo payment hangs after confirmation
43 reports · 37 affected users · High severity

Likely regression: payment callback
Related release: v2.14.1
Task: BUG-291

Status: Fix in testing
```

## 11.4 Knowledge

Nested sections:

```text
Brain
Skills
Verified Experience
Decisions
Sources
```

The existing graph can remain a flagship Brain view, but `Brain` should not be a top-level concept every user must understand.

## 11.5 Code

Founder-level default:

```text
Production    main    ✓ Healthy
Development   dev     ✓ Healthy

Active changes
TASK-182  feature/task-182-payment
TASK-201  feature/task-201-payment-ui
```

Deeper developer view:

```text
Files
Diff
Commits
Tests
Branches
Worktrees
Processes
Deployments
```

## 11.6 Media

For content/video projects:

```text
Videos
Images
Audio
Voice
Assets
Brand Kit
Exports
```

---

# 12. Needs You — human escalation inbox

Needs You is visible globally and inside project context.

The UI must make an important distinction:

```text
technical activity != human decision
```

Example:

```text
Needs You (2)

BIDDI
Recurring Booking
187 users requested this feature.
Chief recommends planning it.
[Review]

CodeLocal
Production Release
Tester passed. Project policy requires approval.
[Approve]
```

Do not put ordinary test/build/model failures here before autonomous recovery is exhausted.

## 12.1 Zero state

When there is nothing to decide:

```text
Nothing needs you.
Everything is operating normally.
```

This should feel like success, not an empty/error state.

---

# 13. Feedback Widget UX

Embed surface for customer-facing products should be lightweight.

Launcher:

```text
Feedback
```

Panel:

```text
How can we help?

[Report a bug]
[Suggest an idea]
[Request a feature]
[Something is confusing]

Message...
Attach screenshot

[Send]
```

Optional context disclosure must clearly explain what safe diagnostics are attached.

After submission:

```text
Received
We are reviewing your feedback.
```

If the project enables customer-facing status:

```text
Received -> Investigating -> In progress -> Resolved
```

---

# 14. Global Executive Chat behavior

Global Chat must feel effortless.

Example conversation:

```text
User:
Có gì cần tôi?

CodeLocal:
2 decisions need you.

1. BIDDI — Recurring booking
187 users requested it. I recommend planning it.

2. CodeLocal — Production release
Tester passed. Your release policy requires approval.

Everything else is being handled automatically.
```

Another example:

```text
User:
Project nào đang trễ?

CodeLocal:
EKIP AI is the only project at risk.
Two tasks are blocked by an external API.
Chief has already switched to a fallback integration.
No action is required from you yet.
```

The answer should emphasize outcomes and actions, not raw event dumps.

---

# 15. Multi-project UX

A user may have many projects.

Project cards show only executive information:

```text
BIDDI
● Operating normally
Payment V2 running
3 AI workers active
1 needs you

CodeLocal
● Healthy
Release passed Tester
0 needs you
```

Do not put dozens of infrastructure metrics on the cards.

---

# 16. Project type adaptive UI

The shell remains stable while project domains adapt.

## Software project

```text
Chat
Work
Company
Project
  Docs
  Design
  Feedback
  Knowledge
  Code
```

## Content project

```text
Chat
Work
Company
Project
  Docs
  Design
  Feedback
  Knowledge
  Media
  Brand
  Channels
```

## Video project

```text
Chat
Work
Company
Project
  Script/Docs
  Design
  Media
  Brand
  Knowledge
  Channels
```

The user should not feel they opened a different product.

---

# 17. Responsive behavior

## Desktop

- left project/thread rail;
- center Chat/Work/Company/Project content;
- optional right context rail;
- Company visualization can use wide canvas.

## Tablet

- collapsible thread rail;
- right context becomes drawer;
- Company visualization simplified.

## Mobile

Primary nav:

```text
Chat   Work   Company   More
```

Company becomes list/flow first rather than giant graph.

Example:

```text
BIDDI
● Running

4 working
2 waiting
0 needs you

Current Goal
Payment V2

Backend  ● TASK-182
Mobile   ● TASK-201
Tester   ○ Waiting
```

---

# 18. Visual language

The existing CodeLocal cinematic direction remains valid, but visual intensity differs by surface.

## Executive Home

- premium and calm;
- generous whitespace;
- chat is focal point;
- strong typography;
- minimal operational clutter;
- subtle company pulse.

## Chat / Work

- Codex-like clarity;
- restrained glow;
- fast/dense interaction;
- familiar code/chat ergonomics.

## Company / Brain

- higher visual depth;
- meaningful connections;
- event-driven animation;
- real system state as visual material.

## Trust / Settings / Permission

- minimal motion;
- high clarity;
- explicit consequences.

---

# 19. Empty and idle states

Do not fabricate activity.

Healthy idle project:

```text
No active work.
Everything is healthy.
```

Global company idle:

```text
Nothing needs you.
All projects are healthy.
```

New project:

```text
What do you want this project to achieve?
```

Then show real setup progress only when real processing occurs:

```text
Reading project
Building project context
Preparing initial knowledge
Ready
```

---

# 20. Accessibility and trust

Requirements:

- keyboard navigation;
- accessible labels for icon-only controls;
- visible focus;
- normal text contrast >= 4.5:1 where applicable;
- status is not represented by color alone;
- reduced motion;
- no essential information only in graph animation;
- safe context disclosure for feedback widget;
- explicit confirmation for high-risk actions according to project policy.

---

# 21. UX anti-patterns

Do not ship:

- sidebar with 15 first-class technical concepts;
- giant dashboard full of equal-weight metric cards;
- full Company graph inside every chat message;
- fake agent typing/activity;
- mandatory Founder/Developer mode switch;
- forcing task creation for every direct code edit;
- showing worktree/session/model details to normal users;
- making the user relay information between agents;
- surfacing recoverable technical failures as `Needs You` immediately.

---

# 22. 5-second comprehension tests

## Global Home

Within five seconds, a user should know:

```text
I can ask anything here.
How many projects are healthy/active.
Whether something needs me.
```

## Project Chat

Within five seconds:

```text
I am inside BIDDI.
I can talk/code like Codex.
This conversation may have work running behind it.
```

## Company

Within five seconds:

```text
What goal is being executed.
Which agents are currently working.
Whether I need to intervene.
```

## Feedback

Within five seconds:

```text
What users are reporting.
Which reports are duplicates/clusters.
What is already being fixed.
Which ideas need product decisions.
```

---

# 23. Final UX rule

Before approving a screen, ask:

```text
Can a lazy user complete the intended job by chatting?
If they do nothing, can the system continue safely?
Is Needs You limited to real decisions?
Can a developer still drill into code immediately?
Does this screen reveal power without forcing complexity?
```

If not, simplify it.