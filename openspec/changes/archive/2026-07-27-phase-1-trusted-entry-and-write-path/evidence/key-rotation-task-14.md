# Task 14 Signing-Key Rotation Evidence

Date: 2026-07-27
Task: P1-ING-005, OpenSpec task 14
Status: **implementation complete; task 14 is checked**

## Key Manifest And Reloading Key Set

- `pkg/iam` defines a strict, versioned JSON `KeyManifest` containing only
  issuer, generation, active key ID, and per-key public path, lifecycle status,
  not-before, and not-after. It has no private-key field.
- Parsing rejects unknown or duplicate JSON fields, trailing values, missing or
  reused generations, dirty/relative paths, non-regular/empty/oversized files,
  unsafe permissions, invalid key IDs or windows, non-P-256 PEM keys, changed
  immutable key metadata, forbidden lifecycle transitions, and anything other
  than exactly one currently valid active key.
- `ReloadingKeySet` publishes an immutable snapshot through `atomic.Pointer`.
  A failed reload returns an error, increments failure stats, and preserves the
  previous snapshot. Lower generations and different content under the same
  generation are rejected without restoring old key state.
- `CurrentSigningKey` exposes only the active, currently valid key.
  `VerificationKey` exposes only current active/retiring keys. Pending,
  revoked, expired, out-of-window, and unknown keys fail closed.
- Apiserver supplies a separate active-private path. Every forward-generation
  reload reopens it and proves that it matches the manifest's active public
  key before publishing the snapshot. A mismatch leaves the old snapshot
  intact. No verifier-only service has private-key configuration.
- Reload polling is configurable and validated from 1 through 60 seconds.
  `KeyReloadStats` reports generation/success/failure counts; successful
  generation changes and reload failures are also logged at every entry point.

## Entry Points And Persistence

- `cmd/apiserver` uses the reloadable set for access-token signing,
  access-token verification, and tunnel service-token verification. It is the
  only plane that injects `IAMDBStore` as a manifest recorder.
- `cmd/platform-api`, `cmd/app-market`, `cmd/kubernetes-provider`,
  `cmd/edge-provider`, and `cmd/tunnel-server` use the same verifier-only
  reloadable set and do not write IAM key metadata.
- Production startup requires `API_TOKEN_KEY_MANIFEST_FILE`; there is no
  fallback from a missing manifest to `API_TOKEN_VERIFICATION_KEYS` or a static
  environment map. `API_TOKEN_KEY_RELOAD_INTERVAL` defaults to five seconds.
- Successful apiserver generations update existing
  `signing_key_metadata.version/status` rows and append status transitions to
  `signing_key_lifecycle_events` under a transaction-scoped issuer lock.
  Duplicate generations are idempotent. The store rejects durable generation
  rollback and revoked/expired key restoration across apiserver restarts. SQL
  arguments contain public references and manifest handles only, never private
  key paths or material.

## Deployment And Operations

- Helm subcharts and parent values mount a Secret containing `manifest.json`
  plus public PEM files as one projected volume with mode 0400. Apiserver mounts
  its active private key from a separate mode-0400 Secret and path.
- Compose verifier services mount an operator-provided key-set directory and
  use the manifest and bounded polling settings. No key material was added to
  the repository.
- `deploy/key-rotation-runbook.md` documents staging K2 as pending, confirming
  fleet visibility, switching K1 to retiring and K2 to active, waiting maximum
  token TTL plus clock skew, expiring K1, emergency K1 revocation, monotonic
  generation rules, and safe failure/rollback behavior.

## Tests

`pkg/iam` tests cover:

- active K1 issuance, K2 pending rejection, K1/K2 overlap, hot K2 signing
  switch, no retiring-key signing, immediate K1 token rejection after
  revocation, and continued K2 acceptance;
- expired, not-yet-valid, pending, revoked, expired, and unknown key rejection;
- generation rollback, generation reuse, invalid-manifest atomic retention,
  active public/private mismatch, duplicate JSON fields, unsafe permissions,
  and 1..60 second polling bounds;
- concurrent reload/verification under the race detector;
- recorder duplicate-generation idempotency, append-only lifecycle inserts,
  and SQL arguments with no private fields or values.

Every affected service has config/main coverage for required manifest settings
and the polling propagation bound.

## Verification

- `go test -race -count=1 ./...` passed in `pkg/iam`, `cmd/apiserver`,
  `cmd/platform-api`, `cmd/app-market`, `cmd/kubernetes-provider`,
  `cmd/edge-provider`, and `cmd/tunnel-server`.
- `npm run contracts:generate -- --check` passed; task 14 changes no public
  contract.
- `npm run contracts:check` passed: 16 tests plus schema, compatibility,
  generated-drift, OpenAPI, and Buf validation.
- `openspec validate phase-1-trusted-entry-and-write-path --strict
  --no-interactive` passed before the task status update.
- `docker compose -f deploy/docker-compose/compose.yml config --quiet` passed
  with required external fixture paths supplied.
- `git diff --check` passed.
- Helm rendering was not run because the Helm CLI is unavailable. Static scans
  confirmed all six verifier deployments have manifest/reload settings,
  mode-0400 manifest Secret volumes, no legacy key-map settings, and the
  apiserver-only separate active-private mount.
- No migration was added; migration 026 already owns the required metadata and
  append-only lifecycle tables.

## Explicit Exclusions

- No RuntimeIntent or Console behavior was implemented or changed for task 14.
- No real routine rotation or emergency-revocation drill was performed.
  OpenSpec task 25 remains unchecked.
- Broad verification task 20 remains unchecked.
