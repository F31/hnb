# gslb

## Purpose
定义全局流量负载均衡能力，通过 DNS、健康探测、权重和多集群集成实现跨集群故障转移、灰度流量和服务入口治理。
## Requirements
### Requirement: [GSLB-001] 集群健康探测
GSLB Controller SHALL 定期探测成员集群的 API Server 和关键服务健康状态。

**Traceability:** T2

#### Scenario: 健康探测成功
- **GIVEN** 集群 A 运行正常
- **WHEN** GSLB 执行健康探测
- **THEN** 集群 A 标记为 healthy
- **AND** DNS 记录正常解析到集群 A

### Requirement: [GSLB-002] DNS 流量分发
GSLB SHALL 基于健康状态和权重，通过 DNS 将流量分发到多个集群。

**Traceability:** T2

#### Scenario: 故障转移
- **GIVEN** 集群 A 健康探测失败
- **WHEN** GSLB 检测到异常
- **THEN** 集群 A 的 DNS 记录被移除
- **AND** 流量自动转移到集群 B

### Requirement: [GSLB-003] 多集群流量权重
GSLB SHALL 支持按权重分配流量（如 70% 集群 A、30% 集群 B）。

**Traceability:** T2

#### Scenario: 灰度流量
- **GIVEN** 集群 A 权重 70、集群 B 权重 30
- **WHEN** DNS 查询发起
- **THEN** 约 70% 请求解析到集群 A、30% 到集群 B

### Requirement: [GSLB-004] 与 Karmada 集成
GSLB SHALL 读取 Karmada 集群状态作为健康探测数据源之一。

**Traceability:** T2

#### Scenario: Karmada 报告集群不可用
- **GIVEN** Karmada 将成员集群 A 标记为不可用
- **WHEN** GSLB 刷新集群健康状态
- **THEN** GSLB 将该状态纳入集群 A 的流量调度决策

### Requirement: [GSLB-005] 受控流量变更
所有 GSLB 流量变更（故障转移、回切、权重调整、启用禁用）SHALL 经平台 Operation 执行，控制器 SHALL NOT 绕过 Operation Engine 直接修改 DNS 数据面；自动故障转移 SHALL 由平台判定后以 Operation 落地；高风险流量变更 SHALL 默认 require_approval 并支持维护窗口自动批准。

**Traceability:** OP-001, GOV-004

#### Scenario: 用户手动切换
- **GIVEN** 用户选择将 GSLB 服务切换到备用池
- **WHEN** 用户提交切换（需审批策略）
- **THEN** 生成 gslb.failover Operation 并进入待审批状态
- **AND** 审批通过后控制器执行 DNS 变更并验证目标生效

#### Scenario: 自动故障转移
- **GIVEN** 健康聚合判定主池成员全部失健康
- **WHEN** 平台生成自动切换 Operation
- **THEN** 切换经审批（或维护窗口自动批准）后落地
- **AND** 切换过程与结果完整写入审计

#### Scenario: 拒绝旁路
- **GIVEN** gslb-controller 收到 DNS 写入意图
- **WHEN** 该意图不是来自 Operation 命令
- **THEN** 控制器 SHALL NOT 执行任何 DNS 变更
- **AND** 平台记录违规尝试

### Requirement: [GSLB-006] DNS Provider 可插拔
GSLB 的 DNS 数据面 SHALL 通过 gslb-dns-provider 契约接入；平台 SHALL 内置 ExternalDNS DNSEndpoint 参考实现；其他实现 SHALL 经 gslb Conformance Harness 认证后方可接入，内核 SHALL NOT 编译绑定任何具体 DNS 厂商。

**Traceability:** PROV-001, GW-001

#### Scenario: 接入第二 DNS Provider
- **GIVEN** 某 DNS 服务商实现 gslb-dns-provider 契约
- **WHEN** 其通过 Conformance Harness 认证并注册
- **THEN** 平台可使用该 Provider 执行 GSLB 流量变更
- **AND** 平台内核无需发版

#### Scenario: Provider 独立执行入口
- **GIVEN** DNS Provider 具备数据面能力
- **WHEN** Provider 尝试不经 Operation 直接修改 DNS
- **THEN** Provider SHALL NOT 建立独立执行入口
- **AND** 所有动作仍经 Operation Engine 编排

### Requirement: [GSLB-007] Read Model 查询
GSLB 的列表、详情与流量分布查询 SHALL 读取 Read Model 投影；请求路径 SHALL NOT 实时探测集群健康或实时查询 DNS 权威数据；健康状态与当前 DNS 目标 SHALL 由控制器投影并经领域事件同步。

**Traceability:** OP-005, OBS-001

#### Scenario: 查看 GSLB 服务详情
- **GIVEN** 用户打开资源 → GSLB 服务详情
- **WHEN** 页面请求健康与流量分布
- **THEN** 平台从 Read Model 返回投影快照
- **AND** 请求路径不发起任何实时探测或 DNS 查询

### Requirement: [GSLB-008] 多租户隔离
每个 GSLBService SHALL 归属且仅归属一个租户；策略、健康状态、DNS 视图与切换历史 SHALL 按租户隔离；跨租户读取或修改 SHALL 被拒绝；平台级全局入口仅平台管理员可管理。

**Traceability:** TENANT-005, TENANT-006

#### Scenario: 租户隔离
- **GIVEN** 租户 A 与租户 B 各自拥有 GSLBService
- **WHEN** 租户 B 用户尝试读取租户 A 的服务
- **THEN** 查询 SHALL 被拒绝且不泄漏任何 A 的数据
- **AND** DNS 视图与策略互不可见

### Requirement: [GSLB-009] 流量层容灾联动
GSLB 切换 SHALL 可被 DRProtectionGroup 编排为流量层步骤，顺序 SHALL 位于数据层切换之后；回切 SHALL 需要显式人工确认并默认 require_approval；控制面中断 SHALL NOT 影响已下发的 DNS 目标。

**Traceability:** OBS-001, OBS-004

#### Scenario: 地域级故障切换
- **GIVEN** DRProtectionGroup 覆盖主地域的数据层与流量层
- **WHEN** 触发地域级切换
- **THEN** 先完成数据层切换，再执行 gslb.failover 流量层步骤
- **AND** 切换链整体作为可审计的 Operation 序列执行

#### Scenario: 控制面中断
- **GIVEN** 平台控制面中断
- **WHEN** 用户访问已下发的入口域名
- **THEN** 已生效的 DNS 目标继续服务
- **AND** 控制面恢复后按权威状态对账

### Requirement: [GSLB-010] 故障演练
GSLB SHALL 提供只读演练能力：演练 SHALL NOT 产生任何真实 DNS 变更；演练 SHALL 计算并展示假设切换结果，产出演练报告并写入 Read Model 与 Operation 历史。

**Traceability:** OBS-004

#### Scenario: 执行流量切换演练
- **GIVEN** 用户对 GSLB 服务发起演练
- **WHEN** 演练执行
- **THEN** 平台计算假设切换后的目标与影响范围
- **AND** 不 Apply 任何 DNS 变更，报告标记为演练结果

### Requirement: [GSLB-011] 健康数据源契约
GSLB 健康探测数据源 SHALL 通过 HealthSource 契约接入（HTTP/API Server 探测、Karmada 集群状态、人工标记）；平台 SHALL 聚合多源结果并以防抖阈值判定成员健康，单一数据源抖动 SHALL NOT 直接触发流量变更。

**Traceability:** MC-001, GSLB-004

#### Scenario: 多源健康判定
- **GIVEN** 某集群 HTTP 探测连续失败但 Karmada 状态正常
- **WHEN** 健康聚合评估
- **THEN** 未达失败阈值前不标记为失健康
- **AND** 不触发任何流量变更

