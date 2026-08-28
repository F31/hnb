# HNB Cloud V3.6 方案评审报告与 KubeEdge 边缘计算补充方案

> **文档性质**：评审报告 + 优化建议 + 补充方案（建议并入 V3.7）
> **评审对象**：《HNB Cloud 开放可组装云原生平台项目落地方案 V3.6》
> **评审日期**：2026-07-18
> **评审范围**：总体架构、四平面解耦、制品存储、组合编排、AI 增强架构、安全供应链、实施路线、验收体系，以及边缘计算能力缺口

---

## 第一部分 总体评审结论

### 1.1 总体评价

V3.6 是一份成熟度很高的平台工程方案。其核心判断——微内核、四平面分离（市场 / 制品存储 / 运行治理 / AI 扩展）、全部容器化、OCI 优先统一制品、不可变发布、Operation 状态机、AI 可选增强且不设执行旁路——方向正确、边界清晰、可落地性强。方案在以下维度表现突出：

| 评审维度 | 评分 | 简评 |
|---|---|---|
| 架构边界与解耦 | ★★★★★ | 市场/平台/制品/AI 四平面职责矩阵（RACI）和"禁止的耦合方式"清单是同类方案中少见的严谨设计 |
| 制品与供应链 | ★★★★★ | OCI 优先、digest 锁定、Referrer 关联签名/SBOM、双重门禁、撤销处置闭环完整 |
| 制品存储工程化 | ★★★★★ | 分档位 HA、直传直取、安全 GC、巡检周期表、RPO/RTO 按实测承诺，工程细节扎实 |
| AI 架构治理 | ★★★★☆ | "AI 不是事实源、不设执行旁路、故障隔离"原则正确；治理对象（Prompt/Guardrail/评测）全部纳入版本与摘要管理，设计领先 |
| 组合编排 | ★★★★☆ | CompositionRelease → ExecutionPlan → DAG/Saga 分工明确，Helm 定位准确 |
| 可观测与运维 | ★★★★☆ | 巡检、演练、容量预测产品化思路好 |
| 实施路线 | ★★★★☆ | 先闭环后扩展、MVP 收敛合理，退出标准可检验 |
| **边缘计算** | ★★☆☆☆ | **最大缺口**：全文仅在部署形态表（15.1）中出现"边缘轻量版"、在 ContainerEngineTarget（13.3）提及"小规模边缘"，无系统性边缘架构；但第 1 章承诺"同一套容器化产品可以部署到……边缘环境"，承诺与能力不匹配 |
| 文档质量 | ★★★★☆ | 结构完整；存在编号错误等可快速修复的缺陷（见 2.1） |

**一句话结论**：方案在云侧已达到可评审立项的水平；**边缘计算是唯一显著的体系性缺口**，建议以本文第三部分（KubeEdge 边缘计算补充方案）并入 V3.7，同时修复第二部分列出的文档与工程细节问题。

### 1.2 方案亮点（应保留并发扬）

1. **"禁止的耦合方式"清单（7.2）**：把反模式写成显式约束，是防止架构腐化的有效手段，建议每版评审时回归检查。
2. **AI 确定性降级原则**：AI Extension Plane 可整体关闭、AI 结论不是事实源、Copilot 无执行旁路——这在当前"AI 功能堆砌"的行业环境中是难得的清醒设计。
3. **制品存储的"伪高可用"禁令**：明确禁止"多 Registry 副本各自使用独立本地目录"，并要求按实测定 SLA，避免了大量同类平台的宣称性 HA。
4. **安全 GC 与引用保护**：运行实例、回滚点、组合、灾备、离线 Bundle 五类引用联合判定后再回收，设计周密。
5. **发布不可变 + 摘要寻址贯穿全文**：从市场 Release 到 ExecutionPlan 到审计，digest 主线一致，可复现性强。
6. **JAR/WAR 双轨制**：不可变镜像（生产推荐）+ 标准运行时承载（受控兼容），既守住容器化底线又给出迁移路径。
7. **AIOps 四级渐进 + 证据/置信度强制输出**：自动化边界定义清晰。
8. **验收体系可执行**：CTN/MKT/ART/STO/CMP/INT/SEC/OFF/AI 系列验收项均可测试，不是口号。

---

## 第二部分 问题清单与优化建议

按优先级分为 P0（并入下一版前必须处理）、P1（建议处理）、P2（可选优化）。

### 2.1 P0：必须处理的问题

#### P0-1 边缘计算体系缺失（最重要）

**问题**：方案多处承诺边缘能力，但无架构支撑：

- 执行摘要承诺"同一套容器化产品可以部署到物理机、虚拟机、私有云、公有云或**边缘**环境"；
- 13.2 称 KubernetesTarget"可运行在……边缘节点上"；
- 15.1 部署形态表有"边缘轻量版（k3s/K0s 或 Podman）"；
- 6.5.10 三级分发提到"边缘缓存"，但无边缘节点如何注册、如何自治、如何升级、如何纳管设备的内容。

真实的边缘场景（工厂、园区、门店、变电站、车载）具备以下特征，当前方案均未覆盖：

- 边缘节点数量大（数百到数万）、单节点资源小（2-8 vCPU）、arch 多样（含 ARM）；
- 云边网络弱、按流量计费、可能断连数小时到数天，**断连期间必须本地自治**；
- 需要接入非 IP 设备（Modbus、OPC UA、CAN、BLE 等）；
- 无人值守，要求云端批量 OTA 升级与回滚；
- k3s/K0s 整站轻量集群方案在**断连自治**和**设备管理**两个关键点上不满足（其 kubelet 直连 API Server，断连后无法本地重启调度、无设备抽象）。

**建议**：采纳本文第三部分方案，以 KubeEdge 作为边缘运行目标的一等实现，新增 `Edge Pack` 能力包、`edge.kubeedge` 能力域、`EdgeRuntimeTarget` 目标类型、边缘资源模型、EDGE 系列需求与验收项。

#### P0-2 RuntimeTarget 模型缺少边缘字段（13.1）

**问题**：`connection.mode` 仅枚举了 `outbound-agent`；`capabilities`、`labels` 中无边缘语义；无断连容忍、带宽档位、自治策略字段。

**建议**：扩展 RuntimeTarget：

- `spec.type` 增加 `kubeedge-edge`（或 `edge`）；
- `connection.mode` 增加 `cloudedge-tunnel`（云边隧道）与 `offline-bundle`（纯离线）；
- 增加 `spec.edge` 子结构：nodeGroup、autonomyPolicy（断连自治级别）、bandwidthProfile（限速/窗口）、offlineRevocationPolicy（离线撤销处置）。

#### P0-3 ReleaseManifest 兼容性声明不足（8.3）

**问题**：`compatibility` 只有 `kubernetes` 版本与 `architectures`，无法表达"该 Release 是否可部署到边缘、是否支持断连自治、最小内存/磁盘要求、是否需要持续在线"。

**建议**：增加：

```yaml
compatibility:
  kubernetes: ">=1.30 <1.34"
  architectures: [amd64, arm64]
  targetTypes: [kubernetes, container-engine, kubeedge-edge]   # 新增
  edge:                                                        # 新增
    offlineAutonomy: required | supported | not-supported
    minMemory: 512Mi
    minDisk: 2Gi
    wanRequired: false
```

市场发布门禁应校验：声明支持 `kubeedge-edge` 的 Release 必须通过边缘兼容性测试（arm64 镜像、资源上限、无云依赖探活）。

#### P0-4 撤销列表的离线分发缺失（8.4、18.5）

**问题**：平台本地缓存含"撤销列表"，但边缘节点断连数天时，既无法收到 `market.release.revoked` 事件，也无法实时校验。边缘节点可能在断连期间继续运行已撤销版本，重连后的处置策略未定义。

**建议**：

- 撤销列表必须**签名**，并作为独立 OCI Artifact 通过制品分发通道下发到边缘缓存；
- 定义离线撤销处置策略：断连期间允许继续运行（默认，保障业务连续），重连后按风险等级执行告警/隔离/强制升级，全部进入审计；
- 高安全场景允许配置"边缘心跳超时后自动降级运行集"策略。

#### P0-5 文档编号错误（第 28 节）

**问题**：“最终落地原则”列表在 22 条之后又出现“**18. 运维内建**”，编号回退且 18 重复；实际共 23 条原则。

**建议**：修正为连续编号 1–23；同时建议将"运维内建"前移（它是平台级原则，不应垫底）。另建议为全文生成目录（TOC）——3000+ 行文档无目录，评审和查阅成本高。

#### P0-6 Capability 与能力包缺边缘域（11.1、11.2）

**建议**：

- Capability 增加：`edge.kubeedge`、`edge.node-group`、`edge.device-management`、`edge.offline-autonomy`、`edge.ai-inference`、`edge.ota-upgrade`；
- CapabilityPack 增加 **Edge Pack**（KubeEdge 控制面、边缘应用管理、设备管理、批量 OTA），默认策略：可选。

### 2.2 P1：建议处理的问题

| 编号 | 位置 | 问题 | 建议 |
|---|---|---|---|
| P1-1 | 13.1 / 6.5.10 | Agent 主动连接模型与边缘通道并存，未说明关系 | 明确：**中心集群走 HNB Agent（mTLS 主动连接）；边缘节点走 KubeEdge CloudHub 隧道，不重复部署 HNB Agent**。平台通过 Edge Provider 适配 KubeEdge API，避免双通道运维与双份凭据 |
| P1-2 | 19.2 | Operation 状态机缺终态与滞留 SLO 定义；无边缘排队状态 | 明确 Succeeded/Failed/Cancelled 为终态；增加 `QueuedOffline`（目标离线，等待重连投递）状态；为每个非终态定义最大滞留时间与超时升级策略 |
| P1-3 | 18.2 | 签名机制未具体化 | 明确采用 Sigstore Cosign（keyless 或 KMS 密钥）签名；构建证明对齐 SLSA L3 目标；准入策略以 OPA/Kyverno Provider 实现并纳入认证等级 |
| P1-4 | 5.1 / 18 | Secret Reference 只提概念 | 明确 Secret 分层：平台只存引用；后端支持外部 KMS/Vault Provider；**边缘节点 Secret 由 MetaManager 本地加密落盘**，密钥由节点证书派生，禁止明文 |
| P1-5 | 19.1 | 可观测未覆盖断连补传 | 边缘侧指标/日志本地环形缓冲（按容量水位），重连后补传；Read Model 中边缘对象必须标注 `lastKnownStateAt`，Copilot 回答边缘问题时必须展示该时间戳，防止把过期状态当事实 |
| P1-6 | 12.10 | AI 可观测缺边缘推理指标 | 增加边缘模型端点的冷启动、端侧 Token 吞吐、模型下发进度、边云 Fallback 次数 |
| P1-7 | 15.3 | hnbctl 无边缘命令族 | 增加 `hnbctl edge` 子命令（见第三部分 3.13） |
| P1-8 | 17.4 | 场景化向导无边缘场景 | 增加："批量纳管边缘节点"、"发布边缘应用到节点组"、"边缘设备接入与数据采集"、"边缘模型下发与灰度"四个向导 |
| P1-9 | 22.1 | 团队无边缘方向 | 增加边缘计算 2–3 人（Edge Provider、设备 Mapper、边缘交付与现场运维工具） |
| P1-10 | 24.7 | 性能指标无边缘项 | 增加：单 CloudCore 域支撑 ≥5,000 边缘节点（设计目标，实测校准）；云边隧道消息 P95 延迟与吞吐可观测；断连 72 小时后重连状态收敛时间可测量 |
| P1-11 | 25 | 风险表无边缘风险 | 增加边缘专项风险（见第三部分 3.16） |
| P1-12 | 4.1 / 13 | 多集群（Karmada）与边缘边界未划清 | 明确：Karmada 管"集群级"多集群分发（V2）；KubeEdge 管"节点级"边缘纳管；同一节点不得同时被两套体系接管 |
| P1-13 | 21 | 实施路线无边缘里程碑 | V1.5 增加 Edge Pack（见第三部分 3.15） |
| P1-14 | 6.3 / 6.4 | 市场分类与标签无边缘维度 | 一级分类"应用"下增加"边缘应用"，"基础能力"下增加"设备接入"；标签增加：离线自治、低功耗、边缘协议（Modbus/OPC UA/CAN/BLE）、ARMv7 |
| P1-15 | 15.1 | "边缘轻量版 k3s/K0s 或 Podman"与 KubeEdge 选型关系未说明 | 增加选型矩阵（见第三部分 3.14）：整站轻量集群选 k3s/K0s；云边拆分 + 断连自治 + 设备管理选 KubeEdge；单机受限选 Podman/ContainerEngineTarget |

### 2.3 P2：可选优化

1. **26.6 表格后存在多余空行**（第 3134–3135 行），格式清理。
2. **计量计费前置预留**：计量与商业授权排在 V2（21.5），但 `AIUsageRecord` 已存在；建议 MVP 期即预留统一的 `UsageRecord`/`MeterEvent` 事件契约，避免后期补埋点。
3. **ISV 沙箱**：V1.5 有 ISV 发布者门户，建议同步提供"发布者沙箱环境"（扫描、兼容性预检、Conformance 自助跑），降低生态接入成本。
4. **采纳度量**：建议定义平台采纳指标（DORA 四指标 + 市场月活发布者 + 部署成功率），作为产品化运营的北极星。
5. **国际化**：Portal 与 CLI 预留 i18n 框架，边缘现场运维界面尤需多语言。
6. **时间同步治理**：边缘节点时钟漂移会影响证书校验、Token TTL 与审计时序，建议将 NTP/PTP 检查纳入边缘节点入网预检与巡检。

---

## 第三部分 KubeEdge 边缘计算补充方案（建议并入 V3.7）

> 本部分按 V3.6 文档风格编写，建议作为新增章节并入下一版；对现有章节的修订点汇总见 3.17。
>
> 技术基线：KubeEdge（CNCF 毕业项目）v1.2x，云边拆分架构——云端 CloudCore（EdgeController / DeviceController / CloudHub / SyncController），边缘侧 EdgeCore（EdgeHub / Edged / MetaManager / DeviceTwin / EventBus / ServiceBus），云边之间通过单条 WebSocket/QUIC 隧道复用 API 流量、设备数据与元数据同步；MetaManager 在边缘本地持久化期望状态，提供断连自治能力 [^1^][^2^]。v1.20 起提供批量节点运维、EdgeApplication 与节点组解耦（支持 targetNodeLabels）、多语言 Mapper-Framework [^3^]；v1.21 起提供 NodeUpgradeJob / ImagePrePullJob v1alpha2、ConfigUpdateJob 云端批量配置更新、节点组流量闭环 [^4^]；v1.23 引入设备异常检测框架、边缘本地节点查询优化，Kubernetes 依赖升级至 v1.32.10 [^5^]。

### 3.1 定位与设计原则

边缘计算能力作为 HNB Cloud 的**可选能力包（Edge Pack）**，遵循 V3.6 既有原则并补充边缘特化原则：

1. **不进入微内核**：KubeEdge 控制面（CloudCore）以容器化 Provider/组件形式部署，通过 `runtime-kubernetes` Driver 安装到管理集群；未启用 Edge Pack 的平台与现有功能完全一致。
2. **云边权责一致**：市场仍定义"有什么、是什么版本"；平台仍决定"部署到哪里、能否部署"；KubeEdge 只是新增的一类 **RuntimeTarget 与执行通道**，不引入第二套发布、授权、审批体系。
3. **云为权威、边可自治**：期望状态由云端下发，边缘 MetaManager 本地持久化；断连期间边缘按最后已知期望状态继续运行、重启、本地调度；重连后批量对账（SyncController） [^2^]。
4. **单隧道、可计量**：云边管理流量复用 CloudHub-EdgeHub 单条隧道，支持限速、压缩与流量窗口，适应按流量计费的弱网链路。
5. **离线安全不降级的可控例外**：断连期间允许继续运行已下发版本（保障业务连续），但撤销列表、策略更新按签名离线包分发；重连后强制执行撤销处置并审计（对应 P0-4）。
6. **无人值守运维**：节点入网、升级、配置变更、镜像预拉取全部支持批量、灰度、自动回滚，不要求现场人员。

### 3.2 总体架构

```text
┌───────────────────────────── 中心侧（管理集群/中心 Region）─────────────────────────────┐
│ HNB Portal / CLI / OpenAPI                                                              │
│ HNB Cloud Platform                                                                      │
│   ├── Edge Provider（edge-adapter）：适配 KubeEdge API，实现 RuntimeDriver 契约          │
│   ├── Edge Read Model：EdgeNode/NodeGroup/EdgeApplication/Device 只读视图               │
│   └── Operation Engine ── 边缘 Operation（部署/升级/回滚/OTA/配置下发）                  │
│ HNB App Market ── 边缘应用/设备 Mapper/边缘 AI 产品分类                                  │
│ HNB Artifact Storage ── 中心权威 Registry（arm64 多架构制品）                            │
│ KubeEdge CloudCore（容器化，多副本）                                                     │
│   ├── CloudHub（WebSocket/QUIC，多副本 + LB）                                            │
│   ├── EdgeController / DeviceController / SyncController                                 │
│   └── ControllerManager / TaskManager（NodeUpgradeJob/ImagePrePullJob/ConfigUpdateJob）  │
│ 可选 AI Extension Plane ── 中心 AI Gateway / 模型训练与评测                              │
└───────────────────────────────────┬─────────────────────────────────────────────────────┘
                                     │ CloudHub–EdgeHub 隧道（mTLS，单连接复用，限速/压缩）
        ┌────────────────────────────┼────────────────────────────┐
        ▼                            ▼                            ▼
┌───────────────┐           ┌───────────────┐             ┌───────────────┐
│ 边缘站点 A     │           │ 边缘站点 B     │             │ 离线站点 C     │
│ （节点组 GA）  │           │ （节点组 GB）  │             │（纯离线 Bundle）│
│ EdgeCore      │           │ EdgeCore      │             │ 本地 Registry  │
│ ├ EdgeHub     │           │ ├ EdgeHub     │             │ + EdgeCore     │
│ ├ Edged       │           │ ├ Edged       │             │ （人工导入）   │
│ ├ MetaManager │           │ ├ MetaManager │             └───────────────┘
│ ├ DeviceTwin  │           │ ├ DeviceTwin  │
│ ├ EventBus    │           │ ├ EventBus    │
│ └ 边缘工作负载 │           │ └ 边缘推理服务 │
│ 边缘 Registry Mirror/节点缓存（可重建）                                    │
│ 设备 Mapper（Modbus/OPC UA/MQTT/BLE/CAN）                                  │
└───────────────┘           └───────────────┘
```

组件交付约束（延续 3.4 容器化硬约束）：

- CloudCore、ControllerManager、TaskManager、Edge Provider 全部容器化，由 Helm Chart 部署到管理集群；
- EdgeCore 以**节点级代理**形态交付（二进制 + systemd，由 keadm/批量任务安装），这是容器化硬约束的**唯一显式例外**——它本身是容器运行底座的管理者，且其安装、升级、回滚由 `NodeUpgradeJob`/批量 OTA 任务接管，纳入 Operation 与审计；
- 边缘业务负载、Mapper、边缘 AI 推理服务**必须容器化**，遵守全部 CTN 要求。

### 3.3 边缘运行目标模型

#### 3.3.1 EdgeRuntimeTarget（对 13.1 的扩展）

```yaml
apiVersion: platform.hnb.io/v1
kind: RuntimeTarget
metadata:
  name: edge-factory-east
spec:
  type: kubeedge-edge                 # 新增目标类型
  region: cn-east-1
  connection:
    mode: cloudedge-tunnel            # outbound-agent | cloudedge-tunnel | offline-bundle
  edge:
    nodeGroupRef: factory-east        # KubeEdge NodeGroup
    autonomy:
      offlineRun: allow               # 断连允许继续运行
      offlineSchedule: deny           # 断连禁止接收新调度（默认）
      heartbeatTimeout: 10m
      maxOfflineDuration: 720h        # 超过则标记 Lost，触发处置策略
    bandwidth:
      profile: metered                # metered | broadband | offline
      maxMbps: 20
      transferWindows: ["01:00-06:00"]
    revocation:
      offlinePolicy: run-and-report   # run-and-report | quarantine-on-reconnect
  capabilities:
    - runtime.kubernetes
    - edge.kubeedge
    - edge.device-management
    - artifact.oci-image
  labels:
    environment: production
    site: factory-east
    arch: arm64
```

#### 3.3.2 边缘资源模型（平台 Read Model 新对象）

```text
EdgeNode          # 边缘节点：架构、资源、EdgeCore 版本、证书状态、最后心跳、自治状态
NodeGroup         # 节点组：站点/区域逻辑分组，流量闭环边界，批量运维单元
EdgeApplication   # 边缘应用：跨节点组批量部署单元，支持 targetNodeLabels 与 per-node overriders
DeviceModel       # 设备模型：属性、遥测、协议、采集频率
Device            # 设备实例：Twin 期望/上报状态、Mapper 绑定、异常检测配置
EdgeOTAJob        # OTA 任务：节点升级/镜像预拉取/配置更新的平台侧投影
```

以上对象在 KubeEdge 侧对应同名 CRD（`apps.kubeedge.io`、`devices.kubeedge.io`、`operations.kubeedge.io`）[^3^][^4^]；平台只保存投影与关联关系，权威状态在管理集群，边缘状态经隧道汇聚。

#### 3.3.3 与 Capability/Blueprint 的衔接

- 新增 Capability：`edge.kubeedge`、`edge.node-group`、`edge.device-management`、`edge.offline-autonomy`、`edge.ai-inference`、`edge.ota-upgrade`；
- ServiceBlueprint 增加 `supportedTargets: [kubeedge-edge]` 的产品支持范围；依赖 CRD/Operator 的复杂有状态产品**默认不支持** `kubeedge-edge`（与 13.3 对 ContainerEngineTarget 的限制一致，边缘资源小、无 etcd，禁止把 HA 数据库塞到边缘节点）；
- 边缘适用的典型产品：协议采集 Mapper、流式/批处理边缘应用、轻量规则引擎、边缘推理服务、边缘网关代理、边缘 AI Gateway 轻量实例。

### 3.4 云边通道与 HNB Agent 的职责划分（关键设计决策）

| 场景 | 通道 | 执行者 | 说明 |
|---|---|---|---|
| 中心/区域 Kubernetes 集群 | HNB cluster-agent（mTLS 主动连接） | Agent + Runtime Driver | 维持 V3.6 现状 |
| 单机容器主机 | HNB container-agent | Agent + Container Engine Driver | 维持 V3.6 现状 |
| **KubeEdge 边缘节点** | **CloudHub–EdgeHub 隧道** | **Edge Provider → K8s API → KubeEdge** | **不再部署 HNB Agent 到边缘节点** |

理由：

- 边缘节点资源小，双 Agent 浪费内存/CPU 且增加证书、升级、排障的双重成本；
- KubeEdge 隧道已提供认证、加密、断连缓存与重连对账，与 HNB Agent 的 mTLS 主动连接设计目标等价 [^1^]；
- Edge Provider 作为平台侧的 Domain Provider + Runtime Driver 组合，把平台 Operation 翻译成 KubeEdge CRD（EdgeApplication / Device / NodeUpgradeJob 等），保持"平台唯一编排入口"原则不被破坏；
- 平台对边缘的权限、配额、审计照常生效：Edge Provider 以平台 ServiceAccount 调用管理集群 K8s API，RBAC 边界不变。

### 3.5 边缘应用交付闭环

```text
市场选择边缘兼容 Release（targetTypes 含 kubeedge-edge）
→ 租户/项目/环境 + 选择 NodeGroup 或 targetNodeLabels
→ 平台预检：授权、撤销、签名、arm64/资源下限、自治声明、配额
→ 生成不可变 ExecutionPlan（含限速窗口、灰度批次、回滚策略）
→ Operation Engine 执行：
     ① 制品预分发（中心 Registry → 区域 Mirror → ImagePrePullJob 节点预拉取）
     ② 按批次创建 EdgeApplication（含 per-node overriders）
     ③ 健康门禁：边缘上报 Running/Ready 才放行下一批
     ④ 失败补偿：暂停批次、回滚上一稳定版本、标记 NeedsAttention
→ 状态聚合回 Edge Read Model（标注 lastKnownStateAt）
→ 审计 + 市场兼容性回传（最小字段）
```

约束：

- 批量部署必须提供**灰度批次与失败容忍度**（如 5% → 25% → 100%，failureTolerate 0.3），对齐 KubeEdge 节点任务的并发/容错模型 [^4^]；
- 边缘 Operation 支持 `QueuedOffline`：目标节点离线时进入排队，重连后按序投递，超过 `maxOfflineDuration` 转为 Failed 并告警；
- 同一站点内多组件组合（如 采集 Mapper + 规则引擎 + 上行网关）仍走 CompositionRelease，由平台保证站点内依赖顺序；
- 节点组内服务访问默认**流量闭环**：组内应用只能访问同组服务，实现区域网络隔离 [^4^]；跨组/回源访问需在 ExecutionPlan 中显式声明。

### 3.6 边缘自治与断连策略

| 断连场景 | 边缘行为 | 平台行为 |
|---|---|---|
| 短时闪断（分钟级） | EdgeHub 自动重连；工作负载不受影响 | 记录抖动指标，不告警升级 |
| 持续断连（小时–天） | MetaManager 按本地缓存继续运行、重启失败容器；本地配置/设备状态继续采集 [^2^] | Read Model 标记 `Unknown` + `lastKnownStateAt`；禁止对该节点发起新写操作（除非策略允许排队） |
| 节点本地重启 | Edged 依据本地持久化状态恢复工作负载 | — |
| 重连恢复 | 批量对账：SyncController 解决云边差异；指标/日志补传 | 收敛时间可观测；执行排队的 QueuedOffline Operation；按撤销策略处置过期版本 |
| 超过 maxOfflineDuration | 节点标记 `Lost` | 触发处置策略（告警/工单/现场派单）；副本语义允许的负载按策略在健康节点重建 |

Copilot/AIOps 交互约束：回答任何边缘对象状态时，必须附带 `lastKnownStateAt`；对断连节点的诊断建议必须区分"最后已知事实"与"推断"。

### 3.7 边缘制品分发与离线交付

在 6.5.10 三级分发基础上落地边缘细节：

```text
中心权威 Registry
  → 区域 Registry Mirror/Proxy Cache（可重建）
    → 边缘站点 Registry Mirror（站点级，可重建）
      → 节点 containerd 缓存 + ImagePrePullJob 预拉取
```

- **多架构强制**：声明支持边缘的 Release 必须提供 arm64（及按需 armv7）镜像，发布门禁校验 manifest list；
- **限速与窗口**：跨 WAN 复制与节点预拉取遵守 `bandwidthProfile`；大镜像/模型按传输窗口执行；
- **ImagePrePullJob**：发布窗口前按节点组预热镜像，平台投影为 EdgeOTAJob 并纳入 Operation [^4^]；
- **离线站点**：通过签名 OCI Image Layout Bundle 导入站点本地 Registry（模式同 15.4）；EdgeCore 指向站点 Registry；撤销列表、策略、模型均以签名 Bundle 定期人工/摆渡更新；
- 边缘缓存一律非权威，GC 策略与中心一致（引用保护 + Tombstone）。

### 3.8 设备管理（IoT 接入）

KubeEdge 的差异化能力在于把物理设备纳管为一等 K8s 对象 [^1^]，HNB 将其产品化：

```text
DeviceModel（市场可发布的"设备驱动产品"）
  └── Device 实例（平台侧建模，绑定 EdgeNode + Mapper）
        └── Mapper（容器化，Modbus/OPC UA/MQTT/BLE/CAN，市场分类"设备接入"）
              └── DeviceTwin：desired/reported 状态同步、差量上报
                    └── EventBus（边缘 MQTT）→ 边缘应用消费 / 经隧道上行
```

要点：

- Mapper 作为普通市场产品发布、审核、签名、部署，遵守全部 ART/SEC 门禁；协议参数（串口、从站地址、点表）走参数 Schema，点表文件以 config-package 制品管理；
- 设备数据**默认边缘本地闭环处理**（过滤、聚合、规则引擎），仅摘要/告警/特征值上行，避免遥测打满隧道；上行数据经 EventBus/隧道，不经过平台 API；
- v1.23 的设备异常检测框架（Device CRD pushMethod 配置，Mapper 级可插拔）作为可选能力发布 [^5^]；
- 设备写操作（下发控制指令）必须走平台 Operation：权限校验 → 参数 Schema 校验 → 审计 → 经 DeviceTwin desired 下发；
- 设备指令类 Operation 属高风险，默认要求人工确认或审批策略（延续 AI-OPS-06 的风险分级）。

### 3.9 边缘 AI

延续 12.1"AI 可选化"原则，边缘 AI 是 AI Extension Plane 的边缘延伸：

- **边缘推理作为 ModelService 的一种 target**：`ModelService.spec.targetRef` 可指向 NodeGroup；AI Runtime Provider 与 Edge Provider 协同，把模型制品经三级分发预置到边缘，推理容器由 EdgeApplication 承载；
- **模型下发**：模型权重走制品通道（OCI Artifact + 限速窗口 + 预拉取），不走 AI Gateway/平台 API；版本、摘要、许可证、评测门禁与中心一致（AI-RUN-02/03 在边缘同样生效）；
- **边缘 AI Gateway 轻量实例**：站点内多应用共享本地模型时，可在站点部署单副本 AI Gateway（ Minimal 档），提供 OpenAI-compatible 端点、本地限流与脱敏；中心不可用时按策略本地自治或 Fallback 到中心端点；
- **边云协同**：小模型/量化模型在边缘推理，大模型回源中心 AI Gateway，路由由 AI Gateway 策略决定；Sedna 类边云协同训练、增量学习、联邦学习作为 **V2+ 的市场产品/外部集成**，不进入 Edge Pack 核心；
- **数据合规**：边缘推理的输入输出默认不出站点；需要上传样本做再训练时，按 AI-GOV-04 的数据区域与脱敏策略执行；
- **边缘 AIOps**：断连期间 AIOps 只读能力不可用属预期；边缘本地规则引擎可执行预授权的确定性处置（重启容器、切换本地主备），与 L4 受控修复策略一致并回传证据。

### 3.10 边缘网络与流量闭环

- 默认利用 KubeEdge 节点组**流量闭环**实现区域隔离 [^4^]；
- 跨节点服务发现：可选 EdgeMesh（独立 Provider，按需安装，不进入 Edge Pack 强依赖）；
- 站点出口统一经边缘网关产品（市场发布），支持 TLS 终止、协议转换（MQTT→HTTPS/Kafka）与回源熔断；
- NetworkPolicy 在边缘同样生效，但需注意部分 CNI 能力在 Edged 环境的差异，纳入边缘兼容性测试矩阵。

### 3.11 边缘安全

1. **入网认证**：节点凭一次性 Token + 证书签发流程入网（keadm join），平台侧记录节点身份、证书指纹与站点绑定；批量入网经 `keadm batch` 或平台引导镜像完成 [^3^]；
2. **隧道安全**：CloudHub–EdgeHub 全程 mTLS；证书支持独立轮换，轮换失败节点进入受限模式；高安全环境要求平台、市场、CloudCore 证书链可独立轮换（延续 18.2）；
3. **Secret 边缘落盘**：下发到边缘的 Secret 由 MetaManager 本地加密存储，密钥由节点证书派生；禁止在 Mapper 配置、点表、环境变量中明文落密；
4. **物理安全假设**：边缘节点按"物理不可信"设计——磁盘加密可选、最小化本地数据、撤销列表签名分发、节点丢失（Lost）后可远程吊销证书并作废其 Secret；
5. **准入与策略**：边缘工作负载遵守 Pod Security 与平台准入策略；HostPath/特权容器默认禁止，Mapper 类设备访问容器走白名单豁免并审计；
6. **时钟治理**：入网预检 NTP/PTP 偏差，超阈值拒绝入网；审计事件以云端接收时间为准。

### 3.12 边缘运维与批量 OTA

| 运维动作 | 机制 | 平台投影 |
|---|---|---|
| 节点入网/移除/重置 | keadm join/reset、keadm batch 批量文件驱动 [^3^] | EdgeNode 生命周期 Operation |
| EdgeCore 升级 | NodeUpgradeJob v1alpha2（预检→备份→升级→健康确认→失败自动回滚；keadm edge backup/upgrade/rollback）[^4^] | EdgeOTAJob + 灰度批次 |
| 节点配置批量变更 | ConfigUpdateJob（默认关闭，启用云端 ControllerManager/TaskManager 与边缘 TaskManager；变更会重启 EdgeCore）[^4^] | EdgeOTAJob，强制维护窗口 |
| 镜像预拉取 | ImagePrePullJob | EdgeOTAJob（限速窗口） |
| 日志/exec 诊断 | `keadm ctl` 边缘本地 logs/exec/describe（断连可用）[^3^] | 诊断包采集 Operation |
| 现场恢复 | SD/eMMC 镜像重刷 + 重新入网 | 站点恢复 Runbook |

约束：所有批量任务必须定义并发度、失败容忍、超时与回滚；配置更新类任务因会重启 EdgeCore，必须强制维护窗口 + 站点级串行。

### 3.13 hnbctl 边缘命令族（对 15.3 的补充）

```text
hnbctl pack enable edge
hnbctl edge node list --group factory-east
hnbctl edge node join --batch nodes.yaml
hnbctl edge node upgrade --group factory-east --to v1.23 --batch 5%:25%:100%
hnbctl edge node config-update --group factory-east --field modules.edgeStream.enable=true
hnbctl edge app deploy --release edge-collector-2.1.0 --group factory-east
hnbctl edge device list --node edge-01
hnbctl edge prepull --release edge-collector-2.1.0 --group factory-east
hnbctl edge offline-bundle export --site site-c
hnbctl edge health --group factory-east
```

### 3.14 部署档位与选型矩阵（对 15.1 的澄清）

| 边缘场景 | 推荐形态 | 理由 |
|---|---|---|
| 站点可稳定联网、需完整 K8s 能力、无设备接入 | k3s/K0s 轻量集群 + HNB cluster-agent | 架构简单，复用现有 Agent 体系 |
| **节点多、弱网/断连自治、非 IP 设备接入、批量 OTA** | **KubeEdge（本方案）** | 断连自治 + 设备一等抽象 + 单隧道，唯一同时满足三项的形态 [^1^][^2^] |
| 单机受限环境、无 K8s 需求 | Podman + ContainerEngineTarget | 最小资源占用，接受功能限制（13.3） |
| 完全隔离网络 | 离线 Bundle + 站点本地 Registry（+ 可选 KubeEdge 离线模式） | 摆渡式更新，签名 Bundle 保证供应链 |

Edge Pack 资源建议（CloudCore 侧，随规模按压测校准）：

| 档位 | 边缘节点规模 | CloudCore | 说明 |
|---|---|---|---|
| Edge Lite | ≤ 200 节点 | 2 副本 ×（2 vCPU/4 GB） | 与管理集群共部署 |
| Edge Standard | ≤ 2,000 节点 | 3 副本 ×（4 vCPU/8 GB）+ 独立 LB | CloudHub 独立扩缩 |
| Edge Enterprise | ≥ 5,000 节点/多地域 | 按地域分域部署 CloudCore 集群 | 分域故障隔离，单域 ≥5,000 节点为设计目标 |

### 3.15 实施路线调整建议

| 阶段 | 新增边缘内容 |
|---|---|
| 阶段 0 | KubeEdge 概念验证：单边缘节点入网、断连自治验证（断网 24h 负载持续运行、重连对账）、EdgeApplication 灰度部署原型、arm64 制品三级分发原型 |
| MVP | 不含边缘（保持收敛），但 ReleaseManifest 预留 `targetTypes` 字段 |
| V1.5 | **Edge Pack GA**：Edge Provider、EdgeNode/NodeGroup 纳管、EdgeApplication 批量部署、设备管理（MQTT + Modbus 两个 Mapper 参考实现）、ImagePrePullJob/NodeUpgradeJob 投影、边缘场景化向导、edge 命令族 |
| V2 | 多地域 CloudCore 分域、边缘 AI（模型下发 + 边缘推理 ModelService + 边缘 AI Gateway 轻量实例）、Sedna 协同、设备异常检测产品化、Edge Enterprise 档位认证 |

### 3.16 边缘风险与应对（对第 25 节的补充）

| 风险 | 表现 | 应对 |
|---|---|---|
| 断连期间运行已撤销版本 | 安全漏洞暴露窗口变长 | 签名撤销列表离线分发；重连强制处置；高风险场景配置"超时自动降级运行集" |
| 边缘证书过期/时钟漂移 | 隧道永久断开、审计时序错乱 | 入网 NTP/PTP 预检；证书提前轮换 + 轮换失败受限模式；巡检证书有效期 |
| CloudHub 单点/拥塞 | 数千节点同时重连打爆隧道 | CloudHub 多副本 + LB；重连指数退避 + 随机抖动；消息限速与优先级；分域部署 |
| 边缘存储介质损坏/寿命 | SD/eMMC 磨损导致节点失联 | 日志/缓存写放大控制（环形缓冲、采样）；只读根分区 + 数据分区；介质健康巡检 |
| 批量 OTA 变砖 | 全站点节点同时升级失败 | 强制灰度批次 + 失败容忍度 + 看门狗自动回滚；站点级串行；升级前备份（keadm edge backup）[^4^] |
| 设备协议碎片化 | Mapper 生态失控 | Mapper-Framework 多语言框架统一接入 [^3^]；Mapper 认证等级；点表 Schema 化 |
| 物理不可信节点 | 节点被拆走/篡改 | 远程吊销证书；最小本地数据；磁盘加密可选；Secret 派生密钥不落盘 |
| 边缘资源超限 | 大镜像/大模型压垮小节点 | 发布门禁校验资源下限声明；预检磁盘水位；镜像/模型瘦身与量化标签 |
| 遥测打满隧道 | 原始传感器数据上行 | 边缘本地闭环处理为默认；上行白名单 + 采样/聚合；隧道带宽计量告警 |
| 云边版本偏斜 | EdgeCore 与 CloudCore 不兼容 | 版本兼容矩阵纳入 Provider Conformance；NodeUpgradeJob 先升 CloudCore 后分批升 EdgeCore |

### 3.17 对 V3.6 现有章节的修订点清单

| 章节 | 修订 |
|---|---|
| 0 修订说明 | 增加 V3.7 边缘修订行：Edge Pack、EdgeRuntimeTarget、边缘 AI、边缘制品分发、EDGE 需求与验收 |
| 2.1 架构基础表 | 增加"边缘计算｜新增｜KubeEdge 可选能力包，云边拆分、断连自治、设备管理" |
| 4 总体架构图 | 容器执行面与基础设施层增加 CloudCore/EdgeCore 与边缘节点层 |
| 5.1 内核禁止清单 | 增加"KubeEdge CloudCore 不得编译进入内核"（以 Edge Pack 交付） |
| 6.3/6.4 分类标签 | 增加"边缘应用/设备接入"分类与边缘标签组（P1-14） |
| 8.3 ReleaseManifest | 增加 `compatibility.targetTypes` 与 `compatibility.edge`（P0-3） |
| 11.1/11.2 | 增加 edge.* Capability 与 Edge Pack（P0-6） |
| 13 容器运行目标 | 13.1 扩展 RuntimeTarget（P0-2）；新增 13.6 EdgeRuntimeTarget 节 |
| 15.1 部署形态 | 增加边缘选型矩阵（3.14） |
| 15.3 hnbctl | 增加 edge 命令族（3.13） |
| 17.4 场景向导 | 增加四个边缘向导（P1-8） |
| 18 安全 | 增加 18.7 边缘安全（3.11）；撤销列表签名离线分发（P0-4） |
| 19 可观测 | 增加断连补传与 lastKnownStateAt 约束（P1-5） |
| 21 实施路线 | 按 3.15 调整 |
| 22 团队 | 增加边缘计算 2–3 人 |
| 23.2 验收 | 增加 EDGE-ACC 系列（3.19） |
| 24 资源 | 增加 Edge Pack 资源档位（3.14）与边缘性能指标（P1-10） |
| 25 风险 | 增加边缘风险表（3.16） |
| 26 需求清单 | 增加 26.12 边缘计算需求（3.18） |
| 28 落地原则 | 修复编号（P0-5），新增原则："**云边一体**：边缘是同构运行目标而非特殊通道；云为权威、边可自治、撤销必达" |

### 3.18 边缘计算需求清单（建议新增 26.12）

| 编号 | 优先级 | 需求 |
|---|---|---|
| EDGE-01 | P0 | 提供可选 Edge Pack，基于 KubeEdge 实现边缘节点纳管，未启用时平台功能不受影响。 |
| EDGE-02 | P0 | 边缘节点通过 CloudHub–EdgeHub 隧道接入，不额外部署 HNB Agent；平台通过 Edge Provider 统一管理。 |
| EDGE-03 | P0 | 支持 EdgeNode、NodeGroup、EdgeApplication、DeviceModel、Device 的统一建模与 Read Model 投影。 |
| EDGE-04 | P0 | 边缘节点断连期间必须按本地缓存状态自治运行；重连后自动对账，收敛时间可观测。 |
| EDGE-05 | P0 | 边缘应用批量部署必须支持灰度批次、失败容忍、健康门禁与自动回滚。 |
| EDGE-06 | P0 | 边缘兼容 Release 必须声明 targetTypes、架构、资源下限与离线自治能力，发布门禁强制校验。 |
| EDGE-07 | P1 | 支持 ImagePrePullJob 节点预拉取，跨 WAN 传输支持限速与传输窗口。 |
| EDGE-08 | P1 | 支持 NodeUpgradeJob 驱动的 EdgeCore 批量 OTA 升级，含预检、备份、健康确认与失败自动回滚。 |
| EDGE-09 | P1 | 支持 ConfigUpdateJob 云端批量配置变更，且必须强制维护窗口与站点级串行。 |
| EDGE-10 | P0 | 撤销列表必须签名并支持离线分发；断连运行与重连处置策略可配置、全量审计。 |
| EDGE-11 | P1 | 设备 Mapper 容器化并经市场发布门禁；设备写操作必须经 Operation、权限与审计。 |
| EDGE-12 | P1 | 节点组内服务访问默认流量闭环，跨组访问须显式声明。 |
| EDGE-13 | P1 | 边缘 Secret 本地加密存储，节点证书支持远程吊销。 |
| EDGE-14 | P1 | 边缘指标/日志断连本地缓存、重连补传；Read Model 必须标注 lastKnownStateAt。 |
| EDGE-15 | P2 | 支持边缘推理 ModelService：模型制品经三级分发下发，边缘 AI Gateway 轻量实例可选。 |
| EDGE-16 | P0 | CloudCore 多副本高可用，支持按地域分域部署；单域边缘节点规模目标 ≥5,000（实测校准）。 |
| EDGE-17 | P1 | 离线站点支持签名 Bundle 导入本地 Registry 并驱动 EdgeCore 运行。 |
| EDGE-18 | P0 | 边缘 Operation 支持 QueuedOffline 排队与 maxOfflineDuration 超时处置。 |

### 3.19 边缘验收项（建议新增至 23.2）

| 编号 | 验收项 |
|---|---|
| EDGE-ACC-01 | 未启用 Edge Pack 时，平台安装、运行与既有全部验收项不受影响。 |
| EDGE-ACC-02 | 边缘节点断网 24 小时：已部署负载持续运行、容器自重启正常；恢复后状态自动对账且收敛时间可测量。 |
| EDGE-ACC-03 | 对 100 个边缘节点执行 EdgeApplication 灰度部署（5%→25%→100%），批次间健康门禁生效，失败批次自动暂停并可回滚。 |
| EDGE-ACC-04 | EdgeCore 批量 OTA 升级含预检、备份、看门狗回滚；人为注入升级失败时节点自动回退且负载不中断。 |
| EDGE-ACC-05 | 声明 kubeedge-edge 的 Release 缺 arm64 镜像或未声明资源下限时，发布门禁拒绝。 |
| EDGE-ACC-06 | 撤销版本在断连站点重连后按策略完成处置，全程审计可查；撤销列表签名可被边缘离线验证。 |
| EDGE-ACC-07 | Modbus 与 MQTT 两个 Mapper 经市场发布、部署、采集、上行闭环；设备写指令有 Operation 记录与权限校验。 |
| EDGE-ACC-08 | 节点组 A 内应用无法访问节点组 B 内服务（流量闭环），显式声明后按策略放行。 |
| EDGE-ACC-09 | 边缘节点 Secret 落盘加密验证通过；节点证书吊销后隧道拒绝重连。 |
| EDGE-ACC-10 | CloudCore 单副本故障期间边缘管理面可用；断连节点指标在重连后完成补传，Copilot 回答携带 lastKnownStateAt。 |
| EDGE-ACC-11 | 离线站点通过签名 Bundle 完成导入与边缘应用部署，全程无互联网访问。 |
| EDGE-ACC-12 | QueuedOffline Operation 在节点重连后按序执行；超过 maxOfflineDuration 转为 Failed 并告警。 |

---

## 附：评审结论摘要（供立项会使用）

1. **V3.6 方案整体通过评审**，云侧架构（四平面、制品、编排、AI 治理）达到可立项水平。
2. **必须补充**：KubeEdge 边缘计算方案（本报告第三部分，建议作为 V3.7 核心增量），修复第 28 节编号错误，扩展 RuntimeTarget/ReleaseManifest/Capability 边缘语义，定义撤销列表离线分发。
3. **建议补充**：Secret/KMS 分层、Cosign/SLSA 具体化、边缘可观测补传、边缘团队与验收项、计量契约预留（见 2.2）。
4. **风险提醒**：边缘是唯一"承诺在先、架构在后"的领域；建议在 V3.7 发布前完成 EDGE-ACC-02（断连自治）与 EDGE-ACC-03（批量灰度）两项概念验证。

---

## 参考来源

[^1^]: KubeEdge vs K3s vs MicroK8s: Edge Kubernetes (2026) — CloudCore/EdgeCore 模块组成、单隧道复用、MetaManager 断连自治、DeviceTwin/Mapper 设备管理. https://iotdigitaltwinplm.com/kubeedge-vs-k3s-vs-microk8s-edge-kubernetes-2026/
[^2^]: K3s vs MicroK8s vs KubeEdge: Edge K8s ADR (2026) — MetaManager 本地持久化期望状态、断连批量对账、CloudCore HA 与版本偏斜窗口. https://iotdigitaltwinplm.com/k3s-vs-microk8s-vs-kubeedge-edge-adr-2026/
[^3^]: KubeEdge v1.20 Release — 批量节点运维（keadm batch）、多语言 Mapper-Framework、EdgeApplication 与 NodeGroup 解耦（targetNodeLabels）、CloudHub-EdgeHub IPv6、keadm ctl 边缘 logs/exec. https://kubeedge.io/blog/release-v1.20/
[^4^]: KubeEdge v1.21 Release — NodeUpgradeJob/ImagePrePullJob v1alpha2、ConfigUpdateJob（云端批量配置更新，默认关闭）、节点组流量闭环、keadm edge backup/upgrade/rollback. https://kubeedge.io/blog/release-v1.21/
[^5^]: KubeEdge v1.23 Release Notes — Kubernetes 依赖 v1.32.10、Windows EdgeCore 增强、设备异常检测框架（Device CRD pushMethod）、边缘本地节点查询优化、边缘 DB 重构（Gorm）. https://github.com/kubeedge/website/pull/799
