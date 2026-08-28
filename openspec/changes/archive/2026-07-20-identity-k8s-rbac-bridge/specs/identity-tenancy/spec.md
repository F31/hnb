## MODIFIED Requirements

### Requirement: [TENANT-007] RBAC 角色与权限
平台 SHALL 支持平台管理员、租户管理员、项目管理员、运维人员、发布者和只读用户六种角色，支持角色继承、权限矩阵和策略绑定，授权作用域 SHALL 支持 tenant/project/namespace 三级；高风险 Operation SHALL 绑定审批策略，进入 PendingApproval 状态。`user_roles` 表的变更 SHALL 触发 K8s RoleBinding 的自动同步。

**Traceability:** TENANT-003

#### Scenario: 审批策略阻止高风险操作
- **GIVEN** 运维用户提交生产数据库切换 Operation
- **WHEN** 审批策略要求租户管理员审批
- **THEN** Operation 进入 PendingApproval
- **AND** 只有租户管理员审批后进入 Queued

#### Scenario: Namespace 级授权
- **GIVEN** 用户被授予 Project A 的 Namespace NS1 的 `operator` 角色
- **WHEN** 其尝试操作 Project A 的同环境下另一 Namespace NS2
- **THEN** 授权引擎拒绝该请求
- **AND** 返回 403 Forbidden

#### Scenario: 角色分配触发 K8s 同步
- **GIVEN** 平台管理员为用户授予 Project P1 的 `operator` 角色
- **WHEN** `user_roles` 表写入新记录
- **THEN** K8s RBAC Syncer 在 30 秒内在 P1 的所有 namespace 中创建 RoleBinding
- **AND** 同步结果记录在审计日志中
