# Tasks 6.1-6.8 Agent / CloudCore Observers and Canonical Projector

## Canonical projector (`cmd/platform-api/internal/observer`)

- `validate.go` enforces the RT-008 runtime-target observation contract against
  the **authenticated observer identity** (never the payload): schema version
  `1.0.0`, UUID `eventId`/`targetId`, generation/sequence >= 1, Full/Delta mode,
  observedAt within a 300s clock skew, targetKind/observerKind pairing, strict
  identity binding (tenant/target/kind/observerId/observerKind), unknown-field
  rejection, and Full-inventory tombstone prohibition.
- `projector.go` implements ordering invariants:
  - First observation must be generation 1, sequence 1.
  - Lower generation → `ErrFenced`; generation jump without source-reset →
    `ErrFenced`.
  - Lower sequence → `ErrReplay` (idempotent discard); equal sequence with
    matching eventId/digest → `ErrReplay`; equal sequence with different content
    → conflict; sequence > committed+1 → `ErrGap` (recorded in the inbox with a
    `processing_error` dead-letter).
  - `ApplyReset` fences the previous generation and requires the new generation
    to be greater, only advancing an already-committed generation.
- `PGCursorStore.SaveObservation` commits target projection, immutable
  capability snapshot (deduped by content digest), node inventory, cursor, and
  inbox row in a **single transaction** (no torn reads).
- `PGCursorStore` also implements `OldestUnprocessedObservedAt` for
  `Projector.ReportLag`.

## Observer identity (`pkg/iam/observer_token.go`)

- New `hnb.observer/v1` short-lived JWT binds a workload identity to
  tenant/target/targetKind/observerId/observerKind, an observer lease UUID, and
  the observer generation. Sign/verify tests cover round-trip, tampering,
  expired tokens, and mismatched kind/source rejection.
- `cmd/platform-api/internal/observer/http.go` exposes
  `POST /v1/observations` and `POST /v1/observations/reset`; the platform-api
  server dispatches these routes before the browser/service access-token
  middleware and verifies the observer token as authoritative.

## Producers

- `cmd/cluster-agent/internal/observer` (Agent):
  - `Producer` maintains monotonic generation/sequence, `Full` (complete
    inventory replace + cache), `Delta` (added/changed nodes + explicit
    tombstones), `SourceReset` (new generation fences the old), and 1 MiB
    payload bound.
  - `KubeDiscovery` reads nodes and capability from the local Kubernetes API
    (CPU/memory quantity parsing, health/connectivity from Ready conditions).
  - `Reporter` posts observations to the platform ingest URL on an interval
    with exponential backoff; wired into `cluster-agent/main.go` when
    `OBSERVATION_INGEST_URL` is configured.
- `cmd/edge-provider/internal/observer` (CloudCore):
  - `CloudCoreObserver` discovers edge nodes and capability through the
    CloudCore kube client (never EdgeCore), and reuses the same producer
    semantics with `CloudCore` observer kind. Node disconnect flips
    connectivity/health in a Delta.

## Metrics (`observer` package, Prometheus)

- `hnb_observation_projected_total`, `hnb_observation_replay_total`,
  `hnb_observation_sequence_gap_total`, `hnb_observation_fenced_total`,
  `hnb_observation_rejected_total`, `hnb_observation_out_of_order_total`,
  `hnb_observer_generation_jump_total`, `hnb_source_reset_accepted_total`,
  `hnb_projection_lag_seconds` (via `Projector.ReportLag`).

## Migration

- `database/postgresql/migrations/057_node_identity_source_uid.sql` drops the
  legacy `UNIQUE(target_id, name)` on `runtime_target_nodes` (RT-008 keys nodes
  by stable nodeId/source_node_uid; the source_uid partial unique index from
  migration 051 is authoritative). Forward/rollback/reapply verified against
  PostgreSQL 16.

## Verification

- `go test ./... -race` in `cmd/platform-api`, `cmd/cluster-agent`,
  `cmd/edge-provider`, `pkg/iam`: pass.
- PostgreSQL 16 integration (`HNB_TEST_POSTGRES_DSN`): atomic projection,
  idempotent replay, gap/fencing, Full tombstone, Delta tombstone,
  capability-snapshot dedup, and source-reset integration tests pass.
- Migrations 050-057 applied idempotently to the live `hnb` database; 057
  forward/rollback/reapply verified.

## Scope note

These tasks produce code-level conformance evidence (validated observation
flows, ordering/fencing, atomic projection, producers, and metrics). Live-stack
observation convergence (task 11.9/11.10) still requires a running
Kubernetes/KubeEdge cluster and the E2E stack, which is outside the current
environment.
