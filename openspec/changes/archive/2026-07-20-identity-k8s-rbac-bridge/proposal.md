## Why

平台 RBAC（应用层 6 角色 + 审批策略）与 Kubernetes RBAC 之间没有映射关系。用户在平台 Portal 被授予角色后，无法直接通过 kubectl 或工作负载身份访问对应 K8s 资源；运维人员需要手动创建 RoleBinding，导致权限管理分散、审计断裂。本 change 实现平台 RBAC 到 K8s RBAC 的自动同步，使平台成为权限管理的唯一真相来源，K8s 侧通过自定义 ClusterRole + Controller 自动同步。

## What Changes

- 定义 4 个自定义 K8s ClusterRole 映射平台角色权限（`hnb:tenant-admin`、`hnb:project-admin`、`hnb:publisher`、`hnb:readonly`）
- 实现 K8s RBAC Syncer Controller，监听 `user_roles` 表变更，自动创建/更新/删除对应 namespace 的 RoleBinding
- 处理 namespace 创建时的回填（现有角色分配自动补齐 RoleBinding）
- 处理角色 revoked 时的清理（自动删除对应 RoleBinding）
- 平台 Token 认证对接 K8s OIDC 或 TokenReview，使平台 JWT 可同时访问 K8s API
- 新增 RoleBinding 审计日志和同步状态指标

## Capabilities

### New Capabilities
- `identity-k8s-rbac-bridge`: 平台 RBAC 到 Kubernetes RBAC 的自动同步机制。平台是唯一真相来源，通过 Syncer Controller 将用户角色分配转换为 K8s RoleBinding，支持 tenant/project/namespace 三级作用域的自定义 ClusterRole 映射。

### Modified Capabilities
- `identity-tenancy`: `user_roles` 表的变更现在会触发 K8s 侧 RBAC 同步；TenantContext 中的 namespace_id 作用域将映射到 K8s namespace 级别的 RoleBinding。

## Impact

- **代码**: K8s RBAC Syncer Controller（Go）、自定义 ClusterRole YAML 清单、平台 Token 认证适配（OIDC/TokenReview）、审计日志扩展。
- **API/事件**: `user_roles` 表变更触发 CDC 事件，Syncer 订阅并响应。
- **数据**: 无新表；`user_roles` 现有结构直接使用。新增审计事件类型 `rbac_sync`。
- **依赖**: 依赖 `bootstrap-identity-tenancy` 提供的 `user_roles` 表结构；依赖 K8s API（client-go）；不新增强制中间件。
- **资源**: T1 标准可选；Controller 为独立 Deployment，资源需求 < 0.5 CPU / 256MB。
- **运维**: 新增 Syncer 部署、监控（同步延迟、失败计数）、回滚（禁用 Syncer，K8s 侧 RoleBinding 保留）。
