# Contracts supply-chain and offline evidence

## Locked inputs

- `contracts/toolchain.lock.json` fixes Node.js, npm, Go, Buf, all generators, lint tools, download URLs, source commits, SHA-256 values, and direct binary licenses.
- `package-lock.json` fixes every npm artifact by version and integrity digest and records its SPDX license metadata.
- Direct binary licenses are Go `BSD-3-Clause` and Buf `Apache-2.0`. Go-installed tools are oapi-codegen `Apache-2.0`, protoc-gen-go `BSD-3-Clause`, and oasdiff `Apache-2.0`.
- Direct npm tool licenses are Buf Protobuf `(Apache-2.0 AND BSD-3-Clause)`, protoc-gen-es `Apache-2.0`, Redocly `MIT`, AJV `MIT`, ajv-formats `MIT`, openapi-typescript-codegen `MIT`, tsx `MIT`, and TypeScript `Apache-2.0`.

## Vulnerability scan

The approved registry scan completed on 2026-07-20:

```text
npm audit --audit-level=high --registry=https://registry.npmjs.org
found 0 vulnerabilities
```

The initial scan found a high-severity `fast-json-patch` advisory through `ajv-cli`. The CLI was removed in favor of direct AJV 8.20.0 validation; the follow-up scan above is the acceptance result.

## Release Bundle

An offline Linux x64 bundle contains these paths from a successfully bootstrapped checkout:

```text
contracts/toolchain.lock.json
package.json
package-lock.json
.tools/contracts/
node_modules/
```

Create the bundle and its external digest from the repository root:

```bash
tar --sort=name --mtime='UTC 1970-01-01' --owner=0 --group=0 --numeric-owner \
  -czf hnb-contract-tools-linux-x64.tar.gz \
  contracts/toolchain.lock.json package.json package-lock.json .tools/contracts node_modules
sha256sum hnb-contract-tools-linux-x64.tar.gz > hnb-contract-tools-linux-x64.tar.gz.sha256
```

Restore it into a checkout with matching Schema and generation configuration, verify the external digest, then run:

```bash
sha256sum --check hnb-contract-tools-linux-x64.tar.gz.sha256
tar -xzf hnb-contract-tools-linux-x64.tar.gz
HNB_CONTRACT_TOOLS_OFFLINE=1 node scripts/validate-contracts.mjs
```

`HNB_CONTRACT_TOOLS_OFFLINE=1` makes the bootstrap fail if a download, Go tool install, or npm install would be required and sets `GOPROXY=off` during generation.

## Offline drill

The cached full gate completed without a download or package installation:

```text
Contract gate passed: 1 operations, 4 messages, 4 JSON schemas,
compatibility=no-baseline, 26792 ms
Stage timings: environment=1422ms, schema=6378ms, compatibility=63ms, drift=18922ms
```

The Release Bundle is an external release artifact and remains excluded by `.gitignore`; it is not committed to the source repository.
