# Design: Phase 0 Project-Truth Baseline

## Context

HNB Cloud V3.8.6 is the approved implementation baseline. The inspected
working tree contains active and archived OpenSpec changes, Go services and
providers, PostgreSQL migrations, deployment assets, and a Vue plugin shell.
Many implementation paths are uncommitted at the captured `main` working tree,
so Phase 0 must describe both the Git anchor and the working-tree state.

This change is evidence-only. It records what is present and connected; it does
not approve observed deviations.

## Goals

- Produce a repeatable source-anchored maturity assessment.
- Verify mandatory architecture and security concerns.
- Separate implemented components from integrated end-to-end capability.
- Give later changes stable blocker identifiers and exit gates.
- Preserve the existing Marketplace foundation.

## Non-Goals

- Implement fixes, add services, alter schemas, or change deployment topology.
- Reconcile every active OpenSpec change with the main specs.
- Declare production readiness based on unit tests or checked task lists.
- Select Phase 1 technology or product scope.

## Architecture Under Assessment

```text
Web Console / CLI / SDK
          |
          v
Authenticated platform boundary
          |
          v
App Market ---- immutable Release/CompositionRelease
          |                       |
          +-----------------------v
                              ExecutionPlan
                                   |
                                   v
                               Operation
                                   |
                    +--------------+--------------+
                    v              v              v
             Provider/Driver  Provider/Driver  Provider/Driver
                    |              |              |
                    v              v              v
          KubernetesTarget  ContainerTarget  EdgeRuntimeTarget

Logical planes (no shared internal database):
  1. App Market
  2. Artifact Storage
  3. Runtime Governance
  4. AI Extension Plane
```

The assessment rejects any observed path around `Operation`; it does not add a
second path to make current code appear complete.

## Decisions

### Decision 1: Evidence Record

Each material finding uses this logical record:

| Field | Meaning |
|---|---|
| ID | Stable fact, seam, security, or blocker identifier |
| Scope | Component or cross-component flow |
| Source | File and line, command, or generated validation output |
| Observation | What the repository proves |
| Maturity | L0-L5 evidence and consecutive rating |
| Confidence | Verified, partial, or not verified |
| Consequence | Risk or planning effect |
| Closure | Future requirement/change owner; never implemented by Phase 0 |

The record is Markdown rather than a new database or schema because Phase 0 has
no runtime behavior and must remain reviewable in a source diff.

### Decision 2: Consecutive Maturity

- L0: a route, subject, menu, manifest, or declared surface exists.
- L1: a handler/consumer/controller accepts and processes input.
- L2: the behavior is functional and, where required, persists authoritative
  state or produces a durable effect.
- L3: authentication, authorization, tenant isolation, and trust-boundary
  propagation are verified.
- L4: relevant automated tests pass, including integration or failure-path
  coverage proportional to risk.
- L5: deployment, observability, SLOs, capacity, upgrades, rollback, disaster
  recovery, supply-chain controls, and runbooks are evidenced.

The "current" level is the highest consecutive completed level. A test suite
may be cited in the L4 column while the current rating remains L2 because L3 is
not met.

### Decision 3: Confidence and Negative Evidence

Absence claims use bounded searches and named scopes. "No consumer found" means
the captured repository search found no consumer for the named subject; it does
not claim external systems are impossible. Runtime behavior requiring a live
database, NATS, Kubernetes, browser, or toolchain is labelled unverified when
the environment cannot execute it.

### Decision 4: Baseline Lifecycle

```text
Draft -> Captured -> Reviewable -> Accepted
   |         |            |
   +---------+------------+----> Superseded
```

- Draft: scope is incomplete.
- Captured: snapshot, searches, and findings exist.
- Reviewable: required artifacts, IDs, and validation outcomes exist.
- Accepted: reviewers approve Phase 0; blockers remain blockers.
- Superseded: a later dated baseline replaces this evidence.

This change reaches Reviewable, not Accepted, through its own authoring.

## API and Event Contracts

No API or event contract changes are introduced. Existing APIs and subjects are
observed as evidence only. In particular, `hnb.market.install` and
`hnb.market.uninstall` are not promoted into approved canonical commands by
this change.

## Database and Persistence

No table or migration is added. The design inspects the migration chain and
records incompatible table redefinitions as blockers. It does not repair them.

## Security Assessment

The evidence baseline covers:

- token issuance versus middleware verification format and secret;
- tenant and actor propagation into handlers and repositories;
- authorization rule/resource/scope matching;
- direct app-market and platform-api ingress;
- tenant predicates on read and write queries;
- query-string tunnel tokens, origin policy, and agent listing;
- provider fencing versus transport authentication;
- Web Console token storage and authenticated API propagation;
- absence of PostgreSQL row-level-security evidence.

Documenting a weakness is not a waiver. Separate changes must define threat
models, compatibility, rollout, rollback, and tests before remediation.

## Cross-Plane Boundary Proof

This change introduces no shared database, cross-plane proxy, binary
dependency, event, or runtime call. The evidence document explicitly treats:

- `app-market` as owner of Product/Release/Channel/Entitlement;
- platform/Operation as owner of runtime execution;
- artifact bytes as an OCI/data-plane concern;
- the AI plane as optional and unable to bypass Operation.

## Failure Modes

| Failure | Treatment |
|---|---|
| Dirty working tree changes during capture | Record Git anchor and dirty-state limitation; require a later recapture for a reproducible release claim |
| Route mistaken for capability | L0/L1 cannot imply L2+ |
| Tests exist but cannot run locally | Cite test files and record execution as unverified |
| Validation tooling missing | Run repository-local checks that do not replace the missing gate, then report the gate as blocked |
| Active change duplicates an applied main spec | Record pre-existing semantic validation failure; do not edit unrelated changes |
| Line references drift | Baseline remains tied to its capture date and is superseded rather than silently rewritten |

## Operational Review

- Tenant isolation: assessed; no runtime change.
- Secrets: assessed; no secret data added.
- Supply chain: N/A for implementation; no dependency or image change.
- Permissions: assessed; no role or policy change.
- Audit: findings are source-controlled; no runtime audit event added.
- Performance/capacity: no runtime impact; L5 evidence gaps are recorded.
- Upgrade/rollback: documentation-only removal; no service action.
- Disaster recovery: no runtime state; repository history is the recovery path.
- Observability: no telemetry; missing telemetry is a readiness blocker.
- Provider/RuntimeTarget/Gateway/Edge compatibility matrix: N/A because no
  provider or runtime contract changes.
- Conformance plan: validate document structure and stable IDs only; product
  conformance remains owned by later implementation changes.

## Alternatives Considered

1. **Generate a numeric score from file counts.** Rejected because routes,
   stubs, tests, and disconnected handlers would be overvalued.
2. **Fix blockers while auditing.** Rejected because it would mix Phase 0 truth
   capture with Phase 1 implementation and obscure review.
3. **Create a replacement Marketplace service.** Rejected because
   `app-market` is the approved existing foundation.
4. **Edit main specs directly.** Rejected because the baseline behavior is new
   governance and must be reviewed as an OpenSpec delta.

## Migration and Rollback

There is no rollout sequence beyond review. Rollback removes this change
directory. A future accepted baseline should supersede this change with a
dated evidence capture rather than mutate historical observations.

