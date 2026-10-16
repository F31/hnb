# 4.2 Streaming registry and OCI Chart source adapters

- task: 4.2
- requirements: DPLY-003, DBND-001
- date: 2026-10-12
- recorded-at: 2026-10-12T06:22:27Z (after registry observation)
- status: partial; authorized bootstrap preload remains unimplemented

## Implemented increment

RegistryResolver now exposes one internal streaming response path used by both
lock identity resolution and the preparation downloader. It negotiates anonymous
repository-pull Bearer challenges, limits token response size, validates the scope,
and applies the lock's sourceRedirectOrigins to content redirects. It does not
buffer image layers in the resolver. Operator/password credentials for private
repositories are not accepted or inferred by this increment.

Tokens stay bound to scheme + host, including an explicitly declared same-host
HTTPS-to-HTTP redirect. CDN replies cannot trigger registry token acquisition.
Redirects are bounded to four hops; undeclared origins and non-HTTP(S) destinations
refuse. Only origins, never signed queries or tokens, enter download progress.
Docker-Content-Digest headers are checked against the locked content identity.

OCI Chart preparation treats the release-lock digest as the packaged tar.gz digest
(the same interpretation as lock generate), not a guessed manifest digest. The
OCI locator resolves a Helm manifest, its content layer must match the locked
package digest/size, and actual Chart.yaml name/version must match. A warm package
cache is rehashed and inspected without looking up the mutable locator tag again.

## Real registry download (not a platform acceptance run)

Reused the previously declared source policy, through the user-required external
proxy. Command executed from cmd/hnbctl:

```bash
HTTPS_PROXY=http://127.0.0.1:7892 \
HTTP_PROXY=http://127.0.0.1:7892 \
NO_PROXY=127.0.0.1,localhost \
HNB_PREPARE_E2E_REF=registry.aliyuncs.com/google_containers/pause:3.10 \
HNB_PREPARE_E2E_REDIRECT_ORIGINS=http://aliregistry.oss-cn-hangzhou.aliyuncs.com \
go test ./internal/toolkit -run '^TestPrepareRealRegistryClosure$' -count=1 -v -timeout=3m
```

The first real attempt failed `invalid selected manifest` (11.42 seconds). A real
single manifest omits the index's manifests field; unmarshalling into the reused
index struct retained the old slice. The parser now resets the struct before
decoding the selected manifest. The fixture was changed to omit that field too.
This failed attempt is retained and is not reclassified as a successful download.

After the fix, the same command exited 0 in 16.20 seconds:

```text
root=sha256:0ca1162b75bf9fc55c4cac99a1ff06f7095c881d5c07acfa07c853e72225c36f
manifest=sha256:7c38f24774e3cbd906d2d33c38354ccf787635581c122965132c9bd309754d4a
artifacts=4 verifiedBytes=319634 coldCacheHits=0
warmCacheHits=4; image bytes only, no node import or platform certification
PASS
```

Cold means a new empty installer cache in this test, not empty OS/cluster/runtime
state. All four index/manifest/config/layer objects were streamed, hashed and
atomically cached. The second pass rehashed four hits. The temporary cache was
cleaned by Go's test lifecycle. No remote node, runtime import, package install,
Chart installation or complete HNB release was involved. Source is the current
uncommitted workspace, not a certified build/release identity.

## Automated regression coverage

- Bearer scope negotiation plus actual TLS CDN streaming; no token or signed query
  appears at the CDN/in progress, and progress records the actual CDN origin.
- Declared same-host HTTP redirect receives no HTTPS registry token.
- OCI Helm package is pinned by its real tar.gz checksum and metadata.
- Wrong package digest refuses before Chart blob download.
- Warm OCI Chart/package cache issues no new network request.
- Existing retry, interruption, corrupt-cache, architecture, source mapping,
  local release-package, cancellation and preload-refusal tests remain passing.

```text
cd cmd/hnbctl
go test -race ./... -count=1      PASS
go vet ./...                     PASS
```

## Remaining 4.2 scope

Authorized node preload still needs the Day-0 ownership and durable receipt/fencing
contracts from 4.4/4.5. --preload continues refusing before any node/cache action.
Private registry operator credential references and repository dependency adapters
remain unsupported. No offline capability or full platform install is certified.
The task checkbox stays open; overall task completion remains 20/48.
