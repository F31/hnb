## Context

当前 Minimal 验收覆盖已有单节点 K3s 上的 19 负载就绪、登录/读 API、Agent 连接及滚动恢复。`docs/acceptance/registry.yaml` 仍记录正式 import 的服务凭据和观测缺口、插件全生命周期部分验收、生产 HA/离线阻塞；`hnbctl --execute` 的夹具 Chart 演练不能替代正式发布包首次安装。现有 `kubernetes-runtime-provider` 的 KRP-001 仅允许受限 Deployment deploy/delete，不能推导 Service、PVC、入口及任意升级能力。

S0 源码核对更正：Portal 已存在于本仓库 `web/` pnpm workspace，Shell 为 `@hnb/shell` 0.1.0，拥有共享 API client、Schema/插件模块、Vitest 与 Playwright 入口；不再把“缺独立前端仓库”视为阻塞。人员 owner、实际构建摘要和真实前后端 E2E 仍需确认。盘点见 `docs/acceptance/product-loop-baseline.md`，四档断言见 `docs/deployment/profile-composition-contract.md`。

1.4/1.5 的分组输入见 `docs/deployment/decision-baseline.md` 修订 0.1。2026-10-17 用户指示继续后采纳 S1-IDENTITY 为开发输入，其他组仍 Proposed；复核现有 IAM owner、令牌 profile 上限、条件/联合模型及复制恢复边界。本批范围/消费者契约见 `docs/deployment/service-credential-renewal.md`，不以决策文档代替完整在线 Schema、迁移、环境预算或验收证据。

本变更服务实施人员、应用管理员、平台运维和扩展开发者。采用阶段性实现和证据门，现有 change 继续拥有其代码和演练任务；本变更只补产品闭环差额及跨能力验收。

## Goals / Non-Goals

**Goals:**
- 四档共享统一技术底座、组件目录和扩展机制，所有已发布组件/服务在各档位均可按条件选装；档位只决定推荐默认部署、容量及保障目标。
- S4 可交付的基础闭环：正式包→独立安装→独立集群纳管→有状态应用→升级/故障恢复。
- S6 可运营的生产组合：身份持续有效、状态可信、可诊断、预算可验证、真实扩展及按范围认证的 HA/完整恢复。
- 用同一组版本化行为契约支撑 UI/CLI；基础部署低配置成本，能力可按需添加并安全移除。

**Non-Goals:**
- 不扩展为全功能竞品复制，不自研指标/日志/CI 存储引擎，不默认部署全部生态组件。
- 不强制合并 L2 进程；不将外部 Provider 编译进 Core；不合并跨平面数据库。
- 不以开发夹具、同集群自导入、合成身份材料、requests 或仅渲染成功认证业务/生产。
- S7 的更多云厂商、联邦、边缘和 AI 集成不是 S4 基础闭环的前置条件。

## Decisions

### 1. 阶段与执行依赖

| 阶段 | 优先级 | 进入条件 | 交付及退出证据 |
|---|---|---|---|
| S0 范围与基线 | P0 | 本提案获批 | 版本/目标/前端 owner、依赖任务映射、参考应用与数值预算批准；缺环境按项 blocked |
| S1 持续身份 | P0 | S0 契约明确 | 服务续期/撤销、Agent 入网/重连，跨 TTL、服务重启和跨租户负向测试 |
| S2 独立目标纳管 | P0 | S1 通过 | 正式 import→Agent→有序观测→读模型，独立目标断连/重连验证 |
| S3 应用黄金路径 | P0 | S2 与所需动作 Conformance 通过 | 有状态参考应用创建、外部读写、版本升级、故障恢复和保留数据；候选包证据 |
| S4 可重复交付 | P0 | S3、安装工具包对应发布任务完成 | 正式包在干净已有 Kubernetes 安装，另一实施人员复现 S2/S3；基础闭环退出 |
| S5 可运营与轻量 | P1 | S4 通过；基线测量可从 S0 开始 | UI/CLI 一致状态、诊断链、标准遥测接入和能力组合资源回归 |
| S6 生产与生态 | P1 | S5；所选 HA/恢复环境到位 | 真实参考扩展、三节点故障/容量及整组恢复；只认证实测矩阵 |
| S7 可选集成 | P2 | S4 通过后可独立推进 | 首个 GitOps 集成的单写入所有权验收；DevOps/异构/边缘逐项独立 change |

S4 是第一次可对外声明“基础产品闭环”的门；S6 是对应组合的生产声明门。S7 可选择延后，延期须留存范围决定；不得阻塞已通过的基础组合，也不得把未实现集成列为支持。每阶段失败不撤销已有较低范围证据，但禁止提升更高阶段。单项按约一工作日拆分，跨天演练拆为准备、执行、归档。

### 2. 复用边界与现有 change 的责任表

| 已有工作 | 本变更依赖 | 本变更新增差额 |
|---|---|---|
| standardize 3.x/4.x 身份材料、Secret、安装执行 | 复用身份根、迁移器、RunStore、ReleaseLock 和发布目录 | 长期服务身份自动续期、撤销传播、正式包跨用户旅程 |
| standardize 4.7/4.8/5.9 纳管、handoff | 复用正式 Intent、Observer 和动作矩阵 | 不使用临时 token，在独立业务目标贯穿观测与读模型 |
| standardize 4.10/5.7/8.1 交付 | 任务仍在原 change 完成并附证据 | 本变更 S4 只做包身份核验、实施人员独立安装与业务复验 |
| standardize 5.6/7.2–7.5 备份、离线、HA | 引用适用 OS/拓扑/版本报告 | 恢复集合一致性、业务可用性及产品声明；离线声明仍以原任务断网实测为前置 |
| in-cluster-ha-backings 5.4/5.5/6.2 | 复用 CNPG/JetStream 故障、容量证据与归档后主规格 | 检查实际 operator/CRD 就绪、应用持续探针及全站恢复闭环 |

若依赖尚未归档，引用 change 路径和固定 Requirement ID；归档后更新引用位置，不复制 ADDED Requirement 或把其已勾选项当作业务 E2E。外部 backing 的证据不能替代 in-cluster-ha，反之亦然。

### 3. 架构与事实源

```text
Portal / hnbctl ──versioned public APIs──> apiserver BFF + IAM owner
                                           │ service identity + actor
                       ┌───────────────────┴────────────────────┐
                       v                                        v
             platform-api / Core                         App Market API
             Resource Graph / SecretReference refs       Releases / Catalog
             Plan / Operation / Read Model               Artifact descriptors
                       │                                        │
             Core PG + transactional outbox              owning-plane PG
                       │                                        │
                   JetStream                    Artifact authority (OCI/S3/PVC)
                       │                             │ direct bytes
               operation-worker                      v
                       │ pinned typed steps      target/provider
          ┌────────────┼─────────────────────┐
          v            v                     v
     runtime       workload/storage       gateway Provider
     lifecycle     capability handlers      │
          └────────────┬─────────────────────┘
                       v
            independent KubernetesTarget
           Agent/observer ──authenticated observations──> Core projection

Target collectors ──> external metrics/logs backends
Portal ──> scoped query/diagnostic BFF ──> approved backends
```

- 各平面拥有自己的数据与数据库身份；即便共用 PG 服务，也不得跨 owner 查询/写表。
- 运行业务变更只走不可变 Plan/Operation；Agent 入网身份端点只建立身份，不接受任意部署命令。Day-0 安装和受限灾难恢复仍按现有 ownership/receipt 契约执行，平台恢复后关联运行记录，不能覆写 Operation 历史。
- Provider 不获得 Operation 数据库写权限；NATS 只传元数据命令/事件。制品字节和客户卷数据在目标与存储之间传输，Portal/BFF 不代理。
- 查询复用 Read Model；诊断日志/终端另走有界、逐动作授权的通道，不用列表查询隐式扇出到全部目标。

### 4. 逻辑边界与物理组合分离

#### 4.1 统一底座与四档推荐预设

四档是同一平台的推荐部署预设，不是四条源码分支、四套内核或互斥服务目录。同一软件发布以统一 Core、身份/租户模型、Plan/Operation/观测契约、Provider 协议和发布列车交付。相同组件版本不因档位更换为功能裁剪镜像；不同拓扑所需的角色、配置和部署模式仍可不同。

| 对外档位 | 产品定位 | 默认差异的边界 | 共同可选能力 |
|---|---|---|---|
| Minimal | 演示/评估 | 单实例、小规模、低默认负担，不附生产/HA 保证 | 统一目录内全部已发布能力 |
| Lite | 最小生产 | 最低已验收生产拓扑、基础运维与恢复保障 | 同上 |
| Standard | 常用生产 | 常用容量、默认运维组合及维护余量 | 同上 |
| Pro | 高级特性与企业生产 | 更高保障目标和高级场景推荐组合 | 同上，不自动安装全部组件 |

高级特性可在 Pro 默认推荐，但不能成为 Pro 专属技术实现。例如 Minimal 可以选装 GitOps，Lite 可以接入灾备能力；是否可执行取决于实际目标/站点、资源、版本、权限和认证，不因档位名称拒绝。启用能力不自动改写档位，不自动赋予生产/HA 认证，也不豁免该能力在实际组合的验收。能力需要更多节点或外部服务时展示可选基础设施变更计划；可以推荐升档，但不能用名义升档代替真实依赖检查。

T0/T1/T2/T3 表达交付基线、默认性、成熟度和认证条件，不是 Minimal/Lite/Standard/Pro 的功能授权表。Portal 的能力目录可展示用户有权发现的未安装/不满足依赖项，标注原因与安装路径；业务菜单仍遵守 UX-001，不能因目录可见而授予操作权限。

#### 4.2 组合与档位的正交关系

能力组合使用版本化 `compositionId`（工作名 `foundation`、`container-management`、`production-enhancements`），推荐档位继续用现有 Minimal/Lite/Standard/Pro，另有 backing 和 `releaseVersion` 维度。组合是可调整的模板，不是封闭档位；“生产增强”不表示 Lite 缺少生产必需保障，也不表示能力只对 Pro 开放。产品名到既有 Minimal/LiteHA/StandardHA/Enterprise 的映射保持兼容。组合不重命名旧 overlay，也不能绕过必需 T0/T1 校验；稀疏 foundation 只能保持 startup/明确未认证状态，不能冒充 Minimal。

组合清单从 overlay 与 Provider manifest 派生，记录依赖、外部服务、目标侧 Agent/采集器及资源增量，禁止另建手工组件列表成为第二事实源。按依赖闭包校验启用/禁用；已有 Operation/资源引用时拒绝移除，数据默认保留。UI 按已安装能力、Conformance、目标能力与权限的交集显示。

不预设“合并一定更省”。S5 可提交同平面内部模块共进程的测量型 ADR，但只能在接口、所有权、凭据最小权限及故障隔离验证后决定；外部 Provider 仍独立。替代方案“所有组件合并”缺乏收益证据；“同 Pod 多容器”不消除各进程 RSS，不能单凭 Pod 数计算收益。

#### 4.3 实际安装态与三类增量演进

不能把“发布版本”“档位”和“能力包版本”都折叠成一个 edition。当前安装态由受信运行记录、锁、配置和实际观测共同核对，至少区分：

| 字段 | 含义 |
|---|---|
| `releaseVersion` / `releaseLockRef` | 当前软件发布与固定物料 |
| `profile` / `profileDefaultsVersion` | 当前推荐预设及其版本 |
| `selectedCapabilities` | 用户实际选择的能力及版本/配置引用，独立于预设 |
| `effectiveComponents` / `configurationOverrides` | 依赖解析后的实际组件、拓扑、资源及覆写 |
| `backing` / resource bindings | 数据面方式、PVC/外部服务/目标身份引用 |
| `evidenceRefs` | 该实际组合已验证的动作、环境及保障范围 |

这些是既有配置/锁/运行记录的契约增量，不创建第二个权威状态库。敏感配置只保留引用。合成规则：

```text
desired capabilities = target preset required capabilities
                     ∪ retained installed capabilities
                     ∪ explicitly requested additions
effective components = dependency closure(desired capabilities, pinned manifests)
effective configuration = compatible current overrides merged with target requirements
```

“保留”是保留逻辑能力、用户配置意图与资源身份，不是盲目固定不兼容旧二进制。软件升级为每个已装能力求解目标 release 兼容版本并输出变更；无解、证据缺失或迁移条件不符时，在副作用前列出阻断，不静默删除能力、退回默认值或更换数据源。源自用户选择的排除项保留，除非与目标必需基线冲突，此时明确要求重新计划，不强制安装无关 Pack。

| 变更类型 | 示例 | 保留/执行语义 |
|---|---|---|
| 同档能力变更 | Minimal 添加 GitOps；Lite 接入遥测/灾备 | 保持 profile，通过依赖规划和正常生命周期增量加载 |
| 档位演进 | Minimal + GitOps → Lite | 保留 GitOps/版本兼容意图及配置，在原实例增加目标必需拓扑与保障；不将 Lite 默认覆盖当前组合 |
| 软件版本升级 | Lite 的 release A → B | profile、能力选择与数据绑定保持，按兼容矩阵更新 Core/已装组件和迁移，不要求先改 Pro |
| 组合变更 | 将观测组件改为外部后端 | 显式计划并验证引用/依赖/数据及可逆性，不等价于重装或删除客户资源 |

现有预设 `Minimal ⊆ Lite ⊆ Standard ⊆ Pro` 偏序继续描述推荐基线的非降级演进；不对任意两个自定义能力集合施加“高级功能只能出现在高档”的全序。普通升档与升级不得减少实际安装态的能力、已调高资源、副本、故障保障或 PVC；缩减/移除必须走独立、显式且受依赖保护的计划，沿用现有有界降级/卸载契约。

计划固定当前安装态摘要与目标锁，副作用前重新验证；并发能力变更、配置漂移或目标身份改变使计划失效，必须重新求解。所有演进复用现有 RunStore/Operation、幂等与 fencing，不重建租户、目标或业务应用。backing 转换复用 PUPG-005，不能因 overlay 切换直接替换存储。

“平滑”意味着原实例、保留配置/身份/数据、兼容预检、可恢复阶段及可观测验证，并非对所有拓扑承诺零停机。单实例转换 HA、不可逆迁移等需在计划中明确中断范围、备份、维护窗口和恢复条件；有冗余且验证过的路径优先滚动执行，并以持续业务探针验证允许的中断预算。

### 5. 身份与 Agent 入网

复用受信身份发行与 SecretReference，增加 tenant/subject/audience/action/target 绑定的短期服务身份。客户端在批准的提前量内续期，采用有界退避和抖动；不自动扩大 TTL/权限，不靠重启读取新 token。保留 actor 与 service subject 的不同审计身份。

Agent bootstrap credential 一次使用、有过期时间、绑定目标和预期身份；交换由受信服务原子消费并签发可轮换的目标身份。首次连接或受控重新入网产生新 observer generation，旧 generation fencing。续期失败时旧身份仅在未过期/未撤销期限内有效；超时后停止受保护调用，保留业务运行和最后已知状态。重连使用身份与观测协议，不重新创建 RuntimeTarget。

撤销包括新请求及持久隧道重新鉴权/关闭，传播上限必须在 S0 的预算中批准并由实测证明。发行/撤销状态不可用且无法证明身份仍有效时 fail closed；不将过期身份默认为可用缓存。

### 6. 应用黄金路径与类型边界

参考应用采用 digest 锁定的 HTTP 应用和挂载 PVC，提供数据写入/读取及版本响应；它必须真实访问卷数据，不能只返回“healthy”。可由受限 Deployment 挂载卷实现，不强制把 StatefulSet 纳入首轮。

`ApplicationDeliveryBinding` 关联 Release/CompositionRelease、tenant、target、namespace、资源 UID、期望/观测 revision、PVC/SecretReference、Gateway binding、Operation 与观测水位。复用现有组合和资源绑定，缺失字段再做增量迁移，不新建第二套调度/状态机。

能力动作按版本化 Schema 分离：Deployment 继续受 KRP-001 约束；PVC、Service、Secret 挂载引用、HTTPRoute/Gateway binding 由显式声明的 capability handler/Provider 执行。S0 冻结 action 名与 Schema；新动作在契约、幂等/fencing、权限与真实 Conformance 完成前保持 unsupported。禁止把旧 deploy 放宽为任意 YAML。Gateway/CSI controller 和实际数据面独立预检。

应用升级固定源/目标 digest、配置、数据兼容声明及备份点；先验证候选，失败保留前版本或建立新的受控恢复 Operation。回滚生成新 Operation，不修改历史成功结果；不兼容数据迁移不得自动降级。删除按所有权、UID 和 generation 检查，默认保留 PVC/数据；保留 KRP-006 墓碑防止迟到部署复活。

### 7. 数据模型、API 与事件契约

以下为逻辑模型和待 S0 冻结的契约增量，不代表当前接口已存在：

| 模型 | Owner / 内容 | 持久化策略 |
|---|---|---|
| ServiceIdentityLifecycle | 身份 owner：subject、tenant、audience、scope、key/credential version、expiry、revocation version | 复用现有身份记录；只存引用/加密材料与审计，不留明文 token |
| AgentEnrollment | 身份 owner：bootstrap hash/reference、target、expiry、consumedAt、identity version | 单次消费的事务/CAS；撤销与历史保留 |
| ApplicationDeliveryBinding | Core：release ref、target、resource refs、revisions、Operation、observation refs | 复用组合/绑定表，必要时增量字段；其他平面只通过 API 引用 |
| ReadinessEvidence | 验收 registry：stage、release digests、matrix cell、budgets、report refs、outcome | 扩展既有 registry/artifact，不新增在线事实数据库 |
| CompositionManifest | overlay/manifest 派生：capability closure、external dependencies、resources | 固定发布物料，不另维护手写镜像/组件清单 |
| EffectiveInstallation | Core/安装器既有配置、锁与运行记录：release/profile、实际能力、覆写、数据引用、当前态摘要 | 在已有权威记录上扩展；计划绑定摘要并校验漂移，不另建状态库 |

API 增量：受 audience 限制的服务身份 issue/renew/revoke；一次性 enrollment exchange 与 reenroll；应用组合 validate/plan/submit 与应用就绪 Read Model；统一能力目录、组合/依赖/实际安装态/支持范围只读查询；能力变更/升档/软件升级的增量计划与版本选择；租户隔离的遥测/诊断查询。浏览器调用全部经 BFF，CLI 复用公开契约；运行写入仍复用现有 Intent 提交入口，不新增任意 steps 接口。身份端点的发行权限与普通业务权限分离。

事件增量：identity lifecycle、enrollment consumed、application observation 和 readiness evidence reference；均携带 schemaVersion、eventId、tenant、actor/service subject、correlation 和相关 resource/Operation 引用。发布遵循 owning-plane outbox；身份撤销的在线判定不只依赖最终一致消息。事件不得承载 token、kubeconfig、卷内容或制品字节。

所有新请求/响应/事件先有版本化 Schema、兼容检查及生成客户端；未知字段、越权 scope、过期版本及重放拒绝遵循现有 Problem Details 契约。公共错误码在 S0 固定，UI/CLI 消费同一字典。

### 8. 状态机与成功语义

```text
service identity: Active -> Renewing -> Active(new version)
                                  \-> RenewalFailed -> Expired
                  Active/Renewing -> Revoked
enrollment: Issued -> Consumed | Expired | Revoked
target: existing lifecycle + independent health/connectivity/freshness dimensions
application journey: Accepted -> Executing -> AwaitingObservation -> Verifying -> Ready
                                      \-> Blocked | Failed | Degraded
stage evidence: not_run -> partial | blocked | failed | passed
```

上述 journey 是聚合视图，不新增或改写 Operation 终态。Operation succeeded 后，应用仍可 AwaitingObservation；只有同目标、同版本、足够新鲜的观测与外部访问/读写检查都满足才 Ready。验证超时保留真实 Operation 终态，标记验证失败/未知并给原因；UI/CLI 不把 HTTP 202 或 deploy succeeded 映射成可用。

### 9. 资源、性能与可观测预算

S0 批准版本化 budget artifact，包含每组合空闲 working set/RSS、并发峰值内存/CPU、镜像压缩下载量与解包磁盘、冷安装/就绪时间、每目标 Agent/连接/采集器增量、PG 连接上限与活跃/空闲细分、Outbox/JetStream 积压/保留、API P95/P99、Operation/投影延迟和恢复目标。每项必须有数值、单位、持续时间、采样间隔、硬件/OS/K8s/网络/存储及通过规则；未批准不得进行容量或性能认证。

分别统计控制平台、底层 Kubernetes、目标侧组件、可选外部服务；展示合计和排除项。至少跑冷启动、稳定空闲、读写混合/Operation、一个与多个独立目标、依赖中断恢复；记录失败/超时/429，分位数注明样本范围，不能只报成功请求。生产规模稳定性窗口在 S0 批准；不能把瞬时 RSS 或各时刻 CPU 峰值相加当并发峰值。

复用现有 Prometheus-compatible 查询和租户/目标标签规范，日志使用标准采集/查询适配；完整指标/日志后端可外置。最小组合仍提供健康、结构化执行日志、事件和脱敏诊断，不因未装高级观测而无法排障。监控不可用返回 Unknown/Unavailable，不当作健康。

### 10. 兼容矩阵与 Conformance

| 维度 | 首轮范围 | 门禁 |
|---|---|---|
| 安装 | 已有 Kubernetes，固定 OS/arch/K8s/Chart/digest | 真实包冷安装/恢复；bootstrap/离线单独引用原 change 实测 |
| 目标 | 独立 KubernetesTarget，Agent 主动认证连接 | import、重放、断连/重连、租户/target fencing；Edge/CAPI 不推导 |
| 应用 | Deployment + typed Service/PVC/Secret ref + 一种 Gateway 实现 | 每 action Schema、CAS/幂等、失败/取消、真实目标验收 |
| 存储 | 选择一个真实 CSI/SC 后端 | provisioning、挂载读写、保留和适用扩容/快照声明；fixture 不认证 |
| 入口 | Gateway API + 一个实际 controller/data plane | route programmed、真实外部探针、控制面故障时业务持续 |
| 身份 | Core/Provider/Agent 的固定协议版本 | TTL 跨期、重启、撤销上限、轮换重叠与跨 scope 拒绝 |
| HA | 指定 profile + backing + ≥3 实际故障域 | operator/CRD 实际探测、failover/容量报告、业务探针与 RTO/RPO |
| 扩展 | 一个参考扩展的固定 Core/Shell/协议版本 | install/enable/upgrade/rollback/disable/uninstall 及依赖阻断 |
| S7 GitOps | 一个选定适配器/控制器版本 | 资源所有权唯一、冲突拒绝、handoff 与撤销测试 |
| 跨档组装与升级 | 四档同目录；Minimal + 可选能力→Lite→Standard→Pro 及同档 release A→B | 同组件摘要/契约一致；能力、覆写、数据及身份保留；漂移/兼容缺失拒绝 |

Conformance 的契约/功能/安全/故障/幂等/fencing/性能证据必须绑定 action、版本、实际镜像摘要与有效期；现有已通过 import 不提升新 action。参考扩展覆盖安全的 UI 注册和真实 backend action，不能只装 ConfigMap。

### 11. 升级、备份与灾备

恢复集合以 manifest 关联各平面的备份、schema/发布锁、身份根/撤销版本、PG 一致性点、JetStream cursor/消费者状态、制品权威摘要/字节和必要的业务卷备份。不得假定多服务快照天然同一时刻：按工作负载采用停写/排空或有水位的重放/对账策略，保留恢复前后 Operation 身份、Outbox 与幂等记录。

在隔离的新环境按依赖顺序恢复，先验证身份与 schema，再恢复消息/元数据关联，最后开放写入和实际探针。重复投递不能重复执行；恢复旧身份材料不能使已撤销凭据重新有效。平台数据恢复与客户业务卷恢复分开报告，缺任一所声明必需项则该范围失败。

RPO 以持续写入的有序标记测量已确认但未恢复的数据；RTO 从预定义故障时刻到服务与业务探针连续通过的时刻，不只量 Pod Ready。阈值在演练前批准；单节点恢复通过不提升 HA。

### 12. S7 集成所有权

优先接入一个现有 GitOps 控制器，不自建 CI/GitOps 引擎。每资源明确 `HNB` 或外部 controller owner 及版本：外部 owner 下 HNB 只观测，或由 Operation 修改受控 Git 声明并等待控制器收敛；不能同时直接写相同对象。移交必须暂停旧 writer、验证 revision/UID、CAS 更新 owner 后启用新 writer，审计可回溯。更多 DevOps/边缘/云厂商能力以独立 change 定义，保持禁用直到真实 Conformance。

## Risks / Trade-offs

- [范围跨多服务且可能长期执行] → 阶段门、每任务独立证据、S4/S6 分别发布范围；不以全 change 完成替代阶段验收。
- [前端人员 owner、实际物料和联调环境未确认] → 已定位本仓库 `web/`，S0 固定负责人与构建身份；可以推进本地实现，真实 UI 验收保持 blocked，不以假页面代替。
- [新 typed actions 会扩大运行权限] → 逐动作 Schema/权限/能力矩阵/真实 Conformance，保留旧 Deployment-only 契约。
- [令牌撤销与长连接状态不同步] → 连接级重鉴权/失效、受控缓存和有上限的传播验收。
- [PG 连接和事件保留随集群增长] → 全进程连接池预算、保留/归档与有界背压，按独立目标数测量。
- [仅声明三域或 operator-ready 即被误放行] → 在生产执行副作用前探测实际 Node/CRD/Operator 状态，缺事实阻断；CLI 声明仅规划输入。
- [恢复点不一致或旧凭据复活] → 恢复水位/重放对账、隔离恢复、撤销版本校验与外部业务探针。
- [组合名被误当可靠性档位] → API/文档分别输出 composition/profile/backing/evidenceScope，不继承未测认证。
- [默认预设被误做功能白名单或升档重置配置] → 所有档位共用目录/组件契约，基于实际安装态做保留型增量合成，测试已装可选能力不丢失。
- [“平滑升级”被误解为无条件零停机] → 明示逐路径业务中断预算，单实例路径展示维护窗口，HA 路径以探针验证滚动可用性。

## Migration Plan

1. S0 固定契约、owner、预算、动作白名单与兼容矩阵；不改主规格或既有认证。
2. S1/S2 使用 expand-first 的 owning-plane 迁移，先部署兼容读者、发行/校验服务，再切换客户端；旧安全路径在兼容窗口内保留，不保留不受限 token 回退。
3. S3 通过新 capability/action 逐项启用，未启用目标保持旧行为；历史资源不自动接管。
   - 从已有配置和运行记录补齐实际安装态，不以档位默认值反推全部用户选择；无法确定的引用/覆写需复验或显式补充。兼容读取旧配置/锁，未知能力不静默丢弃。
4. S4 固定 release 包，在独立干净环境复验；中断 resume 复用绑定身份，不重复初始化目标或创建 Operation。
5. S5/S6 分别增加可选组件与 backing，现有 PVC 不缩容；借用已有 change 的升级/故障证据并核对物料身份。
   - 以固定源状态摘要与目标锁生成增量计划，保留已装能力/配置，先演练同档加载和跨档保留，再执行生产演进；纯升档不自动启用未选择的高级 Pack。
6. 回滚优先关新能力、恢复兼容服务/注册快照；不删除撤销/审计/墓碑、不撤销合法 fencing。不可逆迁移恢复整个批准集合或前向修复。
7. 每阶段留存 registry 记录；全部必需任务验证后运行 `openspec validate --all --strict`、统一语义门禁和可用 verify，再 archive 合并 delta。

## Open Questions

- S0 已确认 Portal 为本仓库 `web/`；仍需确认人员维护者、实际构建物料及真实后端联调环境，测试入口存在不等于 UI E2E 完成。
- S0 选择独立业务集群、一个真实 CSI/SC、一个 Gateway 实现及参考应用 digest，记录网络/域名/TLS 条件；单节点现有主机不能替代三故障域。
- S0 批准各数值预算、身份 TTL/续期/撤销界限、稳定性窗口及业务 RTO/RPO；本文不编造尚未测得的保证。
- S7 选择首个 GitOps 控制器及版本；若推迟，须以范围决策记录和后继 change ID 保留追踪，不得把对应未实施任务勾选为已实现。
