# 产品闭环 S0 首批开发验证

- Change：`deliver-lightweight-platform-product-loop`
- Run ID：`product-loop-s0-development-20261016`
- Requirement：PREADY-001、PREADY-004、PREADY-006、PREADY-007、GOV-002、PSHAPE-002
- 类型：本地开发/契约/单元测试，**不是 S0 阶段退出、业务 E2E、安装授权或生产认证**。
- 源码基点：`5e7fc07edd3eb4c1d627ef4c463f9d527676cc93`；工作树有既有未提交修改，
  本批新增代码身份按下列文件字节摘要记录，不能只用 HEAD 标识。
- 环境：Linux / Node.js 22.23.1 / OpenSpec 1.3.1；根工作区已安装的 Ajv/工具链；
  `web/` 为当前仓库 pnpm workspace。没有部署或修改任何远端平台。

## 本批交付

1. `docs/acceptance/product-loop-baseline.md`：源码、registry、原 change 任务的复用/缺口/owner 表，
   更正 Portal 缺失假设，记录独立目标、CSI/Gateway、三域环境和批准预算的实际阻塞。
2. `docs/deployment/profile-composition-contract.md`：四档产品/既有标识/overlay 映射、
   统一目录、能力增减/升档/软件版本的区分，以及实际能力/配置保留断言。
3. Readiness JSON Schema、只读 CLI 与拒绝路径测试：预期计划独立于运行证据，
   固定源码/配置/发布/环境范围，核对报告字节、阶段最小断言、证据层和数值预算。
4. `npm run contracts:check` 增加 readiness 测试步骤；未修改在线 RuntimeIntent、
   安装 EvidenceGate、Provider 路由或现有认证值。

## 最终验证结果

| 命令 | 结果 | 可证明的范围 |
|---|---|---|
| `npm run readiness:test` | 退出 0；44 tests passed，0 failed | Schema/语义、缺报告、篡改摘要、源码/配置/环境不匹配、自导入、旧证据、低层提升、预算缺失/超限/样本不足、CLI 非零/脱敏及只读行为 |
| `npm run contracts:test` | 退出 0；25 tests passed，0 failed | 现有公开契约规则/生成边界回归；不等于全契约发布门禁通过 |
| `pnpm --dir web exec vitest run packages/api-client/src/__tests__/api-client.test.ts packages/schema-engine/src/__tests__/closure.test.ts` | 退出 0；2 files / 18 tests passed | 共享 client 与 Schema 受限端点的本地测试，不是浏览器实机 |
| `node scripts/validate-openspec.mjs` | 退出 0；34 items，48 specs / 287 requirements / 476 scenarios | 全仓严格规格与稳定 ID/Traceability/场景语义 |
| 本批已跟踪文件的 `git diff --check` | 退出 0 | 空白检查 |
| `HNB_CONTRACT_TOOLS_OFFLINE=1 node scripts/validate-contracts.mjs` | 见后注（当时退出 1） | 首跑被既有未提交 RuntimeIntent node-add 契约阻断；随后处理，见下一节 |
| `HNB_CONTRACT_TOOLS_OFFLINE=1 node scripts/generate-contracts.mjs --check` | 见后注（当时 4 项 drift） | 漂移在契约门禁处理时一并消除 |

## 全仓门禁阻塞细节（已处理）

首跑 `validate-contracts.mjs` 在 compatibility 阶段阻断：`origin/main` 的
`POST /v1/runtime-intents` 被新增请求体 `allOf` subschema #5/#6，报
`request-body-all-of-added`；`generate-contracts.mjs --check` 另报 4 项生成漂移。

根因是**工作树未提交**的 `AddNodesRuntimeTarget` 契约改动，而非本批新增的离线
readiness Schema。已按契约治理“同主版本纯增量”重写该表达（枚举值 + `parameters`
可选 `nodes` 属性，去掉条件节点），并重生成产物。处理结论、取舍与证据见
[`20261016-contract-gate-runtime-intent.md`](20261016-contract-gate-runtime-intent.md)。
处理后：完整门禁通过（compatibility=checked）、`contracts:test` 25/25、drift 干净、
`cmd/platform-api` 与 `cmd/hnbctl` 编译通过。未修改门禁本身、未使用忽略项。

## 最终被测代码身份

| 文件 | sha256 |
|---|---|
| `contracts/schema/readiness/v1/product-readiness.schema.json` | `0981a608ef1708e0f6760d076cf517c5cb2cd005c49adee40d45824e419e563f` |
| `scripts/validate-product-readiness.mjs` | `d83f2f687ef4364a6abe8c552eed8464e0229193b96e0c0314a81a70ddbcf411` |
| `scripts/validate-product-readiness.test.mjs` | `9a1edb6ca5b8cca6d04b1213e2d213ee7ff93fc93d51a657606c5dccadb0a2a7` |

## 任务与下一门

- 完成 1.1、1.7，以及从原 1.6 拆出的 1.8/1.9；不以子任务完成勾选真实参考应用。
- 1.2 已找到前端，但人员维护者、构建物料与真实联调入口待确认。
- 1.3 需独立业务集群及实际 CSI/Gateway/TLS 条件；HA 需三实际故障域。
- 1.4 数值预算待批准；测试 fixture 的限制不是产品承诺。
- 1.5 的在线身份/入网、应用及实际安装态契约尚未全部冻结；1.6 实际应用三版本 digest 待固定。
- S1 按规格在身份契约/预算明确后开始；本批没有冒称服务发行/续期或正式业务闭环已完成。
