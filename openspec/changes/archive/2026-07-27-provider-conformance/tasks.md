# Tasks: provider-conformance

## Summary
| | |
|---|---|
| **Change** | `provider-conformance` |
| **Created** | 2026-07-27 |
| **Specs** | provider-conformance (PROV-001~PROV-005) |
| **Status** | In Progress |

## 1. Database Migration

- [ ] 1.1 030_provider_conformance_core.sql: provider_manifests, provider_compatibility_matrix tables

## 2. Provider Manifest Model (PROV-001)

- [ ] 2.1 ProviderManifest struct in pkg/core/registry with validation
- [ ] 2.2 Manifest validation: required fields, actions, permissions, compatibility range
- [ ] 2.3 Manifest CRUD in Platform API store (Create/Get/Update/Delete)

## 3. Platform API: Manifest Registration (PROV-001)

- [ ] 3.1 POST /v1/providers/{id}/manifest endpoint
- [ ] 3.2 GET /v1/providers/{id}/manifest endpoint
- [ ] 3.3 PUT /v1/providers/{id}/manifest endpoint (re-register after upgrade)
- [ ] 3.4 Manifest validation error responses

## 4. Conformance CLI Tool (PROV-004)

- [ ] 4.1 cmd/provider-conformance/ skeleton with CLI flags
- [ ] 4.2 Contract test runner: validate Provider implements declared actions
- [ ] 4.3 Functional test runner: execute standard lifecycle scenarios
- [ ] 4.4 Fault test runner: simulate network failure, timeout, invalid payload
- [ ] 4.5 Security test runner: check fencing token, idempotency, tenant isolation
- [ ] 4.6 Performance baseline runner: measure latency and throughput
- [ ] 4.7 JSON report output with pass/fail/skip per test case

## 5. Compatibility Matrix (PROV-005)

- [ ] 5.1 CompatibilityMatrix model and CRUD in store
- [ ] 5.2 GET /v1/compatibility endpoint query by core_version + provider_id
- [ ] 5.3 POST /v1/compatibility endpoint to register compatibility entries
- [ ] 5.4 Pre-execution check: validate Provider compatibility before Operation dispatch

## 6. Conformance Status Integration

- [ ] 6.1 Provider registration: conformance_level field (none/basic/production_ready)
- [ ] 6.2 Conformance expiration check: periodic job to downgrade expired providers
- [ ] 6.3 Operation pre-check: reject execution if Provider conformance expired

## 7. Unit Tests

- [ ] 7.1 Manifest validation tests
- [ ] 7.2 Compatibility matrix query tests
- [ ] 7.3 Conformance CLI test runner tests
- [ ] 7.4 Conformance status integration tests