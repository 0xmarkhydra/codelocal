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
CODELOCAL_RELEASE_NOTIFY_SECRET=<random secret of at least 32 characters>
```

`MCP_AUTH_SECRET` remains required by CodeLocal Cloud and is also used as the server-side pepper when hashing signup OTPs. The plaintext OTP is never stored in Redis.

The signup verification request lives in Redis for 10 minutes. Five incorrect codes invalidate it.

## GitHub Actions secret

Create this Actions secret in `0xmarkhydra/codex-mcp`:

```text
CODELOCAL_RELEASE_NOTIFY_SECRET=<exact same value as Railway production>
```

Do not put `RESEND_API_KEY` in GitHub. GitHub only calls the guarded CodeLocal endpoint; the Resend key stays in the Cloud service.

## Release flow

```text
merge/push main
  -> Publish CodeLocal to npm
  -> codelocal-vX.Y.Z tag exists
  -> Notify CodeLocal release by email
  -> POST https://codelocal.cloud/internal/releases/notify
  -> CodeLocal reads registered emails from Postgres
  -> Resend /emails/batch in groups of <= 100
```

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
Settings -> Plugins -> Code -> Refresh
```

The email tells the user to restart CodeLocal after updating and verify that `codelocal --version` reports the announced release (or a newer one). `hash -r` is intentionally shown only for macOS/Linux shells.

If `CODELOCAL_RELEASE_NOTIFY_SECRET` has not been added to GitHub yet, the notification workflow logs a warning and skips mail without breaking the npm release.

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
