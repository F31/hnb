## ADDED Requirements

### Requirement: [PUPG-005] 背靠迁移（再背靠）受控且可审计
HNB SHALL 将背靠迁移（`bundled ↔ in-cluster-ha ↔ external`）表达为受控、可审计的操作：仅当发布锁声明允许（`backingChange: true`）时规划，执行时建立平台 Operation 行并广播领域事件，且迁移过程 SHALL 保留原数据面直到目标验证通过；未被声明或被判定为降级的迁移 SHALL 被拒绝。

**Traceability:** DPLY-003, PINST-006; in-cluster-ha-backings

#### Scenario: 未声明迁移被拒绝
- **GIVEN** 发布锁未声明该 from→to 背靠迁移
- **WHEN** 规划再背靠
- **THEN** 拒绝并返回稳定原因码
- **AND** 不产生任何数据面变更

#### Scenario: 迁移建立可审计记录
- **GIVEN** 发布锁声明允许的背靠迁移
- **WHEN** 执行该迁移
- **THEN** 建立平台 Operation 行并记录 from/to
- **AND** 在目标验证通过前保留原数据面

#### Scenario: 迁移可在验证前回退
- **GIVEN** 一个处于双运行/验证阶段的背靠迁移
- **WHEN** 验证未通过且运维中止
- **THEN** 系统回到原背靠
- **AND** 不删除原数据
