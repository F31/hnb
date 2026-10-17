## MODIFIED Requirements

### Requirement: [GOV-002] 部署档位约束
平台 SHALL 定义 Minimal、Lite HA、Standard HA、Enterprise 四档 BOM，并兼容产品名称 Minimal、Lite、Standard、Pro。四档 SHALL 基于统一技术底座与组件目录，分别表达演示、最小生产、常用生产和高级特性/企业生产的推荐默认部署、规模及保障目标，不得据此限制其他档位按条件安装任何已发布组件或服务能力。高级能力可作为 Pro 的推荐组合，但 SHALL NOT 成为 Pro 专属技术实现；Pro SHALL NOT 强制安装全部可选能力。

轻量档位 SHALL NOT 强制依赖重型 T2/T3 组件；用户显式选择时 SHALL 计算其真实依赖、资源、版本兼容与认证范围，不得绕过预检。能力安装 SHALL 不自动改变档位或生产认证，故障保障 SHALL 以实际拓扑和证据为准。T0/T1/T2/T3 SHALL 表示交付/默认性/成熟度分级而非四档功能授权。

**Traceability:** ART-STO-05, GOV-05, GOV-001, PROV-003, PROV-005, PREADY-007

#### Scenario: 安装 Minimal 档位
- **GIVEN** 环境只有单节点资源
- **WHEN** 执行默认 Minimal 安装
- **THEN** 不安装独立搜索集群、Kafka、Ceph、AI Runtime 或 Edge Pack
- **AND** 仍可完成 T0/T1 最小闭环，明确非 HA 范围

#### Scenario: Lite 显式选择高级能力
- **GIVEN** Lite 用户选择一个已发布的可选能力且实际环境满足权限、资源、依赖、兼容性与认证条件
- **WHEN** 生成部署计划
- **THEN** 平台允许按需加载，不因该能力在 Pro 默认推荐而拒绝
- **AND** Lite 档位和其实际保障范围不会被自动改写
