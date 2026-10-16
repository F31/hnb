## MODIFIED Requirements

### Requirement: [PSHAPE-001] 形态能力向量
HNB SHALL 将每个部署形态表达为能力向量 `ProfileSpec`，包含 `capabilities`（功能开关集合）、`components`（组件到副本数）、`ha`（独立故障域数与 quorum）、`backings`（bundled|in-cluster-ha|external）、`storage`（每组件 PVC 尺寸与 StorageClass）、`resources`（每组件 requests）；该向量 SHALL 由 `deploy/charts/hnb/overlays` 的形态 overlay 单一事实源派生，不得另行维护重复定义；HA 档（Lite/Standard/Pro）的 `backings` SHALL 默认为 `in-cluster-ha`，Minimal SHALL 为 `bundled`。

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

#### Scenario: HA 档默认背靠为 in-cluster
- **GIVEN** 未显式指定背靠的 Lite/Standard/Pro 形态
- **WHEN** 构造该形态的 `ProfileSpec`
- **THEN** `backings` 为 `in-cluster-ha`
- **AND** Minimal 的 `backings` 为 `bundled`

## ADDED Requirements

### Requirement: [PSHAPE-004] 背靠模式的偏序与超集判定
HNB SHALL 在形态超集判定中纳入背靠模式：偏序为 `bundled < in-cluster-ha ≈ external`，`in-cluster-ha` 与 `external` 互不为降级，任一向 `bundled` 的迁移 SHALL 判定为降级并拒绝。

**Traceability:** GOV-002, DPLY-001; profile-upgrade

#### Scenario: 背靠升级被接受
- **GIVEN** 源形态背靠为 `bundled`，目标形态背靠为 `in-cluster-ha`
- **WHEN** 判定目标是否为源的超集
- **THEN** 背靠维度判定为升级
- **AND** 不产生背靠相关原因码

#### Scenario: 背靠回退被拒绝
- **GIVEN** 源形态背靠为 `external` 或 `in-cluster-ha`，目标形态背靠为 `bundled`
- **WHEN** 判定目标是否为源的超集
- **THEN** 判定失败并返回 `PROFILE_NOT_SUPERSET` 类原因码
