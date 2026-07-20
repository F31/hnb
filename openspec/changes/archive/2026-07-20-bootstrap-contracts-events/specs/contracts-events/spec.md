## ADDED Requirements

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
