# CodeLocal Autonomous Project OS — Product Master Plan

Status: **Accepted product direction**
Date: **2026-09-05**
Owner: **CodeLocal**
Scope: Product model, user roles, autonomy, task/goal semantics, feedback loop, cross-project executive control, extensibility beyond software.

Related plans:
- `docs/plans/ui/EXECUTIVE_CODEX_COMPANY_OS_UX_MASTER_PLAN.md`
- `docs/plans/execution/AUTONOMOUS_PROJECT_OS_24H_IMPLEMENTATION_PLAN.md`
- `docs/plans/ui/PRODUCT_UI_MASTER_PLAN.md`
- `docs/plans/ui/NEURAL_CONTROL_PLANE_DASHBOARD_MASTER_PLAN.md`

---

# 0. Executive decision

CodeLocal is **one product with two ways to use it**:

1. **Direct AI Workspace** — familiar, fast, Codex-like chat/code experience for users who want to work directly with AI.
2. **Autonomous Company OS** — goal-first operation where the user acts mainly as customer/CEO, while Chief Agent plans, delegates, executes, reviews, tests and drives work until the requested result is actually complete.

The two modes share the same Project, Project Brain, Docs, Tasks, Agents, Skills, Verification, Code, Artifacts and execution infrastructure.

CodeLocal must never force a user to choose between being a developer and being a CEO. The same user can move between both behaviors inside one project and even inside one conversation.

Canonical product statement:

> **The user talks to CodeLocal like one capable person. Behind the chat, CodeLocal operates like a company.**

Canonical autonomy statement:

> **The user manages goals and major decisions. Chief Agent manages work, agents, tasks, skills, retries, verification and operations until the goal is achieved.**

Canonical product promise:

> **You set the goal. CodeLocal runs the company.**

---

# 1. Product principles

## 1.1 Chat is the universal control plane

Every important CodeLocal capability must be reachable through natural language.

Examples:

```text
"BIDDI hôm nay có gì cần tôi?"
"Làm payment mới cho BIDDI."
"Mở file callback payment rồi sửa timeout thành 10s."
"Project nào đang có nguy cơ trễ?"
"Tất cả project có feedback nào đáng lo?"
"Bài công nghệ nào hôm nay đáng đăng Facebook?"
"Làm video 30s từ bài này."
```

The UI may expose Work, Company, Project, Docs, Design, Feedback, Code or Media for inspection and direct control, but those are optional views. Chat remains the fastest path.

## 1.2 User is normally the customer, not the project manager

For autonomous work, the user should primarily:

1. state the desired outcome;
2. clarify business intent when needed;
3. approve or correct the proposed plan;
4. intervene only when a real decision cannot safely be made by the system.

The user should not need to manually:

- create every task;
- assign every agent;
- maintain task dependencies;
- decide retry strategy;
- babysit test/build failures;
- relay information between agents;
- track worktrees or runtime sessions;
- manually convert requirement changes into engineering tasks.

## 1.3 Direct coding remains first-class

CodeLocal must remain excellent as an AI coding tool.

A user may say:

```text
"Đọc auth service này."
"Fix bug này."
"Đổi đoạn này thôi, không tạo workflow lớn."
"Review diff hiện tại."
"Chạy test package này."
```

For such direct work, CodeLocal should behave like a familiar coding agent rather than forcing Goal/Epic ceremony.

## 1.4 Goal-first for substantial work

When a request is broad enough to affect product behavior, multiple components, business rules or several tasks, CodeLocal should treat it as a Goal rather than immediately modifying code.

Example:

```text
User: "Làm payment mới cho BIDDI, hỗ trợ MoMo và ZaloPay."

CodeLocal:
1. Understand current project and requirements.
2. Produce proposed flow and plan.
3. Ask the user to confirm material business decisions.
4. Lock the approved plan.
5. Let Chief Agent split the implementation into any number of tasks required.
6. Execute autonomously.
7. Tester verifies final acceptance criteria.
8. Goal becomes DONE only after Tester declares DONE.
```

## 1.5 Complexity belongs to CodeLocal

Technical concepts may exist internally but should not be required knowledge for normal users:

```text
sessionId
runId
taskId
worktree
checkpoint
lease
runtimeGeneration
model route
skill fingerprint
verification revision
```

Expose them only in advanced inspection surfaces.

---

# 2. User hierarchy and control model

CodeLocal supports three conceptual levels of control.

## 2.1 Global CEO level

The user owns multiple projects and wants one place to ask across all of them.

Examples:

```text
"Có gì cần tôi?"
"Hôm nay làm được gì?"
"Project nào đang trễ?"
"Chi phí hôm nay bao nhiêu?"
"Có feedback khách hàng nào đang tăng mạnh?"
"Ưu tiên BIDDI trong tuần này."
```

Global Executive Chat is authoritative for cross-project questions and commands.

## 2.2 Project Owner / Customer level

Inside a project such as BIDDI, the user focuses on product outcomes:

```text
"Làm recurring booking."
"Sửa onboarding cho dễ hiểu hơn."
"Payment phải hỗ trợ MoMo và ZaloPay."
```

Chief Agent handles planning and execution after user confirmation of the plan.

## 2.3 Direct operator / developer level

The same user may directly inspect or edit:

```text
"Mở payment.service.go."
"Sửa callback này."
"Review PR này."
```

No separate account, persona or hard mode switch is required.

---

# 3. Core hierarchy

The canonical work hierarchy is:

```text
Organization / Account
  -> Project
    -> Goal
      -> Plan
        -> Task Graph
          -> Task
            -> Attempt / Execution
              -> Artifact / Change Set
                -> Verification
                  -> Tester Verdict
```

Important distinctions:

- **Chat** is a conversation surface, not the source of truth of work.
- **Goal** represents the desired business/product outcome.
- **Plan** is the user-approved approach.
- **Task** is a unit of work created and managed by Chief Agent or humans.
- **Attempt** is an execution path for a task; a task may have multiple attempts/forks.
- **Artifact** is the produced result: code, document, design, video, post, deployment, etc.
- **Tester Verdict** is the final authority for DONE.

---

# 4. Definition of Done

A plan, task or goal is never DONE merely because the implementing agent says it finished.

Canonical lifecycle:

```text
PLANNED
  -> READY
  -> ASSIGNED
  -> RUNNING
  -> REVIEWING
  -> TESTING
  -> DONE
```

Additional states:

```text
BLOCKED
WAITING_HUMAN
WAITING_EXTERNAL
RETRYING
STALE
CANCELLED
FAILED
```

## 4.1 Tester is the completion authority

For software:

```text
implementation complete
  -> review
  -> build/test/integration verification
  -> acceptance criteria validation
  -> Tester verdict = DONE
```

For video:

```text
render complete
  -> duration/aspect/audio/caption/branding checks
  -> visual QA
  -> Tester verdict = DONE
```

For content:

```text
draft complete
  -> source/fact/brand review
  -> publish validation where relevant
  -> Tester verdict = DONE
```

The verifier/tester may be an AI agent, an automated test harness or a human depending on policy and risk.

---

# 5. Planning contract

For substantial product work, CodeLocal must not immediately mutate the project before intent is sufficiently clear.

The planning contract is:

```text
User Goal
  -> Project context resolution
  -> BA / Chief analysis
  -> Proposed outcome
  -> Proposed user flow / behavior
  -> Important assumptions
  -> Acceptance criteria
  -> High-level implementation plan
  -> User approves or corrects
  -> PlanApproved event
  -> Autonomous execution begins
```

The user is asked only about decisions that materially affect the outcome.

Do not ask the user how many tasks to create, which agent to assign, which worktree to use or which model to route unless they explicitly care.

---

# 6. Chief Agent

Chief Agent acts as the operational manager for a project.

Responsibilities:

```text
Understand Goal
Resolve Project Context
Prepare Plan
Request material clarification
Build Task Graph
Choose agents/capabilities
Assign work
Track dependencies
Manage concurrency
Handle retries
Change strategy/model/agent when needed
Detect stale work after requirement change
Request review/testing
Enforce project policy
Escalate only true human decisions
Close Goal after Tester DONE
```

Chief Agent has freedom to split work into as many tasks as required as long as the final goal and approved plan are preserved.

Chief Agent must not create meaningless busywork solely to make the system appear active.

---

# 7. AI Workforce

Agents are capability-bearing workers, not decorative personas.

Canonical agent definition:

```text
Agent
  id
  role
  capabilities
  tools
  skills
  policy
  runtime preferences
  model routing policy
  project knowledge scope
  performance evidence
```

Example software workforce:

```text
Chief
BA / Product
Architect
Backend
Frontend
Mobile
Reviewer
Tester / QA
DevOps
Security specialist
```

Example content workforce:

```text
Chief
Research
Trend analysis
Writer
Fact checker
Brand reviewer
Publisher
Analytics
```

Example video workforce:

```text
Chief
Research
Script
Storyboard
Design
Voice / TTS
Video editor
Caption / branding
Video QA
Publisher
```

Specialists may be activated temporarily when required by a task.

---

# 8. Project types and extensibility

CodeLocal must not hard-code Project OS around software only.

All project types reuse:

```text
Goal
Plan
Task Graph
Agent
Artifact
Verification
Feedback
Knowledge
Events
Policy
Needs You
```

Different project types supply different capability packs.

## 8.1 Software project

Primary artifacts:

```text
code
branch
build
test
deployment
technical docs
design
```

## 8.2 Video/content project

Primary artifacts:

```text
research
script
storyboard
image/video/audio
captions
brand assets
published post/video
analytics
```

## 8.3 Future project classes

Examples:

```text
Marketing
Research
Support
Sales operations
Internal operations
Agency/client delivery
```

The product architecture must allow these without creating separate products.

---

# 9. Project first-class objects

A Project may expose these first-class domains:

```text
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

Only domains relevant to that project should be visible.

## 9.1 Docs

Docs are first-class project objects, not merely files under Code.

Examples:

```text
PRD
requirements
business rules
meeting notes
technical specs
API specs
decisions
release notes
```

Storage may be:

1. durable CodeLocal Project Docs;
2. Git-backed documentation;
3. external integrations;
4. hybrid presentation unified by CodeLocal.

Users should not need to care where a document is physically stored.

## 9.2 Design

Design becomes a first-class project object.

Examples:

```text
user flows
wireframes
mockups
prototypes
component states
responsive behavior
design decisions
visual assets
```

Design must link to requirements, tasks, code and verification.

## 9.3 Knowledge

Knowledge includes Project Brain, learned knowledge, canonical decisions, skills and verified experience.

## 9.4 Code

Code covers repository, branch, diff, commits, tests, deployment and environments.

Technical internals stay progressively disclosed.

## 9.5 Media

Media is available for projects that produce content/video/audio/design assets.

---

# 10. Requirement changes are system events

When a BA or authorized user changes a requirement, the system must not rely on humans to manually relay the change.

Canonical flow:

```text
RequirementChanged
  -> create durable revision
  -> Project Brain update
  -> impact analysis
  -> identify affected Goals / Plans / Tasks / Tests / Designs / Code areas
  -> mark obsolete assumptions or tasks STALE
  -> Chief re-evaluates task graph
  -> pause/replan running work where necessary
  -> notify affected agents through durable events
  -> update acceptance criteria
  -> Tester uses the new criteria
```

This is a core architectural requirement, not a convenience feature.

---

# 11. Event-driven company model

Agents should not be modeled as ten chatbots talking to each other.

The canonical collaboration mechanism is durable events + task state.

Representative event vocabulary:

```text
GoalCreated
PlanProposed
PlanApproved
PlanChanged
TaskCreated
TaskAssigned
TaskStarted
TaskBlocked
TaskRetried
TaskCompleted
TaskStale
ArtifactProduced
CodeChanged
DesignChanged
RequirementChanged
ReviewRequested
ReviewPassed
ReviewFailed
TestRequested
TestPassed
TestFailed
TesterDone
DeploymentReady
DeploymentCompleted
FeedbackReceived
FeedbackClustered
FeedbackEscalated
FeedbackResolved
SkillCandidateCreated
SkillPromoted
GoalCompleted
HumanDecisionRequested
HumanDecisionResolved
```

Events are authoritative coordination signals and should be safe to replay/idempotently consume.

---

# 12. Feedback Widget and Customer Signal

Every applicable project may expose an embeddable CodeLocal Feedback Widget or SDK.

Customer feedback categories may include:

```text
Bug report
Idea
Feature request
Complaint
Usability problem
General feedback
```

Optional safe context may include:

```text
route/page
app version
browser/device
internal pseudonymous user identifier
timestamp
screenshot
safe diagnostics
```

Do not collect secrets or unrelated sensitive data merely for convenience.

## 12.1 Feedback ingestion flow

```text
Customer
  -> Feedback Widget
  -> FeedbackReceived
  -> classification
  -> deduplication / clustering
  -> impact and severity analysis
  -> triage
```

Technical defects with clear expected behavior may become tasks automatically.

Feature ideas/business requests should generally become a product signal and escalate to the user only when a real business decision is needed.

## 12.2 Feedback clustering

Example:

```text
43 reports
  -> one cluster: "MoMo payment hangs after confirmation"
  -> affected users: 37
  -> severity: High
  -> likely regression: payment callback
  -> create BUG-291
```

The system should not create 43 duplicate tasks.

## 12.3 Closed loop

```text
Feedback
  -> Task
  -> Fix
  -> Tester DONE
  -> Release
  -> FeedbackResolved
```

Where product policy allows, CodeLocal may notify the reporting customer that the issue has been resolved.

---

# 13. Global Executive Chat

Global Executive Chat is a first-class account-level surface.

It can query and command across all projects the user is authorized to access.

Examples:

```text
"Có gì cần tôi?"
"BIDDI đến đâu rồi?"
"Project nào đang có nguy cơ trễ?"
"Có feedback nào đáng lo?"
"Hôm nay toàn công ty làm được gì?"
"Ưu tiên BIDDI hơn CodeLocal trong tuần này."
"Tự xử lý tất cả những gì không cần tôi quyết định."
```

Cross-project mutation must resolve the target project unambiguously before execution.

If one target is clearly implied, attach automatically.
If several plausible targets exist, ask one short disambiguation question.

---

# 14. Needs You

`Needs You` is the canonical human escalation inbox.

The system should prefer **not** to notify the user for technical noise.

Normally do not escalate immediately for:

```text
test failure
build failure
merge conflict
model timeout
provider 429
agent crash
first execution failure
ordinary retry
staging deployment failure with recoverable cause
```

Chief should attempt bounded recovery first.

Typical escalation categories:

1. business ambiguity;
2. irreversible/high-risk action governed by policy;
3. money/legal/external commitment;
4. missing credential/permission;
5. repeated unresolved failure after sensible recovery;
6. meaningful product tradeoff;
7. explicit project policy requiring approval.

The ideal executive state is:

```text
Needs You: 0
Everything is operating normally.
```

---

# 15. Project policy and autonomy

Policy controls autonomy rather than hard-coded platform rules.

Example:

```text
ProjectPolicy
  productionBranch
  developmentBranch
  featureBranchPattern
  autoMergeDev
  autoMergeMain
  deployStagingPolicy
  deployProductionPolicy
  approvalPolicy
  environmentPermissions
  agentConcurrency
  budgetPolicy
  verificationPolicy
  feedbackAutomationPolicy
```

A project may allow agents to merge into `dev` and `main` automatically when policy permits.

Another project may require human approval for `main` or production.

CodeLocal must not globally hard-code one release policy for all projects.

---

# 16. Recovery and resilience

Failure flow:

```text
failure
  -> classify
  -> retry if safe
  -> adjust strategy
  -> adjust model/provider
  -> use different skill
  -> assign another agent
  -> create repair subtask
  -> restore/reconcile attempt if necessary
  -> only then WAITING_HUMAN when required
```

Prevent infinite loops using bounded budgets for:

```text
attempt count
wall-clock execution
model/token spend
tool calls
external API usage
```

---

# 17. Skills and learning

Agents may learn from successful verified work.

Canonical lifecycle:

```text
Verified successful execution
  -> extract reusable pattern
  -> Skill Candidate
  -> evaluate compatibility/evidence
  -> trusted promotion
  -> reuse
  -> stale when project context invalidates assumptions
```

Do not promote skills solely because an agent claims success.

Learning must remain project/tenant scoped according to existing security and Project Brain policy.

---

# 18. Attempt / fork model

A Task may have several isolated approaches:

```text
TASK-123
  -> Attempt A
  -> Attempt B
```

Each attempt can own its own execution/worktree/artifacts.

Reviewer/Tester compares evidence and selects the best result.

This also supports user instructions such as:

```text
"Bỏ hướng này, thử cách khác."
```

without destroying historical evidence.

---

# 19. Humans and teams

Human members and AI workers share one project work graph.

Human roles may include:

```text
Owner
Admin
Leader
BA
Developer
Designer
Viewer
```

AI workers are governed by capability and policy rather than pretending to be human accounts.

Authorization decisions should consider:

```text
Actor
Project
Capability
Resource
Environment
Risk
```

Example:

A QA Agent may read staging logs and run tests but may not delete a production database.

---

# 20. Product UX north star

CodeLocal should feel different by depth, not by product fragmentation.

```text
Level 1 — Executive
  Global Chat
  Needs You
  Project Health

Level 2 — Management
  Goal
  Plan
  Work
  Company
  Feedback
  Docs / Design

Level 3 — Technical
  Code
  Diff
  Tests
  Branch
  Worktree
  Runtime
  Agent execution details
```

A very lazy founder may live entirely at Level 1.
A leader/BA may work mostly at Levels 1–2.
A developer may use Levels 2–3.

---

# 21. Non-goals

Do not turn CodeLocal into:

- a Jira clone;
- a Linear clone with AI avatars;
- a collection of fake agent chatrooms;
- a developer-only control panel;
- a dashboard that forces users to understand worktrees/sessions/models;
- a fake autonomous system that invents activity when idle;
- a software-only project engine;
- a system where agent self-report equals DONE.

---

# 22. Required invariants

The implementation must preserve these invariants:

1. Chat and Task are distinct.
2. A substantial Goal requires a user-approved Plan before autonomous implementation unless policy explicitly says otherwise.
3. Chief may split work freely after Plan approval.
4. Tester verdict is the final DONE authority.
5. Requirement changes propagate via durable events and impact analysis.
6. User is escalated only for decisions that actually need the user or project policy requires.
7. Direct coding remains available at all times.
8. Project policy governs merge/deploy autonomy.
9. Cross-project access remains tenant and authorization scoped.
10. Agents communicate through durable work/event state, not role-play conversations.
11. Feedback is deduplicated/clustered before becoming work.
12. Project type changes capability packs, not the core Task OS.
13. UI must remain understandable without exposing infrastructure internals.

---

# 23. Success criteria

A successful CodeLocal user should be able to say:

```text
"Tôi chỉ cần nói mình muốn gì."
"CodeLocal lên plan để tôi chốt."
"Sau đó nó tự chia việc và làm tiếp."
"Tôi biết cái gì thực sự cần tôi quyết định."
"Nếu muốn tôi vẫn có thể vào code trực tiếp như Codex."
"BA sửa requirement thì cả hệ thống tự biết."
"Khách hàng feedback thì hệ thống tự gom, phân tích và xử lý."
"Tôi có thể hỏi một ô chat về tất cả dự án."
"Task chỉ Done khi tester thực sự chốt Done."
```

That is the target product behavior.