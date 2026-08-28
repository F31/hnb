# Task 13 Service Identity Evidence

Date: 2026-07-27

Status: **complete; task 13 is checked**.

## Implemented

- Public identity contracts now define `ServiceIdentity` as the verified
  workload/service projection of `AccessTokenClaims`. Service credentials carry
  one exact target audience, non-wildcard tenant scope, `allowedActions`, and
  signed `scopedPermissions`; user tokens cannot become service credentials.
- `pkg/iam` provides `ServiceAuthenticator`, `RequireServiceIdentity`, and
  `FileTokenSource`. Verification rejects user subjects, multiple/wrong/wildcard
  audiences, wrong/wildcard actions, wrong/wildcard tenants, expired tokens, and
  missing scoped permission before the protected handler runs. Token files are
  reopened for every call, bounded to the access-token size, required to be
  regular mode-0600-equivalent files, and never persisted to a database.
- `operation-worker` runtime provider entries require an endpoint, explicit
  audience, and token file. The driver reads the provider-specific token for
  each execution and sends it only as `Authorization: Bearer`. Missing identity
  configuration or token read failure prevents the HTTP request.
- The only HTTP runtime providers currently selected by the worker deployment
  are Kubernetes and Edge. Their `/v2/steps:execute` servers load public
  verification keys and require workload/service identity, exact provider
  audience, `execute`, and a tenant/provider permission matching the execution
  payload. `/healthz` alone remains anonymous. `gateway-provider` is NATS-driven,
  not an HTTP runtime provider selected by `RUNTIME_PROVIDERS`.
- Tunnel authentication occurs before WebSocket upgrade. The client sends the
  credential in `Authorization`, never in the URL; legacy query and
  `X-Auth-Token` credentials are not accepted. Apiserver and standalone tunnel
  server require workload/service identity with exact
  `hnb-apiserver-tunnel`, `execute`, tenant, and cluster permission. The cluster
  agent reads `AGENT_TOKEN_FILE` on every reconnect. The existing Kubernetes
  proxy stub remains unimplemented as required for the Phase 3 boundary.
- `pkg/messaging` provides one NATS connection policy supporting credentials
  files, NKey seed files, or mTLS client certificate/key/CA paths. Missing
  credentials fail closed unless `NATS_INSECURE=true` is explicitly set.
  Every production NATS client uses it. Docker Compose declares insecure mode
  explicitly for local development. Helm and the minimal/lite-HA NATS manifests
  reference per-client mTLS secrets, certificate-to-user mapping, and scoped
  ACLs for each client.
- Tunnel sessions retain the server-verified credential expiry. The server
  closes an established WebSocket when that deadline is reached; reconnects
  reread the token file before authentication.

## Verification

- `go test -race ./...` passed in `pkg/iam`, `pkg/tunnel`, `pkg/messaging`,
  `cmd/operation-worker`, `cmd/kubernetes-provider`, `cmd/edge-provider`,
  `cmd/cluster-agent`, `cmd/tunnel-server`, `cmd/apiserver`, `cmd/app-market`,
  and `cmd/gateway-provider`.
- `go test -race -count=1 ./...` also passed in `cmd/alert-manager`,
  `cmd/extension-controller`, `cmd/network-provider`, `cmd/network-registry`,
  `cmd/calico-provider`, `cmd/cilium-provider`, and `cmd/kube-ovn-provider`.
- Tests cover user-token rejection, wrong/multiple audience, wrong action,
  wrong/wildcard tenant/action, expiry, missing/over-permissive/rotated token
  files, no-handler-on-auth-failure, worker Bearer propagation and token-source
  failure, tunnel URL redaction/query rejection, and NATS missing-credential
  fail-closed behavior.
- `npm run contracts:generate` passed and regenerated Go/TypeScript Protobuf
  bindings.
- `npm run contracts:check` passed: 16 tests, schema/OpenAPI/Buf compatibility,
  and generated-artifact drift checks.
- `openspec validate phase-1-trusted-entry-and-write-path --strict
  --no-interactive` passed.
- `docker compose -f deploy/docker-compose/compose.yml config --quiet` passed
  with required non-secret fixture paths supplied.
- `git diff --check` passed.
- Helm lint was not run because `helm` is not installed in the environment;
  templates were inspected statically.

## Residual Scan

The production-source residual scan finds direct `nats.Connect` only inside
`pkg/messaging`, where the credential and mTLS policy is applied. The remaining
call in `pkg/integration` is a test-environment helper, not a production client.
The NATS server manifests and subject ACL documentation include distinct users
for every migrated client.

Signing-key rotation state machine (task 14) has been implemented; task 20 and task 25 remain unchecked.
