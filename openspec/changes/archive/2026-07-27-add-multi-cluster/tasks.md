# Tasks: add-multi-cluster

## Summary
| | |
|---|---|
| **Change** | `add-multi-cluster` |
| **Created** | 2026-07-24 |
| **Specs** | multi-cluster (MC-001~MC-005), gslb (GSLB-001~GSLB-004) |
| **Status** | In Progress |

## Task List

### T1: Database Migration
- [x] 017_multi_cluster_core.sql: clusters, cluster_heartbeats tables

### T2: Cluster Registry API (MC-001)
- [x] RegisterCluster, UnregisterCluster, ListClusters, GetCluster handlers in platform-api
- [x] Cluster heartbeat endpoint (PUT /api/v1/clusters/{id}/heartbeat)

### T3: Karmada Provider (MC-003)
- [x] New cmd/karmada-provider service (NATS worker + Karmada client)
- [x] PropagationPolicy generation from GatewayProfile / Deployment request
- [x] OverridePolicy support for per-cluster overrides
- [x] Health check relay: Karmada → HNB cluster heartbeat
- [x] Makefile (build, test, vet, docker)

### T4: Cross-Cluster Operation Support (MC-005)
- [x] Add target_cluster_ids to operations table (029_multi_cluster_operation_targets)
- [x] Update platform-api store to persist cluster targets
- [x] Update operation-worker to route steps to target clusters
- [x] Update read model to include cluster info

### T5: Scheduling Policy (MC-004)
- [x] ClusterSelector model (label/region/tenant matchers)
- [x] PropagationStrategy enum (Duplicated/Divide)
- [x] Validation and API integration

### T6: GSLB Controller (GSLB-001~GSLB-004)
- [x] New cmd/gslb-controller service
- [x] Cluster health probe engine
- [x] CoreDNS record management (via External-DNS API — DNSEndpoint CR)
- [x] Weight-based DNS record generation
- [x] Fault detection and auto-failover (consecutive thresholds + debounce)
- [x] Periodic reconciliation loop (PostgreSQL → probe → failover → DNS)
- [x] Cluster registry PostgreSQL integration
- [x] Karmada cluster status integration (GSLB-004 — optional Karmada data source)

### T7: Unit Tests
- [x] Cluster registry handlers
- [x] Karmada propagation policy generation
- [x] GSLB health probe and DNS record logic
- [x] Failover tracker (thresholds, debounce, reset)
- [x] Reconciler helpers (weight, DNS name, state mapping, karmada merge, health filter)
- [x] DNS manager (sanitize endpoint name)
- [x] Karmada client (status mapping)
- [x] Cluster store (integration tests with PostgreSQL)
- [x] Cross-cluster operation routing (scheduling policy validation)

### T8: E2E Tests
- [ ] Cluster registration → heartbeat → status aggregation: N/A, requires running PostgreSQL instance
- [ ] Multi-cluster deployment via Karmada: N/A, requires running Karmada control plane
- [ ] GSLB failover scenario: N/A, requires running CoreDNS/External-DNS