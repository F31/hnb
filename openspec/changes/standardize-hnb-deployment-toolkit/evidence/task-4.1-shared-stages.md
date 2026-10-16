# 4.1 Shared node stages and caller adapters

- task: 4.1
- requirements: DPLY-003, DPLY-004, PINST-001
- date: 2026-10-12
- status: done (shared implementation; installation/resume still pending)

The provision implementation and its four existing test files moved from the
Provider's internal directory to `pkg/provision`. The original import path now
exposes a compatibility adapter with type aliases/function bindings. Provider
callers and HTTP/secret-store behavior remain unchanged. Legacy CAPI helpers are
retained; combining CAPI management with SSH still fails before execution.

Caller adapters remain `SSHRunner`, `KeyResolver` (including ValidateVersion),
`SecretStore`, plus the new synchronous `ProgressObserver`. Stage events expose
only stage/node ID/UTC timestamps/command state. No script/stdout/stderr/error/
SSH key/join token/kubeconfig is copied into progress. The shared module imports
no platform SQL, IAM store or Provider package. The provider's encrypted Secret
bridges stay in internal/provider. Its version evidence check still precedes
all SSH execution.

## Verification executed

```text
cd pkg/provision
go test -race ./... -count=1 -timeout=2m    PASS
go vet ./...                             PASS

cd cmd/runtime-target-lifecycle-provider
go test -race ./... -count=1              PASS (main / internal/provider)
go vet ./...                             PASS
go build ./...                           PASS

cd cmd/hnbctl
go test ./... -count=1                    PASS
go build ./...                           PASS
```

Existing bootstrap, containerd-import, CAPI rejection and script regression tests
now exercise the shared implementation. Added tests prove:

- Ordered start/finish observations, real UTC timestamps and no credential output.
- SSH failure remains failed; remote error content does not leak into progress.
- Caller version evidence rejection causes zero SSH/progress side effects.
- `TestRealSSHConcurrentStdoutStderr`: an in-process x/crypto SSH server writes
  1000 markers on each stream concurrently; the actual RealSSH client receives
  all 2000 markers under `-race`, without executing any host shell command.
- The earlier 8-writer synchronized-buffer regression remains passing.

Module tidy used GOPROXY=off and cached third-party dependencies. Both caller
and shared module retain local workspace resolution.

Progress represents command observations, not durable/fenced receipts, an
Operation, or platform delivery success. Day-0 authorization, receipts and
resume remain tasks 4.4/4.5/4.6. This extraction does not enable uncertified HA,
new backends, or direct CLI execution of managed RuntimeTargets. No new remote
Ubuntu/Rocky installation was performed or claimed.
