## Context

Artifact metadata currently exists as package-bound rows while upload sessions issue project-level Harbor credentials. Upload confirmation trusts caller input and updates a foreign key without first inserting the Artifact, OCI references are parsed inconsistently, and the active planner emits no artifact digests. Storage profiles, distribution state and reference-safe GC do not exist. The design must preserve OCI-first distribution, tenant isolation, direct data-plane access and the Release -> ExecutionPlan -> Operation write path.

Stakeholders are publishers, App Market, Platform Planner, runtime Agents, operators and auditors. Harbor remains the OCI authority; PostgreSQL remains the control-plane fact store; existing Operation/Outbox/NATS infrastructure carries asynchronous work and events.

## Goals / Non-Goals

**Goals:**

- Establish a tenant-scoped `ArtifactDescriptor` as the canonical control-plane representation of every supported artifact type.
- Verify content against Harbor before recording immutable SHA-256 identity and propagate verified digests into production plans.
- Model Local/PVC/S3/OCI profiles and central/regional/edge distribution without coupling callers to physical paths.
- Make deletion reference-safe through preview, tombstone, retention, lock and Operation-backed execution.
- Keep metadata requests bounded and keep all file bytes outside Market/Platform APIs.

**Non-Goals:**

- Implement or bundle Harbor, S3, a registry mirror, CDN or edge cache data plane.
- Proxy uploads/downloads, store robot tokens, or expose backend credentials in events.
- Let App Market write runtime targets or let Platform API query App Market tables directly.
- Migrate existing object bytes between backends in-process; profile migration is represented as an Operation delegated to a provider.

## Architecture

```text
Publisher -- session --> App Market ---- create robot ----> Harbor API
Publisher ===== OCI bytes with temporary credential ======> Harbor Registry
Publisher -- confirm --> App Market ---- HEAD manifest ---> Harbor Registry
                              |
                              +-- transaction --> Descriptor + Session + Outbox
                              |
Platform Planner -- descriptor API/event --> verified digest --> ExecutionPlan

Operator -- GC preview --> Reference Graph --> Tombstone/Lock --> Operation Store
Operation Worker -- authorized delete command --> Harbor / Storage Provider

Distribution reconciler --> profile/target state --> Harbor replication or cache Provider
                         (metadata only; no bytes cross control-plane APIs)
```

App Market owns descriptor, profile, distribution, reference and tombstone data. Other planes use versioned HTTP contracts and domain events; they do not share or directly query these tables. Runtime mutations remain Operations, and NATS carries IDs/status only, never secrets or artifact bytes.

## Decisions

### Canonical descriptor extends existing Artifact identity

Keep the existing `artifacts` identity and add tenant, media type, repository, digest verification state and storage profile fields rather than creating a second competing object. Expose it as `ArtifactDescriptor`; package association becomes optional so non-market artifacts are representable. Supported kinds use stable semantic names while media type remains an independent OCI value.

Alternative considered: retain arbitrary release manifest JSON as the source of truth. Rejected because references cannot be validated, indexed or protected from GC.

### Confirmation verifies Harbor and commits atomically

The session stores its exact repository. Confirmation validates strict `sha256:[0-9a-f]{64}`, performs a Harbor manifest HEAD by repository and digest, compares the authoritative digest and available size/media type, then inserts the descriptor and completes the session in one database transaction. Robot deletion occurs after commit and is idempotently retried by cleanup if it fails. A failed Harbor lookup creates no descriptor.

Upload robots are push-only and scoped as narrowly as Harbor project permissions permit. Tokens are returned once and never persisted or logged. Pull access uses separate read-only credentials outside the upload robot.

Alternative considered: trust the digest returned by the client. Rejected because it permits recording missing or substituted content.

### Digest pinning is validated at release and plan boundaries

Release artifact references become normalized rows keyed by release and artifact descriptor. Publishing requires all descriptors to be verified and immutable. The planner receives descriptor snapshots through the App Market contract and includes sorted digests in the canonical plan hash. Production plans reject tags and malformed or unresolved digests.

Alternative considered: resolve tags during execution. Rejected because mutable resolution destroys reproducibility and auditability.

### Profiles describe authority; providers move bytes

`ArtifactStorageProfile` stores backend (`local`, `pvc`, `s3`, `oci`), service tier, authority role, endpoint/region, SecretReference, RPO/RTO and lifecycle status. Minimal may select local/PVC/OCI; Lite HA and above require a shared authoritative PVC/S3/OCI profile. Secrets remain in the existing secret system and only references are persisted.

Migration creates an Operation with source/target profile IDs and checkpoint metadata. A provider performs copy/verification; descriptor digest and release references never change. No new storage abstraction is introduced until a second concrete data plane requires it.

### Distribution is a control-plane state machine

Distribution targets reference an authoritative profile and have role `regional_mirror` or `edge_cache`, desired digest, observed digest, health and watermarks. Rebuild emits an idempotent Operation/provider command. Cache loss changes target state only; canonical descriptors remain available from the authority.

States are `pending -> syncing -> ready`; failures enter `failed` and may retry; cache loss enters `stale` before rebuild. Eviction may remove only non-authoritative copies.

### GC is mark, tombstone, retain, sweep

References are explicit rows with owner type/ID, purpose and optional expiry. Preview computes blockers for releases, applications, rollback points, compositions, DR snapshots and offline bundles. With no blockers, execute acquires an artifact lock, creates a tombstone with `delete_after`, and submits a GC Operation. The worker rechecks references after retention before deleting through the backend provider, then marks the descriptor deleted and emits an audit event.

States are `active -> tombstoned -> deleting -> deleted`; a new protected reference before sweep returns the artifact to `active`. Direct OCI delete is not exposed to request handlers.

## Data Model

- `artifacts`: canonical descriptor fields including `tenant_id`, optional `package_id`, kind, media type, repository, registry URL, strict digest, size, profile ID, verification and lifecycle state.
- `release_artifacts`: normalized immutable release-to-artifact references and purpose.
- `artifact_storage_profiles`: tenant-scoped backend/tier/authority configuration with SecretReference and RPO/RTO.
- `artifact_distribution_targets`: profile/target role, desired/observed digest, health, watermarks and rebuild operation.
- `artifact_references`: protected owner references used by GC.
- `artifact_tombstones`: retention deadline, state, operation and audit fields.
- `artifact_locks`: one active owner/lease per artifact.

All tenant-owned unique keys and queries include `tenant_id`. Digests use a database check for lowercase SHA-256 format. Foreign keys default to restrictive deletion for protected facts.

## API / Event Contracts

- `GET /api/v1/artifacts/{id}` returns a tenant-scoped descriptor without credentials.
- `POST /api/v1/artifacts/confirm` verifies Harbor and atomically creates the descriptor.
- `POST/GET /api/v1/artifact-storage/profiles` manages validated profiles; secret values are never accepted, only SecretReference.
- `GET /api/v1/artifacts/{id}/references` and `POST /api/v1/artifacts/{id}/gc/preview` return blockers.
- `POST /api/v1/artifacts/{id}/gc` creates a tombstone and Operation; it does not delete synchronously.
- `POST /api/v1/artifact-distributions/{id}/rebuild` creates an idempotent rebuild Operation.
- Events contain schema version, tenant ID, descriptor/profile/operation ID, digest and status: `artifact.verified`, `artifact.distribution.changed`, `artifact.tombstoned`, `artifact.deleted`. No token, secret or file body is allowed.

## Failure Modes

- Harbor unavailable or digest absent: confirmation returns retryable 503/409 and leaves the session pending; no descriptor is created.
- Database transaction failure: session and descriptor both remain uncommitted; robot cleanup remains safe and idempotent.
- Robot deletion failure: confirmation remains committed and cleanup retries deletion without exposing the token.
- Profile provider unavailable: migration/distribution Operation checkpoints and retries; current authority remains unchanged.
- Concurrent reference creation and GC: artifact lock plus final reference recheck prevents deletion.
- Worker crash after backend deletion: idempotent provider delete and operation checkpoint complete metadata transition on retry.

## Security, Reliability, And Operations

- Tenant identity comes from trusted IAM context; request bodies cannot override it. Repository names are generated server-side and credentials are least-privilege, short-lived and redacted.
- Harbor response digest is authoritative; verified descriptors are immutable. Supply-chain metadata such as SBOM remains a first-class descriptor and may be linked by references.
- Every publish, verification, profile change, rebuild, tombstone and delete is auditable with actor and operation correlation.
- List/scan jobs are batch-limited; default cleanup/GC batches are 100. Control-plane memory is O(batch), and no request buffers artifact bodies.
- Capacity metrics cover descriptor count/bytes, profile capacity, cache watermarks, verification latency, GC blockers and retry counts. Alerts cover authority unavailability, robot cleanup backlog and stuck operations.
- PostgreSQL backup includes all new metadata. Harbor/S3 backup and restore remain provider responsibilities; after restore, distribution copies may be rebuilt from the authoritative profile.
- Upgrade uses additive migrations, backfill and delayed constraints. Rollback stops reconcilers/workers first and preserves new tables. Uninstall refuses while protected descriptors or active Operations exist.
- This change does not alter Provider/RuntimeTarget/Gateway compatibility. Distribution providers must pass idempotent sync, rebuild-after-loss and digest-verification conformance tests before activation.

## Risks / Trade-offs

- [Broad P1 scope] -> Deliver in vertical slices: verified descriptor path, digest pinning, profiles, distribution, then GC; each slice has migrations and tests.
- [Harbor project permissions cannot scope robot to one repository] -> Generate unpredictable session repositories, grant push-only project access, use one-hour TTL and delete immediately after confirmation.
- [Legacy manifest JSON lacks references] -> Backfill resolvable references and block publish/GC for ambiguous rows until reconciled.
- [Control-plane distribution state can drift from providers] -> Treat observed digest and health as reconciled status, never as authority; periodically verify.
- [GC races or provider partial failures] -> Lock, retention, final reference check, idempotent delete and operation checkpoints.

## Migration Plan

1. Add descriptor/profile/reference/distribution/tombstone tables and nullable columns; deploy readers compatible with old rows.
2. Backfill tenant, repository and verified state where Harbor confirms the digest; quarantine unresolved rows.
3. Enable atomic upload confirmation and strict SHA-256 validation, then normalize release references.
4. Enable plan digest propagation and production publish validation.
5. Enable profile and distribution APIs/reconcilers.
6. Enable GC preview first; enable sweep only after reference backfill and audit validation.
7. Roll back application workers in reverse order; preserve metadata tables and disable destructive sweep. Database rollback scripts are development-only after proving no production rows exist.

## Open Questions

- Harbor deployment-specific repository permission support must be confirmed; if unavailable, project-level push-only is the accepted P1 boundary.
- The first concrete non-OCI provider used for profile migration conformance will be selected from existing deployment environments; P1 does not install one.
