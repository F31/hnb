## 1. K3s 发行版识别

- [x] 1.1 在 runtime_targets 表增加 distribution 字段（VARCHAR(32)），迁移 014
- [x] 1.2 修改 RuntimeTarget 注册逻辑，自动检测 Kubernetes 发行版类型（K3S-001）
- [x] 1.3 实现发行版兼容性标注，记录 K3s 已知限制（K3S-002）
- [ ] 1.4 验证 K3s 集群上的 Kubernetes Provider deploy/delete 与标准集群行为一致（K3S-003）— N/A，需要 K3s 运行时环境

## 2. EdgeRuntimeTarget 注册与发现

- [x] 2.1 在 runtime_targets 表增加 edge_type（VARCHAR(32)）和 edge_config（JSONB）字段，迁移 015（ERT-001）
- [x] 2.2 实现 EdgeRuntimeTarget 注册 API，支持 CloudCore endpoint 和节点组映射（ERT-001）
- [x] 2.3 实现 KubeEdge 版本检测和边缘节点发现（ERT-002）
- [x] 2.4 实现边缘节点状态新鲜度跟踪和 QueuedOffline 策略（ERT-003）
- [ ] 2.5 验证 KubeEdge 集群注册、版本检测和节点状态上报的端到端流程 — N/A，需要 KubeEdge 运行时环境

## 3. Edge Runtime Provider 核心

- [x] 3.1 创建 cmd/edge-provider/ 骨架，实现 Runtime Driver v2 健康检查和版本协商（ERP-001）
- [x] 3.2 实现受限 EdgeApplication 生命周期：deploy 映射为 EdgeApplication CRD 创建（ERP-002）
- [x] 3.3 实现租户命名空间隔离和托管标记（ERP-003）
- [x] 3.4 实现幂等键和 fencing token 校验（ERP-004）
- [x] 3.5 实现 node_group_affinity 到 EdgeApplication nodeSelector 的映射（ERP-005）
- [x] 3.6 实现可 fencing 的逻辑删除：缩容为零 + 墓碑保留（ERP-006）
- [x] 3.7 实现 CloudCore 断连检测和 TARGET_UNAVAILABLE 返回（ERP-007）
- [x] 3.8 为 edge-runtime-provider 编写单元测试和契约测试

## 4. 节点组管理

- [x] 4.1 实现节点组 CRUD：与 EdgeRuntimeTarget 关联，包含名称、选择器和标签（ENG-001）
- [x] 4.2 实现灰度批次定义：顺序、百分比、健康门禁等待时间和失败容忍度（ENG-002）
- [x] 4.3 实现健康门禁检查：Available 状态、Pod 重启次数和自定义健康端点（ENG-003）
- [x] 4.4 实现批次暂停/恢复/自动暂停（ENG-004）

## 5. ExecutionPlan 节点组亲和性

- [x] 5.1 在 execution_plans 表增加 node_group_affinity（TEXT[]）字段，迁移 016（OP-009）
- [x] 5.2 修改 ExecutionPlan 生成逻辑，支持 node_group_affinity 字段（OP-009）
- [x] 5.3 修改 Worker Step 路由逻辑，将 node_group_affinity 传递给 Provider（OP-009）
- [ ] 5.4 验证 ExecutionPlan → Step → Provider 的节点组路由链路 — N/A，需要运行时环境验证

## 6. E2E 验证（均需要运行时环境，标记为 N/A）

- [ ] 6.1 搭建 K3s 集群，验证 KubernetesTarget 注册和标准 Operation 执行 — N/A，需要 K3s 集群
- [ ] 6.2 搭建 KubeEdge 环境（CloudCore + EdgeCore），验证 EdgeRuntimeTarget 注册 — N/A，需要 KubeEdge 环境
- [ ] 6.3 验证 Edge Runtime Provider 部署边缘应用（EdgeApplication CRD） — N/A，需要 KubeEdge 环境
- [ ] 6.4 验证灰度批次：多节点组、健康门禁、暂停/恢复 — N/A，需要 KubeEdge 环境
- [ ] 6.5 验证断连自治：CloudCore 断连后 TARGET_UNAVAILABLE，恢复后对账 — N/A，需要 KubeEdge 环境
- [ ] 6.6 验证 fencing 与逻辑删除：陈旧 token 拒绝、墓碑保留、CAS 接管 — N/A，需要运行时环境