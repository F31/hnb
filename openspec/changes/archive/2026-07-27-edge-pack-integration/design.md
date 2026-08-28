## Context

HNB Cloud 当前已具备：

- **Kubernetes Runtime Provider**（KRP-001~006）：支持受限 Deployment 生命周期、租户隔离、fencing CAS、逻辑删除
- **Runtime Driver v2**（RDI-001~005）：Provider 路由、版本化契约、可恢复失败、fencing 契约
- **RuntimeTarget 模型**（RT-001~005）：定义了 KubernetesTarget、ContainerEngineTarget、EdgeRuntimeTarget 三类目标

但 EdgeRuntimeTarget 仅停留在抽象定义层面，没有可注册的实现。K3s 虽然是标准 Kubernetes，但平台缺少发行版识别和兼容性标注。KubeEdge 的 EdgeApplication CRD、NodeGroup 和设备模型完全没有对接。

本 change 填补从"平台能管理标准 Kubernetes"到"平台能管理边缘集群"的差距，采用分阶段策略：先 K3s（零成本），再 KubeEdge（新 Provider）。

## Goals / Non-Goals

**Goals:**

- 使 K3s 集群可作为 KubernetesTarget 注册，平台自动识别发行版并标注兼容性
- 实现 EdgeRuntimeTarget 作为可注册、可发现的运行目标类型，支持 KubeEdge 集群
- 新建 Edge Runtime Provider 将 HNB 的 deploy/delete 映射为 KubeEdge EdgeApplication CRD
- 支持边缘节点组定义和灰度批次约束
- 所有新增能力通过 Runtime Driver v2 现有契约集成，不引入新通信协议

**Non-Goals:**

- Device Mapper 框架和设备治理（EDGE-005）— 后续 change
- 离线 Bundle 签名导入（EDGE-006）— 后续 change
- Edge Mesh 或边缘服务网格 — KubeEdge EdgeMesh 已有，HNB 不重复
- 边缘节点上的 Agent 部署 — KubeEdge 节点不重复部署 HNB Agent
- 多集群 Federation（Karmada 等）

## Decisions

### D1: K3s 通过现有 Kubernetes Provider 接入，零新代码

**决策**：K3s 是标准 Kubernetes 发行版，兼容标准 API。不创建独立 Provider，仅在 RuntimeTarget 注册时增加 `distribution` 字段（`k3s`/`kubeedge`/`standard`）用于兼容性标注。

**理由**：避免重复代码。K3s 的差异（SQLite 替代 etcd、内置 Traefik、local-path-provisioner）属于 Kubernetes 发行版特性，不影响 HNB 的受限 Deployment 模型。

### D2: KubeEdge 通过新建 Edge Runtime Provider 接入

**决策**：创建独立的 `edge-runtime-provider` 二进制，实现 Runtime Driver v2 契约，将 HNB 的 deploy/delete 翻译为 KubeEdge EdgeApplication CRD。

**理由**：

- KubeEdge 的 EdgeApplication 不是标准 Kubernetes Deployment，需要特殊的 CRD 操作
- 独立的 Provider 保持 KRP 的纯净性，不把边缘逻辑混入通用 Provider
- 遵循架构中"Edge Pack 不是第五平面，云为权威、边可自治"的原则

### D3: Edge Runtime Provider 通过 CloudCore API 操作

**决策**：Edge Runtime Provider 连接 KubeEdge 的 CloudCore API（而非直接连接边缘节点），CloudCore 负责将状态同步到 EdgeCore。

**理由**：

- RT-002 已明确：KubeEdge 节点使用 CloudHub–EdgeHub 隧道，不重复部署 HNB Agent
- 离线自治由 KubeEdge 自身保证（EdgeStore 本地存储），HNB 只需在重连后对账

### D4: NodeGroup 作为 ExecutionPlan 的亲和性约束

**决策**：在 ExecutionPlan 中增加 `node_group_affinity` 字段，类型为 `[]string`，表示目标节点组列表。Worker 在路由 Step 时根据该字段选择目标 Provider，Provider 将 NodeGroup 传递给 KubeEdge 的 EdgeApplication spec。

**理由**：

- 避免在 Operation 层面引入拓扑路由的复杂性
- 复用现有 ExecutionPlan → Step 的分解逻辑
- 保持与现有 DAG 调度兼容

### D5: DB 迁移最小化

**决策**：`runtime_targets` 表增加 `edge_type VARCHAR(32)` 和 `edge_config JSONB` 两个字段；`execution_plans` 表增加 `node_group_affinity TEXT[]`。不在本 change 中引入新的边缘专用表。

**理由**：T3 POC 阶段优先验证端到端路径，数据模型保持轻量。后续 Device Mapper 和离线 Bundle 可能需要独立表。

## Risks / Trade-offs

| 风险 | 缓解措施 |
|---|---|
| KubeEdge 版本与 HNB 兼容性 | Edge Runtime Provider 启动时检测 CloudCore API 版本，不兼容时拒绝注册 |
| 边缘节点断连期间 Operation 状态不一致 | 复用现有 fencing 机制：Lease 过期后允许更高 generation 接管；重连后对账 |
| EdgeApplication CRD 在不同 KubeEdge 版本间差异 | Provider 封装版本适配层，对 Worker 暴露统一接口 |
| K3s 嵌入 etcd-less 限制（如不支持 Lease） | 验证 K3s 的 Kubernetes API 兼容性；HNB 不依赖 etcd 特有功能 |
| 边缘集群规模对平台 API 的压力 | 边缘状态上报通过 Projector 异步写入 Read Model，不阻塞 Operation 主路径 |