# 4.4 边界契约测试 + 专用空白管理基座 bootstrap 适配

- task: 4.4（机制完成；install 实际执行仍由 4.10 + 5.4/5.5 门禁）
- requirements: PINST-001、DPLY-004、DPLY-006、DPLY-008、CFG-002
- date: 2026-10-12（契约测试）；2026-10-14（适配器实现与验证）
- status: done (mechanism)；bootstrap 路线对用户入口保持关闭，未解除 1.2 边界决策

## 2026-10-14 增量：内部候选适配器 `ManagementBootstrap`

前置说明：本 change 曾决定 bootstrap 路线在完整平台 E2E 前保持 blocked（见下文
"2026-10-12 记录"）。该决策**未被放宽**：适配器是 `internal/toolkit` 内部候选实现，
`hnbctl` CLI 无任何引用（`grep -rn ManagementBootstrap cmd/hnbctl/internal/cli/` 为空），
`install`/`run` 对 bootstrap 仍不产生节点动作；实际执行与路线开放由 4.10
（完整档位 E2E + 发布物料条件）与 5.4/5.5 独立环境证据门禁。

### 实现（`cmd/hnbctl/internal/toolkit/management_bootstrap.go`）

- 构造期边界拒绝：非 bootstrap 模式/existing 策略 → `BOOTSTRAP_BOUNDARY_REFUSED`；
  mirror artifacts、多节点、非控制面角色 → `BOOTSTRAP_ROUTE_UNSUPPORTED`；
  锁校验/兼容失败或缺节点镜像 → `BOOTSTRAP_LOCK_INVALID`；显式源缺失 →
  `BOOTSTRAP_SOURCE_INVALID`。配置/源/锁以 JSON 快照绑定，防调用方后续突变。
- 身份探测（SSH 单脚本）：machine-id、kubeadm 版本、`/etc/hnb/bootstrap/owner`
  claim、RKE2/K3s 生命周期标记、admin.conf 集群 UID。外部 claim →
  `BOOTSTRAP_OWNER_PRESENT`；版本/机器/标记不符 → `BOOTSTRAP_IDENTITY_CONFLICT`；
  resume 时集群 UID 变更 → `BOOTSTRAP_CLUSTER_CHANGED`（拒绝收养或重置）。
- 永久 claim = sha256(run id + config digest + lock content digest + machine-id)，
  由远端 guard 在首段写入 `/etc/hnb/bootstrap/owner`（0600）；
  epoch 单调递增 fencing（`/etc/hnb/bootstrap/epoch`），第二 writer/epoch 回退拒绝。
- 阶段执行复用 4.1 共享脚本（node-prereq/runtime/init/network/cluster-verified），
  失败仅重试失败阶段；成功阶段 resume 只 recheck 不重跑（init 永不重放）。
  run 以 `RoutingMode=bootstrap` 记录 ownership（node/machine-id/claim），
  平台未验证前不 Finalize（成功 ≠ 完整交付）。
- kubeconfig 经 `AdminKubeconfigScript` 取回后以 0600 附件写入 run 目录
  （`management.kubeconfig`），不是平台 Secret 记录。

### 验证

- `boundary_contract_test.go` 3 项 1.2 拒绝契约（配置 platform/target 引用、
  管理/生命周期标记、节点版本冲突）持续通过。
- `management_bootstrap_test.go`：完整运行 + resume 仅 recheck（init 零重放、
  epoch 合法推进）；runtime 失败后重试仅重执行失败阶段（成功 prereq 只 recheck）；
  身份/边界/锁拒绝矩阵与替换集群 resume 拒绝。
- `cd cmd/hnbctl && go test ./... -count=3`（含随机 map 迭代序）PASS；
  `go vet ./...` PASS；`pkg/provision` 回归 PASS。
- 环境注意：t.TempDir() 在本机（WSL DrvFS）返回 0755，测试以 `privateRunDir`
  显式 chmod 0700 满足 run store 私有目录要求；`LoadRunState` 缺失态错误改用
  `%w` 包装，全新 bootstrap 不再误报 `BOOTSTRAP_STATE_UNSAFE`。

### 未覆盖面

- 真机 Day-0 与恢复 E2E（含跨进程/掉电 durability）归 5.4/5.5；
  install 实际执行与路线开放由 4.10 门禁；node preload 授权接通归 4.2。

## 2026-10-12 记录：1.2 遗留拒绝场景契约测试固化

## 4.4 为何未实现

任务描述为"4.4 **在 1.2 边界通过后**实现专用空白管理基座 bootstrap 适配"。
该前置条件**不成立**：

| 出处 | 原文 |
|---|---|
| `evidence/task-1.2-boundaries.md:37` | bootstrap 边界**无法由方法级 provider 证据证明**为完整平台安装（完整 Minimal 平台 E2E 未做），本期**只放行 existing 路线**；bootstrap 保留 blocked，等 **5.4/5.5** 独立环境完整平台 E2E 后再开放 |
| `design.md:56` | 不能证明该边界时**先交付已有 Kubernetes 安装路线** |
| `design.md:232` | 边界不可证明则**延期该路线** |
| `design.md:258` | 该路线保持 blocked，**本 change 不静默放宽 KERNEL-002** |

现在实现 bootstrap 适配等于在完整平台 E2E（5.3/5.4/5.5）缺席时解除 Day-0 限制，
与本 change 自身决策相悖。经确认，本轮选择固化 1.2 遗留的边界契约测试，
**强化而非放宽**边界。

## 1.2 遗留工作的完成

`evidence/task-1.2-boundaries.md:22` 记录："这些拒绝场景在实现 install/cluster
命令时以 Go 契约测试固定"。install/cluster/node 命令现已存在，本轮将该承诺落地。

新增 `cmd/hnbctl/internal/toolkit/boundary_contract_test.go`：
- `TestBootstrapConfigWithPlatformTargetReferenceIsRefused`：mode=bootstrap 且携带
  `platform.targetRef` 在**解码期**即被拒（严格未知字段拒绝），且不留任何副作用文件。
- `TestBootstrapRefusesNodesCarryingManagementOrLifecycleMarkers`：节点带
  `owner=hnb|kubernetes|rke2|k3s` 标记时 `node.owner-markers` 为
  `blocked / NODE_OWNER_PRESENT`，并给出正确管理入口（"do not bootstrap or reset"）。
- `TestBootstrapRefusesNodeVersionConflict`：节点已运行其它 Kubernetes 生命周期时不得被
  静默接管；API 源路径观测不因 owner 冲突被提升为通过。
- `TestManagedModeNeverFallsBackToLocalExecution`：managed 因缺少租户绑定契约被结构拒绝，
  且不产出任何安装计划。
- `TestSharedPlaneCredentialReferenceIsRefused`：两个自有平面复用同一 NATS 凭据引用被拒。

新增 `cmd/hnbctl/internal/cli/boundary_contract_test.go`：
- `TestManagedRequestReturnsConnectionLevelRefusalNotLocalFallback`：managed 请求以
  `mode=managed` 理由非成功返回，不含 `operationId`、本地执行指令，不写任何文件。
- `TestClusterCreateDoesNotFabricateOperation`：cluster create/import 未实现时非成功，
  不伪造 operationId 或成功状态（幂等重放属 4.7）。
- `TestNodeAddReturnsTargetActionUnsupported`：node add 返回退出码 3 +
  `TARGET_ACTION_UNSUPPORTED`。
- `TestBootstrapInstallRefusalLeavesNoRunArtifacts`：被拒的 bootstrap 不产生
  receipt/run state/plan 文件。

## 实际测试结果

```text
cd cmd/hnbctl
go test ./... -count=1            PASS（CLI / toolkit）
go vet ./...                     PASS
go test -race ./... -count=1      PASS（CLI / toolkit）
```

## 未覆盖面（保留原状态）

1.2 表格中"同义 CreateKubernetesTarget 幂等（idempotencyKey 重放）"与"跨租户
SecretReference 引用"需要服务端动作契约，属 4.7；当前仅断言其非成功且不伪造输出，
**不**冒充幂等性已验证。

## 解锁条件

4.4 需在 5.3（已有 Kubernetes 完整平台 E2E）与 5.4/5.5（独立 Ubuntu/Rocky
bootstrap/恢复）产出方法级之上的完整平台证据后方可实现。在此之前
`hnbctl install` 对 bootstrap 路线持续返回 blocked，不生成任何节点动作。
