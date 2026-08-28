# Shadow Mode Migration Evidence

## Purpose
Run the new Alert/Notification system in parallel with the existing monitoring, comparing results without sending external notifications.

## Shadow Mode Flow

```
Existing Monitoring (Prometheus, etc.)
    |
    ├── Existing alerting pipeline (unchanged)
    └── New Source Adapter (read-only, shadow)
        |
        ├── Normalize event → compute fingerprint
        ├── Compare with existing alert state
        ├── Log differences (fingerprint, state, tenant mapping)
        └── NO external notifications sent
```

## Comparison Metrics

| Metric | Source | Expected |
|--------|--------|----------|
| Fingerprint match rate | New vs existing dedup | > 95% |
| State mismatch rate | New vs existing state | < 5% |
| Tenant mapping accuracy | New vs existing tenant | 100% |
| Notification count difference | New vs existing | < 10% |

## Shadow Mode Duration
- Phase 1: 1 week (observation only)
- Phase 2: 1 week (fix discrepancies)
- Phase 3: Ready for Portal-only rollout

## Exit Criteria
- Fingerprint match rate > 95%
- No false positives or false negatives in dedup
- Tenant mapping 100% correct
- All discrepancies documented and resolved