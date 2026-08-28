## Verification Evidence

Date: 2026-07-22

### PostgreSQL And Store

- PostgreSQL 16 empty database executed migrations `001` through `013` in order without error.
- Migration 013 and rollback both rejected a nonempty `worker_leases` table.
- Empty rollback restored v1 column names and removed generation/FK/sequence; reapplying 013 succeeded.
- Store integration tests verified wrong Step version rejection, active Lease exclusion, deliberate sequence gaps, expiry takeover, retained Step generation after Lease deletion, wrong attempt/generation rejection, stale commit rejection, and atomic Step/Audit/ReadModel/Outbox commit.
- Audit details contain both `lease_id` and `fencing_generation`.

### Runtime Driver V2

- Only schema `2.0.0` is accepted at `POST /v2/steps:execute`.
- Generation is encoded as a canonical positive decimal string; execution attempt is a canonical non-nil UUID.
- Success and failure responses must exactly echo both values.
- Unknown/trailing/oversized fields, v1 schema, mismatched echoes, contradictory success, and incomplete typed failures were rejected in automated tests.
- Standard errors were classified into fenced, permanent, and transient behavior; Provider retry hints do not override client policy.

### Crash-Window Recovery

The real integration path used PostgreSQL, NATS JetStream, Operation Worker, HTTP Runtime Driver v2, an independently deployed Kubernetes Provider, and kind Kubernetes v1.36.1.

The first Provider call created a Deployment successfully. The test then expired the matching Lease before Worker result commit. The commit returned `ErrLeaseLost`, JetStream redelivered the command, and a new Lease received a greater generation and different attempt UUID. The Provider CAS-adopted the same Deployment and the second Worker attempt committed successfully.

Assertions passed:

- one Kubernetes Deployment and one UID;
- increasing generation and distinct attempt identity;
- one successful Step/Operation progress increment;
- one `step_completed` audit record;
- one completion Outbox event;
- stale Worker commit rejected.

### Kubernetes CAS And Tombstones

- Real kind takeover retained the same UID while advancing `hnb.io/fencing-generation`.
- Logical delete generation 9 scaled the Deployment to zero and retained its UID.
- Delayed deploy generation 8 returned HTTP 409 `FENCED` and did not resurrect the workload.
- Controlled redeploy generation 10 with matching `expected_uid` restored one replica with the same UID.
- Fake-client tests covered create/AlreadyExists, exact replay, malformed generation, stale generation, CAS conflict bounds, changed spec rejection, availability polling takeover, UID mismatch, tombstone replay, stale resurrection rejection, and controlled redeploy.
- Provider ServiceAccount can `update` Deployments and cannot `delete` them.
- Provider Pod reached Ready `1/1`; service `/healthz` returned `ok`.

### Automated Commands

```text
HNB_TEST_POSTGRES_DSN=... HNB_TEST_NATS_URL=... HNB_TEST_RUNTIME_PROVIDER_URL=... go test -race -count=1 ./...
HNB_TEST_KUBECONFIG=$HOME/.kube/config go test -race -count=1 ./...
go vet ./...
openspec validate --all --strict
git diff --check
```

Both Go modules passed race and vet. Strict OpenSpec validation passed 27 items with zero failures.

## Cutover And Rollback

1. Stop dispatch and all v1 Workers.
2. Require an empty `worker_leases` table and inventory ambiguous v1 Steps/resources.
3. Apply migration 013.
4. Deploy Provider v2, then Worker v2, while dispatch remains stopped.
5. Run v2 probes and a canary before resuming dispatch.

Rollback to v1 is permitted only before the first v2 target write and only with no active Lease. After activation, v1 cannot preserve target generations; recovery is roll-forward only.

## N/A And Known Limits

- New NATS schema: N/A; StepRequested remains v1 because generation is allocated after delivery.
- New middleware/database: N/A; the existing PostgreSQL database and Kubernetes API are reused.
- Telemetry subsystem: N/A for this focused safety change; generation/attempt are present in logs, audit, and Kubernetes annotations, while metrics remain a separate hardening change.
- Authentication: generation orders attempts but does not authenticate callers. Production mTLS/Provider authorization remains a separate P0 change.
- Logical tombstones intentionally retain Deployment metadata and consume no Pods after scaling to zero.
