# Contracts foundation implementation evidence

## Environment and toolchain

- Date: 2026-07-20
- Node.js: 20.20.2
- npm: 10.8.2
- Isolated Go SDK: 1.26.5 (`.tools/contracts/go`)
- Buf: 1.72.0
- oapi-codegen: 2.8.0
- protoc-gen-go: 1.36.11
- protoc-gen-es: 2.12.1
- TypeScript: 7.0.2
- Redocly CLI: 2.39.0
- oasdiff: 1.23.0
- AJV / formats: 8.20.0 / 3.0.1

Versions, source commits, download URLs and available artifact SHA-256 values are recorded in `contracts/toolchain.lock.json`; npm package integrity is recorded in `package-lock.json`. The toolchain does not use Java, the broken system Go installation, global npm packages, or remote generation plugins.

## Schema and generation results

The following command completed successfully:

```bash
node scripts/validate-contracts.mjs
```

Result:

```text
Contract gate passed: 1 operations, 4 messages, 4 JSON schemas,
compatibility=no-baseline, 29721 ms
Tool versions: node=20.20.2, npm=10.8.2, go=1.26.5, buf=1.72.0,
typescript=7.0.2, redocly=2.39.0, oasdiff=v1.23.0
Stage timings: environment=1402ms, schema=9189ms, compatibility=81ms, drift=19042ms
```

Performance environment: 24 logical CPUs, 15 GiB memory (10 GiB available during measurement), Linux x64. The cached gate completed in 29.7 seconds, below the 120-second budget.

The gate verified:

- OpenAPI 3.1 lint and bundle;
- Protobuf Buf lint;
- JSON Schema Draft 2020-12 compilation and valid envelope example;
- required write headers and forbidden sensitive-value fields;
- Go OpenAPI and Protobuf generation plus `go test ./...`;
- TypeScript Fetch and Protobuf generation plus strict TypeScript compilation;
- generated dependency boundaries;
- committed generated output has zero drift.

`compatibility=no-baseline` is expected because `origin/main` does not yet contain a `contracts/` baseline. Once this change is merged, future branches must run oasdiff and Buf breaking against `origin/main`.

## Automated tests

```text
node --test scripts/validate-openspec.test.mjs scripts/contracts.test.mjs
23 tests passed, 0 failed
```

Contract tests cover stable gate error exits, complete release-set rollback, write headers, If-Match, sensitive fields, cross-format RequestContext semantics, generated dependency direction, failed atomic replacement, OpenAPI/Protobuf field removal, JSON Schema removal/type/required/enum changes, explicit major-version migration, Outbox mapping, and Go-to-TypeScript Protobuf interoperability with unknown-field preservation. OpenSpec tests continue to cover the repository governance gate.

## Architecture boundary

- Contract echo artifacts are build-time fixtures and perform no RuntimeTarget write.
- Generated packages contain transport DTOs only and import no service internals, database package or NATS type.
- Database migration, runtime E2E, Provider/RuntimeTarget/Gateway/Edge Conformance, backup and disaster recovery remain N/A for this static foundation.
