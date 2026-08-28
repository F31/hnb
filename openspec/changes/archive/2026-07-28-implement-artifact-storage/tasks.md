## 1. Canonical Descriptor Schema

- [x] 1.1 [ART-001, ART-002] Add additive PostgreSQL migration for tenant-scoped ArtifactDescriptor fields, supported kinds, strict SHA-256 checks and lifecycle state; add non-destructive rollback notes and migration tests.
- [x] 1.2 [ART-001] Add `ArtifactDescriptor` domain model and tenant-scoped repository CRUD/list methods without duplicating Artifact identity.
- [x] 1.3 [ART-001] Add descriptor GET/list API and route authorization; verify cross-tenant requests do not disclose metadata.
- [x] 1.4 [ART-001, ART-002] Add normalized `release_artifacts` references and backfill/reconciliation command for resolvable legacy release manifests.

## 2. Verified Direct Upload

- [x] 2.1 [ART-003] Store the server-generated repository on UploadSession and issue push-only Harbor robot permissions with one-hour TTL.
- [x] 2.2 [ART-002, ART-003] Add strict lowercase SHA-256 validation and Harbor manifest HEAD verification by repository and digest, including status/media type/size handling.
- [x] 2.3 [ART-003] Implement one transaction that inserts ArtifactDescriptor and completes UploadSession; make duplicate confirm idempotent and robot cleanup retryable.
- [x] 2.4 [ART-003] Return 404/410 guidance for the removed proxy upload route without accepting or parsing a request body.
- [x] 2.5 [ART-002, ART-003] Add handler and Harbor mock integration tests for success, forged/missing digest, expired/cross-tenant session, duplicate confirm and cleanup failure.

## 3. Digest-Pinned Release And Planning

- [x] 3.1 [ART-002] Validate Release artifact references on create/publish and reject malformed, tag-only, missing or unverified descriptors.
- [x] 3.2 [ART-002] Populate sorted verified artifact digests in Platform ExecutionPlan and include them in canonical plan hashing.
- [x] 3.3 [ART-002] Add release/planner contract tests proving tag movement cannot change a published plan or rollback digest.

## 4. Storage Profiles

- [x] 4.1 [ART-004] Add profile migration/model/repository for Local/PVC/S3/OCI, tier, authority, SecretReference, endpoint/region, RPO/RTO and lifecycle state.
- [x] 4.2 [ART-004] Implement profile validation for Minimal and Lite HA+ shared-authority rules; reject inline secret values.
- [x] 4.3 [ART-004] Add tenant-scoped profile create/get/list API with audit fields and authorization tests.
- [x] 4.4 [ART-004] Add profile migration Operation request/checkpoint contract that preserves descriptor digest and Release references.
- [x] 4.5 [ART-004] Add profile migration conformance tests using an in-memory provider; installing a new storage middleware is N/A because providers own data-plane deployment.

## 5. Distribution Control Plane

- [x] 5.1 [ART-005] Add migration/model/repository for regional mirror and edge cache targets, desired/observed digest, health, watermarks and rebuild operation ID.
- [x] 5.2 [ART-005] Implement pending/syncing/ready/stale/failed state transitions and tenant-scoped status API.
- [x] 5.3 [ART-005] Implement idempotent rebuild Operation submission and provider command/event contracts containing IDs and digests only.
- [x] 5.4 [ART-005] Implement high-watermark eviction candidate selection that excludes authoritative copies and locally locked artifacts.
- [x] 5.5 [ART-005] Add conformance and failure-recovery tests for cache loss, authority outage, checkpoint retry and digest verification; a bundled mirror/cache data plane is N/A.

## 6. Reference-Safe GC

- [x] 6.1 [ART-006] Add migrations/models/repositories for artifact references, tombstones and leased artifact locks with restrictive foreign keys.
- [x] 6.2 [ART-006] Implement tenant-scoped reference registration/listing for Release, runtime, rollback, composition, DR and offline bundle owners.
- [x] 6.3 [ART-006] Implement GC preview returning all blockers and capacity estimates without mutating state.
- [x] 6.4 [ART-006] Implement GC execute to acquire a lock, recheck references, create a retained Tombstone and submit a high-risk Operation with audit correlation.
- [x] 6.5 [ART-006] Implement idempotent sweep worker with final reference check, pause/retry/rate limit/checkpoint support and Tombstone cancellation when a new reference appears.
- [x] 6.6 [ART-006] Remove request-path access to immediate OCI deletion and add race, retry, authorization and protected-reference integration tests.

## 7. Observability And Operations

- [x] 7.1 [ART-003, ART-004, ART-005, ART-006] Add structured logs and metrics for verification, robot cleanup, profile health, distribution rebuild and GC outcomes without secrets.
- [x] 7.2 [ART-004, ART-005, ART-006] Document capacity limits, backup/restore ownership, DR rebuild, upgrade order, rollback and uninstall refusal conditions.
- [x] 7.3 [ART-001, ART-002] Add OpenAPI/schema contracts and verify App Market/Platform communicate through versioned API/events rather than direct database access.

## 8. Verification And Release

- [x] 8.1 [ART-001, ART-002, ART-003] Run Go unit/integration/contract tests, build and vet for appstore, app-market and platform-api; attach command evidence.
- [x] 8.2 [ART-004, ART-005, ART-006] Run migration forward/rollback rehearsal on an empty database and forward-only rehearsal with retained metadata.
- [x] 8.3 [ART-001–ART-006] Run E2E exercises for direct upload through plan digest, profile migration recovery, cache rebuild and protected GC.
- [x] 8.4 [ART-001–ART-006] Run `openspec validate --all --strict`, complete workflow verify if available, sync delta specs and archive the change.
