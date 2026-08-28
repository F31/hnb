## Why

Provider catalog APIs exist, but Provider installation, enablement, upgrade, rollback, health reconciliation and capability/menu registration are not yet owned by a dedicated controller. Without this boundary, platform-api can grow into a lifecycle executor and apiserver/Console can receive inconsistent extension state.

## What Changes

- Change ID: `wire-extension-controller-provider-lifecycle`
- Tier: T1 default delivery for Provider lifecycle; T0 consumes only registered capabilities and metadata.
- Impacted planes: extension-controller, platform-api Provider catalog, apiserver navigation, Operation worker, Provider registry, capability registry.
- Introduce/complete extension-controller as the owner of Provider Bundle lifecycle.
- Keep platform-api as Provider metadata/query/catalog API, not the installer/executor of Provider lifecycle.
- Provider lifecycle mutations create or correlate to Operation records and publish domain events through Outbox/NATS.
- Controller registers capabilities, raw navigation/menu/route metadata and health state for apiserver BFF consumption.
- Dependencies: boundary hardening and navigation BFF changes can proceed independently but integrate through shared metadata contracts.
- Migration impact: existing provider manifest endpoints remain as catalog APIs; lifecycle operations move behind extension-controller contracts.
- Rollback strategy: lifecycle operations are idempotent and versioned; failed upgrades can roll back to the previously active Bundle and capability snapshot.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `provider-conformance`: Provider lifecycle ownership, health reconciliation, capability registration and install/upgrade/rollback contracts.

## Impact

- Affected code: future `cmd/extension-controller`, provider manifest store, operation submission, NATS subjects, capability registry, apiserver navigation metadata clients.
- APIs/events: versioned ProviderLifecycleRequest/ProviderLifecycleEvent contracts with IDs, digests and SecretReferences only.
- Dependencies: PostgreSQL, Operation Store, Outbox, NATS; no new middleware.
- Security risks: Bundle install can introduce privileged workloads; mitigated by signature/digest verification, declared permission checks, admission policy and audit.
- Resource budget: controller reconciliation queues and bounded status cache; no large artifact proxying.
- Observability: lifecycle phase, install/upgrade duration, rollback count, health checks and conformance expiry.
- Exit criteria: Provider install/enable/upgrade/rollback/uninstall paths are idempotent, audited, Operation-correlated and update capability/navigation metadata safely.
