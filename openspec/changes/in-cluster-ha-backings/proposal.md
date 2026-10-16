# in-cluster-ha-backings

- **Change ID**: `in-cluster-ha-backings`
- **Tier**: T1（HA 档默认交付路径的一部分；Minimal 保持既有 bundled 单实例）
- **影响平面**: 平台内核平面（部署/治理）与运行治理平面（数据面背靠）；不改变四平面业务契约
- **受影响 specs**: `profile-shape`（改）、`profile-upgrade`（改）、`observability-dr`（改）、`in-cluster-ha-backings`（新增）
- **依赖 change**: `standardize-hnb-deployment-toolkit`（部署工具链与形态向量）、`unified-profile-upgrade-and-scaling`（已归档）、`adopt-nats-jetstream-messaging`（已归档）
- **迁移影响**: 新增 backing 模式 `in-cluster-ha`；HA 档（Lite/Standard/Pro）默认切为 in-cluster，`external` 降为显式可选。已有 external 安装不受影响；已认证证据仅覆盖 Minimal/existing，HA 档仍为 blocked 直到新环境演练完成。
- **回滚策略**: 通过 `release-lock` 的 `supportedUpgrades`/backing 声明与 RunStore 的 `upgrade`/`scale` kind 恢复；backing 迁移为显式、可审计的受控操作，不做静默回退。

## Why

当前 `deploy/charts/hnb/overlays/lite-ha.yaml` 直接关闭内置 PostgreSQL/JetStream（`enabled=false`），`ProfileSpec.Backings` 仅 `bundled|external`，HA 档在设计上被强制要求外部托管数据面。这让用户在没有现成外置 PG/JetStream 的环境里无法启用 HA 档位，也把“控制节点内自带高可用数据面”这一常见部署诉求挡在门外。需要让 PostgreSQL 与 JetStream 在单节点 Minimal 及 Lite/Standard/Pro（HA）下都能部署在控制节点内部并具备真实 HA 能力。

## What Changes

- 新增 backing 模式 `in-cluster-ha`：PostgreSQL 由 **CloudNativePG Operator** 托管（主备 + 自动故障转移 + 备份/恢复），JetStream 由**改造后的 nats 子 chart** 以 3 节点集群（每节点 PVC、流 R3）提供。
- HA 档（Lite/Standard/Pro）**默认 in-cluster**；`external` 改为显式可选。Minimal 维持单实例 bundled 不变。
- PG/JetStream 工作负载**固定调度到控制节点**并容忍 `control-plane` taint；要求 ≥3 个独立控制节点以满足 quorum 与故障域。
- 扩展 `ProfileSpec.Backings` 枚举与偏序规则（`bundled` < `in-cluster-ha` ≈ `external`），并让形态向量覆盖 backing 迁移的合法性判定。
- `hnbctl`/release-lock 增加 backing 选择与声明；升级/扩容计划与执行器纳入 backing 迁移与 in-cluster HA 组件的存储/副本。
- **BREAKING（语义）**：HA 档默认 backing 由 external 变为 in-cluster；显式依赖 external 默认值的既有自动化需声明 `--backing external`（当前尚无该默认已认证安装，影响面限于未认证路径）。

## Capabilities

### New Capabilities
- `in-cluster-ha-backings`: 控制节点内部署的高可用 PostgreSQL（CloudNativePG）与 JetStream（3 节点集群）作为一等 backing；覆盖 placement/quorum/故障域、每节点持久化、备份/恢复、Operator 生命周期与卸载语义。

### Modified Capabilities
- `profile-shape`: `backings` 从二值扩展为 `bundled|in-cluster-ha|external`，并定义三者偏序与 `SupersetOf` 判定；HA 档默认值改为 `in-cluster-ha`。
- `profile-upgrade`: 支持 backing 迁移（external↔in-cluster-ha）作为受控、可审计的升级/再背靠操作，纳入 release-lock 的 `supportedUpgrades` 与迁移链。
- `observability-dr`: 当背靠为 in-cluster 时，平台自身承担数据库主实例与 JetStream 节点故障演练（OBS-004）以及这两者的备份恢复与 RPO/RTO 实测（OBS-003）。

## Impact

- **部署物料**: `deploy/charts/hnb/overlays/{lite-ha,standard-ha,enterprise}.yaml` 由关闭内置改为默认启用；新增 CloudNativePG 依赖（`CloudNativePG Cluster` 子 chart 或 CRD 引用）；`charts/postgresql` 与 `charts/nats` 增加 HA 分支；控制节点亲和/容忍与 ≥3 控制节点前置校验。
- **CLI/契约**: `cmd/hnbctl/internal/toolkit/{profilespec,upgrade_contract,upgrade_plan,scale,upgrade_executor}.go` 与 `cli/profile_lifecycle.go`、`evidence.go`；`release-lock/v2` 的 `profiles`/`supportedUpgrades` 增加 backing 维度。
- **依赖**: 新增 CloudNativePG Operator（含其 CRD、webhook 与镜像）；nats 子 chart 仍用官方 `nats-server` 镜像（`2.11.1-scratch`），无新依赖。
- **用户价值**: 无外置数据面也能启用 HA 档位；数据面与平台同域运维、失败域内收敛。
- **非目标**: 不改变 Minimal 单实例默认；不引入第三方 PG chart（Bitnami/Patroni 自建）作为主路径；不认证未演练的 HA 组合；不改动业务四平面契约。
- **兼容性**: `bundled`/`external` 语义保留；新增 `in-cluster-ha`；`release-lock/v1` 读取兼容不变。
- **安全风险**: CloudNativePG Operator 权限（CRD/webhook/Secret 访问）需最小化并纳入 supply-chain 固定 digest；PG/JetStream 凭据沿用 existing Secret，绝不落入 values 或 release metadata；控制节点承载数据面需显式容忍 taint 并审计其调度约束。
- **资源预算**: HA 档每控制节点新增约 1–2 vCPU / 2–4 GiB 与一档持久化（PG 每副本 + JetStream 每节点），具体门限由控制节点容量压测确定，不在本 change 内设定绝对数值。
- **可观测要求**: PG（CloudNativePG 指标）与 JetStream（`/jsz`、流/消费者指标）纳入统一遥测；backing 迁移与故障切换产生 Operation 行与领域事件。
- **退出判据**: 形态向量与偏序测试通过；HA 档默认 in-cluster 渲染成功且控制节点调度约束生效；CloudNativePG 与 3 节点 JetStream 在 ≥3 控制节点环境完成故障演练与备份恢复实测；相应 `EvidenceProfiles`/`ProfilePaths` 翻为 certified，`openspec validate --all --strict` 与语义门禁通过。
