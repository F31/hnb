# profile-shape

## Purpose
定义 HNB 部署形态的能力向量、单调偏序与发布列车锁，使 Minimal/Lite/Standard/Pro 可机器比较、可校验、可规划升级，并以 Chart overlay 为单一事实源。

## Requirements

### Requirement: [PSHAPE-001] 形态能力向量
HNB SHALL 将每个部署形态表达为能力向量 `ProfileSpec`，包含 `capabilities`（功能开关集合）、`components`（组件到副本数）、`ha`（独立故障域数与 quorum）、`backings`（bundled|external）、`storage`（每组件 PVC 尺寸与 StorageClass）、`resources`（每组件 requests）；该向量 SHALL 由 `deploy/charts/hnb/overlays` 的形态 overlay 单一事实源派生，不得另行维护重复定义。

**Traceability:** DPLY-001, PINST-002; profile-shape

#### Scenario: 由 overlay 派生向量
- **GIVEN** 一组形态 overlay 文件作为单一事实源
- **WHEN** 为形态 Minimal 构造 `ProfileSpec`
- **THEN** 其组件集合与副本数与 `minimal.yaml` 一致
- **AND** 携带 T0/T1 组件（含 alert-manager、cluster-agent、network-registry、cilium-provider）

#### Scenario: 未知组件被拒绝
- **GIVEN** 目标形态向量包含 overlay 未定义的组件标识
- **WHEN** 校验该形态
- **THEN** 校验失败并返回稳定原因码
- **AND** 不产生形态

### Requirement: [PSHAPE-002] 形态单调偏序
HNB SHALL 定义形态偏序 `Minimal ⊂ Lite ⊂ Standard ⊂ Pro`，任一形态的能力集、组件副本、故障域数与存储尺寸 SHALL 单调不减；系统 SHALL 拒绝任何违反该偏序的形态定义或升级目标。

**Traceability:** GOV-002; profile-upgrade

#### Scenario: 超集被接受
- **GIVEN** 目标形态的能力集是当前形态的超集
- **WHEN** 判定该形态迁移
- **THEN** 迁移被判定为合法升级

#### Scenario: 回退目标被拒绝
- **GIVEN** 目标形态移除了当前形态已启用的能力或减少了故障域数
- **WHEN** 判定该形态迁移
- **THEN** 系统拒绝并给出 `PROFILE_NOT_SUPERSET` 类原因码

### Requirement: [PSHAPE-003] 发布列车锁
HNB SHALL 支持发布列车锁 `release-lock/v2`：一份锁覆盖该版本全部形态与能力包、镜像 manifest、`profiles`（每形态 requiredCapabilities 与最小 Kubernetes 版本）、`supportedUpgrades`（允许的 from→to）与有序 `migrationChain`（owner + from/to schema + checksum）；读取器 SHALL 继续接受 `release-lock/v1`。

**Traceability:** DPLY-003; profile-upgrade

#### Scenario: v1 锁仍可读
- **GIVEN** 一个不含发布列车字段的 `release-lock/v1`
- **WHEN** 读取并校验该锁
- **THEN** 解析成功且仅启用非升级路径
- **AND** 不因缺少 v2 字段失败

#### Scenario: 升级路径按锁校验
- **GIVEN** 一个声明了 `supportedUpgrades` 与 `migrationChain` 的锁
- **WHEN** 请求升级 from→to
- **THEN** 系统校验该路径已声明且迁移链连续、checksum 真实
- **AND** 未声明或链断裂时拒绝

#### Scenario: 迁移链必须连续
- **GIVEN** `migrationChain` 缺少目标 schema 的中间步骤或 checksum 与制品不符
- **WHEN** 规划升级
- **THEN** 升级规划拒绝执行并返回稳定原因码
