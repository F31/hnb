## ADDED Requirements

### Requirement: [UX-005] 租户、角色和审批策略管理界面
Portal SHALL 提供租户管理、项目/环境管理、Namespace 管理、角色管理、用户角色分配、审批策略配置和 SecretReference 管理界面；界面 SHALL 按登录用户角色显示可用操作，平台管理员可管理所有租户，租户管理员仅管理所属租户。

**Traceability:** TENANT-005, TENANT-006, TENANT-007, TENANT-008

#### Scenario: 租户管理员查看用户角色
- **GIVEN** 租户管理员登录 Portal
- **WHEN** 其导航到用户管理页面
- **THEN** 显示所属租户下的用户列表和角色分配
- **AND** 无法看到其他租户的用户或角色

#### Scenario: 平台管理员创建审批策略
- **GIVEN** 平台管理员登录 Portal
- **WHEN** 其创建审批策略并绑定到 `database-failover` 操作类型
- **THEN** 策略保存后对该操作类型生效
- **AND** 租户管理员可在租户范围内查看该策略

#### Scenario: 项目管理员创建 Namespace
- **GIVEN** 项目管理员登录 Portal
- **WHEN** 其进入 Project A 的 Production 环境，创建一个 Namespace 并指定 suffix 为 `api`
- **THEN** 系统自动生成 Namespace 名称为 `{tenant}-{project}-production-api`
- **AND** Namespace 显示在环境的 Namespace 列表中

#### Scenario: 多 Namespace 环境视图
- **GIVEN** Production 环境包含 api、worker、cache 三个 Namespace
- **WHEN** 项目管理员查看 Production 环境详情
- **THEN** 显示三个 Namespace 及其状态、标签信息
- **AND** 可分别管理每个 Namespace