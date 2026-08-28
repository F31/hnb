# Storage Provider Metric Adapters

Provider metric adapters implement `pkg/storagemetrics.Adapter`. Each adapter descriptor declares one stable source and applicability for all six canonical metrics: capacity (`By`), usage (`By`), IOPS (`1/s`), throughput (`By/s`), latency (`s`), and health (`1`). A snapshot supplies `observedAt`; the platform derives freshness from the declared positive freshness window. Observations cannot override source and create new label values.

Normalization rejects undeclared capabilities, duplicate kinds, negative or non-finite values, values for unsupported/unknown capabilities, and values attached to `Elastic`, `Unknown`, or `NotReported`. A missing measurement becomes `NotReported` without a value. It never becomes zero or healthy.

The read model retains tenant-scoped stable target/resource references for API navigation and later alert evaluation. Prometheus intentionally exports only these bounded labels: `provider`, `metric`, `unit`, `source`, `freshness`, and `applicability`. Tenant IDs, PVC/PV names, volume handles, target IDs, resource UIDs, backend IDs, and offering/binding IDs are prohibited as metric labels. Detailed per-resource facts belong in the read model, not time-series identity.

This boundary does not define or install alert rules. Alert rule reconciliation remains separate.
