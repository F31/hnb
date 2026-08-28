## 1. Custom K8s ClusterRole Definitions

- [x] 1.1 `[K8S-RBAC-002]` 定义 `hnb:tenant-admin` ClusterRole（所有资源的完全控制权，但不可删除 namespace）
- [x] 1.2 `[K8S-RBAC-002]` 定义 `hnb:project-admin` ClusterRole（edit + secrets 读写 + workloads 管理）
- [x] 1.3 `[K8S-RBAC-002]` 定义 `hnb:operator` ClusterRole（edit 但不含 secrets 访问）
- [x] 1.4 `[K8S-RBAC-002]` 定义 `hnb:publisher` ClusterRole（deployments/statefulsets 读写 + pods/services/configmaps/events 只读）
- [x] 1.5 `[K8S-RBAC-002]` 定义 ClusterRole 间继承关系映射（与平台角色继承对齐）并编写验证测试

## 2. K8s RBAC Syncer 核心框架

- [x] 2.1 `[K8S-RBAC-001]` 创建 Syncer Go 模块骨架（main.go、config、启动流程）
- [x] 2.2 `[K8S-RBAC-001]` 实现 UserRoleWatcher：定期轮询 `user_roles` 表（30 秒间隔），提取活跃角色变更
- [x] 2.3 `[K8S-RBAC-001]` 实现 K8s Informer 缓存：ListWatch 当前 namespace 的 RoleBinding 状态
- [x] 2.4 `[K8S-RBAC-001]` 实现 WorkQueue + ReconcileLoop 框架

## 3. RoleBinding 同步逻辑

- [x] 3.1 `[K8S-RBAC-003]` 实现角色→ClusterRole 映射函数（平台角色 → K8s ClusterRole 名称）
- [x] 3.2 `[K8S-RBAC-003]` 实现作用域解析：tenant 级 → 查找所有 namespace，project 级 → 查找项目下 namespace，namespace 级 → 单 namespace
- [x] 3.3 `[K8S-RBAC-001]` 实现 Reconcile 函数：计算期望 RoleBinding → diff 当前状态 → Create/Update/Delete
- [x] 3.4 `[K8S-RBAC-005]` 实现角色撤销的 RoleBinding 删除逻辑（revoked_at 非空时自动清理）
- [x] 3.5 `[K8S-RBAC-001]` 实现重试和错误处理（指数退避 + 最大重试次数）

## 4. Namespace 创建回填

- [x] 4.1 `[K8S-RBAC-004]` 实现 NamespaceWatcher：监听 `namespaces` 表新增记录
- [x] 4.2 `[K8S-RBAC-004]` 实现回填逻辑：查找 tenant/project 下所有活跃角色 → 为每个角色在 namespace 创建 RoleBinding
- [x] 4.3 `[K8S-RBAC-004]` 实现回填的幂等性（已存在的 RoleBinding 不重复创建）

## 5. 同步可观测

- [x] 5.1 `[K8S-RBAC-006]` 暴露 Prometheus 指标：sync_latency_seconds、sync_total（status=success/failed）、managed_rolebindings、sync_errors
- [x] 5.2 `[K8S-RBAC-006]` 实现健康检查端点（/healthz、/readyz）
- [x] 5.3 `[K8S-RBAC-006]` 配置 P2 告警规则（同步失败 > 60 秒）
- [x] 5.4 `[K8S-RBAC-006]` 同步失败事件的审计日志记录

## 6. OIDC 认证集成

- [x] 6.1 `[K8S-RBAC-001]` 配置 K8s API Server OIDC 参数（issuer-url、client-id、username-claim、groups-claim）
- [x] 6.2 `[K8S-RBAC-001]` 实现平台 JWT 的 `sub` claim 格式对齐（`hnb:{user_id}`）
- [x] 6.3 `[K8S-RBAC-001]` 编写 OIDC 配置验证测试（token 签发 → K8s API 认证 → 授权检查）

## 7. 影子模式与灰度发布

- [x] 7.1 `[K8S-RBAC-001]` 实现影子模式：仅日志记录期望的 RoleBinding 变更，不实际执行
- [x] 7.2 `[K8S-RBAC-001]` 配置 Syncer Deployment Helm Chart（资源限制、环境变量、RBAC 权限）
- [x] 7.3 `[K8S-RBAC-001]` 执行 E2E 测试：角色分配 → 同步 → kubectl 访问 → 角色撤销 → 权限移除
- [x] 7.4 `[K8S-RBAC-006]` 执行故障测试：Syncer 故障恢复、K8s API 不可用重试、大量 namespace 性能基准
