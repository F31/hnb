## Context

当前 OpenSpec 治理通过人工 review 保证质量，缺乏自动化校验。GOV-007 要求提供可一致调用的质量门禁，GOV-001~006 需要文档化治理框架。

## Goals / Non-Goals

**Goals:**
- 自动化质量门禁脚本验证 Requirement ID 唯一性、Traceability 和 Scenario 格式
- 能力分级声明模板
- 部署档位 BOM 定义文档

**Non-Goals:**
- 修改现有代码行为
- 引入新服务或中间件

## Decisions

### Decision 1: 质量门禁使用 Bash 脚本实现
无需 Go 编译，可直接在 CI 和本地运行。使用 grep/awk 解析 spec.md 文件。

### Decision 2: 能力分级在 proposal 中声明
每个 change 的 proposal 文档必须声明 T0/T1/T2/T3 分级，门禁检查 proposal 中是否包含分级声明。