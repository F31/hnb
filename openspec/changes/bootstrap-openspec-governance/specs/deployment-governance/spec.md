## ADDED Requirements

### Requirement: [GOV-007] 自动化 OpenSpec 质量门禁
仓库 SHALL 提供可由本地开发环境和持续集成一致调用的 OpenSpec 质量门禁；门禁 SHALL 严格校验全部主规格和活动 change，并 SHALL 拒绝重复 Requirement ID、缺少 Traceability 或缺少 GIVEN/WHEN/THEN Scenario 的 Requirement。

**Traceability:** GOV-005, GOV-006, METH-04

#### Scenario: 合规规格通过质量门禁
- **GIVEN** 全部主规格和活动 change 均符合 OpenSpec 格式、Requirement ID 唯一且具有 Traceability 和完整 Scenario
- **WHEN** 本地开发者或持续集成执行统一质量门禁
- **THEN** 门禁返回成功状态
- **AND** 输出被检查的 spec、Requirement、Scenario 和 Traceability 数量

#### Scenario: 不合规规格阻止合并
- **GIVEN** 一个 Requirement 使用重复 ID、缺少 Traceability 或缺少 GIVEN/WHEN/THEN 任一关键字
- **WHEN** 持续集成执行统一质量门禁
- **THEN** 门禁返回非零状态并阻止合并
- **AND** 输出包含违规类型和对应文件位置
