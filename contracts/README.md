# HNB Public Contracts

This directory owns cross-process schemas and generated transport types.

- `openapi/`, `proto/`, and `schema/` are the only contract sources of truth.
- `generated/` is replaced only by `node scripts/generate-contracts.mjs`.
- Services map generated DTOs to private domain models and must not expose database models.
- Contract changes must pass `node scripts/validate-contracts.mjs`.
- Runtime code must not depend on `.tools/`; it is a local, digest-verified build cache.

No contract grants authorization or permits direct RuntimeTarget writes. Tenant scope is
validated by the owning service, and all target mutations still require an Operation.

## Cluster Management Authorities

- `openapi/console/v1/openapi.yaml` is the browser/BFF authority for cluster list/detail/nodes, the four state dictionaries and compatibility aggregate, typed cluster RuntimeIntent submission, Operation list/detail/actions, the server-driven navigation catalog (`/api/v1/navigation/menus`, UI 规范 V2.6 §6), declarative PageSchema delivery (`/api/v1/schema/page/{id}`, V2.6 §7), and RFC 9457 problems.
- `schema/runtime-target/v1/` owns ordered RuntimeTarget observations, source reset, bounded capability/node sections, planner-authored lifecycle Step inputs, and the exact compatibility matrix.
- `schema/console/v1/cluster-dictionaries.json` owns labels and semantic tokens. `resource.cluster.status` is display-only compatibility aggregation; lifecycle, health, connectivity, and freshness remain authoritative independent dimensions.
- Browsers may provide only typed lifecycle intent fields and SecretReferences. They cannot select a Provider, Step, command, execution endpoint, callback URL, fencing value, or tenant identity.
- `node scripts/generate-contracts.mjs` generates every contract. Use `--cluster-management` to update only console and RuntimeTarget outputs, and combine it with `--check` for a focused drift gate.

## Workload Storage Authority

- `openapi/storage/v1/openapi.yaml` owns dedicated storage desired-state and read-only inventory endpoints with their `x-hnb-authorization` tuples.
- `schema/storage/v1/` owns storage inventory, backend, offering, binding, and condition domain shapes.
- `STORAGE_API_AUTHORIZATION.md` defines resource-ID scoping and deprecates the generic Kubernetes proxy for storage management.
- Storage mutations are intentionally absent until typed intent, ExecutionPlan, and Operation contracts are introduced.

## Service Credential Consumers

- Service credential consumers bind one subject/audience/permission with `schema/identity/v1/service-credential-scope.schema.json`; the scope is not workload authentication.
- Owning-plane registration uses `schema/identity/v1/service-identity-create.schema.json`; issuance/renewal responses use `schema/identity/v1/service-credential.schema.json`, whose credential value is `writeOnly` and must never be persisted or audited.
- The apiserver exposes restricted issue/renew/revoke endpoints with `serviceIdentity` authorization tuples. The BFF continues per-request delegation; worker/Provider deployment wiring and real cross-TTL drills remain pending. See `docs/deployment/service-credential-renewal.md`.
- Agent onboarding uses `schema/identity/v1/agent-enrollment-create.schema.json` (single-use secret, `writeOnly`), `agent-enrollment-exchange.schema.json`, `agent-session.schema.json` (session ID/generation), and `agent-session-revoke.schema.json`. The exchange is atomic and bound to the exact tenant/target; renewal keeps the session/generation and revocation closes the exact tunnel connection.

## Product Readiness Evidence

- `schema/readiness/v1/product-readiness.schema.json` owns offline stage plans, approved numeric budgets, release/environment identity, and retained run evidence.
- `node scripts/validate-product-readiness.mjs --plan PLAN --evidence RUN --root ARTIFACT_ROOT` verifies schema, scope, stage minima, report bytes, and budget observations without changing the registry or granting execution authority.
- See `docs/acceptance/product-readiness-manifests.md` for the trust boundary and `npm run readiness:test` for refusal-path tests. This is an offline JSON contract; it does not add an HTTP API or generated transport client.
- `APP_MARKET_ARTIFACT_STORAGE.md` defines the ownership boundary: workload offerings/bindings are only Kubernetes persistent-volume/StorageClass contracts and never object buckets or App Market `ArtifactStorageProfile` records.
