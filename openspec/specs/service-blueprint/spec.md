# service-blueprint

## Purpose
定义面向用户的应用、数据库和中间件 ServiceBlueprint 抽象、生命周期能力协商、首批服务范围，以及标准 Binding 输出行为。

## Requirements

### Requirement: [BLUE-001] 服务蓝图抽象
用户 SHALL 通过 ServiceBlueprint 选择应用、数据库或中间件服务；默认体验 SHALL 隐藏底层 CRD、Chart 和 Provider 参数，仅通过 Schema 驱动表单暴露受支持选项。

**Traceability:** MKT-02, METH-03

#### Scenario: 创建 PostgreSQL 服务
- **GIVEN** 用户处于简单模式
- **WHEN** 选择 PostgreSQL ServiceBlueprint
- **THEN** 界面展示容量、可用性和备份等业务参数
- **AND** 不要求用户编写 Kubernetes YAML

### Requirement: [BLUE-002] 生命周期能力协商
ServiceBlueprint SHALL 声明 required/optional 生命周期能力；平台 SHALL 将其与 Provider 和 RuntimeTarget 能力求交集并只展示可执行动作。

**Traceability:** ART-STO-11, AI-RUN-04

#### Scenario: 目标不支持快照
- **GIVEN** 数据库蓝图把快照列为 optional
- **WHEN** 部署到无快照 CSI 的目标
- **THEN** 服务仍可创建但隐藏快照动作
- **AND** required 能力缺失则预检失败

### Requirement: [BLUE-003] 首批数据库服务
首个 T1 产品组合 SHALL 至少提供 PostgreSQL Service Provider，覆盖创建、观察、备份、恢复、升级和删除；高可用、PITR 与故障切换 SHALL 按部署档位声明。

**Traceability:** MKT-02, GOV-05

#### Scenario: 创建带备份的 PostgreSQL
- **GIVEN** 目标具备块存储和备份目标
- **WHEN** 用户创建数据库服务
- **THEN** 服务进入 Ready 并生成连接绑定
- **AND** 备份和恢复 Operation 可验收

### Requirement: [BLUE-004] 首批中间件服务
首个 T1 产品组合 SHALL 在 Valkey 与 RabbitMQ 中至少交付一个标准 Service Provider；Kafka/RocketMQ/MQTT SHALL 作为后续可选 Provider。

**Traceability:** MKT-02

#### Scenario: 部署首批缓存服务
- **GIVEN** 市场已发布 Valkey Release
- **WHEN** 用户通过蓝图创建服务
- **THEN** 平台完成部署、观察、扩缩容和删除
- **AND** 连接信息以 SecretReference 输出

### Requirement: [BLUE-005] 服务绑定与输出
服务实例 SHALL 以标准 Binding 输出 Endpoint、Port、Database/VirtualHost、TLS、SecretReference 和健康状态；应用 SHALL 通过输出绑定消费服务而非读取 Provider 内部对象。

**Traceability:** CMPOS-02, INT-07

#### Scenario: 应用绑定数据库
- **GIVEN** 数据库服务已 Ready
- **WHEN** 组合部署业务应用
- **THEN** ExecutionPlan 将标准 Binding 注入应用
- **AND** 明文密码不出现在计划或审计中
