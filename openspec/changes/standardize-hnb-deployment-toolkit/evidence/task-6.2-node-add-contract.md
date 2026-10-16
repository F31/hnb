# 6.2 node add / 平台升级服务端动作契约

- task: 6.2
- requirements: DPLY-006, DPLY-008
- date: 2026-10-14
- status: done (contract + matrix + routing); 执行开放随 6.3 provider 实现与 conformance 登记

## 平台升级（既有契约核实）

`UpgradeRuntimeTarget` 的服务端契约此前已落地并经 5.9 conformance 绑定：
intent kind + 校验（`intent.go`）、matrix cell `upgrade: REQUIRED`、
plan 步骤 `runtime_target.kubernetes.upgrade`（golden fixture）、
provider `Apply → upgrade`、worker fencing/幂等全链路。本轮核实并保留；
6.4 的 CLI diff/计划端与服务端契约互补，无重叠缺口。

## node add 新增契约（本轮交付）

- 契约 Schema：`runtime-intent.schema.json` kind 枚举新增 `AddNodesRuntimeTarget`
  （沿用集群类 kind 模式：要求由服务端校验承载，不新增 allOf 分支，oasdiff 仅告警不阻断）；
  `compatibility-matrix.schema.json` actions 增加 `node-add`；
  `execution-plan.schema.json` action 枚举增加 `node-add`。
  生成绑定经 `generate-contracts.mjs` 重生成，`validate-contracts.mjs` 全门禁通过
  （schema/compatibility/drift；80 操作、73 schema）。
- 能力矩阵：canonical + 内嵌矩阵两行均登记 `node-add: UNSUPPORTED`（基线诚实状态：
  在 6.3 provider 实现与动作级 conformance 证据登记前，路由按矩阵拒绝，
  返回稳定 `TARGET_ACTION_UNSUPPORTED`，不产生节点动作）。
- 引擎（platform-api）：`IntentAddNodesRuntimeTarget` 进入 kind 枚举/valid 表/
  `requiresReleaseID=false`/`IsClusterIntentKind`；`validateClusterIntentSpec` 分支强制
  UUID targetId、expectedVersion≥1、TargetRef 绑定，并以
  `validateNodeAdditions` 固化服务端拥有的载荷形状：parameters 仅允许 `nodes`
  （1..100 个对象，恰有 name/role，RFC-1123 子域名节点名，仅 worker 角色，
  拒绝重复名、额外键、客户端 steps/providerId 等执行字段）。
- 执行路由：`lifecycleAction → "node-add"`；`lifecycleSteps` 生成
  `runtime_target.kubernetes.add-nodes` 单步（`monotonic-worker-lease-v2` fencing、
  step 级幂等键、Retry 3×exponential、timeout 1800s、compensation none），
  parameters.nodes 随 inputs 进入语义摘要，复用既有 Operation 幂等/fencing，无新机制。
- IAM/BFF：`ClusterActionForIntentKind("AddNodesRuntimeTarget") → ActionUpdate`；
  apiserver BFF `clusterIntentKinds` 放行 + `validateBFFClusterIntent` 轻校验
  （UUID、expectedVersion、parameters 恰含 nodes）后转发正式入口。
- store：`intentKindToOperationType → "deploy"`；intent 文档随 parameters 持久化。

## 验证

- 引擎负向矩阵：默认矩阵下 `AddNodesRuntimeTarget`（Kubernetes/Edge）提交 →
  `TARGET_ACTION_UNSUPPORTED`（409），不产生 Plan/Operation（`TestPlannerRefusesNodeAddOnDefaultMatrix`）。
- 计划形状：测试专用 REQUIRED 矩阵下步类型/fencing/幂等键/timeout/compensation/
  inputs.nodes/decision 全部锁定（`TestPlannerShapesNodeAddStepUnderRequiredMatrix`）。
- 载荷负向：13 例（缺 nodes、额外参数、空表、非列表、非对象、额外字段、
  客户端 steps、缺 role、control-plane 角色、非法名、超长名、重名、>100 节点）
  全部以明确字段拒绝（`TestParseRuntimeIntentRejectsInvalidNodeAdditions`）。
- 客户端执行字段禁令维持：`forbiddenIntentParamKeys`（steps/providerid 等）+
  schema propertyNames 负向模式 + `scanRuntimeIntentExecutionFields` CI 扫描均通过。
- 合同门禁 `node scripts/validate-contracts.mjs` PASS；`scripts/contracts.test.mjs` 25/25。
- Go 回归：platform-api / apiserver / hnbctl（CLI+toolkit）/ pkg/iam 全部通过；
  CLI `node add` 保持本地拒绝（`TARGET_ACTION_UNSUPPORTED`），等待 6.3 开放真实提交。

## 未覆盖面

- 真实节点加入执行（join 步骤、观察后验收、已入集群节点重放）属 6.3，
  需 provider `Apply` 实现与动作级 conformance 登记（对齐 5.9 流程）后，
  将矩阵 `node-add` 翻为 REQUIRED 并绑定 worker；真机验证归独立环境（5.4/5.5 同批）。
