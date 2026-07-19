# platform-kernel

## Purpose
定义 HNB Cloud T0 微内核允许承担的最小职责、唯一执行边界、查询与控制分离方式，以及控制面故障时的数据面隔离要求。

## Requirements

### Requirement: [KERNEL-001] 最小内核边界
HNB Core SHALL 仅包含身份与租户上下文、Operation Engine、ExecutionPlan Engine、Read Model、Resource Graph、Provider/Capability Registry、Policy Hook、Audit 与 Agent Gateway；具体 CNI、CSI、数据库、中间件、Gateway、AI Runtime 和边缘实现 SHALL NOT 编译进入内核。

**Traceability:** CTN-01, AI-ARCH-01, ART-STO-24

#### Scenario: 卸载可选能力后内核独立运行
- **GIVEN** 一个已安装 HNB Cloud 的环境
- **WHEN** 全部 T2/T3 能力包被停用或卸载
- **THEN** T0 内核组件仍可启动并通过健康检查
- **AND** 内核镜像依赖清单中不存在具体 Provider 实现镜像

### Requirement: [KERNEL-002] Operation 唯一写入口
所有部署、升级、回滚、扩缩容、备份、恢复、切换、删除、GC、OTA 和高风险配置变更 SHALL 通过持久化 Operation 执行；任何门户、Copilot、Provider 或 Controller SHALL NOT 绕过该状态机直接改变 RuntimeTarget。

**Traceability:** CMPOS-03, CMPOS-04, AI-OPS-02, GW-07, EDGE-18

#### Scenario: 外部组件请求资源变更
- **GIVEN** Gateway Provider 或 Copilot 生成资源变更计划
- **WHEN** 用户或策略批准执行
- **THEN** 平台创建可审计 Operation 并由 Runtime Driver 执行
- **AND** 直接调用集群写 API 的旁路请求被拒绝

### Requirement: [KERNEL-003] 查询与控制解耦
列表、搜索和聚合查询 SHALL 读取 Read Model；控制器 SHALL 通过事件或投影器更新 Read Model，查询接口 SHALL NOT 在请求路径实时遍历全部运行目标。

**Traceability:** METH-04, GW-13, EDGE-14

#### Scenario: 大规模目标下查询应用列表
- **GIVEN** 平台已纳管 100 个以上 RuntimeTarget
- **WHEN** 用户查询应用、Route 或边缘节点列表
- **THEN** 请求从 Read Model 返回
- **AND** 响应时延不随 RuntimeTarget 数量线性增长
- **AND** 结果包含 lastObservedAt 或 lastKnownStateAt

### Requirement: [KERNEL-004] 控制面故障不影响数据面
市场、平台控制面或 AI Extension Plane 不可用时，已运行应用、数据库、中间件、Gateway 数据面和已下发边缘负载 SHALL 继续运行。

**Traceability:** INT-05, AI-ARCH-02, EDGE-04

#### Scenario: 中心控制面中断
- **GIVEN** 一个生产应用已经成功部署并对外提供服务
- **WHEN** 市场和平台 API 同时停止
- **THEN** 应用数据面与 Gateway 数据面继续提供服务
- **AND** 恢复后控制面能够重新对账实际状态
