## Verification Evidence

Date: 2026-07-22

### Target And Compatibility

- Context: `kind-graphify`; API endpoint: `https://127.0.0.1:44040`.
- Server: Kubernetes v1.36.1, containerd 2.3.1, one Ready control-plane node.
- Client: kubectl v1.36.2; Provider: Go 1.24 and client-go v0.34.5.
- CNI: kindnet; default StorageClass: local-path `standard`.
- Stable `apps/v1` Deployment create/get/delete was verified directly against the server.

### Automated Tests

```text
HNB_TEST_KUBECONFIG=$HOME/.kube/config GOTOOLCHAIN=go1.24.5 GOPROXY=https://goproxy.cn,direct go test -count=1 ./...
GOTOOLCHAIN=go1.24.5 GOPROXY=https://goproxy.cn,direct go test -race -count=1 ./...
GOTOOLCHAIN=go1.24.5 GOPROXY=https://goproxy.cn,direct go vet ./...
```

All passed. Tests cover strict HTTP decoding, namespace denial, unmanaged ownership conflict, idempotent replay, fencing conflict, UID-precondition delete, and a real kind create/replay/conflict/delete sequence.

### Independent In-Cluster Provider E2E

The scratch/non-root image `hnb-kubernetes-provider:e2e` built successfully and ran as an independent Pod with readiness `1/1`. `/healthz` returned `ok`. The original kind v0.27 environment required node-local `ctr` import because it could not parse containerd config v4. After upgrading to kind v0.32.0, `kind load docker-image` was retested successfully against the v1.36.1 node and the image was visible through CRI without the workaround.

RBAC checks:

```text
create deployments.apps in hnb-workloads: yes
delete deployments.apps in hnb-workloads: yes
create secrets in hnb-workloads: no
```

Calling the deployed Provider through its Service produced:

- First deploy: HTTP 200, `succeeded`, UID `68d3aa15-eb81-4ce7-8141-adf827757305`.
- Exact replay: HTTP 200 with the same UID and resourceVersion, proving no duplicate creation.
- Different fencing token: HTTP 409, `deployment idempotency or fencing token conflicts`.
- Delete with expected resource fence: HTTP 200, `deleted`, same UID.

All E2E Deployments, Provider manifests, test namespaces, and port-forward processes were removed afterward.

### Specification And Formatting

`openspec validate --all --strict` passed 26 items with zero failures. `git diff --check` passed.

## N/A And Operations

- Database/event migration: N/A; Provider has no Operation persistence or event publication.
- Backup/restore: N/A for the stateless Provider; Kubernetes/etcd policy owns target backup.
- Upgrade: publish a new immutable image, rerun conformance for main/minor compatibility changes, then roll the Provider Deployment.
- Rollback/uninstall: remove the Worker mapping and Provider manifests. Managed workloads are intentionally retained.
- Residual risk: UUID fencing tokens are conflict identifiers, not ordered generations. Cross-lease takeover remains fail-closed until the Operation Engine supplies a monotonic fencing generation.
- Environment compatibility: kind v0.32.0 with Kubernetes v1.36.1/containerd 2.3.1 successfully supports `kind load docker-image`; the prior v0.27 workaround is no longer required.
