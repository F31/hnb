# 7.1 Offline material preparation, verification and local import

- task: 7.1
- requirements: DBND-001, DBND-003
- date: 2026-10-12
- status: done (local material lifecycle; offline installation remains uncertified)

## Implementation

`bundle prepare/verify/import` now operates on actual pinned cache files, with
CLI/Chart/image index-manifest-config-layer/tool/node-package/migration and
release identities. The actual preparation cache layout is supported; the early
split layout remains accepted as an input. Import always publishes flat paths.

OS indexes are independently pinned `package-index` node artifacts declaring
OS/architecture, package roots, versions/digests and transitive dependencies.
Preparation fetches/copies the reachable dependencies; bundle construction and
verification enforce that the declared graph is closed and every locked package
root agrees. Every target requires an index and the lock must pin a pause sandbox
image. This is a publisher-declared dependency graph, not an apt/dnf solver.

Build and verify share lock-driven closure derivation. Verify does not rely on
the bundle's self-reported Required list: root image manifests, selected
architecture manifests, descriptors, package indexes and payload bytes are
rechecked against pinned content. Logical records are keyed by role/name/digest
so sharing bytes does not erase required identities. Paths escaping the source,
symlinks, special files, duplicate contracts, size mismatches and invalid OCI
documents refuse. Material identity includes the lock and canonical object
identities/sizes, and remains stable across relocation.

Import now actually streams/copies, hashes, syncs and atomically publishes local
cache objects, then verifies its destination and saves bundle.json plus
release.lock.json. It refuses invalid source before destination creation; verified
retries rehash cached objects. It never writes nodes, installs software, imports
runtime images or falls back to a public network.

CLI output includes offlineInstallCertified=false. Local closure verification is
not evidence of a cold-cache/forbidden-network platform installation. Tasks 7.2
and 7.3 remain open and require their independent environments.

## Failures corrected during implementation

The earlier Required field insertion had not landed, preventing compilation. The
earlier build skipped requirements on parse failure, used a role-presence rather
than lock identity check, and omitted node/transitive package artifacts. The old
import command only set imported=true without copying material. Those paths were
replaced and are explicitly tested. Unrealistic success fixtures that omitted
OCI config/layer descriptors were replaced with descriptor-backed fixtures.

## Commands actually executed

```text
cd cmd/hnbctl
go test ./... -count=1               PASS (CLI/toolkit)
go vet ./...                        PASS
go test -race ./... -count=1         PASS (CLI/toolkit)
go build ./...                      PASS
```

New/strengthened tests cover:

- Deleting each required logical entry fails verification; deleting each physical
  fixture object fails verification and import before destination creation.
- Removing both Required and delivered records and repairing public hashes still
  fails the independent pinned-closure check.
- Pinned OCI index/selected linux/amd64 manifest closure and missing selected
  manifest, invalid OCI docs, unresolved transitive package dependencies.
- Transitive package preparation actually copies an uncached dependency.
- Traversal, absolute paths, symlinks, duplicate entries, wrong sizes, missing lock
  binding and incomplete contracts refuse.
- A real local import survives deletion of the source directory; destination
  objects are 0600 and its directory 0700, with independent release identity.
- Verified import retry succeeds; an invalid source causes no destination writes.
- CLI prepare/verify/import/verify passes with HTTP transport explicitly disabled,
  recording zero HTTP calls. Import without a destination cannot report success.

These are synthetic material/OCI/package fixtures with hashes calculated from
their actual bytes; no real apt/dnf installation, registry runtime import, node
restart or platform/HA delivery is claimed.

Operational material format and commands:
`docs/deployment/offline-bundle.md`.
