# Tasks 7.1-7.6 Operation BFF and Operation Center

## Operation BFF (apiserver)

`cmd/apiserver/internal/handler/operation_forward.go` implements the
browser-facing Operation Read Model and action forwarding:

- `ListOperations` forwards `GET {platform}/v1/operations` (page/pageSize/
  status/type) and maps to the console `OperationListResponse`
  (`apiVersion`, `items`, `pagination{page,pageSize,total,pageCount,exactTotal}`).
- `GetOperation` forwards `GET {platform}/v1/operations/{id}` and maps to the
  console `OperationDetailResponse` (`executionPlanId`, `steps`,
  `allowedActions`, `links{operation,intent,target}`).
- `OperationApprove/Reject/Cancel` forward the corresponding action with the
  actor's reason and return the refreshed detail projection.
- Mapping derives `targetId`/`targetKind` (with a tenant-scoped
  `runtime_targets` lookup), `progress.percent`, `safeFailure`, and
  `allowedActions` from the operation status machine
  (`pending_approval → [approve, reject]`, running → `[cancel]`, terminal → `[]`).

Every forward signs a short-lived operation-scoped delegation
(`pkg/iam` `hnb.delegation/v1`, `ResourceKind=operation`, `ResourceID`,
action, correlation). The BFF never mutates Operation state or publishes
commands itself; it only calls the canonical platform-api versioned API.

## Platform delegation for operations

`cmd/platform-api/internal/api/server.go`:

- `isOperationDelegationPath` routes operation list/detail/action requests that
  present a trusted service delegation; non-delegation requests fall through to
  the normal access-token middleware (backwards compatible).
- `delegationOperationEvidence` verifies the delegation matches the operationId
  and action, then re-resolves the actor's **current** permissions via the
  permission resolver (authority), rejecting evidence mismatch with 401.
- `pkg/iam/delegation.go` `validateDelegationEvidence` now accepts the
  `operation` resource kind (UUID resourceId + action; list carries no id; no
  intentKind/semanticDigest).

## intentId on the read model

- Migration `058_operation_read_model_intent_id.sql` adds `intent_id` to
  `operation_read_model`, backfilled from `operations.runtime_intent_id`
  (forward/rollback verified on PostgreSQL 16). The platform operation
  responses now carry `intentId` so the console can deep-link to the originating
  intent.

## Frontend (web/plugins/resource · cluster-management)

- `types/operation.ts`: `OperationStatus`, `OperationAction`, `OperationSummary`,
  `OperationDetail`, `SafeFailure`, `isTerminalStatus`.
- `api/operationApi.ts`: `listOperations`, `getOperation`, `operationAction`,
  and the shared `createOperationPoller` with 2s start / 15s cap, ±20% jitter,
  hidden/offline pause, resume reread on visibility, unload/component cancel,
  and terminal stop.
- `schemas/operation.list.ts`: Operation Center L2 `PageSchema`
  (`resource.operation.list`) + columns + status filter options.
- `OperationList.vue`: service-side pagination/filter/exact-total list,
  loading/error/empty states, row → detail navigation.
- `OperationDetail.vue`: steps table, progress (ui-kit semantics),
  `allowedActions` (approve/reject/cancel), safe failure banner, and
  Operation/Intent/Target deep links; drives the shared poller and stops on
  terminal status.
- Plugin registration: routes `/resource/operations` and
  `/resource/operations/:operationId` (permission `operation:list`/`read`),
  menu item, and `setOperationApiClient` injection.
- ClusterList submission modal now offers "前往跟踪" → Operation Center deep link
  (`/resource/operations/{operationId}`).

## Supporting fix

- `HNBPagination.vue` (ui-kit) no longer imports `vue-i18n`; label props are
  explicit, which is the ui-kit-first rule (task 2.3 compliance) and unblocks
  downstream typechecks.

## Verification

- `go build ./...` + `go test ./...` in `cmd/apiserver` and `cmd/platform-api`
  (incl. `operation_delegation_test.go` and `operation_forward_test.go`): pass.
- `pkg/iam` delegation validation tests: pass.
- Migration 058 forward applied idempotently to PostgreSQL 16.
- `pnpm typecheck` + `pnpm build` in `web/plugins/resource`: pass.
- Vitest from `web/` root: `plugins/resource` polling tests (3) and `ui-kit`
  tests (32) pass.

## Scope note

This delivers the Operation BFF/Center code-level evidence (forwarding,
delegation re-auth, read-model intentId, polling client, L2 schema, L3 detail).
Browser E2E (task 11.6/11.7) still requires the full live stack.
