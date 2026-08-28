# Database Migration Evidence: Tasks 6-9 and Partial Task 10

Date: 2026-07-26

## Scope

- PostgreSQL migrations `001` through `026`.
- Legacy `005` TEXT project/environment identifiers and canonical `010`
  provider registry data.
- Additive scoped identity, RuntimeIntent/audit, and signing-key metadata.
- Forward, idempotent rerun, mixed-version, rollback, and consistent snapshot
  recovery paths. WAL point-in-time recovery remains pending.

## Environment

- Docker server: `29.4.1`.
- Database image: `postgres:16`.
- Client: `psql 16.14`.
- The test used a uniquely named temporary container and removed only that
  container through its exit trap.

## Commands And Results

```text
$ bash database/postgresql/scripts/test-migrations.sh
[1/6] empty database forward and idempotent rerun
[2/6] 005/010 legacy shape mixed-version upgrade
[3/6] tenant-safe constraints and immutable facts
[4/6] non-destructive rollback rehearsal
[5/6] consistent snapshot restore
[6/6] migration verification complete on PostgreSQL 16
```

The script invokes the forward runner with `psql -X -v ON_ERROR_STOP=1` and
explicitly excludes every `*.rollback.sql` file.

```text
$ bash -n database/postgresql/scripts/migrate.sh database/postgresql/scripts/test-migrations.sh
exit 0

$ git diff --check -- .github/workflows/ci.yml README.md deploy/docker-compose/compose.yml database/postgresql openspec/changes/phase-1-trusted-entry-and-write-path
exit 0
```

## Verified Assertions

- A new database accepts all forward migrations in filename order, then accepts
  the same sequence a second time.
- A database stopped at `020` accepts representative `005` TEXT projects and
  environments plus a `010` runtime target and provider. The test also creates
  the old partial `workspaces` and `extensions` shapes before applying
  reconciled `021` and `022` through `026`.
- Legacy IDs and rows remain unchanged; default workspaces and hierarchy links
  are backfilled; `env_type` remains present and maps to `type`; the legacy
  extension `target_id` remains TEXT while `runtime_target_id` is added as UUID.
- `provider_registry.tenant_id` and `runtime_target_id` remain populated and
  authoritative; `target_id` is backfilled as a compatible UUID alias.
- A cross-tenant runtime target/workspace reference is rejected.
- RuntimeIntent, ExecutionPlan, Operation, and security audit rows can be
  committed as one deferred-constraint graph.
- RuntimeIntent update and security audit deletion are rejected by immutable
  triggers.
- No signing-key metadata column is named for private or secret key material.
- Running rollbacks `026` through `021` retains the sample RuntimeIntent,
  Operation, and security audit rows.
- A consistent logical snapshot was streamed through `pg_dump --format=custom`
  and `pg_restore` into a new database. The recovered subject-to-intent-to-plan-
  Operation-to-audit chain and correlation ID matched the source. This is the
  snapshot recovery rehearsal for this reproducible baseline. It is not a WAL
  point-in-time recovery test; task 10 remains open until timestamp-targeted WAL
  recovery is exercised on a production-shaped database.
