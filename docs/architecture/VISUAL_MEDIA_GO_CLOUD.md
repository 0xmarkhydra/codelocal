# CodeLocal Visual Media — Go Cloud

## Goal

Keep Computer Use screenshots off the CodeLocal WebSocket/MCP hot path while avoiding a separate MediaUpload service.

## Data flow

```text
CodeLocal runtime
  -> POST /api/client/media/presign (device credential + signed request; JSON metadata only)
CodeLocal Go Cloud
  -> HEAD private S3 object by tenant-scoped SHA-256 key
  -> return presigned PUT when object is missing
  -> return short-lived presigned GET
CodeLocal runtime
  -> PUT image bytes directly to S3-compatible object storage
MCP
  -> ResourceLink with the short-lived signed GET URL
```

The Go Cloud control plane never needs to receive the screenshot bytes on the normal path.

## Security and privacy

- Reuse CodeLocal device authentication and request signing; there is no independent media bearer token.
- Revoke/logout of a device immediately removes its ability to request new media grants.
- Object keys are tenant-scoped by a one-way user scope plus image SHA-256; identical images from different users are not deduplicated together.
- Bucket remains private.
- Signed URLs default to 180 seconds and are clamped to 60–300 seconds.
- Upload size defaults to 12 MiB and is clamped to 1–25 MiB.
- Only PNG, JPEG, WebP and GIF are accepted.
- Presign endpoints are rate-limited by device and IP.
- No per-screenshot database/audit row is written.
- Cleanup runs hourly and removes objects older than the retention window; default retention is 24 hours.
- Cleanup work is bounded to five S3 listing pages per pass to avoid large background spikes.

## Go Cloud environment

Preferred names:

```bash
CODELOCAL_MEDIA_S3_ENDPOINT=https://s3-hcm-r2.s3cloud.vn
CODELOCAL_MEDIA_S3_REGION=us-east-1
CODELOCAL_MEDIA_S3_BUCKET=<private-bucket>
CODELOCAL_MEDIA_S3_ACCESS_KEY_ID=<access-key>
CODELOCAL_MEDIA_S3_SECRET_ACCESS_KEY=<secret-key>
CODELOCAL_MEDIA_PREFIX=codelocal
CODELOCAL_MEDIA_URL_TTL_SECONDS=180
CODELOCAL_MEDIA_MAX_MB=12
CODELOCAL_MEDIA_RETENTION_SECONDS=86400
```

For migration from the MediaUpload demo, the Go server also accepts `S3_ENDPOINT`, `S3_REGION`, `S3_BUCKET`, `S3_ACCESS_KEY_ID`, and `S3_SECRET_ACCESS_KEY` as fallback names.

If all S3 variables are absent, visual media is disabled and existing base64 behavior remains compatible. If only part of the S3 configuration is present, server startup fails rather than running with an ambiguous/insecure media configuration.

## Runtime behavior

When Go visual media is configured:

1. Runtime computes SHA-256 locally.
2. Runtime asks Go Cloud for a media grant using the paired device credential and device request signature.
3. If the object already exists for that user/hash, runtime skips upload.
4. Otherwise runtime PUTs bytes directly to the presigned S3 URL.
5. Runtime removes `__mcpImage` from the tool result and emits `__mcpImageRef`.
6. MCP converts the ref into `ResourceLink`.

If the Go server explicitly reports `media_not_configured`, runtime preserves legacy base64 behavior for rollout compatibility. If media is configured but presign/upload fails, runtime fails closed by default. `CODELOCAL_MEDIA_BASE64_FALLBACK=1` is an explicit emergency compatibility override.

## Health

`GET /health` exposes only non-secret media status:

```json
{
  "visualMedia": {
    "configured": true,
    "urlTTLSeconds": 180,
    "retentionSeconds": 86400,
    "maxBytes": 12582912
  }
}
```

Bucket names, endpoints and credentials are not included in health output.
