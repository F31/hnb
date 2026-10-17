# Shared provisioning stages

This module is the single implementation of the existing kubeadm/SSH node
executor. `cmd/runtime-target-lifecycle-provider/internal/provision` is a
compatibility adapter; platform SQL/Secret bridges remain in `internal/provider`.
The moved script/transport/regression tests belong beside this implementation.

Caller-owned adapters:

- `SSHRunner`: node transport and host trust policy.
- `KeyResolver`: credential resolution and version evidence validation before SSH.
- `SecretStore`: persistence of the observed admin kubeconfig.
- `ProgressObserver`: synchronous stage-start/command-finish observations, with
  node ID and UTC timestamps, without script/output/error/credential content.

Progress reports command completion, not a durable receipt or full platform
readiness. Caller authorization, immutable inputs, fencing and recovery belong
to the calling install/Operation workflow. In particular this module does not
authorize Day-0 CLI execution; that boundary and receipt are tasks 4.4/4.5.

Legacy CAPI helpers are retained for source compatibility. The executor still
rejects combining a management client with SSH and still accepts only one
control-plane node. Extracting this package does not certify HA, CAPI or any new
OS/backend. SSH output is synchronized for concurrent stdout/stderr copies.
