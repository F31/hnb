## ADDED Requirements

### Requirement: [GW-001] Gateway API 优先
新建 Kubernetes 服务入口 SHALL 默认生成 Gateway API 资源；Ingress SHALL 仅用于存量兼容、迁移和明确选择的回退路径。

#### Scenario: 新应用暴露 HTTP 服务
- **GIVEN** 应用通过标准向导申请公网入口
- **WHEN** 平台生成流量资源
- **THEN** 生成 Gateway/HTTPRoute 而非 Ingress
- **AND** 兼容模式必须显式选择

### Requirement: [GW-002] CRD 集中治理
Gateway API CRD Bundle 和 Channel SHALL 由 Cluster Provider 或集群管理员统一管理；生产环境 SHALL 仅使用经认证 Standard Channel。

#### Scenario: 租户升级 Gateway CRD
- **GIVEN** 业务租户拥有 Route 管理权限
- **WHEN** 租户尝试安装 Experimental Bundle
- **THEN** 操作被拒绝
- **AND** 审计记录高权限尝试

### Requirement: [GW-003] 标准资源与特性协商
Gateway Provider SHALL 至少支持 GatewayClass、Gateway、HTTPRoute、GRPCRoute、ReferenceGrant 和 BackendTLSPolicy，并以 GatewayCapabilitySnapshot 声明 Route 类型及 Core/Extended 特性。

#### Scenario: 产品要求 gRPC 路由
- **GIVEN** 目标 Provider 未认证 GRPCRoute
- **WHEN** 平台进行预检
- **THEN** 部署被拒绝或选择兼容 Provider
- **AND** 不依据 CRD 存在性推断完整能力

### Requirement: [GW-004] 多租户与跨命名空间授权
共享 Gateway SHALL 使用 allowedRoutes、Namespace Selector、RBAC 和 Tenant Context 隔离；跨 Namespace Backend、Secret 或证书引用 SHALL 需要 ReferenceGrant 或规范允许的显式授权。

#### Scenario: 跨命名空间引用后端
- **GIVEN** HTTPRoute 位于租户 A 命名空间
- **WHEN** 其引用租户 B Service 且无 ReferenceGrant
- **THEN** Controller 和平台均拒绝绑定
- **AND** 拒绝原因进入 Route 状态

### Requirement: [GW-005] 流量治理能力
GatewayProfile SHALL 可声明 Host、Path、Header、Query、Method 匹配及权重分流、镜像、重写、重定向、Header 修改、超时和后端 TLS；未认证功能 SHALL 不出现在向导中。

#### Scenario: 灰度发布
- **GIVEN** Provider 已认证权重分流
- **WHEN** 用户配置 90/10 流量
- **THEN** 平台生成可验证 Route
- **AND** Route Accepted 且后端权重正确

### Requirement: [GW-006] 流量产品分层
普通 Gateway、API Management、Service Mesh 和 AI Gateway SHALL 使用独立能力模型、凭据、数据面和审计；普通应用流量 SHALL NOT 被自动导向 AI Gateway。

#### Scenario: 普通业务误选 AI Gateway
- **GIVEN** 非 AI ServiceBlueprint 请求普通 HTTP 入口
- **WHEN** ExposurePolicy 指向 AI GatewayProfile
- **THEN** 预检拒绝配置
- **AND** 返回流量治理分层原因
