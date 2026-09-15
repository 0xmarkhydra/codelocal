# Project Brain Evidence and Promotion

Project Brain is intended to preserve useful project knowledge without treating model output as truth. In the public edition this is a **single-user** continuity model: the same user's verified project knowledge can survive chat/client/device boundaries when Cloud continuity is available.

## What a verified Experience looks like

The executable public fixture is [`internal/projectbrain/testdata/verified_experience_reuse.json`](../../internal/projectbrain/testdata/verified_experience_reuse.json). It contains a workflow candidate, its verified outcome, evidence references and two reuse contexts representing different MCP clients and paired machines.

The fixture is exercised by `TestVerifiedExperienceReuseFixture` in `internal/projectbrain/experience_test.go`.

## Promotion gate

`internal/projectbrain.EvaluateExperience` does not promote a candidate merely because a model stated it. Promotion requires all of the following:

1. verification passed;
2. at least one required check exists and all required checks passed;
3. the security gate passed;
4. the verified outcome is regression-free;
5. evidence references are present;
6. verification references are present;
7. confidence is at least `0.8`;
8. failure-recovery experience additionally contains both a failure signature and recovery action.

A promoted item receives `status=promoted`, `trust=verified` and the reason code `verified_experience_promoted`. Failed gates are rejected or remain candidates with explicit reason codes.

## Secret exclusion and sanitization

The durable Cloud Experience path applies sanitization before recording verified execution evidence. `internal/cloud/experience_test.go` includes a regression test that supplies an objective containing an API-key-shaped value and a verification summary containing a bearer-token-shaped value, then asserts the secret material is absent after normalization.

Secrets, credentials, approval tokens, auth cookies and private keys are not eligible Project Brain knowledge. Security policy outranks learned behavior: a remembered workflow cannot grant itself new permissions or weaken current approval policy.

The pure `internal/projectbrain` promotion gate assumes it receives an already sanitized candidate. Cloud ingestion and the security layer are responsible for preventing secret-bearing execution evidence from becoming durable canonical knowledge.

## Cross-client / cross-machine reuse

The public fixture models this sequence:

```text
Client A on machine A
  -> task succeeds
  -> verification + security + regression gates pass
  -> experience is promoted as verified project knowledge
  -> durable Project Brain continuity makes it available for the same user/project
Client B on machine B
  -> same project/branch/trigger context
  -> promoted experience is selected and reused
```

Selection itself is intentionally independent of the client/device name; project/branch/trigger/trust determine whether the Experience is eligible. Cloud authentication, project identity and synchronization are separate transport/persistence layers.

The fixture proves the promotion-and-selection contract, not a live network round trip. Cloud rollout behavior has separate tests under `internal/cloudserver`, and the supported remote flow still depends on `codelocal.cloud` as described in [`SECURITY_AND_PRIVACY.md`](./SECURITY_AND_PRIVACY.md).

## What this does not claim

This evidence does not claim team learning or multi-person collective intelligence in the public edition. The public product is optimized for one person's Project Brain. Enterprise/team source may still be visible while the repository is being separated, but source presence is not a public product guarantee.
