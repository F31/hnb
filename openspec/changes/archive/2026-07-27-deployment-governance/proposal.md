## Why

平台已有 26 个能力规格，但缺乏自动化的质量门禁来验证 Requirement ID 唯一性、Traceability 完整性和 Scenario 格式。需要实现 GOV-001~008 治理要求，确保 OpenSpec 变更质量和部署档位约束。

## What Changes

- **自动化质量门禁 (GOV-007)**：`scripts/validate-specs.sh` 脚本，验证 Requirement ID 唯一性、Traceability 和 Scenario 格式
- **能力分级文档**：各 CapabilityPack 的 T0/T1/T2/T3 分级声明
- **部署档位 BOM 定义**：Minimal / Lite HA / Standard HA / Enterprise 档位
- **需求双向追踪加强**：openspec 变更中 Requirement ID 引用规范

## Capabilities

### New Capabilities
- `deployment-governance`: 能力分级、部署档位、版本化 BOM、阶段门禁、需求追踪、DoD、质量门禁

## Impact

- **新脚本**：`scripts/validate-specs.sh` — OpenSpec 质量门禁
- **文档更新**：阶段门槛和退出判据文档
- **T1 分级**：部署治理为 T1 默认交付能力