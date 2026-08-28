# Verification Evidence

## 2026-07-28

- `pkg/appstore`: `go test ./... && go vet ./... && go build ./...` passed.
- `cmd/app-market`: `go test ./... && go vet ./... && go build ./...` passed.
- `cmd/platform-api`: `go test ./... && go vet ./... && go build ./...` passed.
- `openspec validate "implement-artifact-storage" --type change --strict --no-interactive` passed.
- Disposable PostgreSQL migration rehearsal passed using `postgres:16` on port `55432`:
  - Full forward migration sequence `001` through `037` passed.
  - Artifact-storage rollback sequence `037` through `033` passed.
  - Forward-only retained metadata check passed after reapplying `033` through `037` and inserting representative artifact profile, distribution target, reference, tombstone and lock rows.
- `openspec validate --all --strict --no-interactive` passed after normalizing legacy spec headers/purpose text.

## Deferred Environment Exercises

- Real Harbor direct-upload exercise passed against `http://LAPTOP-1VCJCB49:80` using proxy bypass:
  - Created/verified project `hnb`.
  - Uploaded config and layer blobs with OCI Distribution `POST`/`PATCH`/`PUT` flow.
  - Pushed manifest `hnb/artifact-storage-e2e:e2e-1785250801`.
  - Verified manifest by digest `sha256:00a8a4dda867f154adf9a5cd5848bf7b1b57addc1dfcc36ece086cbf79d3297a` through both Harbor HEAD and `storage.OCIStorage.VerifyManifest`.
- Plan/profile/distribution/GC contract E2E exercises passed:
  - `cmd/app-market/internal/engine/market`: `TestManifestBridgePlanDigestIgnoresTagMovement`, `TestManifestBridgeToExecutionPlan`.
  - `pkg/appstore`: `TestProfileMigrationProviderContractCarriesIDsAndDigestOnly`, `TestDistributionProviderCommandCarriesIDsAndDigestOnly`, `TestGCProviderSweepCommandCarriesDigestOnly`.
  - `pkg/appstore/store`: profile migration digest preservation, distribution transitions/eviction, GC reference and no-secret command tests.
