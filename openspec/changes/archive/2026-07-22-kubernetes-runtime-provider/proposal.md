## Why

The Runtime Driver now has a real execution boundary, but no Provider can mutate a RuntimeTarget. The healthy local `kind-graphify` Kubernetes v1.36.1 cluster provides a controlled target for implementing and proving the first production-shaped Provider path.

## What Changes

- Change ID: `kubernetes-runtime-provider`.
- Add a T1, independently running Go Kubernetes Provider implementing Runtime Driver contract v1.
- Support only bounded `deploy` and `delete` actions for `apps/v1` Deployments; arbitrary YAML and cluster-scoped resources are excluded.
- Validate namespace, name, image, replicas, tenant scope, idempotency key, and fencing token before API writes.
- Mark managed resources with HNB ownership, Operation, Step, idempotency, and fencing annotations; reject ownership or fencing conflicts.
- Use Kubernetes API create/update/delete preconditions and wait for Deployment availability for observable completion.
- Add health endpoint, least-privilege RBAC/deployment manifests, fake-client tests, HTTP contract tests, and real kind E2E evidence.
- Dependency: completed `runtime-driver-integration`, `runtime-target-engine`, and `operation-engine-core` changes.
- Affected plane: runtime governance/control plane. No Portal, artifact data plane, AI, NATS, or Operation database sharing.
- Migration: additive binary and manifests only; no database/event migration. Rollback removes the Provider deployment and mapping, leaving managed workloads intact unless explicitly deleted.
- User value: an Operation Step can create and delete a real Kubernetes Deployment through the single governed write path.
- Non-goals: Service/Ingress/Gateway, StatefulSet, Secret material, Helm, arbitrary manifests, image pull, autoscaling, multi-cluster registry, and production mTLS.
- Compatibility: Kubernetes server v1.36.1 is validated using client-go v0.34.5 because it retains project Go 1.24 compatibility; stable `apps/v1` behavior is tested directly against v1.36.1.
- Security: namespace-scoped RBAC, no shell/kubectl invocation, no kubeconfig in request data, no secret logging, and explicit resource ownership checks.
- Resource budget: one HTTP server, one Kubernetes client, bounded request body, maximum replicas policy, and one rollout watch per request.
- Observability: structured standard logs with operation/step/resource identifiers, `/healthz`, HTTP status, and Kubernetes error classification without Inputs.
- Exit criteria: unit/contract/race/vet tests, strict OpenSpec validation, RBAC review, and real create/idempotent repeat/conflict/delete E2E on `kind-graphify` pass.

## Capabilities

### New Capabilities
- `kubernetes-runtime-provider`: Safe Deployment lifecycle execution over Runtime Driver v1 with Kubernetes ownership, idempotency, fencing, rollout observation, and least privilege.

### Modified Capabilities

None.

## Impact

- New module: `cmd/kubernetes-provider`.
- New deployment assets: namespace-scoped ServiceAccount, Role, RoleBinding, Deployment, and Service.
- Dependencies: Kubernetes `api`, `apimachinery`, and `client-go` v0.34.5; no new middleware or database.
- Install/upgrade/uninstall: apply or remove Provider manifests and Worker endpoint mapping. Kubernetes workloads are retained on Provider uninstall; backup/restore remains the target cluster's responsibility.
