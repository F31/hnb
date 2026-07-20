# HNB Public Contracts

This directory owns cross-process schemas and generated transport types.

- `openapi/`, `proto/`, and `schema/` are the only contract sources of truth.
- `generated/` is replaced only by `node scripts/generate-contracts.mjs`.
- Services map generated DTOs to private domain models and must not expose database models.
- Contract changes must pass `node scripts/validate-contracts.mjs`.
- Runtime code must not depend on `.tools/`; it is a local, digest-verified build cache.

No contract grants authorization or permits direct RuntimeTarget writes. Tenant scope is
validated by the owning service, and all target mutations still require an Operation.
