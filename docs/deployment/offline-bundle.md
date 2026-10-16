# Offline material bundles

The shipped bundle commands prepare, verify and import **local material**. They
do not install Kubernetes/HNB, change a node, import images into containerd, or
certify offline deployment. Ubuntu/Rocky forbidden-public-network installation
and restart acceptance remain tasks 7.2/7.3. JSON output explicitly records
`offlineInstallCertified: false`.

## Inputs and identities

Use a validated release lock that pins all selected images, Chart packages, CLI
binaries, tools, migrations, node packages and package dependency indexes. Each
target OS/architecture needs a matching `package-index` NodeArtifact. The lock
must also pin a container artifact with a fully qualified `.../pause:<tag>` ref.
Currently image closure selection is linux/amd64; other architectures refuse.

Cache objects use `blobs/sha256/<64 hex characters>`. The old split layout
`blobs/sha256/<first 2>/<remaining 62>` is accepted as source input; import always
publishes the flat layout. Symlinks, special files and paths escaping the bundle
are refused. No arbitrary directory scan is used to discover inputs.

The dependency index is JSON, with this structure (replace the symbolic digests
with SHA-256 values computed from the actual bytes before locking/publishing):

```json
{
  "schemaVersion": "os-package-closure/v1",
  "os": "ubuntu-24.04",
  "arch": "amd64",
  "roots": ["kubeadm.deb"],
  "packages": [
    {
      "ref": "kubeadm.deb",
      "version": "1.36.5",
      "digest": "${KUBEADM_SHA256}",
      "dependencies": ["libc.deb"]
    },
    {
      "ref": "libc.deb",
      "version": "${ACTUAL_PACKAGE_VERSION}",
      "digest": "${LIBC_SHA256}",
      "dependencies": []
    }
  ]
}
```

The publisher must derive this declaration from the target OS/package recipe;
the tool is not an apt/dnf dependency solver. The index itself is SHA-256 pinned
in the release lock as `kind: package-index`. Locked `kind: package` roots must
be represented in its reachable graph with the same digest. Transitive packages
are pinned by the locked index. A dependency absent from the index or from the
cache fails validation. OS/architecture mismatches and duplicate package/index
identities also refuse.

## Prepare material

While online, prepare the pinned release into a private cache:

```bash
hnbctl prepare -f online-deployment.yaml \
  --cache-dir /srv/hnb/staging \
  --source-dir /srv/hnb/release-package
```

For a package index, this preparation also copies/downloads all reachable pinned
dependencies. Package refs may be release-package basenames or pinned HTTPS
locators supported by the downloader. It does not install packages.

For an explicit registry mirror, use `artifacts.mode: mirror` and provide its
credential-free HTTPS origin. Repository paths and all locked digests remain
unchanged; the downloader does not fall back to the original registry:

```bash
hnbctl prepare -f mirror-deployment.yaml \
  --cache-dir /srv/hnb/staging \
  --source-dir /srv/hnb/release-package \
  --mirror-origin https://registry.company.example
```

Replace that example origin with the actual mirror. The mapping applies to OCI
images and OCI Charts, not arbitrary HTTPS package locators. Cross-origin CDN
redirects must be explicitly listed in the lock's `sourceRedirectOrigins`; their
content still has to match the locked digest. Mirror support here is for local
material preparation; it does not authorize mirror-mode installation.

Preparation writes a JSON result to stdout and JSON progress events to stderr.
The result includes `status` (`verified`, `failed` or `canceled`), `lockDigest`,
start/finish timestamps, verified artifacts/byte totals and cache hits. A download
failure still outputs the verified partial result and returns a nonzero exit code.
Retry the same command after fixing the source; completed cache entries are
rehashed and reused, while partial transfers restart into private temporary files.
Corrupt cache objects are refused rather than silently overwritten.

Progress timestamps, source origins, attempt numbers, actual downloaded bytes and
retry/failure reasons allow interrupted transfers to be diagnosed. URLs with signed
queries and registry tokens are not included. `installAuthorized` remains false;
`prepare --preload` remains blocked pending the Day-0 bootstrap authorization in
tasks 4.4/4.5.

Build the bundle manifest from the populated cache:

```bash
hnbctl bundle prepare \
  --lock release.lock.yaml \
  --cache-dir /srv/hnb/staging \
  --manifest /srv/hnb/staging/bundle.json \
  --format json
```

`--os-package-index` and `--sandbox-image` are optional file locators **inside**
the source directory when those objects are not already in canonical cache
paths. Their digests must match corresponding artifacts in the release lock.
They cannot substitute unpinned content or waive a missing requirement.

## Verify and import offline

Transport the source cache, bundle manifest and trusted release lock into the
isolated installer environment. Verification never downloads missing content:

```bash
hnbctl bundle verify \
  --lock release.lock.yaml \
  --cache-dir /media/hnb-bundle \
  --manifest /media/hnb-bundle/bundle.json \
  --format json

hnbctl bundle import \
  --lock release.lock.yaml \
  --cache-dir /media/hnb-bundle \
  --manifest /media/hnb-bundle/bundle.json \
  --target-dir /var/lib/hnb/material-cache \
  --format json
```

Import verifies the whole source before creating its destination, then streams
and rehashes objects into private temporary files, syncs and atomically publishes
them, and re-verifies the destination. It records `imported: true` only after
actual local import. It keeps `bundle.json` and `release.lock.json` beside the
material; these remain usable after removing the original source directory.
An existing destination cache must be private. Verified retry hits are rehashed;
corrupt objects or a conflicting destination manifest refuse rather than being
silently overwritten.

```bash
hnbctl bundle verify \
  --lock /var/lib/hnb/material-cache/release.lock.json \
  --cache-dir /var/lib/hnb/material-cache \
  --manifest /var/lib/hnb/material-cache/bundle.json
```

## Closure checks

The manifest records both required identities and delivered entries, but verify
recomputes requirements independently from the trusted lock, hashed OCI
index/manifest descriptors and hashed OS dependency indexes. Removing a
requirement and its entry, even while recomputing the public manifest hash, does
not bypass this check. Shared OCI blobs retain all their logical role/name
references. Descriptor sizes, config architecture, object bytes, relative paths,
release identity, contract and bundle identity must all agree.

Exit 0 means the local declared material closure check/import succeeded. It does
not imply a certified release, sufficient resources, successful OS installation,
cluster readiness, platform login, authenticated Agent observation or managed
Operation completion. Missing/corrupt content produces a nonzero result before
any node action; there is no public-network fallback.
