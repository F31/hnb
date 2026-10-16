# 1.4 owning plane API 盘点与 CLI 适配契约增量

- task: 1.4 盘点 owning plane 的现有公开 API、Secret 登记、兼容矩阵、Intent/Operation/观测接口
- requirement: DPLY-006, PINST-005
- date: 2026-10-11
- status: done

## 现有公开 API 盘点（contracts/）

### Console API `contracts/openapi/console/v1/openapi.yaml`

| 路径 | 用途 | CLI 适配 |
|---|---|---|
| `POST/GET /api/v1/resources/clusters` | 集群资源 | `cluster create/list` 只读展示 |
| `GET /api/v1/resources/clusters/{targetId}/nodes` | 集群节点观测 | `node list` 展示 |
| `GET /api/v1/dictionaries/{dictionaryId}` | 字典（版本等） | 兼容矩阵查询 |
| `POST /api/v1/runtime-intents` | 提交 typed RuntimeIntent | `cluster create/import/upgrade/unmanage` |
| `GET /api/v1/operations`、`/api/v1/operations/{id}`、`.../actions/{approve\|reject\|cancel}` | 执行事实与审批 | `operation get`、审批 |

### Platform API `contracts/openapi/platform/v1/openapi.yaml`

| 路径 | 用途 |
|---|---|
| `POST /v1/session/bootstrap`、`/v1/session/tenant-switch` | 会话引导/租户切换 |
| `POST /v1/runtime-intents`、`GET /v1/runtime-intents/{intentId}` | 服务端权威意图入口 |

### Schema 资产 `contracts/schema/`

- `platform/v1/runtime-intent.schema.json`：`kind` 枚举含 `CreateKubernetesTarget`、`ImportRuntimeTarget`、`UpgradeRuntimeTarget`、`DeleteRuntimeTarget` 等；`metadata.idempotencyKey`/`correlationId` 支持幂等。
- `runtime-target/v1/compatibility-matrix.json`：KubernetesTarget actions `create/import/upgrade/unmanage=REQUIRED`；EdgeRuntimeTarget create=UNSUPPORTED。
- `runtime-target/v1/`：capability/node/observation/storage-inventory schemas（Agent 观测 Read Model）。
- `common/v1/secret-reference.schema.json`：CLI 在线模式只保存凭据引用。

## 缺口与契约增量

| 缺口 | 处理 |
|---|---|
| 无公开 **Secret 登记** 端点（console openapi 只有 SecretReference schema） | 需在服务端增加 tenant-bound secret 登记/校验契约（新 delta），否则 CLI 在线凭据登记无法落地；本期标记为 blocked 前置 |
| 无 **node-add** 动作契约 | 兼容矩阵未声明；CLI 返回 `TARGET_ACTION_UNSUPPORTED`，不得客户端注入 providerId/steps |
| 无平台预检接口 | 本期 preflight 由 CLI 在已有 Kubernetes 路线上使用 kubeconfig 做只读检查（不新增 server 侧预检端点）；bootstrap 节点预检复用工具有限命令集 |
| 无大文件代理 | OCI 下载走内容寻址 URL/OCI 客户端，API 不代理包字节 |

## 负向契约测试设计

| 用例 | 期望 |
|---|---|
| 客户端尝试在 intents 外传 `parameters.steps`/providerId | schema 拒绝（runtime-intent.schema.json 无 steps 字段 + additionalProperties=false） |
| CLI 尝试直接 Provider execute | 无公开端点可调用；测试确认不存在该客户端路径 |
| CLI 直连平台 DB 写 Operation | 工具代码层面禁止（不引入 DB DSN 到 CLI）；`managed` 无本地 DB 路径 |
| 503 时回退 SSH | 拒绝，见 task-1.2 |

负向测试随 CLI 集群命令实现时以 Go 契约测试固定（task 4.7）。本任务完成盘点与增量定义。
