## Context

HNB 以微内核 + Provider/CapabilityPack 组织，形态差异目前散落在 `deploy/charts/hnb/overlays/*.yaml`：`minimal.yaml` 固定 19 个工作负载与 bundled 单实例 PG/JetStream，`lite-ha.yaml`/`standard-ha.yaml`/`enterprise.yaml` 只在中途追加副本与外部 backing。`Capacity`、`EvidenceMatrix.Profiles`、`ReleaseLock.Compatibility` 各持一份形态概念，`hnbctl` 也已有半成品 `upgrade` 动词与 `RunStore`（单写者 + epoch fencing + 原子 state + journal）、owner 顺序迁移、证据门禁。约束：CLI 不得共享各平面数据库、不得代理大文件、托管模式不得绕过 Operation；existing 路线不得触碰外部 CNI/集群 owner。

## Goals / Non-Goals

**Goals:**
- 用统一能力向量表达四形态，并可机器比较、可校验单调偏序。
- 统一升级规划与执行：任意低形态→高形态或直达某形态，以及同形态跨版本，输出可审查计划并门禁化执行。
- 形态升级 = 能力向量求差的 DAG 执行；扩容 = 向量在副本/资源/拓扑维度的增量。
- 复用证据矩阵（按形态/按路径）、RunStore 阶段、迁移与 applier；失败关闭、可恢复、可回滚。

**Non-Goals:**
- 不保证跨大版本无停机在线迁移；不自动切换 lifecycle owner。
- 不引入新中间件或新事实源；不把未认证形态/路径展示为可升级。
- 不删除数据（卸载仍走服务端权威契约）。

## Decisions

- **形态即能力向量（`ProfileSpec`）**。以 `capabilities`/`components`/`ha`/`storage`/`resources` 五元组表达，由 overlay 单一事实源派生而非重复维护。备选：继续用裸 overlay 字符串 → 无法比较/校验，否决；引入独立 profile registry 文件 → 与 Chart 双源易漂移，否决。
- **偏序而非图**。定义 `Minimal ⊂ Lite ⊂ Standard ⊂ Pro`，任何升级目标必须是当前能力集超集。备选：允许任意横向切换 → 会出现"回退式升级"，破坏单调性与证据口径，否决。
- **发布列车 `release-lock/v2`**。一份锁覆盖该版本所有形态/能力包 + `supportedUpgrades` + `migrationChain`，使"能升级到哪、需要哪些迁移"在锁内可验证。v1 保持可读，新增字段可选。备选：每形态一锁 → 升级需跨锁对账，无法离线整体校验，否决。
- **规划-执行分离**。`upgrade` 默认 `--dry-run` 产出 `upgrade-plan/v1`（含每步可逆性与前置），`--execute` 才进入门禁化执行。复用 `RunStore` 新增 run kind=`upgrade` 与阶段 `preflight→backup→infra-delta→database-migrate→platform-release→verify`；一次性 phase 永不重跑。
- **回滚=重放上一锁**。可逆迁移随锁回滚；不可逆迁移在规划期即标为 `irreversible`，无显式确认则拒绝升级。备份是升级前置，未满足即失败关闭（不新增 Helm rollback 作为 DB 回退）。
- **扩容护栏内建**。`scale` 校验不低于 HA quorum、不超单域共置上限、迁移未完成不得扩、PVC 只增不减（需 `allowVolumeExpansion`）。existing 路线只扩 `hnb-system` 受管工作负载。
- **证据按形态+路径**。`EvidenceMatrix` 扩展 `ProfilePaths`（from→to），升到高形态需目标形态 E2E；降级需显式 ack 与可逆性证明。

## Risks / Trade-offs

- [跨版本在线迁移复杂] → 首期限定受控升级（可含短时维护窗口），规划器显式标注停机步骤；无停机保证列为 Non-Goal。
- [PG/JetStream 数据面升级风险] → 沿用 operator/StatefulSet 契约与 owner 顺序迁移；备份前置强制，失败即回滚到原锁。
- [PVC 不可缩] → 规划器拒绝缩容请求；形态升级只做扩容。
- [锁 v2 与 v1 并存] → v2 为增量可选字段，读取器向后兼容；校验器要求 v2 才启用升级路径。
- [证据口径被放宽] → 未认证形态/路径保持 blocked；`upgrade` 门禁复用 `EvidenceGate`，不因命令存在而放行。
- [existing 触碰外部 owner] → 扩缩仅限 `hnb-system` 受管对象，复用既有 ownership 边界校验。

## Migration Plan

1. 新增 `ProfileSpec` 与能力向量、单调校验（纯代码，无持久化）。
2. `release-lock/v2` 增量字段与校验/兼容；v1 读取不回归。
3. 升级规划器（diff → `upgrade-plan/v1`），只读、无副作用。
4. 升级执行器（RunStore 阶段化，门禁化）；先以合成认证矩阵单测，再在已认证 existing 基线上做受控演练。
5. `scale` 动词与护栏。
6. 证据矩阵按形态/路径扩展 + 验收报告。回滚策略：任一步未达到退出判据即回到上一锁；代码层可整体回退（无不可逆持久化 schema 变更）。

## Open Questions

- HPA 是否纳入首期，或以显式 `--replicas` 为主、HPA 为后续？默认后者。
- 迁移链的 schema 版本标识采用 owner 级整数还是内容摘要？默认 owner 级有序 + checksum。
