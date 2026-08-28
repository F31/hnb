# Task 7.2 Storage Import Rehearsal

Date: 2026-08-14

## Scope

The rehearsal imports active observed `StorageClass` projection rows into
`WorkloadStorageOffering` and `StorageClassBinding` desired records. The import
tool connects only to PostgreSQL. It has no Kubernetes client, credential,
manifest, proxy, executor, or delete path.

Dry-run is the default. `--apply` is required to commit the single database
transaction. Deterministic IDs are derived from tenant, target, and Kubernetes
UID; duplicate imports use conflict-safe inserts and preserve desired-state
version, timestamps, StorageClass UID, and resourceVersion. Tenant/target
ownership is checked before writes.

## Command

```bash
bash database/postgresql/scripts/storage_import_rehearsal_test.sh
```

The test creates and removes an isolated `postgres:16` Docker container,
applies the repository migrations, loads tenant/target/observed StorageClass
fixtures, and executes dry-run and committed imports.

## Evidence

```text
PASS dry-run rollback: 0 offerings, 0 bindings
PASS duplicate import: stable digest, 2 offerings, 2 bindings, versions and Kubernetes identities preserved
PASS cross-tenant rejection: tenant B desired rows remain 0
PASS query context: target, cluster, offering and storageClass filters emitted
PASS target safety: observed inventory digest unchanged and no Kubernetes mutation/delete path
PASS PostgreSQL: 16.14(Debian16.14-1.pgdg13+1)
```

The before/after digest covers all observed inventory fixture rows. The safety
check also rejects Kubernetes mutation commands and SQL mutation/deletion of
`runtime_target_storage_inventory` in the import implementation. Imported
offerings remain explicitly review-required because StorageClass observations
do not prove service mode or access-mode semantics.
