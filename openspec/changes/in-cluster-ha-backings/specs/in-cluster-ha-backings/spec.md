## ADDED Requirements

### Requirement: [ICHB-001] 控制节点内部的高可用数据面背靠
HNB SHALL 提供 `in-cluster-ha` 背靠模式：PostgreSQL 以多副本复制拓扑运行并具备自动故障转移，JetStream 以至少 3 节点集群运行且每个节点拥有独立持久化；`in-cluster-ha` SHALL NOT 由单副本实例满足。

**Traceability:** DPLY-001, DBND-004; profile-shape

#### Scenario: 渲染 in-cluster-ha 拓扑
- **GIVEN** 一个 HA 档（Lite/Standard/Pro）且 `backings: in-cluster-ha`
- **WHEN** 渲染部署物料
- **THEN** PostgreSQL 渲染为多实例复制拓扑并具备自动故障转移能力
- **AND** JetStream 渲染为 3 节点集群且每节点绑定独立 PVC

#### Scenario: 单实例不满足 HA
- **GIVEN** 一个声明 `in-cluster-ha` 但数据面仅单副本的形态
- **WHEN** 校验该形态
- **THEN** 校验失败并返回稳定原因码
- **AND** 不产生该形态

### Requirement: [ICHB-002] 控制节点放置与故障域前置
`in-cluster-ha` 的 PostgreSQL 与 JetStream 工作负载 SHALL 固定调度到控制节点并容忍控制面 taint，SHALL 以反亲和或拓扑打散约束使各副本落在不同节点；当可调度控制节点少于 3 时，系统 SHALL 拒绝该模式并返回 `HA_CONTROL_NODES_INSUFFICIENT`。

**Traceability:** DPLY-001, PINST-007; profile-shape

#### Scenario: 控制节点不足
- **GIVEN** 一个 `in-cluster-ha` 部署且可调度控制节点少于 3
- **WHEN** 规划或校验该部署
- **THEN** 拒绝并返回 `HA_CONTROL_NODES_INSUFFICIENT`
- **AND** 不落地任何部分数据面拓扑

#### Scenario: 副本分散到不同控制节点
- **GIVEN** 一个 `in-cluster-ha` 部署且有 3 个可调度控制节点
- **WHEN** 调度数据面副本
- **THEN** 各副本落在不同主机名节点
- **AND** 工作负载携带容忍控制面 taint 的容忍度

### Requirement: [ICHB-003] 背靠模式偏序与迁移合法性
HNB SHALL 定义背靠偏序 `bundled < in-cluster-ha ≈ external`；`in-cluster-ha` 与 `external` 之间互不为降级，任一向 `bundled` 的迁移 SHALL 被拒绝；背靠迁移 SHALL 为显式、可审计的受控操作，仅当发布锁声明允许时才执行。

**Traceability:** GOV-002, DPLY-003; profile-upgrade

#### Scenario: external 与 in-cluster 互迁被接受
- **GIVEN** 发布锁声明 `backingChange: true`
- **WHEN** 请求 external 与 in-cluster-ha 之间的再背靠
- **THEN** 迁移被判定为允许
- **AND** 建立平台 Operation 行记录 from/to

#### Scenario: 回退到单实例被拒绝
- **GIVEN** 当前背靠为 `in-cluster-ha` 或 `external`
- **WHEN** 请求迁移到 `bundled`
- **THEN** 拒绝并返回 `PROFILE_NOT_SUPERSET` 类原因码

### Requirement: [ICHB-004] HA 档默认背靠为 in-cluster
Lite/Standard/Pro 形态 overlay SHALL 默认启用内置高可用数据面（`backings: in-cluster-ha`）；`external` SHALL 作为显式可选；Minimal SHALL 保持单实例 `bundled` 不变。

**Traceability:** DPLY-001, DPLY-008; profile-shape

#### Scenario: HA 档默认渲染内置数据面
- **GIVEN** 未显式指定背靠的 Lite/Standard/Pro 形态
- **WHEN** 渲染部署物料
- **THEN** 内置 PostgreSQL 与 JetStream 被启用且为高可用拓扑
- **AND** 不要求外部托管数据面

#### Scenario: 显式选择 external
- **GIVEN** 运维显式选择 `--backing external`
- **WHEN** 渲染部署物料
- **THEN** 内置 PostgreSQL 与 JetStream 被禁用
- **AND** 依赖外部 DSN 与账号 Secret

### Requirement: [ICHB-005] in-cluster 背靠的备份恢复与证据
当背靠为 `in-cluster-ha` 时，平台 SHALL 负责 PostgreSQL 与 JetStream 的版本化备份与可执行恢复，并 SHALL 出具 PoC/RTO 实测；仅有备份文件 SHALL NOT 视为恢复能力完成，且未经故障演练 SHALL NOT 使该档位 certified。

**Traceability:** OBS-003, OBS-004, DBND-004; observability-dr

#### Scenario: 执行 in-cluster 数据面恢复演练
- **GIVEN** 一个 `in-cluster-ha` 环境具备最近备份
- **WHEN** 运维触发恢复 Runbook
- **THEN** 平台与各平面数据恢复一致
- **AND** 恢复结果包含 RPO/RTO 实测

#### Scenario: 未演练不得认证
- **GIVEN** 一个 `in-cluster-ha` 档位尚无故障演练与 RPO/RTO 证据
- **WHEN** 评估该档位能力证据
- **THEN** 该档位保持未认证
- **AND** 安装或升级在能力门禁处被拒绝

### Requirement: [ICHB-006] 数据面 Operator 生命周期与卸载
`in-cluster-ha` 依赖的数据面 Operator SHALL 以固定 digest 的可选依赖或预装两种方式之一提供；未安装时系统 SHALL 拒绝该模式并返回 `OPERATOR_NOT_INSTALLED`；卸载 SHALL NOT 静默删除持久化数据。

**Traceability:** DPLY-008, SEC-001; deployment-governance

#### Scenario: Operator 缺失时拒绝
- **GIVEN** 一个 `in-cluster-ha` 部署且所需 Operator 未安装
- **WHEN** 校验或规划该部署
- **THEN** 拒绝并返回 `OPERATOR_NOT_INSTALLED`
- **AND** 不静默降级为单实例

#### Scenario: 卸载保留数据
- **GIVEN** 一个 `in-cluster-ha` 环境
- **WHEN** 执行卸载且未显式请求删除数据
- **THEN** 持久化数据与备份保留
- **AND** 明确列出未被删除的资源
