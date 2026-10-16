# Implementation evidence

Requirement references: DBND-002, DBND-005.

Evidence in this directory records completed increments of the deployment toolkit.
The candidate source archive is an implementation snapshot, not a certified HNB
platform release. Historical Rocky reports retain their original source hashes;
Ubuntu's later source snapshot supersedes them for the sudo/APT/readiness changes.
Provider-level reports do not certify platform installation, HA or offline mode.

## CLI and preflight increments

迁移引擎最新进展：[3.7 重编号与显式基线](task-3.7-renumbering-and-baseline.md)；
服务启动门增量见 [3.7 readiness gate](task-3.7-readiness-gate.md)，仅在配置迁移目录时强制；迁移 CLI 依用户决策另行处理。

| Task | Evidence | Status |
|---|---|---|
| 2.1 | [CLI skeleton](task-2.1-cli-skeleton.md) | done |
| 2.2 | [Strict validation and normalization](task-2.2-validation.md) | done; full platform capabilities remain blocked |
| 2.3 | [Interactive/noninteractive init](task-2.3-init.md) | done |
| 2.4 | [Read-only node probes](task-2.4-node-probes.md) | done; Ubuntu real SSH observation and unchanged config/package snapshot |
| 2.5 | [Network/TLS/content probes](task-2.5-network-content-progress.md) | done increment; overall preflight remains blocked for unobserved source paths |
| 2.6 | [Preflight report and redaction](task-2.6-preflight-output.md) | done; human/JSON projection, contract validation and sensitive-field fixtures |
| 3.1 | [Candidate release lock generation](task-3.1-candidate-lock.md) | done; real registry/CLI/Chart/migration identities, declared redirect policy |
| 3.2 | [Node dependency recipe](task-3.2-node-recipe.md) | done; recipe resolution and closure gaps; closure itself still open (18 gaps) |
| — | [Harbor convergence and python3 removal](harbor-convergence-and-python3-removal.md) | code+tests+Ubuntu real acceptance done; zero public-upstream images |
| 3.3 | [Chart digest pinning and Portal](task-3.3-chart-digest-and-portal.md) | done; 23/23 images digest-pinned, zero tags |
| 3.4 | [Provider service identity](task-3.4-provider-auth.md) | done; lifecycle provider refuses to start without verifiable service identity |
| 3.5 | [Per-plane identity (real containers)](task-3.5-real-containers.md) | done; real PG cross-owner denial + NATS account isolation verified in containers |
| 3.6 | [Identity materials](task-3.6-identity-materials.md) | done; identity/KMS/admin generation increment |
| 3.7 | [Schema readiness gate](task-3.7-readiness-gate.md) | done increment; migration-dir opt-in startup gate |
| 3.8 | [Profile capacity](task-3.8-capacity.md) | done; rendered requests/overlays and planning CLI; HA uncertified |
| 3.8 calibration | [Real-host load calibration](task-3.8-capacity-under-load-calibration.md) | done (observation); Startup under login+read load on 2C4G: throughput plateaus ~118-150 ops/s, PostgreSQL ~1.15 cores peak, ~2738MiB memory headroom, probes held; planning values still uncertified |
| 4.1 | [Shared node stages](task-4.1-shared-stages.md) | done; Provider adapter and real SSH concurrent-stream race check |
| 4.2 | [Prepare progress](task-4.2-prepare-progress.md), [source adapters / real registry](task-4.2-source-adapters.md) | partial; streaming/retry/cache, anonymous Bearer/CDN, OCI Chart and authorized bootstrap preload (adapter-gated, OCI layout stream + Day-0 fence) implemented; real-registry blob/ctr import E2E pending 5.4/5.5 |
| 4.3 | [Existing-cluster plan + ownership](task-4.3-existing-plan-ownership.md) | done (planner + ownership manifest); cluster apply/Minimal E2E still in 5.3 |
| 4.4 | [Boundary contract tests + internal bootstrap adapter](task-4.4-blocked-boundary-contract.md) | done (mechanism); 1.2 rejection scenarios as Go contract tests + internal candidate `ManagementBootstrap` (identity/claim/epoch fencing, refuse+resume matrix); CLI/execution route stays closed behind 4.10 + 5.4/5.5 |
| 6.2 | [node add / upgrade server action contract](task-6.2-node-add-contract.md) | done (contract+matrix+routing); `AddNodesRuntimeTarget` end-to-end (schemas, engine validation, matrix `node-add: UNSUPPORTED` pending 6.3 conformance, plan step factory, IAM, BFF); upgrade contract verified pre-existing |
| 4.5 | [Run store + fencing/recovery](task-4.5-run-store-fencing.md) | done (mechanism); private receipt, phase pre/post checks, single-writer epoch fencing, resume never re-runs init/reset; CLI table in 4.6 |
| 4.11 | [Run-store integrity hardening](task-4.11-run-store-integrity.md) | done (mechanism); corrupt state never overwritten, full run-identity change refused, 0600/0700+symlink enforcement, locked read-modify-write, expiry bumps fencing epoch, terminal-state refusal, interrupted one-shots recheck-only, timeouts cover pre/post, layer verdicts not masked by later success |
| 4.6 | [Status/resume + layered delivery](task-4.6-status-resume-layered.md) | done; executable status/resume, per-phase independent timeouts, 4-layer delivery verdict (cluster pass + platform fail ≠ success) |
| 4.7 | [Typed intent client](task-4.7-intent-client.md) | done; server-aligned Intent submission, idempotent replay, cross-tenant secret refusal, 503 -> exit 5 with no local fallback; fixed an `operation get` panic |
| 4.8 | [Management handoff](task-4.8-handoff.md) | done; formal registration Operation + live Agent observation both required, receipt attached only, installer times cannot stand in for observation |
| 5.1 | [Layered verify checks](task-5.1-verify-layers.md) | done; 13 required checks across 4 layers, unevaluated-required-check gate blocks inflation, owned test resources must be cleaned |
| 5.2 | [Redacted diagnostics](task-5.2-diagnostics.md) | done; deterministic private archive with bundle identity/content digest; 10 secret+customer-data fixtures verified absent |
| 5.3-5.5 | [E2E environment blocked](task-5.x-e2e-environment-blocked.md) | blocked; target host accepts TCP/22 but never sends an SSH banner, no platform E2E executed |
| 5.6 | [Restore mechanism](task-5.6-restore-mechanism-blocked-drill.md) | partial; restore-kind separation and evidence gates done; dependency-only real reboot/restore increment below, full platform drill still blocked |
| 5.6 increment | [Ubuntu dependency reboot/restore drill](../../../../docs/acceptance/reports/20261012-ubuntu-dependency-drill.md) | dependency-only real-host experiment passed: three PG databases, JetStream, synthetic identity/KMS material survive reboot and restore; full platform and approved RTO/RPO remain open |
| 6.5 | [Recovery input verification + operation card](task-6.5-recovery-input-verification.md) | done; identity-bound backup/recovery input checks, read-only, secret-safe, restore execution blocked until target identity observed; real restore deferred to server contract |
| 6.6 | [Ownership-based removal](task-6.6-ownership-removal.md) | done; three separate removal entry points, ownership-violation invariant, storage-deletion acknowledgement, execution deferred to server contract |
| 6.4 | [Platform version diff/change](task-6.4-platform-change.md) | done; digest-pinned upgrade/rollback plans, forced DB-compat and verified-backup gates, API-down refuses generic execution |
| 7.1 | [Offline material lifecycle](task-7.1-offline-bundle.md) | done; independently derived lock/OCI/OS-package closure, real private local import and no-network tests; offline installation uncertified |
| 7.2/7.3 | [Host capacity check](task-7.2-7.3-host-capacity-check.md) | blocked; new host 106.75.10.155 reachable (proxy bypass documented) but 2c/3.8Gi/19G fails Minimal single-node budget, disk below capacity CLI floor |
| 3.3/3.4 | [Earlier candidate Chart progress](task-3.3-3.4-chart-progress.md) | superseded by the 3.3/3.4 evidence above |

As of this session, `106.75.8.151` was deleted by the user. Its historical
acceptance report is retained but it is not an available test environment.
`106.75.26.57` was restarted by the user and used for the new read-only test.
The reboot/probe result does not certify cluster/platform readiness or persistence.
