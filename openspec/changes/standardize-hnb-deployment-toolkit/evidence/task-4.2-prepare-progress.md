# 4.2 Preparation increment

- task: 4.2
- requirements: DPLY-003, DBND-001
- date: 2026-10-12
- status: partial; task remains unchecked

Later source-adapter work supersedes the registry/CDN/OCI Chart limitations below:
see [streaming source adapters and actual registry download](task-4.2-source-adapters.md).
Anonymous pull-scoped Bearer authentication is supported; private operator
credentials and authorized node preload remain unsupported.

## 2026-10-14 second increment: authorized bootstrap preload (adapter-gated)

With 4.4's Day-0 ownership in place, the allowed bootstrap preload is now wired:

- `prepare` already downloads and digest-verifies every pinned image blob
  (manifest, config, layers) into the content-addressed private cache
  (`blobs/sha256/<hex>`), re-hashing cache hits.
- New `ImagePreloader` (toolkit) reassembles each pinned image as an OCI layout
  tar from the verified cache, re-verifying digest+size of every blob on the
  way out, and streams it into the blank node's containerd
  (`ctr -n k8s.io images import --digests --all-platforms`, temp file removed
  via trap). There is no standalone entry: only the bootstrap adapter calls it,
  after node-prereq succeeds, under the shared guard (`# hnb-bootstrap-stage=
  preparing`, machine-id check, permanent claim pinning, epoch fencing).
- New SSH primitive `RealSSH.Stream` / `SSHNodeProbe.Stream`: fixed script via
  `sh -c`, binary data on stdin; no caller data touches the command line.
- Failure semantics: preload failure records a retryable failed `preparing`
  phase and never blocks the bootstrap (online pull fallback); resume retries a
  failed preload and never re-executes a succeeded one. The run stays unfinalized.
- `prepare --preload` keeps refusing: authorization lives only inside a
  bootstrap run; install execution remains gated by 4.10 (real-registry blob
  cold/hot cache and node-side ctr import verification belongs to 5.4/5.5).

Tests: preload streams one guarded layout per pinned image before init
(ordering + tar layout + ref.name/digest assertions); failure is best-effort +
retryable + not re-executed after success. `go test -race ./...` (CLI/toolkit)
and `pkg/provision` race suite pass.

## Implemented

`hnbctl prepare -f hnb.yaml --cache-dir <private-dir>
[--source-dir <release-package>] [--mirror-origin https://registry]`

The CLI validates config/lock compatibility before preparation. It prepares
candidate materials without claiming an installable release or platform success.
It never reads SSH keys/kubeconfig, contacts nodes, joins a cluster, imports
runtime images, installs packages or writes an Operation/receipt. Managed and
offline branches refuse before downloads; --preload refuses before file reads.

The downloader prepares pinned image root/architecture manifests, config/layers,
node container closures, HTTPS package/tool sources and local CLI/Chart/migration
files. Local refs are release-package basenames (matching lock generate output).
Provide all named SQL files in that package; owner remains recorded in the result.

Cache is content-addressed at blobs/sha256/<hex>. Every hit is rehashed and any
wrong size/checksum is an error. Cache paths must be private regular directories,
not symlink paths. Unique 0600 temporary files, fsync and atomic rename prevent
partial bytes from entering verified cache. Interrupted transfers restart from
zero; the next run reuses only previously verified objects. This does not perform
HTTP Range continuation or infer that an incomplete object is valid.

Transient request/408/429/5xx/interrupted stream failures receive at most three
attempts with cancelable delay. Digest failure does not retry or publish bytes.
All HTTP requests are context-bound, normally TLS verified, and credential-free.
Manifest/config budget is 1 MiB; other objects default to a 1 GiB ceiling.

Progress is JSON Lines on stderr: actual source origin, artifact/digest, attempt,
downloaded bytes, total (0 if unknown), retry/cache-hit/verified states. Download
bytes are not called verified before complete digest/size validation. Stdout is
the preparation result only, with verified totals and installAuthorized=false.

Mirror mapping is an explicit HTTPS origin substitution retaining repository
paths and locked digests. There is no inferred tag equality or upstream fallback.
An index must contain exactly the locked linux/amd64 descriptor and the image
config must match its recorded platform. Remote response bodies/errors and
credential URLs are never echoed in failure messages.

## Tests actually executed

```text
cd cmd/hnbctl
go test ./... -count=1         PASS (CLI / toolkit)
go vet ./...                  PASS
go test -race ./... -count=1   PASS (CLI / toolkit)
```

New tests exercise:

- TLS fixture registry, locked OCI index/manifest/config/layer mapping and local
  Chart/CLI/migrations; all output paths private, totals complete.
- Second run uses verified cache with zero HTTP requests; corrupt cache refuses.
- Truncated stream followed by a successful retry; partial byte progress is not
  verified; no temporary download file survives completion.
- Three failing 503 attempts, then a new run reusing only the two completed
  manifest/config objects and fetching the remaining layer.
- Wrong digest, redirect, credential URL, canceled context, symlink cache and
  wrong architecture all refuse; invalid bytes are never published.
- Actual CLI execution against a warm synthetic cache yields JSON results and
  progress, without a kubeconfig file or any cluster action.
- --preload refuses before missing-config read/cache creation.

Fixtures use actual computed SHA-256 values and TLS streams, but are not a real
Harbor/OS/platform acceptance run. No production release or node preload is claimed.

## Remaining blockers (do not mark 4.2 complete)

- Allowed remote bootstrap preload cannot be wired before tasks 4.4/4.5 provide
  Day-0 ownership, immutable management-base identity and receipt/fencing. Current
  --preload returns TARGET_ACTION_UNSUPPORTED without executing SSH.
- Authenticated registries and declared CDN redirects are not supported by this
  streaming downloader yet; they fail explicitly. The lock resolver's anonymous
  token/source policy must be shared without buffering large layers or forwarding
  tokens across origins before these sources can be prepared.
- OCI Chart refs and non-HTTP package repository dependency resolution need their
  own source adapters. Local packaged Charts and pinned HTTPS bytes are supported.
- Complete OS dependency closure and cold-cache Harbor/node path acceptance remain
  separate from a locally verified material cache.
