## ADDED Requirements

### Requirement: [ADL-001] 显式类型化应用组合
平台 SHALL 将应用的工作负载、Service、PVC、SecretReference 挂载和入口绑定表达为受版本化 Schema 约束的组合，并在目标写入前验证租户、命名空间、配额、能力快照、实际 CSI/入口实现、依赖和逐动作 Conformance。每项运行变更 SHALL 经服务端生成的不可变 ExecutionPlan 和 Operation 落地，Provider 路由由能力声明决定；不得将现有仅支持 Deployment 的动作解释为支持其他资源或任意 Manifest。

**Traceability:** KERNEL-002, P1-WRITE-002, KRP-001, KRP-002, STO-004, RT-004, PROV-003

#### Scenario: Provider 不支持 PVC 动作
- **GIVEN** 应用引用 PVC，而目标已登记的执行能力只支持 Deployment
- **WHEN** 用户请求应用计划
- **THEN** 平台返回缺失的 PVC 能力与稳定原因，不产生目标侧写入
- **AND** 不回退到任意 YAML 或扩大 Deployment 动作权限

#### Scenario: 配额或租户不匹配
- **GIVEN** 应用的命名空间配额不足或 SecretReference 属于其他租户
- **WHEN** 生成执行计划
- **THEN** 平台在任何资源创建之前拒绝请求，错误不泄露凭据或其他租户内容

### Requirement: [ADL-002] 应用可用性与执行成功分离
平台 SHALL 分别展示请求受理、Operation 执行、目标观测和应用验证结果，只有期望版本对应的新鲜观测、资源就绪、持久卷实际读写与外部入口探针全部通过才声明参考应用 Ready。验证 SHALL 绑定 tenant、target、release/revision、资源 UID、Operation 和观测水位并具有批准的超时；Operation succeeded 或 HTTP 202 SHALL NOT 单独证明应用可用，验证失败不得改写已有 Operation 终态。

**Traceability:** KRP-004, RT-005, RT-008, OBS-001, PREADY-001

#### Scenario: 部署成功但入口不可访问
- **GIVEN** 部署 Operation succeeded，但外部入口探针持续失败
- **WHEN** 应用验证超过批准时限
- **THEN** 平台保留真实 Operation 结果并将应用验证标为失败或降级，给出入口原因
- **AND** UI/CLI 均不显示应用 Ready

#### Scenario: 有状态参考应用交付
- **GIVEN** 独立业务集群具备已验证的工作负载、存储和入口能力
- **WHEN** 正式链路部署参考应用并从集群外写入和读取唯一数据标记
- **THEN** 验收关联版本、卷、入口与观测身份且证明数据实际存于持久卷

### Requirement: [ADL-003] 版本升级和受控恢复
应用升级 SHALL 固定源/目标 release 摘要、配置、资源身份、数据兼容条件和所需备份点，经受治理 Operation 执行并验证目标版本可用。失败 SHALL 保留原可用版本或发起显式关联的恢复 Operation；恢复 SHALL 复验既有数据与外部访问，不覆盖历史执行事实。不可逆数据迁移或不兼容版本 SHALL 阻断自动降级并返回前向修复或批准恢复路径。

**Traceability:** OP-004, KERNEL-002, PROV-008, OBS-003, ADL-002

#### Scenario: 新版本健康检查失败
- **GIVEN** 应用 v1 可用且已有持久数据，v2 不含不可逆数据迁移
- **WHEN** v2 发布后无法通过健康验证
- **THEN** 平台保留 v1 或通过关联恢复 Operation 恢复 v1，并复验原数据与外部访问
- **AND** 升级失败、恢复结果和实际 revision 分别可追溯

#### Scenario: 试图回退不兼容数据版本
- **GIVEN** 应用已执行不可逆数据迁移且没有兼容降级路径
- **WHEN** 用户请求自动回滚旧版本
- **THEN** 平台拒绝直接降级并提供受控恢复或前向修复条件

### Requirement: [ADL-004] 应用删除保留与迟到执行防护
应用删除 SHALL 依据授权范围、资源所有权、UID 和 fencing generation 执行，默认保留 PVC、持久数据、审计和防重放墓碑。永久数据删除 SHALL 使用独立授权、影响预览和非预选的显式确认；非托管或其他租户资源 SHALL NOT 被接管或删除。依赖解除和数据保留结果 SHALL 可由 UI/CLI 查询。

**Traceability:** KRP-002, KRP-003, KRP-006, PROV-009, TENANT-006, STO-005

#### Scenario: 删除应用后迟到的部署请求到达
- **GIVEN** 应用已以更高 generation 删除且 PVC 被保留
- **WHEN** 较旧部署 Step 延迟重放
- **THEN** 执行被 fencing，应用不会复活，原 PVC 和数据不变

### Requirement: [ADL-005] 可选 GitOps 集成的唯一写入所有权
可选 GitOps 集成 SHALL 为每个受管资源声明 HNB 或外部控制器的写入 owner 和版本。外部 owner 模式下 HNB SHALL 仅观测或通过受治理 Operation 修改受控声明源后等待收敛，不得同时直接修改相同目标资源；所有权移交 SHALL 验证身份和 revision、停止旧 writer、原子更新 owner 并留存审计。未安装、未认证或延期的适配器 SHALL 不被表示为可用能力。

**Traceability:** KERNEL-002, PROV-003, PROV-004, ADL-001

#### Scenario: 外部控制器已拥有目标资源
- **GIVEN** 资源 owner 为已接入的 GitOps 控制器
- **WHEN** HNB 收到直接目标写入请求
- **THEN** 平台拒绝冲突写入或提交到声明源的受控流程，并等待真实收敛
- **AND** 不产生两个并行修改同一资源的 writer

#### Scenario: 所有权移交时版本冲突
- **GIVEN** 移交计划固定的资源 revision 已变化
- **WHEN** 申请启用新 writer
- **THEN** 平台拒绝移交并保持原 owner，重新计划之前不启动新 writer
