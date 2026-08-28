# Artifact Storage Operations

Artifact Storage metadata lives in PostgreSQL. Artifact bytes stay in Harbor, S3, PVC or provider-owned storage and are never backed up by App Market APIs.

## Capacity

- Track descriptor count, total verified bytes, profile capacity, distribution target watermarks and GC blocker counts.
- Edge cache eviction may remove only non-authoritative copies with no local lock and current bytes above the high watermark.
- Authoritative profiles must be sized for all retained releases, rollback points, DR snapshots and offline bundles.

## Backup And Restore

- PostgreSQL backup includes descriptors, release references, profiles, distribution state, references, tombstones and locks.
- Harbor/S3/PVC backup remains the storage provider responsibility.
- After metadata restore, mark non-authoritative distribution targets `stale` and rebuild from an authoritative profile by digest.
- If the authority is unavailable, rebuild Operations remain retryable and must not promote caches to authority.

## DR Rebuild

- Rebuild commands contain IDs and digests only: target, artifact, authority profile, target profile and operation ID.
- Providers copy and verify bytes outside the control plane.
- Observed digest must match desired digest before a target becomes `ready`.

## Upgrade And Rollback

- Apply migrations in order: descriptors, release refs, profiles, distributions, GC.
- Stop reconcilers and sweep workers before rollback.
- Rollback scripts are development-only once production metadata exists; preserve metadata tables in production rollback.

## Uninstall Refusal

- Refuse uninstall while active Operations, protected references, retained tombstones or authoritative profiles with active artifacts exist.
- Direct request-path deletion is not exposed. GC requires preview, lock, tombstone, retention and final reference recheck.
