# Contracts rollback rehearsal

## Scope

The rehearsal used the current release set rather than treating generated files as an independent rollback unit:

- source Schema: `contracts/openapi`, `contracts/proto`, and `contracts/schema`;
- tool locks: `contracts/toolchain.lock.json` and `package-lock.json`;
- generation configuration: `package.json`, `redocly.yaml`, and `scripts/generate-contracts.mjs`;
- generated output: `contracts/generated`.

## Drill

The automated test copied the complete release set into an isolated sandbox, modified one OpenAPI source, the tool lock, generation code, and generated toolchain metadata, then restored every release path from one snapshot. SHA-256 maps for every restored file matched the snapshot.

```text
node --test --test-name-pattern="contract release rollback" scripts/contracts.test.mjs
pass 1, fail 0
```

The unified gate then confirmed that the repository release set remained coherent and buildable:

```text
Contract gate passed: 1 operations, 4 messages, 4 JSON schemas,
compatibility=no-baseline, 25674 ms
Generated contracts match committed output
```

The drill does not use a database, Broker, runtime deployment, or partial generated-code rollback. Those concerns are outside this static contract foundation.
