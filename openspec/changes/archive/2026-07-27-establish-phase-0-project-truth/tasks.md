# Tasks: establish-phase-0-project-truth

## Summary

| Field | Value |
|---|---|
| Change | `establish-phase-0-project-truth` |
| Tier | T0 governance and evidence |
| Requirements | P0-BASE-001 through P0-BASE-006 |
| Product behavior | Unchanged |

## Evidence Capture

- [x] **P0-BASE-001, P0-BASE-002** Capture Git anchor, working-tree caveat,
  inspection scope, and maturity rubric.
- [x] **P0-BASE-001, P0-BASE-004** Inspect routes, handlers, middleware,
  repositories, models, and tenant predicates.
- [x] **P0-BASE-001, P0-BASE-003** Inspect PostgreSQL migrations and identify
  canonical Operation persistence and incompatible migration seams.
- [x] **P0-BASE-003** Trace Release/ExecutionPlan/Operation/provider flow and
  identify direct or disconnected writes.
- [x] **P0-BASE-006** Inspect `cmd/app-market`, its repositories, NATS subjects,
  and install/uninstall lifecycle without creating a replacement service.
- [x] **P0-BASE-003, P0-BASE-004** Inspect cluster-agent, tunnel-server, and
  kube-API proxy behavior.
- [x] **P0-BASE-005** Inspect Web Console plugin loading, capability checks,
  permission hydration, routes, and Marketplace page integration.
- [x] **P0-BASE-002** Inventory existing unit, integration, E2E, deployment,
  and conformance evidence.

## OpenSpec Artifacts

- [x] **P0-BASE-001** Add proposal with metadata, scope, non-goals, impact,
  compatibility, security, resources, observability, rollback, and exit gates.
- [x] **P0-BASE-001 through P0-BASE-006** Add delta spec with stable IDs,
  traceability, and GIVEN/WHEN/THEN scenarios.
- [x] **P0-BASE-003, P0-BASE-004** Add design with architecture boundaries,
  evidence model, lifecycle, failure modes, alternatives, and security review.
- [x] **P0-BASE-001 through P0-BASE-006** Add repository-local evidence
  baseline and blocker register.

## Validation

- [x] **P0-BASE-001** Run the strict OpenSpec CLI gate; attach the exact result
  or environmental blocker.
- [x] **P0-BASE-001** Run the repository-local OpenSpec semantic check against
  the new delta and record the result.
- [x] **P0-BASE-002** Run relevant existing Go tests where the Go toolchain is
  executable; otherwise record the environment blocker without claiming pass.
- [x] **P0-BASE-005** Validate the console manifest and run available frontend
  type/build checks; distinguish a valid manifest from functional integration.
- [x] **P0-BASE-001** Recheck the final diff and confirm no product behavior was
  changed.

## Explicitly Not Applicable in Phase 0

- Schema/API generation: N/A; evidence-only change.
- Database migration and rollback SQL: N/A; no database change.
- Runtime implementation: N/A; Phase 1+ is excluded.
- New unit/integration/E2E product tests: N/A; product behavior is unchanged.
- Provider compatibility matrix: N/A; no provider contract change.
- Deployment/upgrade/DR drill: N/A; no deployable artifact changes.
