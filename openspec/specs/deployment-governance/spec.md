# deployment-governance

## Purpose
定义 HNB Cloud 的能力分级、部署档位、版本化 BOM、阶段门槛、需求双向追踪，以及 OpenSpec change 的完成与归档治理行为。

## Requirements

### Requirement: [GOV-001] 能力分级声明
每个 CapabilityPack 和 OpenSpec change SHALL 声明 T0/T1/T2/T3、影响平面、依赖、资源预算、默认开关和卸载影响；缺失 SHALL 视为不完整。

**Traceability:** METH-01

#### Scenario: 发布未分级能力包
- **GIVEN** 一个 CapabilityPack 未声明 tier
- **WHEN** 申请进入 stable
- **THEN** 门禁拒绝发布
- **AND** 提示补充分级和依赖

### Requirement: [GOV-002] 部署档位约束
平台 SHALL 定义 Minimal、Lite HA、Standard HA、Enterprise 四档 BOM；轻量档位 SHALL NOT 强制依赖重型 T2/T3 组件。

**Traceability:** ART-STO-05, GOV-05

#### Scenario: 安装 Minimal 档位
- **GIVEN** 环境只有单节点资源
- **WHEN** 执行安装
- **THEN** 不安装独立搜索集群、Kafka、Ceph、AI Runtime 或 Edge Pack
- **AND** 仍可完成 T0/T1 最小闭环

### Requirement: [GOV-003] 版本化 BOM 与兼容锁
每个交付版本 SHALL 生成 Core BOM、Infrastructure BOM、Provider BOM 和 Optional Pack BOM，记录镜像 digest、Chart digest、Schema 版本和兼容矩阵。

**Traceability:** CTN-07, ART-09

#### Scenario: 离线重建同一版本
- **GIVEN** 运维持有 Release Bundle
- **WHEN** 在隔离环境安装
- **THEN** 安装得到相同组件摘要和 Schema 版本
- **AND** 差异被安装器报告

### Requirement: [GOV-004] 阶段进入与退出判据
阶段 0、MVP、V1、V1.5、V2 SHALL 具有可量化进入门槛和退出判据；未通过当前阶段强制验收 SHALL NOT 进入下一阶段。

**Traceability:** METH-02

#### Scenario: MVP 请求退出评审
- **GIVEN** e2e 第一交付闭环尚未通过回滚测试
- **WHEN** 提交退出评审
- **THEN** 评审拒绝进入 V1
- **AND** 缺口转化为明确 change

### Requirement: [GOV-005] 需求与实现双向追踪
每个 Requirement SHALL 具有稳定 ID；proposal、design、tasks、测试和验收报告 SHALL 引用 Requirement ID，且可从 Requirement 反查实现提交和测试证据。

**Traceability:** METH-04

#### Scenario: 检查一个已完成 change
- **GIVEN** 任务全部勾选
- **WHEN** 执行 verify
- **THEN** 每个受影响 Requirement 至少有一个自动或演练证据
- **AND** 无孤立任务或无实现需求

### Requirement: [GOV-006] 变更 Definition of Done
change 归档前 SHALL 完成规格同步、代码、迁移、自动测试、Conformance、文档、升级/回滚、遥测、安全评审和已知限制；高风险 change SHALL 完成故障演练。

**Traceability:** GOV-02, GOV-05

#### Scenario: 归档 Provider change
- **GIVEN** 实现代码已合并但回滚未验证
- **WHEN** 执行 archive 前 verify
- **THEN** 归档被阻止或明确警告
- **AND** 补齐证据后方可归档
