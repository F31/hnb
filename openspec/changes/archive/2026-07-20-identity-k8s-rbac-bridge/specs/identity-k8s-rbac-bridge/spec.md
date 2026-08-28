## ADDED Requirements

### Requirement: [K8S-RBAC-001] 平台到 K8s 的 RBAC 自动同步
平台 SHALL 自动将 `user_roles` 表的角色分配同步为 Kubernetes RoleBinding；平台 SHALL 是 RBAC 的唯一真相来源，K8s 侧不允许反向修改平台管理的 RoleBinding；同步延迟 SHALL <= 30 秒（P99）。

**Traceability:** TENANT-007

#### Scenario: 用户被授予 project_admin 角色
- **GIVEN** 用户被平台授予 Project P1 的 `project_admin` 角色
- **WHEN** RBAC Syncer 检测到 `user_roles` 表变更
- **THEN** 在 Project P1 的所有 namespace 中创建对应的 RoleBinding
- **AND** 该用户可以通过 kubectl 操作这些 namespace 的资源

### Requirement: [K8S-RBAC-002] 自定义 ClusterRole 映射
平台 SHALL 定义并维护 `hnb:tenant-admin`、`hnb:project-admin`、`hnb:operator`、`hnb:publisher` 四个自定义 K8s ClusterRole，每个 ClusterRole 的权限矩阵 SHALL 与平台对应角色的权限精确对齐。

**Traceability:** TENANT-007

#### Scenario: operator 角色不可访问 Secrets
- **GIVEN 用户被授予 `operator` 角色
- **WHEN** 用户通过 kubectl 尝试读取目标 namespace 的 Secret
- **THEN** K8s API Server 拒绝该请求
- **AND** 返回 403 Forbidden

### Requirement: [K8S-RBAC-003] 三级作用域同步
平台 SHALL 支持 tenant、project、namespace 三级作用域的 RBAC 同步；tenant 级角色 SHALL 同步到该 tenant 下所有 namespace，project 级角色 SHALL 仅同步到该项目下 namespace，namespace 级角色 SHALL 仅同步到目标 namespace。

**Traceability:** TENANT-007

#### Scenario: namespace 级角色精确绑定
- **GIVEN** 用户被授予 Environment E1 的 Namespace NS1 的 `operator` 角色
- **WHEN** RBAC Syncer 执行同步
- **THEN** RoleBinding 仅创建在 NS1 中
- **AND** NS2（同环境）中不包含该用户的 RoleBinding

### Requirement: [K8S-RBAC-004] Namespace 创建回填
当一个新的 namespace 被创建时，平台 SHALL 自动将该 tenant/project 下所有已有活跃角色分配回填到该 namespace。

**Traceability:** TENANT-007

#### Scenario: 新增 namespace 自动继承角色绑定
- **GIVEN** Project P1 已有用户 U1 被授予 `operator` 角色
- **WHEN** 在该 project 下创建新的 namespace NS2
- **THEN** Syncer 自动在 NS2 中创建用户 U1 的 RoleBinding
- **AND** 用户 U1 无需重新授权即可访问 NS2

### Requirement: [K8S-RBAC-005] 角色回收同步
当用户在平台的角色被撤销时，平台 SHALL 自动删除对应 K8s RoleBinding。

**Traceability:** TENANT-007

#### Scenario: 用户角色被撤销
- **GIVEN** 用户 U1 拥有 Project P1 的 `operator` 角色（对应 RoleBinding 已存在）
- **WHEN** 平台撤销该用户的角色
- **THEN** RBAC Syncer 删除 P1 所有 namespace 中对应的 RoleBinding
- **AND** 用户 U1 无法再通过 kubectl 操作这些 namespace

### Requirement: [K8S-RBAC-006] 同步可观测
平台 SHALL 暴露 RBAC 同步的指标：同步延迟（秒）、同步成功/失败计数、当前管理 RoleBinding 数、同步失败详情（失败原因 + 资源标识）。

**Traceability:** TENANT-007

#### Scenario: 同步失败告警
- **GIVEN** RBAC Syncer 因 K8s API 不可用导致同步失败
- **WHEN** 失败持续时间超过 60 秒
- **THEN** 触发 P2 告警通知运维
- **AND** 失败记录包含失败原因和受影响资源标识
