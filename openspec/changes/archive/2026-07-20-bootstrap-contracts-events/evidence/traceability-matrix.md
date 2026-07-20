# Contracts foundation traceability and archive review

## Requirement matrix

| Requirement | Source Schema and mapping | Generated output | Automated and compatibility tests | Validation policy, documentation, and rollback | Status |
|---|---|---|---|---|---|
| CONTRACT-001 Schema First | `contracts/openapi/foundation/v1/openapi.yaml`, `contracts/proto/hnb/contracts/v1/contracts.proto`, `contracts/schema/common/v1/*` | `contracts/generated/go`, `contracts/generated/typescript` | lint, AJV validation, generation compile, drift, semantic alignment, dependency-boundary and interoperability tests | repository validation policy; public contract guide; rollback report | Complete |
| CONTRACT-002 compatibility | versioned `v1` paths/packages and JSON Schema `$id`; oasdiff, Buf breaking, and `jsonSchemaBreakingChanges` | generated packages retain source version boundaries | field removal, type change, required addition, enum removal, Protobuf removal/number reuse, explicit major-version fixtures, and checked `origin/main` baseline | evolution/deprecation guide; complete release-set rollback test | Complete |
| CONTRACT-003 idempotency and correlation | OpenAPI correlation/idempotency/If-Match parameters; RequestContext and EventEnvelope fields across formats | Go and TypeScript headers/message types | missing-header failures; cross-format semantics; Go-to-TypeScript UUID, timestamp, unknown-field, idempotency and correlation checks | public contract guide and unified local gate | Foundation complete; runtime deduplication remains downstream scope |
| CONTRACT-004 transactional event boundary | `contracts/mappings/outbox-event-envelope.json` and broker-neutral EventEnvelope | generated envelope types | Outbox field mapping test rejects Broker subject and sequence leakage | database migration and Relay are explicitly N/A for this static change | Mapping contract complete; runtime Outbox remains downstream scope |
| CONTRACT-006 repository and gate | toolchain BOM plus all three source Schema trees | deterministic Go/TypeScript output and `TOOLCHAIN.json` | 13 contract tests; environment/schema/compatibility/drift exit paths; online/offline gate; zero drift | local pre-review policy, developer guide, supply-chain report, rollback report | Complete; GitHub is code hosting only |

## Final verification

Verification completed on 2026-07-20:

```text
node --test scripts/validate-openspec.test.mjs scripts/contracts.test.mjs
23 passed, 0 failed

openspec validate bootstrap-contracts-events --strict --no-interactive
Change 'bootstrap-contracts-events' is valid

openspec validate --all --strict --no-interactive
20 passed, 0 failed

node scripts/validate-openspec.mjs
23 specs, 105 requirements, 122 scenarios, 105 traceability

node scripts/validate-contracts.mjs
1 operation, 4 messages, 4 JSON schemas, compatibility=checked, 33070 ms

npm audit --audit-level=high --registry=https://registry.npmjs.org
found 0 vulnerabilities

git diff --check
exit 0
```

## Archive review

Local implementation, verification, supply-chain, offline, rollback, applicability, and traceability evidence are complete. GitHub is used only for code hosting, so workflows and required remote checks are intentionally absent. The authoritative local gate checked the merged `origin/main` baseline and returned `compatibility=checked`.

The change is approved for archival. No Schema, generator, CI adapter, or runtime work remains in this change.
