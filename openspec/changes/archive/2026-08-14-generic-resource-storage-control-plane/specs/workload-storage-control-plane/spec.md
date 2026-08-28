## ADDED Requirements

### Requirement: [STO-001] 分离的工作负载存储领域模型
平台 SHALL 分别建模驱动包、驱动安装、存储后端、工作负载存储服务和 StorageClass Binding，SHALL NOT 使用单一类型字段加任意 JSON 替代其独立身份、作用域和生命周期。

**Traceability:** HNB-STO-01, RT-003, PROV-005

#### Scenario: 同一驱动连接多个后端
- **GIVEN** 一个 CSI 驱动安装已就绪
- **WHEN** 平台接入两个独立存储后端
- **THEN** 平台保留一个驱动安装和两个独立 StorageBackend
- **AND** 每个后端具有独立健康、SecretReference 和服务列表

### Requirement: [STO-002] 存储服务与集群映射
WorkloadStorageOffering SHALL 使用结构化公共能力描述业务服务，并通过 StorageClassBinding 显式映射到特定 RuntimeTarget 的 StorageClass UID/resourceVersion；平台 SHALL 支持一个 Offering 绑定多个集群。

**Traceability:** HNB-STO-02, RT-004

#### Scenario: 高性能服务映射到两个集群
- **GIVEN** 租户有一个高性能块存储 Offering
- **WHEN** 管理员导入两个集群的不同 StorageClass
- **THEN** 平台创建两个独立 Binding 并保留各自目标、UID、拓扑和同步状态
- **AND** 应用管理员仅看到被授权集群的可用服务

### Requirement: [STO-003] 真实容量与未知状态
容量、性能和健康数据 SHALL 包含来源、单位、observedAt 与新鲜度；来源无法提供值时平台 SHALL 使用 Elastic、Unknown 或 NotReported，SHALL NOT 显示为零或推断为健康。

**Traceability:** HNB-STO-03, OBS-001

#### Scenario: 云盘没有固定总容量
- **GIVEN** 云盘 Provider 只上报配额而不提供固定池容量
- **WHEN** Portal 展示容量总览
- **THEN** 总容量显示为 Elastic 或 NotReported
- **AND** 不把配额或零值冒充为后端总容量

### Requirement: [STO-004] 受治理的存储写操作
所有驱动安装、升级、卸载、Offering 绑定、StorageClass 变更、PVC 扩容和数据回收 SHALL 生成不可变 ExecutionPlan 与 Operation，并执行细粒度授权、预检、幂等、fencing、审计和观察后确认；Portal SHALL NOT 通过任意 Kubernetes proxy 报告写入成功。

**Traceability:** P1-WRITE-001, P1-WRITE-002, OP-007

#### Scenario: 扩容 PVC
- **GIVEN** Offering、驱动和 StorageClass 均声明支持扩容
- **WHEN** 授权用户提交 PVC 扩容意图
- **THEN** planner 固定目标、资源 UID/resourceVersion、Provider 和幂等键后创建 Operation
- **AND** 最终容量仅由后续观察事实确认

### Requirement: [STO-005] 禁止不安全通用 PV 回收
平台 SHALL NOT 把删除 PV claimRef 视为数据擦除或通用回收；Retain 卷的重新分配 SHALL 使用 Provider 专属清理证据或明确的手动释放工作流。

**Traceability:** HNB-STO-05, SEC-005

#### Scenario: Retain 卷仍含旧数据
- **GIVEN** 一个 Released PV 的后端未提供清理证据
- **WHEN** 用户尝试回收该卷
- **THEN** 平台拒绝自动重新分配并显示数据仍保留
- **AND** 不删除 claimRef 或将卷标记为可安全复用

### Requirement: [STO-006] 制品与对象存储边界
ArtifactStorageProfile SHALL 继续由 App Market 管理；普通对象桶服务 SHALL NOT 被表示为 StorageClass/PV/PVC，除非安装并声明了独立、通过 Conformance 的桶服务契约。

**Traceability:** ART-STO-10, HNB-STO-06

#### Scenario: 接入 MinIO 桶服务
- **GIVEN** 管理员接入一个 MinIO API
- **WHEN** Provider 未声明 Kubernetes 卷能力
- **THEN** 平台不创建 StorageClassBinding
- **AND** 桶服务通过独立 Connector/API 暴露
