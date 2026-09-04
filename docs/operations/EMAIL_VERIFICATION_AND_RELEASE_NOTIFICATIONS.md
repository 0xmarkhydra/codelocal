# CodeLocal email verification and release notifications

CodeLocal uses Resend for two email flows:

1. Signup verification: a six-digit code is emailed before the user row/session is created.
2. npm release notification: after the npm release workflow succeeds, a second GitHub Actions workflow calls the production Cloud endpoint, which batches an update email to registered users.

## Resend setup

Use a dedicated sending subdomain such as `updates.codelocal.cloud` so sending reputation is isolated from the root domain.

In Resend:

1. Add `updates.codelocal.cloud` as a sending domain.
2. Add the SPF/DKIM DNS records shown by Resend to the DNS provider for `codelocal.cloud`.
3. Wait until the Resend domain status is `verified`.
4. Create a production API key with sending access.

Recommended sender:

```text
CodeLocal <noreply@updates.codelocal.cloud>
```

## Railway production variables

Set these only in the CodeLocal production service/environment. Never commit their values.

```text
RESEND_API_KEY=re_...
CODELOCAL_EMAIL_FROM=CodeLocal <noreply@updates.codelocal.cloud>
```

`CODELOCAL_RELEASE_NOTIFY_SECRET` may remain configured as a legacy/fallback bearer secret, but GitHub Actions no longer depends on it. Release notifications use GitHub Actions OIDC with a short-lived token scoped to the CodeLocal release workflow.

`MCP_AUTH_SECRET` remains required by CodeLocal Cloud and is also used as the server-side pepper when hashing signup OTPs. The plaintext OTP is never stored in Redis.

The signup verification request lives in Redis for 10 minutes. Five incorrect codes invalidate it.

## GitHub Actions authentication

No repository secret is required for release notification. `.github/workflows/release-email.yml` grants `id-token: write`, requests a GitHub OIDC token with audience `codelocal-release-notify`, and sends that short-lived token to CodeLocal Cloud.

CodeLocal Cloud verifies the GitHub OIDC signature plus issuer, audience, repository, workflow ref, event name, and token time window before accepting the notification. `RESEND_API_KEY` remains only in Railway production.

## Release flow

For a maintainer-triggered npm release, the canonical local entry point is:

```bash
npm run release:npm
```

Press **Enter**/choose `1` for production `latest`, or choose `2` for `beta`. The full local runbook is [npm Release Operations](./NPM_RELEASE.md). The automated GitHub path below uses an immutable two-stage transaction for provenance and release notifications.

The npm workflow is two-stage so `main` itself is not mutated just to reserve a release version:

```text
merge/push main
  -> CodeLocal CI must complete successfully for that exact main SHA
  -> prepare run checks out the CI-accepted SHA and chooses next version
  -> create immutable release commit
  -> tag release commit as codelocal-vX.Y.Z
  -> tag-dispatched publish run builds/publishes that exact commit
  -> release-email workflow observes successful publish workflow runs
       -> prepare run has no tag at head SHA: notify=false, exit success
       -> immutable tag run has codelocal-vX.Y.Z: validate tag/package/runtime versions
  -> POST https://codelocal.cloud/internal/releases/notify
  -> CodeLocal reads registered emails from Postgres
  -> Resend /emails/batch in groups of <= 100
```

This avoids both failure modes: the first-stage `main` run no longer fails merely because its SHA is intentionally untagged, and the second-stage immutable tag run is no longer skipped because it is not a normal `main` branch run.

Release email delivery is idempotent per version and batch. Redis stores completion markers, while each Resend batch also receives a deterministic idempotency key.

Each release email includes the complete user update flow:

```text
macOS / Linux
npm install -g codelocal@latest
hash -r
codelocal --version

Windows PowerShell
npm install -g codelocal@latest
codelocal --version

ChatGPT
Settings -> Plugins -> CodeLocal -> Refresh
```

The email tells the user to restart CodeLocal after updating and verify that `codelocal --version` reports the announced release (or a newer one). `hash -r` is intentionally shown only for macOS/Linux shells.

Because the workflow now uses OIDC, a missing GitHub repository secret can no longer silently skip release email delivery.

## Signup flow

```text
/signup
  -> validate email/password/referral
  -> create 6-digit OTP
  -> store only OTP hash + pending signup data in Redis (10 minutes)
  -> email OTP
  -> /signup/verify
  -> correct OTP
  -> create user
  -> create session
  -> redirect to dashboard
```

An email address that cannot receive the OTP cannot finish account creation.
