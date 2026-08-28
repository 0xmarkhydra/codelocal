# Cloud Workspace Runtime — Implementation Notes

Status: **Active implementation notes**  
Date: **2026-08-29**  
Branch: **`feat/cloud-workspace-runtime-rsr`**

This file records implementation discoveries made while mapping the RSR onto the current codebase. If this note conflicts with an older conceptual diagram, this note controls implementation placement.

## 1. Important code-boundary correction

`internal/runtime.Runtime` is the execution-node runtime. It is the process that runs on a user's local machine today and connects back to the CodeLocal control plane over `/client`.

It is therefore **not** the correct place for cloud provider orchestration.

The existing runtime node should be reused inside future cloud sandboxes where practical:

```text
Local machine
  -> CodeLocal runtime node
  -> /client WebSocket
  -> Control Plane

Cloud sandbox
  -> CodeLocal runtime node
  -> /client WebSocket
  -> Control Plane
```

This lets cloud execution reuse the mature tool protocol, cancellation, idempotency, media transformation, knowledge synchronization and runtime capability advertisement already implemented by the runtime node.

## 2. Server-side provider boundary

Provider selection belongs on the server/control-plane side, near the existing gateway routing layer.

Current execution path:

```text
MCP / Dashboard
  -> WorkspaceService.Activate
  -> gateway.Hub.Call
  -> local or cross-replica runtime connection
  -> CodeLocal runtime node
```

Target path:

```text
MCP / Dashboard
  -> RuntimeRouter
  -> RuntimeProvider
       |- LocalRuntimeProvider
       |    -> existing WorkspaceService + Hub
       |
       `- OpenSandboxRuntimeProvider
            -> provision/wake sandbox
            -> start CodeLocal runtime node
            -> wait for /client registration
            -> route through existing Hub protocol
```

The first implementation is intentionally thin: `LocalRuntimeProvider` wraps existing behavior rather than rewriting it.

## 3. P1 implementation added

The branch now contains:

```text
internal/gateway/runtime_provider.go
internal/gateway/runtime_provider_test.go
```

The provider contract owns:

- provider identity (`local`, `cloud`, future kinds);
- runtime acquisition;
- execution calls;
- Auto-provider ordering;
- explicit provider override;
- safe fallback semantics.

Auto fallback is intentionally fail-closed:

> Only `ErrRuntimeProviderUnavailable` may cause the router to try another provider.

Authorization errors, invalid workspace errors, conflicts and other hard failures must not be hidden by silently executing on a different backend.

This protects against a dangerous future behavior such as:

```text
Local workspace is unauthorized/conflicted
  -> silently run stale Cloud copy
  -> overwrite user state
```

## 4. Compatibility requirement

Until OpenSandbox is registered, the provider set contains only Local Runtime. Therefore P1 must preserve current user-visible execution behavior.

No MCP tool names are duplicated into `local_*` / `cloud_*` variants.

No provider SDK object may be returned to MCP, Dashboard or Workspace product APIs.

## 5. Next implementation slice

After P1 CI is green:

1. wire `RuntimeRouter` into MCP and Dashboard hot paths while preserving the existing one-rebind retry behavior;
2. introduce OpenSandbox control-plane client behind `OpenSandboxRuntimeProvider`;
3. provision a sandbox with a CodeLocal runtime image;
4. register the sandbox runtime through the existing `/client` protocol;
5. add durable `RuntimeSession` ownership/lease records before enabling multi-replica cloud reuse;
6. keep sandbox destruction separate from workspace persistence.

## 6. Railway implication

Railway remains the CodeLocal control plane deployment target.

The runtime provider boundary means Railway does not need privileged Docker access. Railway can ask an external OpenSandbox control plane/worker pool to provision compute, while the resulting CodeLocal runtime node connects back to the normal CodeLocal gateway.

```text
Railway CodeLocal Control Plane
  -> OpenSandboxRuntimeProvider
  -> OpenSandbox server/worker infrastructure
  -> sandbox
  -> CodeLocal runtime node
  -> Railway /client gateway
```

This is provider-neutral: a future CodeLocalComputeProvider can replace OpenSandbox without changing MCP tools or durable Workspace identity.
