# 6.6 卸载 / 解除纳管 / 删除自建基座的独立入口

- task: 6.6
- requirements: PINST-007, DPLY-006
- date: 2026-10-12
- status: done（计划与门控）；实际删除执行仍走服务端契约，未实现

## 实现

新增 `internal/toolkit/removal.go` 与三个独立 CLI 入口：

```bash
hnbctl platform uninstall-hnb        --run-dir <dir> -f hnb.yaml [--acknowledge-storage-removal]
hnbctl platform unmanage             --run-dir <dir> -f hnb.yaml
hnbctl platform delete-management-base --run-dir <dir> -f hnb.yaml \
        --install-created-base --owned-base-uid <uid> [--acknowledge-storage-removal]
```

三者删除范围严格分离，**不合并为一个带参数的泛化删除命令**。

### 范围划分

| 模式 | 删除 | 保留 | 前提 |
|---|---|---|---|
| `uninstall-hnb` | 仅 ownership 清单的 Claimed（release、RBAC、SA、namespace 内容、自有 PVC） | 外部集群/CNI/业务负载、共享 PG/NATS、操作者提供的 Secret 与 StorageClass | 自有持久存储需 `--acknowledge-storage-removal` |
| `unmanage` | **不删除任何本地资源** | 平台 release 及全部外部资源 | 必须经正式平台 Intent 释放管理关系 |
| `delete-management-base` | release + 自建基座本身 | 全部外部资源 | 必须证明本次安装创建了该基座，且基座 UID 已观测绑定 |

### 安全不变量

- **PINST-007 保留语义**：ownership 清单中的 `PreservedExternal` 与
  `RequiredExisting` 在**任何**模式下都进入 `Retain`，并作为 `RetainedData`
  显式标记（客户 PVC、共享中间件、业务负载、外部存储）。
- **跨所有权删除被阻断**：即使 ownership 清单被污染（某保留项同时出现在
  Claimed 中），删除集与保留集相交即产生 `OWNERSHIP_VIOLATION` 并拒绝授权。
- **数据销毁需显式确认**：`uninstall-hnb` 删除自有持久存储时必须显式确认，
  否则 `PERSISTENT_STORAGE_REMOVAL_UNACKNOWLEDGED` 阻断。
- **run/配置绑定**：计划必须绑定记录的安装运行（按 preflight 同算法重算
  configDigest 并比对），防止用另一份配置对本次运行出具删除计划。
- **不本地执行**：即使计划已授权（`Authorized=true`），命令仍返回
  `PLATFORM_NOT_VERIFIED`——实际删除须走服务端 Operation 契约，不产生本地副作用。

## 验证要求对照

**共享 PG / 原 Kubernetes / 客户 PVC / 业务工作负载均按 ownership 保留**：

| 验证点 | 测试 |
|---|---|
| 卸载只删自有 release | `TestUninstallHNBDeletesOnlyOwnedRelease`、`TestPlatformUninstallPlanPreservesExternalOwner` |
| 共享 PG / 原集群 / 客户 PVC / 业务负载保留 | 同上 + `TestCustomerDataIsAlwaysRetained`（三种模式逐一验证） |
| 外部基座不得删除 | `TestDeleteManagementBaseRefusesExternallyOwnedCluster`、`TestPlatformDeleteBaseRefusesExternalCluster` |
| 自建基座需 UID 绑定 | `TestDeleteManagementBaseRequiresOwnedBaseUID` |
| 存储删除需确认 | `TestUninstallRequiresAcknowledgementForOwnedStorage`、`TestPlatformUninstallBlocksWithoutStorageAcknowledgement` |
| 解除纳管零本地删除 | `TestUnmanageDeletesNothingLocally`、`TestPlatformUnmanageDeletesNothing` |
| ownership 冲突阻断 | `TestRemovalNeverDeletesPreservedResources` |

## 实际测试结果

```text
cd cmd/hnbctl
go build ./...                   PASS
go test ./... -count=1            PASS（CLI / toolkit）
go vet ./...                     PASS
go test -race ./... -count=1      PASS（CLI / toolkit）
```

## 范围说明

本任务交付**计划与门控**：产出可审阅的删除/保留清单并强制前置条件。实际
删除动作需服务端 removal Operation 契约（6.2/6.4 路线），当前未实现，因此
`Authorized` 的计划也不会被执行。这符合 DPLY-006"不直接执行"。
