## Context

当前仓库只有领域主规格、活动 change 和 OpenSpec 治理脚本，没有 `contracts/`、Go/TypeScript 工程、公共 API、事件实现或生成工具链。`CONTRACT-001` 至 `CONTRACT-004` 已批准 Schema First、向后兼容、幂等关联和 Transactional Outbox 行为；`adopt-nats-jetstream-messaging` 又依赖统一 Envelope，但不得让 NATS 产品细节反向进入公共契约。

本 change 是 Identity、Provider、Operation、Market、Portal 和消息实现的前置基础。利益相关方包括 Go/TypeScript 开发者、Provider 开发者、规格评审者、安全人员和 CI 管理员。设计必须可在当前 WSL/Linux 环境和 GitHub Actions 重复执行，不修改系统级工具，不访问数据库或运行 Secret。

当前系统 Go 1.24.2 安装存在标准库源文件冲突，不能作为批准工具链。工具引导程序因此把固定版本安装到仓库忽略的 `.tools/` 缓存；生成与编译不依赖 `/usr/local/go`。

### Architecture

```text
                         source of truth
                               |
              +----------------+----------------+
              |                |                |
              v                v                v
       contracts/openapi  contracts/proto  contracts/schema
              |                |                |
              +----------------+----------------+
                               |
                    scripts/validate-contracts.mjs
                               |
          +--------------------+--------------------+
          |                    |                    |
          v                    v                    v
     lint / security      compatibility         generation
     Redocly / AJV        oasdiff / Buf       OpenAPI / Protobuf
          |                    |                    |
          +--------------------+--------------------+
                               |
                               v
                    contracts/generated/
                         Go / TypeScript
                               |
                               v
                    compile + drift check
```

运行时服务只能依赖版本化生成包或 Schema Artifact，不允许从其他服务的内部包或数据库模型生成契约。

## Goals / Non-Goals

**Goals:**

- 满足 CONTRACT-001 至 CONTRACT-004 和 CONTRACT-006 的首个可执行工程基础。
- 建立 OpenAPI 3.1、Protobuf 和 JSON Schema Draft 2020-12 的固定目录与版本规则。
- 生成可编译的 Go 1.26.5 与 TypeScript 7.0.2 客户端/消息类型。
- 定义跨 API 与事件复用的上下文、关联、幂等、期望版本、分页、资源引用和错误类型。
- 以一个本地和 CI 共用的命令执行 lint、兼容、安全、生成、编译和漂移检查。
- 不依赖系统 Go、全局 npm 包或运行时远程生成插件。

**Non-Goals:**

- 不实现认证授权、租户数据库隔离、业务幂等表、Outbox Relay 或消息 Broker。
- 不定义完整的 Release、Operation、Provider、Alert、Artifact 或 Edge 领域 API。
- 不生成服务端业务实现或把生成代码作为 HNB Core 的领域模型。
- 不发布 npm module、Go module 或 OCI Artifact 到远端 Registry；首期只验证仓库内生成物。
- 不修复或覆盖 `/usr/local/go`，也不要求开发者全局安装 Buf、protoc 或生成器。

## Decisions

### Decision 1: 按契约类型保存源 Schema，生成物独立存放

目录基线为：

```text
contracts/
├── openapi/
│   └── foundation/v1/openapi.yaml
├── proto/
│   └── hnb/contracts/v1/
├── schema/
│   └── common/v1/
├── generated/
│   ├── go/
│   │   ├── openapi/
│   │   └── proto/
│   └── typescript/
│       ├── openapi/
│       └── proto/
├── buf.yaml
├── buf.gen.yaml
└── toolchain.lock.json
```

`openapi/`、`proto/` 和 `schema/` 是唯一真源；`generated/` 可提交但禁止手工修改。不同类型保持各自标准，而不是先创造 HNB 专用 IDL 再转译，避免维护第四种 Schema 语言。

备选方案是 TypeSpec 统一生成 OpenAPI 和其他类型，但它会在尚无团队经验时增加新的抽象层，且 Protobuf、兼容规则和 Provider SPI 仍需单独治理，因此首期拒绝。

### Decision 2: 固定格式版本与兼容规则

- 外部和跨平面 HTTP 契约使用 OpenAPI 3.1.0。
- 内部 RPC、Provider 和高频消息类型使用 Protobuf `proto3`，package 名包含 `v1`。
- Manifest 与独立事件验证使用 JSON Schema Draft 2020-12，并固定 `$id`。
- 同一主版本允许增加可选字段、增加枚举的兼容值和增加新 endpoint/message。
- 删除字段、必选化、改变类型/语义、复用 Protobuf 字段号或改变 operationId/package 被视为破坏性变更。
- Protobuf 删除字段必须 `reserved` 原字段号和名称。

OpenAPI 使用 oasdiff，Protobuf 使用 Buf breaking；JSON Schema 由仓库脚本检查 required、type、enum、format、additionalProperties 和 `$id` 的兼容变化。破坏性变化必须新建主版本目录并提供迁移窗口。

### Decision 3: 锁定工具链并安装到仓库本地缓存

首个 Core BOM 固定：

| 工具 | 版本 | 用途 |
|---|---:|---|
| Node.js | 20.20.2 | 统一脚本与 npm 工具 |
| npm | 10.8.2 | 锁定 Node 开发依赖 |
| TypeScript | 7.0.2 | TypeScript 生成物编译 |
| Go | 1.26.5 | Go 生成器与生成物编译 |
| Buf | 1.72.0 | Protobuf lint、breaking 和 generation |
| protoc-gen-go | 1.36.11 | Protobuf Go 类型 |
| protoc-gen-es | 2.12.1 | Protobuf TypeScript 类型 |
| Redocly CLI | 2.39.0 | OpenAPI lint 和 bundle |
| oapi-codegen | 2.8.0 | OpenAPI Go 类型与客户端 |
| openapi-typescript-codegen | 0.31.0 | OpenAPI TypeScript Fetch SDK |
| oasdiff | 1.23.0 | OpenAPI breaking 检查 |
| AJV | 8.20.0 | JSON Schema 与示例验证 |

`scripts/bootstrap-contract-tools.mjs` 根据 `contracts/toolchain.lock.json` 下载并校验 SHA-256，把二进制和 Go SDK放入 `.tools/contracts/`。npm 依赖使用精确版本和 lockfile。CI 可缓存 `.tools/`，但每次仍校验版本和摘要。

备选方案是依赖开发者全局工具，但已发现系统 Go 安装不可靠，且全局版本会使本地与 CI 输出漂移。另一个方案是每次使用远程 Buf Plugin，但离线环境不可重现，因此拒绝。

### Decision 4: API 公共语义使用标准 Header 和 Problem Details

首个 OpenAPI 基础契约定义：

```text
X-Correlation-ID  必须为 UUID；客户端可提供，入口校验或生成
Idempotency-Key   所有写操作必需，1-128 个可打印字符
If-Match          更新/删除时携带服务端 ETag，对应期望版本
ETag              服务端返回当前资源版本
traceparent       遵守 W3C Trace Context，不替代 Correlation ID
```

错误使用 RFC 9457 Problem Details，扩展 `code`、`correlationId`、`violations[]`，不得包含 Secret 或内部堆栈。列表使用不透明 cursor，不把数据库 offset 或主键定义为公共契约。

Tenant/Project/Environment 由认证上下文、资源路径或受信服务凭据确定，不能仅信任浏览器自报 Header。基础类型允许内部契约传播这些 ID，但授权语义由 `bootstrap-identity-tenancy` 决定。

### Decision 5: Broker-neutral EventEnvelope

Protobuf 与 JSON Schema 表达相同逻辑 Envelope：

```text
EventEnvelope
- messageId: UUID
- messageType: versioned string
- schemaVersion: semantic version
- occurredAt: UTC timestamp
- tenantId/projectId/environmentId: scoped identifiers
- actorId: optional identifier
- correlationId: UUID
- causationId: optional message UUID
- idempotencyKey: bounded string
- aggregateId/aggregateVersion: optional concurrency context
- payloadRef: optional immutable digest/reference
- payload: bounded typed body
```

Envelope 不包含 NATS Subject、Stream sequence、ACK 或 Consumer 信息。Broker metadata 留在 transport adapter。消息不得包含明文 Secret、kubeconfig、Token 或大文件正文。后续 `CONTRACT-005` 可增加持久传输行为而不改变该权威边界。

### Decision 6: 一个命令执行完整门禁

统一入口：

```text
node scripts/validate-contracts.mjs
```

执行顺序：

```text
Check toolchain
  -> lint OpenAPI / Proto / JSON Schema
  -> scan forbidden fields and examples
  -> compare against origin/main when baseline exists
  -> generate into temporary directory
  -> compile Go and TypeScript
  -> compare temporary output with committed generated/
  -> print counts, versions, compatibility result, duration
```

生成命令另提供 `node scripts/generate-contracts.mjs`，它原子替换 `generated/`。校验始终生成到临时目录，不修改工作树。环境错误、契约错误和漂移使用不同错误分类，但统一返回非零状态。

### Decision 7: 生成包只承载传输模型

生成 Go package 和 TypeScript module 不包含数据库标签、ORM、业务状态机或 Provider 实现。服务在边界层显式映射生成 DTO 与内部领域对象。这样可以升级生成器而不迫使内部模型共享，也避免 Market、Platform 和 Alert 共享数据库语义。

## Data Model

```text
RequestContext
- tenantId, projectId?, environmentId?, actorId
- correlationId, traceparent?

ResourceReference
- apiVersion, kind, id, tenantId
- digest?, version?

PageRequest / PageResponse
- cursor?, limit
- nextCursor?, totalEstimate?

ProblemDetails
- type, title, status, detail?, instance?
- code, correlationId, violations[]

EventEnvelope
- identifiers and versions
- scoped context
- causation and idempotency
- typed payload or immutable payloadRef
```

ID 在公共契约中保持不透明字符串或约束 UUID；数据库类型、表名、sequence 和分区键不进入 Schema。时间统一使用 UTC RFC 3339/Protobuf Timestamp。金额、容量和 duration 必须带明确单位，禁止无单位数字。

## API and Event Contracts

首个 OpenAPI 只提供用于证明公共行为的 contract echo endpoint，不产生运行目标写操作：

```text
POST /v1/contract-echo
Headers: X-Correlation-ID, Idempotency-Key
Body: ContractEchoRequest { context, resourceRef?, value }
Response: ContractEchoResponse + ETag
Errors: RFC 9457 ProblemDetails
```

该 endpoint 是契约与生成测试夹具，不进入生产 Platform API。后续业务 change 在独立 OpenAPI 文件中复用基础 components。

事件示例使用 `hnb.contracts.v1.EventEnvelope` 和 `ContractEchoed` payload，验证未知可选字段、重复 Message ID 和跨语言编码。它不发布到 NATS，也不创建 Outbox 记录。

## State Machine

契约本身无运行时业务状态机。仓库变更状态为：

```text
Draft Schema
  -> Linted
  -> Security Checked
  -> Compatibility Checked
  -> Generated
  -> Language Compiled
  -> Drift Checked
  -> Approved

任一步失败 -> Rejected -> 修复 Schema 或显式新主版本
```

生成物不能独立进入 Approved；Schema、锁文件、生成配置和生成物必须作为同一变更评审。

## Security and Isolation

- **租户隔离:** 基础契约传播作用域 ID，但不实现授权。测试验证序列化不丢失 Tenant/Project/Environment；跨租户拒绝由后续 Identity change 实现。
- **Secret:** 禁止字段名和示例值扫描覆盖 secret、password、token、kubeconfig、private key 等；允许的 SecretReference 只包含 provider、scope、name/version 等引用。
- **权限:** 工具只需仓库读写和下载批准工具的权限；CI 校验阶段使用只读仓库权限。
- **供应链:** 版本、下载 URL、SHA-256、npm lockfile 和许可证进入 BOM；禁止浮动 latest 和运行时远程插件。
- **审计:** CI 保存工具版本、Schema 数、兼容结果、生成差异和关联提交；不记录 payload 示例中的敏感值。
- **跨平面:** 只共享生成契约，不共享数据库、内部 struct 或凭据；echo 测试不调用 RuntimeTarget，因此不存在执行旁路或数据面代理。

## Performance, Capacity, and Observability

- 冷启动工具引导可超过常规门禁预算，但单独计时并可缓存；缓存命中后的完整门禁目标低于 120 秒。
- 初始容量按 100 个 OpenAPI operations、500 个 Protobuf messages 和 200 个 JSON Schemas 设计，扫描和生成按输入规模线性增长。
- 输出各格式文件数、operation/message/schema 数、生成器版本、兼容结果、生成与编译耗时。
- 门禁失败即 CI 告警信号；本 change 不新增运行时指标、日志后端或分布式链路。
- 生成日志不得输出完整 payload、Secret 示例或下载凭据。

## Compatibility and Conformance

| 边界 | 首期验证 |
|---|---|
| OpenAPI -> Go | 生成客户端可编译，Header、Problem Details、cursor 类型一致 |
| OpenAPI -> TypeScript | 生成 Fetch SDK 可通过 TypeScript 7.0.2 严格编译 |
| Protobuf -> Go/TypeScript | 同一 Envelope 编码、解码和未知字段保留测试 |
| JSON Schema -> examples | 正反例验证、`$id` 唯一和引用可解析 |
| Schema -> internal service | 仅检查生成边界，不允许导入服务内部包 |

本 change 不修改 Provider、RuntimeTarget、Gateway 或 Edge 生命周期，相关产品兼容矩阵与运行时 Conformance 为 N/A。后续 Provider change 必须在本工具链上增加 Provider SPI Conformance。

## Failure Modes

- `[工具缺失或摘要不匹配]` -> 返回环境错误，不回退到系统全局版本。
- `[Schema 格式错误]` -> lint 失败并报告文件与字段路径，不生成部分 SDK。
- `[破坏性变化]` -> compatibility 失败；要求恢复兼容或创建新主版本。
- `[生成器输出漂移]` -> 门禁失败并提示运行生成命令，禁止手工修补生成物。
- `[生成到一半失败]` -> 临时目录被丢弃，已提交生成物不变。
- `[origin/main 不可用]` -> 首次基线显式标记 no-baseline；已有基线的 CI 不允许静默跳过兼容检查。
- `[下载源不可用]` -> 使用已校验缓存或失败；不切换到未批准镜像。
- `[敏感字段误报]` -> 通过窄化允许列表处理 SecretReference，不关闭全局安全扫描。

## Risks / Trade-offs

- `[多种标准带来工具复杂度]` -> 一个引导脚本、一个锁文件和一个门禁入口屏蔽差异，不引入自定义 IDL。
- `[提交生成物增加仓库体积]` -> 只生成批准语言与公共包，禁止示例、文档站点和冗余客户端；换取离线构建和差异可审计。
- `[不同语言使用不同生成器可能产生语义偏差]` -> 通过共享 OpenAPI、跨语言夹具和严格编译测试校验 Go client 与 TypeScript Fetch SDK 的 Header、错误和可选字段语义。
- `[最新工具版本刚发布]` -> 精确锁定并保存生成快照；升级只能通过显式依赖变更和兼容报告。
- `[JSON Schema compatibility 不如 Protobuf 严格]` -> 仓库脚本覆盖批准子集，复杂 Schema 特性需先扩展门禁再使用。
- `[工具下载影响离线环境]` -> `.tools` 可由 Release Bundle 预置且必须摘要一致；门禁本身不依赖远程插件。

## Alternatives Considered

- **只使用 OpenAPI:** 无法良好覆盖 Provider/RPC 和高频内部消息，拒绝。
- **只使用 Protobuf:** Portal、第三方 HTTP 集成和 Manifest 生态不友好，拒绝。
- **TypeSpec 统一建模:** 增加新的源语言且不能消除 Protobuf/JSON Schema 治理，推迟评估。
- **运行时共享 Go struct:** 绑定语言并破坏跨平面边界，拒绝。
- **Java/OpenAPI Generator:** 可以生成多语言客户端，但引入本项目不需要的 JDK 和较重工具链；Go 与 TypeScript 使用各自原生生成器。
- **远程 Buf Plugin:** 简化安装但不满足离线可重复要求，拒绝。
- **不提交生成物:** 减少仓库体积，但离线消费者和生成差异审计变差，首期拒绝。

## Migration Plan

1. 创建目录、工具锁、npm lockfile和引导脚本，不生成业务 API。
2. 添加基础 OpenAPI、Protobuf、JSON Schema 与正反示例。
3. 添加生成配置并生成 Go/TypeScript 基线。
4. 实现 lint、安全、兼容、编译和漂移自动测试。
5. 在 GitHub Actions 中增加契约门禁，并保存首次成功证据。
6. 后续 Identity、Provider 和 Operation change 只增量扩展契约，不复制基础类型。

回滚必须整体恢复 Schema、工具锁、配置和生成物。首版尚无下游时可完整移除；产生下游依赖后只能回滚到兼容版本，不得删除已发布 package/version。

## Upgrade, Rollback, and Disaster Recovery

- 工具升级先在独立分支重复生成并评审全部差异、许可证、漏洞和兼容报告。
- Schema 使用 additive-first 和主版本目录；消费者先升级，再启用生产者新字段。
- 工具和生成物恢复依赖版本控制与带摘要 Release Bundle；数据库备份、RPO/RTO 和运行时灾备不适用。
- 若新生成器产生不可接受漂移，恢复上一锁文件和生成物，不修改已发布 Schema 语义掩盖差异。

## Open Questions

- GitHub 分支规则中契约门禁的最终作业名称和生成物差异展示保留期是什么？
- 首次 Release Bundle 是否直接包含 `.tools` 的 Linux x64/arm64 二进制，还是由离线构建流程按锁文件组装？
