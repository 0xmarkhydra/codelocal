# CodeLocal Cloud Workspace — Railway Deployment Profile

Status: **Required deployment constraint for `feat/cloud-workspace-runtime-rsr`**  
Date: **2026-08-29**  
Owner: **CodeLocal**

> Goal: keep CodeLocal deployable on Railway while preserving a future-proof sandbox/runtime architecture.

---

# 1. Decision

CodeLocal Cloud MUST remain deployable on Railway for the control-plane/application layer.

Railway is the default target for:

- CodeLocal API / MCP server;
- Web/UI services;
- authentication and workspace APIs;
- Runtime Router;
- Job Scheduler / queue consumers that do not require nested containers;
- Postgres / Redis or equivalent shared state;
- media presign/orchestration logic;
- runtime session metadata, quotas, leases and usage accounting;
- observability and control-plane workers.

OpenSandbox compute MUST NOT be coupled to running inside a Railway service.

Current Railway services can build from Dockerfiles and run containers, but they do not expose the privileged Docker daemon / nested-container environment required for a normal OpenSandbox Docker backend or for installing gVisor/Kata/Firecracker inside the service container.

Therefore the initial deployment topology is:

```text
                         RAILWAY
                 ┌─────────────────────┐
                 │ CodeLocal Web/API   │
                 │ MCP / Auth          │
                 │ Workspace Service   │
                 │ Runtime Router      │
                 │ Scheduler           │
                 │ Session/Lease Store │
                 │ Media orchestration │
                 └──────────┬──────────┘
                            │ secure provider API
                            ▼
                 EXTERNAL EXECUTION PLANE
                 ┌─────────────────────┐
                 │ OpenSandbox Server  │
                 │ Docker/Kubernetes   │
                 │ gVisor/Kata/etc.    │
                 └──────────┬──────────┘
                            │
                      isolated sandboxes
```

This is a deployment boundary, not a product boundary.

---

# 2. Architectural invariant

`RuntimeProvider` MUST make compute location replaceable.

```text
RuntimeProvider
├── LocalRuntimeProvider
├── OpenSandboxRuntimeProvider
├── future RailwayVMRuntimeProvider
├── future CodeLocalComputeProvider
└── future EnterpriseRuntimeProvider
```

No MCP handler, Workspace model, Agent router, media contract or task model may depend directly on Railway or OpenSandbox SDK types.

If Railway later exposes a suitable VM/privileged runtime, CodeLocal MAY add or retarget a provider without changing the upper architecture.

---

# 3. What Railway CAN own

## 3.1 Control plane

The following components SHOULD deploy as ordinary Railway services:

```text
codelocal-api
codelocal-web
runtime-router
job-scheduler
job-event-stream
quota-service
usage-meter
media-coordinator
```

They MAY initially remain in fewer deployable binaries/services if the current repository is monolithic. Logical boundaries do not require premature microservices.

## 3.2 Shared durable state

Use Railway-compatible managed/service primitives for:

- Postgres: workspace/task/execution/runtime session metadata;
- Redis or equivalent: queue, ephemeral locks, rate limits and event fanout where needed;
- existing object/media storage path for media and large output;
- Railway volumes only for service-local persistent needs that do not require horizontal replicas.

Do NOT use a Railway volume as the canonical multi-user Workspace store when that service needs replicas, because a service with a volume cannot be horizontally replicated against the same attached volume.

## 3.3 Horizontal scaling

Stateless CodeLocal API/router/scheduler processes SHOULD be horizontally scalable on Railway.

Any state required for correctness across replicas MUST be moved out of process memory:

```text
wrong:
workers[workspaceID] only in API process memory

correct:
runtime_sessions + distributed lease/shared store
```

---

# 4. What Railway MUST NOT be assumed to provide

The RSR MUST NOT assume an ordinary Railway service provides:

- Docker daemon access;
- `/var/run/docker.sock`;
- Docker-in-Docker;
- privileged containers;
- host-level OCI runtime configuration;
- gVisor `runsc` installation/configuration;
- Kata Containers/KVM configuration;
- Firecracker/Kubernetes RuntimeClass control.

Those are execution-plane requirements and belong behind `RuntimeProvider`.

---

# 5. OpenSandbox deployment profile

For the first production implementation, OpenSandbox SHOULD run on infrastructure that explicitly supports its execution backend.

Acceptable starting profiles:

### Profile A — Docker host + gVisor

```text
Linux VM
├── Docker daemon
├── runsc / gVisor
├── OpenSandbox Server
└── sandbox containers
```

Recommended for early production if workload compatibility is sufficient.

### Profile B — Docker host + runc

Suitable only for controlled development/testing workloads where stronger multi-tenant isolation is not yet required.

### Profile C — Kubernetes + Kata/Firecracker

Future higher-scale/high-isolation profile.

Do not require Kubernetes for the MVP.

---

# 6. Railway ↔ execution-plane communication

CodeLocal on Railway communicates with the runtime provider over a narrow authenticated API.

Required properties:

- TLS;
- OpenSandbox API key or stronger provider credential;
- provider credential stored only in CodeLocal server secrets;
- tenant/user/workspace ownership validated by CodeLocal before provider calls;
- provider IDs never trusted directly from clients;
- request timeouts;
- idempotency keys;
- retry policy;
- health checks;
- audit metadata;
- no secret values in logs.

Preferred production network order:

1. private/VPN network between Railway and compute provider when practical;
2. otherwise public TLS endpoint + strict authentication/firewall/mTLS;
3. never expose unauthenticated OpenSandbox server publicly.

---

# 7. Build requirement

The CodeLocal repository MUST continue to produce Railway-deployable artifacts.

Minimum acceptance criteria:

```text
main/feature branch
   ↓
Railway Dockerfile/Railpack build
   ↓
CodeLocal server starts
   ↓
healthcheck passes
   ↓
connects to Postgres/shared state
   ↓
RuntimeRouter can route:
      local → existing path
      cloud → OpenSandbox provider endpoint
```

The application container MUST NOT require a local Docker daemon merely to boot.

OpenSandbox provider support must therefore be implemented as a remote provider client, not as an assumption that the CodeLocal API process can spawn Docker containers itself.

---

# 8. Deployment environments

## Development

```text
CodeLocal API: Railway dev environment
OpenSandbox: one external Linux VM or developer host
DB: Railway Postgres
Media: existing CodeLocal media backend
```

## Staging

```text
CodeLocal API/UI/Scheduler: Railway
Postgres/Redis: Railway
OpenSandbox pool: dedicated VM(s)
Isolation: gVisor where compatible
```

## Production v1

```text
Railway control plane replicas
        │
        ▼
shared durable DB/queue
        │
        ▼
Runtime Provider Gateway
        │
   ┌────┴─────┐
   ▼          ▼
CPU pool    video pool
OpenSandbox OpenSandbox
```

## Future

The execution plane may become:

```text
Railway VM runtime (if/when suitable)
CodeLocal-managed VM pool
Kubernetes
third-party sandbox provider
enterprise self-hosted cluster
```

No Workspace or Agent contract changes are allowed solely because the underlying provider changes.

---

# 9. Railway storage rules

Railway volumes are useful for single-service persistent state, but CodeLocal MUST NOT bind logical Workspace durability to one Railway service volume.

Canonical Workspace state must be recoverable independent of a specific process/container instance.

Large media output MUST continue through the existing CodeLocal media system and direct signed uploads where supported.

Do not proxy large MP4/ZIP/build artifacts through the main Railway API process unnecessarily.

---

# 10. Multi-user scaling on Railway

Railway scales the control plane, not one permanent Railway service per CodeLocal user.

Correct model:

```text
10,000 accounts
    ↓
N Railway API replicas
    ↓
shared scheduler/DB
    ↓
only active executions consume sandbox compute
```

Do NOT implement:

```text
1 CodeLocal user = 1 permanent Railway service
```

or:

```text
1 Workspace = 1 permanent Railway service
```

The compute plane is provisioned on demand through `RuntimeProvider`.

---

# 11. Failure independence

Railway control-plane availability and execution-plane availability must degrade independently.

Examples:

```text
OpenSandbox unavailable
→ Local Runtime remains usable
→ Cloud jobs queue/fail clearly
→ CodeLocal chat/control plane remains online
```

```text
one sandbox worker host dies
→ runtime session marked lost
→ job recovered/retried from durable execution state
→ Railway API remains healthy
```

```text
Railway API replica restarts
→ distributed session/lease state preserves ownership
→ no duplicate sandbox must be created solely because process memory was lost
```

---

# 12. Security requirement

Production multi-tenant Cloud execution SHOULD use stronger isolation than plain shared-host runc where feasible.

OpenSandbox supports secure runtime configuration such as gVisor and Kata, but those runtimes must be installed/configured on the execution host itself.

This reinforces the control-plane/execution-plane split.

---

# 13. Acceptance test: Railway-compatible MVP

The first Cloud MVP is accepted only when this works end-to-end:

```text
1. CodeLocal backend is deployed on Railway.
2. User sends a task with Runtime=Auto.
3. Local Runtime is unavailable.
4. RuntimeRouter selects OpenSandboxRuntimeProvider.
5. Railway service calls external OpenSandbox endpoint.
6. Sandbox is acquired for the user's workspace.
7. Command executes inside sandbox.
8. Output file is produced.
9. Existing CodeLocal media system publishes the output.
10. User receives the result in ChatGPT/Web.
11. Sandbox can be reused for the workspace session.
12. Idle lifecycle destroys/releases compute without losing Workspace identity.
```

No step may require the user to install Docker or keep a personal machine online.

---

# 14. Final deployment rule

> **Railway hosts CodeLocal. RuntimeProvider hosts execution.**

Today the provider may live on separate Docker/Kubernetes-capable infrastructure.
Tomorrow it may run on a Railway VM/runtime or CodeLocal's own compute pool.

The upper CodeLocal architecture must not care.
