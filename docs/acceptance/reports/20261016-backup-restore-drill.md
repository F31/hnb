# Acceptance: PostgreSQL and identity-material backup/restore drill (2026-10-16)

- run_id: `backup-restore-drill-20261016`
- commit: `f56dfc5` (plus the working tree under review)
- report: `docs/acceptance/reports/20261016-backup-restore-drill.md`
- verifier: `docs/acceptance/reports/20261016-backup-restore-drill.sh` (uploaded to the host and run with sudo)

## Environment

- Host `43.142.101.136`; Ubuntu 26.04 LTS, kernel `7.0.0-30-generic`, amd64; existing single-node
  K3s `v1.36.5+k3s1`.
- PostgreSQL pod `postgresql-0` in `hnb-system`; superuser `hnb_admin`, password mounted at
  `/var/run/hnb-pg-admin/password` (SCRAM host auth). Databases: `hnb_platform`, `hnb_market`,
  `hnb_artifact`.
- Identity materials at `/var/lib/hnb-minimal-eval/identity/` (root, 0600).

## Scope

Logical PG backup/restore round-trip and identity-material backup integrity. The drill is
non-destructive: it restores into a scratch database and drops it afterwards; the live databases
and identity files are not modified.

## PostgreSQL backup/restore

- Source `hnb_platform`: 130 tables (public schema).
- `pg_dump -Fc` produced a 449,824-byte custom-format dump.
- `pg_restore --no-owner --no-privileges` into a fresh `hnb_platform_restore` database.
- Restored table count: **130** (matched the source).
- Scratch database dropped after verification; live databases untouched.

## Identity-material backup integrity

- `tar czf` of the identity directory contains all six materials.
- Extracted to a scratch directory; each file's sha256 matched the live file byte-for-byte:
  - `master.key` `402b7bfa…`, `admin-bootstrap.password` `a24382f5…`, `private.pem` `26ecf086…`,
    `public.pem` `798dd1b6…`, `manifest.json` `db3fb858…`, `identity-state.json` `48d0a4c7…`.
- Result: 6/6 identical.

## Process/restart persistence

Covered by the existing-Minimal E2E: rolling restart of PostgreSQL, NATS and every deployment
recreated all workload Pod UIDs while preserving PVC UIDs/volumes and the three plane ledgers
(`docs/acceptance/reports/20261016-existing-minimal-e2e.md`).

## Not covered

- JetStream (message-plane) backup/restore.
- Measured and approved RTO/RPO values.
- Restore into a *different* cluster (disaster-recovery drill).
