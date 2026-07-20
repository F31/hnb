## 1. Toolchain and Repository Layout

- [x] 1.1 `[CONTRACT-001][CONTRACT-006]` 创建 `contracts/openapi`、`contracts/proto`、`contracts/schema` 和 `contracts/generated` 目录及所有权说明；证据：目录检查和边界评审记录。
- [x] 1.2 `[CONTRACT-006]` 固定 Node.js、npm、TypeScript、Go、Buf、oapi-codegen、openapi-typescript-codegen、protoc 插件、Redocly、oasdiff 和 AJV 版本及摘要；证据：`toolchain.lock.json` 与 Core BOM 评审。
- [x] 1.3 `[CONTRACT-006]` 实现仓库本地 `.tools/contracts` 引导和版本校验，不依赖系统 Go、全局 npm 包、Java 或远程生成插件；证据：空缓存和缓存命中测试。
- [x] 1.4 `[CONTRACT-006]` 增加精确版本的 Node 开发依赖、lockfile 和 `.tools` 忽略规则；证据：锁文件审计及重复安装结果。

## 2. Foundation Schemas

- [x] 2.1 `[CONTRACT-001][CONTRACT-003]` 定义 OpenAPI 3.1 基础 components、Correlation/Idempotency/If-Match Header、cursor 分页和 RFC 9457 Problem Details；证据：OpenAPI lint 和正反例测试。
- [x] 2.2 `[CONTRACT-001][CONTRACT-003]` 定义 Protobuf v1 RequestContext、ResourceReference、EventEnvelope 与 ContractEchoed 测试 payload；证据：Buf lint 和 descriptor 检查。
- [x] 2.3 `[CONTRACT-001][CONTRACT-003]` 定义 JSON Schema Draft 2020-12 基础上下文、资源引用、SecretReference、Envelope 及 `$id` 规则；证据：Schema 与示例验证报告。
- [x] 2.4 `[CONTRACT-003]` 建立跨格式语义矩阵，确认 Tenant/Project/Environment、Actor、Correlation、Causation、Idempotency 和版本字段名称与可选性；证据：矩阵评审及自动一致性测试。
- [x] 2.5 `[CONTRACT-001]` 添加不产生 RuntimeTarget 写操作的 contract echo 测试 API 和事件样例；证据：Schema lint 与执行旁路 N/A 评审。

## 3. Deterministic SDK Generation

- [x] 3.1 `[CONTRACT-001][CONTRACT-006]` 配置 oapi-codegen 生成 Go OpenAPI 类型与客户端，并通过 Go 1.26.5 编译；证据：生成命令和 `go test` 输出。
- [x] 3.2 `[CONTRACT-001][CONTRACT-006]` 配置 openapi-typescript-codegen 生成 TypeScript Fetch SDK，并通过 TypeScript 7.0.2 严格编译；证据：生成命令和 `tsc --noEmit` 输出。
- [x] 3.3 `[CONTRACT-001][CONTRACT-006]` 配置 Buf、protoc-gen-go 和 protoc-gen-es 生成 Go/TypeScript 消息类型；证据：Buf generation、Go 和 TypeScript 编译结果。
- [x] 3.4 `[CONTRACT-006]` 实现 `scripts/generate-contracts.mjs`，在临时目录完成后原子更新生成物且记录工具版本；证据：失败不覆盖和成功生成测试。
- [x] 3.5 `[CONTRACT-006]` 验证相同 Schema 与锁文件连续生成两次无差异；证据：生成摘要和零漂移报告。

## 4. Contract Quality Gate

- [x] 4.1 `[CONTRACT-001][CONTRACT-006]` 实现 `scripts/validate-contracts.mjs`，统一执行工具检查、lint、生成、编译和漂移检查；证据：有效基线成功日志。
- [x] 4.2 `[CONTRACT-002][CONTRACT-006]` 实现 OpenAPI oasdiff、Protobuf Buf breaking 和 JSON Schema 兼容检查，并处理首次无基线场景；证据：兼容与破坏性夹具报告。
- [x] 4.3 `[CONTRACT-003][CONTRACT-006]` 检查所有写 operation 的 Idempotency-Key、Correlation ID 及更新/删除的 If-Match 约定；证据：缺失 Header 的失败夹具。
- [x] 4.4 `[CONTRACT-001][CONTRACT-006]` 实现 Secret、Token、kubeconfig、私钥和大文件正文字段安全门禁，并允许受限 SecretReference/PayloadReference；证据：敏感字段正反例测试。
- [x] 4.5 `[CONTRACT-006]` 输出 Schema/operation/message 数、工具版本、兼容结果和分阶段耗时，并稳定区分环境、Schema、兼容和漂移错误；证据：输出快照和退出路径测试。

## 5. Automated Verification

- [x] 5.1 `[CONTRACT-001][CONTRACT-003]` 创建 Go/TypeScript 跨语言夹具，验证 Envelope、UUID、时间、未知可选字段和同一幂等/关联标识；证据：互操作测试报告。
- [x] 5.2 `[CONTRACT-002][CONTRACT-006]` 覆盖字段删除、类型变化、required 增加、Protobuf 字段号复用和主版本升级夹具；证据：兼容测试矩阵。
- [x] 5.3 `[CONTRACT-004]` 定义 Outbox 事件记录与 Envelope 的映射契约测试，但不实现数据库或 Relay；证据：映射测试及数据库迁移 N/A 评审。
- [x] 5.4 `[CONTRACT-006]` 测量缓存命中后的完整门禁并确认低于 120 秒，记录 CPU、内存、工具和输入规模；证据：绑定环境的性能报告。
- [x] 5.5 `[CONTRACT-001][CONTRACT-006]` 扫描生成物依赖方向，确认不导入服务内部包、数据库模型或具体 Broker 类型；证据：依赖边界测试。

## 6. Validation Policy, Documentation, and Applicability

- [x] 6.1 `[CONTRACT-006]` 记录 GitHub 仅用于代码托管、不承担 CI，并固定提交前统一契约门禁、工具版本和本地证据要求；证据：开发指南、工具锁和完整门禁成功日志。
- [x] 6.2 `[CONTRACT-001][CONTRACT-002][CONTRACT-006]` 编写新增契约、生成 SDK、兼容演进、弃用和主版本升级指南；证据：文档路径和示例变更演练。
- [x] 6.3 `[CONTRACT-001][CONTRACT-006]` 记录数据库迁移、运行时 E2E、Provider/RuntimeTarget/Gateway/Edge Conformance、备份和灾备为 N/A，因为本 change 只交付静态契约基础；证据：适用性评审记录。
- [x] 6.4 `[CONTRACT-006]` 记录工具许可证、下载摘要、漏洞扫描和离线 Release Bundle 组装方式；证据：供应链报告与离线门禁演练。

## 7. Rollback and Archive Gate

- [x] 7.1 `[CONTRACT-002][CONTRACT-006]` 演练整体回滚 Schema、工具锁、配置和生成物，并确认统一门禁恢复通过；证据：回滚与恢复日志。
- [x] 7.2 `[CONTRACT-001][CONTRACT-006]` 运行 `openspec validate bootstrap-contracts-events --strict --no-interactive`、`openspec validate --all --strict --no-interactive` 和统一 OpenSpec/Contracts 门禁；证据：全部零退出码。
- [x] 7.3 `[CONTRACT-001][CONTRACT-002][CONTRACT-003][CONTRACT-004][CONTRACT-006]` 汇总 Requirement 到 Schema、生成物、单元测试、兼容测试、本地门禁、文档和回滚证据，完成 verify 后归档；证据：追踪矩阵和归档评审记录。
