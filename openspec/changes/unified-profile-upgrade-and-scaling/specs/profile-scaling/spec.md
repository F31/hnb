## ADDED Requirements

### Requirement: [PSCALE-001] 按需水平扩容
HNB SHALL 提供按需水平扩容：调整受管组件副本数或节点数；existing 路线 SHALL 只扩缩 `hnb-system` 内的受管工作负载，SHALL NOT 触碰外部 CNI 或集群 lifecycle owner。

**Traceability:** DPLY-006, PINST-005; profile-scaling

#### Scenario: 调整组件副本
- **GIVEN** 一个已交付的形态与目标组件
- **WHEN** 操作者执行 `hnbctl scale --component apiserver --replicas 3`
- **THEN** 计划或执行调整该组件副本并等待就绪
- **AND** 记录阶段与原因码

#### Scenario: 外部 owner 被保留
- **GIVEN** 安装在已有集群上的 HNB
- **WHEN** 在 existing 集群上扩容
- **THEN** 仅 `hnb-system` 受管对象变化
- **AND** 外部 CNI/集群 owner 不被修改，违反时拒绝

### Requirement: [PSCALE-002] 垂直与存储扩容
HNB SHALL 支持垂直扩容（requests/limits 变更并滚动重启）与存储扩容；存储扩容 SHALL 仅作用于启用了 `allowVolumeExpansion` 的 StorageClass，HNB SHALL NOT 缩小任何 PVC。

**Traceability:** DPLY-006; profile-scaling

#### Scenario: 允许存储扩容
- **GIVEN** 目标 PVC 所在 StorageClass 允许扩容
- **WHEN** 请求的新尺寸大于当前值
- **THEN** 变更被纳入计划并执行

#### Scenario: 拒绝缩容
- **GIVEN** 一个已存在的 PVC
- **WHEN** 请求的尺寸小于当前值
- **THEN** 系统拒绝并给出原因码 `STORAGE_SHRINK_REFUSED`
- **AND** 不修改 PVC

### Requirement: [PSCALE-003] 扩容护栏
HNB SHALL 在扩容前校验护栏：副本数 SHALL NOT 低于 HA quorum，SHALL NOT 超过单故障域共置上限，且存在未完成的迁移或升级时 SHALL NOT 扩容；违反任一护栏 SHALL 失败关闭。

**Traceability:** GOV-002; profile-upgrade

#### Scenario: 低于 quorum 被拒绝
- **GIVEN** 一个 HA 形态（独立故障域 ≥ 3）
- **WHEN** 请求将 HA 关键组件副本降到 quorum 以下
- **THEN** 系统拒绝并给出原因码 `SCALE_BELOW_QUORUM`

#### Scenario: 进行中的迁移阻止扩容
- **GIVEN** 存在未完成的迁移或升级运行
- **WHEN** 请求扩容
- **THEN** 扩容被拒绝并给出原因码 `SCALE_UPGRADE_IN_FLIGHT`
- **AND** 直到该运行到达终态
