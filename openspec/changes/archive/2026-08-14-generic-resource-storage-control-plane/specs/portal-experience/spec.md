## ADDED Requirements

### Requirement: [UX-012] 存储供给与消费视图分离
Portal SHALL 在“资源 → 存储”提供平台管理员的存储总览、存储系统、存储服务、驱动与连接器和告警入口，并在“容器 → 存储”提供应用管理员的 StorageClass、PVC、PV 和 Snapshot 消费视图；两个视图 SHALL 通过 Offering/Binding 链接而不重复拥有同一事实。

**Traceability:** UX-001, UX-002, STO-001, STO-002

#### Scenario: 从存储服务跳转 StorageClass
- **GIVEN** 一个 Offering 在三个目标上有 Binding
- **WHEN** 平台管理员点击关联 StorageClass 数量
- **THEN** Portal 跳转到容器存储并携带 Offering 与目标过滤上下文
- **AND** 列表只展示用户有权访问的 Binding

### Requirement: [UX-013] 存储页面基于能力和新鲜度呈现
Portal SHALL 展示存储事实的来源、新鲜度和 Unknown/Elastic/NotReported 状态，并根据安装能力、Provider Conformance、用户权限和目标能力显示动作；隐藏动作 SHALL NOT 替代服务端授权。

**Traceability:** UX-001, P1-CONSOLE-002, STO-003, RT-005

#### Scenario: 观察数据已陈旧
- **GIVEN** 存储系统最后观测时间超过租户阈值
- **WHEN** 用户打开资源存储详情
- **THEN** Portal 显示 Stale 和最后观测时间
- **AND** 需要新鲜事实的危险动作被禁用并由服务端独立拒绝

### Requirement: [UX-014] 存储兼容路由迁移
Portal SHALL 在新资源存储能力稳定前保留旧容器存储路由；切换时 SHALL 使用数据库导航版本和兼容重定向保留目标、命名空间、Offering 与 StorageClass 查询上下文。

**Traceability:** UX-006, UX-007, STO-002

#### Scenario: 访问旧存储书签
- **GIVEN** 用户保存了旧容器存储 URL 与 StorageClass 过滤参数
- **WHEN** 导航版本已切换到新消费视图
- **THEN** Portal 重定向到兼容页面并保留过滤上下文
- **AND** 权限不足时服务端仍拒绝数据读取
