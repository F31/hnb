# profile-upgrade

## Purpose
定义跨形态与跨版本的统一升级契约：无副作用的规划、门禁化且可恢复的执行、以重放上一把锁实现的回滚，以及显式有界的降级。

## Requirements

### Requirement: [PUPG-001] 升级规划无副作用
HNB SHALL 提供 `hnbctl upgrade --to <profile|version>`，默认以 dry-run 运行；规划 SHALL 只读取当前观测状态与目标形态能力向量，输出 `profile-upgrade-plan/v1`，且 SHALL NOT 修改集群、数据库或本地运行状态。

**Traceability:** DPLY-004, DPLY-005; profile-shape

#### Scenario: dry-run 只产出计划
- **GIVEN** 一个有效的当前配置与目标形态
- **WHEN** 操作者执行 `hnbctl upgrade --to Standard` 且未指定 `--execute`
- **THEN** 命令返回计划（能力增启、副本、存储、迁移、每步可逆性与前置）
- **AND** 退出码为计划成功且未产生任何集群副作用

#### Scenario: 计划要求目标已认证
- **GIVEN** 目标形态或升级路径在能力证据矩阵中未认证
- **WHEN** 操作者规划该升级
- **THEN** 计划标注 `TARGET_PROFILE_NOT_CERTIFIED` 或 `UPGRADE_PATH_NOT_CERTIFIED`
- **AND** 不标记为可执行

### Requirement: [PUPG-002] 升级执行门禁化且可恢复
HNB SHALL 仅在显式 `--execute` 且证据门禁通过时执行升级；执行 SHALL 复用运行存储，记录阶段 `preflight → backup → infra-delta → database-migrate → platform-release → verify`，SHALL 幂等、可恢复，并在任一门禁失败时 SHALL NOT 记录成功。

**Traceability:** DPLY-004, PINST-005; profile-upgrade

#### Scenario: 备份前置强制
- **GIVEN** 一次需要数据库迁移的升级且备份前置未满足
- **WHEN** 执行升级
- **THEN** 执行在 `backup` 阶段失败关闭
- **AND** 不进入 `database-migrate`，运行状态不为 succeeded

#### Scenario: 恢复不重跑一次性阶段
- **GIVEN** 一次中断的升级运行
- **WHEN** 恢复执行
- **THEN** 一次性阶段（如 init/join/cluster-verified/database-migrate）不被重跑
- **AND** 改为重新校验后跳过或拒绝

#### Scenario: 失败不记为成功
- **GIVEN** 升级执行到达 `verify` 阶段
- **WHEN** 分层就绪门未通过
- **THEN** 运行终结为 blocked/failed 并返回非成功退出码

### Requirement: [PUPG-003] 回滚重放上一锁
HNB SHALL 以重放上一 `release-lock` 作为升级回滚；可逆迁移 SHALL 随锁回滚，不可逆迁移 SHALL 在执行前标记并要求显式确认，未确认时 SHALL 拒绝升级；HNB SHALL NOT 用 Helm rollback 代替数据库回退。
**Traceability:** DPLY-005; profile-upgrade

#### Scenario: 不可逆迁移需确认
- **GIVEN** 目标升级包含标注为不可逆的迁移
- **WHEN** 操作者未显式确认即请求升级
- **THEN** 升级在规划或执行前置处被拒绝
- **AND** 不执行迁移

#### Scenario: 回滚到上一锁
- **GIVEN** 一次已完成的升级
- **WHEN** 触发回滚
- **THEN** 系统重放上一锁并仅对可逆迁移执行回退
- **AND** 记录回滚阶段与结果

### Requirement: [PUPG-004] 降级显式且有界
HNB SHALL 默认拒绝高形态→低形态的降级；仅在操作者显式声明且可逆性得到证明时 SHALL 允许，并 SHALL 记录降级为受控例外而非普通升级。

**Traceability:** GOV-002; profile-shape

#### Scenario: 静默降级被拒绝
- **GIVEN** 当前形态为 Standard
- **WHEN** 操作者请求 `--to Minimal` 且未声明降级
- **THEN** 系统拒绝并给出原因码 `DOWNGRADE_NOT_ACKNOWLEDGED`

#### Scenario: 已确认的可逆降级
- **GIVEN** 当前形态为 Standard 且所有受影响迁移可逆
- **WHEN** 操作者显式声明降级
- **THEN** 系统产出降级计划
- **AND** 在 `--execute` 下受控执行
