## ADDED Requirements

### Requirement: [RT-011] 结构化存储清单观测
KubernetesTarget observer SHALL 在版本化 RuntimeTargetObservation 中上报 StorageClass、CSIDriver、CSINode、CSIStorageCapacity、VolumeAttachment 以及可选快照 API 清单；每项 SHALL 包含稳定 Kubernetes UID、resourceVersion、规范化字段、observedAt 和来源，且 SHALL 遵守 RT-008 的租户绑定、Full/Delta、generation、sequence 与 fencing 规则。

**Traceability:** RT-003, RT-008, STO-001, STO-003

#### Scenario: 提交完整 CSI 清单
- **GIVEN** tenant-bound Agent 已授权观察一个 KubernetesTarget
- **WHEN** Agent 提交包含 storageInventory 的连续 Full 观测
- **THEN** 投影器在同一有序事务边界更新存储清单与 observer cursor
- **AND** 缺失资源产生 tombstone 而非删除审计历史

#### Scenario: Snapshot API 未安装
- **GIVEN** 集群没有 VolumeSnapshot CRD
- **WHEN** observer 执行能力发现
- **THEN** storageInventory 将 snapshot API 标记为 Unsupported 或 NotInstalled
- **AND** 不把空列表解释为已安装且健康

### Requirement: [RT-012] 存储驱动健康证据
平台 SHALL 分别保存 package/installation 声明、CSIDriver 注册、controller/node 工作负载健康和 StorageClass 引用证据；任何单一证据 SHALL NOT 独立证明驱动 Ready。

**Traceability:** RT-003, PROV-004, STO-001

#### Scenario: StorageClass 引用缺失驱动
- **GIVEN** 集群存在 provisioner 为 `example.csi.io` 的 StorageClass
- **WHEN** observer 未发现对应注册和健康工作负载
- **THEN** 平台保留 StorageClass 清单并标记 missing-driver condition
- **AND** 不将驱动安装显示为 Ready
