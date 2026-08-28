## ADDED Requirements

### Requirement: [RT-001] 运行目标分类
平台 SHALL 以 KubernetesTarget、ContainerEngineTarget 和 EdgeRuntimeTarget 作为三类容器执行目标；ExternalServiceConnector SHALL 仅表示外部服务绑定，不得执行容器部署。

**Traceability:** EDGE-01, GOV-05

#### Scenario: 绑定外部数据库
- **GIVEN** 用户选择一个云数据库 Connector
- **WHEN** 应用创建绑定
- **THEN** 平台只生成连接绑定而不尝试部署数据库容器
- **AND** 对象类型在 Read Model 中保持可区分

#### Scenario: 按 ProviderID 查找运行目标
- **GIVEN** StepSpec 指定 provider_id: "k8s-prod-01"
- **WHEN** Worker 执行该 Step
- **THEN** ProviderRegistry 返回对应 RuntimeTarget
- **AND** Worker 使用该目标的能力信息执行兼容性检查

### Requirement: [RT-002] 主动连接与最小暴露
中心 Kubernetes/Container Engine 目标 SHOULD 使用 Agent 经 mTLS 主动连接平台，不暴露公网管理端口；KubeEdge 节点 SHALL 使用 CloudHub–EdgeHub 隧道且不重复部署 HNB Agent。

**Traceability:** EDGE-02

#### Scenario: 纳管 NAT 后的集群
- **GIVEN** 目标集群无法被中心直接访问
- **WHEN** Agent 主动注册
- **THEN** 平台建立受认证长连接并发现能力
- **AND** 目标侧无需开放入站管理端口

### Requirement: [RT-003] 能力发现与快照
RuntimeTarget SHALL 上报 Kubernetes/运行时版本、架构、资源、CNI/CSI、Gateway API、GPU/NPU、安全和存储能力；平台 SHALL 保存带时间戳的 CapabilitySnapshot。

**Traceability:** GW-04, GW-13

#### Scenario: 能力发生变化
- **GIVEN** 目标集群升级 Gateway Controller
- **WHEN** 下一次能力探测完成
- **THEN** 平台保存新的能力快照并触发兼容性重算
- **AND** 旧快照保留用于审计

### Requirement: [RT-004] 部署前兼容性预检
平台 SHALL 在生成或执行 ExecutionPlan 前比较 ReleaseManifest 与 RuntimeTarget 能力；不兼容 SHALL 在写入目标之前失败。

**Traceability:** EDGE-06, AI-RUN-05

#### Scenario: 目标资源不足
- **GIVEN** Release 需要 8Gi 内存
- **WHEN** 目标仅有 4Gi 可分配内存
- **THEN** 预检拒绝执行并返回差异
- **AND** 不会在目标侧创建部分资源

### Requirement: [RT-005] 目标状态新鲜度
RuntimeTarget 和边缘资源状态 SHALL 包含 observedAt/lastKnownStateAt；超过新鲜度阈值时，写操作 SHALL 排队、拒绝或要求显式风险确认。

**Traceability:** EDGE-14, EDGE-18

#### Scenario: 对陈旧边缘状态执行升级
- **GIVEN** 节点已离线超过策略阈值
- **WHEN** 用户提交升级
- **THEN** 平台显示状态陈旧并按策略进入 QueuedOffline 或拒绝
- **AND** 不得显示为实时成功
