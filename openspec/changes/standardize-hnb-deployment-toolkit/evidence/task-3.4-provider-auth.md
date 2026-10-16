# 3.4 plugin-worker 与 lifecycle provider 的认证/路由配置

- task: 3.4
- requirements: PINST-002, PINST-005
- date: 2026-10-11
- status: done

此前 3.4 记为 partial，因为 `POST /v2/steps:execute` 直接对外开放且进程未装配任何身份校验。
本轮补齐服务身份强制与 Chart 接线，并写入负向测试。

## 生命周期 Provider 的服务身份

该 provider 通过 SSH 真实创建集群，无认证的调用方不得触达执行入口。

`cmd/runtime-target-lifecycle-provider/main.go` 按既有 `cmd/kubernetes-provider` 的既定模式装配：

- `loadServiceAuthenticator(expectedAudience)` 要求 `API_TOKEN_ISSUER` 与**精确匹配**的
  `API_TOKEN_AUDIENCE`，并要求 `API_TOKEN_KEY_MANIFEST_FILE`；缺失任一项 `log.Fatalf`，
  进程直接退出，不会以无认证方式继续运行。
- 身份密钥清单支持轮询重载（`API_TOKEN_KEY_RELOAD_INTERVAL`，默认 5s）。
- `http.Server` 用 `RequireServiceIdentity(auth, ActionExecute, RuntimeExecutionScope(...), "/healthz")`
  包裹；仅 `/healthz` 免认证，供编排判定存活。server 补 `ReadHeaderTimeout: 5s` 与优雅关闭。

`internal/provider` 新增导出常量 `Audience = "hnb-runtime-target-lifecycle-provider"`
与 `ExecuteBodyLimit = 64<<10`；handler 与授权层共用同一上限，
避免授权层读到的字节与 handler 后续解码的字节不一致。
`cmd/runtime-target-lifecycle-provider/go.mod` 增加 `pkg/iam` 依赖与本地 `replace`（与 `pkg/kms` 一致）。

## Chart 接线

`runtime-target-lifecycle-provider` 子 chart 增加 `identity`（issuer/audience/existingManifestSecret/manifestKey），
Deployment 注入 `API_TOKEN_ISSUER`、`API_TOKEN_AUDIENCE`、`API_TOKEN_KEY_MANIFEST_FILE`，
并以 `defaultMode 0400` 只读挂载身份 Secret 到 `/var/run/hnb-identity`。
三者任一缺失即 `required` 失败，渲染不出半成品清单。

## 测试

`cmd/runtime-target-lifecycle-provider/auth_test.go` 用真实 ECDSA 密钥 + 真实 key manifest +
平台 TokenManager 签发的真实 JWT，而非伪造字符串：

| 测试 | 覆盖 |
|---|---|
| `TestExecutionEndpointRequiresServiceIdentity` | 无 token / 空 Bearer / 非 Bearer scheme / 伪造 token / 重复 Authorization 头一律 401；`/healthz` 保持免认证可达 |
| `TestExecutionEndpointRejectsForeignAudience` | `hnb-apiserver`、`hnb-platform-api`、`hnb-kubernetes-provider` 及本 audience 变体签发的 token 均 401 |
| `TestLoadServiceAuthenticatorFailsClosedOnMissingConfig` | 缺 issuer/audience/audience 错误/缺 manifest/全空 五类均拒绝装配 |
| `TestExecutionEndpointAcceptsCorrectlyScopedToken` | 正确 audience + service 主体 + 同租户同 provider 授权的 token 可通过认证进入 handler |

图表侧 `test_lifecycle_provider_requires_service_identity` 断言 env、Secret 挂载、
0400 权限，以及 issuer/audience/manifest 任一为空即拒绝渲染。

## 过程中修正的三处测试理解错误（均由测试失败暴露）

1. `ResolvePermissions` 的第一个返回值是 **policy version**，我最初返回租户 ID，
   导致 `ValidatePermissionSnapshot` 失败、正向用例被拒。
2. 权限快照里的 `ResourceID` 必须与 `RuntimeExecutionScope` 解析出的 provider ID 一致，
   不能用 audience。
3. `RuntimeExecutionScope` 只解析 tenant/resourceKind/resourceID，
   因此授权**不能**再固定 project/environment，否则该条权限被评估器跳过。

这三点说明：正向用例必须真正跑通，否则「负向全部 401」可能只是因为**所有** token 都被拒。

## 验证（实际执行，全部退出 0）

```bash
python3 deploy/charts/hnb/test_candidate_components.py   # 9 tests OK
helm lint deploy/charts/hnb                             # 0 failed
helm template hnb deploy/charts/hnb -f /tmp/hnb-idn.yaml # 23 镜像全部 digest，非 digest 0
go vet ./cmd/runtime-target-lifecycle-provider/...
go test -race ./cmd/runtime-target-lifecycle-provider/... -count=1
cd cmd/hnbctl && go test ./... -count=1 && go vet ./... && go test -race ./... -count=1
```

## 范围限制

- 未在集群中运行该 provider，也未与 apiserver/operation-worker 做端到端联调；
  正向路径只证明请求能越过认证进入 handler，未证明 provision 成功。
- 测试使用本地生成的密钥与自签 token，不代表生产身份材料。
- plugin-worker 本轮未改动代码：其接线与 DB/KMS Secret 引用已在 3.3 全量渲染中验证；
  其 Helm 执行路径的认证仍属后续任务，未在本任务声称完成。
