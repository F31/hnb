## ADDED Requirements

### Requirement: [GOV-007] 自动化 OpenSpec 质量门禁
仓库 SHALL 提供可由本地开发环境和独立验证环境一致调用的 OpenSpec 质量门禁；门禁 SHALL 严格校验全部主规格和活动 change，并 SHALL 拒绝重复 Requirement ID、缺少 Traceability 或缺少 GIVEN/WHEN/THEN Scenario 的 Requirement。当前 GitHub 仓库 SHALL 仅用于代码托管，不承担持续集成。

**Traceability:** GOV-005, GOV-006, METH-04

#### Scenario: 合规规格通过质量门禁
- **GIVEN** 全部主规格和活动 change 均符合 OpenSpec 格式、Requirement ID 唯一且具有 Traceability 和完整 Scenario
- **WHEN** 开发者在提交或评审前执行统一质量门禁
- **THEN** 门禁返回成功状态
- **AND** 输出被检查的 spec、Requirement、Scenario 和 Traceability 数量

#### Scenario: 不合规规格阻止提交评审
- **GIVEN** 一个 Requirement 使用重复 ID、缺少 Traceability 或缺少 GIVEN/WHEN/THEN 任一关键字
- **WHEN** 开发者或评审者执行统一质量门禁
- **THEN** 门禁返回非零状态并阻止提交进入评审
- **AND** 输出包含违规类型和对应文件位置
