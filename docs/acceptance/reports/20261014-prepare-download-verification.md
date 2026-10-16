# Local material preparation — mirror mapping and interrupted-run reporting

## Status and scope

Task 4.2 is **partial**: local downloads, content verification, cache reuse,
explicit registry mapping and progress/failure results are implemented and tested.
Node preload still depends on the dedicated Day-0 authorization in task 4.4;
`prepare --preload` remains rejected before cache creation or any node action.

Base commit: `c23f6e4128e92d7a9457807dce5b10e0b0704e73` plus working-tree changes.
Selected execution-path source hashes, environment, command, times, exit status
and full sanitized test output are retained in `20261014-prepare-real-registry.json`.

This is installer-local material verification with the existing HTTP proxy at
`127.0.0.1:7892`. It did not access the cloud host, import into containerd,
install packages or deploy a platform.

## Changes

- The old general config validator rejected every mirror-mode configuration,
  making the existing `prepare --mirror-origin` route unreachable. Preparation now
  has a dedicated validation entry point that requires a credential-free HTTPS
  mirror origin. OCI repository paths and locked identities are preserved. Missing
  or invalid origins refuse before cache creation; general installation validation
  continues to reject its unimplemented mirror mapping.
- Results now include status, lock digest, UTC start/finish times and a sanitized
  failure reason. Download failure or context cancellation returns the completed,
  verified artifacts and bytes on stdout, with a nonzero command exit status.
  Retrying rehashes and reuses those completed objects; incomplete bytes are never
  reported as verified.
- Retry/failure progress now records UTC observation time, actual response origin,
  attempt, bytes and cause. A CDN failure is attributed to the CDN rather than
  the initial registry. Signed queries and Bearer tokens are not emitted.

## Automated regression

From `cmd/hnbctl`:

```sh
go test -race ./internal/toolkit ./internal/cli -run 'TestPrepare' -count=1
go test -race ./... -count=1
```

Both exited 0. The full race run passed `internal/cli` and `internal/toolkit`.
Coverage added/extended:

- `TestPrepareMirrorRequiresExplicitOriginBeforeCreatingCache`
- `TestPrepareMirrorMappingIsPreparationOnly`
- `TestPrepareFailureOutputsVerifiedPartialResultAndCanRetry`
- `TestPrepareCancellationRetainsCompletedArtifacts`
- `TestPrepareRetryReportsActualCDNOriginAndReason`
- `TestPrepareInterruptedTransferRetriesAndPublishesOnlyVerified`
- `TestPrepareFailedRunCanReuseOnlyCompletedArtifacts`

Existing digest-mismatch, corrupt-cache, platform-selection, source-policy,
private-file and preload-refusal regressions also passed.

## Real registry download

Retained run: `local-prepare-registry-20261014`,
**2026-10-14 02:06:48–02:07:01 UTC**, exit code **0**.

```sh
HNB_PREPARE_E2E_REF=registry-1.docker.io/library/alpine:3.22.2 \
HNB_PREPARE_E2E_REDIRECT_ORIGINS=https://production.cloudfront.docker.com \
go test ./internal/toolkit -run '^TestPrepareRealRegistryClosure$' -count=1 -v
```

Observed index digest:
`sha256:4b7ce07002c69e8f3d704a9c5d6fd3053be500b7f1c69fc0d80990c2ad8dd412`.
Selected linux/amd64 manifest:
`sha256:85f2b723e106c34644cd5851d7e81ee87da98ac54672b29947c052a45d31dc2f`.

- Anonymous Bearer negotiation completed against `auth.docker.io`.
- Cold preparation verified **4 objects / 3,813,273 bytes**, cache hits **0**.
- Registry blob responses redirected to the explicitly declared
  `https://production.cloudfront.docker.com` origin.
- Warm preparation rehashed and reused all **4 objects**.

Earlier attempts without that CDN declaration failed with `registry request
failed`. An origin-only diagnostic trace showed the registry's HTTP 307 target;
only then was the observed origin explicitly provided to the test. The policy
was not weakened and redirects were not auto-approved. Streaming/retry, mirror
mapping and failed-result recovery use deterministic TLS server fixtures; this
public run verifies one real image closure only.

## Remaining scope

Task 4.2 stays unchecked because authorized bootstrap preload is not connected.
Task 4.4, complete release material, Minimal platform E2E, service credential
rotation, Agent observation, HA and disconnected installation retain their
separate pending states. Local material status `verified` always retains
`installAuthorized: false`.
