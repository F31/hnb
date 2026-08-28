# UI Kit Capability Matrix

## Scope

This evidence implements OpenSpec task 2.1 for `cluster-management-full-closure`. The audit covers:

- `web/packages/ui-kit`, including public exports, tokens, component APIs, and tests.
- `web/packages/schema-engine/src/builtins.ts`, `PageRenderer.vue`, `RegionWrapper.vue`, and the Schema Engine data-source contract.
- Cluster list/detail schemas and pages, `ClusterRegisterWizard`, and `ClusterNodesPanel`.
- Existing Operation UI surfaces and placeholders.

The required behavior is derived from UX-021 through UX-025 and the change design. Ratings used below are:

- **Exists**: reusable public capability is present with the behavior needed by these surfaces.
- **Partial**: a reusable base exists, but required behavior or Schema Engine exposure is incomplete.
- **Missing**: no reusable public capability exists.
- **Page-private**: behavior currently exists only as page-local markup/styles and is not an acceptable general solution.

## Capability Matrix

| Required primitive / behavior | Needed by | Existing capability | Missing or insufficient behavior | Reuse / extend decision | Accessibility and mobile gaps |
|---|---|---|---|---|---|
| Page shell and responsive action header | Cluster list/detail; Operation list/detail | **Exists.** `HNBPageShell` provides title, description, actions, body, and a 768px stacked header; Schema Engine registers `PageShell`. | Current cluster pages reproduce page headers instead of using it. Schema-driven pages also render a separate private header in `PageRenderer`. | **Reuse `HNBPageShell`; extend Schema Engine composition only if Schema title/actions cannot be passed through the builtin.** Do not add a cluster page shell. | Existing mobile stacking is suitable. Confirm heading hierarchy and action order when Schema regions are composed. |
| Buttons and action groups | All audited surfaces | **Partial.** `HNBButton` supports primary/secondary/ghost/danger, sizes, disabled, and loading; `HNBActionBar` and `HNBTableActions` compose buttons. | No public disabled-reason contract. `PageRenderer` emits native `.region-action` buttons, while cluster list/detail/wizard/nodes duplicate primary, secondary, danger, icon, retry, page, and text buttons. `EmptyState` and `ErrorState` also contain private native button styles. | **Extend `HNBButton` with an accessible disabled-reason mechanism if required, then reuse it everywhere. Replace native general action controls in Schema Engine and state primitives with `HNBButton`.** Domain code may decide labels, permissions, and actions only. | Loading has `aria-busy`, but no live announcement. Disabled controls do not expose why they are disabled. Focus-visible styling is not consistently defined. Mobile action bars stack, but local page action rows do not consistently do so. |
| Toolbar and filter layout | Cluster list; nodes; Operation list | **Exists for layout.** `HNBToolbar` wraps filters/actions and stacks at 768px. | Cluster list uses a private toolbar. There is no shared search/text input to compose with the toolbar. | **Reuse `HNBToolbar`; add missing general form controls to ui-kit.** Do not create cluster or Operation toolbar components. | Wrapped controls are mobile-friendly, but control labels and accessible search naming must come from shared inputs/form fields. |
| Text, number, textarea, checkbox, and secret-reference selection controls | Wizard; filters; STALE reason; table selection | **Partial.** `HNBSelectInput`, `HNBDateInput`, and `HNBFormField` exist. Error/help association works only for controls that consume the form-field injection. | No public text input, number input, textarea, checkbox, or secret-reference selector/composable option picker. The wizard and list use unassociated native inputs; wizard errors are not consistently connected with `aria-invalid`/`aria-errormessage`. | **Add generic text/number/textarea/checkbox primitives to ui-kit and compose a domain-owned SecretReference picker from shared controls and generated data.** Secret lookup and purpose filtering remain domain behavior, not a ui-kit component. | Required indicators are visual only. Native page controls lack consistent labels, error association, focus styles, and 320px sizing. |
| Remote data table | Cluster list; nodes; Operation list | **Partial.** `HNBTable` wraps Naive UI `NDataTable`, supports loading, remote pagination, row selection, row keys, and an empty slot. Schema Engine registers it as `DataTable`. | No explicit narrow-screen horizontal-scroll contract, table accessible name/caption, empty/error state API, sort/filter events, configurable page sizes, or disabled-action reason. Schema builtin exposes only columns/data/loading/schemaId/dataSource and does not expose pagination/selection. Cluster list and nodes use private native tables. | **Extend `HNBTable` and its Schema builtin for controlled server pagination/filter/sort, semantic naming, selection, and narrow-screen scrolling; reuse it for all three lists.** No page-private table is recommended. | Current ui-kit tests only assert props/mounting. There are no keyboard, axe, screen-reader-name, or narrow-viewport tests. Local tables rely on overflow only in cluster list; nodes can overflow the viewport. |
| Standalone controlled pagination | Cluster list; nodes; Operation list | **Partial.** Pagination is embedded in `HNBTable` through `HNBTablePagination`. | No exported standalone primitive, semantic previous/next labels, current-page announcement, controlled page-size options, disabled reason, or compact narrow-screen mode. `ClusterNodesPanel` tracks page state but renders no pagination controls. Cluster list implements private pagination buttons/select. | **Add an exported controlled pagination primitive and let `HNBTable` compose it; register it as a Schema builtin.** Reuse the same primitive in node and Operation panels. | Local glyph-only previous/next buttons have no accessible names. No `aria-current`, live result-count announcement, or tested 320/375px layout exists. |
| Dictionary-driven status badge | Cluster list/detail; nodes; Operation list/detail | **Partial.** `StatusBadge` supports six semantic tokens, text plus a non-color dot, and reduced motion for processing. It is registered as a builtin. | It accepts caller-provided label/semantic only; no dictionary-entry contract, unknown-code handling, optional timestamp, or composed four-dimensional target state. Cluster status and node status colors are page-local mappings. | **Reuse `StatusBadge` as the leaf primitive; add a reusable dictionary-status adapter/composition in ui-kit or Schema Engine that consumes server label + semantic token.** Four target dimensions remain data composition, not one lossy status. | Badge itself is readable without color. Processing changes are not announced. Combined state needs concise accessible text and wrapping at narrow widths. |
| Multi-dimensional status group | Cluster detail/list; node freshness | **Missing.** | No shared way to present lifecycle, health, connectivity, and freshness together while retaining `lastKnownStateAt`. Current single `status` model collapses STALE into lifecycle. | **Add a reusable status-group/composition primitive to ui-kit, built from dictionary-backed `StatusBadge` instances and optional last-known metadata.** Do not create `ClusterFourStateBadge` in the resource plugin. | Group/label relationships, stale warning announcement, text wrapping, and 320px reading order need tests. Do not rely on color or overwrite last-known values. |
| Description/detail list | Cluster detail; Operation metadata/detail | **Exists.** `DescriptionList` and `HNBDetailPanel` are public and responsive; both are Schema builtins. | Schema regions currently omit required `items`, so current cluster schemas cannot render these builtins without runtime mapping. Long IDs and deep links need richer values than the current scalar type. | **Reuse and minimally extend for link/custom-value slots or safe declarative value rendering if Operation deep links require it.** Fix data binding in Schema Engine rather than adding page-specific detail lists. | Mobile collapses to one column. Verify semantic `<dl>` grouping, long correlation IDs, and keyboard-operable links. |
| Tabs | Cluster detail; wizard source mode; Operation detail | **Page-private / Missing.** Cluster detail and wizard implement native tab-like buttons; no ui-kit export or builtin exists. | No controlled Tabs API, tab/panel IDs, `aria-selected`, roving focus, arrow/Home/End keyboard behavior, disabled reason, lazy panel behavior, or narrow-screen scrolling. Current local markup has `role=tab` but no associated tabpanels or selected state. | **Add controlled Tabs/TabList/TabPanel capability to ui-kit and register a safe Schema builtin.** Reuse it in detail, wizard mode selection, and Operation detail. | Full WAI-ARIA tabs keyboard behavior is missing. Narrow tab lists need horizontal scrolling without hiding focus or key actions. |
| Dialog / modal | Wizard; confirmation; submission result; Operation actions | **Page-private / Missing.** Three cluster surfaces duplicate fixed overlays and cards. No ui-kit dialog exists. | No teleport/layer contract, labelled/described dialog, initial focus, focus trap, Escape handling, backdrop policy, scroll lock, nested async state, or focus restoration. | **Add one reusable Dialog primitive to ui-kit.** The L3 wizard supplies domain body/steps; pages must not retain their own modal shell. Register only safe declarative dialog behavior in Schema Engine where needed. | Existing `role=dialog` + `aria-modal` is insufficient: dialogs lack `aria-labelledby`, focus containment/restoration, Escape behavior, and small-screen height/overflow handling. |
| Dangerous confirmation | Upgrade; unmanage; batch actions; STALE challenge; Operation approve/reject/cancel | **Page-private / Missing.** Cluster list/detail implement Promise-based private confirmation overlays. Schema shell falls back to `window.confirm`. | No impact list, danger semantics, non-preselected acknowledgement, optional bounded reason, server challenge context, async pending/error state, or safe retry. | **Build a reusable Confirmation composition on the shared Dialog, Alert, FormField, and Button primitives.** STALE challenge token/policy handling remains cluster domain logic; its general confirmation UI must not be page-private. | Existing dialogs do not trap/restore focus. Explicit acknowledgement must be keyboard accessible, unselected by default, error-associated, and remain operable at 320px. Async status needs announcement. |
| Alert / inline message | STALE warning; validation; offline; policy outcome; safe failures | **Page-private / Missing.** Pages use styled `role=alert` divs. | No severity variants, title/body/actions, dismissibility policy, `role=alert` versus `role=status`, stable live-region behavior, or dictionary/token-only visual contract. | **Add a reusable Alert primitive to ui-kit and a Schema builtin.** Reuse it for stale, offline, validation, safe Operation failure, and compatibility messages. | Avoid repeated assertive announcements on rerender. Add contrast checks, semantic icons/text, responsive actions, and screen-reader tests. |
| Loading and skeleton | All audited surfaces | **Partial.** `HNBSkeleton` exists, uses tokens, has reduced motion, `aria-busy`, and an accessible label; tables expose loading. | Page and panel loading states are plain text. Skeleton label is hard-coded Chinese and has no externally supplied label/live-region policy or table/detail shapes. | **Reuse and extend `HNBSkeleton` with localized label and reusable variants only where demonstrably shared.** Do not add cluster skeletons. | Reduced motion is handled. Add locale support, prevent duplicate announcements, and test representative table/detail shapes at narrow widths. |
| Empty state | All list/panel/action surfaces | **Partial.** `EmptyState` is public and a builtin with title, description, and action. | It uses a private native action button and has no no-action compact/block variants. Current pages duplicate empty markup. | **Reuse `EmptyState`; refactor its action to `HNBButton` and add minimal layout variants only if shared fixtures prove need.** | Ensure empty action has clear accessible text and wraps at 320px. The decorative symbol is correctly hidden. |
| Error state and retry | All queried blocks; Operation polling | **Partial.** `ErrorState` is public, a builtin, uses `role=alert`, and emits retry. RegionWrapper provides block isolation. | It uses a private native retry button. No safe failure code/trace slot, retry pending state, or distinction between initial load and polling degradation. `PageRenderer` and `SchemaPage` use separate private error placeholders. | **Reuse and extend `ErrorState` with `HNBButton`, optional safe metadata, and retry pending state; consume it in Schema Engine block isolation.** | Prevent repeated assertive announcements, associate safe trace metadata, retain last Operation state during polling failure, and test focus after retry. |
| No-permission state | Cluster list/detail/nodes/actions; Operation pages | **Missing.** | No dedicated primitive or builtin; existing pages mostly hide controls and do not render the required state. | **Add a reusable NoPermission state primitive to ui-kit and Schema Engine.** Permission decisions/data clearing remain in the page/data layer. | Must not leak protected data, must have a clear heading/message, and should move focus predictably when permission is revoked at runtime. |
| Offline state | Cluster list/detail/wizard/nodes/actions; Operation polling | **Missing.** | No dedicated primitive or builtin; no last-known-data marker or write-disabled affordance. | **Add a reusable Offline state/alert composition to ui-kit and Schema Engine.** Poll pause/resume and cache authority remain domain/data-source logic. | Announce transition without repeated noise, identify cached data as last-known, expose retry/reconnect action, and keep all writes disabled on mobile and keyboard paths. |
| Schema/provider incompatible state | Cluster list/detail/wizard/nodes/actions; Operation pages | **Missing.** Schema shell has private text for min-shell incompatibility only. | No reusable fail-closed state, required/current version details, safe recovery action, or zero-action guarantee at block level. | **Add a reusable Incompatible state primitive to ui-kit and Schema Engine and use it for shell, schema, target/provider compatibility.** | Must receive focus or be announced when replacing content, expose no stale write controls, wrap version details at 320px, and preserve readable recovery guidance. |
| Notifications and live status announcements | Submit/validation; Operation transitions; async actions | **Missing.** Schema shell dispatches `hnb:notify`, but ui-kit exports no notification or live-region primitive. | No severity/lifetime/action contract and no reusable polite/assertive announcer for accepted, approval, queued, running, terminal, retry, or validation states. | **Add shared notification presentation plus a live-region/announcer primitive or composable in ui-kit.** Domain code supplies authoritative state text; it must not infer success from HTTP 202. | Define deduplication and polite/assertive policy, preserve focus, support reduced motion, and avoid toasts as the only source of status. |
| Wizard step indicator and review layout | Cluster register/create wizard | **Page-private / Partial need.** The wizard has a two-step ordered list and uses `DescriptionList`-like private review markup. | No accessible current/completed-step semantics, plan-validation state, or responsive step overflow. A whole generic wizard framework is not proven necessary by this audit. | **Keep `ClusterRegisterWizard` as L3 domain composition. Reuse Dialog, Tabs, FormField/inputs, DescriptionList, Alert, states, and buttons. Extend ui-kit only with a small reusable step indicator if another workflow or shared fixture proves reuse.** | Current steps do not expose `aria-current`; source tabs are incomplete; fields lack robust error association; dialog focus and 320px form-grid behavior are missing. |
| Operation progress / step timeline | Wizard tracking; cluster actions; Operation detail | **Missing.** Cluster pages only display the submission receipt's intent/status/operation IDs. | No progress value/indeterminate mode, ordered step states, timestamps, safe failure, current-step semantics, cancellation/approval state, or terminal distinction. | **Add reusable Operation progress and step-list primitives to ui-kit as required by task 2.4; compose them in an L3 Operation detail/tracker.** Do not implement progress markup privately in cluster or Operation pages. | Use ordered semantic structure, text plus icons (not color only), `aria-current=step`, bounded live announcements, reduced motion, and a vertically readable 320px layout. |
| Operation list/detail/actions and deep links | Operation Center; all cluster lifecycle actions | **Placeholder / Missing.** No Operation Center route, schema, list, detail, or polling UI exists. `OperationApproval.vue` is only a heading/description placeholder; cluster pages show a receipt modal with no link or polling. | Entire L2 list and L3 detail composition is absent, including filters, steps, allowed actions, audit-safe correlation, target/intent links, polling degradation, approval/cancel confirmations, and terminal-state semantics. | **Build Operation Center from the shared primitives above: L2 Schema + `HNBTable`/Pagination/status/states, and L3 detail using DescriptionList, Tabs, OperationProgress, Alert, Confirmation, and links.** Only Operation-specific orchestration belongs in the page. | Requires keyboard/axe coverage for filters, rows, tabs, actions, polling announcements, focus restoration, and 320/375/768px layouts. |

## Schema Engine Exposure Gaps

The reusable component inventory is not sufficient unless Schema Engine can consume it safely:

1. `builtins.ts` currently registers only PageShell, Toolbar, Button, TableActions, SelectInput, DateInput, FormField, DetailPanel, ActionBar, DataTable, StatusBadge, MetricCard, DescriptionList, EmptyState, ErrorState, and Skeleton. Dialog/Confirmation, Alert, Tabs, standalone Pagination, multi-status, NoPermission, Offline, Incompatible, notification/live-region, and Operation progress are absent.
2. The `DataTable` builtin does not expose the existing ui-kit pagination and selection APIs. Its current props schema also does not close additional properties, unlike most other builtins.
3. `PageRenderer` always fetches paginated regions at page 1/page size 20, discards `total`, and has no controlled query/filter/page event loop. This prevents a Schema-driven remote table from using the existing `HNBTablePagination` capability.
4. `PageRenderer` uses native private action buttons, private validation/error placeholders, and a private page header. `SchemaPage` uses `window.confirm` and private loading/error/empty markup. These must consume shared primitives rather than become another general component implementation.
5. Cluster list schema's `DataTable` region supplies neither required `columns` nor `dataSource`; cluster detail's `DescriptionList` regions supply no required `items`. The present schemas therefore do not demonstrate executable builtin composition even though local pages import the schema constants.
6. The shell registry contains builtins only. Resource L3 components are registered in the plugin manifest, but there is no demonstrated registry bridge that makes `resource.ClusterSummaryCards` or `resource.ClusterNodesPanel` available to `SchemaPage`.

These are integration findings for subsequent tasks, not recommendations to add page-private renderer controls.

## Existing Page-Private General Implementations

The following implementations are audit evidence of missing shared capability, not candidates to preserve as recommendations:

- `ClusterList.vue`: toolbar, text/select filters, native table, row actions, pagination, loading/error/empty states, dangerous confirmation, and submission-result dialog.
- `ClusterDetail.vue`: action buttons, alert banner, tabs, loading/error/empty states, dangerous confirmation, and submission-result dialog.
- `ClusterRegisterWizard.vue`: dialog shell, tab-like mode selector, form controls/errors, step indicator, action buttons, and submit alert.
- `ClusterNodesPanel.vue`: native table, status pills/local color mapping, loading/error/empty states, and retry button; page state exists but pagination controls do not.
- `PageRenderer.vue`, `RegionWrapper.vue`, and `SchemaPage.vue`: native actions, private error/loading/empty rendering, and `window.confirm`.

Thin domain adapters remain acceptable only when they compose public primitives and contain domain semantics. Examples are `ClusterRegisterWizard` as an L3 workflow, `ClusterNodesPanel` as an L3 data/orchestration component, and a dictionary adapter that passes server-provided labels/semantic tokens to `StatusBadge`. They must not own general dialog, table, pagination, tabs, status-color, state-view, or button implementations.

## Required Shared Work Set

### Reuse without a new general component

- `HNBPageShell`, `HNBToolbar`, `HNBActionBar`, `HNBTableActions`, `DescriptionList`, `HNBDetailPanel`, and `MetricCard`.
- `StatusBadge` as the leaf for server-dictionary statuses.
- `HNBSkeleton`, `EmptyState`, and `ErrorState` after the small behavior/accessibility extensions identified above.
- `HNBFormField`, `HNBSelectInput`, and `HNBDateInput` as form foundations.

### Extend existing shared capability

- `HNBButton`: disabled reason, consistent focus-visible behavior, and async announcement integration.
- `HNBTable`: controlled server pagination/sort/filter exposure, semantic name, responsive scroll, empty/error integration, and Schema builtin API.
- `StatusBadge`: dictionary adapter and reusable multi-status composition without collapsing freshness into lifecycle.
- Shared states and skeletons: localized labels, safe metadata, and consistent shared-button usage.
- Schema Engine: expose shared APIs, preserve block isolation, bind runtime data/total/query state, and remove native/private general controls.

### Add reusable shared capability

- Dialog and Confirmation.
- Alert.
- Tabs.
- Standalone controlled Pagination.
- Text/number/textarea/checkbox form controls.
- NoPermission, Offline, and Incompatible states.
- Notification/live-region support.
- Dictionary status group and Operation progress/step list.

### Keep domain-specific, composed from shared primitives

- `ClusterRegisterWizard` workflow, validation/planning review, SecretReference domain selection, tenant cleanup, and submission orchestration.
- `ClusterNodesPanel` query/filter orchestration and target/tenant race protection.
- STALE challenge token/policy handling and presentation data.
- Operation polling, allowed-action decisions, target/intent links, and Operation detail composition.

## Decision

No page-private general component is recommended. Every general gap identified by this audit is assigned to `@hnb/ui-kit` first, with a Schema Engine builtin/adapter where declarative pages need it. Cluster and Operation L3 components are limited to domain workflow, data orchestration, and composition of those public primitives.

## Audited Sources

- `web/packages/ui-kit/src/index.ts`
- `web/packages/ui-kit/src/types.ts`
- `web/packages/ui-kit/src/tokens.css`
- `web/packages/ui-kit/src/components/*.vue`
- `web/packages/ui-kit/src/components/__tests__/*.test.ts`
- `web/packages/schema-engine/src/builtins.ts`
- `web/packages/schema-engine/src/components/PageRenderer.vue`
- `web/packages/schema-engine/src/components/RegionWrapper.vue`
- `web/packages/schema-engine/src/DataSourceManager.ts`
- `web/packages/schema-engine/src/types.ts`
- `web/shell/src/pages/SchemaPage.vue`
- `web/plugins/resource/src/pages/cluster-management/**`
- `web/plugins/resource/src/index.ts`
- `web/plugins/system/src/pages/OperationApproval.vue`
- `web/plugins/system/src/index.ts`
