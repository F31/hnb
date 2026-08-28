## 1. Contract And Configuration

- [x] 1.1 Define the strict versioned HTTP request/response models and bounded decoder [RDI-002, RDI-003]
- [x] 1.2 Parse and validate explicit `RUNTIME_PROVIDERS` endpoint mappings at startup [RDI-001, RDI-004]
- [x] 1.3 Record Schema/database migration as N/A because the adapter adds no persisted fields or events [RDI-002]

## 2. Runtime Driver

- [x] 2.1 Implement exact Provider routing, context cancellation, and fail-closed HTTP execution [RDI-001, RDI-002, RDI-003]
- [x] 2.2 Preserve outputs/checkpoints on Provider failure and propagate tenant, idempotency, and fencing fields [OP-004, RDI-002, RDI-003, RDI-004]
- [x] 2.3 Inject the configured Runtime Driver into Operation Worker startup without adding an execution bypass [RDI-001, RDI-004]

## 3. Verification And Operations

- [x] 3.1 Add unit and HTTP contract tests for configuration, routing, success, retry checkpoint, malformed/oversized responses, non-2xx, and cancellation [RDI-001, RDI-002, RDI-003]
- [x] 3.2 Run module tests, race tests, vet, and a configured Worker startup smoke test; record evidence [RDI-001, RDI-004]
- [x] 3.3 Record E2E target mutation as N/A until a concrete Provider is implemented; document rollout and rollback evidence [RDI-004]
- [x] 3.4 Run `openspec validate --all --strict` before archive [RDI-001, RDI-002, RDI-003, RDI-004]
