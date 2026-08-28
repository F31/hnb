## 1. Schema And Store

- [x] 1.1 Add migration 013 and guarded rollback for the global generation, Lease identity/generation, retained Step generation, and Step FK [OP-008]
- [x] 1.2 Refactor acquisition into one authoritative transaction returning `Lease{ID, Generation}` and fence renewal/retry/commit on both values [OP-008]
- [x] 1.3 Add PostgreSQL tests for monotonicity, conflict gaps, expiry takeover, retained generation, wrong fences, and migration/rollback guards [OP-008]

## 2. Runtime Driver V2

- [x] 2.1 Replace the v1 execution fields with v2 attempt identity and decimal-string generation plus strict response echo validation [RDI-005]
- [x] 2.2 Add typed Provider error codes and Worker classification for fenced, permanent, transient, and protocol failures [RDI-005, OP-008]
- [x] 2.3 Update Worker heartbeat, retry, audit, and commit paths to carry the Lease object without changing StepRequested [OP-008, RDI-005]

## 3. Kubernetes CAS

- [x] 3.1 Implement strict generation annotations, idempotent equal-generation replay, and higher-generation resourceVersion CAS deploy takeover [KRP-005]
- [x] 3.2 Implement `expected_uid` logical tombstone delete, zero-replica observation, and stale-deploy rejection [KRP-006]
- [x] 3.3 Update Provider v2 HTTP responses, RBAC, Provider manifest, and bounded CAS tests [RDI-005, KRP-005, KRP-006]

## 4. Verification And Cutover

- [x] 4.1 Add deterministic external-success-before-DB-commit recovery and delayed stale-attempt integration tests [OP-008, KRP-005]
- [x] 4.2 Run the complete 001-013 migration chain, rollback guard tests, module/race/vet suites, and real kind takeover/tombstone E2E [OP-008, RDI-005, KRP-005, KRP-006]
- [x] 4.3 Record stopped-dispatch cutover, roll-forward-only boundary, compatibility, rollback, security, telemetry N/A, and cleanup evidence [RDI-005]
- [x] 4.4 Run `openspec validate --all --strict` and diff/format checks [OP-008, RDI-005, KRP-005, KRP-006]
