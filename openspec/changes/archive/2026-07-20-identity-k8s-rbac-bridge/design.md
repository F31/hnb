## Context

`bootstrap-identity-tenancy` 建立了平台应用层 RBAC：6 角色、13 权限、tenant/project/namespace 三级作用域，存储在 PostgreSQL `user_roles` 表。但该 RBAC 仅控制平台 API 访问（Portal、API Gateway），Kubernetes 集群自身的 RBAC 完全独立。用户在 Portal 被授予 `operator` 角色后，无法通过 kubectl 操作对应 namespace 的资源；运维需要手动创建 RoleBinding，权限管理分散且审计断裂。

本 change 实现平台 RBAC 到 K8s RBAC 的自动同步，使平台成为唯一真相来源。

## Goals / Non-Goals

**Goals:**
- 平台 `user_roles` 表的变更自动同步为 K8s RoleBinding
- 自定义 K8s ClusterRole 精确映射平台角色权限
- 支持 tenant/project/namespace 三级作用域映射
- 平台 JWT 可通过 OIDC 或 TokenReview 被 K8s API Server 信任
- 同步延迟 <= 30 秒（P99）
- 同步失败有重试和告警

**Non-Goals:**
- 不实现 K8s RBAC 反向同步到平台（K8s 不是真相来源）
- 不修改 `user_roles` 表结构（复用现有 schema）
- 不实现 ABAC 或细粒度属性级权限
- 不处理非平台用户（如 K8s ServiceAccount）的 RBAC
- 不实现跨集群同步（单集群 MVP）

## Decisions

### Decision 1: Syncer 架构 — Watch + Reconcile

选择 **List-Watch + WorkQueue + Reconcile** 模式，复用 controller-runtime 惯用模式。

```
user_roles 表 (PostgreSQL)
    |
    ├── Watcher (CDC / 定期轮询)
    │   └── 变更事件入 WorkQueue
    │
    └── Reconcile Loop
        ├── 读取 user_role 记录
        ├── 查找对应的 namespace(s)
        ├── 计算期望的 RoleBinding
        ├── 与当前 K8s 状态 diff
        └── Apply (Create/Update/Delete)
```

**备选方案：**

| 方案 | 评估 |
|------|------|
| 直接监听 PostgreSQL 逻辑复制 (CDC) | 引入 Debezium 或 pgoutput 插件，增加运维复杂度。放弃。 |
| 应用层发出事件（NATS） | 需要改造角色分配 API 发送事件。可行但增加侵入性。 |
| **定期轮询 + Informer 缓存** ✅ | 简单可靠，30 秒轮询间隔可接受，无额外组件依赖。 |

最终选择定期轮询（30 秒） + Informer 缓存 K8s 端 RoleBinding 状态，避免全量扫描。

### Decision 2: K8s ClusterRole 映射

| 平台角色 | K8s ClusterRole | 类型 | 说明 |
|----------|----------------|------|------|
| `platform_admin` | `cluster-admin` | 预置 | 集群级别 ClusterRoleBinding |
| `tenant_admin` | `hnb:tenant-admin` | 自定义 | 该 tenant 下所有 namespace 的管理权限 |
| `project_admin` | `hnb:project-admin` | 自定义 | edit + secrets 读写 + 部署 |
| `operator` | `hnb:operator` | 自定义 | edit (不含 secrets 管理) |
| `publisher` | `hnb:publisher` | 自定义 | edit (部署) + view (只读) |
| `readonly` | `view` | 预置 | 只读 |

自定义 ClusterRole 的权限定义：

```yaml
# hnb:tenant-admin
rules:
  - apiGroups: ["*"]
    resources: ["*"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  # 限制：不能删除 namespace 本身（防止误删）
  - apiGroups: [""]
    resources: ["namespaces"]
    verbs: ["get", "list", "watch"]

# hnb:project-admin
rules:
  - apiGroups: ["*"]
    resources: ["*"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["apps", "extensions"]
    resources: ["deployments", "statefulsets", "daemonsets"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

# hnb:operator
rules:
  - apiGroups: ["*"]
    resources: ["*"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: []  # 不可访问 secrets

# hnb:publisher
rules:
  - apiGroups: ["apps", "extensions"]
    resources: ["deployments", "statefulsets", "daemonsets"]
    verbs: ["get", "list", "watch", "create", "update", "patch"]
  - apiGroups: [""]
    resources: ["pods", "services", "configmaps", "events"]
    verbs: ["get", "list", "watch"]
```

### Decision 3: 作用域映射规则

| 平台作用域 | K8s RoleBinding 位置 |
|-----------|---------------------|
| tenant 级 (`user_id + tenant_id + role_id`, project_id IS NULL) | 该 tenant 下**所有 namespace** 各创建一个 RoleBinding |
| project 级 (`user_id + tenant_id + project_id + role_id`, namespace_id 为 scope) | 该项目下**所有 namespace** 各创建一个 RoleBinding |
| namespace 级 (`user_id + tenant_id + project_id + namespace_id + role_id`) | 单个 namespace 创建一个 RoleBinding |

RoleBinding 命名约定：`hnb:{role_name}:{tenant_id}:{project_id}:{namespace_id}`

RoleBinding 中引用的 Subject：
```yaml
subjects:
  - kind: User
    name: hnb:{user_id}    # platform 用户 ID
    apiGroup: rbac.authorization.k8s.io
```

### Decision 4: 认证 — OIDC 优先

平台 JWT 通过 OIDC 配置被 K8s API Server 信任。

```
K8s API Server 配置：
--oidc-issuer-url=https://platform.hnb.cloud/auth
--oidc-client-id=hnb-platform
--oidc-username-claim=sub
--oidc-groups-claim=roles
--oidc-username-prefix=hnb:
```

User 名称为 `hnb:{user_id}`，与 RoleBinding Subject 中的 name 一致。

**备选方案：** TokenReview API。需要在 K8s 侧部署一个认证 webhook，增加运维复杂度且引入单点故障。OIDC 是更成熟的标准方案。

### Decision 5: 处理 Namespace 创建回填

当一个新的 namespace 被创建时（在平台 `namespaces` 表中），Syncer 需要回填已有的角色绑定。

流程：
1. Watcher 检测到 `namespaces` 表新增记录
2. 查找该 environment_id 对应的 project_id 和 tenant_id
3. 查找该 tenant_id + project_id 下所有活跃的 user_roles
4. 为每个角色在 namespace 中创建 RoleBinding

## 架构

```
Platform API (角色分配)
    |
    v
user_roles 表 (PostgreSQL)
    |
    | (每 30 秒轮询)
    v
K8s RBAC Syncer
    ├── UserRoleWatcher  ──→  WorkQueue  ──→  ReconcileLoop
    ├── NamespaceWatcher ──→  WorkQueue  ──→  ReconcileLoop
    └── K8s Informer (缓存现有 RoleBinding)
          |
          v
    K8s API (Create/Update/Delete RoleBinding)
```

## Risks / Trade-offs

| 风险 | 缓解措施 |
|------|----------|
| Syncer 故障导致权限延迟同步 | 引入重试队列 + 健康检查 + Prometheus 告警；同步延迟 > 60 秒触发告警 |
| OIDC 故障导致认证失败 | 保留 kubectl x509 证书作为紧急备用通道 |
| 大量 namespace 导致轮询性能问题 | Informer 缓存 + 分批 reconcile；1000 namespace 测试基准 |
| 自定义 ClusterRole 与平台角色不同步 | 版本化 ClusterRole YAML，随平台版本发布；变更需经过 spec review |
| 误删除 RoleBinding 导致权限丢失 | Reconcile 使用 diff 模式，不做全量替换；保留手动创建的 RoleBinding |

## Migration Plan

1. 创建自定义 ClusterRole YAML 清单并部署到目标集群
2. 部署 K8s RBAC Syncer（独立 Deployment）
3. 启动影子模式（仅日志，不实际创建 RoleBinding），验证差异
4. 观察 1 周，确认差异率 < 0.1%
5. 切换为主动模式，开始同步
6. 配置 OIDC 对接（可选步骤，不影响 RoleBinding 同步）
7. 灰度：先启用一个测试租户，观察 1 周，再全量

回滚：禁用 Syncer Deployment，K8s 侧 RoleBinding 保留（不自动删除），平台侧回滚 `user_roles` 后手动清理。

## Open Questions

- 如何处理 `platform_admin` 的集群级别 ClusterRoleBinding？是否需要在所有 namespace 都绑定？
- OIDC issuer 的生产部署方案（独立服务还是复用平台认证服务？）
- 存量集群中已有手动创建的 RoleBinding 如何处理？是否纳入管理？
