# Storage Provider Conformance Matrix

The version-bound storage suite is implemented by `cmd/provider-conformance/storage_matrix.go`. It reuses the provider-conformance `TestResult` report contract and fixture convention; it does not implement or certify a vendor Provider.

Run it with:

```sh
go test ./cmd/provider-conformance/...
go run ./cmd/provider-conformance \
  -storage-matrix cmd/provider-conformance/testdata/storage-matrix.v1.json \
  -storage-evidence cmd/provider-conformance/testdata/storage-evidence.v1.json
```

The normative fixture contracts are:

- `cmd/provider-conformance/testdata/storage-matrix.v1.json`: suite, matrix, HNB Core and Provider protocol versions; generic CSI, NFS/static PV, Ceph, cloud disk and local PV capability/lifecycle cells.
- `cmd/provider-conformance/testdata/storage-evidence.v1.json`: package/version/Kubernetes binding, package claims, observed supported or rejected evidence, planner/provider routing, idempotency, fencing, rollback metadata and persisted metadata samples.

Every capability cell is closed: `Supported` requires matching package and observed evidence, while `Unsupported` requires explicit rejection evidence. Every lifecycle cell is also closed: supported install/upgrade/uninstall actions must route the planner Step to the package Provider, replay idempotently, reject stale fencing generations and preserve rollback metadata for install/upgrade; unsupported actions must not route a Step.

Persisted plans, Operations, events, audits, logs and Provider outputs are recursively checked for inline password, token, kubeconfig, private-key or Secret-value fields. `SecretReference` metadata is permitted.

The included fixtures set `providerImplemented=false` and `productionReadyRequested=false`. They prove the matrix contract only. The CLI deliberately reports storage fixture runs below `production_ready`; requesting Production Ready from fixture evidence fails closed, and T2 Ceph, cloud disk and local PV tiers are additionally ineligible for initial T1 readiness. Real certification must replace fixture references with current execution evidence from an implemented Provider bound to the exact package, Kubernetes, matrix, suite, Core and protocol versions.
