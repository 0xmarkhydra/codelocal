# CodeLocal — Reviewer Sandbox Guide

## Goal

OpenAI reviewers should be able to evaluate the authenticated CodeLocal plugin without installing CodeLocal on a personal machine, pairing a private developer laptop, accessing a private repository, using MFA, or joining a private network.

CodeLocal therefore ships a dedicated reviewer runtime entrypoint and deterministic sample repository:

- binary entrypoint: `cmd/codelocal-reviewer`
- reviewer image: `deploy/docker/reviewer.Dockerfile`
- fixture: `cmd/codelocal-reviewer/fixture/`

## Deployment model

```text
OpenAI reviewer
      |
      | OAuth demo credentials
      v
CodeLocal Cloud / MCP
      |
      | paired reviewer device credential
      v
codelocal-reviewer service
      |
      v
isolated temporary fixture repo
```

The reviewer runtime creates a fresh temporary fixture on startup, initializes a local Git baseline when Git is available, authorizes only that fixture and connects to CodeLocal Cloud using a dedicated paired-device credential injected through deployment secrets.

## Required deployment secrets

Never commit these values:

```text
CODELOCAL_REVIEWER_SERVER_URL=https://codelocal.cloud
CODELOCAL_REVIEWER_CREDENTIAL_ID=<paired reviewer credential id>
CODELOCAL_REVIEWER_CREDENTIAL_SECRET=<paired reviewer secret>
CODELOCAL_REVIEWER_DEVICE_ID=codelocal-openai-reviewer
CODELOCAL_REVIEWER_DEVICE_NAME=CodeLocal OpenAI Reviewer Sandbox
CODELOCAL_REVIEWER_WORKSPACE_NAME=codelocal-reviewer-project
```

Recommended runtime settings:

```text
CODELOCAL_ALLOW_SHELL=1
CODELOCAL_APPROVAL_MODE=prompt
CODELOCAL_NETWORK=approval
```

## Reviewer account

Create one dedicated user solely for OpenAI review. The account should:

- contain no customer or production developer data;
- have the reviewer device paired to it;
- expose only the deterministic reviewer fixture;
- use a login/password that can be shared in the submission portal;
- not require MFA, SMS, inaccessible email confirmation, VPN or a private network;
- remain available for the entire review window.

Do not place reviewer passwords or OAuth credentials in this repository.

## Reset procedure

Restart the `codelocal-reviewer` service before a formal test pass. A restart creates a fresh fixture and Git baseline. Project Brain data for the reviewer account may persist intentionally for continuity tests; clear the reviewer project's Cloud knowledge separately if a completely fresh memory state is required.

## Pre-submission smoke

Run the eight cases in `TEST_CASES.md` from a clean reviewer login. Confirm:

1. the workspace appears without manual local pairing;
2. read/edit/Git/terminal verification works only inside the fixture;
3. path escape and force push remain blocked;
4. external write actions do not bypass approval;
5. no credentials or private developer data appear in MCP responses;
6. `npm test` can run inside the reviewer image;
7. the tool surface count/hash matches the production submission build.
