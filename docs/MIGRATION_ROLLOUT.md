# CodeLocal database migration rollout

Status: source-ready; deployment proof required

CodeLocal supports three explicit database migration modes through `CODELOCAL_MIGRATION_MODE`.

| Mode | Behavior | Intended use |
|---|---|---|
| `startup` | Run schema migrations during Store initialization, then start the normal service. | Compatibility/default and rollback path. |
| `only` | Run migrations and schema maintenance, start no long-lived workers or HTTP server, then exit. | Railway one-shot migration job/service. |
| `external` | Do not execute database schema DDL during steady-state Store initialization. | Normal web service after the migration job has succeeded. |

An empty value is intentionally equivalent to `startup`. An unknown value fails closed and prevents the process from starting.

## Why the default remains `startup`

The source change must not make an existing production deployment fail simply because Railway has not yet been given a migration job. The transition is therefore staged. `startup` remains compatible with current deployments and the existing PostgreSQL advisory lock keeps migrations serialized while the migration job is being introduced.

## Railway rollout sequence

1. Deploy this source with the existing web service unchanged. Leave `CODELOCAL_MIGRATION_MODE` unset or set it to `startup`.
2. Create or reuse a one-shot Railway service/job from the **same release image/commit**. Give it the same `DATABASE_URL` and `REDIS_URL` and set `CODELOCAL_MIGRATION_MODE=only`.
3. Run the migration job. It must exit successfully and log `CodeLocal migration completed` before any steady-state service is switched to external migrations.
4. Change one canary web replica/service to `CODELOCAL_MIGRATION_MODE=external` and deploy the exact same release.
5. Verify health, login/session flows, OAuth, workspace listing and normal MCP traffic against the canary.
6. Promote `CODELOCAL_MIGRATION_MODE=external` to the remaining steady-state web replicas.
7. For every future release that contains schema changes, run the `only` job for that exact release before rolling the web service forward.

## Rollback

If the external-migration rollout has an application-startup/schema compatibility problem:

1. set the web service back to `CODELOCAL_MIGRATION_MODE=startup` (or remove the variable);
2. redeploy the same or previous compatible application release;
3. inspect `codelocal_schema_migrations` and application logs before retrying.

CodeLocal migrations remain serialized with the existing PostgreSQL advisory lock. The compatibility path is intentionally retained until the external migration workflow has been proven in production.

## Safety properties

- `only` does not start Store workers or the HTTP server;
- `external` skips `Store.Migrate` and canonical embedding schema DDL during normal startup;
- Redis/runtime initialization that is not database schema mutation still occurs normally;
- invalid migration mode values fail closed;
- the migration job and web release must use the same source/artifact revision;
- never run a newer migration job against a database while intentionally keeping an older incompatible web release active without checking backwards compatibility first.

## Removal gate for startup migrations

Do not remove the `startup` compatibility mode until all of these are true:

- production has completed at least two schema-changing releases through `only -> external` without falling back to startup migration;
- Railway migration job ownership and failure alerting are documented;
- deployment automation blocks web rollout when the migration job for that release fails;
- rollback procedures have been exercised at least once in staging/dev.

Until then, `startup` is a deliberate recovery path rather than accidental lifecycle coupling.
