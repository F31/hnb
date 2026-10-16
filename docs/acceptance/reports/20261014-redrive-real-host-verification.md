# Orphaned step redrive — real-host verification

## Final status — passed for task 5.10

Run `redrive-v4-aa8be79c`, 2026-10-14 **01:35:09–01:37:24 UTC**, recovered a
genuinely unconsumed, injected-lost request through the real worker and lifecycle
provider to `succeeded`. Duplicate delivery and exhaustion were checked separately
through **01:43:37 UTC**. OpenSpec task 5.10 is complete.

Environment: `106.75.34.133`, Ubuntu 24.04, 2 vCPU/3914 MiB, existing single-node
K3s; warmed local images and existing database/NATS/identity/conformance setup.
Base commit: `c23f6e4128e92d7a9457807dce5b10e0b0704e73` plus working-tree changes.
Exact build-input hashes and binary/archive identities are retained in
`20261014-redrive-artifacts.json`.

### Verified observations

| Assertion | Retained observation |
| --- | --- |
| Correct code actually running | OCI content hash, imported target, requested image and running Pod `imageID` all resolve to `sha256:c1fb52bd0f206018dbe081ca4f00ddc30fd09ab6e9c96ff83e6078016af8bf7a` |
| Real admission and deterministic injection | Secret registration HTTP 201, unique-key Intent HTTP 202; worker fully terminated before SQL marked exactly **one** pending request published (`UPDATE 1`) |
| Lost request recovered | Operation `36d171ec-5721-41fd-a35e-4e33345881e6`: queued/pending → succeeded, completed_steps=1/1; step `0edfc722-f39e-43e2-9595-65f7ed25c20f` executed at 01:37:19 UTC |
| Identity preserved | One request row, message ID `0688d61b-98aa-420e-a74f-8aa15c4a946c` unchanged; key matches stored step; delivery attempt 0 → 1 |
| Duplicate reaches worker without re-execution | Republished the same request after the two-minute broker duplicate window. At 01:39:25 worker logged `operation 36d171ec... is terminal (succeeded); acknowledging stale command`. Step start/completion timestamps, result and completed_steps stayed unchanged; still one request row (attempt=2 after this explicit duplicate injection) |
| Exhaustion remains explicit | `659bffac-4d3e-4d14-9ffc-87cd61508f75`, step `ef0e3c85...`: pending, attempt=3, max_retries=3, max_attempts=10; repeated exhaustion logs at 01:37:19/01:38:19/01:39:19 |
| Old derived rows handled | Archived then deleted exactly 12 published `#redrive-*` rows belonging to four allowlisted test Operations; canonical rows and original failed terminal states retained |

### Additional defects found and fixed during completion

1. **False image verification.** The old apply script hard-coded `redrive-v2` as its
   source and tagged it with the v3 digest name. Merely finding that name in
   `ctr images ls --quiet` passed, while the running Pod showed v2's actual image ID
   `sha256:3b79d54a...`. Consequently the previous run still inserted derived rows.
   `20261014-redrive-verified-worker-deploy.py` now verifies actual OCI content
   hashes, the source's amd64 manifest and the running image ID. Using v2 for the v4
   digest is rejected before deployment; the formerly misleading v3 alias was also
   repaired only after proving the real v3 source contains its expected manifest.
2. **Candidate selection.** The SQL lacked its documented unpublished-request
   exclusion and selected the latest row even if it carried a derived key. It now
   excludes steps with any unpublished request and selects only the request whose
   key equals `operation_steps.idempotency_key`.
3. **Outbox budget off-by-one.** Re-arming increments attempt, but relay requires
   `attempt < max_attempts`. The sweep now checks room for that increment, so it
   cannot create a permanently unpublishable pending row at the limit.
4. **Stale Provider binding.** The first correctly deployed v4 injection recovered
   request delivery but failed execution: the plan pinned current manifest
   `sha256:ec30de8b74af5ab6fc1d3e14c8f78a4ca421a504cf63715fee2b3941b3589e92`,
   while the worker still pinned `sha256:52277423...`. Read the current digest from
   the formal manifest API's `X-Provider-Manifest-Digest` header and updated worker
   configuration. The failed Operation `616eee38...` and four recovered historical
   Operations remain **failed**, not rewritten to success. The successful run used
   a new Intent and fresh idempotency key.
5. **Test fixture lifetime/replay.** Each run uses unique credential/idempotency
   names, fresh admin login after worker termination, waits 125 seconds for idleness,
   then stages a fresh **60-second** evaluation service token immediately before
   restarting the worker. The test asserts `UPDATE 1`, saves JSON progress and
   restores worker replicas in `finally`.

The previous suspicion that `max_attempts` was zero was disproved: observed rows
had attempt=0/max_attempts=10 under the old build. Actual exhausted original now
has attempt=3/max_retries=3. SSH routing through local tunnel eth5
(`198.18.0.2`) was also confirmed; pinning a host route through physical eth2
(`10.0.0.129`) restored SSH. Clearing proxy variables alone cannot bypass a TUN route.

### Commands and evidence

On the evaluation host, as root (all three completed with exit code 0):

```sh
python3 20261014-redrive-verified-worker-deploy.py --archive /home/ubuntu/redrive-v4.tar --manifest /home/ubuntu/worker-provider.yaml --source docker.io/hnb-fix/operation-worker:redrive-v4
python3 20261014-redrive-recovery.py
python3 20261014-redrive-followup.py
```

The successful recovery ran after refreshing the worker binding from the formal
manifest API; the retained recovery script now performs that preflight itself
(`--refresh-binding` was separately verified against the same digest).

- `20261014-redrive-recovery.json`: successful injection/recovery, before/after.
- `20261014-redrive-binding-mismatch.json`: retained earlier failed injection.
- `20261014-redrive-followup.json`: duplicate acknowledgment, unchanged execution
  timestamps, historical failure states, archive count and exhaustion logs.
- `20261014-redrive-final-state.json`: real image identities, wrong-source rejection,
  corrected old alias and actual exhausted-step budgets.
- `20261014-redrive-artifacts.json`: build-input, binary, OCI archive and evidence hashes.
- Private full-row archive: `/home/ubuntu/redrive-v4-legacy-outbox-archive.json`
  (root-owned, 0600), SHA-256
  `7208acae434a80a51ee7e25e402300d7938244ff156d6ffe57c4493ea6ceb1ad`.

Local checks: worker `go test -race ./... -count=1` with PostgreSQL fixtures passed;
new regressions cover canonical selection, published+unpublished exclusion and the
outbox budget edge. The existing shared-consumer/relay integration and envelope
contract passed with real PostgreSQL and NATS in an empty fixture database. Its
initial run against the reused database failed to reach the target outbox event
within the test's ten-row drain limit; the isolated empty-database rerun passed.
The separate provider-lease-loss integration requires additional provider fixtures
and was not run locally. `git diff --check` and `openspec validate --all --strict`
passed (29/29).

Scope: this completes orphaned-step redrive only. Evaluation service-token issuance
and rotation, Agent/Read Model closure, complete Minimal installation, HA and offline
qualification remain outside this evidence. The over-long-key historical
`659bffac` remains pending and explicitly exhausted for operator handling; neither
its committed input nor its budget was rewritten.

## Historical partial status — superseded by the completed run above

**Partially verified.** The sweeper was deployed to the evaluation host and was
observed re-driving an orphaned step, enforcing its delivery budget and reporting an
exhausted step. End-to-end recovery to a *successful* execution is **not** verified:
the run was interrupted when SSH access to the host was lost.

Run ID: `startup-redrive-recovery-20261013`, base commit
`c23f6e4128e92d7a9457807dce5b10e0b0704e73` plus uncommitted working-tree changes.
Environment: `106.75.34.133`, Ubuntu 24.04, 2 vCPU/3914 MiB, single-node K3s.

## What the real host proved

1. The sweeper runs (`[redrive] starting orphaned step sweeper (idle=2m0s interval=1m0s limit=20)`)
   and re-drove the step orphaned the previous day: `requeued 1 orphaned step request(s)`.
2. The delivery budget is enforced and exhaustion is explicit rather than silent:
   `step ef0e3c85... exhausted its delivery budget and still has no worker; manual intervention required`.
3. Injection is deterministic. Scaling `operation-worker` to zero stops both the outbox
   relay and the consumer, so a step request can be marked published without ever
   reaching the stream; operation `0c4711d6-5c92-4c97-8d83-90cf3e8818dc` was created
   in that state and was re-driven.
4. Provider qualification fails closed on expiry. After the four-hour window lapsed,
   a new `ImportRuntimeTarget` was rejected with HTTP 503 `UPSTREAM_UNAVAILABLE`
   instead of executing against an unqualified provider.

## Defects found by this verification and fixed

| Defect | Evidence | Fix |
| --- | --- | --- |
| Sweeper never selected anything: the idle threshold was passed in milliseconds but cast to `interval`, which PostgreSQL read as **seconds**, so the condition could never be true | no re-drive after several sweeps on a step idle for hours | `now() - ($5 * interval '1 millisecond')`, plus a regression test that distinguishes 10s from 3m under a 2m threshold |
| Re-drive inserted a copy with a derived `#redrive-N` idempotency key, but `operation-worker` requires the envelope, the payload and `operation_steps` to carry the same key | `isolated message on hnb.failed.command.operation.step-requested.v1: envelope and payload idempotencyKey differ` | re-arm the existing outbox row (`status=pending`, `attempt+1`, `next_attempt_at=now()`), which the outbox attempt counter exists for |
| The API accepted client idempotency keys up to 128 characters, but the derived step key is always longer, so the worker refuses the step | `isolated message ...: idempotencyKey exceeds 128 characters` on an operation created the same day | `store.ValidateStepIdempotencyKey` rejects the intent before an Operation is created, instead of silently creating one that can never execute |

## Not verified at the earlier interruption (historical)

- A re-driven step reaching `succeeded`. The corrected worker image
  (`sha256:704d2088dde3ed428259098547cef986b38c2182b0a10b3bac1befe1214f3cd1`) is built and
  its unit plus isolated PostgreSQL integration tests pass, but it was not deployed.
- The injected-orphan operation `0c4711d6` and the long-orphaned `659bffac` are still
  `queued`; `659bffac` has exhausted its budget under the superseded build and needs the
  corrected worker (or operator action).
- A separate boundary defect surfaced by `659bffac`: its idempotency key was
  double-scoped by an earlier test-script error and exceeds the worker limit, so it can
  never execute regardless of re-drive. The admission fix prevents new occurrences; the
  existing row is retained as evidence.

## Regression coverage added after the run

- `TestRequeueOrphanedStepsUsesMillisecondThreshold` distinguishes 10s from 3m under a
  2m threshold, which the seconds/milliseconds mix-up could not have survived.
- The redrive now re-arms one row instead of adding copies, and the store test asserts
  the row count stays 1, the idempotency key is unchanged and the payload is untouched.
- `TestReArmedStepRequestPassesEnvelopeValidation` runs a re-armed request through the
  worker's own `validateStepRequest`: the re-armed form is accepted, a derived-key copy
  is rejected with `envelope and payload idempotencyKey differ`, and a 129-character
  step key is rejected with the 128-character limit. This is the gap that let the
  original design ship: store-level tests never exercised the worker's validation.
- `TestValidateStepIdempotencyKeyRejectsUndeliverableKey` covers the admission bound.

## Host access note at the earlier interruption (historical)

At the end of the run the host answered ICMP and accepted TCP/22 and TCP/443, but every
SSH session was closed during key exchange. No further host observation was possible,
so nothing above is inferred from a later state.
