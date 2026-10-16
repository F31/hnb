## 1. Profile shape model (profile-shape)

- [x] 1.1 在 `cmd/hnbctl/internal/toolkit` 新增 `profilespec.go`：定义 `ProfileSpec`（capabilities/components/ha/storage/resources）与 kebab-case 稳定标识；单测覆盖字段解析与未知标识拒绝。（profile-shape）
- [x] 1.2 由 `deploy/charts/hnb/overlays/*.yaml` 派生 Minimal/LiteHA/StandardHA/Enterprise 四形态向量；单测断言 Minimal 含 T0/T1 组件且与 overlay 一致。（profile-shape）
- [x] 1.3 实现单调偏序校验 `Superset`/`Monotonic`：能力/组件/故障域/存储不减；违反返回 `PROFILE_NOT_SUPERSET` 类原因码；单测覆盖接受与回退拒绝。（profile-shape）
- [x] 1.4 建立形态向量的 canonical 摘要（sha256），供计划/运行记录引用；单测断言稳定。（profile-shape）

## 2. Release train lock (profile-shape)

- [x] 2.1 在 `lock.go` 增加 `release-lock/v2` 可选字段：`profiles`（requiredCapabilities/最小 k8s）、`supportedUpgrades`、`migrationChain`；保持 v1 读取不回归。（profile-shape）
- [x] 2.2 实现 v2 校验与兼容：路径在 `supportedUpgrades`、迁移链连续且 checksum 真实；单测覆盖 v1 向后兼容、缺链/坏 checksum 拒绝。（profile-shape）
- [x] 2.3 `ValidateCompatibility` 扩展按目标形态校验 requiredCapabilities 与最小 Kubernetes 版本；单测覆盖。（profile-shape）

## 3. Upgrade planner (profile-upgrade)

- [x] 3.1 新增 `upgrade_plan.go`：由当前观测状态与目标 `ProfileSpec` 求差，产出 `upgrade-plan/v1`（能力增启、副本、存储、镜像/Chart、owner 顺序迁移、每步可逆性与前置）；单测覆盖各形态对。（profile-upgrade）
- [x] 3.2 强制执行单调性/降级规则：无显式降级声明时拒绝高→低；不可逆迁移在计划中标为 `irreversible` 并阻止未确认执行；单测覆盖。（profile-upgrade）
- [x] 3.3 计划门禁：目标形态/路径未认证时标注 `blocked` 与原因码；复用 `EvidenceGate`，无副作用；单测覆盖。（profile-upgrade）

## 4. Upgrade executor (profile-upgrade)

- [x] 4.1 在 `RunStore` 增加 run kind=`upgrade`（`Kind` 字段 + `CreateRun`/`CreateHistoryRunKind`）与阶段 `preflight/backup/infra-delta/database-migrate/platform-release/verify`（`UpgradePhaseOrder`）；复用 epoch fencing 与 journal；单测覆盖阶段顺序与终结状态。（profile-upgrade）
- [x] 4.2 实现 `backup` 前置门：未满足即失败关闭，不进入 `database-migrate`；单测覆盖失败关闭。（profile-upgrade）
- [x] 4.3 复用 applier/迁移管线执行 `infra-delta → database-migrate → platform-release → verify`；一次性 phase 永不重跑（resume 重新校验后跳过或拒绝）；单测覆盖 resume 与不可重跑拒绝。（profile-upgrade）
- [x] 4.4 实现回滚=重放上一 lock（`PlanRollback`/`RunUpgradeRollback`，仅可逆迁移与可缩存储），记录回滚阶段；单测覆盖可逆回滚与不可逆/存储拒绝。（profile-upgrade）

## 5. Scaling (profile-scaling)

- [x] 5.1 新增 `hnbctl scale`：`--component --replicas`、PVC 扩缩接口；计划/执行分离（execute 未接线，诚实拒绝）；单测覆盖。（profile-scaling）
- [x] 5.2 护栏：不低于 HA quorum、不超单域共置、迁移/升级进行中禁止扩容、PVC 只增不减（`allowVolumeExpansion`）；单测覆盖各护栏。（profile-scaling）
- [x] 5.3 existing 路线仅扩 `hnb-system` 受管工作负载：`PlanScale` 以 profile 组件集为白名单，外部 CNI/owner 组件（cilium/kube-proxy/coredns/calico/kube-ovn）以 `SCALE_UNKNOWN_COMPONENT` 拒绝；单测覆盖外部 owner 不被触碰。（profile-scaling）

## 6. CLI and evidence (profile-upgrade, profile-scaling)

- [x] 6.1 接入 `upgrade`/`scale` 动词到 `commands.go`，`upgrade`（无 `--to` 时回退平台升级）与 `scale` 默认 dry-run，输出 `profile-upgrade-plan/v1`/`scale-plan/v1`；CLI 测试覆盖。（profile-upgrade, profile-scaling）
- [x] 6.2 扩展 `EvidenceMatrix`：按形态与 from→to 路径认证 `ProfilePaths`；未认证路径保持 blocked；单测覆盖。（profile-shape, profile-upgrade）
- [x] 6.3 更新 `docs/deployment/hnbctl-interface.md`（升级/扩容/发布列车）；openspec 门禁（`openspec validate --strict` 与语义门禁）通过。registry 条目待受控演练后补。（profile-upgrade, profile-scaling）

## 7. Verification

- [x] 7.1 全量 `go test -race ./cmd/hnbctl/...`、`go vet`、`gofmt` 通过。
- [x] 7.2 在已认证 existing 基线上执行一次受控升级演练：`hnbctl upgrade --to Minimal --execute`（run kind=upgrade，status=succeeded，真实 ConfigMap apply），备份前置/不可逆确认/resume 不重跑由单测覆盖；报告 `docs/acceptance/reports/20261016-profile-upgrade-rehearsal.md`。（含 Minimal→Minimal 列车锁；更高形态因未认证不可执行）
- [x] 7.3 `hnbctl scale --execute` 接线完成并在云主机实机演练：apiserver 1→3→1（`--current` 观测基线），两次 run kind=scale 均 succeeded，平台保持 19/19；护栏与 `STORAGE_SHRINK_REFUSED` 亦验证。报告 `docs/acceptance/reports/20261016-profile-upgrade-rehearsal.md`。
- [x] 7.4 `openspec validate unified-profile-upgrade-and-scaling --strict` 通过。
