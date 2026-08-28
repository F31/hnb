## 1. Contracts And Backend

- [x] 1.1 Add apiserver `GET /api/v1/schema/page/{id}` route, handler and service/repository interface for trusted schema pages. Covers CONTRACT-009.
- [x] 1.2 Add a built-in schema fixture that includes regions, endpoint/dataSource declarations, at least one action, texts and a permission condition. Covers UX-008, UX-009, CONTRACT-010.
- [x] 1.3 Add apiserver tests for successful schema fetch, missing page, unauthorized/missing permission behavior, and no executable/secret URL fields in responses. Covers CONTRACT-009, CONTRACT-010.
- [x] 1.4 Update apiserver OpenAPI documentation for the schema page endpoint and common error/trace response. Covers CONTRACT-009.

## 2. Shell Runtime Wiring

- [x] 2.1 Replace direct `fetch` in `SchemaPage.vue` with the shared authenticated API client and safe trace-aware error handling. Covers UX-008, CONTRACT-009.
- [x] 2.2 Wire `DataSourceManager` and trusted endpoint registration into schema page runtime, including paginated query support. Covers UX-008, CONTRACT-010.
- [x] 2.3 Wire `ActionEngine` callbacks for navigate, API/operation request, confirm, overlay placeholder and notification. Covers UX-008.
- [x] 2.4 Add controlled `ConditionEvaluator` and apply it to PageRenderer regions and action enabled/visible state. Covers UX-009.
- [x] 2.5 Add explicit minShellVersion upgrade-required UX for incompatible schemas. Covers UX-010.

## 3. Tests And Verification

- [x] 3.1 Add schema-engine unit tests for condition evaluation, unknown endpoint rejection, action execution through trusted endpoint ID, and incompatible schema UX mapping. Covers UX-008, UX-009, UX-010, CONTRACT-010.
- [x] 3.2 Add Shell component/store tests for SchemaPage loading through the shared API client and rendering filtered regions. Covers UX-008, UX-009.
- [x] 3.3 Add Playwright schema-page E2E smoke covering schema fetch, dataSource query, button action, and permission-denied/hidden control. Covers UX-008, UX-009, CONTRACT-010.
- [x] 3.4 Run `cmd/apiserver` Go test/vet/build, Web schema/Shell tests/typecheck, Playwright schema smoke, and `openspec validate --all --strict --no-interactive`. Covers all requirements.

## 4. N/A Checks

- [x] 4.1 Database migration: N/A for this change because schema pages use a trusted static repository in this slice.
- [x] 4.2 Event/NATS contract: N/A for this change because realtime schema/navigation invalidation is deferred to a separate runtime events change.
- [x] 4.3 Provider/Gateway/Edge conformance: N/A because this change only affects browser-facing Console schema runtime and apiserver BFF schema DTOs.
