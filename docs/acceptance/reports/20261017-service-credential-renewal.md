# S1 服务凭据续期：实现与验证

- Run ID：`service-credential-renewal-20261017`
- Change：`deliver-lightweight-platform-product-loop`
- Requirement：IDENT-001（客户端续期部分）、任务 2.3；1.4/1.5 的 S1 子集
- 日期：2026-10-17（工具观察 `2026-10-17T06:56:00Z`）
- 类型：本地单元/竞态/契约验证。**不是** real_e2e、撤销时延认证、在线 API 冻结或生产资格。

## 1. 决策输入

用户在决策基线交付后指示“按规划继续执行”，据此采纳 `docs/deployment/decision-baseline.md`
修订 0.1 的 **S1-IDENTITY 开发输入**（基线摘要
`sha256:f0a52c4ae54c5e06e43f5a3501bc253181baf9cd92de9252149a3ab7121f1c1c`）。
仅 S1 数值作为实现/测试目标；生产容量、RTO/RPO、独立集群与整站环境仍 pending。

## 2. 本批交付

| 产物 | 内容 |
|---|---|
| `contracts/schema/identity/v1/service-credential-scope.schema.json` | 精确 subject/audience/single permission 的消费者范围契约；禁止通配符、空 resourceId、未知字段、非法层次；**不是** workload 身份证明 |
| `pkg/iam/service_renewal.go` | `ServiceCredentialIssuer` 接口 + `RenewingTokenSource`（实现既有 `TokenSource`） |
| `pkg/iam/service_renewal_test.go` | 伪时钟/并发/错误/重启/脱敏测试 |
| `scripts/contracts.test.mjs` | 新增范围契约接受/拒绝测试 |
| `docs/deployment/service-credential-renewal.md` | 消费者边界、失效语义与接入顺序 |

### 行为语义

- 首次 `Token` 才启动单个后台续期；冷启动 100 个并发调用共享一次签发。
- 到寿命 50% 启动续期，时刻抖动 ±5% TTL；失败退避 1s/2s/4s（±20%，硬上限 4s）。
- 单次签发 RPC 超时 2s；无法在旧凭据到期前完成时不启动该次尝试，过期后重新证明身份。
- 响应必须经既有 `TokenVerifier` 验签；拒绝 user token、错误 profile/audience/subject/tenant、
  多余 audience/action/permission、过期/未来签发/超 TTL，以及更宽的资源范围。
- `ErrServiceIdentityRevoked` / `ErrServiceCredentialRejected` 立即丢弃缓存并停止；
  其他错误保留旧凭据仅至原 `exp`，过期后 fail closed。
- 不持久化 token，不修改 verifier profile/TTL/权限，不泄漏 issuer 原始错误或凭据。

## 3. 验证

| 命令 | 结果 |
|---|---|
| `go test -race ./pkg/iam/...` | 通过（3.1s） |
| `go test ./pkg/iam/ -run TestRenewingTokenSource -v` | **33 PASS**（含子测试） |
| `go vet ./pkg/iam/...` | 无输出，退出 0 |
| `gofmt -l`（改动文件） | 干净 |
| `npm run contracts:test` | 26/26 通过（新增 scope 测试第 8 项） |
| `HNB_CONTRACT_TOOLS_OFFLINE=1 node scripts/validate-contracts.mjs` | 通过：80 operations、5 APIs、36 messages、**75 JSON schemas**、compatibility=checked |

## 4. 被验证的负向路径

- 服务凭据被当作 user token、错误/多 audience、错误 subject/tenant、多余 action/permission、
  过期、未来 `iat`、超 60s TTL、错误 profile、更宽 resourceId → 拒绝并停止。
- 临时 issuer 失败 → 旧凭据仅用到 `exp`，之后 `ErrServiceCredentialUnavailable`，不泄漏
  issuer 正文；恢复后必须取得新凭据。
- 永久撤销/拒绝 → 即使旧凭据未过期也不再返回。
- `Close`/父 context 取消 → 取消共享签发，之后不返回缓存。
- 非法 scope：通配符、空 resourceId、`resourceKind:"*"`、无父级的 namespace、非法字符 → 构造失败。

## 5. 明确未完成

- 2.1 owning-plane 迁移与原子消费/撤销存储：未做；本批 scope/消费者无数据库，不表示 S1 持久化 N/A。
- 2.2 在线 issue/renew/revoke API、独立权限、audit、5s 撤销：未做。
- 2.4 worker→Provider / BFF→领域 API 接线、重新回到在线 issuer：未做（当前 BFF 仍每请求 delegation）。
- 2.5–2.7 Agent 入网/轮换/长连接失效：未做。
- 2.8 真实跨 TTL/重启/撤销时延演练：未做；本报告不得提升 real_e2e/HA。

## 6. 被测身份

| 文件 | sha256 |
|---|---|
| `pkg/iam/service_renewal.go` | `149db43a0d53e87dead14ee33dae3d96a24e5def7e3dcb96aef90c0864fb4df5` |
| `pkg/iam/service_renewal_test.go` | `fed7d366b8e72d11089c8967028ea1008c9cbbc14749e814b87799c0da235282` |
| `contracts/schema/identity/v1/service-credential-scope.schema.json` | `4a8825d4a9df2ffd79536ce7afcdc398f11da97a03bf88a8a4ac94cf90b61080` |

工作树与 HEAD 仍不同，不能只用提交号标识；以上为最终被测试文件字节摘要。
