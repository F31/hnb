## 1. Contract And Process

- [x] 1.1 Create the independent Go module, strict Runtime Driver v1 HTTP handler, health endpoint, and bounded configuration [KRP-001, KRP-004]
- [x] 1.2 Record database/event migrations as N/A because the Provider owns no Operation state [KRP-004]

## 2. Kubernetes Execution

- [x] 2.1 Implement validated allowlisted Deployment deploy and availability observation [KRP-001, KRP-002, KRP-004]
- [x] 2.2 Implement ownership, idempotency, fencing conflict, and UID-precondition delete checks [KRP-002, KRP-003]
- [x] 2.3 Add least-privilege namespace-scoped deployment manifests and Provider manifest metadata [KRP-002, KRP-004]

## 3. Conformance

- [x] 3.1 Add fake-client unit and HTTP contract tests covering success and all safety conflicts [KRP-001, KRP-002, KRP-003, KRP-004]
- [x] 3.2 Run tests, race, vet, and strict OpenSpec validation [KRP-004]
- [x] 3.3 Execute real kind create, idempotent replay, fencing conflict, and delete E2E and record evidence [KRP-002, KRP-003, KRP-004]
- [x] 3.4 Document installation, upgrade, rollback, retained workload, compatibility, and residual fencing risk [KRP-003, KRP-004]
