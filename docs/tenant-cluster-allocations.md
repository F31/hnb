# 共享集群的租户配额分配

## 模型

集群是平台共享资源池，不再是单一租户的所有物。租户获得某个集群的访问资格和硬额度，均由 `tenant_cluster_allocations` 记录；Workspace 仅用于租户内部的团队、项目或成本中心分组，可完全省略。

```text
Tenant
  ├─ TenantClusterAllocation (Cluster A, quota A)
  │    └─ NamespaceBinding (Cluster A, namespace, optional Workspace)
  └─ TenantClusterAllocation (Cluster B, quota B)
       └─ NamespaceBinding (Cluster B, namespace, optional Workspace)
```

租户的展示总配额为所有 `active` allocation 的资源逐项求和；准入不能只看总数，必须同时验证所选集群上该 allocation 的剩余额度。

## 管理 API

| API | 用途 |
| --- | --- |
| `GET /api/v1/tenants/{id}/cluster-allocations` | 列出分配并返回 `total_quota` |
| `PUT /api/v1/tenants/{id}/cluster-allocations/{cluster_id}` | 创建或更新该租户在集群中的配额 |
| `DELETE /api/v1/tenants/{id}/cluster-allocations/{cluster_id}` | 删除空分配；仍有命名空间时拒绝 |
| `GET /api/v1/namespaces` | 租户直接列出命名空间，支持 `cluster_id` 过滤 |
| `POST /api/v1/namespaces` | 创建命名空间；`workspace_id` 可选，`cluster_id` 必填 |

Allocation 写入示例：

```json
{
  "quota": { "cpu": 40, "memory": 160, "gpu": 2 },
  "status": "active",
  "namespace_prefix": "tenant-a",
  "isolation_enabled": true
}
```

命名空间创建时，服务端检查：租户存在有效 allocation、命名空间声明配额与同一集群中该租户其他命名空间配额之和不超过 allocation。删除 allocation 前必须先迁移或删除其命名空间。

## 兼容与迁移

`cluster_shares` 与 `/api/v1/workspaces/{workspace_id}/bind-cluster` 暂时保留，以兼容已部署的工作空间绑定。新建租户用量应改走 allocation API；Workspace 是可选的 `workspace_id`，不能再作为共享集群访问或容量分配的唯一依据。

执行数据库迁移 `066_tenant_cluster_allocations.sql` 后，`namespaces.workspace_id` 可为 NULL，命名空间名称在同一 `tenant + cluster` 内唯一，因此同一租户可在不同集群都使用 `prod`。
