# Alert Storm Stress Test Evidence

## Test Objectives

1. Verify dedup and aggregation work under high load
2. Verify tenant and channel rate limiting prevent overload
3. Verify Critical alerts are always processed first
4. Verify capacity budgets are respected

## Test Parameters

| Parameter | Value |
|-----------|-------|
| Concurrent sources | 50 |
| Events per second | 1000 |
| Unique fingerprints | 100 |
| Test duration | 5 minutes |
| Tenants | 5 |
| Channels per tenant | 3 (portal, email, webhook) |

## Expected Results

| Metric | Target |
|--------|--------|
| P95 source-to-firing | < 5s |
| P99 source-to-firing | < 10s |
| P95 firing-to-portal | < 2s |
| Max pending jobs | < 10000 |
| Critical alert latency | < 1s (always prioritized) |
| Max memory per worker | < 512 MiB |
| CPU per worker | < 1 core |

## Acceptance Criteria

1. All 1000 events/sec are processed without data loss
2. Only 1000 alert instances are created (100 unique fingerprints × 10 tenants)
3. No tenant's rate limiting starves another tenant
4. Critical alerts are delivered before warning/info
5. No worker crashes or OOM during the test
6. All metrics are within capacity budget

## Test Plan
1. Deploy 50 concurrent source emitters
2. Ramp up event rate from 100 to 1000 events/sec over 2 minutes
3. Sustain 1000 events/sec for 5 minutes
4. Measure P95/P99 latency for each pipeline stage
5. Verify alert instance count matches expected dedup
6. Verify no tenant chokes on another tenant's traffic
7. Generate report with P95/P99 values bound to version and environment