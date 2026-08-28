## MODIFIED Requirements

### Requirement: [RT-001] 运行目标分类
平台 SHALL 以 KubernetesTarget、ContainerEngineTarget 和 EdgeRuntimeTarget 作为三类容器执行目标；ExternalServiceConnector SHALL 仅表示外部服务绑定，不得执行容器部署。

集群 Read Model、集群列表、集群详情和集群节点接口 SHALL 仅接受 `targetKind` 为 `KubernetesTarget` 或 `EdgeRuntimeTarget` 的目标。`ContainerEngineTarget` SHALL 作为独立运行时目标展示和管理，SHALL NOT 被投影为集群，也 SHALL NOT 复用 Kubernetes 集群节点模型。服务端 SHALL 在数据库查询和计数边界执行该分类过滤，而不是依赖客户端隐藏不符合条件的目标。

**Traceability:** EDGE-01, GOV-05, UX-021

#### Scenario: 绑定外部数据库
- **GIVEN** 用户选择一个云数据库 Connector
- **WHEN** 应用创建绑定
- **THEN** 平台只生成连接绑定而不尝试部署数据库容器
- **AND** 对象类型在 Read Model 中保持可区分

#### Scenario: 查询集群列表
- **GIVEN** 同一租户拥有 KubernetesTarget、EdgeRuntimeTarget 和 ContainerEngineTarget
- **WHEN** 用户查询集群 Read Model
- **THEN** 结果和精确总数仅包含 KubernetesTarget 与 EdgeRuntimeTarget
- **AND** ContainerEngineTarget 仅可从独立运行时目标接口查询

### Requirement: [RT-003] 能力发现与快照
RuntimeTarget SHALL 上报 Kubernetes/运行时版本、架构、资源、CNI/CSI、Gateway API、GPU/NPU、安全和存储能力；平台 SHALL 保存带时间戳的 CapabilitySnapshot。

能力及资源清单观测 SHALL 使用 RT-008 定义的版本化 Agent/CloudCore 观测契约。每个 CapabilitySnapshot SHALL 绑定 `tenantId`、`targetId`、`targetKind`、来源 observer、来源序列、`observedAt`、规范化能力内容和内容摘要；投影器 SHALL 对不同内容追加不可变快照，对幂等重放不得创建重复快照。兼容性检查 SHALL 使用被 ExecutionPlan 固定的快照标识，不得在执行期间静默切换到更新快照。

**Traceability:** GW-04, GW-13, RT-004, RT-008, PROV-005

#### Scenario: 能力发生变化
- **GIVEN** 目标集群升级 Gateway Controller
- **WHEN** 下一次能力探测完成
- **THEN** 平台保存新的能力快照并触发兼容性重算
- **AND** 旧快照保留用于审计

#### Scenario: 重放相同能力观测
- **GIVEN** 已投影某 observer 序列的 CapabilitySnapshot
- **WHEN** 投影器再次收到相同 `eventId` 或相同 observer 序列
- **THEN** 平台返回幂等成功且不追加重复快照
- **AND** 当前目标能力和节点投影不发生倒退

### Requirement: [RT-005] 目标状态新鲜度
RuntimeTarget 和边缘资源状态 SHALL 包含 observedAt/lastKnownStateAt；超过新鲜度阈值时，写操作 SHALL 排队、拒绝或要求显式风险确认。

RuntimeTarget 与节点 Read Model SHALL 将 `lifecycleStatus`、`healthStatus`、`connectivityStatus` 和 `freshnessStatus` 建模为四个独立维度。`freshnessStatus` 至少包含 `FRESH`、`STALE` 和 `UNKNOWN`，并 SHALL 由服务端使用当前时间、最新已接受观测的 `observedAt` 及租户/目标策略阈值计算；`STALE` SHALL NOT 覆盖或改写最后已知的生命周期、健康或连接状态。`lastKnownStateAt` SHALL 表示最近一次已接受且包含已知来源状态的观测时间，在无新观测、断连或进入 `STALE` 时保持不变；服务端计算时间 SHALL NOT 被写入为 `lastKnownStateAt`。

所有目标写入在计划提交前 SHALL 由服务端重新计算新鲜度并执行 STALE 策略，客户端传入的状态或确认布尔值不得替代该决策。策略结果 SHALL 仅为：要求显式风险确认后允许、进入 `PendingApproval`/`QueuedOffline` 队列，或拒绝。风险确认 SHALL 绑定 tenant、actor、targetId、动作、被确认的 `observedAt`/投影版本和有效期；状态版本变化、过期、跨租户或跨动作重放 SHALL 重新决策。每次决策及确认 SHALL 写入审计，排队或拒绝不得被报告为实时执行成功。

**Traceability:** EDGE-14, EDGE-18, GOV-003, P1-WRITE-002

#### Scenario: 对陈旧边缘状态执行升级
- **GIVEN** 节点已离线超过策略阈值
- **WHEN** 用户提交升级
- **THEN** 平台显示状态陈旧并按策略进入 QueuedOffline 或拒绝
- **AND** 不得显示为实时成功

#### Scenario: STALE 不覆盖最后已知生命周期
- **GIVEN** 目标最后一次已接受观测的 `lifecycleStatus` 为 `RUNNING`
- **AND** 此后没有观测且已超过新鲜度阈值
- **WHEN** 服务端生成目标 Read Model
- **THEN** `freshnessStatus` 为 `STALE`
- **AND** `lifecycleStatus` 仍为 `RUNNING` 且 `lastKnownStateAt` 保持为最后观测时间

#### Scenario: 服务端要求风险确认
- **GIVEN** STALE 策略对目标升级返回要求显式风险确认
- **WHEN** 用户在未提交有效确认或使用旧投影版本确认的情况下提交升级
- **THEN** 服务端拒绝提交并返回可确认的策略原因
- **AND** 不创建可执行 Provider Step

#### Scenario: 服务端将陈旧写入排队
- **GIVEN** 目标为 STALE 且策略结果为排队
- **WHEN** 用户提交写操作
- **THEN** 服务端将 Operation 置为 `PendingApproval` 或 `QueuedOffline`
- **AND** 客户端确认不得把该决策提升为立即执行

### Requirement: [RT-006] EdgeRuntimeTarget 具体注册
EdgeRuntimeTarget SHALL 支持注册 KubeEdge 集群，包含 CloudCore endpoint、节点组映射和 KubeEdge 版本。平台 SHALL 通过 CloudCore 代理发现边缘节点状态，不直接连接 EdgeCore。

CloudCore endpoint SHALL 是不含用户信息、token 或内联凭据的规范化地址；认证材料 SHALL 仅以归属同一 tenant 的 SecretReference 表示。注册成功后，Edge lifecycle Provider SHALL 建立 tenant-bound CloudCore observer，并按照 RT-008 发布目标、能力和节点清单观测；注册响应或同步探测结果不得绕过该投影契约直接写集群 Read Model。

**Traceability:** EDGE-02, ERT-001, ERT-002, RT-008, RT-009, CFG-002

#### Scenario: 注册 KubeEdge 集群
- **GIVEN** KubeEdge 集群已部署 CloudCore
- **WHEN** 用户提供 CloudCore endpoint 注册 EdgeRuntimeTarget
- **THEN** 平台记录 CloudCore endpoint 并开始探测
- **AND** 平台发现边缘节点列表和状态

#### Scenario: CloudCore 凭据以内联值提交
- **GIVEN** EdgeRuntimeTarget 注册输入的 endpoint 含用户信息或输入包含明文 token
- **WHEN** 平台校验 RuntimeIntent
- **THEN** 平台拒绝请求且不创建 observer 或 Operation
- **AND** 错误和日志仅包含违规字段路径而不包含凭据值

### Requirement: [RT-007] KubeEdge 隧道集成
KubeEdge 节点 SHALL 使用 CloudHub–EdgeHub 隧道与中心通信，平台 SHALL NOT 在 KubeEdge 节点上部署 HNB Agent。平台通过 CloudCore API 获取节点状态和事件。

CloudCore observer SHALL 通过受认证的 CloudCore API 读取边缘节点状态和事件，将其规范化为 RT-008 观测契约后再更新 Read Model。该 observer SHALL NOT 把 CloudCore 接收顺序当作平台提交顺序，且 SHALL 使用 observer generation 和 sequence 对重连、重放及乱序事件进行 fencing。CloudCore 不可达时 SHALL 更新独立的连接性和新鲜度信息，不得伪造节点生命周期变化或清空最后已知节点清单。

**Traceability:** EDGE-02, RT-002, RT-005, RT-008

#### Scenario: 通过 CloudCore 查询边缘节点
- **GIVEN** 边缘节点通过 CloudHub–EdgeHub 隧道连接
- **WHEN** 平台查询节点状态
- **THEN** 平台通过 CloudCore API 获取节点信息
- **AND** 不直接连接边缘节点

#### Scenario: CloudCore 重连后重放旧事件
- **GIVEN** CloudCore observer 已投影更高 generation 或 sequence 的节点清单
- **WHEN** 重连后收到较旧事件
- **THEN** 投影器丢弃旧事件并记录乱序指标
- **AND** target、CapabilitySnapshot 和节点投影均不倒退

## ADDED Requirements

### Requirement: [RT-008] 租户绑定的观测与有序资源清单投影
Agent 和 CloudCore observer SHALL 通过同一个版本化 RuntimeTargetObservation 契约上报目标状态、能力和节点资源清单。契约 SHALL 至少包含 `schemaVersion`、`eventId`、`tenantId`、`targetId`、`targetKind`、`observerId`、`observerKind`（`Agent` 或 `CloudCore`）、`observerGeneration`、单调递增的 `sequence`、`observedAt`、`inventoryMode`（`Full` 或 `Delta`）以及可选的 target、capability 和 nodes 分区；节点 SHALL 具有在目标内稳定的 `nodeId`、状态、资源、版本、`observedAt` 和 `lastKnownStateAt`。

observer SHALL 使用 mTLS 或等价工作负载身份认证，身份授权 SHALL 绑定唯一 tenant、允许的 targetId/targetKind 和 observerKind。平台 SHALL 以认证身份为权威校验 payload，跨租户、跨目标、来源类型不符、未知 schema 或未来时间超出允许时钟偏差的观测 SHALL 在持久化前拒绝。

投影器 SHALL 以 `(tenantId, targetId, observerId)` 保存最高已接受 `observerGeneration`、`sequence` 和幂等 eventId，并在单个数据库事务中按 generation、sequence 顺序更新 observer cursor、RuntimeTarget 投影、不可变 CapabilitySnapshot 和节点投影。重复事件 SHALL 幂等成功；较低 generation/sequence SHALL 丢弃；同 generation 的序列缺口 SHALL 延迟或拒绝并请求重放，不得越过缺口提交后续状态。较高 generation 仅可由重新认证并获授权的 observer lease 建立，建立后旧 generation SHALL 被 fencing。

`Full` 节点清单 SHALL 对同一 target 中未出现的旧节点写入带时间戳的 absent/tombstone 投影，`Delta` SHALL 仅修改明确列出的节点；两者均不得物理删除审计历史。Read Model 查询 SHALL 只读取该投影，不得在请求路径实时调用 Agent、CloudCore 或遍历 RuntimeTarget。

**Traceability:** RT-003, RT-005, RT-006, RT-007, CONTRACT-004, TENANT-005, TENANT-006

#### Scenario: Agent 首次提交完整资源清单
- **GIVEN** tenant-bound Agent identity 被授权观测一个 KubernetesTarget
- **WHEN** Agent 提交合法的 generation 1、sequence 1、`inventoryMode=Full` 观测
- **THEN** 投影器在一个事务中更新 target、能力快照、节点和 observer cursor
- **AND** 集群列表、详情和节点接口从投影读取一致结果

#### Scenario: 同一观测被重复投递
- **GIVEN** 某 observation 已成功提交
- **WHEN** 相同 `eventId` 或 generation/sequence 再次到达
- **THEN** 投影器返回幂等成功且不重复创建快照或节点
- **AND** observer cursor 和 `lastKnownStateAt` 不被重放时间推进

#### Scenario: 观测序列存在缺口
- **GIVEN** observer generation 4 的最后接受序列为 20
- **WHEN** 序列 22 在序列 21 之前到达
- **THEN** 投影器不提交序列 22 的 target、snapshot 或 nodes 分区
- **AND** 平台记录缺口并请求或等待序列 21 重放

#### Scenario: observer 尝试跨租户上报
- **GIVEN** observer identity 绑定 tenant A 和 target A1
- **WHEN** payload 声明 tenant B 或 target B1
- **THEN** 平台在投影事务前拒绝观测
- **AND** tenant B 的 target、snapshot、nodes 和 cursor 均不改变

### Requirement: [RT-009] RuntimeTarget 生命周期 Provider 与可执行计划
平台 SHALL 将集群生命周期 RuntimeIntent 解析为不可变 ExecutionPlan，并为每个可执行 Step 固定非空 `providerId`、版本、Step 类型、目标引用、幂等键、fencing generation、经 Schema 白名单化和规范化的 inputs 以及 SecretReference。调用方 SHALL NOT 提交 Step 或选择 providerId；planner SHALL 根据下列规范矩阵路由，未知、重复、不健康、不合规或与矩阵不匹配的 Provider SHALL fail closed，且 SHALL NOT 回退到通用 Kubernetes 应用 Provider、模拟 Provider 或无操作 Step。

| targetKind | create | import | upgrade | unmanage | lifecycle providerId | observation source |
|---|---|---|---|---|---|---|
| `KubernetesTarget` | REQUIRED | REQUIRED | REQUIRED | REQUIRED | `runtime-target.lifecycle.kubernetes` | `Agent` |
| `EdgeRuntimeTarget` | UNSUPPORTED | REQUIRED | REQUIRED | REQUIRED | `runtime-target.lifecycle.edge` | `CloudCore` |
| `ContainerEngineTarget` | UNSUPPORTED | UNSUPPORTED | UNSUPPORTED | UNSUPPORTED | none in cluster contract | separate runtime-target contract |

在本矩阵中，`create`、`import`、`upgrade` 和 `unmanage` 分别 SHALL 生成至少一个由 lifecycle Provider 执行的 `provision-and-register`、`register`、`upgrade` 或 `unregister` 副作用 Step；状态等待/校验 Step MAY 独立存在但不得替代副作用 Step。`unmanage` SHALL 撤销平台 observer/管理凭据和管理关系，SHALL NOT 删除未由该 Operation 明确拥有的工作负载或基础设施。`UNSUPPORTED` 组合 SHALL 在计划阶段以稳定原因 `TARGET_ACTION_UNSUPPORTED` 拒绝且不创建可执行 Operation；providerId 无法精确解析 SHALL 以 `PROVIDER_ROUTE_NOT_FOUND` 失败。

持久化的计划、Operation、事件、审计、日志和 Provider 输出 SHALL 仅包含经 Schema 允许的非敏感 inputs 与 SecretReference，SHALL NOT 包含 kubeconfig、token、私钥或 Secret 值。URL inputs SHALL 移除或拒绝 userinfo、敏感 query/fragment；SecretReference SHALL 在计划阶段验证 tenant 和用途归属，并仅在 Step 执行期间按最小权限解析到内存。Provider MUST 在副作用边界校验 tenant/target 归属、幂等键和 fencing generation，并通过 RT-008 观测而非直接伪造 Read Model 成功状态。

**Traceability:** P1-WRITE-001, P1-WRITE-002, P1-WRITE-004, RDI-001, RDI-002, RDI-004, RDI-005, CFG-002, PROV-003, PROV-005

#### Scenario: 纳管 KubernetesTarget
- **GIVEN** 已授权的 `import` RuntimeIntent 引用 tenant-owned kubeconfig SecretReference
- **WHEN** planner 生成 ExecutionPlan
- **THEN** 计划包含路由到 `runtime-target.lifecycle.kubernetes` 的可执行 `register` Step
- **AND** 计划只保存 SecretReference 和白名单化 inputs，不保存 kubeconfig 内容

#### Scenario: 纳管 EdgeRuntimeTarget
- **GIVEN** 已授权的 `import` RuntimeIntent 引用规范化 CloudCore endpoint 和 tenant-owned SecretReference
- **WHEN** planner 生成 ExecutionPlan
- **THEN** 计划包含路由到 `runtime-target.lifecycle.edge` 的可执行 `register` Step
- **AND** Provider 成功后由 CloudCore observer 通过 RT-008 投影目标和节点

#### Scenario: 请求创建 EdgeRuntimeTarget
- **GIVEN** 当前兼容矩阵将 EdgeRuntimeTarget `create` 标记为 `UNSUPPORTED`
- **WHEN** 用户提交该组合的 RuntimeIntent
- **THEN** planner 返回 `TARGET_ACTION_UNSUPPORTED`
- **AND** 不创建 Provider Step 或目标侧副作用

#### Scenario: 生命周期 Provider 路由缺失
- **GIVEN** 一个 KubernetesTarget 升级计划要求 `runtime-target.lifecycle.kubernetes`
- **WHEN** Worker 无法精确解析该 providerId
- **THEN** Step 以 `PROVIDER_ROUTE_NOT_FOUND` 失败且不调用其他 Provider
- **AND** Operation 不得被标记为成功

#### Scenario: 解除纳管被幂等重放
- **GIVEN** lifecycle Provider 已以相同 tenant、target、幂等键和 fencing generation 撤销管理关系
- **WHEN** Worker 重放 `unregister` Step
- **THEN** Provider 返回相同资源结果且不重复产生副作用
- **AND** 非托管工作负载和基础设施不被删除

### Requirement: [RT-010] 集群目标兼容与 Conformance 门禁
RT-009 的 targetKind/action/providerId 矩阵 SHALL 是服务端 planner、Provider Registry、能力 API 和 UI 可用动作的共同规范来源，SHALL 版本化且不得分别硬编码出互相矛盾的副本。UI 隐藏不支持动作不构成安全边界；服务端 SHALL 对每次 RuntimeIntent 重新执行矩阵、目标归属、Provider 健康/版本、SecretReference 和 STALE 策略检查。

Kubernetes 与 Edge lifecycle Provider 在声明 Production Ready 前 SHALL 通过版本绑定的 conformance suite。该 suite SHALL 至少覆盖矩阵的每个 REQUIRED/UNSUPPORTED 单元格、真实可执行 Step 路由、成功与失败终态、幂等重放、fencing、取消/超时、输入白名单与 Secret 泄漏检查、跨租户拒绝，以及 RT-008 的重复、乱序、序列缺口、observer generation 切换和 target/snapshot/nodes 原子投影。仅验证计划生成、使用 mock/no-op Provider 或直接写 Read Model SHALL NOT 作为生命周期或观测 conformance 通过证据。

**Traceability:** RT-001, RT-005, RT-008, RT-009, PROV-004, PROV-005, RDI-005

#### Scenario: Provider 宣称支持但未通过矩阵测试
- **GIVEN** lifecycle Provider manifest 宣称支持某 REQUIRED targetKind/action 组合
- **WHEN** 对应版本缺少 conformance 证据或测试失败
- **THEN** Provider Registry 不得将该组合标记为 Production Ready
- **AND** planner 对生产执行 fail closed

#### Scenario: 前端提交被隐藏的不支持动作
- **GIVEN** 客户端绕过 UI 并为 ContainerEngineTarget 提交集群升级 RuntimeIntent
- **WHEN** 服务端执行兼容矩阵检查
- **THEN** 请求以 `TARGET_ACTION_UNSUPPORTED` 被拒绝
- **AND** 不创建可执行 Operation 或 Provider 调用

#### Scenario: Conformance 验证有序原子投影
- **GIVEN** 测试按重复、乱序和跨 generation 顺序提交同时包含 target、capability 和 nodes 的观测
- **WHEN** conformance suite 检查数据库与 Read Model
- **THEN** 仅合法连续序列原子可见且重复观测无额外副作用
- **AND** 不存在 target、CapabilitySnapshot 与 nodes 来自不同已提交序列的撕裂状态
