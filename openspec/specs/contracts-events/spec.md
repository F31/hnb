# contracts-events

## Purpose
定义市场、平台、AI、Edge 与 Provider 之间的版本化公共 API、事件、幂等、关联和可靠投递契约。

## Requirements

### Requirement: [CONTRACT-001] Schema First 公共契约
跨进程和跨平面的 OpenAPI、Protobuf、事件、Manifest 与 Provider 契约 SHALL 先定义 Schema，再生成 SDK；实现 SHALL NOT 共享内部数据库表或内部语言结构体作为公共契约。

**Traceability:** INT-01, INT-02

#### Scenario: 新增市场发布接口
- **GIVEN** 一个 change 需要新增 Release 查询字段
- **WHEN** 设计进入实现前
- **THEN** 对应 OpenAPI/Schema 先完成评审和兼容性检查
- **AND** 客户端 SDK 由 Schema 生成

### Requirement: [CONTRACT-002] 向后兼容与弃用
公共 API 和事件在同一主版本内 SHALL 保持向后兼容；删除或改变语义的字段 SHALL 经过弃用窗口、兼容读写和迁移计划。

**Traceability:** INT-01, GOV-05

#### Scenario: 升级平台 API
- **GIVEN** 旧版 Market Connector 仍在运行
- **WHEN** 平台升级到新次版本
- **THEN** 旧客户端仍可完成既有调用
- **AND** 弃用字段在文档和遥测中可见

### Requirement: [CONTRACT-003] 幂等与关联
所有写 API、事件消费者和 Provider 命令 SHALL 支持 IdempotencyKey 和 Correlation ID；更新或删除既有资源的命令 SHALL 支持期望版本或等价并发控制；重复消息 SHALL NOT 造成重复资源或重复扣费。

**Traceability:** CMPOS-04, EDGE-18

#### Scenario: 事件重复投递
- **GIVEN** 同一个 OperationStarted 事件被投递两次
- **WHEN** 消费者处理第二次投递
- **THEN** 系统识别为重复并返回已处理结果
- **AND** 不会创建第二个运行实例

### Requirement: [CONTRACT-004] 事务事件可靠投递
需要与业务状态一致的事件 SHALL 使用事务 Outbox 或等价机制；事件发布失败 SHALL 可重试且不回滚已提交业务事实。

**Traceability:** INT-01, INT-05

#### Scenario: 发布 Release 后事件总线短时不可用
- **GIVEN** Release 已成功写入市场数据库
- **WHEN** 事件发布失败
- **THEN** Outbox 保留待发送事件并在恢复后投递
- **AND** 事件顺序和去重键可验证

### Requirement: [CONTRACT-006] 公共契约仓库与生成门禁
仓库 SHALL 将版本化 OpenAPI、Protobuf、事件和 Manifest Schema 作为跨进程公共契约的唯一真源，并 SHALL 提供本地与独立验证环境一致调用的 lint、兼容检查、Go/TypeScript 生成和生成物漂移门禁；门禁 SHALL 固定工具版本，SHALL 拒绝同一主版本内的破坏性变化、手工修改生成物以及包含 Secret、目标凭据或大文件正文的公共消息定义。当前 GitHub 仓库 SHALL 仅用于代码托管，不承担持续集成。

**Traceability:** CONTRACT-001, CONTRACT-002, CONTRACT-003, CONTRACT-004, INT-01, INT-05

#### Scenario: 从公共 Schema 重复生成 SDK
- **GIVEN** 一组通过评审的 OpenAPI、Protobuf、事件和 Manifest Schema 以及固定版本的生成工具
- **WHEN** 开发者在提交或评审前连续执行两次统一契约门禁
- **THEN** Go 与 TypeScript 生成物可通过各自编译检查且第二次生成不产生差异
- **AND** 输出包含 Schema 数量、生成器版本、兼容结果和执行耗时

#### Scenario: 提交同一主版本的破坏性字段变更
- **GIVEN** 一个已发布公共字段在同一主版本中被删除、改变类型或复用 Protobuf 字段号
- **WHEN** 开发者或评审者执行兼容性检查
- **THEN** 门禁拒绝该变更并指出契约、字段和兼容规则
- **AND** 修复方式要求恢复兼容定义、完成弃用窗口或创建新主版本

#### Scenario: 公共消息定义包含敏感正文
- **GIVEN** 一个事件或 Manifest Schema 新增 Secret、Token、kubeconfig 或超过批准边界的大文件正文字段
- **WHEN** 执行契约安全门禁
- **THEN** 门禁拒绝该字段并只报告字段路径和违规类型
- **AND** 输出不包含任何敏感值
