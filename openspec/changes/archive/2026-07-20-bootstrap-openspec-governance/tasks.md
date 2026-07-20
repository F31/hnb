## 1. Validation Entry Point

- [x] 1.1 `[GOV-007]` 记录并校验 Node.js 与 `@fission-ai/openspec` 批准版本，定义退出码 0/1/2；证据：版本检查和退出码测试输出。
- [x] 1.2 `[GOV-005][GOV-007]` 创建 `scripts/validate-openspec.mjs`，调用 `openspec validate --all --strict --no-interactive` 并透传原生错误；证据：当前基线执行日志。
- [x] 1.3 `[GOV-007]` 实现主规格与活动 delta spec 文件发现，排除 `openspec/changes/archive/` 并忽略 fenced code block；证据：文件发现单元测试。

## 2. Semantic Governance Checks

- [x] 2.1 `[GOV-005][GOV-007]` 解析 Requirement ID、标题、文件和行号，拒绝缺失或重复 ID，并允许 MODIFIED Requirement 与对应主规格复用 ID；证据：ID 合法、缺失、冲突和 MODIFIED 测试。
- [x] 2.2 `[GOV-005][GOV-007]` 检查每个新增或修改 Requirement 的 Traceability，报告违规文件和行号；证据：完整与缺失 Traceability 测试。
- [x] 2.3 `[GOV-006][GOV-007]` 检查每个 Requirement 至少一个 Scenario 且覆盖 GIVEN/WHEN/THEN，汇总 spec、Requirement、Scenario 和 Traceability 数量；证据：关键字缺失测试和成功摘要快照。
- [x] 2.4 `[GOV-007]` 对环境错误、OpenSpec 格式错误和语义错误实现稳定的退出码与标准输出/错误输出；证据：三类失败路径集成测试。

## 3. Automated Verification

- [x] 3.1 `[GOV-007]` 使用 Node.js 内置测试能力创建临时 OpenSpec 工作区，覆盖有效规格、重复 ID、缺失 Traceability、缺失 Scenario 关键字和代码块误识别；证据：自动测试报告。
- [x] 3.2 `[GOV-005][GOV-006][GOV-007]` 在当前 18 个主规格及本活动 change 上运行统一门禁，确认 delta 与主规格 ID 处理正确；证据：零退出码和检查计数。
- [x] 3.3 `[GOV-007]` 测量统一门禁执行时间并确认常规环境低于 60 秒预算；证据：带 Node.js、OpenSpec 和环境信息的计时记录。

## 4. Validation Policy and Documentation

- [x] 4.1 `[GOV-006][GOV-007]` 记录 GitHub 仅用于代码托管、不承担 CI，并固定提交前统一校验命令与批准的 `@fission-ai/openspec` 版本；证据：治理指南、版本检查和本地成功日志。
- [x] 4.2 `[GOV-005][GOV-007]` 编写本地运行、失败排查、Requirement ID、Traceability 和证据引用约定；证据：文档路径及命令复现记录。
- [x] 4.3 `[GOV-006]` 记录适用性评估：网络 API、事件、数据库迁移、运行时 E2E、Provider Conformance、备份和灾备均为 N/A，因为本 change 仅增加仓库静态治理；证据：评审记录。

## 5. Rollback and Archive Gate

- [x] 5.1 `[GOV-006][GOV-007]` 移除废弃的 GitHub Actions 调用并确认主规格读取、统一门禁和自动测试仍正常；证据：工作流删除记录和本地成功日志。
- [x] 5.2 `[GOV-005][GOV-006][GOV-007]` 运行 `openspec validate bootstrap-openspec-governance --strict --no-interactive` 和 `openspec validate --all --strict --no-interactive`；证据：两条命令的成功输出。
- [x] 5.3 `[GOV-005][GOV-006]` 汇总 Requirement 到实现、单元测试、集成测试、本地门禁、文档和回滚证据的追踪表，确认无孤立任务后再归档；证据：追踪表和归档评审记录。
