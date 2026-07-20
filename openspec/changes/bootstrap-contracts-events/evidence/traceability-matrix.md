# Contracts foundation traceability and archive review

## Requirement matrix

| Requirement | Source Schema and mapping | Generated output | Automated and compatibility tests | CI, documentation, and rollback | Status |
|---|---|---|---|---|---|
| CONTRACT-001 Schema First | `contracts/openapi/foundation/v1/openapi.yaml`, `contracts/proto/hnb/contracts/v1/contracts.proto`, `contracts/schema/common/v1/*` | `contracts/generated/go`, `contracts/generated/typescript` | lint, AJV validation, generation compile, drift, semantic alignment, dependency-boundary and interoperability tests | `contracts-quality-gate.yml`; public contract guide; rollback report | Local evidence complete; remote CI evidence pending 6.1 |
| CONTRACT-002 compatibility | versioned `v1` paths/packages and JSON Schema `$id`; oasdiff, Buf breaking, and `jsonSchemaBreakingChanges` | generated packages retain source version boundaries | field removal, type change, required addition, enum removal, Protobuf removal/number reuse, and explicit major-version fixtures | evolution/deprecation guide; complete release-set rollback test | Local evidence complete; remote baseline starts after first merge |
| CONTRACT-003 idempotency and correlation | OpenAPI correlation/idempotency/If-Match parameters; RequestContext and EventEnvelope fields across formats | Go and TypeScript headers/message types | missing-header failures; cross-format semantics; Go-to-TypeScript UUID, timestamp, unknown-field, idempotency and correlation checks | public contract guide and unified CI gate | Foundation complete; runtime deduplication remains downstream scope |
| CONTRACT-004 transactional event boundary | `contracts/mappings/outbox-event-envelope.json` and broker-neutral EventEnvelope | generated envelope types | Outbox field mapping test rejects Broker subject and sequence leakage | database migration and Relay are explicitly N/A for this static change | Mapping contract complete; runtime Outbox remains downstream scope |
| CONTRACT-006 repository and gate | toolchain BOM plus all three source Schema trees | deterministic Go/TypeScript output and `TOOLCHAIN.json` | 13 contract tests; environment/schema/compatibility/drift exit paths; online/offline gate; zero drift | unified workflow, developer guide, supply-chain report, rollback report | Local evidence complete; branch rule and first Actions URL pending 6.1 |

## Final verification

Verification completed on 2026-07-20:

```text
node --test scripts/validate-openspec.test.mjs scripts/contracts.test.mjs
23 passed, 0 failed

openspec validate bootstrap-contracts-events --strict --no-interactive
Change 'bootstrap-contracts-events' is valid

openspec validate --all --strict --no-interactive
22 passed, 0 failed

node scripts/validate-openspec.mjs
25 specs, 105 requirements, 122 scenarios, 105 traceability

node scripts/validate-contracts.mjs
1 operation, 4 messages, 4 JSON schemas, compatibility=no-baseline, 29721 ms

npm audit --audit-level=high --registry=https://registry.npmjs.org
found 0 vulnerabilities

git diff --check
exit 0
```

## Archive review

Local implementation, verification, supply-chain, offline, rollback, applicability, and traceability evidence are complete. Archival is not yet approved because task 6.1 still requires the workflow to be pushed, its first successful GitHub Actions URL to be recorded, and `Validate Contracts` to be configured as a required `main` check. The repository also has no merged Contracts baseline yet, so `compatibility=no-baseline` remains the correct result.

After task 6.1 is complete, rerun the final verification commands, update compatibility evidence if `origin/main` contains Contracts, mark task 7.3 complete, and archive the change. No Schema, generator, or runtime work is otherwise outstanding.
