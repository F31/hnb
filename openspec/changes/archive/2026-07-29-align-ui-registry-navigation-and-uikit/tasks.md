## 1. OpenSpec And Contract Alignment

- [x] 1.1 Validate this change strictly and keep proposal/design/specs/tasks in sync.
- [x] 1.2 Confirm bootstrap path alignment between platform-api `/v1/session/bootstrap`, compatibility `/v1/console/bootstrap`, and apiserver `/api/v1/session/bootstrap`. Covers CONTRACT-011.

## 2. DB-Backed Navigation

- [x] 2.1 Add PostgreSQL migration and rollback for Console plugin, route, navigation item, and navigation version metadata, including parent hierarchy and `sort_order`. Covers UX-006, PROV-007.
- [x] 2.2 Seed default Console navigation metadata in the database for local/test deployments with order stored in DB, not code. Covers UX-006.
- [x] 2.3 Implement apiserver DB-backed navigation repository and wire production handler to it; keep static/stub repository only for tests. Covers UX-006.
- [x] 2.4 Preserve server-side route permission/capability filtering and recursive parent pruning for database-backed metadata. Covers UX-006, PROV-007.
- [x] 2.5 Add Go tests for ordering, permission denial, capability denial, and parent pruning. Covers UX-006, PROV-007.

## 3. Bootstrap And Web Shell Cleanup

- [x] 3.1 Add platform-api contract-aligned `/v1/session/bootstrap` alias and apiserver BFF `/api/v1/session/bootstrap`. Covers CONTRACT-011.
- [x] 3.2 Update Web bootstrap/auth/context/permission/capability initialization to use apiserver and `@hnb/api-client` where touched. Covers CONTRACT-011.
- [x] 3.3 Remove Web hardcoded top-menu ordering and home/sidebar decisions; render from API data shape/metadata. Covers UX-006.
- [x] 3.4 Ensure plugin `menuItems` and plugin self-registered routes are not used as authoritative Shell navigation. Covers UX-006.

## 4. UI Kit Baseline

- [x] 4.1 Add or enhance reusable UI Kit primitives for page shell, toolbar, button, table actions, select/date inputs, form field, detail panel, and action bar. Covers UX-011.
- [x] 4.2 Register stable UI Kit primitives in Schema Engine ComponentRegistry with props validation. Covers UX-011.
- [x] 4.3 Add representative tests for new UI Kit primitives and Schema registration. Covers UX-011.

## 5. Verification

- [x] 5.1 Run apiserver/platform-api Go test/build for touched packages.
- [x] 5.2 Run Web ui-kit/schema-engine/shell typecheck and focused unit tests.
- [x] 5.3 Run Console Playwright smoke for login, bootstrap, navigation, and home/sidebar behavior.
- [x] 5.4 Run `openspec validate --all --strict --no-interactive`.
- [x] 5.5 Rebuild/restart local containers and verify navigation menus come from DB-backed metadata with permission filtering.

## 6. N/A Checks

- [x] 6.1 New external middleware: N/A; this change uses existing PostgreSQL and existing apiserver/platform-api boundaries.
- [x] 6.2 NATS/KV runtime invalidation: deferred; version/ETag behavior remains compatible with future event-driven invalidation.
