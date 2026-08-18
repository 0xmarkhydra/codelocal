# CodeLocal — Submission Data Handling Notes

This document is the reviewer-facing summary of the production privacy boundary. The public source of truth is `https://codelocal.cloud/privacy` after deployment.

## Intended local-only data

CodeLocal's paired runtime is the execution boundary for:

- raw source files and repository contents;
- `.env` files and project secrets;
- local credentials and device secrets;
- terminal execution context;
- one-time approval tokens;
- machine-specific runtime state and bindings.

These values must not be added to Cloud Project Brain records merely because they were observed during a task.

## Cloud-stored service data

CodeLocal Cloud may store:

- account identity and authentication records;
- paired-device/workspace routing metadata;
- sanitized Project Brain knowledge objects and revisions;
- provenance, confidence, lifecycle and project/repository identity metadata;
- verified Experience and portable-safe learned-skill metadata;
- consent/preferences for optional collective features;
- aggregate usage, health, canary and security/audit metadata needed to operate the service.

## Collective Intelligence

Collective features are separately gated and disabled by default. Eligible contributions must be structured, privacy-classified and de-identified. Raw cross-tenant source code, raw conversations and private Project Brain objects must not be exposed to another user.

## MCP response review

Before submission and every material public-tool change, inspect MCP responses for:

- OAuth/access tokens;
- device credential secrets;
- approval tokens;
- raw `.env` values;
- unnecessary personal data;
- debug stack traces or internal payloads;
- private internal identifiers with no user-facing value;
- local absolute paths when a workspace-relative path is sufficient.

## Retention and deletion

Durable account/Project Brain data is retained while needed to provide the service or until it is deleted/revoked or a deletion request is completed. Durable Project Brain data does not currently claim automatic expiry. Users can revoke local workspace/device access without deleting local files. Cloud deletion/export requests are handled through the public support contact until self-service account-wide deletion/export is implemented.

## Logging

Application and security logging should use correlation/request IDs and structured metadata instead of raw prompt/source contents. Sensitive values must be redacted before logging. Production infrastructure log retention must be documented and configured consistently with the public privacy page before submission.

## Reviewer data

The reviewer account and reviewer runtime must contain sample data only. No real customer data, production developer credentials or private repositories are permitted in the reviewer fixture.
