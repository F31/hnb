## Why

HNB Cloud 的 edge-pack 规格定义了零侵入边缘计算能力（EDGE-001~006），但自 V3.8.6 基线以来从未有 change 落地。当前平台已具备 Kubernetes Provider（KRP-001~006）和 Runtime Driver v2（RDI-001~005），可以直接支持 K3s 等轻量 Kubernetes 发行版，同时需要新增 Edge Runtime Provider 来对接 KubeEdge 的边缘自治和设备管理能力。用户需要统一的边缘应用治理，而不是每个边缘集群独立管理。

## What Changes

### New

- **EdgeRuntimeTarget 具体化**：在 runtime-target-engine 中使 EdgeRuntimeTarget 成为可注册、可发现的运行目标类型，支持 K3s（通过现有 Kubernetes Provider）和 KubeEdge（通过新 Provider）两种接入模式
- **K3s 作为受支持 KubernetesTarget**：K3s 零集成成本，通过现有 Kubernetes Provider 直接管理；仅在 RuntimeTarget 注册时增加发行版识别和兼容性标注
- **Edge Runtime Provider（KubeEdge）**：新建 Provider 实现 Runtime Driver v2 契约，将 HNB 的 deploy/delete 映射为 KubeEdge EdgeApplication CRD，支持边缘节点组、离线自治和断连对账
- **边缘节点组与批次管理**：在 Operation 管道中支持 NodeGroup 概念，用于灰度部署和 OTA 批次控制

### Modified

- **runtime-target**：RT-001 的 EdgeRuntimeTarget 从抽象定义具体化为可注册类型；RT-002 的 KubeEdge 隧道接入方式从文字描述变为可验证行为
- **composition-operation**：OP-006 的 ExecutionPlan 需支持 NodeGroup 亲和性约束

### Not in Scope

- Device Mapper 框架和设备治理（EDGE-005）——留给后续 change
- 离线 Bundle 签名导入（EDGE-006）——留给后续 change
- 多集群 Federation（Karmada 等）——与 KubeEdge 不重叠

## Capabilities

### New Capabilities

- `edge-runtime-target`: EdgeRuntimeTarget 的具体注册、能力发现和状态上报
- `edge-runtime-provider`: 对接 KubeEdge 的 Runtime Driver v2 Provider，将 HNB 模型翻译为 EdgeApplication CRD
- `edge-node-group`: 边缘节点组定义、灰度批次和健康门禁策略
- `k3s-support`: K3s 发行版识别、兼容性标注和零成本接入

### Modified Capabilities

- `runtime-target`: 扩展 EdgeRuntimeTarget 为可注册类型，补充 KubeEdge 连接方式
- `composition-operation`: ExecutionPlan 增加 NodeGroup 亲和性字段

## Impact

- **新二进制**：`cmd/edge-provider/`，独立的 Go 二进制，通过 Runtime Driver v2 与 Worker 通信
- **新包**：`internal/edge/` 包含 KubeEdge 客户端适配、EdgeApplication 翻译、节点组管理
- **修改包**：`internal/runtime/` 扩展 EdgeRuntimeTarget 注册逻辑；`internal/operation/` 的 ExecutionPlan 增加 NodeGroup 字段
- **数据库迁移**：`runtime_targets` 表增加 `edge_type` 和 `edge_config` 字段；`execution_plans` 表增加 `node_group_affinity` 字段
- **无新中间件**：KubeEdge 需要 CloudCore 已在集群中运行，HNB 不部署或管理 KubeEdge 自身
- **Tier**：T3 POC，先验证断连自治，再生产化