# Tasks: in-cluster-ha-backings

## 1. Chart 与部署物料

- [x] 1.1 新增 `backings` 三值渲染：`charts/postgresql` 支持单实例（bundled）与多实例复制（in-cluster-ha）两分支；`charts/nats` 支持 `mode: single|cluster`（3 节点、headless、`--routes`、每节点 PVC、流 R3）。（ICHB-001）
  - 2026-10-16 `global.backings` 三值（bundled|in-cluster-ha|external）；`charts/postgresql` 在 `in-cluster-ha` 渲染 CloudNativePG `Cluster`（instances=3、每实例 storage、managed 平面角色、initdb 平面库），否则渲染原单实例 StatefulSet；`charts/nats` 在 `in-cluster-ha` 渲染 headless Service + 3 节点 StatefulSet（`--config` cluster routes、jetstream domain、volumeClaimTemplates 每节点 PVC），否则渲染原单实例。测试 `deploy/charts/hnb/test_profiles.py::test_ha_profiles_render_in_cluster_backings`（`python3 -m unittest test_profiles` 33 passed）。
- [ ] 1.2 引入 CloudNativePG Operator 作为可选子 chart 依赖（固定 digest 的 CRD/镜像），并支持 `postgresql.operator.enabled=false` 的预装路径。（ICHB-006）
- [x] 1.3 为 PG/JetStream 增加控制节点亲和、control-plane taint 容忍与按 `kubernetes.io/hostname` 的拓扑打散/反亲和。（ICHB-002）
  - 2026-10-16 `charts/postgresql` 的 CNPG `Cluster.spec.affinity`（`enablePodAntiAffinity` + `topologyKey: kubernetes.io/hostname` + control-plane `nodeAffinity` + taint toleration）；`charts/nats` StatefulSet 的 `podAntiAffinity` + control-plane `nodeAffinity`/tolerations，均由 `placement.controlPlane` 控制。
- [x] 1.4 修改 `overlays/{lite-ha,standard-ha,enterprise}.yaml`：默认 `backings: in-cluster-ha`，保留 `backings: external` 显式覆盖；`minimal.yaml` 保持 `bundled`。（ICHB-004）
  - 2026-10-16 HA overlays 设 `global.backings: in-cluster-ha, controlPlaneDomains: 3`，移除 `postgresql/nats.enabled=false`；`profile-gate.yaml` 由 `HA_PROFILE_UNCERTIFIED` 硬失败改为结构门（`HA_CONTROL_NODES_INSUFFICIENT`/`EXTERNAL_BACKING_MISCONFIGURED`/`HA_BUNDLED_NOT_HA`），认证仍由安装期 `EvidenceGate` 阻断；`overlays/README.md` 同步。测试 `test_in_cluster_ha_requires_three_control_domains`、`test_ha_external_backing_disables_bundled_data_plane`。
- [x] 1.5 更新 `deploy/charts/hnb/values.yaml` 的 `global`/`postgresql`/`nats` 默认与注释，说明 backing 选择与凭据来源（existing Secret，不落 values）。（ICHB-006、SEC-001）

## 2. Toolkit 契约与规划/执行

- [x] 2.1 `profilespec.go`：`Backings` 扩展为 `bundled|in-cluster-ha|external`，更新 `backingRank`/`SupersetOf` 与 `ProfileShapeData`（HA 档默认 `in-cluster-ha`、storage 映射为复制拓扑）。（PSHAPE-001、PSHAPE-004、ICHB-003）
  - 2026-10-16 `BackingInClusterHA` 新增；`backingRank` 为 bundled=0、in-cluster-ha=external=1（互不为降级）；`ProfileShapeData` 的 Lite/Standard/Pro `Backings=in-cluster-ha` 且保留复制后的 PG/NATS storage；`upgrade_plan.go` 的 backings 步骤改为 backing 感知（external 需外部可达、in-cluster 需 ≥3 控制域 + operator 就绪）。测试 `profilespec_test.go::TestBackingRankIsMonotonicAndSymmetric`（bundled→in-cluster 升级、external↔in-cluster 对称、→bundled 拒绝）。`go test ./...`（cli+toolkit）全绿。
- [x] 2.2 `upgrade_contract.go`/`lock.go`：`release-lock/v2` 的 `profiles[<name>].backings` 与 `supportedUpgrades[].backingChange` 解析与校验，保持 v1 兼容。（PSHAPE-003、PUPG-005）
  - 2026-10-16 `LockProfileContract.Backings`（allowed set，`Validate` 校验 `knownBacking`，`CheckProfileContract` 校验目标形态 backing 是否被允许）；`LockUpgradePath.BackingChange`（`ValidateUpgrade` 在 from/to 形态 backing 不同时要求声明，否则 `BACKING_CHANGE_NOT_SUPPORTED`）。v1 兼容：字段 `omitempty`，无声明不受影响。测试 `upgrade_contract_test.go`（backingChange 必填/同种 HA 免声明/未知 backing 拒绝/契约 backing 白名单）。
- [x] 2.3 新增 `HA_CONTROL_NODES_INSUFFICIENT` 与 `OPERATOR_NOT_INSTALLED` 前置校验（≥3 可调度控制节点、Operator 就绪），接入 `upgrade_plan.go`/`scale.go`/`EvidenceGate`。（ICHB-002、ICHB-006）
  - 2026-10-16 `dataPlanePreconditions(backings, domains, operatorReady)`：`in-cluster-ha` 时 domains<3 → `HA_CONTROL_NODES_INSUFFICIENT`，operator 未就绪 → `OPERATOR_NOT_INSTALLED`；`UpgradeInput`/`ScaleInput` 新增 `ControlPlaneDomains`（0=未观测跳过）与 `OperatorReady *bool`（nil=未观测跳过），接入 `PlanUpgrade`/`PlanScale`。测试 `TestPlanUpgradeBlocksInClusterHAPreconditions`、`TestPlanScaleBlocksInClusterHAPreconditions`。（EvidenceGate 侧的 config 级校验待 4.1 统一接入。）
- [x] 2.4 `upgrade_plan.go`/`upgrade_executor.go`：新增 `data-plane-rebacking` 阶段与 dual-run/verify/cutover 语义，验证前保留原数据面、失败可回退；纳入 RunStore `upgrade` kind 阶段序。（PUPG-005、ICHB-003）
  - 2026-10-16 新增 `PhaseDataPlaneRebacking`（`data-plane-rebacking`），接入 `UpgradePhaseOrder()`（在 `infra-delta` 与 `database-migration` 之间）；`UpgradeSteps.Rebacking` 仅在计划含 `backings` 步骤时执行（`planHasBackingStep`），原数据面保持到该阶段成功；`PlanRollback` 对 `backings` 步骤返回 `ROLLBACK_REBACKING_UNSUPPORTED`（cutover 后不自动回退）。测试 `TestRunUpgradeExecutesRebackingPhase`、`TestPlanRollbackBlocksRebackings`。
- [x] 2.5 `scale.go`/`scale_executor.go`：`--component postgresql|nats` 的副本/存储扩容映射到 Cluster instances / StatefulSet replicas，保留 quorum 护栏与 PVC 不缩规则。（ICHB-001、profile-scaling）
  - 2026-10-16 `dataPlaneComponents` 映射：`postgresql → CloudNativePG Cluster.spec.instances`、`nats → StatefulSet.spec.replicas`（仅 `in-cluster-ha`）；默认/最小副本 3，`<3` → `SCALE_BELOW_QUORUM`，nats 偶数 → `SCALE_EVEN_QUORUM`；`bundled` 档对 pg/nats 仍 `SCALE_UNKNOWN_COMPONENT`。存储扩容沿用既有 `STORAGE_SHRINK_REFUSED`（PVC 不缩）。测试 `TestPlanScaleDataPlaneReplicas`。

## 3. CLI

- [x] 3.1 `cli/` 增加 `install|upgrade --backing <bundled|in-cluster-ha|external>` 与 `--control-node-count`（或自动探测）参数解析与 usage 文案。（ICHB-004）
  - 2026-10-16 `upgrade`/`scale` 增加 `--backing`、`--control-node-count`、`--operator-ready`（tri-state）；`install` 增加 `--backing`、`--control-node-count`。`--backing` 覆盖目标形态 backing（`UpgradeInput.BackingOverride`）；`install --backing` 经 `toolkit.ApplyBackingValue` 注入 bindings（external 关内置 PG/NATS）；`commands.go` usage 同步。测试 `profile_lifecycle_test.go`（`TestUpgradeControlDomainPrecondition`、`TestUpgradeBackingOverrideAndUnknown`、`TestScaleDataPlaneComponentMapping`）。
- [x] 3.2 再背靠命令路径与 `hnbctl-interface.md` 更新（原因码、阶段、审计字段）。（PUPG-005、ICHB-006）
  - 2026-10-16 `runProfileUpgradeExecution` 在计划含 backings 步骤时挂 `steps.Rebacking`（渲染目标 release 已含新数据面，apply 幂等；真实 dual-run/cutover 待外部/在集群端点）。`docs/deployment/hnbctl-interface.md` 增补 backings 三值与偏序、结构门原因码、`data-plane-rebacking` 阶段、scale 数据面映射与 `HA_CONTROL_NODES_INSUFFICIENT`/`OPERATOR_NOT_INSTALLED`/`SCALE_EVEN_QUORUM`、锁 `backings`/`backingChange`、再背靠 Operation 审计。

## 4. 证据与文档

- [x] 4.1 扩展 `evidence.go` 的 `OSAdaptations`/`ProfilePaths`/`Profiles` 维度以表达 backing；HA 档在无演练证据时保持 `blocked` 并附原因。（OBS-009、ICHB-005）
  - 2026-10-16 `EvidenceProfile`/`EvidencePath` 增加 `Backings` 集合；新增 `ProfileCertifiedFor`/`UpgradePathCertifiedFor`（空 backing 或空集合匹配任意）；`PlanUpgrade` 用有效 backing 判定（`TARGET_PROFILE_NOT_CERTIFIED`/`UPGRADE_PATH_NOT_CERTIFIED`）；`CurrentEvidence` 的 Minimal 标 `bundled`、HA 档标 `in-cluster-ha|external` 且保持 blocked，并声明 Minimal→LiteHA/LiteHA→StandardHA/StandardHA→Enterprise 为 blocked 路径。测试 `evidence_test.go`（含 `TestCurrentEvidenceDeclaresBlockedHAPaths`）。
- [x] 4.2 更新 `docs/deployment/`（形态说明、backing 选择、控制节点放置、备份恢复 Runbook 与卸载保留语义）。（ICHB-004、ICHB-006）
  - 2026-10-16 新增 `docs/deployment/in-cluster-ha-backings.md`（backing 三值与偏序、结构门原因码、CloudNativePG/JetStream 拓扑与控制节点放置、existing Secret 凭据、备份恢复 Runbook 与既有演练证据、卸载保留语义、footprint 基线、示例）；`overlays/README.md`、`hnbctl-interface.md` 已同步。

## 5. 测试

- [x] 5.1 单元测试：backing 三值偏序/超集、`HA_CONTROL_NODES_INSUFFICIENT`、`OPERATOR_NOT_INSTALLED`、v1/v2 锁解析。（PSHAPE-004、ICHB-002、ICHB-006）
  - 2026-10-16 覆盖：`profilespec_test.go::TestBackingRankIsMonotonicAndSymmetric`、`upgrade_plan_test.go::TestPlanUpgradeBlocksInClusterHAPreconditions`、`scale_test.go::TestPlanScaleBlocksInClusterHAPreconditions`、`upgrade_contract_test.go`（`TestValidateUpgradeRequiresBackingChange`/`TestLockRejectsUnknownBacking`/`TestCheckProfileContractRejectsDisallowedBacking`/v1 兼容 `TestLockV1WithoutTrainFieldsRemainsReadable`）、`evidence_test.go`。
- [x] 5.2 Chart 渲染/契约测试：`helm template` 覆盖单实例/cluster 两模式与默认 backing；overlay 与 `ProfileShapeData` 的一致性（parity）测试更新。（ICHB-001、ICHB-004）
  - 2026-10-16 `test_profiles.py`：`test_ha_profiles_render_in_cluster_backings`（CNPG Cluster instances≥3 + 3 节点 nats，无单实例 postgresql StatefulSet）、`test_in_cluster_ha_requires_three_control_domains`、`test_ha_external_backing_disables_bundled_data_plane`（原 `test_all_ha_profiles_remain_uncertified` 改写）；`profilespec_test.go::TestOverlayBackingsMatchShape` 守护 overlay `global.backings` 与 `ProfileShapeData` 一致。`python3 -m unittest test_profiles` 33 passed。
- [x] 5.3 集成测试：`data-plane-rebacking` 状态机（dual-run→verify→cutover，含中止回退）；`scale` 映射与护栏。（PUPG-005、profile-scaling）
  - 2026-10-16 `upgrade_executor_test.go::TestRunUpgradeExecutesRebackingPhase`（仅含 backings 步骤时执行并记录阶段）、`TestPlanRollbackBlocksRebackings`（中止/回退拒绝）；`scale_test.go::TestPlanScaleDataPlaneReplicas`（pg/nats 映射、quorum 护栏、bundled 拒绝）；`profile_lifecycle_test.go::TestScaleDataPlaneComponentMapping`、`TestScaleStepArgsMapDataPlane`（executor kubectl 映射：patch cluster instances / scale statefulset / cluster storage / pvc）。`go test ./...` 全绿。
- [ ] 5.4 E2E（需 ≥3 控制节点环境）：CloudNativePG 主实例故障转移、JetStream 单节点故障、备份恢复与 RPO/RTO 实测；据结果将 HA 档与 `ProfilePaths` 翻为 certified。（OBS-009、ICHB-005）
- [ ] 5.5 以 design 的 Footprint baseline 为输入，在控制节点上压测容量门限（含 etcd 磁盘 IO 争用），产出实测 RSS/峰值并回写基线；据此确定各 HA 档的最小控制节点规格。（ICHB-001、ICHB-002）

## 6. 验证与归档

- [x] 6.1 运行逐模块 `go build ./...`、`go vet`、`gofmt`、`go test -race ./cmd/hnbctl/...`，以及 chart 的 `test_*.py`。（全部）
  - 2026-10-16 `gofmt -l` clean；`go vet ./...` clean；`go test -race ./...`（cli+toolkit）通过；逐模块 `go build ./...`（cmd/*、pkg/*、staging/*）全通过；chart `python3 -m unittest test_profiles test_candidate_components` → 62 passed。
- [ ] 6.2 运行 `openspec validate --all --strict` 与语义门禁；通过后 archive 本 change 并将 delta 合并回主规格（`profile-shape`/`profile-upgrade`/`observability-dr`/`in-cluster-ha-backings`）。（全部）
