# Change: Establish the Phase 0 Project-Truth Baseline

## Change Metadata

| Field | Value |
|---|---|
| Change ID | `establish-phase-0-project-truth` |
| Tier | T0 governance and evidence |
| Status | Ready for independent review |
| Baseline | HNB Cloud OpenSpec V3.8.6 |
| Affected planes | App Market, Artifact Storage, Runtime Governance, AI Extension Plane; cross-cutting Web Console and platform kernel |
| Affected specs | New `project-truth-baseline` capability |
| Dependencies | `openspec/architecture.md`, the V3.8.6 implementation baseline, and the repository working tree captured on 2026-07-26 |
| Database/API/event migration | None |

## Why

The repository contains substantially more implementation than the committed
OpenSpec baseline, but route presence, handler depth, persistence, security,
tests, and production readiness are uneven. Phase 1 planning cannot safely use
file or route counts as proof of capability.

This change records the current working tree as evidence, assigns a repeatable
L0-L5 maturity rating, and identifies integration and security blockers without
changing product behavior. It also protects the V3.8.6 invariants:

- microkernel plus Provider/CapabilityPack;
- four logically decoupled planes;
- `Release/CompositionRelease -> ExecutionPlan -> Operation` as the only path
  that writes runtime targets;
- a capability- and permission-driven Web Console;
- the existing `app-market` implementation as the Marketplace foundation.

## What Changes

- Add a stable Phase 0 evidence contract (`P0-BASE-001` through
  `P0-BASE-006`).
- Add a repository-local evidence baseline with:
  - route/handler/persistence/test inventory;
  - an L0-L5 maturity matrix;
  - verified stubs and partial integration seams;
  - security-boundary findings;
  - blockers and follow-up ownership.
- Define Phase 0 exit criteria and review rules.
- Record validation results and environmental limitations.

## Capabilities

### New Capabilities

- `project-truth-baseline`: repeatable, source-anchored assessment of the
  implementation state used to gate later phases.

### Modified Capabilities

None. This change does not modify approved product behavior.

## User Value

Reviewers receive one independently reviewable source of truth that separates
"a route exists" from "the capability is secure, tested, and production
ready." Product and engineering planning can therefore prioritize closure of
verified seams instead of recreating or replacing existing foundations.

## Non-Goals

- No Phase 1 or later product functionality.
- No new Marketplace service and no replacement of `cmd/app-market`.
- No route, handler, migration, model, event, UI, deployment, or runtime
  behavior changes.
- No claim that an existing unchecked OpenSpec task is complete merely because
  implementation files exist.
- No remediation of the blockers documented by this change.

## Impact

### Compatibility

Documentation-only. Existing APIs, schemas, events, database objects, binaries,
charts, and UI bundles are unchanged.

### Security

This change exposes security gaps as blockers; it does not weaken or repair
boundaries. Phase 1 work must not treat a documented gap as accepted risk
without a separate approved change.

### Resources and Capacity

No runtime CPU, memory, storage, network, or database capacity impact. Review
and future re-capture require repository inspection and existing validation
tools only.

### Observability

No telemetry is added. The evidence baseline records missing production
telemetry and runbooks as L5 blockers.

### Migration and Rollback

There is no data or runtime migration. Rollback is removal of this active
OpenSpec change and its evidence document. Because no product behavior changes,
rollback requires no service restart or data restoration.

## Phase 0 Exit Criteria

Phase 0 is complete when all of the following are true:

1. The evidence document names the repository snapshot and inspection scope.
2. Every assessed surface has an L0-L5 rating with source evidence or an
   explicit "not verified" marker.
3. The four mandatory concerns are verified and classified.
4. Security boundaries, broken seams, stubs, and blockers are recorded without
   silently converting them into accepted architecture.
5. The V3.8.6 architecture invariants and `app-market` continuity are explicit.
6. The new delta spec passes repository-local semantic checks.
7. Available existing validation commands are run and unavailable or failing
   commands are reported exactly.
8. No product behavior or Phase 1+ implementation is introduced.

