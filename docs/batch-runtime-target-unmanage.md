# 批量解除纳管：父子 Operation 契约

批量解除纳管不是数据库事务。平台必须把批次建模为一个父 Operation，并为每个集群建立独立的 `DeleteRuntimeTarget` 子 Intent / 子 Operation。任何一个子项失败不回滚已经成功的子项。

## 必须新增的契约

1. `POST /api/v1/runtime-intent-batches` 接收有界的 `targetIds`、风险确认、幂等键和关联 ID。
2. BFF 创建或转发一个 `BatchDeleteRuntimeTargets` 编排请求；平台 API 持久化父 Operation 与子项关系。
3. 父状态按子项聚合：`PENDING`、`RUNNING`、`PARTIAL_SUCCEEDED`、`SUCCEEDED`、`FAILED`、`CANCELLED`。
4. 每个子项仍是既有 `DeleteRuntimeTarget`，保留 fencing、审批、重试、审计和 Provider 执行路径。

## 前置条件

每个 target 必须逐项校验平台管理员权限、有效集群访问、`TenantClusterAllocation`、命名空间、扩展部署和工作负载依赖。阻断项在创建父 Operation 前返回预检结果；可执行项才创建子 Operation。取消父操作只阻止尚未开始的子项。

## 扩展实例

扩展实例是平台实体，不能由 Workspace 拥有。批量解除纳管的预检需检查目标集群是否承载平台扩展实例的 deployment binding；存在时必须先迁移或卸载绑定，再允许解除纳管。
