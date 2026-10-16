# 4.3 已有 Kubernetes 安装依赖阶段与所有权清单（规划器交付）

- task: 4.3
- requirements: PINST-001、PINST-002、PINST-007
- date: 2026-10-12
- status: done（依赖阶段规划器 + 所有权清单）；实际 apply 与 Minimal 平台 E2E 属 5.3
- 交付范围：经用户确认采用"规划器+所有权清单"深度，不在本任务实现集群写入执行。

## 实现

新增 `cmd/hnbctl/internal/toolkit/ownership.go` 与 `existing_plan.go`：

### 所有权清单（`ownership-manifest/v1`）

严格区分三类资源，避免把"外部 owner"误当成 HNB 可删除范围：

- **Claimed（本次创建、卸载时才可处理）**：仅平台 namespace 内的 release、
  chart 生成的 RBAC/ServiceAccount/ClusterRoleBinding、以及 bundled PG/NATS 的 PVC。
- **RequiredExisting（操作者提供、必须保留）**：TLS Secret、StorageClass、每平面
  PG/NATS 凭据 Secret、external 模式的 backing Secret。清单显式列出这些引用，
  使卸载（6.6）不会隐式删除它们。
- **PreservedExternal（外部所有、永不触碰）**：外部 Kubernetes 生命周期 owner、
  外部 CNI daemonsets/deployments、namespace 外的 PV/业务工作负载。

清单由已校验的 `DeploymentConfig` 推导，不接受自由格式输入；一旦配置中出现
bootstrap 节点，`NoNodeInitAction` 立即置假并作为错误返回。

### 依赖阶段规划器（`existing-install-plan/v1`）

existing 路线固定为：validating → preflight → preparing → identity →
database-migration → infrastructure → platform-release → platform-verified →
handoff。每一阶段声明前置条件与是否只读。规划器：

- 只接受 `mode=existing` + `strategy=existing`，其余拒绝。
- backing 必须 bundled-both 或 external-both，混合模式无证据，拒绝。
- 固定 `routingMode: external-cluster cluster.reader/shadow`、`failureDomains: 1`、
  `singleNodeEntry: true`，不暗示 HA。
- 不生成任何 kubeadm init/reset、join、CNI 安装或节点初始化动作。

### CLI

`hnbctl install -f hnb.yaml [--lock ...] [--output plan.json]` 在 existing 路线下输出
计划 + 所有权清单（`--output` 走 O_EXCL 0600），随后返回 `PLATFORM_NOT_VERIFIED`，
理由含 `PLATFORM_NOT_CERTIFIED` 与 `NO_NODE_INIT_ACTION`。计划文本不含凭据或密钥。

## 实际测试结果

```text
cd cmd/hnbctl
go test ./... -count=1            PASS（CLI / toolkit）
go test -race ./... -count=1      PASS（CLI / toolkit）
go vet ./...                     PASS

python3 deploy/charts/hnb/test_candidate_components.py  13 tests OK
python3 deploy/charts/hnb/test_profiles.py               8 tests OK
openspec validate --all --strict                         29 passed, 0 failed
node scripts/validate-openspec.mjs                       31 specs / 242 requirements / 377 scenarios
git diff --check                                         PASS
```

验证要点：

- `TestExistingOwnershipPreservesExternalOwnerAndCNI`：外部 owner/CNI 在保留清单中；
  claimed 范围不含 CNI/node 生命周期；操作者 Secret 只出现在 required-existing，
  不被 claimed（卸载不删除）。
- `TestExistingOwnershipRejectsAnyBootstrapNode`：声明节点即破坏边界。
- `TestExistingPlanHasNoNodeInitAndPreservesExternalStage`：逐阶段扫描禁用词，
  无 kubeadm/join/CNI 安装；preflight/preparing/handoff 均为只读。
- `TestExistingPlanRejectsMixedBackingsAndProvisionStrategy`：混合 backing 与
  provision 路线被拒。
- `TestInstallExistingEmitsPlanAndRefusesAuthorization`：真实 CLI 执行验证
  退出码为 4（not verified）、文件权限 0600、输出含边界声明且不含 `kubeadm` 或密钥。

## 范围限制

本任务不执行任何集群写入。计划是"方法"层交付；实际 apply、状态观察与
Minimal 平台 E2E 属于 5.3。在 5.3 完成前 `install` 始终返回 PLATFORM_NOT_VERIFIED。
bootstrap（4.4/4.5）与授权节点 preload（4.2）仍未实现，install 对 bootstrap
路线保持 blocked。
