# 契约门禁处理：RuntimeIntent node-add

- 日期：2026-10-16
- 关联：`standardize-hnb-deployment-toolkit` 6.2（node-add）未提交工作；`deliver-lightweight-platform-product-loop` 9.2。
- 基线：`origin/main` = `e6603d352b92f385f65dfefa54eab4fc34928cc7`（远端 `git ls-remote` 确认一致，非本地过期）。
- 结论：门禁已恢复为**通过**（compatibility=checked）；未弱化门禁本身，未使用忽略项。

## 阻塞现象

`HNB_CONTRACT_TOOLS_OFFLINE=1 node scripts/validate-contracts.mjs` 在 compatibility 阶段失败：

```
error [request-body-all-of-added] at .../openapi/platform/v1/openapi.yaml
  in API POST /v1/runtime-intents
    added `subschema #5, subschema #6` to the request body `allOf` list
```

`generate-contracts.mjs --check` 同时报告 4 个生成物漂移文件（platform/runtime-target 的 Go 与 TypeScript 产物）。

## 定位

- `origin/main → HEAD` 的 `runtime-intent.schema.json` 只新增了 `credentialSecretRef`、`sshSecretRef` 两个可选字段，非破坏性。
- 引入枚举 `AddNodesRuntimeTarget` 与请求体 `allOf #5/#6` 的是**工作树未提交**改动。
- `oasdiff` 会递归标记任何在请求体新增的条件节点：
  - 根 `allOf` 增长 → `request-body-all-of-added`；
  - 根级 `if/then` → `request-body-if-added` / `request-body-then-added`；
  - 既有 `then` 内嵌 `allOf` → `request-property-all-of-added`；
  - `patternProperties` → `request-property-pattern-property-added`。
- 通过 `additionalProperties.oneOf` 放宽参数值类型 → 命中 `response-property-list-of-types-widened`（因 `RuntimeIntent` 同时用于响应体）。
- `oapi-codegen` 在 `properties` 与 `additionalProperties` 并存（尤其 union/`$ref`）时生成**无法编译**的指针/值不匹配代码。
- 契约治理（`docs/公共契约开发与演进指南.md`）允许同主版本“增加可选字段/枚举值”，禁止“改变字段类型/格式或既有语义”。

即：`kind` 条件约束是 `oasdiff` 无法接受的表达方式，而 `parameters` 值类型放宽会改变响应契约。

## 处理

按治理“同主版本纯增量”重写 `AddNodesRuntimeTarget` 的表达，避免新增任何条件节点：

1. 保留 `kind` 枚举新增 `AddNodesRuntimeTarget`（允许项）。
2. 保留 `credentialSecretRef`、`sshSecretRef`。
3. 节点形状以 `$defs.workerNodes` + `parameters.properties.nodes` 声明，而不是新增 `allOf`。
4. 移除 `parameters` 的 base `additionalProperties` 标量联合，`Nodes` 作为显式可选属性。

`role:"worker"`、RFC-1123 名称、≤100、去重等**运行时形状仍由引擎强校验**（`cmd/platform-api/internal/engine/intent.go:698` `validateNodeAdditions`；`IntentSpec.Parameters` 为 `map[string]any`，JSON Schema 不参与运行时校验）。fixtures `runtime-intent.node-add.{valid,invalid}.json` 的“接受/拒绝”语义保持不变。

最终 `runtime-intent.schema.json`：`sha256:c2c8f78c825707bd823b5f83ddbea31633a98b929c183f73816759bf44f22b5c`。

## 取舍

- 代价：`parameters` 不再在 Schema 层限定“值为标量”；旧 kind 的自由参数从“标量联合”放宽为“任意值”。此为**放宽**（非破坏），`oasdiff` 与 JSON-Schema 兼容规则均不报错，治理未列为禁止项。
- 备选（未采用，供确认）：保留原 `allOf #5/#6` 的逐 kind 精确约束，改为给门禁增加**路径范围、文本匹配的 `oasdiff --err-ignore`**。代价是修改门禁机制本身。若团队更看重 Schema 精确度，可切换此方案。
- 未采用的其它方案：`patternProperties`/`additionalProperties.oneOf`（被标记）；`properties`+`additionalProperties` 并存（codegen 无法编译）；`spec.nodes` 顶层字段（改变线上形状，需改引擎）。

## 验证

| 命令 | 结果 |
|---|---|
| `HNB_CONTRACT_TOOLS_OFFLINE=1 node scripts/validate-contracts.mjs` | **通过**：80 operations、5 APIs、36 messages、74 JSON schemas、compatibility=checked |
| `npm run contracts:test` | 25/25 通过（含 node-add fixtures 接受/拒绝与 `steps` 走私拒绝） |
| `npm run readiness:test` | 44/44 通过 |
| `node scripts/generate-contracts.mjs --check` | 生成物与源一致（drift 阶段通过） |
| `go build ./...`（`cmd/platform-api`、`cmd/hnbctl`） | 退出 0 |

## 方案 B 前提：引擎层参数严格化（2026-10-16 追加）

采用方案 B 后，`parameters` 不再由 Schema 限定标量，需把“拒绝嵌套执行语义”收拢到引擎层：

- 新增 `validateScalarParameters(kind, parameters)`（`cmd/platform-api/internal/engine/intent.go`），要求所有非 `AddNodesRuntimeTarget` 的参数值必须为标量（string/number/bool/null），拒绝对象/数组；`AddNodesRuntimeTarget` 的 `nodes` 列表仍由 `validateNodeAdditions` 严格校验。
- 在 `ParseRuntimeIntent` 与 `IntentValidator.Validate`（`ValidateWithScope` 复用）两处入口调用，覆盖 release 与 cluster 类 intent。
- 顶层执行字段黑名单（`forbiddenIntentParamKeys`）保持不变；本次补齐的是**嵌套**走私路径。

测试（`cmd/platform-api/internal/engine`）：

- 新增 `TestParseRuntimeIntentRejectsNestedParameters`（release 与 create-target 的嵌套对象/数组均被拒）。
- 新增 `TestParseRuntimeIntentAcceptsScalarParameters`（标量参数接受；node-add `nodes` 列表仍接受）。
- `go test ./...`（`cmd/platform-api`）全绿；`gofmt -l` 干净；`go vet ./...` 仅报既有无关项 `internal/config/config.go:123` self-assignment。

## 备注

- 该修正作用于未提交工作树；`node-add` 契约仍属 `standardize-hnb-deployment-toolkit` 6.2，正式提交时应在该变更评审该增量建模。
- 建议在契约治理文档补充一条：**为标签联合新增 `kind` 时，kind 专属约束应放在引擎校验，Schema 只做纯增量**，以避免与 `oasdiff` 的保守规则冲突；否则应走主版本升级。
