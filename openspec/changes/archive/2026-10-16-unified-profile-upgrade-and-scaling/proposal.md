## Why

HNB 目前提供 Minimal、LiteHA、StandardHA、Enterprise 四种形态，但形态只用零散的 overlay 表达，没有可比较的模型；升级、扩容、降级各自缺失统一规划与执行路径。现场从单机试用演进到 HA/生产时只能重装，无法平滑升档或按需扩容。现在 Minimal 已有端到端交付证据，正是把形态升级与扩容固化为统一、可规划、可恢复、失败关闭能力的时候。

## What Changes

- Change ID：`unified-profile-upgrade-and-scaling`。主能力 T1：统一形态模型、平滑升级与按需扩容；复用既有 Operation/身份/审计与 `hnbctl` 运行存储，不新增事实源。T3 边界（Provider/RuntimeTarget 受管扩缩）沿用现有 Provider 契约。
- 引入形态的**能力向量**：把每个形态表达为 `capabilities`（功能开关）、`components`（组件→replicas）、`ha`（独立故障域/quorum）、`storage`（每组件 PVC/类）、`resources`（requests/limits）的可比较集合，并由既有 `overlays/*.yaml` 单一事实源派生。定义偏序 `minimal ⊂ lite ⊂ standard ⊂ pro`，禁止跨形态回退式升级；单调性由校验器强制。
- 发布锁升级为**发布列车** `release-lock/v2`：一份锁覆盖该版本全部形态 overlay 与能力包、镜像 manifest、`profiles`（每形态 requiredCapabilities/最小 k8s 版本）、`supportedUpgrades`（允许的 from→to）与有序 `migrationChain`（owner + from/to schema + checksum）。兼容校验：目标形态必须是当前能力超集；跨版本必须命中 `supportedUpgrades`；迁移链连续且校验和真实。
- `hnbctl upgrade --to <profile|version> [--dry-run]`（默认 dry-run）：基于当前观测状态与目标能力向量求差，输出 `upgrade-plan/v1`，含能力增启用、副本、存储扩容（仅 `allowVolumeExpansion`、不可缩）、镜像/Chart 增量、owner 顺序迁移；每步标注可逆性与前置（备份）。执行复用 `RunStore` 新增 run kind=`upgrade`，阶段 `preflight/backup → infra-delta → database-migrate → platform-release → verify`，幂等可恢复；回滚=重放上一锁，schema 仅对可逆迁移回滚，否则拒绝。
- 按需扩容 `hnbctl scale`：水平（组件副本 / 节点 `add nodes`）、垂直（requests/limits + 滚动重启）、存储扩容、自动（HPA 与 PG/JetStream 拓扑），带护栏（不低于 HA quorum、不超单域共置、迁移未完成不扩）。existing 路线只扩 `hnb-system` 受管工作负载，不触碰外部 CNI/集群 owner。
- 命令面对齐 KubeKey 风格且不破坏既有：`create config` / `add nodes` / `upgrade --to` / `scale` / `delete`，全部支持 `--dry-run` 与 `status`/`logs`/`history` 追踪。
- 证据矩阵扩展为**按形态 + 按升级路径**认证：升到更高形态需目标形态 E2E 证据；降级需显式确认与可逆性证明。未认证形态/路径保持 blocked，不展示为可升。

## Capabilities

### New Capabilities

- `profile-shape`: 形态能力向量、单调偏序与发布列车（`release-lock/v2`）的校验契约。
- `profile-upgrade`: 跨形态/跨版本的统一升级规划与门禁化、可恢复执行与回滚契约。
- `profile-scaling`: 组件/节点/存储/自动的水平与垂直按需扩容契约与护栏。

### Modified Capabilities

无。既有主规格行为不削弱；本 change 通过新增能力落实，升级与扩容复用既有 Operation/身份/审计与 RunStore 契约，不建立数据库共享、执行旁路或数据面代理。

## Impact

- 影响平面：平台运行治理/内核交付与实施工具；AI/Edge 默认关闭，沿用可选 Pack。
- 受影响规格：新增 `profile-shape`、`profile-upgrade`、`profile-scaling`；引用既有 platform-kernel、runtime-target、deployment-governance、config-secret、security-supply-chain、observability-dr 契约与 `standardize-hnb-deployment-toolkit` 交付面。
- 代码落点：`cmd/hnbctl/internal/toolkit`（形态模型/锁 v2/规划器/执行器/扩容）与 `cmd/hnbctl/internal/cli`（upgrade/scale 动词）；`deploy/charts/hnb/overlays` 保持单一事实源。
- 依赖 change：无活动依赖；依赖当前未提交的 `standardize-hnb-deployment-toolkit` 工作树（RunStore/证据门禁/applier）先进入可追溯版本。
- 新中间件：无；复用 PostgreSQL、NATS JetStream、Helm、既有制品能力。PG/JetStream 数据面升级沿用现有 operator/StatefulSet 契约，不引入新中间件。
- 兼容/迁移：发布锁 v1 保持可读；v2 为增量字段，历史锁不重写。迁移 forward-only，按 owner 顺序；receipt/run 关联不覆盖历史事实。
- 安全与供应链：增量渲染/迁移沿用锁定镜像 digest 与 existingSecret 绑定；计划与日志脱敏；升级需先完成备份前置，未满足即失败关闭。
- 资源预算：形态升级即请求量/副本/PVC 的单调增长，容量报告复用 `capacity` 命令；HA 形态最小 3 独立故障域，PVC 只增不减。
- 可观测：`upgrade-plan/v1`、runId/configDigest/lockDigest、阶段/组件耗时、原因码、每步可逆性与前置，脱敏诊断包复用既有能力。
- 回滚：重放上一 `release-lock`；可逆迁移随锁回滚，不可逆迁移需显式确认否则拒绝升级；卸载仍由服务端权威契约执行，不由升级路径删除数据。
- 非目标：首期不实现跨大版本滚动在线迁移的无停机保证；不自动切换 lifecycle owner；不把未认证形态/路径展示为可升级；不改写 Kubernetes 发行版。
- 退出判据：`hnbctl upgrade --to <更高形态>` 在已有认证基线上产出可审查计划并完成一次受控升级演练（含备份前置、失败关闭、回滚到原锁），`hnbctl scale` 完成水平/垂直/存储扩容且护栏生效；全部留痕。
