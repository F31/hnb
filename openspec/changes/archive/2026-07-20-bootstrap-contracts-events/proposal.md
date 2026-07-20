## Metadata

- Change ID: `bootstrap-contracts-events`
- Tier: T0
- Planes: market, runtime-governance, artifact-storage, ai-extension
- Affected Specs: new `contracts-events/CONTRACT-006`; implements the foundation required by `CONTRACT-001` through `CONTRACT-004`
- Depends On: `bootstrap-openspec-governance`
- Target Milestone: Stage-0
- Risk: medium

## Why

HNB Cloud 已批准 Schema First、向后兼容、幂等关联和 Transactional Outbox 行为，但仓库尚无公共契约目录、生成工具链或兼容门禁。后续 Identity、Provider、Operation、Market、Portal 和 NATS change 如果各自定义类型，将形成不可审计的重复模型和跨平面耦合，因此必须先建立可重复生成、可检查漂移的 T0 契约基础。

## What Changes

- 建立版本化 OpenAPI、Protobuf、JSON Schema/Manifest 和生成物目录约定，明确源 Schema 是唯一真源。
- 定义跨 API 与事件复用的 Tenant/Actor/Correlation、Idempotency、期望版本、资源引用、分页、时间和错误问题详情基础类型。
- 建立 Go 与 TypeScript 契约生成入口、格式/lint、破坏性变化和生成物漂移检查。
- 为公共写 API 定义 Idempotency Key、Correlation ID 和期望版本的传输约定，但不实现业务幂等存储。
- 为领域事件定义与具体 Broker 无关的基础 Envelope，并为后续 Transactional Outbox 与 JetStream change 保留扩展点。
- 增加仓库级契约质量门禁和最小示例契约，证明 SDK 可重复生成且不同语言结果可互操作。
- 固定生成器、运行时和 Schema 版本到 Core BOM；不使用浮动版本或运行时远程插件。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `contracts-events`: 增加公共契约仓库、确定性生成和兼容性门禁行为，落实 CONTRACT-001 至 CONTRACT-004 的工程基础。

## Non-Goals

- 不实现 Identity、Tenant、Provider、Operation Engine、Outbox Relay、NATS JetStream 或任何业务 API。
- 不定义 Release、ExecutionPlan、Alert、Provider 生命周期等完整领域模型；这些由对应 change 增量增加。
- 不建立跨服务共享数据库模型，也不从数据库结构生成公共 API。
- 不选择 Go Web 框架、ORM、消息 Broker、API Gateway 或 Portal 状态管理库。
- 不承诺传输层 exactly-once，也不把基础 Envelope 作为业务状态机。

## Impact

- **代码:** 新增 `contracts/` 源 Schema、生成配置与生成物，新增统一契约校验/生成脚本和自动测试。
- **API/事件:** 新增基础公共类型与示例 API/事件；不发布可执行的业务写接口。
- **依赖:** 增加固定版本的契约 lint、兼容检查和 Go/TypeScript 生成工具；不增加运行时中间件或数据库。
- **数据:** 无数据库 Schema 或数据迁移；生成物只表示公共传输模型。
- **资源:** 本地使用单个短时生成作业，目标在常规环境 120 秒内完成 lint、生成、兼容和漂移检查。
- **运维:** 生成工具进入 Core BOM；升级必须验证输出差异、兼容性和回滚，不新增常驻服务、备份或灾备组件。

## Compatibility and Migration

- 当前没有已发布 SDK 或公共 API，因此首版基础契约不存在存量客户端迁移。
- 后续同一主版本只能增加向后兼容字段；删除、重命名、类型或语义变化必须经过弃用窗口或新主版本。
- 生成物与源 Schema 同版本提交；门禁拒绝手工修改或未重新生成的差异。
- 回滚时恢复上一组 Schema、生成配置和生成物；不得只回滚生成物而保留不匹配 Schema。

## Security and Isolation

- 基础类型只传播 Tenant、Project、Environment、Actor、Correlation 等标识，不携带认证凭据。
- Secret、Token、kubeconfig 和大文件正文不得进入示例或基础事件；敏感数据仅允许使用 SecretReference 或不可变 Payload Reference。
- 生成过程不读取运行 Secret、数据库、RuntimeTarget 或仓库凭据；本地门禁使用固定工具版本。
- 契约不得暴露内部表名、内部 Go struct、数据库主键实现或其他平面私有字段。

## Reliability and Operations

- 相同提交、相同 BOM 和相同平台架构上的生成结果必须稳定，本地门禁通过重新生成后的零差异验证。
- lint、兼容或生成失败返回非零退出码和可定位的 Schema/字段信息。
- 记录各工具版本、生成耗时、Schema 数量和生成物差异，作为 CONTRACT-006 证据。
- 本 change 不新增运行时可用性、备份或灾备要求；仓库版本控制保存 Schema 与生成物历史。

## Rollout and Rollback

1. 固定契约格式、目录、工具版本和生成命令。
2. 引入基础类型、示例 API/事件及 Go/TypeScript 生成物。
3. 运行 lint、兼容、互操作、重复生成和漂移测试。
4. 将统一契约门禁纳入提交前评审证据，后续领域 change 必须复用该入口。
5. 回滚时整体恢复上一版 Schema、配置、工具版本和生成物，并重新执行门禁。

本 change 无数据库或运行时部署，卸载影响仅为后续 change 暂时失去生成基础；一旦下游契约依赖首版包，不得单独删除该基础。

## Exit Criteria

- **GIVEN** 一组有效的 OpenAPI、Protobuf 和 JSON Schema 基础契约，**WHEN** 执行统一契约门禁，**THEN** lint、生成、兼容和漂移检查全部通过，并产生可编译的 Go 与 TypeScript 生成物。
- **GIVEN** 同一输入和固定 BOM，**WHEN** 连续执行两次生成，**THEN** 第二次执行不产生仓库差异。
- **GIVEN** 一个删除字段、改变字段类型或复用 Protobuf 字段号的测试变更，**WHEN** 执行兼容门禁，**THEN** 变更被拒绝并指出契约位置。
- **GIVEN** 一个重复事件携带相同 Idempotency Key 和 Correlation ID，**WHEN** Go 与 TypeScript 生成类型分别编码和读取，**THEN** 标识保持一致且未知可选字段不破坏读取。
- **GIVEN** 基础 Schema 尝试加入 Secret、Token、kubeconfig 或大文件正文，**WHEN** 执行安全门禁，**THEN** 构建失败且输出不包含敏感值。
