## Why

HNB 当前为单集群架构，所有 Provider 和 Operation Worker 部署在单一 Kubernetes 集群中。生产环境需要多集群治理能力：跨集群调度工作负载、故障转移、流量分发。引入 Karmada 作为多集群管理控制面，GSLB 作为跨集群流量调度。

## What Changes

- **新增 multi-cluster 领域规格**：定义多集群资源模型、调度策略、故障转移策略
- **新增 Karmada 集成 Provider**：HNB 控制面通过 Karmada API 将资源下发到成员集群
- **新增 GSLB Provider**：跨集群 DNS/流量分发（基于 CoreDNS + 健康检查）
- **修改 kubernetes-provider**：支持多集群 deployment 目标（通过 Karmada PropagationPolicy）
- **修改 operation-worker**：支持跨集群 operation 路由
- **新增数据库迁移**：cluster 资源注册表、多集群调度策略、跨集群 operation 追踪

## Capabilities

### New Capabilities
- `multi-cluster`: 多集群资源注册、心跳、状态聚合、跨集群调度策略
- `gslb`: 全局流量负载均衡，基于 DNS 的健康感知流量分发

### Modified Capabilities
- `kubernetes-runtime-provider`: 新增 PropagationPolicy 支持，允许跨集群 deployment
- `composition-operation`: 新增跨集群 operation 类型和路由规则

## Impact

- **新依赖**：Karmada v1.12+（控制面）、CoreDNS + 外部 DNS（GSLB）
- **新服务**：`cmd/multi-cluster-controller`（Karmada 集成）、`cmd/gslb-controller`（DNS 流量调度）
- **修改服务**：`cmd/kubernetes-provider`（多集群 deployment）、`cmd/operation-worker`（跨集群路由）
- **数据库**：新增 2 张表（clusters、cluster_heartbeats）
- **T2 分级**：多集群治理为 T2 标准可选能力，非 T0 内核必装
- **不涉及**：不修改 Portal、不修改 NATS 主题结构、不修改 Outbox 事件格式