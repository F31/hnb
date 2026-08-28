# Tasks: observability-dr

## Summary
| | |
|---|---|
| **Change** | `observability-dr` |
| **Created** | 2026-07-27 |
| **Specs** | observability-dr (OBS-001~OBS-006) |
| **Status** | In Progress |

## 1. Database Migration

- [ ] 1.1 031_observability_dr_core.sql: operation_slo_config, operation_slo_alerts tables

## 2. Unified Telemetry Context (OBS-001)

- [ ] 2.1 Add structured telemetry helpers to pkg/observability (TenantID, CorrelationID, OperationID, ResourceID)
- [ ] 2.2 Add telemetry context to platform-api handlers
- [ ] 2.3 Add telemetry context to operation-worker execution

## 3. Operation SLO Monitoring (OBS-002)

- [ ] 3.1 OperationSLOConfig model and CRUD store
- [ ] 3.2 SLO check job: periodic query for stalled operations
- [ ] 3.3 SLO alert integration with existing alert-notification system
- [ ] 3.4 GET /v1/operations/slo endpoint to check active SLOs

## 4. Backup & Recovery (OBS-003)

- [ ] 4.1 Backup script: pg_dump with versioned naming
- [ ] 4.2 Restore runbook: documented recovery procedure
- [ ] 4.3 Makefile targets: make backup, make restore

## 5. Fault Drill Framework (OBS-004)

- [ ] 5.1 Fault drill scenario definitions for Lite HA / Standard HA / Enterprise
- [ ] 5.2 Automated drill runner: simulate Pod failure, node failure, DB failover

## 6. Performance Budget (OBS-005)

- [ ] 6.1 Performance baseline test definitions
- [ ] 6.2 Budget check in CI pipeline

## 7. Unit Tests

- [ ] 7.1 SLO config model tests
- [ ] 7.2 SLO check job logic tests
- [ ] 7.3 Telemetry context tests